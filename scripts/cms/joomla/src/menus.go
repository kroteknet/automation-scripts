// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: Joomla CMS Application Server (joomla)
//
// This module provides the interactive terminal menus, status overview screens,
// operations actions (start, stop, reload), live log viewers, and uninstaller.
// ==============================================================================

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"krotek-joomla-installer/core"
	"krotek-joomla-installer/install"
	"krotek-joomla-installer/modules"
	"krotek-joomla-installer/system"
	"krotek-joomla-installer/tui"
)

// noServiceDiagnosticMenu guides the user when Joomla CMS Application Server is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		tui.ClearScreen()
		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("Joomla Service", core.RED+"[NOT INSTALLED]"+core.NC, core.RED),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			tui.FormatKV("Nginx Engine Tool", system.GetToolBadge("nginx"), core.YELLOW),
			tui.FormatKV("MariaDB Tool", system.GetToolBadge("mariadb"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + "Status: Joomla Content Management System is not provisioned on this machine." + core.NC,
			"Choose a deployment method below to install Joomla with automated MariaDB.",
		}

		options := []string{
			"Install Joomla Docker Stack (Joomla + MariaDB Containerized)",
			"Install Joomla Native Stack (Nginx + PHP-FPM + MariaDB LEMP)",
			"Run Deep System & Network Diagnostics Scan",
			"Exit Diagnostics",
		}

		idx, _, status := tui.SelectMenuWithContent("joomla-manager - System Diagnostics (Joomla Not Found)", diagLines, options)
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
// for the containerized Joomla CMS Application Server Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.JoomlaHealth) {
	for {
		health = modules.DetectJoomlaHealth()
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart Joomla Docker Stack"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start Joomla Stack  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		options := []string{
			"─── APPLICATION DASHBOARD ───",
			"View Application Dashboard & Access URLs",
			"Create Instant Backup (Files + Database Dump)",
			"─── STACK MANAGEMENT ───",
			fmt.Sprintf("Edit docker-compose.yml (%s/docker-compose.yml)", composeDir),
			restartLabel,
			"Update Joomla! CMS & Check Upgrades (Safe Backup)",
			"View Container Logs (last 50 lines)",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Guided Setup / Reinstallation Wizard",
			"Run Deep System Diagnostics",
			"View Service Logs & System Journal",
			"Uninstall Joomla Docker Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("joomla-manager  -  Management (Docker: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "View Application Dashboard"):
			tui.ClearScreen()
			state := system.LoadState()
			localIP := system.GetLocalIP()
			pubIP := system.GetPublicIP()
			port := state["PORT"]
			if port == "" {
				port = "80"
			}

			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "Joomla CMS (Docker Stack)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Status", statusBadge, core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
			if state["DOMAIN_NAME"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Domain URL", fmt.Sprintf("http://%s:%s", state["DOMAIN_NAME"], port), core.YELLOW))
			}
			dbLines = append(dbLines, "")
			dbLines = append(dbLines, core.BOLD+core.YELLOW+"DATABASE & SETTINGS:"+core.NC)
			dbLines = append(dbLines, tui.FormatKV("Database Host", "db (Container) / localhost", core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Database Name", state["DB_NAME"], core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Database User", state["DB_USER"], core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Database Password", state["DB_PASS"], core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Admin Suggested User", state["ADMIN_USER"], core.YELLOW+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Public Server IP", pubIP, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
			tui.PrintInBox("Joomla Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Create Instant Backup"):
			tui.ClearScreen()
			state := system.LoadState()
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile := fmt.Sprintf("/root/joomla_instant_backup_%s.tar.gz", ts)
			cmd := exec.Command("docker", "compose", "--project-directory", composeDir, "exec", "-T", "-e", "MYSQL_PWD", "db", "mariadb-dump", "-u", "root", "joomla")
			if state["DB_PASS"] != "" {
				cmd.Env = append(os.Environ(), "MYSQL_PWD="+state["DB_PASS"])
			}
			tmpDir, tErr := os.MkdirTemp("", "krotek-joomla-bkp-*")
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
			tui.PrintInBox("Backup Complete", []string{
				core.GREEN + "[SUCCESS] Full backup created successfully." + core.NC,
				tui.FormatKV("Archive File", bkpFile, core.CYAN),
			})

		case strings.HasPrefix(cleanOpt, "Edit docker-compose.yml"):
			tui.ClearScreen()
			_ = tui.RunEditor(composeDir + "/docker-compose.yml")

		case strings.HasPrefix(cleanOpt, "Restart Joomla Docker Stack") || strings.HasPrefix(cleanOpt, "Start Joomla Stack"):
			tui.ClearScreen()
			_ = system.RunDockerCompose(composeDir, "Restarting Joomla Docker stack", false, "restart")
			time.Sleep(1 * time.Second)

		case strings.HasPrefix(cleanOpt, "View Container Logs"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "compose", "--project-directory", composeDir, "logs", "--tail", "50").CombinedOutput()
			tui.PrintInBox("Container Logs (Last 50 Lines)", tui.FormatStyledLogLines(string(out), "No container logs found."))

		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.Contains(cleanOpt, "Guided Setup"):
			install.FreshInstallWizard("docker")
			return

		case strings.HasPrefix(cleanOpt, "Run Deep System Diagnostics"):
			modules.RunDeepDiagnostics()

		case strings.HasPrefix(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Uninstall Joomla"):
			install.UninstallWizard()
			return
		}
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.JoomlaHealth) {
	for {
		health = modules.DetectJoomlaHealth()
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart LEMP Services (Nginx + PHP-FPM + MariaDB)"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start LEMP Services  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		options := []string{
			"─── APPLICATION DASHBOARD ───",
			"View Application Dashboard & Access URLs",
			"Create Instant Backup (Files + Database Dump)",
			"─── SERVICE MANAGEMENT ───",
			restartLabel,
			"Reload Nginx Configuration",
			"Update Joomla! CMS & Check Upgrades (Safe Backup)",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Guided Setup / Reinstallation Wizard",
			"Run Deep System Diagnostics",
			"View Service Logs & System Journal",
			"Uninstall Joomla Native Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("joomla-manager  -  Management (Native: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "View Application Dashboard"):
			tui.ClearScreen()
			state := system.LoadState()
			localIP := system.GetLocalIP()
			pubIP := system.GetPublicIP()
			port := state["PORT"]
			if port == "" {
				port = "80"
			}

			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "Joomla CMS (Native LEMP)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Status", statusBadge, core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
			if state["DOMAIN_NAME"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Domain URL", fmt.Sprintf("http://%s:%s", state["DOMAIN_NAME"], port), core.YELLOW))
			}
			dbLines = append(dbLines, "")
			dbLines = append(dbLines, core.BOLD+core.YELLOW+"DATABASE & SETTINGS:"+core.NC)
			dbLines = append(dbLines, tui.FormatKV("Database Name", state["DB_NAME"], core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Database User", state["DB_USER"], core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Database Password", state["DB_PASS"], core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Public Server IP", pubIP, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Document Root", "/var/www/joomla", core.CYAN))
			tui.PrintInBox("Joomla Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Create Instant Backup"):
			tui.ClearScreen()
			state := system.LoadState()
			dbName := state["DB_NAME"]
			if dbName == "" {
				dbName = "joomla"
			}
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile := fmt.Sprintf("/root/joomla_instant_backup_%s.tar.gz", ts)
			tmpDir, tErr := os.MkdirTemp("", "krotek-joomla-inst-*")
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
					for _, p := range []string{"/var/www/joomla", dumpFile} {
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
			tui.PrintInBox("Backup Complete", []string{
				core.GREEN + "[SUCCESS] Full backup created successfully." + core.NC,
				tui.FormatKV("Archive File", bkpFile, core.CYAN),
			})

		case strings.HasPrefix(cleanOpt, "Restart LEMP Services") || strings.HasPrefix(cleanOpt, "Start LEMP Services"):
			tui.ClearScreen()
			_ = system.RunMultiStep("Restarting LEMP services", "rc-service nginx restart 2>/dev/null || systemctl restart nginx 2>/dev/null || true; rc-service php-fpm restart 2>/dev/null || systemctl restart php-fpm 2>/dev/null || true; rc-service mariadb restart 2>/dev/null || systemctl restart mariadb 2>/dev/null || true")
			time.Sleep(1 * time.Second)

		case strings.HasPrefix(cleanOpt, "Reload Nginx"):
			tui.ClearScreen()
			system.ReloadService("nginx")

		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.Contains(cleanOpt, "Guided Setup"):
			install.FreshInstallWizard("native")
			return

		case strings.HasPrefix(cleanOpt, "Run Deep System Diagnostics"):
			modules.RunDeepDiagnostics()

		case strings.HasPrefix(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Uninstall Joomla"):
			install.UninstallWizard()
			return
		}
	}
}

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	health := modules.DetectJoomlaHealth()

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
			envType = "docker"
		}
	}

	if envType == "docker" {
		composeDir := state["COMPOSE_DIR"]
		if composeDir == "" {
			composeDir = "/etc/joomla-docker"
		}
		dockerManagementMenu(composeDir, health)
	} else {
		nativeManagementMenu(health)
	}
}
