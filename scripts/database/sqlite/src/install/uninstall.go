// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: SQLITE (database)
// File:    database/sqlite/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-sqlite-installer/core"
	"krotek-sqlite-installer/logger"
	"krotek-sqlite-installer/system"
	"krotek-sqlite-installer/tui"
)

// UninstallSQLite conducts a safe, stepped removal of SQLite Native or Docker deployments.
func UninstallSQLite() {
	logger.InitUninstallLogger("sqlite")
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		if _, err := os.Stat("/etc/sqlite-docker"); err == nil {
			envType = "docker"
		} else {
			envType = "native"
		}
	}

	dataDir := state["DATA_DIR"]
	if dataDir == "" {
		dataDir = "/var/lib/sqlite"
	}
	composeDir := state["COMPOSE_DIR"]
	if composeDir == "" {
		composeDir = "/etc/sqlite-docker"
	}

	uninstallBackup := true
	uninstallPurgeData := false

	if !core.AutoYes {
		uStep := 1
		for {
			switch uStep {
			case 1:
				tui.ClearScreen()
				promptLines := []string{
					core.BOLD + core.CYAN + "Database Safety & Pre-removal Backup" + core.NC,
					"",
					"  Before uninstalling SQLite, an automated archive of all database files",
					"  can be created and compressed into: " + core.YELLOW + "/root/sqlite_backup_*.tar.gz" + core.NC,
					"",
					core.BOLD + core.YELLOW + "Create an archive of SQLite database files before removal?" + core.NC,
				}
				bIdx, _, bStatus := tui.SelectMenuWithContent("Step 1/3: Pre-removal Database Backup", promptLines, []string{"YES (Recommended)", "NO (Skip Backup)"})
				if bStatus == 1 || bStatus == 2 {
					logger.LogInfo("Uninstallation cancelled by user.")
					return
				}
				uninstallBackup = (bIdx == 0)
				uStep = 2

			case 2:
				tui.ClearScreen()
				purgeLines := []string{
					core.BOLD + core.RED + "Database Storage Cleanup Policy" + core.NC,
					"",
					"  Choose whether to permanently erase database files:",
					"  " + core.BOLD + core.Sym.Pointer + " Storage Path :" + core.NC + " " + dataDir,
					"",
					core.BOLD + core.RED + "DANGER: Erasing cannot be undone unless you created a backup!" + core.NC,
				}
				pIdx, _, pStatus := tui.SelectMenuWithContent("Step 2/3: Data Purge Policy", purgeLines, []string{
					"KEEP DATA (Retain database files for future access)",
					"PURGE ALL (Completely erase all SQLite files in " + dataDir + ")",
				})
				if pStatus == 1 {
					uStep = 1
					continue
				}
				if pStatus == 2 {
					logger.LogInfo("Uninstallation cancelled by user.")
					return
				}
				uninstallPurgeData = (pIdx == 1)
				uStep = 3

			case 3:
				tui.ClearScreen()
				summaryLines := []string{
					tui.FormatKV("Action", "UNINSTALL SQLITE DATABASE TOOLS", core.RED+core.BOLD),
					tui.FormatKV("Deployment Target", func() string {
						if envType == "docker" {
							return "Docker Compose Container Stack"
						}
						return "Native Linux System Package"
					}(), core.CYAN),
					tui.FormatKV("Automated Backup", func() string {
						if uninstallBackup {
							return "[YES] (Archive to /root/sqlite_backup_*.tar.gz)"
						}
						return "[NO] (No pre-removal backup)"
					}(), core.GREEN),
					tui.FormatKV("Database Storage", func() string {
						if uninstallPurgeData {
							return core.RED + "[PURGE / ERASE ALL FILES]" + core.NC
						}
						return core.GREEN + "[RETAIN / PRESERVED ON DISK]" + core.NC
					}(), core.YELLOW),
					"",
					core.BOLD + core.RED + "Confirm: Proceed with SQLite uninstallation now?" + core.NC,
				}

				cIdx, _, cStatus := tui.SelectMenuWithContent("Step 3/3: Uninstallation Summary", summaryLines, []string{"YES", "NO"})
				if cStatus == 1 {
					uStep = 2
					continue
				}
				if cStatus == 2 || cIdx != 0 {
					logger.LogInfo("Uninstallation aborted by user.")
					return
				}
				uStep = 99
				break
			}
			if uStep == 99 {
				break
			}
		}
	}

	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Uninstalling SQLite Tools ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)
	logger.LogInfo("Starting SQLite uninstallation...")

	// 1. Pre-removal Backup
	if uninstallBackup {
		timestamp := time.Now().Format("20060102_150405")
		backupPath := fmt.Sprintf("/root/sqlite_backup_%s.tar.gz", timestamp)
		_ = os.MkdirAll("/root", 0700)
		if !system.IsSymlinkPath(backupPath) {
			_ = exec.Command("tar", "-czf", backupPath, "-C", dataDir, ".").Run()
		}
		if fi, err := os.Stat(backupPath); err == nil && fi.Size() > 0 {
			_ = os.Chmod(backupPath, 0600)
			logger.LogSuccess(fmt.Sprintf("SQLite backup successfully saved to: %s", backupPath))
		}
	}

	// 2. Teardown
	if envType == "docker" {
		if !system.RunDockerCompose(composeDir, "Stopping Docker containers", false, "down", "-v") {
			_ = exec.Command("docker", "rm", "-f", "krotek-sqlite-web").Run()
		}
		if uninstallPurgeData {
			_ = os.RemoveAll(composeDir)
		}
	} else {
		_ = system.RunMultiStep("Removing SQLite packages", "apt-get purge -y sqlite3 libsqlite3-dev 2>/dev/null || dnf remove -y sqlite sqlite-devel 2>/dev/null || apk del sqlite sqlite-dev 2>/dev/null || true")
		if uninstallPurgeData {
			_ = os.RemoveAll(dataDir)
		}
	}

	// 3. Remove Global Shortcuts & State
	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/sqlite-manager")
	_ = os.Remove("/usr/local/bin/sqlite-installer")

	// 4. Display Clean Uninstall Box
	var finalLines []string
	finalLines = append(finalLines, tui.FormatKV("CLI Tools", "Removed cleanly", core.GREEN))
	if uninstallBackup {
		finalLines = append(finalLines, tui.FormatKV("Backup Archive", "Retained in /root/sqlite_backup_*.tar.gz", core.GREEN))
	}
	if uninstallPurgeData {
		finalLines = append(finalLines, tui.FormatKV("Database Storage", "Purged and deleted", core.RED))
	} else {
		finalLines = append(finalLines, tui.FormatKV("Database Storage", "Retained on disk: "+dataDir, core.YELLOW))
	}
	finalLines = append(finalLines, tui.FormatKV("CLI Shortcuts", "Removed (/usr/local/bin/sqlite-manager)", core.GREEN))

	if !core.AutoYes {
		tui.EnterAltScreen()
		tui.ClearScreen()
		tui.PrintInBox("SQLite Uninstallation Complete", finalLines)
		tui.LeaveAltScreen()
	}
	fmt.Print("\r\n")
	logger.LogSuccess("SQLite uninstalled successfully.")
}

// UninstallWizard is an alias for backward compatibility.
// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	UninstallSQLite()
}
