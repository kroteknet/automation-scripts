// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: Caddy 2 Web Server with Automatic HTTPS (caddy)
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

	"krotek-caddy-installer/core"
	"krotek-caddy-installer/install"
	"krotek-caddy-installer/modules"
	"krotek-caddy-installer/system"
	"krotek-caddy-installer/tui"
)

func noCaddyDiagnosticMenu() {
	for {
		tui.ClearScreen()
		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("Caddy Service", core.RED+"[NOT INSTALLED]"+core.NC, core.RED),
			tui.FormatKV("Caddy Binary Tool", system.GetToolBadge("caddy"), core.YELLOW),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + "Status: Caddy web server is not provisioned on this machine yet." + core.NC,
			"Choose an action below to deploy Caddy or run system network diagnostics.",
		}

		options := []string{
			"Install Caddy Docker (Containerized Stack - Recommended)",
			"Install Caddy Native (Clean Installation + Firewall)",
			"Run Deep System & Network Diagnostics Scan",
			"Exit Diagnostics",
		}

		idx, _, status := tui.SelectMenuWithContent("caddy-manager - System Diagnostics (Caddy Not Found)", diagLines, options)
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

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	modules.EnsureCaddyWorking()
	health := modules.DetectCaddyHealth()

	if !health.Installed {
		noCaddyDiagnosticMenu()
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
			composeDir = "/opt/caddy-docker"
		}
		dockerManagementMenu(composeDir, health)
	} else {
		nativeManagementMenu(health)
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.CaddyHealth) {
	mainConf := "/etc/caddy/Caddyfile"

	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart CADDY Service"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start CADDY Service  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		configLabel := fmt.Sprintf("Edit main config  (%s)", mainConf)
		if !health.SyntaxOK {
			configLabel = core.BOLD + core.RED + "Edit main config  ->  [SYNTAX ERROR DETECTED]" + core.NC
		}

		options := []string{
			"─── VIRTUAL HOSTS & DOMAINS ───",
			"View Application Dashboard & IP Info",
			"Add virtual host / domain",
			"List configured sites",
			"Edit virtual host / domain config",
			"Generate background app service (systemd)",
			"─── SERVICE MANAGEMENT ───",
			configLabel,
			"Reload CADDY Service",
			restartLabel,
			"Update Caddy Web Server & Check Upgrades (Safe Backup)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & System Journal",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System & Network Diagnostics",
			"Uninstall CADDY Server",
			"Exit",
		}

		titleStr := fmt.Sprintf("caddy-manager  -  Management (Native: %s)", statusBadge)
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
			dbLines = append(dbLines, tui.FormatKV("Application", "Caddy Web Server", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Local IP", system.GetLocalIP(), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Public IP", system.GetPublicIP(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("HTTP Port", "80", core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("HTTPS Port", "443 (Auto SSL/TLS)", core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("Admin API Port", "2019 (Localhost)", core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("Main Config", mainConf, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Site Configs Dir", "/etc/caddy/conf.d", core.CYAN))
			tui.PrintInBox("CADDY Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Add virtual host"):
			modules.DoAddDomain()

		case strings.HasPrefix(cleanOpt, "List configured sites"):
			tui.ClearScreen()
			var siteInfo []string
			siteInfo = append(siteInfo, core.BOLD+core.YELLOW+"CADDY CONFIGS (/etc/caddy/ & /etc/caddy/conf.d/):"+core.NC)
			siteInfo = append(siteInfo, fmt.Sprintf("  %s%s%s %s%-24s%s : %s[PRIMARY CADDYFILE]%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, "Caddyfile", core.NC, core.GREEN, core.NC))
			confCount := 0
			if files, err := filepath.Glob("/etc/caddy/conf.d/*.caddy"); err == nil {
				for _, f := range files {
					bname := filepath.Base(f)
					confCount++
					siteInfo = append(siteInfo, fmt.Sprintf("  %s%s%s %s%s%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, bname, core.NC))
				}
			}
			tui.PrintInBox("Configured CADDY Sites", siteInfo)

		case strings.HasPrefix(cleanOpt, "Edit virtual host"):
			tui.ClearScreen()
			searchDir := "/etc/caddy/conf.d"
			var siteFiles []string
			var siteLabels []string

			if files, err := filepath.Glob(filepath.Join(searchDir, "*.caddy")); err == nil {
				for _, f := range files {
					bname := filepath.Base(f)
					siteFiles = append(siteFiles, f)
					siteLabels = append(siteLabels, bname)
				}
			}

			if len(siteFiles) == 0 {
				tui.PrintInBox("No Config Files Found", "No site configuration files found in "+searchDir+".")
			} else {
				siteLabels = append(siteLabels, "---", "Cancel / Back to Menu")
				sIdx, _, sSt := tui.SelectMenu("caddy-manager - Select Config File", siteLabels)
				if sSt == 0 && sIdx < len(siteFiles) {
					selectedFile := siteFiles[sIdx]
					_ = tui.RunEditor(selectedFile)

					var resInfo []string
					if exec.Command("caddy", "validate", "--config", mainConf).Run() == nil {
						_ = exec.Command("systemctl", "reload", "caddy").Run()
						resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
						resInfo = append(resInfo, tui.FormatKV("Edited File", selectedFile, core.CYAN))
						resInfo = append(resInfo, tui.FormatKV("Caddy Service", "Syntax OK & Service Reloaded", core.GREEN))
					} else {
						resInfo = append(resInfo, tui.FormatKV("Operation Status", "FAILED", core.RED))
						resInfo = append(resInfo, tui.FormatKV("Edited File", selectedFile, core.CYAN))
						resInfo = append(resInfo, tui.FormatKV("Caddy Service", "Config Syntax Test Failed!", core.RED))
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
			if exec.Command("caddy", "validate", "--config", mainConf).Run() == nil {
				_ = exec.Command("systemctl", "reload", "caddy").Run()
				resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resInfo = append(resInfo, tui.FormatKV("Config File", mainConf, core.CYAN))
				resInfo = append(resInfo, tui.FormatKV("Caddy Service", "Syntax OK & Service Reloaded", core.GREEN))
			} else {
				resInfo = append(resInfo, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resInfo = append(resInfo, tui.FormatKV("Config File", mainConf, core.CYAN))
				resInfo = append(resInfo, tui.FormatKV("Caddy Service", "Main Caddyfile Syntax Test Failed!", core.RED))
			}
			tui.ClearScreen()
			tui.PrintInBox("Main Config Result", resInfo)

		case strings.HasPrefix(cleanOpt, "Reload CADDY"):
			tui.ClearScreen()
			modules.EnsureCaddyWorking()
			var resMsg []string
			if exec.Command("caddy", "validate", "--config", mainConf).Run() == nil && (exec.Command("systemctl", "reload", "caddy").Run() == nil || exec.Command("rc-service", "caddy", "reload").Run() == nil || exec.Command("caddy", "reload", "--config", mainConf).Run() == nil) {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("Caddy Service", "Reloaded successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resMsg = append(resMsg, tui.FormatKV("Caddy Service", "Reload failed! Run 'caddy validate'", core.RED))
			}
			tui.PrintInBox("Reload CADDY Status", resMsg)

		case strings.HasPrefix(cleanOpt, "Restart CADDY") || strings.HasPrefix(cleanOpt, "Start CADDY"):
			tui.ClearScreen()
			modules.EnsureCaddyWorking()
			var resMsg []string
			if exec.Command("systemctl", "restart", "caddy").Run() == nil || exec.Command("systemctl", "start", "caddy").Run() == nil || exec.Command("rc-service", "caddy", "restart").Run() == nil || exec.Command("rc-service", "caddy", "start").Run() == nil {
				health.IsRunning = true
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("Caddy Service", "Started/Restarted successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resMsg = append(resMsg, tui.FormatKV("Caddy Service", "Failed to start/restart!", core.RED))
			}
			tui.PrintInBox("Restart CADDY Status", resMsg)

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			health = modules.DetectCaddyHealth()
		case strings.HasPrefix(cleanOpt, "View Service Logs") || strings.HasPrefix(cleanOpt, "View Service"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallCaddy()
			return
		}
	}
}

// dockerManagementMenu provides container lifecycle controls (start/stop/restart/logs)
// for the containerized Caddy 2 Web Server with Automatic HTTPS Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.CaddyHealth) {
	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart CADDY Container"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start CADDY Container  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		options := []string{
			"─── DOCKER STACK MANAGEMENT ───",
			"Check Container Status (docker ps)",
			"View Application Dashboard & IP Info",
			fmt.Sprintf("Edit docker-compose.yml  (%s/docker-compose.yml)", composeDir),
			restartLabel,
			"View Container Logs (last 50 lines)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System & Network Diagnostics",
			"Uninstall CADDY Docker Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("caddy-manager  -  Management (Docker: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "Check Container Status"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "ps", "--filter", "name=caddy").CombinedOutput()
			tui.PrintInBox("Docker Container Status", tui.FormatDockerContainerStatus(string(out), "No running Docker containers found for caddy."))

		case strings.HasPrefix(cleanOpt, "View Application Dashboard"):
			tui.ClearScreen()
			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "Caddy Web Server (Docker)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Local IP", system.GetLocalIP(), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Public IP", system.GetPublicIP(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("HTTP Port", "80", core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("HTTPS Port", "443", core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
			tui.PrintInBox("CADDY Dashboard Info", dbLines)

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

		case strings.HasPrefix(cleanOpt, "Restart CADDY") || strings.HasPrefix(cleanOpt, "Start CADDY"):
			tui.ClearScreen()
			var resMsg []string
			cmdStr := fmt.Sprintf("cd %s && docker compose restart 2>/dev/null || docker-compose restart 2>/dev/null || docker restart krotek-caddy", composeDir)
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
			cmdStr := fmt.Sprintf("docker logs --tail 50 krotek-caddy 2>/dev/null || (cd %s && docker compose logs --tail 50 2>/dev/null)", composeDir)
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
			health = modules.DetectCaddyHealth()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallCaddy()
			return
		}
	}
}
