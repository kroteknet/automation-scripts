// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: Universal Linux Firewall Management Engine (firewall-manager)
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
	"strings"

	"krotek-firewall-manager-installer/core"
	"krotek-firewall-manager-installer/install"
	"krotek-firewall-manager-installer/modules"
	"krotek-firewall-manager-installer/system"
	"krotek-firewall-manager-installer/tui"
)

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	composeDir := state["COMPOSE_DIR"]
	if composeDir == "" {
		composeDir = "/etc/firewall-manager-docker"
	}

	if envType == "" {
		if _, err := os.Stat(composeDir); err == nil {
			envType = "docker"
		} else {
			envType = "native"
		}
	}

	if envType == "docker" {
		dockerManagementMenu(composeDir)
	} else {
		nativeManagementMenu()
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu() {
	_ = system.LoadState()
	for {
		options := []string{
			"─── FIREWALL RULES & PORTS ───",
			"[1] View & Delete Active Rules (Interactive Table)",
			"[2] Add New Firewall Rule (Port / Service / IP)",
			"[3] Quick Service Security Presets (Web, SSH, VPN, DB)",
			"─── INTRUSION PREVENTION & FAIL2BAN ───",
			"[4] Fail2ban Dashboard & Active Jails",
			"[5] View Banned IPs & Quick Unban Engine",
			"─── NETWORK MONITORING & DIAGNOSTICS ───",
			"[6] Open Listening Ports & Process Scanner",
			"[7] Live Active Connections Inspector",
			"[8] View Firewall & Security Log Streams",
			"─── SYSTEM HARDENING & FAILSAFE ───",
			"[9] Kernel Sysctl Hardening (Anti-DDoS & SYN Flood)",
			"[10] Emergency Panic Mode (Lock Down All Except Current IP)",
			"[11] Backup & Restore Firewall Rules",
			"─── MAINTENANCE ───",
			"Run Guided Setup / Reinstallation Wizard",
			"[12] Deep System Security Diagnostics",
			"[13] Uninstall Firewall Stack",
			"Exit",
		}

		titleStr := "FIREWALL & SECURITY MANAGER - Management Hub"
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "[1] View & Delete Active Rules"):
			modules.ViewAndManageRulesMenu()
		case strings.HasPrefix(cleanOpt, "[2] Add New Firewall Rule"):
			modules.AddNewRuleWizard()
		case strings.HasPrefix(cleanOpt, "[3] Quick Service Security Presets"):
			modules.QuickPresetsMenu()
		case strings.HasPrefix(cleanOpt, "[4] Fail2ban Dashboard"):
			modules.Fail2banMenu()
		case strings.HasPrefix(cleanOpt, "[5] Unban IP Address"):
			modules.Fail2banUnbanMenu()
		case strings.HasPrefix(cleanOpt, "[6] Open Listening Ports"):
			modules.OpenPortsScanner()
		case strings.HasPrefix(cleanOpt, "[7] Live Active Connections"):
			modules.ActiveConnectionsInspector()
		case strings.HasPrefix(cleanOpt, "[8] View Firewall & Security Log"):
			modules.ShowLogsMenu()
		case strings.HasPrefix(cleanOpt, "[9] Kernel Sysctl Hardening"):
			modules.KernelHardeningMenu()
		case strings.HasPrefix(cleanOpt, "[10] Emergency Panic Mode"):
			modules.PanicEmergencyLockdown()
		case strings.HasPrefix(cleanOpt, "[11] Backup & Restore Firewall Rules"):
			modules.BackupRestoreMenu()
		case strings.Contains(cleanOpt, "Guided Setup"):
			installerMenu()
			return
		case strings.HasPrefix(cleanOpt, "[12] Deep System Security"):
			modules.RunDeepDiagnostics()
		case strings.HasPrefix(cleanOpt, "[13] Uninstall Firewall Stack"):
			install.UninstallWizard()
			return
		}
	}
}

// dockerManagementMenu provides container lifecycle controls (start/stop/restart/logs)
// for the containerized Universal Linux Firewall Management Engine Docker Compose stack.
func dockerManagementMenu(composeDir string) {
	for {
		options := []string{
			"─── DOCKER STACK MANAGEMENT ───",
			"Update Firewall Manager & Check Upgrades (Safe Backup)",
			"Check Container Status (docker ps)",
			fmt.Sprintf("Edit docker-compose.yml  (%s/docker-compose.yml)", composeDir),
			"Restart Docker Stack (docker compose restart)",
			"View Container Logs (last 50 lines)",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Guided Setup / Reinstallation Wizard",
			"Uninstall Docker Stack",
			"Exit",
		}

		titleStr := "FIREWALL & SECURITY MANAGER - Management (Docker)"
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "Check Container Status"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "ps", "--filter", "name=firewall-manager").CombinedOutput()
			tui.PrintInBox("Docker Container Status", tui.FormatDockerContainerStatus(string(out), "No running Docker containers found for firewall-manager."))

		case strings.HasPrefix(cleanOpt, "Edit docker-compose.yml"):
			tui.ClearScreen()
			composeFile := filepath.Join(composeDir, "docker-compose.yml")
			_ = tui.RunEditor(composeFile)

			tui.ClearScreen()
			res := tui.AskYesNo("Recreate and restart Docker Compose container stack (docker compose up -d)?")
			var resInfo []string
			if res == 0 {
				if system.RunDockerCompose(composeDir, "Recreating Docker stack", false, "up", "-d") {
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

		case strings.HasPrefix(cleanOpt, "Restart Docker Stack"):
			tui.ClearScreen()
			var resMsg []string
			okRestart := system.RunDockerCompose(composeDir, "Restarting Docker stack", false, "restart")
			if !okRestart {
				okRestart = system.RunStep("Restarting Docker container fallback", "docker", "restart", "krotek-firewall-manager")
			}
			if okRestart {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("Docker Stack", "Restarted successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
			}
			tui.PrintInBox("Restart Docker Stack Result", resMsg)

		case strings.HasPrefix(cleanOpt, "View Container Logs"):
			tui.ClearScreen()
			out, err := exec.Command("docker", "logs", "--tail", "50", "krotek-firewall-manager").CombinedOutput()
			if (err != nil || len(out) == 0) && system.ValidateSafeComposeDir(composeDir) == nil {
				out, err = exec.Command("docker", "compose", "--project-directory", composeDir, "logs", "--tail", "50").CombinedOutput()
			}
			_ = err
			tui.PrintInBox("Docker Container Logs (Last 50 lines)", tui.FormatStyledLogLines(string(out), "No container logs found."))

		case strings.Contains(cleanOpt, "Guided Setup"):
			installerMenu()
			return

		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallWizard()
			return
		}
	}
}
