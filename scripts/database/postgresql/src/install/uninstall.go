// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: POSTGRESQL (database)
// File:    database/postgresql/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-postgresql-installer/core"
	"krotek-postgresql-installer/logger"
	"krotek-postgresql-installer/system"
	"krotek-postgresql-installer/tui"
)

// UninstallPostgreSQL conducts a safe, stepped removal of PostgreSQL Native or Docker deployments.
func UninstallPostgreSQL() {
	logger.InitUninstallLogger("postgresql")
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		if _, err := os.Stat("/etc/postgresql-docker"); err == nil {
			envType = "docker"
		} else {
			envType = "native"
		}
	}

	uninstallBackup := true
	uninstallPurgeData := false
	composeDir := state["COMPOSE_DIR"]
	if composeDir == "" {
		composeDir = "/etc/postgresql-docker"
	}
	port := state["PORT"]
	if port == "" {
		port = "5432"
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
					"  Before uninstalling PostgreSQL, an automated full logical cluster dump",
					"  (pg_dumpall) can be created and compressed into: " + core.YELLOW + "/root/postgres_backup_*.sql.gz" + core.NC,
					"",
					core.BOLD + core.YELLOW + "Create a full PostgreSQL cluster dump archive before removal?" + core.NC,
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
					core.BOLD + core.RED + "Database Storage & Cluster Cleanup Policy" + core.NC,
					"",
					"  Choose whether to permanently purge database clusters and tables:",
					"  " + core.BOLD + core.Sym.Pointer + " Native Mode :" + core.NC + " /var/lib/postgresql and configuration files",
					"  " + core.BOLD + core.Sym.Pointer + " Docker Mode :" + core.NC + " Named volume (krotek_postgres_data) & " + composeDir,
					"",
					core.BOLD + core.RED + "DANGER: Purging cannot be undone unless you created a backup!" + core.NC,
				}
				pIdx, _, pStatus := tui.SelectMenuWithContent("Step 2/3: Data Purge Policy", purgeLines, []string{
					"KEEP DATA (Retain database files for future re-installation)",
					"PURGE ALL (Completely erase all database tables & clusters)",
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
					tui.FormatKV("Action", "UNINSTALL POSTGRESQL DATABASE SERVER", core.RED+core.BOLD),
					tui.FormatKV("Deployment Target", func() string {
						if envType == "docker" {
							return "Docker Compose Container Stack"
						}
						return "Native Linux System Service"
					}(), core.CYAN),
					tui.FormatKV("Automated Cluster Backup", func() string {
						if uninstallBackup {
							return "[YES] (Archive to /root/postgres_backup_*.sql.gz)"
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
					core.BOLD + core.RED + "Confirm: Proceed with PostgreSQL uninstallation now?" + core.NC,
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
	fmt.Printf("\r\n%s=== Uninstalling PostgreSQL Database Server ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)
	logger.LogInfo("Starting PostgreSQL uninstallation...")

	// 1. Perform Pre-Removal SQL Backup
	if uninstallBackup {
		timestamp := time.Now().Format("20060102_150405")
		backupPath := fmt.Sprintf("/root/postgres_backup_%s.sql.gz", timestamp)
		_ = os.MkdirAll("/root", 0700)

		if envType == "docker" {
			_ = system.StreamCommandGzipNoFollow(backupPath, exec.Command("docker", "exec", "krotek-postgres", "pg_dumpall", "-U", "postgres"))
		} else {
			_ = system.StreamCommandGzipNoFollow(backupPath, exec.Command("su", "-", "postgres", "-c", "pg_dumpall"))
		}

		if fi, err := os.Stat(backupPath); err == nil && fi.Size() > 0 {
			logger.LogSuccess(fmt.Sprintf("Database backup successfully saved to: %s", backupPath))
		}
	}

	// 2. Teardown Service / Containers
	if envType == "docker" {
		if uninstallPurgeData {
			if !system.RunDockerCompose(composeDir, "Stopping & purging Docker containers and volumes", false, "down", "-v") {
				_ = exec.Command("docker", "rm", "-f", "krotek-postgres").Run()
			}
			_ = os.RemoveAll(composeDir)
		} else {
			if !system.RunDockerCompose(composeDir, "Stopping Docker containers (retaining volumes)", false, "down") {
				_ = exec.Command("docker", "stop", "krotek-postgres").Run()
			}
		}
	} else {
		if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
			_ = system.RunMultiStep("Stopping & disabling PostgreSQL in OpenRC", "rc-service postgresql stop 2>/dev/null || true; rc-update del postgresql default 2>/dev/null || true")
		} else {
			_ = system.RunMultiStep("Stopping & disabling PostgreSQL service", "systemctl stop postgresql 2>/dev/null || systemctl stop postgres 2>/dev/null || true; systemctl disable postgresql 2>/dev/null || systemctl disable postgres 2>/dev/null || true")
		}

		if uninstallPurgeData {
			_ = system.RunMultiStep("Purging PostgreSQL packages", "apt-get purge -y postgresql postgresql-contrib 2>/dev/null || dnf remove -y postgresql-server postgresql 2>/dev/null || apk del postgresql 2>/dev/null || true")
			_ = os.RemoveAll("/var/lib/postgresql")
			_ = os.RemoveAll("/var/lib/pgsql")
			_ = os.RemoveAll("/etc/postgresql")
		}
	}

	// 3. Remove Firewall Rules
	system.RemoveFirewallRules(port)

	// 4. Remove Global Shortcuts & State
	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/postgresql-manager")
	_ = os.Remove("/usr/local/bin/postgresql-installer")

	// 5. Display Clean Uninstall Box
	var finalLines []string
	finalLines = append(finalLines, tui.FormatKV("Service State", "Completely Stopped & Unregistered", core.GREEN))
	if uninstallBackup {
		finalLines = append(finalLines, tui.FormatKV("Backup Archive", "Retained in /root/postgres_backup_*.sql.gz", core.GREEN))
	}
	if uninstallPurgeData {
		finalLines = append(finalLines, tui.FormatKV("Database Data", "Purged and deleted cleanly", core.RED))
	} else {
		finalLines = append(finalLines, tui.FormatKV("Database Data", "Retained on disk for safety", core.YELLOW))
	}
	finalLines = append(finalLines, tui.FormatKV("CLI Shortcuts", "Removed (/usr/local/bin/postgresql-manager)", core.GREEN))

	if !core.AutoYes {
		tui.EnterAltScreen()
		tui.ClearScreen()
		tui.PrintInBox("PostgreSQL Uninstallation Complete", finalLines)
		tui.LeaveAltScreen()
	}
	fmt.Print("\r\n")
	logger.LogSuccess("PostgreSQL uninstalled successfully.")
}

// UninstallWizard is an alias for backward compatibility.
// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	UninstallPostgreSQL()
}
