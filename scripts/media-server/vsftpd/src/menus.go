// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: vsftpd Secure FTP Daemon (vsftpd)
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

	"krotek-vsftpd-installer/core"
	"krotek-vsftpd-installer/install"
	"krotek-vsftpd-installer/modules"
	"krotek-vsftpd-installer/system"
	"krotek-vsftpd-installer/tui"
)

func getServiceStatusBadge() string {
	cmd := exec.Command("systemctl", "is-active", "vsftpd")
	if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) == "active" {
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
	backupFile := fmt.Sprintf("/root/vsftpd_backup_%s.tar.gz", timestamp)

	var err error
	if system.IsSymlinkPath(backupFile) {
		err = fmt.Errorf("refusing symlink backup target: %s", backupFile)
	} else if state["ENV_TYPE"] == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd == "" {
			cd = "/etc/vsftpd-docker"
		}
		err = exec.Command("tar", "-czf", backupFile, cd).Run()
		if err == nil {
			_ = os.Chmod(backupFile, 0600)
		}
	} else {
		confPath := state["CONF_PATH"]
		if confPath == "" {
			confPath = "/etc/vsftpd.conf"
		}
		if _, statErr := os.Stat(confPath); statErr == nil {
			err = exec.Command("tar", "-czf", backupFile, confPath).Run()
			if err == nil {
				_ = os.Chmod(backupFile, 0600)
			}
		} else {
			err = fmt.Errorf("configuration file %s does not exist", confPath)
		}
	}

	if err == nil {
		tui.PrintInBox("Instant Backup Success", []string{
			core.GREEN + "[SUCCESS] VSFTPD backup archive generated successfully!" + core.NC,
			fmt.Sprintf("Archive: %s%s%s", core.BOLD, backupFile, core.NC),
		})
	} else {
		tui.PrintInBox("Backup Failed", []string{
			core.RED + "[ERROR] Failed to generate backup: " + err.Error() + core.NC,
		})
	}
}

func viewActiveFTPSessions() {
	tui.ClearScreen()
	var out []byte
	var err error
	if system.CommandExists("ss") {
		out, err = exec.Command("ss", "-tnp").CombinedOutput()
	} else if system.CommandExists("netstat") {
		out, err = exec.Command("netstat", "-tnp").CombinedOutput()
	} else {
		err = fmt.Errorf("neither ss nor netstat available")
	}
	var rows [][]string
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, ":21") {
				fields := strings.Fields(core.SanitizeTerminalText(line))
				if len(fields) >= 5 {
					proc := ""
					if len(fields) >= 6 {
						proc = fields[len(fields)-1]
					}
					rows = append(rows, []string{fields[0], fields[3], fields[4], proc})
				}
			}
		}
	}
	if len(rows) == 0 {
		tui.PrintInBox("Active FTP Sessions", []string{
			tui.FormatKV("Active Connections", "0 inbound FTP sessions on port 21", core.YELLOW),
			core.DIM + "  No active client transfers at this moment." + core.NC,
		})
	} else {
		tui.PrintInBox("Active FTP Inbound Sessions", tui.FormatTableRows([]string{"STATE", "LOCAL ADDRESS", "REMOTE PEER", "PROCESS"}, rows, []int{12, 22, 22, 18}, ""))
	}
}

func installSelectionMenu() {
	options := []string{
		"Docker Containerized Deployment (Recommended)",
		"Native Linux System Service",
		"<- Back to Main Menu",
	}

	for {
		idx, opt, status := tui.SelectMenu("KROTEK VSFTPD" + " " + core.Sym.EmDash + " " + "Deployment Mode Selection", options)
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

// noServiceDiagnosticMenu guides the user when vsftpd Secure FTP Daemon is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		options := []string{
			"Install VSFTPD Server (Fresh Deployment Wizard) (Guided Deployment Wizard)",
			"Run Deep System & Network Diagnostics",
			"Exit",
		}

		idx, opt, status := tui.SelectMenu("KROTEK VSFTPD" + " " + core.Sym.EmDash + " " + "Service Not Detected", options)
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
// for the containerized vsftpd Secure FTP Daemon Docker Compose stack.
func dockerManagementMenu(composeDir string) {
	for {
		badge := getDockerStatusBadge(composeDir)
		title := fmt.Sprintf("KROTEK VSFTPD [Docker] %s", badge)

		options := []string{
			"─── DOCKER STACK MANAGEMENT ───",
			"Show Docker Container Status",
			"View Active FTP Sessions",
			"Restart Docker Container",
			"View Container Logs",
			"Run Guided Setup / Reinstallation Wizard",
			"Create Instant Configuration Backup",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System Diagnostics",
			"Update VSFTPD FTP Server & Check Upgrades (Safe Backup)",
			"─── DANGER ZONE ───",
			"Uninstall VSFTPD",
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
			tui.PrintInBox("Docker Container Status", tui.FormatDockerContainerStatus(string(out), "No running VSFTPD Docker containers found."))
		case strings.Contains(cleanOpt, "View Active FTP Sessions"):
			viewActiveFTPSessions()
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
		case strings.Contains(cleanOpt, "Uninstall VSFTPD"):
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
		title := fmt.Sprintf("KROTEK VSFTPD [Native] %s", badge)

		options := []string{
			"─── SESSIONS & SERVICE STATUS ───",
			"Show Service Status",
			"View Active FTP Sessions",
			"─── SERVICE OPERATIONS ───",
			"Restart VSFTPD Service",
			"Run Guided Setup / Reinstallation Wizard",
			"Create Instant Configuration Backup",
			"─── LOGS & DIAGNOSTICS ───",
			"Run Deep System Diagnostics",
			"View Service Logs & Journalctl",
			"Update VSFTPD FTP Server & Check Upgrades (Safe Backup)",
			"─── DANGER ZONE ───",
			"Uninstall VSFTPD",
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
			out, _ := exec.Command("systemctl", "status", "vsftpd").CombinedOutput()
			tui.PrintInBox("VSFTPD Status", tui.FormatSystemctlStatusOutput("vsftpd", string(out)))
		case strings.Contains(cleanOpt, "View Active FTP Sessions"):
			viewActiveFTPSessions()
		case strings.Contains(cleanOpt, "Restart VSFTPD Service"):
			tui.ClearScreen()
			_ = exec.Command("systemctl", "restart", "vsftpd").Run()
			tui.PrintInBox("Restart Service", []string{core.GREEN + "[SUCCESS] VSFTPD service restarted." + core.NC})
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			return
		case strings.Contains(cleanOpt, "Create Instant Configuration Backup"):
			createInstantBackup(state)
		case strings.Contains(cleanOpt, "Run Deep System Diagnostics"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()
		case strings.Contains(cleanOpt, "Uninstall VSFTPD"):
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
			cd = "/etc/vsftpd-docker"
		}
		dockerManagementMenu(cd)
	} else {
		nativeManagementMenu()
	}
}
