// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: MONGODB (database)
// File:    database/mongodb/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-mongodb-installer/core"
	"krotek-mongodb-installer/logger"
	"krotek-mongodb-installer/system"
	"krotek-mongodb-installer/tui"
)

// UninstallMongoDB conducts a safe, stepped removal of MongoDB Native or Docker deployments.
func UninstallMongoDB() {
	logger.InitUninstallLogger("mongodb")
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		if _, err := os.Stat("/etc/mongodb-docker"); err == nil {
			envType = "docker"
		} else {
			envType = "native"
		}
	}

	uninstallBackup := true
	uninstallPurgeData := false
	composeDir := state["COMPOSE_DIR"]
	if composeDir == "" {
		composeDir = "/etc/mongodb-docker"
	}
	port := state["PORT"]
	if port == "" {
		port = "27017"
	}

	if !core.AutoYes {
		uStep := 1
		for {
			switch uStep {
			case 1:
				tui.ClearScreen()
				promptLines := []string{
					core.BOLD + core.CYAN + "Database Safety & Pre-removal Backup (mongodump)" + core.NC,
					"",
					"  Before uninstalling MongoDB, an automated logical database dump",
					"  can be created and compressed into: " + core.YELLOW + "/root/mongodb_backup_*.gz" + core.NC,
					"",
					core.BOLD + core.YELLOW + "Create a MongoDB database dump archive before removal?" + core.NC,
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
					core.BOLD + core.RED + "Database Storage & Collection Cleanup Policy" + core.NC,
					"",
					"  Choose whether to permanently purge database collections and volumes:",
					"  " + core.BOLD + core.Sym.Pointer + " Native Mode :" + core.NC + " /var/lib/mongodb and configuration files",
					"  " + core.BOLD + core.Sym.Pointer + " Docker Mode :" + core.NC + " Named volume (krotek_mongodb_data) & " + composeDir,
					"",
					core.BOLD + core.RED + "DANGER: Purging cannot be undone unless you created a backup!" + core.NC,
				}
				pIdx, _, pStatus := tui.SelectMenuWithContent("Step 2/3: Data Purge Policy", purgeLines, []string{
					"KEEP DATA (Retain database files for future re-installation)",
					"PURGE ALL (Completely erase all MongoDB collections & storage)",
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
					tui.FormatKV("Action", "UNINSTALL MONGODB NOSQL DATABASE", core.RED+core.BOLD),
					tui.FormatKV("Deployment Target", func() string {
						if envType == "docker" {
							return "Docker Compose Container Stack"
						}
						return "Native Linux System Service"
					}(), core.CYAN),
					tui.FormatKV("Automated Dump Backup", func() string {
						if uninstallBackup {
							return "[YES] (Archive to /root/mongodb_backup_*.gz)"
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
					core.BOLD + core.RED + "Confirm: Proceed with MongoDB uninstallation now?" + core.NC,
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
	fmt.Printf("\r\n%s=== Uninstalling MongoDB Database Server ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)
	logger.LogInfo("Starting MongoDB uninstallation...")

	// 1. Perform Pre-Removal SQL Backup
	if uninstallBackup {
		timestamp := time.Now().Format("20060102_150405")
		backupPath := fmt.Sprintf("/root/mongodb_backup_%s.gz", timestamp)
		_ = os.MkdirAll("/root", 0700)

		adminUser := state["ADMIN_USER"]
		if adminUser == "" {
			adminUser = "admin"
		}
		adminPass := state["ADMIN_PASS"]

		if envType == "docker" {
			var dumpCmd *exec.Cmd
			if adminPass != "" {
				if passJSON, jErr := json.Marshal(adminPass); jErr == nil {
					dumpCmd = exec.Command("docker", "exec", "-i", "krotek-mongod", "sh", "-c",
						`umask 077; cfg=$(mktemp) && trap 'rm -f "$cfg"' EXIT && cat > "$cfg" && exec mongodump -u "$1" --authenticationDatabase admin --config "$cfg" --archive`,
						"_", adminUser)
					dumpCmd.Stdin = bytes.NewReader([]byte("password: " + string(passJSON) + "\n"))
				}
			} else {
				dumpCmd = exec.Command("docker", "exec", "krotek-mongod", "mongodump", "--archive")
			}
			if dumpCmd != nil {
				_ = system.StreamCommandGzipNoFollow(backupPath, dumpCmd)
			}
		} else {
			if adminPass != "" {
				if tmpDir, cfgPath, cErr := system.BuildMongoDumpConfig(adminPass); cErr == nil {
					dumpCmd := exec.Command("mongodump", "--port", port, "-u", adminUser, "--authenticationDatabase", "admin", "--config", cfgPath, "--archive")
					_ = system.StreamCommandGzipNoFollow(backupPath, dumpCmd)
					_ = os.RemoveAll(tmpDir)
				}
			} else {
				_ = system.StreamCommandGzipNoFollow(backupPath, exec.Command("mongodump", "--port", port, "--archive"))
			}
		}

		if fi, err := os.Stat(backupPath); err == nil && fi.Size() > 0 {
			logger.LogSuccess(fmt.Sprintf("MongoDB backup successfully saved to: %s", backupPath))
		}
	}

	// 2. Teardown Service / Containers
	if envType == "docker" {
		if uninstallPurgeData {
			if !system.RunDockerCompose(composeDir, "Stopping & purging Docker containers and volumes", false, "down", "-v") {
				_ = exec.Command("docker", "rm", "-f", "krotek-mongod").Run()
			}
			_ = os.RemoveAll(composeDir)
		} else {
			if !system.RunDockerCompose(composeDir, "Stopping Docker containers (retaining volumes)", false, "down") {
				_ = exec.Command("docker", "stop", "krotek-mongod").Run()
			}
		}
	} else {
		if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
			_ = system.RunMultiStep("Stopping & disabling MongoDB in OpenRC", "rc-service mongodb stop 2>/dev/null || rc-service mongod stop 2>/dev/null || true; rc-update del mongodb default 2>/dev/null || true")
		} else {
			_ = system.RunMultiStep("Stopping & disabling MongoDB service", "systemctl stop mongod 2>/dev/null || systemctl stop mongodb 2>/dev/null || true; systemctl disable mongod 2>/dev/null || systemctl disable mongodb 2>/dev/null || true")
		}

		if uninstallPurgeData {
			_ = system.RunMultiStep("Purging MongoDB packages", "apt-get purge -y mongodb-org* mongodb* 2>/dev/null || dnf remove -y mongodb-org* 2>/dev/null || apk del mongodb 2>/dev/null || true")
			_ = os.RemoveAll("/var/lib/mongodb")
			_ = os.RemoveAll("/etc/mongod.conf")
			_ = os.RemoveAll("/etc/mongodb.conf")
		}
	}

	// 3. Remove Firewall Rules
	system.RemoveFirewallRules(port)

	// 4. Remove Global Shortcuts & State
	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/mongodb-manager")
	_ = os.Remove("/usr/local/bin/mongodb-installer")

	// 5. Display Clean Uninstall Box
	var finalLines []string
	finalLines = append(finalLines, tui.FormatKV("Service State", "Completely Stopped & Unregistered", core.GREEN))
	if uninstallBackup {
		finalLines = append(finalLines, tui.FormatKV("Backup Archive", "Retained in /root/mongodb_backup_*.gz", core.GREEN))
	}
	if uninstallPurgeData {
		finalLines = append(finalLines, tui.FormatKV("MongoDB Storage", "Purged and deleted cleanly", core.RED))
	} else {
		finalLines = append(finalLines, tui.FormatKV("MongoDB Storage", "Retained on disk for safety", core.YELLOW))
	}
	finalLines = append(finalLines, tui.FormatKV("CLI Shortcuts", "Removed (/usr/local/bin/mongodb-manager)", core.GREEN))

	if !core.AutoYes {
		tui.EnterAltScreen()
		tui.ClearScreen()
		tui.PrintInBox("MongoDB Uninstallation Complete", finalLines)
		tui.LeaveAltScreen()
	}
	fmt.Print("\r\n")
	logger.LogSuccess("MongoDB uninstalled successfully.")
}

// UninstallWizard is an alias for backward compatibility.
// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	UninstallMongoDB()
}
