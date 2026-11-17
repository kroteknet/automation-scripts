// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: SQLite Embedded Database Toolkit (sqlite)
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

	"krotek-sqlite-installer/core"
	"krotek-sqlite-installer/install"
	"krotek-sqlite-installer/modules"
	"krotek-sqlite-installer/system"
	"krotek-sqlite-installer/tui"
)

// noServiceDiagnosticMenu guides the user when SQLite Embedded Database Toolkit is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		tui.ClearScreen()
		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("SQLite CLI Tool", system.GetToolBadge("sqlite3"), core.YELLOW),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + "Status: SQLite3 engine is not installed on this system yet." + core.NC,
			"Choose an action below to install SQLite3 tools or run system diagnostics.",
		}

		options := []string{
			"Install SQLite Web GUI Stack (Docker) - Recommended)",
			"Install SQLite3 Native (System Package + Dev Headers)",
			"Run Deep System & Database Diagnostics Scan",
			"Exit Diagnostics",
		}

		idx, _, status := tui.SelectMenuWithContent("sqlite-manager - System Diagnostics (SQLite Not Found)", diagLines, options)
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
// for the containerized SQLite Embedded Database Toolkit Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.SQLiteHealth) {
	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart SQLite-Web Container"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start SQLite-Web Container  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		state := system.LoadState()
		webPort := state["WEB_PORT"]
		if webPort == "" {
			webPort = "8080"
		}

		options := []string{
			"─── SQLITE DASHBOARD & TOOLS ───",
			"View Application Dashboard & Connection Info",
			"Open Interactive sqlite3 Shell (CLI)",
			"─── DOCKER STACK MANAGEMENT ───",
			fmt.Sprintf("Edit docker-compose.yml  (%s/docker-compose.yml)", composeDir),
			restartLabel,
			"Update SQLite Database & Check Upgrades (Safe Backup)",
			"View Container Logs (last 50 lines)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"Run Deep System & Database Diagnostics",
			"─── DANGER ZONE ───",
			"Uninstall SQLite Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("sqlite-manager  -  Management (Docker: %s)", statusBadge)
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
			dbLines = append(dbLines, tui.FormatKV("Application", "SQLite Web GUI (Docker Stack)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Service State", func() string {
				if health.IsRunning {
					return core.GREEN + "[ACTIVE / RUNNING]" + core.NC
				}
				return core.RED + "[STOPPED / INACTIVE]" + core.NC
			}(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Web UI URL", fmt.Sprintf("http://%s:%s", system.GetLocalIP(), webPort), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost URL", fmt.Sprintf("http://127.0.0.1:%s", webPort), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("CLI Command", "sqlite-manager", core.GREEN))
			tui.PrintInBox("SQLite Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Open Interactive sqlite3"):
			tui.LeaveAltScreen()
			tui.ClearScreen()
			fmt.Printf("%sConnecting to SQLite CLI in container...%s\n\n", core.CYAN, core.NC)
			c := exec.Command("docker", "exec", "-it", "krotek-sqlite-web", "sqlite3", "/data/app.db")
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = c.Run()
			tui.EnterAltScreen()

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

		case strings.HasPrefix(cleanOpt, "Restart SQLite-Web") || strings.HasPrefix(cleanOpt, "Start SQLite-Web"):
			tui.ClearScreen()
			if exec.Command("docker", "restart", "krotek-sqlite-web").Run() == nil || exec.Command("docker", "start", "krotek-sqlite-web").Run() == nil {
				health.IsRunning = true
				tui.PrintInBox("Restart Container", []string{core.GREEN + "[SUCCESS] Container restarted successfully." + core.NC})
			} else {
				tui.PrintInBox("Restart Container", []string{core.RED + "[ERROR] Failed to start/restart container." + core.NC})
			}

		case strings.HasPrefix(cleanOpt, "View Container Logs"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "logs", "--tail", "50", "krotek-sqlite-web").CombinedOutput()
			tui.PrintInBox("Container Logs (Tail 50)", tui.FormatStyledLogLines(string(out), "No container logs found."))

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()

		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("docker")
			health = modules.DetectSQLiteHealth()

		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallSQLite()
			return
		}
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.SQLiteHealth) {
	for {
		statusBadge := core.GREEN + "[INSTALLED]" + core.NC

		state := system.LoadState()
		dataDir := state["DATA_DIR"]
		if dataDir == "" {
			dataDir = "/var/lib/sqlite"
		}
		sampleDB := state["SAMPLE_DB"]
		if sampleDB == "" {
			sampleDB = "app.db"
		}
		dbPath := filepath.Join(dataDir, sampleDB)

		options := []string{
			"─── SQLITE DASHBOARD & TOOLS ───",
			"View Application Dashboard & Storage Info",
			"Open Interactive sqlite3 Shell (CLI)",
			"Create New SQLite Database File",
			"List Databases in Storage Directory",
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"Run Deep System & Database Diagnostics",
			"─── DANGER ZONE ───",
			"Uninstall SQLite Tools",
			"Exit",
		}

		titleStr := fmt.Sprintf("sqlite-manager  -  Management (Native: %s)", statusBadge)
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
			dbLines = append(dbLines, tui.FormatKV("Application", "SQLite3 C-Engine & Tools (Native)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Engine State", core.GREEN+"[AVAILABLE / INSTALLED]"+core.NC, core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("CLI Binary Tool", "/usr/bin/sqlite3", core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Storage Directory", dataDir, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Default Database", dbPath, core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("CLI Command", "sqlite-manager", core.GREEN))
			tui.PrintInBox("SQLite Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Open Interactive sqlite3"):
			tui.LeaveAltScreen()
			tui.ClearScreen()
			fmt.Printf("%sOpening SQLite3 session for: %s%s\n\n", core.CYAN, dbPath, core.NC)
			c := exec.Command("sqlite3", dbPath)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = c.Run()
			tui.EnterAltScreen()

		case strings.HasPrefix(cleanOpt, "Create New SQLite"):
			tui.ClearScreen()
			newDBName, st := tui.ReadTextInput("Enter New Database Filename", "project.db")
			if st == 0 && newDBName != "" {
				newPath := filepath.Join(dataDir, newDBName)
				_ = exec.Command("sqlite3", newPath, "VACUUM;").Run()
				tui.PrintInBox("Database Created", []string{
					core.GREEN + "[SUCCESS] Database initialized successfully!" + core.NC,
					tui.FormatKV("Database File", newPath, core.GREEN),
				})
			}

		case strings.HasPrefix(cleanOpt, "List Databases"):
			tui.ClearScreen()
			var fileLines []string
			if files, err := filepath.Glob(filepath.Join(dataDir, "*")); err == nil && len(files) > 0 {
				for _, f := range files {
					fi, _ := os.Stat(f)
					sz := "0 B"
					if fi != nil {
						sz = fmt.Sprintf("%d KB", fi.Size()/1024)
					}
					fileLines = append(fileLines, fmt.Sprintf("  %s%s%s %-30s %s(%s)%s", core.CYAN, core.Sym.Pointer, core.NC, filepath.Base(f), core.YELLOW, sz, core.NC))
				}
			} else {
				fileLines = append(fileLines, "  (No database files found in "+dataDir+")")
			}
			tui.PrintInBox("Databases in "+dataDir, fileLines)

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()

		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			health = modules.DetectSQLiteHealth()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallSQLite()
			return
		}
	}
}

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	health := modules.DetectSQLiteHealth()
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
			cd = "/etc/sqlite-docker"
		}
		dockerManagementMenu(cd, health)
	} else {
		nativeManagementMenu(health)
	}
}
