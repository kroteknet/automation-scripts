// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: PostgreSQL Advanced Object-Relational Database (postgresql)
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

	"krotek-postgresql-installer/core"
	"krotek-postgresql-installer/install"
	"krotek-postgresql-installer/modules"
	"krotek-postgresql-installer/system"
	"krotek-postgresql-installer/tui"
)

// noServiceDiagnosticMenu guides the user when PostgreSQL Advanced Object-Relational Database is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		tui.ClearScreen()
		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("PostgreSQL Service", core.RED+"[NOT INSTALLED]"+core.NC, core.RED),
			tui.FormatKV("PostgreSQL Binary Tool", system.GetToolBadge("psql"), core.YELLOW),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + "Status: PostgreSQL database server is not provisioned on this machine yet." + core.NC,
			"Choose an action below to deploy PostgreSQL or run system network diagnostics.",
		}

		options := []string{
			"Install PostgreSQL Docker (Containerized Stack - Recommended)",
			"Install PostgreSQL Native (System Packages + Auto-Tuned)",
			"Run Deep System & Network Diagnostics Scan",
			"Exit Diagnostics",
		}

		idx, _, status := tui.SelectMenuWithContent("postgresql-manager - System Diagnostics (PostgreSQL Not Found)", diagLines, options)
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
// for the containerized PostgreSQL Advanced Object-Relational Database Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.PostgreSQLHealth) {
	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart PostgreSQL Container"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start PostgreSQL Container  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		state := system.LoadState()
		port := state["PORT"]
		if port == "" {
			port = "5432"
		}

		options := []string{
			"─── DATABASE DASHBOARD & TOOLS ───",
			"View Application Dashboard & Connection Info",
			"Run Database Health & Connection Check",
			"Open Interactive psql Shell (CLI)",
			"Create Instant SQL Database Dump Backup",
			"─── DOCKER STACK MANAGEMENT ───",
			fmt.Sprintf("Edit docker-compose.yml  (%s/docker-compose.yml)", composeDir),
			restartLabel,
			"Update PostgreSQL Server & Check Upgrades (Safe Backup)",
			"View Container Logs (last 50 lines)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & System Journal",
			"Run Deep System & Database Diagnostics",
			"─── DANGER ZONE ───",
			"Uninstall PostgreSQL Docker Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("postgresql-manager  -  Management (Docker: %s)", statusBadge)
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
			dbLines = append(dbLines, tui.FormatKV("Application", "PostgreSQL Server (Docker Stack)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Service State", func() string {
				if health.IsRunning {
					return core.GREEN + "[ACTIVE / RUNNING]" + core.NC
				}
				return core.RED + "[STOPPED / INACTIVE]" + core.NC
			}(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost Access", fmt.Sprintf("127.0.0.1:%s", port), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Network IP", fmt.Sprintf("%s:%s", system.GetLocalIP(), port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Database Port", port, core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("Superuser Role", "postgres", core.CYAN))
			if state["APP_DB"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Application DB", state["APP_DB"], core.CYAN))
				dbLines = append(dbLines, tui.FormatKV("Application Role", state["APP_USER"], core.CYAN))
			}
			dbLines = append(dbLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("CLI Command", "postgresql-manager", core.GREEN))
			tui.PrintInBox("PostgreSQL Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Run Database Health"):
			tui.ClearScreen()
			var healthLines []string
			pingCmd := "docker exec krotek-postgres pg_isready -U postgres 2>/dev/null"
			out, err := exec.Command(system.GetShell(), "-c", pingCmd).CombinedOutput()
			if err == nil && strings.Contains(strings.ToLower(string(out)), "accepting connections") {
				healthLines = append(healthLines, core.GREEN+core.BOLD+"[SUCCESS] PostgreSQL server daemon is accepting connections!"+core.NC)
				healthLines = append(healthLines, tui.FormatKV("pg_isready", strings.TrimSpace(string(out)), core.GREEN))
			} else {
				healthLines = append(healthLines, core.RED+core.BOLD+"[WARNING] PostgreSQL connection check failed!"+core.NC)
				healthLines = append(healthLines, "Ensure container is running.")
			}
			tui.PrintInBox("PostgreSQL Health Check", healthLines)

		case strings.HasPrefix(cleanOpt, "Open Interactive psql Shell"):
			tui.LeaveAltScreen()
			tui.ClearScreen()
			fmt.Printf("%sConnecting to PostgreSQL shell in container 'krotek-postgres'...%s\n\n", core.CYAN, core.NC)
			c := exec.Command("docker", "exec", "-it", "krotek-postgres", "psql", "-U", "postgres")
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = c.Run()
			tui.EnterAltScreen()

		case strings.HasPrefix(cleanOpt, "Create Instant SQL"):
			tui.ClearScreen()
			timestamp := time.Now().Format("20060102_150405")
			backupPath := fmt.Sprintf("/root/postgres_manual_dump_%s.sql.gz", timestamp)
			_ = os.MkdirAll("/root", 0700)
			if system.StreamCommandGzipNoFollow(backupPath, exec.Command("docker", "exec", "krotek-postgres", "pg_dumpall", "-U", "postgres")) == nil {
				tui.PrintInBox("Backup Creation Complete", []string{
					core.GREEN + "[SUCCESS] Full cluster backup exported successfully!" + core.NC,
					tui.FormatKV("Backup Archive", backupPath, core.GREEN),
				})
			} else {
				tui.PrintInBox("Backup Creation Failed", []string{core.RED + "[ERROR] Failed to export PostgreSQL cluster dump." + core.NC})
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

		case strings.HasPrefix(cleanOpt, "Restart PostgreSQL Container") || strings.HasPrefix(cleanOpt, "Start PostgreSQL Container"):
			tui.ClearScreen()
			if exec.Command("docker", "restart", "krotek-postgres").Run() == nil || exec.Command("docker", "start", "krotek-postgres").Run() == nil {
				health.IsRunning = true
				tui.PrintInBox("Restart Container", []string{core.GREEN + "[SUCCESS] Docker container restarted successfully." + core.NC})
			} else {
				tui.PrintInBox("Restart Container", []string{core.RED + "[ERROR] Failed to start/restart container." + core.NC})
			}

		case strings.HasPrefix(cleanOpt, "View Container Logs"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "logs", "--tail", "50", "krotek-postgres").CombinedOutput()
			tui.PrintInBox("Container Logs (Tail 50)", tui.FormatStyledLogLines(string(out), "No container logs found."))

		case strings.HasPrefix(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()

		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("docker")
			health = modules.DetectPostgreSQLHealth()

		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallPostgreSQL()
			return
		}
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.PostgreSQLHealth) {
	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart PostgreSQL Service"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start PostgreSQL Service  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		state := system.LoadState()
		port := state["PORT"]
		if port == "" {
			port = "5432"
		}

		options := []string{
			"─── DATABASE DASHBOARD & TOOLS ───",
			"View Application Dashboard & Connection Info",
			"Run Database Health & Socket Check",
			"Open Interactive psql Shell (CLI)",
			"Create Instant SQL Database Dump Backup",
			"─── SERVICE MANAGEMENT ───",
			restartLabel,
			"Reload Configuration (pg_ctl reload)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & Journalctl",
			"Run Deep System & Database Diagnostics",
			"─── DANGER ZONE ───",
			"Uninstall PostgreSQL Server",
			"Exit",
		}

		titleStr := fmt.Sprintf("postgresql-manager  -  Management (Native: %s)", statusBadge)
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
			dbLines = append(dbLines, tui.FormatKV("Application", "PostgreSQL Server (Native Service)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Service State", func() string {
				if health.IsRunning {
					return core.GREEN + "[ACTIVE / RUNNING]" + core.NC
				}
				return core.RED + "[STOPPED / INACTIVE]" + core.NC
			}(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost Port", fmt.Sprintf("127.0.0.1:%s", port), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Network IP", fmt.Sprintf("%s:%s", system.GetLocalIP(), port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Data Root", "/var/lib/postgresql", core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Superuser Role", "postgres", core.CYAN))
			if state["APP_DB"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Application DB", state["APP_DB"], core.CYAN))
				dbLines = append(dbLines, tui.FormatKV("Application Role", state["APP_USER"], core.CYAN))
			}
			dbLines = append(dbLines, tui.FormatKV("CLI Command", "postgresql-manager", core.GREEN))
			tui.PrintInBox("PostgreSQL Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Run Database Health"):
			tui.ClearScreen()
			var healthLines []string
			out, err := exec.Command(system.GetShell(), "-c", "pg_isready 2>/dev/null || su -s /bin/sh postgres -c 'pg_isready' 2>/dev/null").CombinedOutput()
			if err == nil && strings.Contains(strings.ToLower(string(out)), "accepting connections") {
				healthLines = append(healthLines, core.GREEN+core.BOLD+"[SUCCESS] PostgreSQL daemon is active and accepting connections!"+core.NC)
				healthLines = append(healthLines, tui.FormatKV("pg_isready", strings.TrimSpace(string(out)), core.GREEN))
			} else {
				healthLines = append(healthLines, core.RED+core.BOLD+"[WARNING] PostgreSQL connection check failed!"+core.NC)
				if core.SysInfo.Family == "alpine" || core.SysInfo.InitSystem == "openrc" {
					healthLines = append(healthLines, "Ensure service is active: rc-service postgresql status")
				} else {
					healthLines = append(healthLines, "Ensure service is active: systemctl status postgresql")
				}
			}
			tui.PrintInBox("PostgreSQL Socket Health Check", healthLines)

		case strings.HasPrefix(cleanOpt, "Open Interactive psql Shell"):
			tui.LeaveAltScreen()
			tui.ClearScreen()
			fmt.Printf("%sConnecting to local PostgreSQL server as 'postgres'...%s\n\n", core.CYAN, core.NC)
			c := exec.Command("su", "-s", "/bin/sh", "postgres", "-c", "psql")
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = c.Run()
			tui.EnterAltScreen()

		case strings.HasPrefix(cleanOpt, "Create Instant SQL"):
			tui.ClearScreen()
			timestamp := time.Now().Format("20060102_150405")
			backupPath := fmt.Sprintf("/root/postgres_manual_dump_%s.sql.gz", timestamp)
			_ = os.MkdirAll("/root", 0700)
			if system.StreamCommandGzipNoFollow(backupPath, exec.Command("su", "-s", "/bin/sh", "postgres", "-c", "pg_dumpall")) == nil {
				tui.PrintInBox("Backup Creation Complete", []string{
					core.GREEN + "[SUCCESS] Full cluster backup exported successfully!" + core.NC,
					tui.FormatKV("Backup Archive", backupPath, core.GREEN),
				})
			} else {
				tui.PrintInBox("Backup Creation Failed", []string{core.RED + "[ERROR] Failed to export PostgreSQL cluster dump." + core.NC})
			}

		case strings.HasPrefix(cleanOpt, "Restart PostgreSQL") || strings.HasPrefix(cleanOpt, "Start PostgreSQL"):
			tui.ClearScreen()
			system.RestartService("postgresql")
			time.Sleep(1 * time.Second)
			health = modules.DetectPostgreSQLHealth()
			if health.IsRunning {
				tui.PrintInBox("Restart Service", []string{core.GREEN + "[SUCCESS] PostgreSQL service started / restarted successfully." + core.NC})
			} else {
				tui.PrintInBox("Restart Service", []string{core.RED + "[ERROR] PostgreSQL service failed to start. Run setup wizard or check logs." + core.NC})
			}

		case strings.HasPrefix(cleanOpt, "Reload Configuration"):
			tui.ClearScreen()
			system.ReloadService("postgresql")
			tui.PrintInBox("Reload Config", []string{core.GREEN + "[SUCCESS] Configuration reloaded." + core.NC})

		case strings.HasPrefix(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()

		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			health = modules.DetectPostgreSQLHealth()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallPostgreSQL()
			return
		}
	}
}

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	health := modules.DetectPostgreSQLHealth()
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
			cd = "/etc/postgresql-docker"
		}
		dockerManagementMenu(cd, health)
	} else {
		nativeManagementMenu(health)
	}
}
