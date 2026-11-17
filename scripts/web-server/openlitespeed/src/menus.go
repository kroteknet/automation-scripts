// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: OpenLiteSpeed High-Performance Web Server (openlitespeed)
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
	"regexp"
	"strings"

	"krotek-openlitespeed-installer/core"
	"krotek-openlitespeed-installer/install"
	"krotek-openlitespeed-installer/modules"
	"krotek-openlitespeed-installer/system"
	"krotek-openlitespeed-installer/tui"
)

func noOpenlitespeedDiagnosticMenu() {
	for {
		tui.ClearScreen()
		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("OpenLiteSpeed Service", core.RED+"[NOT INSTALLED]"+core.NC, core.RED),
			tui.FormatKV("OLS lsws Tool", system.GetToolBadge("/usr/local/lsws/bin/lswsctrl"), core.YELLOW),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + "Status: OpenLiteSpeed web server is not provisioned on this machine yet." + core.NC,
			"Choose an action below to deploy OpenLiteSpeed or run system network diagnostics.",
		}

		var options []string
		if core.SysInfo.Family == "alpine" {
			options = []string{
				"Install OpenLiteSpeed (Docker Container Stack - Required for Alpine)",
				"Run Deep System & Network Diagnostics Scan",
				"Exit Diagnostics",
			}
		} else {
			options = []string{
				"Install OpenLiteSpeed Native (Clean Installation + Firewall)",
				"Install OpenLiteSpeed Docker (Containerized Stack)",
				"Run Deep System & Network Diagnostics Scan",
				"Exit Diagnostics",
			}
		}

		idx, _, status := tui.SelectMenuWithContent("openlitespeed-manager - System Diagnostics (OLS Not Found)", diagLines, options)
		if status == 2 || (core.SysInfo.Family == "alpine" && (status == 0 && idx == 2)) || (core.SysInfo.Family != "alpine" && (status == 0 && idx == 3)) {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		if core.SysInfo.Family == "alpine" {
			switch idx {
			case 0:
				install.FreshInstallWizard("docker")
				return
			case 1:
				modules.RunDeepDiagnostics()
			}
		} else {
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
}

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	modules.EnsureOpenLiteSpeedWorking()
	health := modules.DetectOpenlitespeedHealth()

	if !health.Installed {
		noOpenlitespeedDiagnosticMenu()
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
		composeDir := state["COMPOSE_DIR"]
		if composeDir == "" {
			composeDir = "/opt/openlitespeed-docker"
		}
		dockerManagementMenu(composeDir, health)
	} else {
		nativeManagementMenu(health)
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.OpenlitespeedHealth) {
	mainConf := "/usr/local/lsws/conf/httpd_config.conf"
	if _, err := os.Stat(mainConf); os.IsNotExist(err) {
		mainConf = "/etc/openlitespeed/openlitespeed.conf"
	}

	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart OPENLITESPEED Service"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start OPENLITESPEED Service  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		configLabel := fmt.Sprintf("Edit main config  (%s)", mainConf)
		if !health.SyntaxOK {
			configLabel = core.BOLD + core.RED + "Edit main config  ->  [SYNTAX ERROR DETECTED]" + core.NC
		}

		options := []string{
			"─── VIRTUAL HOSTS & DOMAINS ───",
			"View Application Dashboard & IP Info",
			"View Saved Credentials & Secrets",
			"Add virtual host / domain",
			"List configured sites",
			"Edit virtual host / domain config",
			"Generate background app service (systemd)",
			"─── SERVICE MANAGEMENT ───",
			configLabel,
			"Reload OPENLITESPEED Service",
			restartLabel,
			"Update OpenLiteSpeed Web Server & Check Upgrades (Safe Backup)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & System Journal",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System & Network Diagnostics",
			"Uninstall OPENLITESPEED Server",
			"Exit",
		}

		titleStr := fmt.Sprintf("openlitespeed-manager  -  Management (Native: %s)", statusBadge)
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
			dbLines = append(dbLines, tui.FormatKV("Application", "OpenLiteSpeed Web Server", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Local IP", system.GetLocalIP(), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Public IP", system.GetPublicIP(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Web Console URL", "https://"+system.GetLocalIP()+":7080", core.YELLOW+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("HTTP Port", "8080 / 80", core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("Admin Console Port", "7080", core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("Main Config", mainConf, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("VHosts Dir", "/usr/local/lsws/conf/vhosts", core.CYAN))
			tui.PrintInBox("OPENLITESPEED Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "View Saved Credentials"):
			if !modules.AuthenticateAdminAccess() {
				tui.ClearScreen()
				var failLines []string
				failLines = append(failLines, tui.FormatKV("Access Control", "SECURITY AUTHENTICATION FAILED", core.RED+core.BOLD))
				failLines = append(failLines, tui.FormatKV("Reason", "Incorrect root/sudo password or request cancelled", core.YELLOW))
				failLines = append(failLines, tui.FormatKV("Protection", "Secrets remain locked & protected against unauthorized access", core.GREEN))
				tui.PrintInBox("SECURITY AUTHORIZATION DENIED", failLines)
				break
			}
			tui.ClearScreen()
			state := system.LoadState()
			var credLines []string
			u := state["ADMIN_USER"]
			if u == "" {
				u = "admin"
			}
			p := state["ADMIN_PASS"]
			if p == "" {
				p = "[Default / Unset]"
			}
			credLines = append(credLines, tui.FormatKV("WebAdmin Username", u, core.YELLOW+core.BOLD))
			credLines = append(credLines, tui.FormatKV("WebAdmin Password", p, core.GREEN+core.BOLD))
			credLines = append(credLines, tui.FormatKV("WebAdmin Console URL", "https://"+system.GetLocalIP()+":7080", core.PURPLE+core.BOLD))
			credLines = append(credLines, tui.FormatKV("HTTP Test Port", "http://"+system.GetLocalIP()+":8088", core.CYAN))
			tui.PrintInBox("OpenLiteSpeed Saved Credentials & Access Info", credLines)

		case strings.HasPrefix(cleanOpt, "Add virtual host"):
			modules.DoAddDomain()

		case strings.HasPrefix(cleanOpt, "List configured sites"):
			tui.ClearScreen()
			var siteInfo []string
			siteInfo = append(siteInfo, core.BOLD+core.YELLOW+"OPENLITESPEED VHOSTS (/usr/local/lsws/conf/vhosts/):"+core.NC)
			siteInfo = append(siteInfo, fmt.Sprintf("  %s%s%s %s%-24s%s : %s[PRIMARY VHOST]%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, "Example", core.NC, core.GREEN, core.NC))
			confCount := 0
			if files, err := filepath.Glob("/usr/local/lsws/conf/vhosts/*"); err == nil {
				for _, f := range files {
					bname := filepath.Base(f)
					if bname == "Example" {
						continue
					}
					confCount++
					siteInfo = append(siteInfo, fmt.Sprintf("  %s%s%s %s%s%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, bname, core.NC))
				}
			}
			tui.PrintInBox("Configured OPENLITESPEED Sites", siteInfo)

		case strings.HasPrefix(cleanOpt, "Edit virtual host"):
			tui.ClearScreen()
			searchDir := "/usr/local/lsws/conf/vhosts"
			var siteFiles []string
			var siteLabels []string

			if files, err := filepath.Glob(filepath.Join(searchDir, "*")); err == nil {
				for _, f := range files {
					confPath := filepath.Join(f, "vhconf.conf")
					if _, err := os.Stat(confPath); err == nil {
						bname := filepath.Base(f)
						siteFiles = append(siteFiles, confPath)
						siteLabels = append(siteLabels, bname+"  (vhconf.conf)")
					}
				}
			}

			if len(siteFiles) == 0 {
				tui.PrintInBox("No Config Files Found", "No virtual host configuration files found in "+searchDir+".")
			} else {
				siteLabels = append(siteLabels, "---", "Cancel / Back to Menu")
				sIdx, _, sSt := tui.SelectMenu("openlitespeed-manager - Select Config File", siteLabels)
				if sSt == 0 && sIdx < len(siteFiles) {
					selectedFile := siteFiles[sIdx]
					_ = tui.RunEditor(selectedFile)

					var resInfo []string
					if exec.Command("/usr/local/lsws/bin/lswsctrl", "restart").Run() == nil || exec.Command("systemctl", "reload", "openlitespeed").Run() == nil {
						resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
						resInfo = append(resInfo, tui.FormatKV("Edited File", selectedFile, core.CYAN))
						resInfo = append(resInfo, tui.FormatKV("OpenLiteSpeed Service", "Syntax OK & Service Reloaded", core.GREEN))
					} else {
						resInfo = append(resInfo, tui.FormatKV("Operation Status", "FAILED", core.RED))
						resInfo = append(resInfo, tui.FormatKV("Edited File", selectedFile, core.CYAN))
						resInfo = append(resInfo, tui.FormatKV("OpenLiteSpeed Service", "Config Syntax Test Failed!", core.RED))
					}
					tui.ClearScreen()
					tui.PrintInBox("Edit Config Result", resInfo)
				}
			}

		case strings.HasPrefix(cleanOpt, "Generate background app service"):
			modules.GenerateSystemdServiceWizard()

		case strings.HasPrefix(cleanOpt, "Edit main config"):
			tui.ClearScreen()
			_ = tui.RunEditor(mainConf)

			var resInfo []string
			if exec.Command("/usr/local/lsws/bin/lswsctrl", "restart").Run() == nil || exec.Command("systemctl", "reload", "openlitespeed").Run() == nil {
				resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resInfo = append(resInfo, tui.FormatKV("Config File", mainConf, core.CYAN))
				resInfo = append(resInfo, tui.FormatKV("OpenLiteSpeed Service", "Syntax OK & Service Reloaded", core.GREEN))
			} else {
				resInfo = append(resInfo, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resInfo = append(resInfo, tui.FormatKV("Config File", mainConf, core.CYAN))
				resInfo = append(resInfo, tui.FormatKV("OpenLiteSpeed Service", "Main Config Syntax Test Failed!", core.RED))
			}
			tui.ClearScreen()
			tui.PrintInBox("Main Config Result", resInfo)

		case strings.HasPrefix(cleanOpt, "Reload OPENLITESPEED"):
			tui.ClearScreen()
			modules.EnsureOpenLiteSpeedWorking()
			var resMsg []string
			if exec.Command("/usr/local/lsws/bin/lswsctrl", "restart").Run() == nil || exec.Command("systemctl", "reload", "openlitespeed").Run() == nil {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("OpenLiteSpeed Service", "Reloaded successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resMsg = append(resMsg, tui.FormatKV("OpenLiteSpeed Service", "Reload failed!", core.RED))
			}
			tui.PrintInBox("Reload OPENLITESPEED Status", resMsg)

		case strings.HasPrefix(cleanOpt, "Restart OPENLITESPEED") || strings.HasPrefix(cleanOpt, "Start OPENLITESPEED"):
			tui.ClearScreen()
			modules.EnsureOpenLiteSpeedWorking()
			var resMsg []string
			if exec.Command("/usr/local/lsws/bin/lswsctrl", "restart").Run() == nil || exec.Command("systemctl", "restart", "lsws").Run() == nil || exec.Command("systemctl", "restart", "openlitespeed").Run() == nil {
				health.IsRunning = true
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("OpenLiteSpeed Service", "Started/Restarted successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resMsg = append(resMsg, tui.FormatKV("OpenLiteSpeed Service", "Failed to start/restart!", core.RED))
			}
			tui.PrintInBox("Restart OPENLITESPEED Status", resMsg)

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			health = modules.DetectOpenlitespeedHealth()
		case strings.HasPrefix(cleanOpt, "View Service Logs") || strings.HasPrefix(cleanOpt, "View Service"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallOpenlitespeed()
			return
		}
	}
}

// dockerManagementMenu provides container lifecycle controls (start/stop/restart/logs)
// for the containerized OpenLiteSpeed High-Performance Web Server Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.OpenlitespeedHealth) {
	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart OPENLITESPEED Container"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start OPENLITESPEED Container  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		options := []string{
			"─── DOCKER STACK MANAGEMENT ───",
			"Check Container Status (docker ps)",
			"View Application Dashboard & IP Info",
			"View Saved Credentials & Secrets",
			fmt.Sprintf("Edit docker-compose.yml  (%s/docker-compose.yml)", composeDir),
			restartLabel,
			"View Container Logs (last 50 lines)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System & Network Diagnostics",
			"Uninstall OPENLITESPEED Docker Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("openlitespeed-manager  -  Management (Docker: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "Check Container Status"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "ps", "--filter", "name=openlitespeed").CombinedOutput()
			tui.PrintInBox("Docker Container Status", tui.FormatDockerContainerStatus(string(out), "No running Docker containers found for openlitespeed."))

		case strings.HasPrefix(cleanOpt, "View Application Dashboard"):
			tui.ClearScreen()
			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "OpenLiteSpeed (Docker Stack)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Local IP", system.GetLocalIP(), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Public IP", system.GetPublicIP(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Web Console URL", "https://"+system.GetLocalIP()+":7080", core.YELLOW+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
			tui.PrintInBox("OPENLITESPEED Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "View Saved Credentials"):
			if !modules.AuthenticateAdminAccess() {
				tui.ClearScreen()
				var failLines []string
				failLines = append(failLines, tui.FormatKV("Access Control", "SECURITY AUTHENTICATION FAILED", core.RED+core.BOLD))
				failLines = append(failLines, tui.FormatKV("Reason", "Incorrect root/sudo password or request cancelled", core.YELLOW))
				failLines = append(failLines, tui.FormatKV("Protection", "Secrets remain locked & protected against unauthorized access", core.GREEN))
				tui.PrintInBox("SECURITY AUTHORIZATION DENIED", failLines)
				break
			}
			tui.ClearScreen()
			state := system.LoadState()
			var credLines []string
			u := state["ADMIN_USER"]
			if u == "" {
				u = "admin"
			}
			p := state["ADMIN_PASS"]
			if p == "" {
				p = "[Default / Unset]"
			}
			credLines = append(credLines, tui.FormatKV("WebAdmin Username", u, core.YELLOW+core.BOLD))
			credLines = append(credLines, tui.FormatKV("WebAdmin Password", p, core.GREEN+core.BOLD))
			credLines = append(credLines, tui.FormatKV("WebAdmin Console URL", "https://"+system.GetLocalIP()+":7080", core.PURPLE+core.BOLD))
			credLines = append(credLines, tui.FormatKV("HTTP Test Port", "http://"+system.GetLocalIP()+":8088", core.CYAN))
			tui.PrintInBox("OpenLiteSpeed Saved Credentials & Access Info", credLines)

		case strings.HasPrefix(cleanOpt, "Edit docker-compose.yml"):
			tui.ClearScreen()
			composeFile := filepath.Join(composeDir, "docker-compose.yml")
			_ = tui.RunEditor(composeFile)

			tui.ClearScreen()
			res := tui.AskYesNo("Recreate and restart Docker Compose container stack (docker compose up -d)?")
			var resInfo []string
			if res == 0 {
				cmdStr := fmt.Sprintf("cd %s && docker compose up -d 2>/dev/null || docker-compose up -d", composeDir)
				if system.RunMultiStep("Recreating Docker stack", cmdStr) {
					resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
					resInfo = append(resInfo, tui.FormatKV("Compose File", composeFile, core.CYAN))
					resInfo = append(resInfo, tui.FormatKV("Docker Container", "Recreated & Running", core.GREEN))
				} else {
					resInfo = append(resInfo, tui.FormatKV("Operation Status", "FAILED", core.RED))
					resInfo = append(resInfo, tui.FormatKV("Compose File", composeFile, core.CYAN))
				}
			} else {
				resInfo = append(resInfo, tui.FormatKV("Operation Status", "SAVED", core.GREEN))
				resInfo = append(resInfo, tui.FormatKV("Compose File", composeFile, core.CYAN))
			}
			tui.ClearScreen()
			tui.PrintInBox("Docker Compose Edit Result", resInfo)

		case strings.HasPrefix(cleanOpt, "Restart OPENLITESPEED") || strings.HasPrefix(cleanOpt, "Start OPENLITESPEED"):
			tui.ClearScreen()
			var resMsg []string
			cmdStr := fmt.Sprintf("cd %s && docker compose restart 2>/dev/null || docker-compose restart 2>/dev/null || docker restart krotek-openlitespeed", composeDir)
			if system.RunMultiStep("Restarting Docker stack", cmdStr) {
				health.IsRunning = true
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("Docker Stack", "Restarted successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
			}
			tui.PrintInBox("Restart Docker Stack Result", resMsg)

		case strings.HasPrefix(cleanOpt, "View Container Logs"):
			tui.ClearScreen()
			var logLines []string
			cmdStr := fmt.Sprintf("docker logs --tail 50 krotek-openlitespeed 2>/dev/null || (cd %s && docker compose logs --tail 50 2>/dev/null)", composeDir)
			if out, err := exec.Command(system.GetShell(), "-c", cmdStr).CombinedOutput(); err == nil && len(out) > 0 {
				raw := strings.Split(string(out), "\n")
				code2xx := regexp.MustCompile(`" 2[0-9]{2} `)
				codeErr := regexp.MustCompile(`" [4-5][0-9]{2} `)
				for _, line := range raw {
					if strings.TrimSpace(line) != "" {
						line = code2xx.ReplaceAllString(line, fmt.Sprintf("\" %s$0%s ", core.GREEN, core.NC))
						line = codeErr.ReplaceAllString(line, fmt.Sprintf("\" %s$0%s ", core.RED, core.NC))
						logLines = append(logLines, line)
					}
				}
			}
			if len(logLines) == 0 {
				logLines = append(logLines, "No container logs found.")
			}
			if len(logLines) > 15 {
				logLines = logLines[len(logLines)-15:]
			}
			tui.PrintInBox("Docker Container Logs (Last 50 lines)", logLines)

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()

		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("docker")
			health = modules.DetectOpenlitespeedHealth()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallOpenlitespeed()
			return
		}
	}
}
