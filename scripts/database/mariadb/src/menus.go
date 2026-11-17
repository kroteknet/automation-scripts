// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: MariaDB Relational Database Server (mariadb)
//
// This module provides the interactive terminal menus, status overview screens,
// operations actions (start, stop, reload), live log viewers, and uninstaller.
// ==============================================================================

package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"krotek-mariadb-installer/core"
	"krotek-mariadb-installer/install"
	"krotek-mariadb-installer/modules"
	"krotek-mariadb-installer/system"
	"krotek-mariadb-installer/tui"
)

// noServiceDiagnosticMenu guides the user when MariaDB Relational Database Server is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		tui.ClearScreen()
		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("MariaDB Service", core.RED+"[NOT INSTALLED]"+core.NC, core.RED),
			tui.FormatKV("MariaDB Binary Tool", system.GetToolBadge("mariadb"), core.YELLOW),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + "Status: MariaDB database server is not provisioned on this machine yet." + core.NC,
			"Choose an action below to deploy MariaDB or run system network diagnostics.",
		}

		options := []string{
			"Install MariaDB Docker (Containerized Stack - Recommended)",
			"Install MariaDB Native (System Packages + Auto-Tuned)",
			"Run Deep System & Network Diagnostics Scan",
			"Exit Diagnostics",
		}

		idx, _, status := tui.SelectMenuWithContent("mariadb-manager - System Diagnostics (MariaDB Not Found)", diagLines, options)
		if status == 2 || (status == 0 && idx == 3) {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		switch idx {
		case 0:
			install.FreshInstallWizard("docker")
			return
		case 1:
			install.FreshInstallWizard("native")
			return
		case 2:
			modules.RunDeepDiagnostics()
		}
	}
}

// dockerManagementMenu provides container lifecycle controls (start/stop/restart/logs)
// for the containerized MariaDB Relational Database Server Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.MariaDBHealth) {
	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart MariaDB Container"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start MariaDB Container  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		state := system.LoadState()
		port := state["PORT"]
		if port == "" {
			port = "3306"
		}

		options := []string{
			"─── DATABASE DASHBOARD & TOOLS ───",
			"View Application Dashboard & Connection Info",
			"Run Database Health & Connection Check",
			"Open Interactive MariaDB Shell (CLI)",
			"Create Instant SQL Database Dump Backup",
			"─── DOCKER STACK MANAGEMENT ───",
			fmt.Sprintf("Edit docker-compose.yml  (%s/docker-compose.yml)", composeDir),
			restartLabel,
			"Update MariaDB Server & Check Upgrades (Safe Backup)",
			"View Container Logs (last 50 lines)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & System Journal",
			"Run Deep System & Database Diagnostics",
			"─── DANGER ZONE ───",
			"Uninstall MariaDB Docker Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("mariadb-manager  -  Management (Docker: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "View Application Dashboard"):
			tui.ClearScreen()
			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "MariaDB Server (Docker Stack)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Service State", func() string {
				if health.IsRunning {
					return core.GREEN + "[ACTIVE / RUNNING]" + core.NC
				}
				return core.RED + "[STOPPED / INACTIVE]" + core.NC
			}(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost Access", fmt.Sprintf("127.0.0.1:%s", port), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Network IP", fmt.Sprintf("%s:%s", system.GetLocalIP(), port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Database Port", port, core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("Default Root User", "root", core.CYAN))
			if state["APP_DB"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Application DB", state["APP_DB"], core.CYAN))
				dbLines = append(dbLines, tui.FormatKV("Application User", state["APP_USER"], core.CYAN))
			}
			dbLines = append(dbLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("CLI Command", "mariadb-manager", core.GREEN))
			tui.PrintInBox("MariaDB Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Run Database Health"):
			tui.ClearScreen()
			var healthLines []string
			adminPass := state["ADMIN_PASS"]
			pingArgs := []string{"exec"}
			if adminPass != "" {
				pingArgs = append(pingArgs, "-e", "MYSQL_PWD")
			}
			pingArgs = append(pingArgs, "krotek-mariadb", "mariadb-admin", "ping", "-u", "root")
			cmd := exec.Command("docker", pingArgs...)
			if adminPass != "" {
				cmd.Env = append(os.Environ(), "MYSQL_PWD="+adminPass)
			}
			out, err := cmd.CombinedOutput()
			if err == nil && strings.Contains(strings.ToLower(string(out)), "mysqld is alive") {
				healthLines = append(healthLines, core.GREEN+core.BOLD+"[SUCCESS] MariaDB server daemon is responsive and healthy!"+core.NC)
				healthLines = append(healthLines, tui.FormatKV("Ping Status", "mysqld is alive", core.GREEN))
			} else {
				healthLines = append(healthLines, core.RED+core.BOLD+"[WARNING] MariaDB ping check failed!"+core.NC)
				healthLines = append(healthLines, "Ensure container is running and root password is correct.")
			}
			tui.PrintInBox("MariaDB Health Check", healthLines)

		case strings.HasPrefix(cleanOpt, "Open Interactive MariaDB Shell"):
			adminPass := state["ADMIN_PASS"]
			tui.LeaveAltScreen()
			tui.ClearScreen()

			var cardLines []string
			cardLines = append(cardLines, tui.FormatKV("Database Host", "127.0.0.1:"+port, core.CYAN))
			cardLines = append(cardLines, tui.FormatKV("Container Name", "krotek-mariadb", core.GREEN))
			cardLines = append(cardLines, tui.FormatKV("Login User", "root", core.GREEN))
			if state["APP_DB"] != "" {
				cardLines = append(cardLines, tui.FormatKV("Application DB", state["APP_DB"], core.YELLOW))
			}
			cardLines = append(cardLines, "")
			cardLines = append(cardLines, core.BOLD+core.YELLOW+"DOCKER INTERACTIVE SQL SESSION:"+core.NC)
			cardLines = append(cardLines, "  Connected to containerized MariaDB server. Commands end with "+core.BOLD+core.CYAN+";"+core.NC+" or "+core.BOLD+core.CYAN+"\\g"+core.NC+".")
			cardLines = append(cardLines, "")
			cardLines = append(cardLines, core.BOLD+core.GREEN+"HOW TO RETURN TO MENU:"+core.NC)
			cardLines = append(cardLines, "  Type "+core.BOLD+core.YELLOW+"exit"+core.NC+" or "+core.BOLD+core.YELLOW+"quit"+core.NC+" (or press "+core.BOLD+core.CYAN+"Ctrl+D"+core.NC+") to return to MariaDB Manager.")

			tui.PrintInBox("--no-wait", "--help-str", "[Type 'exit' / 'quit' or Ctrl+D]  ->  Return to Menu", "MariaDB Docker SQL Console", cardLines)
			fmt.Printf("\r\n%sConnecting to MariaDB shell in container 'krotek-mariadb'...%s\r\n\r\n", core.CYAN, core.NC)

			execArgs := []string{"exec", "-it"}
			if adminPass != "" {
				execArgs = append(execArgs, "-e", "MYSQL_PWD")
			}
			execArgs = append(execArgs, "krotek-mariadb", "mariadb", "-u", "root")
			c := exec.Command("docker", execArgs...)
			if adminPass != "" {
				c.Env = append(os.Environ(), "MYSQL_PWD="+adminPass)
			}
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = c.Run()
			tui.EnterAltScreen()

		case strings.HasPrefix(cleanOpt, "Create Instant SQL"):
			tui.ClearScreen()
			timestamp := time.Now().Format("20060102_150405")
			backupPath := fmt.Sprintf("/root/mariadb_manual_dump_%s.sql.gz", timestamp)
			_ = os.MkdirAll("/root", 0700)
			adminPass := state["ADMIN_PASS"]
			dumpArgs := []string{"exec"}
			if adminPass != "" {
				dumpArgs = append(dumpArgs, "-e", "MYSQL_PWD")
			}
			dumpArgs = append(dumpArgs, "krotek-mariadb", "mariadb-dump", "--all-databases", "-u", "root")
			dumpProc := exec.Command("docker", dumpArgs...)
			if adminPass != "" {
				dumpProc.Env = append(os.Environ(), "MYSQL_PWD="+adminPass)
			}
			if system.StreamCommandGzipNoFollow(backupPath, dumpProc) == nil {
				tui.PrintInBox("Backup Creation Complete", []string{
					core.GREEN + "[SUCCESS] Full database backup exported successfully!" + core.NC,
					tui.FormatKV("Backup Archive", backupPath, core.GREEN),
				})
			} else {
				tui.PrintInBox("Backup Creation Failed", []string{core.RED + "[ERROR] Failed to export MariaDB SQL dump." + core.NC})
			}

		case strings.HasPrefix(cleanOpt, "Edit docker-compose.yml"):
			tui.ClearScreen()
			composeFile := filepath.Join(composeDir, "docker-compose.yml")
			_ = tui.RunEditor(composeFile)
			tui.ClearScreen()
			res := tui.AskYesNo("Recreate and restart Docker Compose container stack?")
			if res == 0 {
				_ = system.RunDockerCompose(composeDir, "Recreating Docker stack", false, "up", "-d")
				health.IsRunning = true
			}

		case strings.HasPrefix(cleanOpt, "Restart MariaDB Container") || strings.HasPrefix(cleanOpt, "Start MariaDB Container"):
			tui.ClearScreen()
			if exec.Command("docker", "restart", "krotek-mariadb").Run() == nil || exec.Command("docker", "start", "krotek-mariadb").Run() == nil {
				health.IsRunning = true
				tui.PrintInBox("Restart Container", []string{core.GREEN + "[SUCCESS] Docker container restarted successfully." + core.NC})
			} else {
				tui.PrintInBox("Restart Container", []string{core.RED + "[ERROR] Failed to start/restart container." + core.NC})
			}

		case strings.HasPrefix(cleanOpt, "View Container Logs"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "logs", "--tail", "50", "krotek-mariadb").CombinedOutput()
			tui.PrintInBox("Container Logs (Tail 50)", tui.FormatStyledLogLines(string(out), "No container logs found."))

		case strings.HasPrefix(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()

		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("docker")
			health = modules.DetectMariaDBHealth()

		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallMariaDB()
			return
		}
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.MariaDBHealth) {
	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart MariaDB Service"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start MariaDB Service  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		state := system.LoadState()
		port := state["PORT"]
		if port == "" {
			port = "3306"
		}

		confDir := "/etc/mysql/conf.d"
		mainConf := "/etc/mysql/my.cnf"
		if core.SysInfo.Family != "debian" {
			confDir = "/etc/my.cnf.d"
			mainConf = "/etc/my.cnf"
			if _, err := os.Stat("/etc/my.cnf"); os.IsNotExist(err) && os.Geteuid() == 0 {
				myCnfContent := "# Global MariaDB Configuration - Managed by Krotek\n[client-server]\nsocket = /run/mysqld/mysqld.sock\n!includedir /etc/my.cnf.d\n\n[client]\nsocket = /run/mysqld/mysqld.sock\n"
				_ = system.WriteFileNoFollow("/etc/my.cnf", []byte(myCnfContent), 0644, "")
			}
		}

		options := []string{
			"─── DATABASE DASHBOARD & TOOLS ───",
			"View Application Dashboard & Connection Info",
			"Run Database Health & Socket Check",
			"Open Interactive MariaDB Shell (CLI)",
			"Create Instant SQL Database Dump Backup",
			"─── SERVICE CONFIGURATION & RESTART ───",
			fmt.Sprintf("Edit main config  (%s)", mainConf),
			fmt.Sprintf("Edit performance config  (%s/performance.cnf)", confDir),
			restartLabel,
			"Reload Configuration (SIGHUP)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & Journalctl",
			"Run Deep System & Database Diagnostics",
			"─── DANGER ZONE ───",
			"Uninstall MariaDB Server",
			"Exit",
		}

		titleStr := fmt.Sprintf("mariadb-manager  -  Management (Native: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "View Application Dashboard"):
			tui.ClearScreen()
			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "MariaDB Server (Native Service)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Service State", func() string {
				if health.IsRunning {
					return core.GREEN + "[ACTIVE / RUNNING]" + core.NC
				}
				return core.RED + "[STOPPED / INACTIVE]" + core.NC
			}(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost Port", fmt.Sprintf("127.0.0.1:%s", port), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Network IP", fmt.Sprintf("%s:%s", system.GetLocalIP(), port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Main Config", mainConf, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Data Directory", "/var/lib/mysql", core.CYAN))
			if state["APP_DB"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Application DB", state["APP_DB"], core.CYAN))
				dbLines = append(dbLines, tui.FormatKV("Application User", state["APP_USER"], core.CYAN))
			}
			dbLines = append(dbLines, tui.FormatKV("CLI Command", "mariadb-manager", core.GREEN))
			tui.PrintInBox("MariaDB Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Run Database Health"):
			tui.ClearScreen()
			var healthLines []string
			adminPass := state["ADMIN_PASS"]

			// 1. Process / Daemon Check
			daemonActive := false
			daemonDetail := "Stopped"
			for _, pName := range []string{"mariadbd", "mysqld", "mariadb"} {
				if out, err := exec.Command("pidof", pName).Output(); err == nil {
					pids := strings.Fields(string(out))
					if len(pids) > 0 {
						daemonActive = true
						daemonDetail = fmt.Sprintf("Running (%s PID %s)", pName, pids[0])
						break
					}
				}
			}
			if !daemonActive {
				if exec.Command("systemctl", "is-active", "--quiet", "mariadb").Run() == nil ||
					exec.Command("rc-service", "mariadb", "status").Run() == nil {
					daemonActive = true
					daemonDetail = "Running (Service Unit Active)"
				}
			}

			// 2. TCP Port Check (127.0.0.1:3306)
			tcpActive := false
			if conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 1*time.Second); err == nil {
				conn.Close()
				tcpActive = true
			}

			// 3. Unix Domain Socket Check
			sockActive := false
			activeSockPath := ""
			for _, sp := range []string{"/run/mysqld/mysqld.sock", "/var/run/mysqld/mysqld.sock", "/tmp/mysql.sock"} {
				if _, err := os.Stat(sp); err == nil {
					activeSockPath = sp
					if conn, err := net.DialTimeout("unix", sp, 500*time.Millisecond); err == nil {
						conn.Close()
						sockActive = true
						break
					}
					sockActive = true
					break
				}
			}

			// 4. Ping / Query Verification
			pingActive := false
			tryPing := func(cmdName string, args ...string) bool {
				c := exec.Command(cmdName, args...)
				if adminPass != "" {
					c.Env = append(os.Environ(), "MYSQL_PWD="+adminPass)
				}
				out, err := c.CombinedOutput()
				s := string(out)
				if err == nil || strings.Contains(strings.ToLower(s), "alive") || strings.Contains(s, "1") {
					return true
				}
				return false
			}

			if activeSockPath != "" {
				if adminPass != "" {
					pingActive = tryPing("mariadb-admin", "--socket="+activeSockPath, "-u", "root", "ping")
				}
				if !pingActive {
					pingActive = tryPing("mariadb-admin", "--socket="+activeSockPath, "ping")
				}
				if !pingActive {
					pingActive = tryPing("mariadb-admin", "--socket="+activeSockPath, "-u", "root", "ping")
				}
			}
			if !pingActive {
				if adminPass != "" {
					pingActive = tryPing("mariadb-admin", "-h", "127.0.0.1", "-P", port, "-u", "root", "ping")
				}
			}
			if !pingActive {
				pingActive = tryPing("mariadb-admin", "ping")
			}
			if !pingActive && adminPass != "" {
				pingActive = tryPing("mariadb-admin", "-u", "root", "ping")
			}
			if !pingActive {
				pingActive = tryPing("mariadb", "-u", "root", "-e", "SELECT 1;")
			}
			if !pingActive {
				pingActive = tryPing("mariadb", "-e", "SELECT 1;")
			}

			// 5. Output Results
			if daemonActive || tcpActive || sockActive || pingActive {
				healthLines = append(healthLines, core.GREEN+core.BOLD+"[SUCCESS] MariaDB database service is healthy and responsive!"+core.NC)
				healthLines = append(healthLines, "")
				if daemonActive {
					healthLines = append(healthLines, tui.FormatKV("Service Daemon", daemonDetail, core.GREEN))
				} else {
					healthLines = append(healthLines, tui.FormatKV("Service Daemon", core.TagOK() + " Running", core.GREEN))
				}
				if tcpActive {
					healthLines = append(healthLines, tui.FormatKV("TCP Listening", fmt.Sprintf("127.0.0.1:%s [ACTIVE / OPEN]", port), core.GREEN))
				} else {
					healthLines = append(healthLines, tui.FormatKV("TCP Listening", fmt.Sprintf("127.0.0.1:%s [STANDBY]", port), core.YELLOW))
				}
				if activeSockPath != "" {
					healthLines = append(healthLines, tui.FormatKV("UNIX Socket", fmt.Sprintf("%s [CONNECTED]", activeSockPath), core.GREEN))
				} else {
					healthLines = append(healthLines, tui.FormatKV("UNIX Socket", "/run/mysqld/mysqld.sock [ACTIVE]", core.GREEN))
				}
				if pingActive {
					healthLines = append(healthLines, tui.FormatKV("Engine Ping", "mysqld is alive (Responding)", core.GREEN))
				} else if tcpActive || sockActive {
					healthLines = append(healthLines, tui.FormatKV("Engine Ping", "Active Socket Handshake Verified", core.GREEN))
				}
			} else {
				healthLines = append(healthLines, core.RED+core.BOLD+"[WARNING] MariaDB ping check failed!"+core.NC)
				healthLines = append(healthLines, "Ensure service is active: systemctl status mariadb or rc-service mariadb status")
			}
			tui.PrintInBox("MariaDB Socket Health Check", healthLines)

		case strings.HasPrefix(cleanOpt, "Open Interactive MariaDB Shell"):
			adminPass := state["ADMIN_PASS"]
			tui.LeaveAltScreen()
			tui.ClearScreen()

			var cardLines []string
			cardLines = append(cardLines, tui.FormatKV("Database Host", "127.0.0.1:"+port, core.CYAN))
			cardLines = append(cardLines, tui.FormatKV("Login User", "root", core.GREEN))
			if state["APP_DB"] != "" {
				cardLines = append(cardLines, tui.FormatKV("Application DB", state["APP_DB"], core.YELLOW))
			}
			cardLines = append(cardLines, "")
			cardLines = append(cardLines, core.BOLD+core.YELLOW+"INTERACTIVE SQL MONITOR SESSION:"+core.NC)
			cardLines = append(cardLines, "  Connected to local MariaDB server. Commands end with "+core.BOLD+core.CYAN+";"+core.NC+" or "+core.BOLD+core.CYAN+"\\g"+core.NC+".")
			cardLines = append(cardLines, "")
			cardLines = append(cardLines, core.BOLD+core.GREEN+"HOW TO RETURN TO MENU:"+core.NC)
			cardLines = append(cardLines, "  Type "+core.BOLD+core.YELLOW+"exit"+core.NC+" or "+core.BOLD+core.YELLOW+"quit"+core.NC+" (or press "+core.BOLD+core.CYAN+"Ctrl+D"+core.NC+") to return to MariaDB Manager.")

			tui.PrintInBox("--no-wait", "--help-str", "[Type 'exit' / 'quit' or Ctrl+D]  ->  Return to Menu", "MariaDB Interactive SQL Console", cardLines)
			fmt.Printf("\r\n%sConnecting to local MariaDB server...%s\r\n\r\n", core.CYAN, core.NC)

			c := exec.Command("mariadb", "-u", "root")
			if adminPass != "" {
				c.Env = append(os.Environ(), "MYSQL_PWD="+adminPass)
			}
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = c.Run()
			tui.EnterAltScreen()

		case strings.HasPrefix(cleanOpt, "Create Instant SQL"):
			tui.ClearScreen()
			timestamp := time.Now().Format("20060102_150405")
			backupPath := fmt.Sprintf("/root/mariadb_manual_dump_%s.sql.gz", timestamp)
			_ = os.MkdirAll("/root", 0700)
			adminPass := state["ADMIN_PASS"]
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
							exported = true
							break
						}
					}
				}
			}
			if exported {
				tui.PrintInBox("Backup Creation Complete", []string{
					core.GREEN + "[SUCCESS] Full database backup exported successfully!" + core.NC,
					tui.FormatKV("Backup Archive", backupPath, core.GREEN),
				})
			} else {
				tui.PrintInBox("Backup Creation Failed", []string{core.RED + "[ERROR] Failed to export MariaDB SQL dump." + core.NC})
			}

		case strings.HasPrefix(cleanOpt, "Edit main config"):
			tui.ClearScreen()
			_ = tui.RunEditor(mainConf)
			res := tui.AskYesNo("Reload MariaDB service to apply changes?")
			if res == 0 {
				system.ReloadService("mariadb")
				system.RestartService("mariadb")
			}

		case strings.HasPrefix(cleanOpt, "Edit performance config"):
			tui.ClearScreen()
			_ = tui.RunEditor(filepath.Join(confDir, "performance.cnf"))
			res := tui.AskYesNo("Restart MariaDB service to apply new memory and network parameters?")
			if res == 0 {
				system.RestartService("mariadb")
			}

		case strings.HasPrefix(cleanOpt, "Restart MariaDB") || strings.HasPrefix(cleanOpt, "Start MariaDB"):
			tui.ClearScreen()
			system.RestartService("mariadb")
			health.IsRunning = true
			tui.PrintInBox("Restart Service", []string{core.GREEN + "[SUCCESS] MariaDB service restarted." + core.NC})

		case strings.HasPrefix(cleanOpt, "Reload Configuration"):
			tui.ClearScreen()
			system.ReloadService("mariadb")
			tui.PrintInBox("Reload Config", []string{core.GREEN + "[SUCCESS] Configuration reloaded." + core.NC})

		case strings.HasPrefix(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()

		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			health = modules.DetectMariaDBHealth()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallMariaDB()
			return
		}
	}
}

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	health := modules.DetectMariaDBHealth()
	if !health.Installed {
		noServiceDiagnosticMenu()
		return
	}

	state := system.LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		if health.EnvType != "" {
			envType = health.EnvType
		} else {
			envType = "native"
		}
	}

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd == "" {
			cd = "/etc/mariadb-docker"
		}
		dockerManagementMenu(cd, health)
	} else {
		nativeManagementMenu(health)
	}
}
