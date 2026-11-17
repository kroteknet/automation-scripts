// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: Plex Media Server Suite (plex)
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

	"krotek-plex-installer/core"
	"krotek-plex-installer/install"
	"krotek-plex-installer/modules"
	"krotek-plex-installer/system"
	"krotek-plex-installer/tui"
)

// noServiceDiagnosticMenu guides the user when Plex Media Server Suite is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		tui.ClearScreen()
		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("Plex Service", core.RED+"[NOT INSTALLED]"+core.NC, core.RED),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			tui.FormatKV("Plex Server Tool", system.GetToolBadge("plexmediaserver"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + "Status: Plex Media Server is not provisioned on this machine." + core.NC,
			"Choose a deployment method below to install Plex media streaming server.",
		}

		var options []string
		if core.SysInfo.Family == "alpine" {
			options = []string{
				"Install Plex Docker Stack (Official Container - Required for Alpine)",
				"Run Deep System & Network Diagnostics Scan",
				"Exit Diagnostics",
			}
		} else {
			options = []string{
				"Install Plex Docker Stack (Official Plex Media Server Container - Recommended)",
				"Install Plex Native Stack (plexmediaserver Linux Service)",
				"Run Deep System & Network Diagnostics Scan",
				"Exit Diagnostics",
			}
		}

		idx, _, status := tui.SelectMenuWithContent("plex-manager - System Diagnostics (Plex Not Found)", diagLines, options)
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
// for the containerized Plex Media Server Suite Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.PlexHealth) {
	for {
		health = modules.DetectPlexHealth()
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart Plex Docker Stack"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start Plex Stack  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		options := []string{
			"─── APPLICATION DASHBOARD ───",
			"View Application Dashboard & Access URLs",
			"Create Instant Configuration Backup",
			"─── STACK MANAGEMENT ───",
			fmt.Sprintf("Edit docker-compose.yml (%s/docker-compose.yml)", composeDir),
			restartLabel,
			"Update Plex Media Server & Check Upgrades (Safe Backup)",
			"View Container Logs (last 50 lines)",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Guided Setup / Reinstallation Wizard",
			"Run Deep System Diagnostics",
			"View Service Logs & System Journal",
			"Uninstall Plex Docker Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("plex-manager  -  Management (Docker: %s)", statusBadge)
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
				port = "32400"
			}

			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "Plex Media Server (Docker Stack)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Status", statusBadge, core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Web App Local URL", fmt.Sprintf("http://%s:%s/web", localIP, port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost URL", fmt.Sprintf("http://localhost:%s/web", port), core.GREEN))
			dbLines = append(dbLines, "")
			dbLines = append(dbLines, core.BOLD+core.YELLOW+"MEDIA & STORAGE PATHS:"+core.NC)
			dbLines = append(dbLines, tui.FormatKV("Media Directory", state["MEDIA_DIR"], core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Public Server IP", pubIP, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
			tui.PrintInBox("Plex Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Create Instant Configuration Backup"):
			tui.ClearScreen()
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile := fmt.Sprintf("/root/plex_instant_backup_%s.tar.gz", ts)
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

		case strings.HasPrefix(cleanOpt, "Restart Plex Docker Stack") || strings.HasPrefix(cleanOpt, "Start Plex Stack"):
			tui.ClearScreen()
			_ = system.RunDockerCompose(composeDir, "Restarting Plex Docker stack", false, "restart")
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

		case strings.HasPrefix(cleanOpt, "Uninstall Plex"):
			install.UninstallWizard()
			return
		}
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.PlexHealth) {
	for {
		health = modules.DetectPlexHealth()
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart Plex Service"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start Plex Service  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		options := []string{
			"─── APPLICATION DASHBOARD ───",
			"View Application Dashboard & Access URLs",
			"Create Instant Configuration Backup",
			"─── SERVICE MANAGEMENT ───",
			restartLabel,
			"Update Plex Media Server & Check Upgrades (Safe Backup)",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Guided Setup / Reinstallation Wizard",
			"Run Deep System Diagnostics",
			"View Service Logs & System Journal",
			"Uninstall Plex Native Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("plex-manager  -  Management (Native: %s)", statusBadge)
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
				port = "32400"
			}

			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "Plex Media Server (Native Linux)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Status", statusBadge, core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Web App Local URL", fmt.Sprintf("http://%s:%s/web", localIP, port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost URL", fmt.Sprintf("http://localhost:%s/web", port), core.GREEN))
			dbLines = append(dbLines, "")
			dbLines = append(dbLines, core.BOLD+core.YELLOW+"MEDIA & STORAGE PATHS:"+core.NC)
			dbLines = append(dbLines, tui.FormatKV("Media Directory", state["MEDIA_DIR"], core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Public Server IP", pubIP, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Config Directory", "/var/lib/plexmediaserver", core.CYAN))
			tui.PrintInBox("Plex Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Create Instant Configuration Backup"):
			tui.ClearScreen()
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile := fmt.Sprintf("/root/plex_instant_backup_%s.tar.gz", ts)
			if !system.IsSymlinkPath(bkpFile) {
				if _, sErr := os.Stat("/var/lib/plexmediaserver"); sErr == nil {
					_ = exec.Command("tar", "-czf", bkpFile, "/var/lib/plexmediaserver").Run()
					_ = os.Chmod(bkpFile, 0600)
				}
			}
			tui.PrintInBox("Backup Complete", []string{
				core.GREEN + "[SUCCESS] Configuration backup created successfully." + core.NC,
				tui.FormatKV("Archive File", bkpFile, core.CYAN),
			})

		case strings.HasPrefix(cleanOpt, "Restart Plex Service") || strings.HasPrefix(cleanOpt, "Start Plex Service"):
			tui.ClearScreen()
			system.RestartService("plexmediaserver")
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

		case strings.HasPrefix(cleanOpt, "Uninstall Plex"):
			install.UninstallWizard()
			return
		}
	}
}

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	health := modules.DetectPlexHealth()

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
			composeDir = "/etc/plex-docker"
		}
		dockerManagementMenu(composeDir, health)
	} else {
		nativeManagementMenu(health)
	}
}
