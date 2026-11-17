// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: Samba SMB/CIFS Network File Sharing (samba)
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

	"krotek-samba-installer/core"
	"krotek-samba-installer/install"
	"krotek-samba-installer/modules"
	"krotek-samba-installer/system"
	"krotek-samba-installer/tui"
)

func getServiceStatusBadge() string {
	cmd := exec.Command("systemctl", "is-active", "smbd")
	if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) == "active" {
		return core.GREEN + "[ACTIVE]" + core.NC
	}
	cmd2 := exec.Command("systemctl", "is-active", "smb")
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
	backupFile := fmt.Sprintf("/root/samba_backup_%s.tar.gz", timestamp)

	var err error
	if state["ENV_TYPE"] == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd == "" {
			cd = "/etc/samba-docker"
		}
		err = exec.Command("tar", "-czf", backupFile, cd).Run()
	} else {
		if _, statErr := os.Stat("/etc/samba"); statErr == nil {
			err = exec.Command("tar", "-czf", backupFile, "/etc/samba").Run()
		} else {
			err = fmt.Errorf("/etc/samba does not exist")
		}
	}

	if err == nil {
		tui.PrintInBox("Instant Backup Success", []string{
			core.GREEN + "[SUCCESS] Samba backup archive generated successfully!" + core.NC,
			fmt.Sprintf("Archive: %s%s%s", core.BOLD, backupFile, core.NC),
		})
	} else {
		tui.PrintInBox("Backup Failed", []string{
			core.RED + "[ERROR] Failed to generate backup: " + err.Error() + core.NC,
		})
	}
}

func formatSmbStatusOutput(raw string) []string {
	trimmedRaw := strings.TrimSpace(raw)
	if trimmedRaw == "" {
		return tui.FormatInfoNotice("INFO", "No active Samba client connections.", core.YELLOW)
	}
	var out []string
	for _, line := range strings.Split(trimmedRaw, "\n") {
		t := strings.TrimSpace(core.SanitizeTerminalText(line))
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "Samba version") {
			out = append(out, tui.FormatKV("Samba Version", strings.TrimPrefix(t, "Samba version "), core.CYAN))
			out = append(out, "")
		} else if strings.HasPrefix(t, "PID") {
			out = append(out, "  "+core.BOLD+core.CYAN+t+core.NC)
		} else if strings.HasPrefix(t, "---") {
			out = append(out, "  "+core.DIM+strings.Repeat(core.Sym.BoxH, 68)+core.NC)
		} else {
			out = append(out, fmt.Sprintf("  %s%s%s %s", core.GREEN, core.Sym.Pointer, core.NC, t))
		}
	}
	return out
}

func viewActiveSambaStatus() {
	tui.ClearScreen()
	out, err := exec.Command("smbstatus", "-b").CombinedOutput()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		out2, _ := exec.Command("testparm", "-s").CombinedOutput()
		tui.PrintInBox("Samba Shares Definition (testparm -s)", tui.FormatKeyValueOutput(string(out2), "No Samba share configuration found."))
	} else {
		tui.PrintInBox("Active Samba Connections (smbstatus)", formatSmbStatusOutput(string(out)))
	}
}

func installSelectionMenu() {
	options := []string{
		"Docker Containerized Deployment (Recommended)",
		"Native Linux System Service",
		"<- Back to Main Menu",
	}

	for {
		idx, opt, status := tui.SelectMenu("KROTEK SAMBA" + " " + core.Sym.EmDash + " " + "Deployment Mode Selection", options)
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

// noServiceDiagnosticMenu guides the user when Samba SMB/CIFS Network File Sharing is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		options := []string{
			"Install Samba Server (Fresh Deployment Wizard) (Guided Deployment Wizard)",
			"Run Deep System & Network Diagnostics",
			"Exit",
		}

		idx, opt, status := tui.SelectMenu("KROTEK SAMBA" + " " + core.Sym.EmDash + " " + "Service Not Detected", options)
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
// for the containerized Samba SMB/CIFS Network File Sharing Docker Compose stack.
func dockerManagementMenu(composeDir string) {
	for {
		badge := getDockerStatusBadge(composeDir)
		title := fmt.Sprintf("KROTEK SAMBA [Docker] %s", badge)

		options := []string{
			"─── DOCKER STACK MANAGEMENT ───",
			"Show Docker Container Status",
			"Restart Docker Container",
			"View Container Logs",
			"Run Guided Setup / Reinstallation Wizard",
			"Create Instant Configuration Backup",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System Diagnostics",
			"Update Samba SMB Server & Check Upgrades (Safe Backup)",
			"─── DANGER ZONE ───",
			"Uninstall Samba",
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
			tui.PrintInBox("Docker Container Status", tui.FormatDockerContainerStatus(string(out), "No running Samba Docker containers found."))
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
		case strings.Contains(cleanOpt, "Uninstall Samba"):
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
		title := fmt.Sprintf("KROTEK SAMBA [Native] %s", badge)

		options := []string{
			"─── SHARES & SERVICE STATUS ───",
			"Show Service Status",
			"View Active Shares & Connections (smbstatus)",
			"─── SERVICE OPERATIONS ───",
			"Reload Samba Configuration",
			"Restart Samba Service",
			"Run Guided Setup / Reinstallation Wizard",
			"Create Instant Configuration Backup",
			"─── LOGS & DIAGNOSTICS ───",
			"Run Deep System Diagnostics",
			"View Service Logs & Journalctl",
			"Update Samba SMB Server & Check Upgrades (Safe Backup)",
			"─── DANGER ZONE ───",
			"Uninstall Samba",
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
			out, _ := exec.Command("systemctl", "status", "smbd", "smb", "nmbd", "nmb").CombinedOutput()
			tui.PrintInBox("Samba Status", tui.FormatSystemctlStatusOutput("smbd", string(out)))
		case strings.Contains(cleanOpt, "View Active Shares"):
			viewActiveSambaStatus()
		case strings.Contains(cleanOpt, "Reload Samba Configuration"):
			tui.ClearScreen()
			_ = exec.Command("smbcontrol", "all", "reload-config").Run()
			_ = exec.Command("systemctl", "reload", "smbd").Run()
			tui.PrintInBox("Reload Config", []string{core.GREEN + "[SUCCESS] Samba configuration reloaded." + core.NC})
		case strings.Contains(cleanOpt, "Restart Samba Service"):
			tui.ClearScreen()
			_ = exec.Command("systemctl", "restart", "smbd").Run()
			_ = exec.Command("systemctl", "restart", "smb").Run()
			_ = exec.Command("systemctl", "restart", "nmbd").Run()
			_ = exec.Command("systemctl", "restart", "nmb").Run()
			tui.PrintInBox("Restart Service", []string{core.GREEN + "[SUCCESS] Samba service restarted." + core.NC})
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			return
		case strings.Contains(cleanOpt, "Create Instant Configuration Backup"):
			createInstantBackup(state)
		case strings.Contains(cleanOpt, "Run Deep System Diagnostics"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()
		case strings.Contains(cleanOpt, "Uninstall Samba"):
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
			cd = "/etc/samba-docker"
		}
		dockerManagementMenu(cd)
	} else {
		nativeManagementMenu()
	}
}
