// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: OpenVPN Community Access Server (openvpn)
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

	"krotek-openvpn-installer/core"
	"krotek-openvpn-installer/install"
	"krotek-openvpn-installer/modules"
	"krotek-openvpn-installer/system"
	"krotek-openvpn-installer/tui"
)

func getServiceStatusBadge() string {
	cmd := exec.Command("systemctl", "is-active", "openvpn-server@server")
	if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) == "active" {
		return core.GREEN + "[ACTIVE]" + core.NC
	}
	cmd2 := exec.Command("systemctl", "is-active", "openvpn@server")
	if out, err := cmd2.Output(); err == nil && strings.TrimSpace(string(out)) == "active" {
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
	backupFile := fmt.Sprintf("/root/openvpn_backup_%s.tar.gz", timestamp)

	var err error
	if state["ENV_TYPE"] == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd == "" {
			cd = "/etc/openvpn-docker"
		}
		err = exec.Command("tar", "-czf", backupFile, cd).Run()
	} else {
		if _, statErr := os.Stat("/etc/openvpn"); statErr == nil {
			err = exec.Command("tar", "-czf", backupFile, "/etc/openvpn").Run()
		} else {
			err = fmt.Errorf("/etc/openvpn directory does not exist")
		}
	}

	if err == nil {
		tui.PrintInBox("Instant Backup Success", []string{
			core.GREEN + "[SUCCESS] OpenVPN backup archive generated successfully!" + core.NC,
			fmt.Sprintf("Archive: %s%s%s", core.BOLD, backupFile, core.NC),
		})
	} else {
		tui.PrintInBox("Backup Failed", []string{
			core.RED + "[ERROR] Failed to generate backup: " + err.Error() + core.NC,
		})
	}
}

func formatOpenVPNStatusLog(raw string) []string {
	var out []string
	var clients [][]string
	var routes [][]string
	section := ""
	updatedAt := ""

	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		t := strings.TrimSpace(core.SanitizeTerminalText(line))
		if t == "" || strings.HasPrefix(t, "END") || strings.HasPrefix(t, "GLOBAL STATS") {
			continue
		}
		if strings.HasPrefix(t, "Updated,") {
			updatedAt = strings.TrimSpace(strings.TrimPrefix(t, "Updated,"))
			continue
		}
		if strings.HasPrefix(t, "OpenVPN CLIENT LIST") {
			section = "clients"
			continue
		}
		if strings.HasPrefix(t, "ROUTING TABLE") {
			section = "routes"
			continue
		}
		if strings.HasPrefix(t, "Common Name,") || strings.HasPrefix(t, "Virtual Address,") {
			continue
		}

		cols := strings.Split(t, ",")
		if section == "clients" && len(cols) >= 5 {
			clients = append(clients, []string{cols[0], cols[1], cols[2] + " / " + cols[3], cols[4]})
		} else if section == "routes" && len(cols) >= 4 {
			routes = append(routes, []string{cols[0], cols[1], cols[2], cols[3]})
		}
	}

	if updatedAt != "" {
		out = append(out, tui.FormatKV("Last Updated", updatedAt, core.CYAN))
		out = append(out, "")
	}

	if len(clients) == 0 && len(routes) == 0 {
		if len(strings.TrimSpace(raw)) == 0 {
			return tui.FormatInfoNotice("INFO", "No active client sessions recorded in OpenVPN status log.", core.YELLOW)
		}
		return tui.FormatKeyValueOutput(raw, "No active client sessions recorded.")
	}

	out = append(out, core.BOLD+core.YELLOW+"CONNECTED CLIENTS:"+core.NC)
	out = append(out, tui.FormatTableRows([]string{"COMMON NAME", "REAL ADDRESS", "RX / TX BYTES", "CONNECTED SINCE"}, clients, []int{18, 20, 18, 18}, "No active clients connected.")...)
	if len(routes) > 0 {
		out = append(out, "")
		out = append(out, core.BOLD+core.YELLOW+"VPN ROUTING TABLE:"+core.NC)
		out = append(out, tui.FormatTableRows([]string{"VIRTUAL IP", "COMMON NAME", "REAL ADDRESS", "LAST REF"}, routes, []int{16, 18, 20, 18}, "")...)
	}
	return out
}

func viewConnectedClients() {
	tui.ClearScreen()
	statusLog := "/var/log/openvpn-status.log"
	if out, err := os.ReadFile(statusLog); err == nil {
		tui.PrintInBox("OpenVPN Connected Clients (/var/log/openvpn-status.log)", formatOpenVPNStatusLog(string(out)))
	} else {
		tui.PrintInBox("Connected Clients", tui.FormatInfoNotice("INFO", "No active client sessions recorded or status log not available yet.", core.YELLOW))
	}
}

func installSelectionMenu() {
	options := []string{
		"Docker Containerized Deployment (Recommended)",
		"Native Linux System Service",
		"<- Back to Main Menu",
	}

	for {
		idx, opt, status := tui.SelectMenu("KROTEK OPENVPN" + " " + core.Sym.EmDash + " " + "Deployment Mode Selection", options)
		if status == 2 || (status == 0 && strings.Contains(opt, "Back")) {
			return
		}

		switch idx {
		case 0:
			install.FreshInstallWizard("docker")
			return
		case 1:
			install.FreshInstallWizard("native")
			return
		}
	}
}

// noServiceDiagnosticMenu guides the user when OpenVPN Community Access Server is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		options := []string{
			"Install OpenVPN Server (Fresh Deployment Wizard) (Guided Deployment Wizard)",
			"Run Deep System & Network Diagnostics",
			"Exit",
		}

		idx, opt, status := tui.SelectMenu("KROTEK OPENVPN" + " " + core.Sym.EmDash + " " + "Service Not Detected", options)
		if status == 2 || (status == 0 && strings.Contains(opt, "Exit")) {
			return
		}

		switch idx {
		case 0:
			installSelectionMenu()
			return
		case 1:
			modules.RunDeepDiagnostics()
		}
	}
}

// dockerManagementMenu provides container lifecycle controls (start/stop/restart/logs)
// for the containerized OpenVPN Community Access Server Docker Compose stack.
func dockerManagementMenu(composeDir string) {
	for {
		badge := getDockerStatusBadge(composeDir)
		title := fmt.Sprintf("KROTEK OPENVPN [Docker] %s", badge)

		options := []string{
			"─── DOCKER STACK MANAGEMENT ───",
			"Show Docker Container Status",
			"Restart Docker Container",
			"View Container Logs",
			"Run Guided Setup / Reinstallation Wizard",
			"Create Instant Configuration Backup",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System Diagnostics",
			"Update OpenVPN Server & Check Upgrades (Safe Backup)",
			"─── DANGER ZONE ───",
			"Uninstall OpenVPN",
			"Exit",
		}

		_, opt, status := tui.SelectMenu(title, options)
		cleanOpt := core.StripANSI(opt)
		if status == 2 || (status == 0 && strings.Contains(cleanOpt, "Exit")) {
			return
		}

		if strings.Contains(cleanOpt, "Update ") && strings.Contains(cleanOpt, "Check Upgrades") {
			system.RunServiceUpdateFlow()
			continue
		}

		state := system.LoadState()
		switch {
		case strings.Contains(cleanOpt, "Show Docker Container Status"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "compose", "--project-directory", composeDir, "ps").CombinedOutput()
			tui.PrintInBox("Docker Container Status", tui.FormatDockerContainerStatus(string(out), "No running OpenVPN Docker containers found."))
		case strings.Contains(cleanOpt, "Restart Docker Container"):
			tui.ClearScreen()
			_ = exec.Command("docker", "compose", "--project-directory", composeDir, "restart").Run()
			tui.PrintInBox("Restart Container", []string{core.GREEN + "[SUCCESS] Docker container restarted." + core.NC})
		case strings.Contains(cleanOpt, "View Container Logs"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "compose", "--project-directory", composeDir, "logs", "--tail", "50").CombinedOutput()
			tui.PrintInBox("Container Logs (Tail 50)", tui.FormatStyledLogLines(string(out), "No container logs found."))
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("docker")
			return
		case strings.Contains(cleanOpt, "Create Instant Configuration Backup"):
			createInstantBackup(state)
		case strings.Contains(cleanOpt, "Run Deep System Diagnostics"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "Uninstall OpenVPN"):
			install.UninstallWizard()
			return
		}
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu() {
	for {
		badge := getServiceStatusBadge()
		title := fmt.Sprintf("KROTEK OPENVPN [Native] %s", badge)

		options := []string{
			"─── VPN CLIENTS & SERVICE STATUS ───",
			"Show Service Status",
			"View Connected Clients & Status",
			"─── SERVICE OPERATIONS ───",
			"Restart OpenVPN Service",
			"Reload Configuration",
			"Run Guided Setup / Reinstallation Wizard",
			"Create Instant Configuration Backup",
			"─── LOGS & DIAGNOSTICS ───",
			"Run Deep System Diagnostics",
			"View Service Logs & Journalctl",
			"Update OpenVPN Server & Check Upgrades (Safe Backup)",
			"─── DANGER ZONE ───",
			"Uninstall OpenVPN",
			"Exit",
		}

		_, opt, status := tui.SelectMenu(title, options)
		cleanOpt := core.StripANSI(opt)
		if status == 2 || (status == 0 && strings.Contains(cleanOpt, "Exit")) {
			return
		}

		if strings.Contains(cleanOpt, "Update ") && strings.Contains(cleanOpt, "Check Upgrades") {
			system.RunServiceUpdateFlow()
			continue
		}

		state := system.LoadState()
		switch {
		case strings.Contains(cleanOpt, "Show Service Status"):
			tui.ClearScreen()
			out, _ := exec.Command("systemctl", "status", "openvpn-server@server", "openvpn@server", "openvpn").CombinedOutput()
			tui.PrintInBox("OpenVPN Status", tui.FormatSystemctlStatusOutput("openvpn", string(out)))
		case strings.Contains(cleanOpt, "View Connected Clients"):
			viewConnectedClients()
		case strings.Contains(cleanOpt, "Restart OpenVPN Service"):
			tui.ClearScreen()
			_ = exec.Command("systemctl", "restart", "openvpn-server@server").Run()
			_ = exec.Command("systemctl", "restart", "openvpn@server").Run()
			tui.PrintInBox("Restart Service", []string{core.GREEN + "[SUCCESS] OpenVPN service restarted." + core.NC})
		case strings.Contains(cleanOpt, "Reload Configuration"):
			tui.ClearScreen()
			_ = exec.Command("systemctl", "reload", "openvpn-server@server").Run()
			_ = exec.Command("systemctl", "reload", "openvpn@server").Run()
			tui.PrintInBox("Reload Config", []string{core.GREEN + "[SUCCESS] Configuration reloaded." + core.NC})
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			return
		case strings.Contains(cleanOpt, "Create Instant Configuration Backup"):
			createInstantBackup(state)
		case strings.Contains(cleanOpt, "Run Deep System Diagnostics"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()
		case strings.Contains(cleanOpt, "Uninstall OpenVPN"):
			install.UninstallWizard()
			return
		}
	}
}

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	if !system.IsServiceInstalled() {
		noServiceDiagnosticMenu()
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
			cd = "/etc/openvpn-docker"
		}
		dockerManagementMenu(cd)
	} else {
		nativeManagementMenu()
	}
}
