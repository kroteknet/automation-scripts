// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: NFS Kernel Network File Server (nfs)
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

	"krotek-nfs-installer/core"
	"krotek-nfs-installer/install"
	"krotek-nfs-installer/modules"
	"krotek-nfs-installer/system"
	"krotek-nfs-installer/tui"
)

func getServiceStatusBadge() string {
	cmd := exec.Command("systemctl", "is-active", "nfs-server")
	if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) == "active" {
		return core.GREEN + "[ACTIVE]" + core.NC
	}
	cmd2 := exec.Command("systemctl", "is-active", "nfs-kernel-server")
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
	backupFile := fmt.Sprintf("/root/nfs_backup_%s.tar.gz", timestamp)

	var err error
	if state["ENV_TYPE"] == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd == "" {
			cd = "/etc/nfs-docker"
		}
		err = exec.Command("tar", "-czf", backupFile, cd).Run()
	} else {
		if _, statErr := os.Stat("/etc/exports"); statErr == nil {
			err = exec.Command("tar", "-czf", backupFile, "/etc/exports").Run()
		} else {
			err = fmt.Errorf("/etc/exports does not exist")
		}
	}

	if err == nil {
		tui.PrintInBox("Instant Backup Success", []string{
			core.GREEN + "[SUCCESS] NFS backup archive generated successfully!" + core.NC,
			fmt.Sprintf("Archive: %s%s%s", core.BOLD, backupFile, core.NC),
		})
	} else {
		tui.PrintInBox("Backup Failed", []string{
			core.RED + "[ERROR] Failed to generate backup: " + err.Error() + core.NC,
		})
	}
}

func formatNFSExportsOutput(raw string) []string {
	trimmedRaw := strings.TrimSpace(raw)
	if trimmedRaw == "" {
		return tui.FormatInfoNotice("INFO", "No active NFS exports configured.", core.YELLOW)
	}
	var out []string
	out = append(out, core.BOLD+core.YELLOW+"ACTIVE NFS EXPORT RULES:"+core.NC)
	out = append(out, "  "+core.DIM+strings.Repeat(core.Sym.BoxH, 68)+core.NC)
	for _, line := range strings.Split(trimmedRaw, "\n") {
		clean := core.SanitizeTerminalText(strings.TrimRight(line, "\r"))
		t := strings.TrimSpace(clean)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "#") {
			out = append(out, "  "+core.DIM+t+core.NC)
			continue
		}
		fields := strings.Fields(t)
		if len(fields) >= 2 {
			exportPath := fields[0]
			targets := strings.Join(fields[1:], " ")
			if parenIdx := strings.Index(targets, "("); parenIdx != -1 {
				host := strings.TrimSpace(targets[:parenIdx])
				opts := targets[parenIdx:]
				out = append(out, fmt.Sprintf("  %s%s%s %s%-24s%s  %s%s%s %s%s%s",
					core.CYAN, core.Sym.Pointer, core.NC,
					core.BOLD+core.CYAN, exportPath, core.NC,
					core.GREEN, host, core.NC,
					core.YELLOW, opts, core.NC))
				continue
			}
			out = append(out, tui.FormatKV(exportPath, targets, core.GREEN))
			continue
		}
		out = append(out, "    "+core.YELLOW+t+core.NC)
	}
	return out
}

func viewActiveNFSExports() {
	tui.ClearScreen()
	out, err := exec.Command("exportfs", "-v").CombinedOutput()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		out2, _ := exec.Command("cat", "/etc/exports").CombinedOutput()
		tui.PrintInBox("Active NFS Exports (/etc/exports)", formatNFSExportsOutput(string(out2)))
	} else {
		tui.PrintInBox("Active NFS Exports (exportfs -v)", formatNFSExportsOutput(string(out)))
	}
}

func installSelectionMenu() {
	options := []string{
		"Docker Containerized Deployment (Recommended)",
		"Native Linux System Service",
		"<- Back to Main Menu",
	}

	for {
		idx, opt, status := tui.SelectMenu("KROTEK NFS" + " " + core.Sym.EmDash + " " + "Deployment Mode Selection", options)
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

// noServiceDiagnosticMenu guides the user when NFS Kernel Network File Server is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		options := []string{
			"Install NFS Server (Fresh Deployment Wizard) (Guided Deployment Wizard)",
			"Run Deep System & Network Diagnostics",
			"Exit",
		}

		idx, opt, status := tui.SelectMenu("KROTEK NFS" + " " + core.Sym.EmDash + " " + "Service Not Detected", options)
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
// for the containerized NFS Kernel Network File Server Docker Compose stack.
func dockerManagementMenu(composeDir string) {
	for {
		badge := getDockerStatusBadge(composeDir)
		title := fmt.Sprintf("KROTEK NFS [Docker] %s", badge)

		options := []string{
			"─── DOCKER STACK MANAGEMENT ───",
			"Show Docker Container Status",
			"View Active NFS Exports",
			"Restart Docker Container",
			"View Container Logs",
			"Run Guided Setup / Reinstallation Wizard",
			"Create Instant Configuration Backup",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System Diagnostics",
			"Update NFS Kernel Server & Check Upgrades (Safe Backup)",
			"─── DANGER ZONE ───",
			"Uninstall NFS",
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
			tui.PrintInBox("Docker Container Status", tui.FormatDockerContainerStatus(string(out), "No running NFS Docker containers found."))
		case strings.Contains(cleanOpt, "View Active NFS Exports"):
			viewActiveNFSExports()
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
		case strings.Contains(cleanOpt, "Uninstall NFS"):
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
		title := fmt.Sprintf("KROTEK NFS [Native] %s", badge)

		options := []string{
			"─── EXPORTS & SERVICE STATUS ───",
			"Show Service Status",
			"View Active NFS Exports",
			"─── SERVICE OPERATIONS ───",
			"Reload NFS Exports (exportfs -rav)",
			"Restart NFS Service",
			"Run Guided Setup / Reinstallation Wizard",
			"Create Instant Configuration Backup",
			"─── LOGS & DIAGNOSTICS ───",
			"Run Deep System Diagnostics",
			"View Service Logs & Journalctl",
			"Update NFS Kernel Server & Check Upgrades (Safe Backup)",
			"─── DANGER ZONE ───",
			"Uninstall NFS",
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
			out, _ := exec.Command("systemctl", "status", "nfs-server", "nfs-kernel-server", "nfs").CombinedOutput()
			tui.PrintInBox("NFS Status", tui.FormatSystemctlStatusOutput("nfs-server", string(out)))
		case strings.Contains(cleanOpt, "View Active NFS Exports"):
			viewActiveNFSExports()
		case strings.Contains(cleanOpt, "Reload NFS Exports"):
			tui.ClearScreen()
			_ = exec.Command("exportfs", "-rav").Run()
			tui.PrintInBox("Reload Exports", []string{core.GREEN + "[SUCCESS] NFS export tables reloaded." + core.NC})
		case strings.Contains(cleanOpt, "Restart NFS Service"):
			tui.ClearScreen()
			_ = exec.Command("systemctl", "restart", "nfs-server").Run()
			_ = exec.Command("systemctl", "restart", "nfs-kernel-server").Run()
			_ = exec.Command("systemctl", "restart", "nfs").Run()
			tui.PrintInBox("Restart Service", []string{core.GREEN + "[SUCCESS] NFS service restarted." + core.NC})
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			return
		case strings.Contains(cleanOpt, "Create Instant Configuration Backup"):
			createInstantBackup(state)
		case strings.Contains(cleanOpt, "Run Deep System Diagnostics"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()
		case strings.Contains(cleanOpt, "Uninstall NFS"):
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
			cd = "/etc/nfs-docker"
		}
		dockerManagementMenu(cd)
	} else {
		nativeManagementMenu()
	}
}
