// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: MongoDB NoSQL Document Database (mongodb)
//
// This module provides the interactive terminal menus, status overview screens,
// operations actions (start, stop, reload), live log viewers, and uninstaller.
// ==============================================================================

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"krotek-mongodb-installer/core"
	"krotek-mongodb-installer/install"
	"krotek-mongodb-installer/modules"
	"krotek-mongodb-installer/system"
	"krotek-mongodb-installer/tui"
)

// noServiceDiagnosticMenu guides the user when MongoDB NoSQL Document Database is not yet installed or stopped,
// offering instant setup wizards and port conflict checks.
func noServiceDiagnosticMenu() {
	for {
		tui.ClearScreen()
		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("MongoDB Service", core.RED+"[NOT INSTALLED]"+core.NC, core.RED),
			tui.FormatKV("MongoDB Binary Tool", system.GetToolBadge("mongod"), core.YELLOW),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + "Status: MongoDB database is not provisioned on this machine yet." + core.NC,
			"Choose an action below to deploy MongoDB or run system network diagnostics.",
		}

		// Docker is always listed first as the full server option.
		// Native option installs client tools only (mongodump, mongorestore, mongosh etc.).
		dockerOption := core.BOLD + core.GREEN + "[RECOMMENDED]" + core.NC + " Install MongoDB via Docker Container Stack" +
			"  " + core.CYAN + "(Full official MongoDB server" + " " + core.Sym.EmDash + " " + "works on all distros)" + core.NC

		var nativeToolsOpt string
		if core.SysInfo.Family == "alpine" {
			nativeToolsOpt = core.YELLOW + "[CLIENT TOOLS ONLY]" + core.NC + " Install MongoDB Native Client Tools" +
				"  " + core.RED + "(Alpine: server not available" + " " + core.Sym.EmDash + " " + "installs mongodump, mongorestore, mongosh etc.)" + core.NC
		} else {
			nativeToolsOpt = "[CLIENT TOOLS ONLY] Install MongoDB Native Client Tools" +
				"  " + core.YELLOW + "(mongodump, mongorestore, mongoimport, mongoexport, mongosh)" + core.NC
		}

		options := []string{
			dockerOption,
			nativeToolsOpt,
			"Run Deep System & Network Diagnostics Scan",
			"Exit Diagnostics",
		}

		idx, _, status := tui.SelectMenuWithContent("mongodb-manager - System Diagnostics (MongoDB Not Found)", diagLines, options)
		if status == 2 || (status == 0 && idx == 3) {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		switch idx {
		case 0:
			install.FreshInstallWizard("docker")
			return
		case 1:
			install.ExecuteNativeToolsInstall()
			return
		case 2:
			modules.RunDeepDiagnostics()
		}
	}
}

// dockerManagementMenu provides container lifecycle controls (start/stop/restart/logs)
// for the containerized MongoDB NoSQL Document Database Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.MongoDBHealth) {
	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart MongoDB Container"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start MongoDB Container  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		state := system.LoadState()
		port := state["PORT"]
		if port == "" {
			port = "27017"
		}

		options := []string{
			"─── MONGODB DASHBOARD & TOOLS ───",
			"View Application Dashboard & Connection Info",
			"Run MongoDB Connection & Ping Check",
			"Open Interactive mongosh Shell (CLI)",
			"Create Instant Database Dump Backup",
			"─── DOCKER STACK MANAGEMENT ───",
			fmt.Sprintf("Edit docker-compose.yml  (%s/docker-compose.yml)", composeDir),
			restartLabel,
			"Update MongoDB Database & Check Upgrades (Safe Backup)",
			"View Container Logs (last 50 lines)",
			"Fix MongoDB Image (auto-detect CPU-compatible version)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & System Journal",
			"Run Deep System & Database Diagnostics",
			"─── DANGER ZONE ───",
			"Uninstall MongoDB Docker Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("mongodb-manager  -  Management (Docker: %s)", statusBadge)
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
			dbLines = append(dbLines, tui.FormatKV("Application", "MongoDB NoSQL Database (Docker Stack)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Service State", func() string {
				if health.IsRunning {
					return core.GREEN + "[ACTIVE / RUNNING]" + core.NC
				}
				return core.RED + "[STOPPED / INACTIVE]" + core.NC
			}(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost URI", fmt.Sprintf("mongodb://127.0.0.1:%s", port), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Network IP", fmt.Sprintf("%s:%s", system.GetLocalIP(), port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Port", port, core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("Admin User", state["ADMIN_USER"], core.CYAN))
			if state["APP_DB"] != "" {
				dbLines = append(dbLines, tui.FormatKV("App Database", state["APP_DB"], core.CYAN))
				dbLines = append(dbLines, tui.FormatKV("App User", state["APP_USER"], core.CYAN))
			}
			dbLines = append(dbLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("CLI Command", "mongodb-manager", core.GREEN))
			tui.PrintInBox("MongoDB Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Run MongoDB Connection"):
			tui.ClearScreen()
			var healthLines []string
			adminUser := state["ADMIN_USER"]
			if adminUser == "" {
				adminUser = "admin"
			}
			adminPass := state["ADMIN_PASS"]
			pingArgs := []string{"exec"}
			if adminPass != "" {
				pingArgs = append(pingArgs, "-e", "KROTEK_MONGO_USER", "-e", "KROTEK_MONGO_PASS")
			}
			pingArgs = append(pingArgs, "krotek-mongod", "mongosh", "--quiet", "--eval",
				"if(process.env.KROTEK_MONGO_PASS){db.getSiblingDB('admin').auth(process.env.KROTEK_MONGO_USER, process.env.KROTEK_MONGO_PASS);} printjson(db.runCommand({ ping: 1 }));")
			pingCmd := exec.Command("docker", pingArgs...)
			if adminPass != "" {
				pingCmd.Env = append(os.Environ(), "KROTEK_MONGO_USER="+adminUser, "KROTEK_MONGO_PASS="+adminPass)
			}
			out, err := pingCmd.CombinedOutput()
			if err == nil && strings.Contains(strings.ToLower(string(out)), "ok") {
				healthLines = append(healthLines, core.GREEN+core.BOLD+"[SUCCESS] MongoDB responded with ping OK!"+core.NC)
				healthLines = append(healthLines, tui.FormatKV("Ping Command", "ok: 1 (Responsive)", core.GREEN))
			} else {
				healthLines = append(healthLines, core.RED+core.BOLD+"[WARNING] MongoDB connection ping failed!"+core.NC)
				healthLines = append(healthLines, "Ensure container is running and credentials are valid.")
			}
			tui.PrintInBox("MongoDB Health Check", healthLines)

		case strings.HasPrefix(cleanOpt, "Open Interactive mongosh"):
			adminUser := state["ADMIN_USER"]
			if adminUser == "" {
				adminUser = "admin"
			}
			adminPass := state["ADMIN_PASS"]
			tui.LeaveAltScreen()
			tui.ClearScreen()
			fmt.Printf("%sConnecting to MongoDB shell in container 'krotek-mongod'...%s\n", core.CYAN, core.NC)
			args := []string{"exec", "-it", "krotek-mongod", "mongosh"}
			if adminPass != "" {
				fmt.Printf("%sEnter password when prompted by mongosh (credentials omitted from process argv).%s\n\n", core.YELLOW, core.NC)
				args = append(args, "-u", adminUser, "--authenticationDatabase", "admin")
			} else {
				fmt.Println()
			}
			c := exec.Command("docker", args...)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = c.Run()
			tui.EnterAltScreen()

		case strings.HasPrefix(cleanOpt, "Create Instant Database"):
			tui.ClearScreen()
			timestamp := time.Now().Format("20060102_150405")
			backupPath := fmt.Sprintf("/root/mongodb_manual_backup_%s.gz", timestamp)
			_ = os.MkdirAll("/root", 0700)
			adminUser := state["ADMIN_USER"]
			if adminUser == "" {
				adminUser = "admin"
			}
			adminPass := state["ADMIN_PASS"]
			var dumpCmd *exec.Cmd
			if adminPass != "" {
				passJSON, jErr := json.Marshal(adminPass)
				if jErr == nil {
					dumpCmd = exec.Command("docker", "exec", "-i", "krotek-mongod", "sh", "-c",
						`umask 077; cfg=$(mktemp) && trap 'rm -f "$cfg"' EXIT && cat > "$cfg" && exec mongodump -u "$1" --authenticationDatabase admin --config "$cfg" --archive`,
						"_", adminUser)
					dumpCmd.Stdin = bytes.NewReader([]byte("password: " + string(passJSON) + "\n"))
				}
			} else {
				dumpCmd = exec.Command("docker", "exec", "krotek-mongod", "mongodump", "--archive")
			}
			if dumpCmd != nil && system.StreamCommandGzipNoFollow(backupPath, dumpCmd) == nil {
				tui.PrintInBox("Backup Creation Complete", []string{
					core.GREEN + "[SUCCESS] Full MongoDB backup archive exported successfully!" + core.NC,
					tui.FormatKV("Backup Archive", backupPath, core.GREEN),
				})
			} else {
				tui.PrintInBox("Backup Creation Failed", []string{core.RED + "[ERROR] Failed to export MongoDB archive." + core.NC})
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

		case strings.HasPrefix(cleanOpt, "Restart MongoDB Container") || strings.HasPrefix(cleanOpt, "Start MongoDB Container"):
			tui.ClearScreen()
			if exec.Command("docker", "restart", "krotek-mongod").Run() == nil || exec.Command("docker", "start", "krotek-mongod").Run() == nil {
				health.IsRunning = true
				tui.PrintInBox("Restart Container", []string{core.GREEN + "[SUCCESS] Docker container restarted successfully." + core.NC})
			} else {
				tui.PrintInBox("Restart Container", []string{core.RED + "[ERROR] Failed to start/restart container." + core.NC})
			}

		case strings.HasPrefix(cleanOpt, "View Container Logs"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "logs", "--tail", "50", "krotek-mongod").CombinedOutput()
			tui.PrintInBox("Container Logs (Tail 50)", tui.FormatStyledLogLines(string(out), "No container logs found."))

		case strings.HasPrefix(cleanOpt, "Fix MongoDB Image"):
			tui.ClearScreen()
			newImage := install.SelectMongoImage()
			composeFile := filepath.Join(composeDir, "docker-compose.yml")
			fixLines := []string{
				core.BOLD + core.CYAN + "Detected CPU-compatible MongoDB Image:" + core.NC,
				"  " + core.GREEN + core.BOLD + newImage + core.NC,
				"",
				"  This will update docker-compose.yml and recreate the container.",
				"  Your data volume (krotek_mongodb_data) will be preserved.",
			}
			tui.PrintInBox("--no-wait", "Fix MongoDB Image", fixLines)
			if tui.AskYesNo("Update docker-compose.yml to "+newImage+" and restart?") == 0 {
				// Read, replace image line, write back
				if data, err := os.ReadFile(composeFile); err == nil {
					updated := regexp.MustCompile(`image:\s*mongo:[\w.]+`).ReplaceAllString(string(data), "image: "+newImage)
					_ = system.WriteFileNoFollow(composeFile, []byte(updated), 0600, "")
				}
				// Pull new image and recreate
				_ = system.RunStep("Pulling "+newImage, "docker", "pull", newImage)
				if system.RunDockerCompose(composeDir, "Recreating MongoDB container with "+newImage, false, "up", "-d", "--force-recreate") {
					health = modules.DetectMongoDBHealth()
					tui.PrintInBox("Image Fix Complete", []string{
						core.GREEN + "[SUCCESS] MongoDB container recreated with " + newImage + core.NC,
						tui.FormatKV("New Image", newImage, core.GREEN),
					})
				}
			}

		case strings.HasPrefix(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()

		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("docker")
			health = modules.DetectMongoDBHealth()

		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallMongoDB()
			return
		}
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.MongoDBHealth) {
	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart MongoDB Service"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start MongoDB Service  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		state := system.LoadState()
		port := state["PORT"]
		if port == "" {
			port = "27017"
		}

		confPath := "/etc/mongod.conf"
		if _, err := os.Stat(confPath); os.IsNotExist(err) {
			confPath = "/etc/mongodb.conf"
		}

		options := []string{
			"─── MONGODB DASHBOARD & TOOLS ───",
			"View Application Dashboard & Connection Info",
			"Run MongoDB Connection & Socket Check",
			"Open Interactive mongosh Shell (CLI)",
			"Create Instant Database Dump Backup",
			"─── SERVICE MANAGEMENT ───",
			fmt.Sprintf("Edit mongod config  (%s)", confPath),
			restartLabel,
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & Journalctl",
			"Run Deep System & Database Diagnostics",
			"─── DANGER ZONE ───",
			"Uninstall MongoDB Server",
			"Exit",
		}

		titleStr := fmt.Sprintf("mongodb-manager  -  Management (Native: %s)", statusBadge)
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
			dbLines = append(dbLines, tui.FormatKV("Application", "MongoDB NoSQL Database (Native Service)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Service State", func() string {
				if health.IsRunning {
					return core.GREEN + "[ACTIVE / RUNNING]" + core.NC
				}
				return core.RED + "[STOPPED / INACTIVE]" + core.NC
			}(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Localhost URI", fmt.Sprintf("mongodb://127.0.0.1:%s", port), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Network IP", fmt.Sprintf("%s:%s", system.GetLocalIP(), port), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("Config Path", confPath, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Data Storage", "/var/lib/mongodb", core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Admin User", state["ADMIN_USER"], core.CYAN))
			if state["APP_DB"] != "" {
				dbLines = append(dbLines, tui.FormatKV("App Database", state["APP_DB"], core.CYAN))
				dbLines = append(dbLines, tui.FormatKV("App User", state["APP_USER"], core.CYAN))
			}
			dbLines = append(dbLines, tui.FormatKV("CLI Command", "mongodb-manager", core.GREEN))
			tui.PrintInBox("MongoDB Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Run MongoDB Connection"):
			tui.ClearScreen()
			var healthLines []string
			cmd := exec.Command("mongosh", "--port", port, "--eval", "db.runCommand({ ping: 1 })")
			out, err := cmd.CombinedOutput()
			if err == nil && strings.Contains(strings.ToLower(string(out)), "ok") {
				healthLines = append(healthLines, core.GREEN+core.BOLD+"[SUCCESS] MongoDB daemon is active and responsive!"+core.NC)
				healthLines = append(healthLines, tui.FormatKV("Ping Command", "ok: 1 (Responsive)", core.GREEN))
			} else {
				healthLines = append(healthLines, core.RED+core.BOLD+"[WARNING] MongoDB ping check failed!"+core.NC)
				healthLines = append(healthLines, "Ensure service is running: systemctl status mongod")
			}
			tui.PrintInBox("MongoDB Socket Health Check", healthLines)

		case strings.HasPrefix(cleanOpt, "Open Interactive mongosh"):
			adminUser := state["ADMIN_USER"]
			if adminUser == "" {
				adminUser = "admin"
			}
			adminPass := state["ADMIN_PASS"]
			tui.LeaveAltScreen()
			tui.ClearScreen()
			fmt.Printf("%sConnecting to local MongoDB server...%s\n\n", core.CYAN, core.NC)
			var mArgs []string
			mArgs = append(mArgs, "--port", port)
			if adminPass != "" {
				fmt.Printf("%sEnter password when prompted by mongosh (credentials omitted from process argv).%s\n\n", core.YELLOW, core.NC)
				mArgs = append(mArgs, "-u", adminUser, "--authenticationDatabase", "admin")
			}
			c := exec.Command("mongosh", mArgs...)
			if _, err := exec.LookPath("mongosh"); err != nil {
				c = exec.Command("mongo", mArgs...)
			}
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = c.Run()
			tui.EnterAltScreen()

		case strings.HasPrefix(cleanOpt, "Create Instant Database"):
			tui.ClearScreen()
			timestamp := time.Now().Format("20060102_150405")
			backupPath := fmt.Sprintf("/root/mongodb_manual_backup_%s.gz", timestamp)
			_ = os.MkdirAll("/root", 0700)
			adminUser := state["ADMIN_USER"]
			if adminUser == "" {
				adminUser = "admin"
			}
			adminPass := state["ADMIN_PASS"]
			exported := false
			if adminPass != "" {
				if tmpDir, cfgPath, cErr := system.BuildMongoDumpConfig(adminPass); cErr == nil {
					dumpCmd := exec.Command("mongodump", "--port", port, "-u", adminUser, "--authenticationDatabase", "admin", "--config", cfgPath, "--archive")
					if system.StreamCommandGzipNoFollow(backupPath, dumpCmd) == nil {
						exported = true
					}
					_ = os.RemoveAll(tmpDir)
				}
			} else {
				if system.StreamCommandGzipNoFollow(backupPath, exec.Command("mongodump", "--port", port, "--archive")) == nil {
					exported = true
				}
			}
			if exported {
				tui.PrintInBox("Backup Creation Complete", []string{
					core.GREEN + "[SUCCESS] Full MongoDB archive exported successfully!" + core.NC,
					tui.FormatKV("Backup Archive", backupPath, core.GREEN),
				})
			} else {
				tui.PrintInBox("Backup Creation Failed", []string{core.RED + "[ERROR] Failed to export MongoDB archive." + core.NC})
			}

		case strings.HasPrefix(cleanOpt, "Edit mongod config"):
			tui.ClearScreen()
			_ = tui.RunEditor(confPath)
			res := tui.AskYesNo("Restart MongoDB service to apply changes?")
			if res == 0 {
				_ = system.RestartService("mongod")
				_ = system.RestartService("mongodb")
			}

		case strings.HasPrefix(cleanOpt, "Restart MongoDB") || strings.HasPrefix(cleanOpt, "Start MongoDB"):
			tui.ClearScreen()
			_ = system.RestartService("mongod")
			_ = system.RestartService("mongodb")
			health.IsRunning = true
			tui.PrintInBox("Restart Service", []string{core.GREEN + "[SUCCESS] MongoDB service restarted." + core.NC})

		case strings.HasPrefix(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()

		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			health = modules.DetectMongoDBHealth()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallMongoDB()
			return
		}
	}
}

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	health := modules.DetectMongoDBHealth()
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
			cd = "/etc/mongodb-docker"
		}
		dockerManagementMenu(cd, health)
	} else {
		nativeManagementMenu(health)
	}
}
