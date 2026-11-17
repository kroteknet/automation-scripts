// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: Nextcloud Enterprise Cloud Collaboration (nextcloud)
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

	"krotek-nextcloud-installer/core"
	"krotek-nextcloud-installer/install"
	"krotek-nextcloud-installer/modules"
	"krotek-nextcloud-installer/system"
	"krotek-nextcloud-installer/tui"
)

// noServiceDiagnosticMenu guides the user when Nextcloud Enterprise Cloud Collaboration is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		tui.ClearScreen()
		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("Nextcloud Service", core.RED+"[NOT INSTALLED]"+core.NC, core.RED),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			tui.FormatKV("MariaDB Tool", system.GetToolBadge("mariadb"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + "Status: Nextcloud Hub is not provisioned on this machine." + core.NC,
			"Choose a deployment method below to install Nextcloud with automated MariaDB.",
		}

		options := []string{
			"Install Nextcloud Docker Stack (Nextcloud + MariaDB Containerized)",
			"Install Nextcloud Native Stack (Choose NGINX or Apache)",
			"Run Deep System & Network Diagnostics Scan",
			"Exit Diagnostics",
		}

		idx, _, status := tui.SelectMenuWithContent("nextcloud-manager - System Diagnostics (Nextcloud Not Found)", diagLines, options)
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
// for the containerized Nextcloud Enterprise Cloud Collaboration Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.NextcloudHealth) {
	for {
		health = modules.DetectNextcloudHealth()
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart Nextcloud Docker Stack"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start Nextcloud Stack  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		options := []string{
			"─── APPLICATION DASHBOARD ───",
			"View Application Dashboard & Access URLs",
			"Create Instant Backup (Files + Database Dump)",
			"─── STACK MANAGEMENT ───",
			fmt.Sprintf("Edit docker-compose.yml (%s/docker-compose.yml)", composeDir),
			restartLabel,
			"Update Nextcloud Hub & Check Upgrades (Safe Backup)",
			"View Container Logs (last 50 lines)",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Guided Setup / Reinstallation Wizard",
			"Run Deep System Diagnostics",
			"View Service Logs & System Journal",
			"Uninstall Nextcloud Docker Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("nextcloud-manager  -  Management (Docker: %s)", statusBadge)
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
			dbLines = append(dbLines, tui.FormatKV("Application", "Nextcloud Hub (Docker Stack)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Status", statusBadge, core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
			if state["DOMAIN_NAME"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Domain URL", fmt.Sprintf("http://%s:%s", state["DOMAIN_NAME"], port), core.YELLOW))
			}
			dbLines = append(dbLines, "")
			dbLines = append(dbLines, core.BOLD+core.YELLOW+"ADMIN & DATABASE CREDENTIALS:"+core.NC)
			dbLines = append(dbLines, tui.FormatKV("Admin Username", state["ADMIN_USER"], core.YELLOW+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Admin Password", state["ADMIN_PASS"], core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Database Host", "db (Container) / localhost", core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Database Name", state["DB_NAME"], core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Database User", state["DB_USER"], core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Database Password", state["DB_PASS"], core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Public Server IP", pubIP, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
			tui.PrintInBox("Nextcloud Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Create Instant Backup"):
			tui.ClearScreen()
			state := system.LoadState()
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile := fmt.Sprintf("/root/nextcloud_instant_backup_%s.tar.gz", ts)
			cmd := exec.Command("docker", "compose", "--project-directory", composeDir, "exec", "-T", "-e", "MYSQL_PWD", "db", "mysqldump", "-u", state["DB_USER"], state["DB_NAME"])
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
			tui.PrintInBox("Backup Complete", []string{
				core.GREEN + "[SUCCESS] Full backup created successfully." + core.NC,
				tui.FormatKV("Archive File", bkpFile, core.CYAN),
			})

		case strings.HasPrefix(cleanOpt, "Edit docker-compose.yml"):
			tui.ClearScreen()
			_ = tui.RunEditor(composeDir + "/docker-compose.yml")

		case strings.HasPrefix(cleanOpt, "Restart Nextcloud Docker Stack") || strings.HasPrefix(cleanOpt, "Start Nextcloud Stack"):
			tui.ClearScreen()
			_ = system.RunDockerCompose(composeDir, "Restarting Nextcloud Docker stack", false, "restart")
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

		case strings.HasPrefix(cleanOpt, "Uninstall Nextcloud"):
			install.UninstallWizard()
			return
		}
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.NextcloudHealth) {
	for {
		health = modules.DetectNextcloudHealth()
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		state := system.LoadState()
		webserver := state["WEBSERVER"]
		if webserver == "" {
			if exec.Command("systemctl", "is-active", "--quiet", "apache2").Run() == nil || exec.Command("rc-service", "apache2", "status").Run() == nil || exec.Command("pidof", "apache2").Run() == nil || exec.Command("pidof", "httpd").Run() == nil {
				webserver = "apache"
			} else {
				webserver = "nginx"
			}
		}

		wsTitle := "NGINX"
		stackLabel := "LEMP"
		if webserver == "apache" {
			wsTitle = "Apache"
			stackLabel = "LAMP"
		}

		restartLabel := fmt.Sprintf("Restart %s Services (%s + PHP-FPM + MariaDB)", stackLabel, wsTitle)
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + fmt.Sprintf("Start %s Services (%s)  ->  [STOPPED - Click to Start]", stackLabel, wsTitle) + core.NC
		}

		reloadLabel := fmt.Sprintf("Reload %s Configuration", wsTitle)

		options := []string{
			"─── APPLICATION DASHBOARD ───",
			"View Application Dashboard & Access URLs",
			"Create Instant Backup (Files + Database Dump)",
			"─── SERVICE MANAGEMENT ───",
			restartLabel,
			reloadLabel,
			"Update Nextcloud Hub & Check Upgrades (Safe Backup)",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Guided Setup / Reinstallation Wizard",
			"Run Deep System Diagnostics",
			"View Service Logs & System Journal",
			"Uninstall Nextcloud Native Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("nextcloud-manager  -  Management (Native: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "View Application Dashboard"):
			tui.ClearScreen()
			localIP := system.GetLocalIP()
			pubIP := system.GetPublicIP()
			port := state["PORT"]
			if port == "" {
				port = "80"
			}

			appTitle := "Nextcloud Hub (Native LEMP - NGINX)"
			if webserver == "apache" {
				appTitle = "Nextcloud Hub (Native LAMP - Apache)"
			}

			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", appTitle, core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Web Server", wsTitle, core.CYAN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Status", statusBadge, core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
			if state["DOMAIN_NAME"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Domain URL", fmt.Sprintf("http://%s:%s", state["DOMAIN_NAME"], port), core.YELLOW))
			}
			dbLines = append(dbLines, "")
			dbLines = append(dbLines, core.BOLD+core.YELLOW+"DATABASE & CREDENTIALS:"+core.NC)
			dbLines = append(dbLines, tui.FormatKV("Database Name", state["DB_NAME"], core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Database User", state["DB_USER"], core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Database Password", state["DB_PASS"], core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Admin Username", state["ADMIN_USER"], core.YELLOW+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Public Server IP", pubIP, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Document Root", "/var/www/nextcloud", core.CYAN))
			tui.PrintInBox("Nextcloud Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Create Instant Backup"):
			tui.ClearScreen()
			dbName := state["DB_NAME"]
			if dbName == "" {
				dbName = "nextcloud"
			}
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile := fmt.Sprintf("/root/nextcloud_instant_backup_%s.tar.gz", ts)
			tmpDir, tErr := os.MkdirTemp("", "krotek-nc-inst-*")
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
			tui.PrintInBox("Backup Complete", []string{
				core.GREEN + "[SUCCESS] Full backup created successfully." + core.NC,
				tui.FormatKV("Archive File", bkpFile, core.CYAN),
			})

		case strings.HasPrefix(cleanOpt, "Restart LEMP Services") || strings.HasPrefix(cleanOpt, "Start LEMP Services") || strings.HasPrefix(cleanOpt, "Restart LAMP Services") || strings.HasPrefix(cleanOpt, "Start LAMP Services") || strings.Contains(cleanOpt, "Services ("):
			tui.ClearScreen()
			if webserver == "apache" {
				apacheSvc := "apache2"
				if core.SysInfo.Family == "rhel" {
					apacheSvc = "httpd"
				}
				_ = system.RunMultiStep("Restarting LAMP services", fmt.Sprintf("rc-service %s restart 2>/dev/null || systemctl restart %s 2>/dev/null || true; rc-service php-fpm restart 2>/dev/null || systemctl restart php-fpm 2>/dev/null || true; rc-service mariadb restart 2>/dev/null || systemctl restart mariadb 2>/dev/null || true", apacheSvc, apacheSvc))
			} else {
				_ = system.RunMultiStep("Restarting LEMP services", "rc-service nginx restart 2>/dev/null || systemctl restart nginx 2>/dev/null || true; rc-service php-fpm restart 2>/dev/null || systemctl restart php-fpm 2>/dev/null || true; rc-service mariadb restart 2>/dev/null || systemctl restart mariadb 2>/dev/null || true")
			}
			time.Sleep(1 * time.Second)

		case strings.HasPrefix(cleanOpt, "Reload"):
			tui.ClearScreen()
			if webserver == "apache" {
				apacheSvc := "apache2"
				if core.SysInfo.Family == "rhel" {
					apacheSvc = "httpd"
				}
				system.ReloadService(apacheSvc)
			} else {
				system.ReloadService("nginx")
			}

		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.Contains(cleanOpt, "Guided Setup"):
			install.FreshInstallWizard("native")
			return

		case strings.HasPrefix(cleanOpt, "Run Deep System Diagnostics"):
			modules.RunDeepDiagnostics()

		case strings.HasPrefix(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Uninstall Nextcloud"):
			install.UninstallWizard()
			return
		}
	}
}

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	health := modules.DetectNextcloudHealth()

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
			composeDir = "/etc/nextcloud-docker"
		}
		dockerManagementMenu(composeDir, health)
	} else {
		nativeManagementMenu(health)
	}
}
