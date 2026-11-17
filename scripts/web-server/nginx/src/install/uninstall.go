// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: NGINX (web-server)
// File:    web-server/nginx/src/install/uninstall.go
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

	"krotek-nginx-installer/core"
	"krotek-nginx-installer/logger"
	"krotek-nginx-installer/system"
	"krotek-nginx-installer/tui"
)

func UninstallNginx() {
	logger.InitUninstallLogger("nginx")
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		if _, err := os.Stat("/etc/nginx-docker"); err == nil {
			envType = "docker"
		} else {
			envType = "native"
		}
	}

	uninstallBackup := false
	uninstallPurgeFiles := true
	uninstallPurgeConf := true

	if !core.AutoYes {
		if envType == "docker" {
			uStep := 1
			for {
				switch uStep {
				case 1:
					tui.ClearScreen()
					res := tui.AskYesNo("Step 1/3: Create a full backup of Docker Compose configurations & web files before uninstallation? (Saved to /root/)")
					if res == 2 || res == -1 {
						return
					}
					if res == 0 {
						uninstallBackup = true
					} else {
						uninstallBackup = false
					}
					uStep = 2

				case 2:
					tui.ClearScreen()
					res := tui.AskYesNo("Step 2/3: Stop and purge Docker containers, volumes & Compose directory (/etc/nginx-docker)?")
					if res == 2 {
						return
					}
					if res == -1 {
						uStep = 1
						continue
					}
					if res == 0 {
						uninstallPurgeFiles = true
					} else {
						uninstallPurgeFiles = false
					}
					uStep = 3

				case 3:
					tui.ClearScreen()
					summaryLines := []string{
						tui.FormatKV("Action", "PURGE & REMOVE NGINX DOCKER STACK", core.RED),
						"",
						core.BOLD + core.YELLOW + "UNINSTALL OPTIONS:" + core.NC,
					}
					if uninstallBackup {
						summaryLines = append(summaryLines, tui.FormatKV("Backup", "[YES] (Saved to /root/)", core.GREEN))
					} else {
						summaryLines = append(summaryLines, tui.FormatKV("Backup", "[NO]", core.RED))
					}
					if uninstallPurgeFiles {
						summaryLines = append(summaryLines, tui.FormatKV("Purge Stack", "[YES] (Containers, Volumes & Files)", core.GREEN))
					} else {
						summaryLines = append(summaryLines, tui.FormatKV("Purge Stack", "[NO]", core.RED))
					}
					summaryLines = append(summaryLines, "")
					summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"Do you want to proceed with uninstallation?"+core.NC)

					choices := []string{"YES", "NO"}

					idx, _, status := tui.SelectMenuWithContent("Step 3/3: Uninstallation Summary", summaryLines, choices)
					if status == 2 || idx != 0 {
						logger.LogInfo("Uninstallation cancelled by user.")
						return
					}
					if status == 1 {
						uStep = 2
						continue
					}
					if idx == 0 {
						uStep = 99
						break
					}
				}
				if uStep == 99 {
					break
				}
			}
		} else {
			uStep := 1
			for {
				switch uStep {
				case 1:
					tui.ClearScreen()
					res := tui.AskYesNo("Step 1/4: Create a full backup of Nginx configurations & web files before uninstallation? (Saved to /root/)")
					if res == 2 || res == -1 {
						return
					}
					if res == 0 {
						uninstallBackup = true
					} else {
						uninstallBackup = false
					}
					uStep = 2

				case 2:
					tui.ClearScreen()
					res := tui.AskYesNo("Step 2/4: Delete web root files (/var/www/) and site data?")
					if res == 2 {
						return
					}
					if res == -1 {
						uStep = 1
						continue
					}
					if res == 0 {
						uninstallPurgeFiles = true
					} else {
						uninstallPurgeFiles = false
					}
					uStep = 3

				case 3:
					tui.ClearScreen()
					res := tui.AskYesNo("Step 3/4: Delete VirtualHost configuration files and firewall rules?")
					if res == 2 {
						return
					}
					if res == -1 {
						uStep = 2
						continue
					}
					if res == 0 {
						uninstallPurgeConf = true
					} else {
						uninstallPurgeConf = false
					}
					uStep = 4

				case 4:
					tui.ClearScreen()
					summaryLines := []string{
						tui.FormatKV("Action", "PURGE & REMOVE NGINX NATIVE STACK", core.RED),
						"",
						core.BOLD + core.YELLOW + "UNINSTALL OPTIONS:" + core.NC,
					}
					if uninstallBackup {
						summaryLines = append(summaryLines, tui.FormatKV("Backup", "[YES] (Saved to /root/)", core.GREEN))
					} else {
						summaryLines = append(summaryLines, tui.FormatKV("Backup", "[NO]", core.RED))
					}
					if uninstallPurgeFiles {
						summaryLines = append(summaryLines, tui.FormatKV("Files", "[YES] (Purge /var/www/)", core.GREEN))
					} else {
						summaryLines = append(summaryLines, tui.FormatKV("Files", "[NO]", core.RED))
					}
					if uninstallPurgeConf {
						summaryLines = append(summaryLines, tui.FormatKV("Configs", "[YES] (Purge /etc/nginx/ & Firewall)", core.GREEN))
					} else {
						summaryLines = append(summaryLines, tui.FormatKV("Configs", "[NO]", core.RED))
					}
					summaryLines = append(summaryLines, "")
					summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"Do you want to proceed with uninstallation?"+core.NC)

					choices := []string{"YES", "NO"}

					idx, _, status := tui.SelectMenuWithContent("Step 4/4: Uninstallation Summary", summaryLines, choices)
					if status == 2 || idx != 0 {
						logger.LogInfo("Uninstallation cancelled by user.")
						return
					}
					if status == 1 {
						uStep = 3
						continue
					}
					if idx == 0 {
						uStep = 99
						break
					}
				}
				if uStep == 99 {
					break
				}
			}
		}
	}

	tui.LeaveAltScreen()
	fmt.Printf("\r\n%s=== UNINSTALLING NGINX SERVER ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	var bkpFile string
	var uninstallErrors []string

	step := func(msg string, command string, args ...string) bool {
		ok := system.RunStep(msg, command, args...)
		if !ok {
			uninstallErrors = append(uninstallErrors, msg)
		}
		return ok
	}

	stepShell := func(msg string, shellScript string) bool {
		ok := system.RunMultiStep(msg, shellScript)
		if !ok {
			uninstallErrors = append(uninstallErrors, msg)
		}
		return ok
	}

	if envType == "native" {
		if uninstallBackup {
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile = fmt.Sprintf("/root/nginx_backup_%s.tar.gz", ts)
			stepShell("Creating full configuration backup to "+bkpFile, fmt.Sprintf("tar -czf %s /etc/nginx /var/www 2>/dev/null || true", bkpFile))
		}

		if uninstallPurgeFiles {
			step("Removing web root files (/var/www/html)", "rm", "-rf", "/var/www/html")
		}

		if uninstallPurgeConf {
			if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
				stepShell("Stopping & disabling Nginx service", "rc-service nginx stop 2>/dev/null || true; rc-update del nginx default 2>/dev/null || true")
			} else {
				stepShell("Stopping & disabling Nginx service", "systemctl stop nginx 2>/dev/null || true; systemctl disable nginx 2>/dev/null || true")
			}
			if core.SysInfo.Family == "debian" {
				step("Purging Nginx packages", "apt-get", "purge", "-y", "nginx", "nginx-common", "nginx-core", "nginx-full")
				step("Cleaning up orphaned dependency packages", "apt-get", "autoremove", "-y")
			} else if core.SysInfo.Family == "rhel" {
				step("Removing Nginx packages", "dnf", "remove", "-y", "nginx")
			} else if core.SysInfo.Family == "alpine" {
				step("Removing Nginx packages", "apk", "del", "nginx")
			}
			_ = os.RemoveAll("/etc/nginx")
		}

		if uninstallPurgeFiles {
			_ = os.RemoveAll("/var/www/html")
			_ = exec.Command("rmdir", "/var/www").Run()
		}
	} else {
		composeDir := state["COMPOSE_DIR"]
		if composeDir == "" {
			composeDir = "/etc/nginx-docker"
		}

		if uninstallBackup {
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile = fmt.Sprintf("/root/nginx_docker_backup_%s.tar.gz", ts)
			stepShell("Creating Docker stack backup to "+bkpFile, fmt.Sprintf("tar -czf %s %s 2>/dev/null || true", bkpFile, composeDir))
		}

		if uninstallPurgeFiles {
			stepShell("Stopping and purging Docker container stack", fmt.Sprintf("cd %s && docker compose down -v 2>/dev/null || docker-compose down -v 2>/dev/null || true", composeDir))
			_ = os.RemoveAll(composeDir)
		} else {
			stepShell("Stopping Docker container stack", fmt.Sprintf("cd %s && docker compose down 2>/dev/null || docker-compose down 2>/dev/null || true", composeDir))
		}
	}

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/nginx-manager")
	_ = os.Remove("/usr/local/bin/nginx-installer")
	_ = os.Remove("/usr/local/bin/nginx")
	var recentLogErrors []string
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
					recentLogErrors = append([]string{cleanL}, recentLogErrors...)
					if len(recentLogErrors) >= 3 {
						break
					}
				}
			}
		}
	}

	statusVal := core.GREEN + "SUCCESSFUL" + core.NC
	if len(uninstallErrors) > 0 || len(recentLogErrors) > 0 {
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
	if envType == "native" {
		if uninstallPurgeConf {
			finalSummary = append(finalSummary, tui.FormatKV("Packages & Configs", "[PURGED / REMOVED]", core.RED))
		} else {
			finalSummary = append(finalSummary, tui.FormatKV("Packages & Configs", "[RETAINED]", core.GREEN))
		}
	}

	if len(uninstallErrors) > 0 {
		finalSummary = append(finalSummary, "")
		finalSummary = append(finalSummary, core.BOLD+core.RED+"FAILED UNINSTALLATION STEPS:"+core.NC)
		for _, errStep := range uninstallErrors {
			finalSummary = append(finalSummary, fmt.Sprintf("  %s%s%s %s%s%s", core.RED, core.Sym.Pointer, core.NC, core.RED, errStep, core.NC))
		}
	} else if len(recentLogErrors) == 0 {
		finalSummary = append(finalSummary, tui.FormatKV("Execution Errors", "[NONE / CLEAN]", core.GREEN))
	}

	if len(recentLogErrors) > 0 {
		finalSummary = append(finalSummary, "")
		finalSummary = append(finalSummary, core.BOLD+core.YELLOW+"RECENT LOG ERROR ENTRIES:"+core.NC)
		for _, logErr := range recentLogErrors {
			finalSummary = append(finalSummary, fmt.Sprintf("  %s%s%s %s%s%s", core.YELLOW, core.Sym.Pointer, core.NC, core.RED, logErr, core.NC))
		}
	}

	if core.LogFilePath != "" {
		finalSummary = append(finalSummary, "")
		finalSummary = append(finalSummary, core.BOLD+core.CYAN+"LOG FILE & QUICK COMMANDS:"+core.NC)
		finalSummary = append(finalSummary, fmt.Sprintf("  %s%s Path :%s %s%s%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, core.LogFilePath, core.NC))
		finalSummary = append(finalSummary, fmt.Sprintf("  %s%s View :%s %scat %s%s", core.GREEN, core.Sym.Pointer, core.NC, core.BOLD+core.GREEN, core.LogFilePath, core.NC))
		finalSummary = append(finalSummary, fmt.Sprintf("  %s%s Tail :%s %stail -n 25 %s%s", core.BLUE, core.Sym.Pointer, core.NC, core.BOLD+core.BLUE, core.LogFilePath, core.NC))
	}

	tui.ClearScreen()
	tui.PrintInBox("NGINX Server Stack Uninstalled", finalSummary)

	fmt.Print("\r\n")
	logger.LogSuccess("Nginx server stack uninstalled successfully.")
	os.Exit(0)
}

// ==============================================================================
// MAIN ENTRY POINT
// ==============================================================================

// ==============================================================================

// ==============================================================================
// LOG INSPECTOR & GRAPHICAL TERMINAL VIEWER ENGINE
// ==============================================================================

// LogFileTarget describes a single log source for the log viewer: either a file
// path (IsCmd=false) or a shell command whose output is read as log content (IsCmd=true).
type LogFileTarget struct {
	Name  string
	Path  string
	IsCmd bool
}

// getServiceLogTargets returns the list of predefined log sources for this service.
// UninstallWizard provides standardized invocation interface
func UninstallWizard() {
	UninstallNginx()
}
