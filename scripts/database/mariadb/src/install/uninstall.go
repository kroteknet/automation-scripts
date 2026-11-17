// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: MARIADB (database)
// File:    database/mariadb/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-mariadb-installer/core"
	"krotek-mariadb-installer/logger"
	"krotek-mariadb-installer/system"
	"krotek-mariadb-installer/tui"
)

// UninstallMariaDB conducts a safe, stepped removal of MariaDB Native or Docker deployments.
func UninstallMariaDB() {
	logger.InitUninstallLogger("mariadb")
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		if _, err := os.Stat("/etc/mariadb-docker"); err == nil {
			envType = "docker"
		} else {
			envType = "native"
		}
	}

	uninstallBackup := true
	uninstallPurgeData := false
	composeDir := state["COMPOSE_DIR"]
	if composeDir == "" {
		composeDir = "/etc/mariadb-docker"
	}
	port := state["PORT"]
	if port == "" {
		port = "3306"
	}

	if !core.AutoYes {
		uStep := 1
		for {
			switch uStep {
			case 1:
				tui.ClearScreen()
				promptLines := []string{
					core.BOLD + core.CYAN + "Database Safety & Pre-removal Backup" + core.NC,
					"",
					"  Before uninstalling MariaDB, an automated full SQL logical dump",
					"  can be created and compressed into: " + core.YELLOW + "/root/mariadb_backup_*.sql.gz" + core.NC,
					"",
					core.BOLD + core.YELLOW + "Create a full SQL database backup archive before removal?" + core.NC,
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
					core.BOLD + core.RED + "Database Storage & Table Cleanup Policy" + core.NC,
					"",
					"  Choose whether to permanently purge database files and tables:",
					"  " + core.BOLD + core.Sym.Pointer + " Native Mode :" + core.NC + " /var/lib/mysql and configuration files",
					"  " + core.BOLD + core.Sym.Pointer + " Docker Mode :" + core.NC + " Named volume (krotek_mariadb_data) & " + composeDir,
					"",
					core.BOLD + core.RED + "DANGER: Purging cannot be undone unless you created a backup!" + core.NC,
				}
				pIdx, _, pStatus := tui.SelectMenuWithContent("Step 2/3: Data Purge Policy", purgeLines, []string{
					"KEEP DATA (Retain database files for future re-installation)",
					"PURGE ALL (Completely erase all database tables & storage)",
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
					tui.FormatKV("Action", "UNINSTALL MARIADB DATABASE SERVER", core.RED+core.BOLD),
					tui.FormatKV("Deployment Target", func() string {
						if envType == "docker" {
							return "Docker Compose Container Stack"
						}
						return "Native Linux System Service"
					}(), core.CYAN),
					tui.FormatKV("Automated SQL Backup", func() string {
						if uninstallBackup {
							return "[YES] (Archive to /root/mariadb_backup_*.sql.gz)"
						}
						return "[NO] (No pre-removal backup)"
					}(), core.GREEN),
					tui.FormatKV("Database Data & Storage", func() string {
						if uninstallPurgeData {
							return core.RED + "[PURGE / ERASE ALL DATA]" + core.NC
						}
						return core.GREEN + "[RETAIN / PRESERVED ON DISK]" + core.NC
					}(), core.YELLOW),
					"",
					core.BOLD + core.RED + "Confirm: Proceed with MariaDB uninstallation now?" + core.NC,
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
	fmt.Printf("\r\n%s=== Uninstalling MariaDB Database Server ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)
	logger.LogInfo("Starting MariaDB uninstallation...")

	// 1. Perform Pre-Removal SQL Backup
	if uninstallBackup {
		timestamp := time.Now().Format("20060102_150405")
		backupPath := fmt.Sprintf("/root/mariadb_backup_%s.sql.gz", timestamp)
		_ = os.MkdirAll("/root", 0700)

		adminPass := state["ADMIN_PASS"]
		if envType == "docker" {
			dumpArgs := []string{"exec"}
			if adminPass != "" {
				dumpArgs = append(dumpArgs, "-e", "MYSQL_PWD")
			}
			dumpArgs = append(dumpArgs, "krotek-mariadb", "mariadb-dump", "--all-databases", "-u", "root")
			dumpProc := exec.Command("docker", dumpArgs...)
			if adminPass != "" {
				dumpProc.Env = append(os.Environ(), "MYSQL_PWD="+adminPass)
			}
			_ = system.StreamCommandGzipNoFollow(backupPath, dumpProc)
		} else {
			exported := false
			if adminPass != "" {
				if tmpDir, cnfFile, cErr := system.BuildMySQLClientCnf("root", adminPass); cErr == nil {
					for _, bin := range []string{"mariadb-dump", "mysqldump"} {
						if system.CommandExists(bin) {
							if system.StreamCommandGzipNoFollow(backupPath, exec.Command(bin, "--defaults-extra-file="+cnfFile, "--all-databases")) == nil {
								exported = true
								break
							}
						}
					}
					_ = os.RemoveAll(tmpDir)
				}
			}
			if !exported {
				for _, bin := range []string{"mariadb-dump", "mysqldump"} {
					if system.CommandExists(bin) {
						if system.StreamCommandGzipNoFollow(backupPath, exec.Command(bin, "--all-databases", "-u", "root")) == nil {
							break
						}
					}
				}
			}
		}

		if fi, err := os.Stat(backupPath); err == nil && fi.Size() > 20 {
			logger.LogSuccess(fmt.Sprintf("Database backup successfully saved to: %s", backupPath))
		}
	}

	// 2. Teardown Service / Containers
	if envType == "docker" {
		if uninstallPurgeData {
			if !system.RunDockerCompose(composeDir, "Stopping & purging Docker containers and volumes", false, "down", "-v") {
				_ = exec.Command("docker", "rm", "-f", "krotek-mariadb").Run()
			}
			_ = os.RemoveAll(composeDir)
		} else {
			if !system.RunDockerCompose(composeDir, "Stopping Docker containers (retaining volumes)", false, "down") {
				_ = exec.Command("docker", "stop", "krotek-mariadb").Run()
			}
		}
	} else {
		if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
			_ = system.RunMultiStep("Stopping & disabling MariaDB in OpenRC", "rc-service mariadb stop 2>/dev/null || rc-service mysql stop 2>/dev/null || true; rc-update del mariadb default 2>/dev/null || rc-update del mysql default 2>/dev/null || true")
		} else {
			_ = system.RunMultiStep("Stopping & disabling MariaDB service", "systemctl stop mariadb 2>/dev/null || systemctl stop mysqld 2>/dev/null || true; systemctl disable mariadb 2>/dev/null || systemctl disable mysqld 2>/dev/null || true")
		}

		if uninstallPurgeData {
			if core.SysInfo.Family == "alpine" {
				_ = system.RunMultiStep("Purging MariaDB packages", "apk del mariadb mariadb-client mariadb-common mysql mysql-client 2>/dev/null || true")
			} else if core.SysInfo.Family == "rhel" {
				_ = system.RunMultiStep("Purging MariaDB packages", "dnf remove -y mariadb-server mariadb 2>/dev/null || yum remove -y mariadb-server mariadb 2>/dev/null || true")
			} else {
				_ = system.RunMultiStep("Purging MariaDB packages", "apt-get purge -y mariadb-server mariadb-client 2>/dev/null || true")
			}
			_ = os.RemoveAll("/var/lib/mysql")
			_ = os.RemoveAll("/etc/mysql")
			_ = os.RemoveAll("/etc/my.cnf.d")
			_ = os.Remove("/etc/my.cnf")
		}
	}

	// 3. Remove Firewall Rules
	system.RemoveFirewallRules(port)

	// 4. Remove Global Shortcuts & State
	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/mariadb-manager")
	_ = os.Remove("/usr/local/bin/mariadb-installer")

	// 5. Display Clean Uninstall Box
	var finalLines []string
	finalLines = append(finalLines, tui.FormatKV("Service State", "Completely Stopped & Unregistered", core.GREEN))
	if uninstallBackup {
		finalLines = append(finalLines, tui.FormatKV("Backup Archive", "Retained in /root/mariadb_backup_*.sql.gz", core.GREEN))
	} else {
		finalLines = append(finalLines, tui.FormatKV("Backup Archive", "[NO BACKUP CREATED]", core.YELLOW))
	}
	if uninstallPurgeData {
		finalLines = append(finalLines, tui.FormatKV("Database Data", "Purged and deleted cleanly", core.RED))
	} else {
		finalLines = append(finalLines, tui.FormatKV("Database Data", "Retained on disk for safety", core.YELLOW))
	}
	finalLines = append(finalLines, tui.FormatKV("CLI Shortcuts", "Removed (/usr/local/bin/mariadb-manager)", core.GREEN))

	fmt.Println()
	tui.PrintInBox("MariaDB Uninstallation Complete", finalLines)
	tui.ShowCursor()
	tui.DisableRawMode()
	fmt.Print("\r\n")
	logger.LogSuccess("MariaDB uninstalled successfully.")
	os.Exit(0)
}

// UninstallWizard is an alias for backward compatibility.
// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	UninstallMariaDB()
}
