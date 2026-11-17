// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: Exim Mail Transfer Agent (exim)
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

	"krotek-exim-installer/core"
	"krotek-exim-installer/install"
	"krotek-exim-installer/modules"
	"krotek-exim-installer/system"
	"krotek-exim-installer/tui"
)

func getServiceStatusBadge() string {
	cmd := exec.Command("systemctl", "is-active", "exim4")
	if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) == "active" {
		return core.GREEN + "[ACTIVE]" + core.NC
	}
	cmd2 := exec.Command("systemctl", "is-active", "exim")
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
	backupFile := fmt.Sprintf("/root/exim_backup_%s.tar.gz", timestamp)

	var err error
	if state["ENV_TYPE"] == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd == "" {
			cd = "/etc/exim-docker"
		}
		err = exec.Command("tar", "-czf", backupFile, cd).Run()
	} else {
		if _, statErr := os.Stat("/etc/exim4"); statErr == nil {
			err = exec.Command("tar", "-czf", backupFile, "/etc/exim4").Run()
		} else {
			err = fmt.Errorf("/etc/exim4 directory does not exist")
		}
	}

	if err == nil {
		tui.PrintInBox("Instant Backup Success", []string{
			core.GREEN + "[SUCCESS] Exim backup archive generated successfully!" + core.NC,
			fmt.Sprintf("Archive: %s%s%s", core.BOLD, backupFile, core.NC),
		})
	} else {
		tui.PrintInBox("Backup Failed", []string{
			core.RED + "[ERROR] Failed to generate backup: " + err.Error() + core.NC,
		})
	}
}

func formatMailQueueOutput(raw string) []string {
	trimmedRaw := strings.TrimSpace(raw)
	if trimmedRaw == "" || strings.Contains(strings.ToLower(trimmedRaw), "mail queue is empty") {
		return []string{
			tui.FormatKV("Queue Status", "EMPTY (0 queued messages)", core.GREEN+core.BOLD),
		}
	}
	var out []string
	out = append(out, core.BOLD+core.YELLOW+"ACTIVE MAIL QUEUE ENTRIES:"+core.NC)
	out = append(out, "  "+core.DIM+strings.Repeat(core.Sym.BoxH, 68)+core.NC)
	for _, line := range strings.Split(trimmedRaw, "\n") {
		clean := core.SanitizeTerminalText(strings.TrimRight(line, "\r"))
		t := strings.TrimSpace(clean)
		if t == "" {
			continue
		}
		upper := strings.ToUpper(t)
		switch {
		case strings.Contains(upper, "FROZEN") || strings.Contains(upper, "BOUNCE") || strings.Contains(upper, "ERROR"):
			out = append(out, "  "+core.RED+core.BOLD+t+core.NC)
		case strings.HasPrefix(t, "***") || strings.Contains(t, "Total requests"):
			out = append(out, "  "+core.YELLOW+t+core.NC)
		case strings.Contains(t, "@"):
			out = append(out, fmt.Sprintf("  %s%s%s %s", core.CYAN, core.Sym.Pointer, core.NC, t))
		default:
			out = append(out, "    "+core.DIM+t+core.NC)
		}
	}
	return out
}

func viewMailQueue() {
	tui.ClearScreen()
	out, err := exec.Command("exim", "-bp").CombinedOutput()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		out2, _ := exec.Command("mailq").CombinedOutput()
		tui.PrintInBox("Exim Mail Queue (mailq)", formatMailQueueOutput(string(out2)))
	} else {
		tui.PrintInBox("Exim Mail Queue (exim -bp)", formatMailQueueOutput(string(out)))
	}
}

func flushMailQueue() {
	tui.ClearScreen()
	_ = exec.Command("exim", "-qf").Run()
	tui.PrintInBox("Flush Mail Queue", []string{
		core.GREEN + "[SUCCESS] Attempted delivery for all queued messages." + core.NC,
	})
}

func installSelectionMenu() {
	options := []string{
		"Docker Containerized Deployment (Recommended)",
		"Native Linux System Service",
		"<- Back to Main Menu",
	}

	for {
		idx, opt, status := tui.SelectMenu("KROTEK EXIM" + " " + core.Sym.EmDash + " " + "Deployment Mode Selection", options)
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

// noServiceDiagnosticMenu guides the user when Exim Mail Transfer Agent is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		options := []string{
			"Install Exim4 Mail Server (Fresh Deployment Wizard) (Guided Deployment Wizard)",
			"Run Deep System & Network Diagnostics",
			"Exit",
		}

		idx, opt, status := tui.SelectMenu("KROTEK EXIM" + " " + core.Sym.EmDash + " " + "Service Not Detected", options)
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
// for the containerized Exim Mail Transfer Agent Docker Compose stack.
func dockerManagementMenu(composeDir string) {
	for {
		badge := getDockerStatusBadge(composeDir)
		title := fmt.Sprintf("KROTEK EXIM [Docker] %s", badge)

		options := []string{
			"─── DOCKER STACK MANAGEMENT ───",
			"Show Docker Container Status",
			"Restart Docker Container",
			"View Container Logs",
			"Run Guided Setup / Reinstallation Wizard",
			"Create Instant Configuration Backup",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System Diagnostics",
			"Update Exim MTA & Check Upgrades (Safe Backup)",
			"─── DANGER ZONE ───",
			"Uninstall Exim",
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
			tui.PrintInBox("Docker Container Status", tui.FormatDockerContainerStatus(string(out), "No running Exim Docker containers found."))
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
		case strings.Contains(cleanOpt, "Uninstall Exim"):
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
		title := fmt.Sprintf("KROTEK EXIM [Native] %s", badge)

		options := []string{
			"─── MAIL QUEUE & SERVICE STATUS ───",
			"Show Service Status",
			"View Active Mail Queue",
			"Flush Mail Queue (Deliver Queued Mail)",
			"─── SERVICE OPERATIONS ───",
			"Restart Exim4 Service",
			"Reload Exim4 Configuration",
			"Run Guided Setup / Reinstallation Wizard",
			"Create Instant Configuration Backup",
			"─── LOGS & DIAGNOSTICS ───",
			"Run Deep System Diagnostics",
			"View Service Logs & Journalctl",
			"Update Exim MTA & Check Upgrades (Safe Backup)",
			"─── DANGER ZONE ───",
			"Uninstall Exim",
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
			out, _ := exec.Command("systemctl", "status", "exim4", "exim").CombinedOutput()
			tui.PrintInBox("Exim Status", tui.FormatSystemctlStatusOutput("exim4", string(out)))
		case strings.Contains(cleanOpt, "View Active Mail Queue"):
			viewMailQueue()
		case strings.Contains(cleanOpt, "Flush Mail Queue"):
			flushMailQueue()
		case strings.Contains(cleanOpt, "Restart Exim4 Service"):
			tui.ClearScreen()
			_ = exec.Command("systemctl", "restart", "exim4").Run()
			_ = exec.Command("systemctl", "restart", "exim").Run()
			tui.PrintInBox("Restart Service", []string{core.GREEN + "[SUCCESS] Exim service restarted." + core.NC})
		case strings.Contains(cleanOpt, "Reload Exim4 Configuration"):
			tui.ClearScreen()
			_ = exec.Command("systemctl", "reload", "exim4").Run()
			_ = exec.Command("systemctl", "reload", "exim").Run()
			tui.PrintInBox("Reload Config", []string{core.GREEN + "[SUCCESS] Exim configuration reloaded." + core.NC})
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			return
		case strings.Contains(cleanOpt, "Create Instant Configuration Backup"):
			createInstantBackup(state)
		case strings.Contains(cleanOpt, "Run Deep System Diagnostics"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()
		case strings.Contains(cleanOpt, "Uninstall Exim"):
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
			cd = "/etc/exim-docker"
		}
		dockerManagementMenu(cd)
	} else {
		nativeManagementMenu()
	}
}
