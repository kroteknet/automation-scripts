// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: Jellyfin Free Software Media System (jellyfin)
//
// This module provides the interactive terminal menus, status overview screens,
// operations actions (start, stop, reload), live log viewers, and uninstaller.
// ==============================================================================

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"krotek-jellyfin-installer/core"
	"krotek-jellyfin-installer/install"
	"krotek-jellyfin-installer/modules"
	"krotek-jellyfin-installer/system"
	"krotek-jellyfin-installer/tui"
)

// noServiceDiagnosticMenu guides the user when Jellyfin Free Software Media System is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		tui.ClearScreen()
		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("Jellyfin Service", core.RED+"[NOT INSTALLED]"+core.NC, core.RED),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			tui.FormatKV("Jellyfin CLI Tool", system.GetToolBadge("jellyfin"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + "Status: Jellyfin Media System is not provisioned on this machine." + core.NC,
			"Choose a deployment method below to install Jellyfin media streaming server.",
		}

		options := []string{
			"Install Jellyfin Docker Stack (Official Jellyfin Container)",
			"Install Jellyfin Native Stack (jellyfin Linux Service)",
			"Run Deep System & Network Diagnostics Scan",
			"Exit Diagnostics",
		}

		idx, _, status := tui.SelectMenuWithContent("jellyfin-manager - System Diagnostics (Jellyfin Not Found)", diagLines, options)
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
// for the containerized Jellyfin Free Software Media System Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.JellyfinHealth) {
	for {
		health = modules.DetectJellyfinHealth()
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart Jellyfin Docker Stack"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start Jellyfin Stack  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		options := []string{
			"─── APPLICATION DASHBOARD ───",
			"View Application Dashboard & Access URLs",
			"Create Instant Configuration Backup",
			"─── STACK MANAGEMENT ───",
			fmt.Sprintf("Edit docker-compose.yml (%s/docker-compose.yml)", composeDir),
			restartLabel,
			"Update Jellyfin Server & Check Upgrades (Safe Backup)",
			"View Container Logs (last 50 lines)",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Guided Setup / Reinstallation Wizard",
			"Run Deep System Diagnostics",
			"View Service Logs & System Journal",
			"Uninstall Jellyfin Docker Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("jellyfin-manager  -  Management (Docker: %s)", statusBadge)
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
				port = "8096"
			}

			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "Jellyfin Media Server (Docker Stack)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Status", statusBadge, core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
			if state["DOMAIN_NAME"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Domain URL", fmt.Sprintf("http://%s:%s", state["DOMAIN_NAME"], port), core.YELLOW))
			}
			dbLines = append(dbLines, "")
			dbLines = append(dbLines, core.BOLD+core.YELLOW+"MEDIA & NETWORK PATHS:"+core.NC)
			dbLines = append(dbLines, tui.FormatKV("Media Directory", state["MEDIA_DIR"], core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("DLNA Discovery Port", "1900/udp", core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Client Discovery Port", "7359/udp", core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Public Server IP", pubIP, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
			tui.PrintInBox("Jellyfin Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Create Instant Configuration Backup"):
			tui.ClearScreen()
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile := fmt.Sprintf("/root/jellyfin_instant_backup_%s.tar.gz", ts)
			if !system.IsSymlinkPath(bkpFile) {
				if _, sErr := os.Stat(composeDir); sErr == nil {
					_ = exec.Command("tar", "-czf", bkpFile, composeDir).Run()
					_ = os.Chmod(bkpFile, 0600)
				}
			}
			tui.PrintInBox("Backup Complete", []string{
				core.GREEN + "[SUCCESS] Configuration backup created successfully." + core.NC,
				tui.FormatKV("Archive File", bkpFile, core.CYAN),
			})

		case strings.HasPrefix(cleanOpt, "Edit docker-compose.yml"):
			tui.ClearScreen()
			_ = tui.RunEditor(composeDir + "/docker-compose.yml")

		case strings.HasPrefix(cleanOpt, "Restart Jellyfin Docker Stack") || strings.HasPrefix(cleanOpt, "Start Jellyfin Stack"):
			tui.ClearScreen()
			_ = system.RunDockerCompose(composeDir, "Restarting Jellyfin Docker stack", false, "restart")
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

		case strings.HasPrefix(cleanOpt, "Uninstall Jellyfin"):
			install.UninstallWizard()
			return
		}
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.JellyfinHealth) {
	for {
		health = modules.DetectJellyfinHealth()
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart Jellyfin Service"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start Jellyfin Service  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		options := []string{
			"─── APPLICATION DASHBOARD ───",
			"View Application Dashboard & Access URLs",
			"Create Instant Configuration Backup",
			"─── SERVICE MANAGEMENT ───",
			restartLabel,
			"Update Jellyfin Server & Check Upgrades (Safe Backup)",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Guided Setup / Reinstallation Wizard",
			"Run Deep System Diagnostics",
			"View Service Logs & System Journal",
			"Uninstall Jellyfin Native Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("jellyfin-manager  -  Management (Native: %s)", statusBadge)
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
				port = "8096"
			}

			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "Jellyfin Media Server (Native Linux)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Status", statusBadge, core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
			if state["DOMAIN_NAME"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Domain URL", fmt.Sprintf("http://%s:%s", state["DOMAIN_NAME"], port), core.YELLOW))
			}
			dbLines = append(dbLines, "")
			dbLines = append(dbLines, core.BOLD+core.YELLOW+"MEDIA & NETWORK PATHS:"+core.NC)
			dbLines = append(dbLines, tui.FormatKV("Media Directory", state["MEDIA_DIR"], core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Public Server IP", pubIP, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Config Directory", "/etc/jellyfin", core.CYAN))
			tui.PrintInBox("Jellyfin Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Create Instant Configuration Backup"):
			tui.ClearScreen()
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile := fmt.Sprintf("/root/jellyfin_instant_backup_%s.tar.gz", ts)
			if !system.IsSymlinkPath(bkpFile) {
				if _, sErr := os.Stat("/etc/jellyfin"); sErr == nil {
					_ = exec.Command("tar", "-czf", bkpFile, "/etc/jellyfin").Run()
					_ = os.Chmod(bkpFile, 0600)
				}
			}
			tui.PrintInBox("Backup Complete", []string{
				core.GREEN + "[SUCCESS] Configuration backup created successfully." + core.NC,
				tui.FormatKV("Archive File", bkpFile, core.CYAN),
			})

		case strings.HasPrefix(cleanOpt, "Restart Jellyfin Service") || strings.HasPrefix(cleanOpt, "Start Jellyfin Service"):
			tui.ClearScreen()
			system.RestartService("jellyfin")
			time.Sleep(1 * time.Second)

		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.Contains(cleanOpt, "Guided Setup"):
			install.FreshInstallWizard("native")
			return

		case strings.HasPrefix(cleanOpt, "Run Deep System Diagnostics"):
			modules.RunDeepDiagnostics()

		case strings.HasPrefix(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Uninstall Jellyfin"):
			install.UninstallWizard()
			return
		}
	}
}

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	health := modules.DetectJellyfinHealth()

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
			composeDir = "/etc/jellyfin-docker"
		}
		dockerManagementMenu(composeDir, health)
	} else {
		nativeManagementMenu(health)
	}
}
