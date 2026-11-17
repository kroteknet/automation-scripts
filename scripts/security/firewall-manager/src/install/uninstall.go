// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: FIREWALL-MANAGER (security)
// File:    security/firewall-manager/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"krotek-firewall-manager-installer/core"
	"krotek-firewall-manager-installer/logger"
	"krotek-firewall-manager-installer/system"
	"krotek-firewall-manager-installer/tui"
)

// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	logger.InitUninstallLogger("firewall-manager")
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		if _, err := os.Stat("/etc/firewall-manager-docker"); err == nil {
			envType = "docker"
		} else {
			envType = "native"
		}
	}

	uninstallBackup := false
	uninstallPurgeFiles := true

	if !core.AutoYes {
		uStep := 1
		for {
			switch uStep {
			case 1:
				tui.ClearScreen()
				bRes := tui.AskYesNo("Create full configuration and data backup before uninstallation?")
				if bRes == 2 || bRes == -1 {
					logger.LogInfo("Uninstallation cancelled by user.")
					return
				}
				uninstallBackup = (bRes == 0)
				uStep = 2
			case 2:
				tui.ClearScreen()
				fRes := tui.AskYesNo("Purge FIREWALL-MANAGER configuration files and data directories?")
				if fRes == 2 {
					logger.LogInfo("Uninstallation cancelled by user.")
					return
				}
				if fRes == -1 {
					uStep = 1
					continue
				}
				uninstallPurgeFiles = (fRes == 0)
				uStep = 3
			case 3:
				tui.ClearScreen()
				sumLines := []string{
					core.BOLD + core.RED + "CONFIRM FIREWALL-MANAGER UNINSTALLATION:" + core.NC,
					"",
					tui.FormatKV("Deployment Environment", envType, core.CYAN),
					tui.FormatKV("Create Full Backup", func() string {
						if uninstallBackup {
							return core.GREEN + "YES"
						}
						return core.YELLOW + "NO"
					}(), core.GREEN),
					tui.FormatKV("Purge Files & Data", func() string {
						if uninstallPurgeFiles {
							return core.RED + "YES (PURGE ALL DATA)"
						}
						return core.GREEN + "NO (RETAIN)"
					}(), core.RED),
					"",
					core.BOLD + core.YELLOW + "Are you sure you want to proceed with uninstallation?" + core.NC,
				}
				confIdx, _, status := tui.SelectMenuWithContent("Step 3/3: Confirm Uninstallation", sumLines, []string{"YES, UNINSTALL NOW", "NO, CANCEL"})
				if status == 1 {
					uStep = 2
					continue
				}
				if status == 2 || confIdx != 0 {
					logger.LogInfo("Uninstallation cancelled by user.")
					return
				}
				uStep = 4
			}
			if uStep == 4 {
				break
			}
		}
	}

	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== UNINSTALLING FIREWALL-MANAGER STACK ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	var bkpFile string

	if envType == "native" {
		if uninstallBackup {
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile = fmt.Sprintf("/root/firewall-manager_backup_%s.tar.gz", ts)
			if !system.IsSymlinkPath(bkpFile) {
				var tarArgs []string
				for _, p := range []string{"/etc/firewall-manager", "/etc/sysctl.d/99-krotek-security.conf"} {
					if _, sErr := os.Stat(p); sErr == nil {
						tarArgs = append(tarArgs, p)
					}
				}
				if len(tarArgs) > 0 {
					_ = exec.Command("tar", append([]string{"-czf", bkpFile}, tarArgs...)...).Run()
					_ = os.Chmod(bkpFile, 0600)
				}
			}
		}

		if uninstallPurgeFiles {
			system.RunStep("Resetting kernel sysctl security tweaks", "rm", "-f", "/etc/sysctl.d/99-krotek-security.conf")
			_ = exec.Command("sysctl", "--system").Run()
			system.RunStep("Removing FIREWALL MANAGER configuration & data files", "rm", "-rf", "/etc/firewall-manager", "/var/log/firewall-manager")
		}
	} else {
		composeDir := state["COMPOSE_DIR"]
		if composeDir == "" {
			composeDir = "/etc/firewall-manager-docker"
		}

		if uninstallBackup {
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile = fmt.Sprintf("/root/firewall-manager_docker_backup_%s.tar.gz", ts)
			if !system.IsSymlinkPath(bkpFile) {
				if _, sErr := os.Stat(composeDir); sErr == nil {
					_ = exec.Command("tar", "-czf", bkpFile, composeDir).Run()
					_ = os.Chmod(bkpFile, 0600)
				}
			}
		}

		if uninstallPurgeFiles {
			_ = system.RunDockerCompose(composeDir, "Stopping and purging FIREWALL-MANAGER Docker stack", false, "down", "-v")
			_ = os.RemoveAll(composeDir)
		} else {
			_ = system.RunDockerCompose(composeDir, "Stopping FIREWALL-MANAGER Docker stack", false, "down")
		}
	}

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/firewall-manager")
	_ = os.Remove("/usr/local/bin/firewall-installer")
	var uninstallErrors []string
	if core.LogFilePath != "" {
		if data, err := os.ReadFile(core.LogFilePath); err == nil {
			lines := strings.Split(string(data), "\n")
			for i := len(lines) - 1; i >= 0; i-- {
				l := strings.TrimSpace(lines[i])
				if l == "" {
					continue
				}
				cleanL := core.StripANSI(l)
				if strings.Contains(cleanL, "[ERROR]") || strings.Contains(cleanL, "Failed!") {
					if len(cleanL) > 65 {
						cleanL = cleanL[:65] + "..."
					}
					uninstallErrors = append([]string{cleanL}, uninstallErrors...)
					if len(uninstallErrors) >= 4 {
						break
					}
				}
			}
		}
	}

	statusVal := core.GREEN + "SUCCESSFUL" + core.NC
	if len(uninstallErrors) > 0 {
		statusVal = core.BOLD + core.RED + "COMPLETED WITH WARNINGS/ERRORS" + core.NC
	}

	finalSummary := []string{
		tui.FormatKV("Uninstallation Status", statusVal, core.GREEN),
		tui.FormatKV("Deployment Environment", envType, core.CYAN),
	}
	if uninstallBackup && bkpFile != "" {
		finalSummary = append(finalSummary, tui.FormatKV("Config/Data Backup", bkpFile, core.GREEN))
	} else {
		finalSummary = append(finalSummary, tui.FormatKV("Config/Data Backup", "[NO BACKUP CREATED]", core.YELLOW))
	}
	if uninstallPurgeFiles {
		finalSummary = append(finalSummary, tui.FormatKV("Files & Storage", "[PURGED / REMOVED]", core.RED))
	} else {
		finalSummary = append(finalSummary, tui.FormatKV("Files & Storage", "[RETAINED]", core.GREEN))
	}

	if len(uninstallErrors) > 0 {
		finalSummary = append(finalSummary, "")
		finalSummary = append(finalSummary, core.BOLD+core.RED+"LOGGED ERRORS / WARNINGS DURING UNINSTALLATION:"+core.NC)
		for _, errLine := range uninstallErrors {
			finalSummary = append(finalSummary, fmt.Sprintf("  %s%s%s %s%s%s", core.RED, core.Sym.Pointer, core.NC, core.RED, errLine, core.NC))
		}
	} else {
		finalSummary = append(finalSummary, tui.FormatKV("Execution Errors", "[NONE / CLEAN]", core.GREEN))
	}

	if core.LogFilePath != "" {
		finalSummary = append(finalSummary, "")
		finalSummary = append(finalSummary, core.BOLD+core.CYAN+"LOG FILE & QUICK COMMANDS:"+core.NC)
		finalSummary = append(finalSummary, fmt.Sprintf("  %s%s Path :%s %s%s%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, core.LogFilePath, core.NC))
		finalSummary = append(finalSummary, fmt.Sprintf("  %s%s View :%s %scat %s%s", core.GREEN, core.Sym.Pointer, core.NC, core.BOLD+core.GREEN, core.LogFilePath, core.NC))
		finalSummary = append(finalSummary, fmt.Sprintf("  %s%s Tail :%s %stail -n 25 %s%s", core.BLUE, core.Sym.Pointer, core.NC, core.BOLD+core.BLUE, core.LogFilePath, core.NC))
	}

	tui.ClearScreen()
	tui.PrintInBox("FIREWALL-MANAGER Stack Uninstalled", finalSummary)
	tui.LeaveAltScreen()

	fmt.Print("\r\n")
	logger.LogSuccess("FIREWALL-MANAGER stack uninstalled successfully.")
	os.Exit(0)
}
