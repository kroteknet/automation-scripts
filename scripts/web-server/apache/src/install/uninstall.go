// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: APACHE (web-server)
// File:    web-server/apache/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"strings"
	"time"

	"krotek-apache-installer/core"
	"krotek-apache-installer/logger"
	"krotek-apache-installer/system"
	"krotek-apache-installer/tui"
)

func UninstallApache() {
	logger.InitUninstallLogger("apache")
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		if _, err := os.Stat("/etc/apache-docker"); err == nil {
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
				bRes := tui.AskYesNo("Step 1/3: Create a full backup of APACHE configuration and web files before uninstallation? (Saved to /root/)")
				if bRes == 2 || bRes == -1 {
					logger.LogInfo("Uninstallation cancelled by user.")
					return
				}
				uninstallBackup = (bRes == 0)
				uStep = 2

			case 2:
				tui.ClearScreen()
				fRes := tui.AskYesNo("Step 2/3: Purge APACHE configuration files, web roots, and data directories?")
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
					core.BOLD + core.RED + "CONFIRM APACHE UNINSTALLATION:" + core.NC,
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
	fmt.Printf("\r\n%s=== UNINSTALLING APACHE STACK ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	var bkpFile string

	if envType == "native" {
		srvName := "apache2"
		cfgDir := "/etc/apache2"
		if core.SysInfo.Family != "debian" {
			srvName = "httpd"
			cfgDir = "/etc/httpd"
		}

		if uninstallBackup {
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile = fmt.Sprintf("/root/apache_backup_%s.tar.gz", ts)
			system.RunMultiStep("Creating APACHE backup to "+bkpFile, fmt.Sprintf("tar -czf %s %s /var/www 2>/dev/null || true", bkpFile, cfgDir))
		}

		if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
			system.RunMultiStep("Stopping & disabling APACHE service", fmt.Sprintf("rc-service %s stop 2>/dev/null || true; rc-update del %s default 2>/dev/null || true", srvName, srvName))
		} else {
			system.RunMultiStep("Stopping & disabling APACHE service", fmt.Sprintf("systemctl stop %s 2>/dev/null || true; systemctl disable %s 2>/dev/null || true", srvName, srvName))
		}

		if uninstallPurgeFiles {
			if core.SysInfo.Family == "debian" {
				system.RunStep("Purging APACHE packages", "apt-get", "purge", "-y", "apache2", "apache2-utils", "apache2-bin")
			} else if core.SysInfo.Family == "rhel" {
				system.RunStep("Removing APACHE packages", "dnf", "remove", "-y", "httpd", "httpd-core", "httpd-tools")
			} else if core.SysInfo.Family == "alpine" {
				system.RunStep("Removing APACHE packages", "apk", "del", "apache2")
			}
			system.RunStep("Removing APACHE configuration & data files", "rm", "-rf", cfgDir, "/var/www/html")
		}
	} else {
		composeDir := state["COMPOSE_DIR"]
		if composeDir == "" {
			composeDir = "/etc/apache-docker"
		}

		if uninstallBackup {
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile = fmt.Sprintf("/root/apache_docker_backup_%s.tar.gz", ts)
			system.RunMultiStep("Creating Docker stack backup to "+bkpFile, fmt.Sprintf("tar -czf %s %s 2>/dev/null || true", bkpFile, composeDir))
		}

		if uninstallPurgeFiles {
			system.RunMultiStep("Stopping and purging APACHE Docker stack", fmt.Sprintf("cd %s && docker compose down -v 2>/dev/null || docker-compose down -v 2>/dev/null || true", composeDir))
			_ = os.RemoveAll(composeDir)
		} else {
			system.RunMultiStep("Stopping APACHE Docker stack", fmt.Sprintf("cd %s && docker compose down 2>/dev/null || docker-compose down 2>/dev/null || true", composeDir))
		}
	}

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/apache-manager")
	_ = os.Remove("/usr/local/bin/apache-installer")
	_ = os.Remove("/usr/local/bin/apache")

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
	tui.PrintInBox("APACHE Stack Uninstalled", finalSummary)
	tui.LeaveAltScreen()

	fmt.Print("\r\n")
	logger.LogSuccess("APACHE stack uninstalled successfully.")
	os.Exit(0)
}

// UninstallWizard provides standardized invocation interface
func UninstallWizard() {
	UninstallApache()
}
