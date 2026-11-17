// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: WireGuard Fast Kernel VPN Tunnel (wireguard)
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

	"krotek-wireguard-installer/core"
	"krotek-wireguard-installer/install"
	"krotek-wireguard-installer/modules"
	"krotek-wireguard-installer/system"
	"krotek-wireguard-installer/tui"
)

func getServiceStatusBadge() string {
	cmd := exec.Command("systemctl", "is-active", "wg-quick@wg0")
	if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) == "active" {
		return core.GREEN + "[ACTIVE]" + core.NC
	}
	cmd2 := exec.Command("wg", "show", "wg0")
	if out, err := cmd2.Output(); err == nil && len(strings.TrimSpace(string(out))) > 0 {
		return core.GREEN + "[ACTIVE]" + core.NC
	}
	return core.RED + "[STOPPED]" + core.NC
}

func getDockerStatusBadge(composeDir string) string {
	cmd := exec.Command("docker", "compose", "--project-directory", composeDir, "ps", "--status", "running", "-q")
	if out, err := cmd.Output(); err == nil && len(strings.TrimSpace(string(out))) > 0 {
		return core.GREEN + "[ACTIVE]" + core.NC
	}
	return core.RED + "[STOPPED]" + core.NC
}

func createInstantBackup(state map[string]string) {
	tui.ClearScreen()
	timestamp := time.Now().Format("20060102_150405")
	backupFile := fmt.Sprintf("/root/wireguard_backup_%s.tar.gz", timestamp)

	var err error
	if state["ENV_TYPE"] == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd == "" {
			cd = "/etc/wireguard-docker"
		}
		err = exec.Command("tar", "-czf", backupFile, cd).Run()
	} else {
		if _, statErr := os.Stat("/etc/wireguard"); statErr == nil {
			err = exec.Command("tar", "-czf", backupFile, "/etc/wireguard").Run()
		} else {
			err = fmt.Errorf("/etc/wireguard directory does not exist")
		}
	}

	if err == nil {
		tui.PrintInBox("Instant Backup Success", []string{
			core.GREEN + "[SUCCESS] WireGuard backup archive generated successfully!" + core.NC,
			fmt.Sprintf("Archive: %s%s%s", core.BOLD, backupFile, core.NC),
		})
	} else {
		tui.PrintInBox("Backup Failed", []string{
			core.RED + "[ERROR] Failed to generate backup: " + err.Error() + core.NC,
		})
	}
}

func noWireguardDiagnosticMenu() {
	for {
		tui.ClearScreen()
		diag := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("WireGuard VPN Service", core.RED+"[NOT INSTALLED]"+core.NC, core.RED),
			tui.FormatKV("WireGuard VPN Binary Tool", system.GetToolBadge("wg"), core.YELLOW),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + "Status: WireGuard VPN is not provisioned on this machine yet." + core.NC,
			"Choose an action below to deploy WireGuard VPN or run system network diagnostics.",
		}

		options := []string{
			"Install WireGuard VPN Docker (Container Stack - Recommended)",
			"Install WireGuard VPN Native (Clean Setup)",
			"Run Deep System & Network Diagnostics Scan",
			"Exit Diagnostics",
		}

		idx, _, status := tui.SelectMenuWithContent("KROTEK WIREGUARD VPN" + " " + core.Sym.EmDash + " " + "Diagnostics & Setup", diag, options)
		if status == 2 || (status == 0 && idx == 3) {
			tui.LeaveAltScreen()
			return
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
// for the containerized WireGuard Fast Kernel VPN Tunnel Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.WireguardHealth) {
	for {
		badge := getDockerStatusBadge(composeDir)
		title := fmt.Sprintf("KROTEK WIREGUARD VPN [Docker] %s", badge)

		options := []string{
			"─── DOCKER STACK MANAGEMENT ───",
			"Show Docker Stack Status",
			"Restart Docker Container",
			"View Container Logs",
			"Run Guided Setup / Reinstallation Wizard",
			"Create Instant Configuration Backup",
			"─── LOGS & DIAGNOSTICS ───",
			"Run Deep System Diagnostics",
			"View Service Logs & Journalctl",
			"─── DANGER ZONE ───",
			"Uninstall WireGuard VPN",
			"Back / Exit",
		}

		_, opt, status := tui.SelectMenu(title, options)
		cleanOpt := core.StripANSI(opt)
		if status == 2 || (status == 0 && strings.Contains(cleanOpt, "Back / Exit")) {
			return
		}

		state := system.LoadState()
		switch {
		case strings.Contains(cleanOpt, "Show Docker Stack Status"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "compose", "--project-directory", composeDir, "ps").CombinedOutput()
			tui.PrintInBox("Docker Container Status", tui.FormatDockerContainerStatus(string(out), "No running WireGuard Docker containers found."))
		case strings.Contains(cleanOpt, "Restart Docker Container"):
			tui.ClearScreen()
			_ = exec.Command("docker", "compose", "--project-directory", composeDir, "restart").Run()
			tui.PrintInBox("Container Restart", []string{core.GREEN + "[SUCCESS] Restart command sent." + core.NC})
		case strings.Contains(cleanOpt, "View Container Logs"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "logs", "--tail", "50", "krotek-wireguard").CombinedOutput()
			tui.PrintInBox("Docker Logs", tui.FormatStyledLogLines(string(out), "No container logs found for krotek-wireguard."))
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("docker")
			health = modules.DetectWireguardHealth()
		case strings.Contains(cleanOpt, "Create Instant Configuration Backup"):
			createInstantBackup(state)
		case strings.Contains(cleanOpt, "Run Deep System Diagnostics"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "View Service Logs & Journalctl"):
			modules.ShowLogsMenu()
		case strings.Contains(cleanOpt, "Uninstall WireGuard VPN"):
			install.UninstallWizard()
			return
		}
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.WireguardHealth) {
	for {
		badge := getServiceStatusBadge()
		title := fmt.Sprintf("KROTEK WIREGUARD VPN [Native] %s", badge)

		options := []string{
			"─── CLIENT & PEER MANAGEMENT ───",
			"Add New Client Peer (Generate Config & QR Code)",
			"List Client Peers & Show Mobile QR Codes",
			"Show Live Interface & Peer Telemetry (wg show)",
			"─── SERVICE OPERATIONS ───",
			"Show WireGuard Service Status (systemctl)",
			"Restart WireGuard VPN Tunnel Service",
			"Reload WireGuard VPN Configuration",
			"Run Guided Setup / Reinstallation Wizard",
			"Create Instant Configuration Backup",
			"─── MAINTENANCE ───",
			"Run Deep System & Network Diagnostics",
			"View Service Logs & Journalctl",
			"Uninstall WireGuard VPN",
			"Exit",
		}

		_, opt, status := tui.SelectMenu(title, options)
		if status == 2 || (status == 0 && strings.Contains(opt, "Exit")) {
			return
		}

		state := system.LoadState()
		cleanOpt := core.StripANSI(opt)

		switch {
		case strings.Contains(cleanOpt, "Add New Client Peer"):
			modules.AddNewClientPeerWizard()
		case strings.Contains(cleanOpt, "List Client Peers"):
			modules.ListAndManagePeersMenu()
		case strings.Contains(cleanOpt, "Show Live Interface & Peer Telemetry"):
			modules.ShowLiveWireguardTelemetry()
		case strings.Contains(cleanOpt, "Show WireGuard Service Status"):
			tui.ClearScreen()
			out, _ := exec.Command("systemctl", "status", "wg-quick@wg0").CombinedOutput()
			tui.PrintInBox("WireGuard VPN Service Status", tui.FormatSystemctlStatusOutput("wg-quick@wg0", string(out)))
		case strings.Contains(cleanOpt, "Restart WireGuard VPN Tunnel Service"):
			tui.ClearScreen()
			_ = exec.Command("systemctl", "restart", "wg-quick@wg0").Run()
			tui.PrintInBox("Restart Service", []string{core.GREEN + "[SUCCESS] WireGuard wg-quick@wg0 restarted successfully." + core.NC})
		case strings.Contains(cleanOpt, "Reload WireGuard VPN Configuration"):
			tui.ClearScreen()
			_ = modules.SyncWireGuardConfig("wg0")
			tui.PrintInBox("Reload Config", []string{core.GREEN + "[SUCCESS] WireGuard wg0 configuration synchronized and reloaded." + core.NC})
		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			health = modules.DetectWireguardHealth()
		case strings.Contains(cleanOpt, "Create Instant Configuration Backup"):
			createInstantBackup(state)
		case strings.Contains(cleanOpt, "Run Deep System & Network Diagnostics"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()
		case strings.Contains(cleanOpt, "Uninstall WireGuard VPN"):
			install.UninstallWizard()
			return
		}
	}
}

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	health := modules.DetectWireguardHealth()
	if !health.Installed {
		noWireguardDiagnosticMenu()
		return
	}

	state := system.LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		envType = "native"
	}

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd == "" {
			cd = "/etc/wireguard-docker"
		}
		dockerManagementMenu(cd, health)
	} else {
		nativeManagementMenu(health)
	}
}
