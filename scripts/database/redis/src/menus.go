// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: Redis In-Memory Data Structure Store (redis)
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
	"time"

	"krotek-redis-installer/core"
	"krotek-redis-installer/install"
	"krotek-redis-installer/modules"
	"krotek-redis-installer/system"
	"krotek-redis-installer/tui"
)

// noServiceDiagnosticMenu guides the user when Redis In-Memory Data Structure Store is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		tui.ClearScreen()
		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("Redis Service", core.RED+"[NOT INSTALLED]"+core.NC, core.RED),
			tui.FormatKV("Redis Binary Tool", system.GetToolBadge("redis-cli"), core.YELLOW),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + "Status: Redis in-memory database is not provisioned on this machine yet." + core.NC,
			"Choose an action below to deploy Redis or run system network diagnostics.",
		}

		options := []string{
			"Install Redis Docker (Containerized Stack - Recommended)",
			"Install Redis Native (System Packages + Auto-Tuned)",
			"Run Deep System & Network Diagnostics Scan",
			"Exit Diagnostics",
		}

		idx, _, status := tui.SelectMenuWithContent("redis-manager - System Diagnostics (Redis Not Found)", diagLines, options)
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
// for the containerized Redis In-Memory Data Structure Store Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.RedisHealth) {
	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart Redis Container"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start Redis Container  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		state := system.LoadState()
		port := state["PORT"]
		if port == "" {
			port = "6379"
		}

		options := []string{
			"─── REDIS DASHBOARD & TOOLS ───",
			"View Application Dashboard & Connection Info",
			"Run Redis Ping & Health Check",
			"Open Interactive redis-cli Shell",
			"Save In-Memory Snapshot to Disk (SAVE)",
			"Create Instant RDB Backup Archive",
			"─── DOCKER STACK MANAGEMENT ───",
			fmt.Sprintf("Edit docker-compose.yml  (%s/docker-compose.yml)", composeDir),
			restartLabel,
			"Update Redis Server & Check Upgrades (Safe Backup)",
			"View Container Logs (last 50 lines)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & System Journal",
			"Run Deep System & Database Diagnostics",
			"─── DANGER ZONE ───",
			"Uninstall Redis Docker Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("redis-manager  -  Management (Docker: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "View Application Dashboard"):
			tui.ClearScreen()
			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "Redis In-Memory Data Store (Docker)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Service State", func() string {
				if health.IsRunning {
					return core.GREEN + "[ACTIVE / RUNNING]" + core.NC
				}
				return core.RED + "[STOPPED / INACTIVE]" + core.NC
			}(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost Access", fmt.Sprintf("127.0.0.1:%s", port), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Network IP", fmt.Sprintf("%s:%s", system.GetLocalIP(), port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Port", port, core.YELLOW))
			if state["ADMIN_PASS"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Authentication", "Password Protected", core.GREEN))
			} else {
				dbLines = append(dbLines, tui.FormatKV("Authentication", "Passwordless", core.YELLOW))
			}
			dbLines = append(dbLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("CLI Command", "redis-manager", core.GREEN))
			tui.PrintInBox("Redis Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Run Redis Ping"):
			tui.ClearScreen()
			var healthLines []string
			adminPass := state["ADMIN_PASS"]

			pingArgs := []string{"exec"}
			if adminPass != "" {
				pingArgs = append(pingArgs, "-e", "REDISCLI_AUTH")
			}
			pingArgs = append(pingArgs, "krotek-redis-server", "redis-cli", "PING")
			pingCmd := exec.Command("docker", pingArgs...)
			if adminPass != "" {
				pingCmd.Env = append(os.Environ(), "REDISCLI_AUTH="+adminPass)
			}
			out, err := pingCmd.CombinedOutput()
			if err == nil && strings.Contains(strings.ToUpper(string(out)), "PONG") {
				healthLines = append(healthLines, core.GREEN+core.BOLD+"[SUCCESS] Redis responded with PONG!"+core.NC)
				healthLines = append(healthLines, tui.FormatKV("Response", "PONG (Healthy)", core.GREEN))
			} else {
				healthLines = append(healthLines, core.RED+core.BOLD+"[WARNING] Redis ping failed!"+core.NC)
				healthLines = append(healthLines, "Ensure container is running and password is correct.")
			}
			tui.PrintInBox("Redis Health Check", healthLines)

		case strings.HasPrefix(cleanOpt, "Open Interactive redis-cli"):
			adminPass := state["ADMIN_PASS"]
			tui.LeaveAltScreen()
			tui.ClearScreen()
			fmt.Printf("%sConnecting to Redis shell in container 'krotek-redis-server'...%s\n\n", core.CYAN, core.NC)
			args := []string{"exec", "-it"}
			if adminPass != "" {
				args = append(args, "-e", "REDISCLI_AUTH")
			}
			args = append(args, "krotek-redis-server", "redis-cli")
			c := exec.Command("docker", args...)
			if adminPass != "" {
				c.Env = append(os.Environ(), "REDISCLI_AUTH="+adminPass)
			}
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = c.Run()
			tui.EnterAltScreen()

		case strings.HasPrefix(cleanOpt, "Save In-Memory Snapshot"):
			tui.ClearScreen()
			adminPass := state["ADMIN_PASS"]

			saveArgs := []string{"exec"}
			if adminPass != "" {
				saveArgs = append(saveArgs, "-e", "REDISCLI_AUTH")
			}
			saveArgs = append(saveArgs, "krotek-redis-server", "redis-cli", "BGSAVE")
			saveCmd := exec.Command("docker", saveArgs...)
			if adminPass != "" {
				saveCmd.Env = append(os.Environ(), "REDISCLI_AUTH="+adminPass)
			}
			out, err := saveCmd.CombinedOutput()
			if err == nil {
				tui.PrintInBox("Redis Save Complete", []string{core.GREEN + "[SUCCESS] " + core.SanitizeTerminalText(strings.TrimSpace(string(out))) + core.NC})
			} else {
				tui.PrintInBox("Redis Save Failed", []string{core.RED + "[ERROR] Could not trigger save in Redis container." + core.NC})
			}

		case strings.HasPrefix(cleanOpt, "Create Instant RDB"):
			tui.ClearScreen()
			timestamp := time.Now().Format("20060102_150405")
			backupPath := fmt.Sprintf("/root/redis_manual_backup_%s.rdb.gz", timestamp)
			_ = os.MkdirAll("/root", 0700)
			if system.StreamCommandGzipNoFollow(backupPath, exec.Command("docker", "cp", "krotek-redis-server:/data/dump.rdb", "-")) == nil {
				tui.PrintInBox("Backup Creation Complete", []string{
					core.GREEN + "[SUCCESS] Full Redis RDB snapshot exported successfully!" + core.NC,
					tui.FormatKV("Backup Archive", backupPath, core.GREEN),
				})
			} else {
				tui.PrintInBox("Backup Creation Failed", []string{core.RED + "[ERROR] Failed to export Redis RDB snapshot." + core.NC})
			}

		case strings.HasPrefix(cleanOpt, "Edit docker-compose.yml"):
			tui.ClearScreen()
			composeFile := filepath.Join(composeDir, "docker-compose.yml")
			_ = tui.RunEditor(composeFile)
			tui.ClearScreen()
			res := tui.AskYesNo("Recreate and restart Docker Compose container stack?")
			if res == 0 {
				_ = system.RunDockerCompose(composeDir, "Recreating Docker stack", false, "up", "-d")
				health.IsRunning = true
			}

		case strings.HasPrefix(cleanOpt, "Restart Redis Container") || strings.HasPrefix(cleanOpt, "Start Redis Container"):
			tui.ClearScreen()
			if exec.Command("docker", "restart", "krotek-redis-server").Run() == nil || exec.Command("docker", "start", "krotek-redis-server").Run() == nil {
				health.IsRunning = true
				tui.PrintInBox("Restart Container", []string{core.GREEN + "[SUCCESS] Docker container restarted successfully." + core.NC})
			} else {
				tui.PrintInBox("Restart Container", []string{core.RED + "[ERROR] Failed to start/restart container." + core.NC})
			}

		case strings.HasPrefix(cleanOpt, "View Container Logs"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "logs", "--tail", "50", "krotek-redis-server").CombinedOutput()
			tui.PrintInBox("Container Logs (Tail 50)", tui.FormatStyledLogLines(string(out), "No container logs found."))

		case strings.HasPrefix(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()

		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("docker")
			health = modules.DetectRedisHealth()

		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallRedis()
			return
		}
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.RedisHealth) {
	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart Redis Service"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start Redis Service  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		state := system.LoadState()
		port := state["PORT"]
		if port == "" {
			port = "6379"
		}

		confPath := "/etc/redis/redis.conf"
		if _, err := os.Stat(confPath); os.IsNotExist(err) {
			confPath = "/etc/redis.conf"
		}

		options := []string{
			"─── REDIS DASHBOARD & TOOLS ───",
			"View Application Dashboard & Connection Info",
			"Run Redis Ping & Health Check",
			"Open Interactive redis-cli Shell",
			"Save In-Memory Snapshot to Disk (SAVE)",
			"Create Instant RDB Backup Archive",
			"─── SERVICE MANAGEMENT ───",
			fmt.Sprintf("Edit redis config  (%s)", confPath),
			restartLabel,
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & Journalctl",
			"Run Deep System & Database Diagnostics",
			"─── DANGER ZONE ───",
			"Uninstall Redis Server",
			"Exit",
		}

		titleStr := fmt.Sprintf("redis-manager  -  Management (Native: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "View Application Dashboard"):
			tui.ClearScreen()
			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "Redis In-Memory Store (Native Service)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Service State", func() string {
				if health.IsRunning {
					return core.GREEN + "[ACTIVE / RUNNING]" + core.NC
				}
				return core.RED + "[STOPPED / INACTIVE]" + core.NC
			}(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost Port", fmt.Sprintf("127.0.0.1:%s", port), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Network IP", fmt.Sprintf("%s:%s", system.GetLocalIP(), port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Config Path", confPath, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Dump Path", "/var/lib/redis/dump.rdb", core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("CLI Command", "redis-manager", core.GREEN))
			tui.PrintInBox("Redis Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Run Redis Ping"):
			tui.ClearScreen()
			var healthLines []string
			adminPass := state["ADMIN_PASS"]

			pingArgs := []string{"-p", port, "PING"}
			cPing := exec.Command("redis-cli", pingArgs...)
			if adminPass != "" {
				cPing.Env = append(os.Environ(), "REDISCLI_AUTH="+adminPass)
			}
			out, err := cPing.CombinedOutput()
			if err == nil && strings.Contains(strings.ToUpper(string(out)), "PONG") {
				healthLines = append(healthLines, core.GREEN+core.BOLD+"[SUCCESS] Redis responded with PONG!"+core.NC)
				healthLines = append(healthLines, tui.FormatKV("Response", "PONG (Healthy)", core.GREEN))
			} else {
				healthLines = append(healthLines, core.RED+core.BOLD+"[WARNING] Redis ping failed!"+core.NC)
				healthLines = append(healthLines, "Ensure service is running: systemctl status redis-server")
			}
			tui.PrintInBox("Redis Health Check", healthLines)

		case strings.HasPrefix(cleanOpt, "Open Interactive redis-cli"):
			adminPass := state["ADMIN_PASS"]
			tui.LeaveAltScreen()
			tui.ClearScreen()
			fmt.Printf("%sConnecting to local Redis server...%s\n\n", core.CYAN, core.NC)
			args := []string{"-p", port}
			c := exec.Command("redis-cli", args...)
			if adminPass != "" {
				c.Env = append(os.Environ(), "REDISCLI_AUTH="+adminPass)
			}
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = c.Run()
			tui.EnterAltScreen()

		case strings.HasPrefix(cleanOpt, "Save In-Memory Snapshot"):
			tui.ClearScreen()
			adminPass := state["ADMIN_PASS"]

			saveArgs := []string{"-p", port, "BGSAVE"}
			cSave := exec.Command("redis-cli", saveArgs...)
			if adminPass != "" {
				cSave.Env = append(os.Environ(), "REDISCLI_AUTH="+adminPass)
			}
			out, err := cSave.CombinedOutput()
			if err == nil {
				tui.PrintInBox("Redis Save Complete", []string{core.GREEN + "[SUCCESS] " + strings.TrimSpace(string(out)) + core.NC})
			} else {
				tui.PrintInBox("Redis Save Failed", []string{core.RED + "[ERROR] Could not trigger save in Redis service." + core.NC})
			}

		case strings.HasPrefix(cleanOpt, "Create Instant RDB"):
			tui.ClearScreen()
			timestamp := time.Now().Format("20060102_150405")
			backupPath := fmt.Sprintf("/root/redis_manual_backup_%s.rdb.gz", timestamp)
			_ = os.MkdirAll("/root", 0700)
			if system.CompressFileGzipNoFollow("/var/lib/redis/dump.rdb", backupPath) == nil {
				tui.PrintInBox("Backup Creation Complete", []string{
					core.GREEN + "[SUCCESS] Full Redis RDB snapshot exported successfully!" + core.NC,
					tui.FormatKV("Backup Archive", backupPath, core.GREEN),
				})
			} else {
				tui.PrintInBox("Backup Creation Failed", []string{core.RED + "[ERROR] Could not locate or export /var/lib/redis/dump.rdb." + core.NC})
			}

		case strings.HasPrefix(cleanOpt, "Edit redis config"):
			tui.ClearScreen()
			_ = tui.RunEditor(confPath)
			res := tui.AskYesNo("Restart Redis service to apply changes?")
			if res == 0 {
				_ = exec.Command("systemctl", "restart", "redis-server").Run()
				_ = exec.Command("systemctl", "restart", "redis").Run()
			}

		case strings.HasPrefix(cleanOpt, "Restart Redis") || strings.HasPrefix(cleanOpt, "Start Redis"):
			tui.ClearScreen()
			_ = exec.Command("systemctl", "restart", "redis-server").Run()
			_ = exec.Command("systemctl", "restart", "redis").Run()
			health.IsRunning = true
			tui.PrintInBox("Restart Service", []string{core.GREEN + "[SUCCESS] Redis service restarted." + core.NC})

		case strings.HasPrefix(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()

		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			health = modules.DetectRedisHealth()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallRedis()
			return
		}
	}
}

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	health := modules.DetectRedisHealth()
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
			envType = "native"
		}
	}

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd == "" {
			cd = "/etc/redis-docker"
		}
		dockerManagementMenu(cd, health)
	} else {
		nativeManagementMenu(health)
	}
}
