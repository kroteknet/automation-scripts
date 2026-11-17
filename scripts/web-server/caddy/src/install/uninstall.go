// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: CADDY (web-server)
// File:    web-server/caddy/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"strings"
	"time"

	"krotek-caddy-installer/core"
	"krotek-caddy-installer/logger"
	"krotek-caddy-installer/system"
	"krotek-caddy-installer/tui"
)

func UninstallCaddy() {
	logger.InitUninstallLogger("caddy")
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		if _, err := os.Stat("/etc/caddy-docker"); err == nil {
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
				fRes := tui.AskYesNo("Purge CADDY configuration files and data directories?")
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
					core.BOLD + core.RED + "CONFIRM CADDY UNINSTALLATION:" + core.NC,
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
	fmt.Printf("\r\n%s=== UNINSTALLING CADDY STACK ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	var bkpFile string

	if envType == "native" {
		if uninstallBackup {
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile = fmt.Sprintf("/root/caddy_backup_%s.tar.gz", ts)
			system.RunMultiStep("Creating CADDY backup to "+bkpFile, fmt.Sprintf("tar -czf %s /etc/caddy /var/www/caddy 2>/dev/null || true", bkpFile))
		}

		if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
			system.RunMultiStep("Stopping & disabling CADDY service", "rc-service caddy stop 2>/dev/null || true; rc-update del caddy default 2>/dev/null || true")
		} else {
			system.RunMultiStep("Stopping & disabling CADDY service", "systemctl stop caddy 2>/dev/null || true; systemctl disable caddy 2>/dev/null || true")
		}

		if uninstallPurgeFiles {
			system.RunStep("Removing CADDY configuration & data files", "rm", "-rf", "/etc/caddy", "/var/www/caddy")
		}
	} else {
		composeDir := state["COMPOSE_DIR"]
		if composeDir == "" {
			composeDir = "/etc/caddy-docker"
		}

		if uninstallBackup {
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile = fmt.Sprintf("/root/caddy_docker_backup_%s.tar.gz", ts)
			system.RunMultiStep("Creating Docker stack backup to "+bkpFile, fmt.Sprintf("tar -czf %s %s 2>/dev/null || true", bkpFile, composeDir))
		}

		if uninstallPurgeFiles {
			system.RunMultiStep("Stopping and purging CADDY Docker stack", fmt.Sprintf("cd %s && docker compose down -v 2>/dev/null || docker-compose down -v 2>/dev/null || true", composeDir))
			_ = os.RemoveAll(composeDir)
		} else {
			system.RunMultiStep("Stopping CADDY Docker stack", fmt.Sprintf("cd %s && docker compose down 2>/dev/null || docker-compose down 2>/dev/null || true", composeDir))
		}
	}

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/caddy-manager")
	_ = os.Remove("/usr/local/bin/caddy-installer")
	_ = os.Remove("/usr/local/bin/caddy")
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
	tui.PrintInBox("CADDY Stack Uninstalled", finalSummary)
	tui.LeaveAltScreen()

	fmt.Print("\r\n")
	logger.LogSuccess("CADDY stack uninstalled successfully.")
	os.Exit(0)
}

// freshInstallWizard runs the interactive installation setup wizard.
// It guides the user through configuration preset selection, credential setup,
// and a final confirmation summary before dispatching to the appropriate installer.
// UninstallWizard provides standardized invocation interface
func UninstallWizard() {
	UninstallCaddy()
}
