// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: Mail-in-a-Box All-In-One Mail Server (mailinabox)
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

	"krotek-mailinabox-installer/core"
	"krotek-mailinabox-installer/install"
	"krotek-mailinabox-installer/modules"
	"krotek-mailinabox-installer/system"
	"krotek-mailinabox-installer/tui"
)

func getServiceStatusBadge() string {
	cmd := exec.Command("systemctl", "is-active", "mailinabox")
	if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) == "active" {
		return core.GREEN + "[ACTIVE]" + core.NC
	}
	cmd2 := exec.Command("systemctl", "is-active", "postfix")
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
	backupFile := fmt.Sprintf("/root/mailinabox_backup_%s.tar.gz", timestamp)

	var err error
	if state["ENV_TYPE"] == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd == "" {
			cd = "/etc/mailinabox-docker"
		}
		err = exec.Command("tar", "-czf", backupFile, cd).Run()
	} else {
		if _, statErr := os.Stat("/home/user-data"); statErr == nil {
			err = exec.Command("tar", "-czf", backupFile, "/home/user-data").Run()
		} else {
			err = fmt.Errorf("/home/user-data directory does not exist")
		}
	}

	if err == nil {
		tui.PrintInBox("Instant Backup Success", []string{
			core.GREEN + "[SUCCESS] Mail-in-a-Box backup archive generated successfully!" + core.NC,
			fmt.Sprintf("Archive: %s%s%s", core.BOLD, backupFile, core.NC),
		})
	} else {
		tui.PrintInBox("Backup Failed", []string{
			core.RED + "[ERROR] Failed to generate backup: " + err.Error() + core.NC,
		})
	}
}

func installSelectionMenu() {
	var options []string
	if core.SysInfo.Family == "alpine" {
		options = []string{
			"Docker Containerized Deployment (Recommended - Required for Alpine)",
			"<- Back to Main Menu",
		}
	} else {
		options = []string{
			"Docker Containerized Deployment (Recommended)",
			"Native Linux System Service",
			"<- Back to Main Menu",
		}
	}

	for {
		idx, opt, status := tui.SelectMenu("KROTEK MAILINABOX" + " " + core.Sym.EmDash + " " + "Deployment Mode Selection", options)
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

// noServiceDiagnosticMenu guides the user when Mail-in-a-Box All-In-One Mail Server is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		options := []string{
			"Install Mail-in-a-Box Server (Fresh Deployment Wizard) (Guided Deployment Wizard)",
			"Run Deep System & Network Diagnostics",
			"Exit",
		}

		idx, opt, status := tui.SelectMenu("KROTEK MAILINABOX" + " " + core.Sym.EmDash + " " + "Service Not Detected", options)
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
// for the containerized Mail-in-a-Box All-In-One Mail Server Docker Compose stack.
func dockerManagementMenu(composeDir string) {
	for {
		badge := getDockerStatusBadge(composeDir)
		title := fmt.Sprintf("KROTEK MAILINABOX [Docker] %s", badge)

		options := []string{
			"─── DOCKER STACK MANAGEMENT ───",
			"Show Docker Container Status",
			"Restart Docker Container",
			"View Container Logs",
			"Run Guided Setup / Reinstallation Wizard",
			"Create Instant Mailbox Backup",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System Diagnostics",
			"Update Mail-in-a-Box & Check Upgrades (Safe Backup)",
			"─── DANGER ZONE ───",
			"Uninstall Mail-in-a-Box",
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
			tui.PrintInBox("Docker Container Status", tui.FormatDockerContainerStatus(string(out), "No running Mail-in-a-Box Docker containers found."))
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
		case strings.Contains(cleanOpt, "Create Instant Mailbox Backup"):
			createInstantBackup(state)
		case strings.Contains(cleanOpt, "Run Deep System Diagnostics"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "Uninstall Mail-in-a-Box"):
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
		title := fmt.Sprintf("KROTEK MAILINABOX [Native] %s", badge)

		options := []string{
			"─── SERVICE OPERATIONS ───",
			"Show Service Status",
			"Restart Mail-in-a-Box Services",
			"Run Guided Setup / Reinstallation Wizard",
			"Create Instant Mailbox Backup",
			"─── LOGS & DIAGNOSTICS ───",
			"Run Deep System Diagnostics",
			"View Service Logs & Journalctl",
			"Update Mail-in-a-Box & Check Upgrades (Safe Backup)",
			"─── DANGER ZONE ───",
			"Uninstall Mail-in-a-Box",
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
			out, _ := exec.Command("mailinabox", "--status").CombinedOutput()
			if len(strings.TrimSpace(string(out))) > 0 {
				tui.PrintInBox("Mail-in-a-Box Status", tui.FormatKeyValueOutput(string(out), "No Mail-in-a-Box status output available."))
			} else {
				out, _ = exec.Command("systemctl", "status", "mailinabox", "postfix", "dovecot").CombinedOutput()
				tui.PrintInBox("Mail-in-a-Box Status", tui.FormatSystemctlStatusOutput("mailinabox", string(out)))
			}
		case strings.Contains(cleanOpt, "Restart Mail-in-a-Box Services"):
			tui.ClearScreen()
			_ = exec.Command("systemctl", "restart", "mailinabox", "postfix", "dovecot").Run()
			tui.PrintInBox("Restart Service", []string{core.GREEN + "[SUCCESS] Mail services restarted." + core.NC})
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			return
		case strings.Contains(cleanOpt, "Create Instant Mailbox Backup"):
			createInstantBackup(state)
		case strings.Contains(cleanOpt, "Run Deep System Diagnostics"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()
		case strings.Contains(cleanOpt, "Uninstall Mail-in-a-Box"):
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
			cd = "/etc/mailinabox-docker"
		}
		dockerManagementMenu(cd)
	} else {
		nativeManagementMenu()
	}
}
