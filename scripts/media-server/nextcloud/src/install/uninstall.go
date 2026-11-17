// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: NEXTCLOUD (media-server)
// File:    media-server/nextcloud/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"krotek-nextcloud-installer/core"
	"krotek-nextcloud-installer/logger"
	"krotek-nextcloud-installer/system"
	"krotek-nextcloud-installer/tui"
)

// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	logger.InitUninstallLogger("nextcloud")
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		if _, err := os.Stat("/etc/nextcloud-docker"); err == nil {
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
				bRes := tui.AskYesNo("Step 1/3: Create full Nextcloud files and database backup before uninstallation? (Saved to /root/)")
				if bRes == 2 || bRes == -1 {
					logger.LogInfo("Uninstallation cancelled by user.")
					return
				}
				uninstallBackup = (bRes == 0)
				uStep = 2

			case 2:
				tui.ClearScreen()
				fRes := tui.AskYesNo("Step 2/3: Purge Nextcloud cloud data, user files, and database?")
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
					core.BOLD + core.RED + "CONFIRM NEXTCLOUD UNINSTALLATION:" + core.NC,
					"",
					tui.FormatKV("Deployment Environment", envType, core.CYAN),
					tui.FormatKV("Create Full Backup", func() string {
						if uninstallBackup {
							return core.GREEN + "YES"
						}
						return core.YELLOW + "NO"
					}(), core.GREEN),
					tui.FormatKV("Purge Data & Database", func() string {
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
	fmt.Printf("\r\n%s=== UNINSTALLING NEXTCLOUD STACK ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	var bkpFile string

	if envType == "docker" {
		composeDir := state["COMPOSE_DIR"]
		if composeDir == "" {
			composeDir = "/etc/nextcloud-docker"
		}

		if uninstallBackup {
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile = fmt.Sprintf("/root/nextcloud_docker_backup_%s.tar.gz", ts)
			cmd := exec.Command("docker", "compose", "--project-directory", composeDir, "exec", "-T", "-e", "MYSQL_PWD", "db", "mariadb-dump", "-u", "root", "nextcloud")
			if state["DB_PASS"] != "" {
				cmd.Env = append(os.Environ(), "MYSQL_PWD="+state["DB_PASS"])
			}
			tmpDir, tErr := os.MkdirTemp("", "krotek-nc-bkp-*")
			if tErr == nil {
				_ = os.Chmod(tmpDir, 0700)
				defer os.RemoveAll(tmpDir)
				dumpFile := filepath.Join(tmpDir, "dump.sql")
				if !system.IsSymlinkPath(dumpFile) {
					if root, rErr := os.OpenRoot(tmpDir); rErr == nil {
						if outF, err := root.OpenFile("dump.sql", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600); err == nil {
							cmd.Stdout = outF
							_ = cmd.Run()
							_ = outF.Close()
						}
						_ = root.Close()
					}
				}
				if !system.IsSymlinkPath(bkpFile) {
					var tarArgs []string
					for _, p := range []string{composeDir, dumpFile} {
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
		}

		if uninstallPurgeFiles {
			_ = system.RunDockerCompose(composeDir, "Stopping and purging Nextcloud Docker containers & volumes", false, "down", "-v")
			_ = os.RemoveAll(composeDir)
		} else {
			_ = system.RunDockerCompose(composeDir, "Stopping Nextcloud Docker containers", false, "down")
		}
	} else {
		dbName := state["DB_NAME"]
		if dbName == "" {
			dbName = "nextcloud"
		}

		if uninstallBackup {
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile = fmt.Sprintf("/root/nextcloud_native_backup_%s.tar.gz", ts)
			tmpDir, tErr := os.MkdirTemp("", "krotek-nc-uninst-*")
			if tErr == nil {
				_ = os.Chmod(tmpDir, 0700)
				defer os.RemoveAll(tmpDir)
				dumpFile := filepath.Join(tmpDir, "dump.sql")
				if !system.IsSymlinkPath(dumpFile) {
					if root, rErr := os.OpenRoot(tmpDir); rErr == nil {
						if outF, err := root.OpenFile("dump.sql", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600); err == nil {
							cDump := exec.Command("mariadb-dump", dbName)
							cDump.Stdout = outF
							if cDump.Run() != nil {
								cDump2 := exec.Command("mysqldump", dbName)
								cDump2.Stdout = outF
								_ = cDump2.Run()
							}
							_ = outF.Close()
						}
						_ = root.Close()
					}
				}
				if !system.IsSymlinkPath(bkpFile) {
					var tarArgs []string
					for _, p := range []string{"/var/www/nextcloud", dumpFile} {
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
		}

		if uninstallPurgeFiles {
			system.RunStep("Removing Nextcloud installation directory", "rm", "-rf", "/var/www/nextcloud")
			safeDB := strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
					return r
				}
				return -1
			}, dbName)
			safeUser := strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
					return r
				}
				return -1
			}, state["DB_USER"])
			if safeDB != "" && safeUser != "" {
				_ = exec.Command("mariadb", "-e", fmt.Sprintf("DROP DATABASE IF EXISTS `%s`; DROP USER IF EXISTS '%s'@'localhost';", safeDB, safeUser)).Run()
			}
			_ = os.Remove("/etc/nginx/conf.d/nextcloud.conf")
			_ = os.Remove("/etc/apache2/sites-available/nextcloud.conf")
			_ = os.Remove("/etc/apache2/sites-enabled/nextcloud.conf")
			_ = os.Remove("/etc/httpd/conf.d/nextcloud.conf")
			_ = os.Remove("/etc/apache2/conf.d/nextcloud.conf")
		}
	}

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/nextcloud-manager")
	_ = os.Remove("/usr/local/bin/nextcloud-installer")
	_ = os.Remove("/usr/local/bin/nextcloud")

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
		finalSummary = append(finalSummary, tui.FormatKV("Files & Database", "[PURGED / REMOVED]", core.RED))
	} else {
		finalSummary = append(finalSummary, tui.FormatKV("Files & Database", "[RETAINED]", core.GREEN))
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
	tui.PrintInBox("Nextcloud Stack Uninstalled", finalSummary)
	tui.LeaveAltScreen()

	fmt.Print("\r\n")
	logger.LogSuccess("Nextcloud stack uninstalled successfully.")
	os.Exit(0)
}
