// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: Nitrox Subnautica Multiplayer Server (nitrox)
//
// This module provides the interactive terminal menus, status overview screens,
// operations actions (start, stop, reload), live log viewers, server.cfg editor,
// game files validator, world backup manager, and uninstaller.
// ==============================================================================

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"krotek-nitrox-installer/core"
	"krotek-nitrox-installer/install"
	"krotek-nitrox-installer/modules"
	"krotek-nitrox-installer/system"
	"krotek-nitrox-installer/tui"
)

func getServiceStatusBadge() string {
	cmd := exec.Command("systemctl", "is-active", "nitrox")
	if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) == "active" {
		return core.GREEN + "[ACTIVE]" + core.NC
	}
	if exec.Command("pidof", "Nitrox.Server.Subnautica").Run() == nil || exec.Command("pidof", "nitrox").Run() == nil {
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

func showServerStatusDetails(state map[string]string) {
	tui.ClearScreen()
	h := modules.DetectNitroxHealth()

	serverName := state["SERVER_NAME"]
	if serverName == "" {
		serverName = "Subnautica Nitrox Server"
	}
	port := state["PORT"]
	if port == "" {
		port = "11000"
	}
	maxPlayers := state["MAX_PLAYERS"]
	if maxPlayers == "" {
		maxPlayers = "8"
	}
	gameMode := state["GAME_MODE"]
	if gameMode == "" {
		gameMode = "SURVIVAL"
	}
	saveName := state["SAVE_NAME"]
	if saveName == "" {
		saveName = "world"
	}
	installDir := state["INSTALL_DIR"]
	if installDir == "" {
		if _, err := os.Stat("/srv/nitrox"); err == nil {
			installDir = "/srv/nitrox"
		} else {
			installDir = "/opt/nitrox"
		}
	}
	gameDir := state["GAME_PATH"]
	if gameDir == "" {
		gameDir = filepath.Join(installDir, "game")
	}

	statusBadge := core.RED + "[STOPPED]" + core.NC
	if h.IsRunning {
		statusBadge = core.GREEN + "[RUNNING / ACTIVE]" + core.NC
	}

	gameFilesBadge := core.YELLOW + "[PENDING] Subnautica_Data not found" + core.NC
	if h.GameFilesFound {
		gameFilesBadge = core.GREEN + core.TagOK() + " Subnautica_Data assets verified" + core.NC
	}

	pwDisplay := "Disabled (Open / Public Server)"
	if pw := state["SERVER_PASSWORD"]; pw != "" {
		pwDisplay = pw
	}
	adminPw := state["ADMIN_PASSWORD"]
	if adminPw == "" {
		adminPw = "Configured in server.cfg"
	}

	cardLines := []string{
		core.BOLD + core.CYAN + "SUBNAUTICA NITROX SERVER TELEMETRY" + core.NC,
		"",
		tui.FormatKV("Service Status", statusBadge, core.BOLD),
		tui.FormatKV("Deployment Stack", strings.ToUpper(h.EnvType), core.CYAN),
		tui.FormatKV("Server Name", serverName, core.BOLD+core.WHITE),
		tui.FormatKV("UDP Port", port+" (11000 UDP default)", core.CYAN),
		tui.FormatKV("Game Mode", gameMode, core.YELLOW),
		tui.FormatKV("Max Capacity", maxPlayers+" players", core.CYAN),
		tui.FormatKV("Join Password", pwDisplay, core.DIM),
		tui.FormatKV("Admin Password", adminPw, core.CYAN),
		tui.FormatKV("Active Save Name", saveName, core.WHITE),
		tui.FormatKV("Game Assets", gameFilesBadge, core.BOLD),
		tui.FormatKV("Game Directory", gameDir, core.DIM),
		tui.FormatKV("Installation Dir", installDir, core.DIM),
	}

	tui.PrintInBox("Nitrox Server Telemetry", cardLines)
}

func showGameFilesCheckGuide(state map[string]string) {
	tui.ClearScreen()
	h := modules.DetectNitroxHealth()

	installDir := state["INSTALL_DIR"]
	if installDir == "" {
		if _, err := os.Stat("/srv/nitrox"); err == nil {
			installDir = "/srv/nitrox"
		} else {
			installDir = "/opt/nitrox"
		}
	}
	gameDir := state["GAME_PATH"]
	if gameDir == "" {
		gameDir = filepath.Join(installDir, "game")
	}

	var statusLines []string
	if h.GameFilesFound {
		statusLines = append(statusLines,
			core.BOLD+core.GREEN+core.TagOK() + " Subnautica Game Files DETECTED & VERIFIED!"+core.NC,
			"",
			"  The Subnautica game data directory is properly installed at:",
			"  "+core.BOLD+gameDir+core.NC,
			"",
			"  Your Nitrox multiplayer server has full access to the required game assets,",
			"  physics calculations, vehicles (Seamoth, Prawn, Cyclops), and world entities.",
		)
	} else {
		statusLines = append(statusLines,
			core.BOLD+core.YELLOW+"[!] Subnautica Game Files MISSING from Game Directory"+core.NC,
			"",
			"  Nitrox Dedicated Server requires the base Subnautica game files to simulate the ocean.",
			"  Target directory: "+core.BOLD+gameDir+core.NC,
			"",
			core.BOLD+"  HOW TO UPLOAD YOUR GAME FILES TO THIS SERVER:"+core.NC,
			"",
			"  Option A: Windows PowerShell (Copy & run in your local PC PowerShell):",
			"    "+core.CYAN+`scp -r "C:\Program Files (x86)\Steam\steamapps\common\Subnautica\*" root@<SERVER_IP>:`+gameDir+`/`+core.NC,
			"",
			"  Option B: SFTP Client (WinSCP / FileZilla / Cyberduck):",
			"    Connect to this server port 22 as root and upload your entire local Subnautica",
			"    game folder into: "+gameDir+`/`,
			"",
			"  Option C: Linux / Mac terminal via rsync:",
			"    "+core.CYAN+`rsync -avzP /path/to/Subnautica/ root@<SERVER_IP>:`+gameDir+`/`+core.NC,
			"",
			"  After upload completes, run ownership fix:",
			"    "+core.YELLOW+`sudo chown -R nitrox:nitrox `+gameDir+core.NC,
		)
	}

	tui.PrintInBox("Subnautica Game Files Inspector & Guide", statusLines)
}

func editServerConfiguration(state map[string]string) {
	installDir := state["INSTALL_DIR"]
	if installDir == "" {
		if _, err := os.Stat("/srv/nitrox"); err == nil {
			installDir = "/srv/nitrox"
		} else {
			installDir = "/opt/nitrox"
		}
	}
	dataDir := state["DATA_PATH"]
	if dataDir == "" {
		dataDir = filepath.Join(installDir, "data")
	}
	saveName := state["SAVE_NAME"]
	if saveName == "" {
		saveName = "world"
	}

	mainCfg := filepath.Join(dataDir, "server.cfg")
	saveCfg := filepath.Join(dataDir, "saves", saveName, "server.cfg")

	for {
		tui.ClearScreen()
		curName := state["SERVER_NAME"]
		if curName == "" {
			curName = "Subnautica Nitrox Server"
		}
		curPort := state["PORT"]
		if curPort == "" {
			curPort = "11000"
		}
		curMax := state["MAX_PLAYERS"]
		if curMax == "" {
			curMax = "8"
		}
		pwDisplay := "None (Public Server)"
		if pw := state["SERVER_PASSWORD"]; pw != "" {
			pwDisplay = pw
		}
		admPw := state["ADMIN_PASSWORD"]
		if admPw == "" {
			admPw = "SecretAdminToken123"
		}
		curMode := state["GAME_MODE"]
		if curMode == "" {
			curMode = "SURVIVAL"
		}

		options := []string{
			"─── MULTIPLAYER & NETWORKING ───",
			fmt.Sprintf("Change Server Display Name [Current: %s]", curName),
			fmt.Sprintf("Change UDP Network Port [Current: %s/UDP]", curPort),
			fmt.Sprintf("Change Max Concurrent Players [Current: %s slots]", curMax),
			"─── SECURITY & ACCESS CONTROL ───",
			fmt.Sprintf("Change Server Join Password [Current: %s]", pwDisplay),
			fmt.Sprintf("Change Admin Secret Password [Current: %s]", admPw),
			"─── GAMEPLAY RULES ───",
			fmt.Sprintf("Change World Game Mode [Current: %s]", curMode),
			"─── CONFIGURATION INSPECTION ───",
			"View Raw server.cfg Content",
			"─── NAVIGATION ───",
			"<- Back to Main Management Menu",
		}

		_, opt, status := tui.SelectMenu("KROTEK NITROX" + " " + core.Sym.EmDash + " " + "Server Configuration (server.cfg)", options)
		if status == 2 || (status == 0 && strings.Contains(opt, "Back")) {
			return
		}

		cleanOpt := core.StripANSI(opt)
		switch {
		case strings.Contains(cleanOpt, "Change Server Display Name"):
			newVal, st := tui.ReadTextInput("Enter New Server Display Name", curName)
			if st == 0 && strings.TrimSpace(newVal) != "" {
				state["SERVER_NAME"] = strings.TrimSpace(newVal)
				saveServerConfig(mainCfg, saveCfg, state)
			}
		case strings.Contains(cleanOpt, "Change UDP Network Port"):
			newPort, st := tui.ReadTextInput("Enter New UDP Port (1-65535)", curPort)
			if st == 0 && strings.TrimSpace(newPort) != "" {
				pNum, err := strconv.Atoi(strings.TrimSpace(newPort))
				if err == nil && pNum >= 1 && pNum <= 65535 {
					state["PORT"] = strings.TrimSpace(newPort)
					system.ConfigureFirewallRules(state["PORT"] + "/udp")
					saveServerConfig(mainCfg, saveCfg, state)
				}
			}
		case strings.Contains(cleanOpt, "Change Max Concurrent Players"):
			newMax, st := tui.ReadTextInput("Enter Max Concurrent Players (1-100)", curMax)
			if st == 0 && strings.TrimSpace(newMax) != "" {
				state["MAX_PLAYERS"] = strings.TrimSpace(newMax)
				saveServerConfig(mainCfg, saveCfg, state)
			}
		case strings.Contains(cleanOpt, "Change Server Join Password"):
			newPass, st := tui.ReadPasswordInput("Enter New Server Join Password (Enter for none / public)")
			if st == 0 {
				state["SERVER_PASSWORD"] = strings.TrimSpace(newPass)
				saveServerConfig(mainCfg, saveCfg, state)
			}
		case strings.Contains(cleanOpt, "Change Admin Secret Password"):
			newAdm, st := tui.ReadPasswordInput("Enter New Admin Password")
			if st == 0 && strings.TrimSpace(newAdm) != "" {
				state["ADMIN_PASSWORD"] = strings.TrimSpace(newAdm)
				saveServerConfig(mainCfg, saveCfg, state)
			}
		case strings.Contains(cleanOpt, "Change World Game Mode"):
			modes := []string{
				core.GREEN + "SURVIVAL" + core.NC + " (Standard with hunger, thirst, oxygen, health)",
				core.CYAN + "FREEDOM" + core.NC + "  (No hunger or thirst, pure building & exploration)",
				core.RED + "HARDCORE" + core.NC + " (Permadeath, single life, no voice warnings)",
				core.YELLOW + "CREATIVE" + core.NC + " (Unlimited materials, instant craft, invulnerable)",
			}
			gmIdx, _, gmSt := tui.SelectMenu("Select World Game Mode", modes)
			if gmSt == 0 {
				switch gmIdx {
				case 0:
					state["GAME_MODE"] = "SURVIVAL"
				case 1:
					state["GAME_MODE"] = "FREEDOM"
				case 2:
					state["GAME_MODE"] = "HARDCORE"
				case 3:
					state["GAME_MODE"] = "CREATIVE"
				}
				saveServerConfig(mainCfg, saveCfg, state)
			}
		case strings.Contains(cleanOpt, "View Raw server.cfg Content"):
			tui.ClearScreen()
			content, err := os.ReadFile(mainCfg)
			if err != nil {
				content, err = os.ReadFile(saveCfg)
			}
			if err == nil {
				tui.PrintInBox("server.cfg Contents", tui.FormatKeyValueOutput(string(content), "server.cfg is currently empty."))
			} else {
				tui.PrintInBox("server.cfg Not Found", tui.FormatInfoNotice("INFO", "Configuration file not found yet. It will be generated when server starts.", core.YELLOW))
			}
		}
	}
}

func saveServerConfig(mainCfg, saveCfg string, state map[string]string) {
	port := state["PORT"]
	if port == "" {
		port = "11000"
	}
	maxP := state["MAX_PLAYERS"]
	if maxP == "" {
		maxP = "8"
	}
	pw := state["SERVER_PASSWORD"]
	admPw := state["ADMIN_PASSWORD"]
	if admPw == "" {
		admPw = "AdminToken123"
	}
	gm := state["GAME_MODE"]
	if gm == "" {
		gm = "SURVIVAL"
	}
	saveName := state["SAVE_NAME"]
	if saveName == "" {
		saveName = "world"
	}

	cfgContent := fmt.Sprintf(`[Server]
ServerPort=%s
ServerPassword=%s
AdminPassword=%s
SaveInterval=120000
MaxConnections=%s
SaveName=%s
GameMode=%s
DisableAutoSave=False
`, port, pw, admPw, maxP, saveName, gm)

	_ = system.WriteFileNoFollow(mainCfg, []byte(cfgContent), 0600, "")
	_ = system.WriteFileNoFollow(saveCfg, []byte(cfgContent), 0600, "")
	system.SaveState(state)

	tui.PrintInBox("Configuration Updated", []string{
		core.GREEN + "[SUCCESS] server.cfg updated successfully!" + core.NC,
		"",
		"Restart the Nitrox service to apply modified settings to active game session.",
	})
	time.Sleep(1 * time.Second)
}

func backupManagerMenu(state map[string]string) {
	for {
		options := []string{
			"─── BACKUP CREATION ───",
			"Create Instant World Save Backup Archive (.tar.gz)",
			"─── ARCHIVES & RESTORATION ───",
			"List Existing World Backup Archives",
			"Restore World State from a Previous Backup Archive",
			"─── NAVIGATION ───",
			"<- Back to Main Management Menu",
		}

		_, opt, status := tui.SelectMenu("KROTEK NITROX" + " " + core.Sym.EmDash + " " + "World Saves & Backup Hub", options)
		if status == 2 || (status == 0 && strings.Contains(opt, "Back")) {
			return
		}

		cleanOpt := core.StripANSI(opt)
		switch {
		case strings.Contains(cleanOpt, "Create Instant World Save"):
			createInstantBackup(state)
		case strings.Contains(cleanOpt, "List Existing World Backup"):
			listBackups(state)
		case strings.Contains(cleanOpt, "Restore World State"):
			restoreBackupWizard(state)
		}
	}
}

func createInstantBackup(state map[string]string) {
	tui.ClearScreen()
	timestamp := time.Now().Format("20060102_150405")
	backupDir := "/var/backups/krotek/nitrox"
	_ = os.MkdirAll(backupDir, 0700)
	backupFile := fmt.Sprintf("%s/nitrox_world_backup_%s.tar.gz", backupDir, timestamp)

	var err error
	if state["ENV_TYPE"] == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd == "" {
			cd = "/etc/nitrox-docker"
		}
		err = exec.Command("tar", "-czf", backupFile, "-C", cd, ".").Run()
	} else {
		installDir := state["INSTALL_DIR"]
		if installDir == "" {
			if _, statErr := os.Stat("/srv/nitrox"); statErr == nil {
				installDir = "/srv/nitrox"
			} else {
				installDir = "/opt/nitrox"
			}
		}
		dataDir := filepath.Join(installDir, "data")
		if _, statErr := os.Stat(dataDir); statErr == nil {
			err = exec.Command("tar", "-czf", backupFile, "-C", dataDir, ".").Run()
		} else {
			err = exec.Command("tar", "-czf", backupFile, "-C", installDir, ".").Run()
		}
	}

	if err == nil {
		_ = os.Chmod(backupFile, 0600)
		tui.PrintInBox("Instant Backup Success", []string{
			core.GREEN + "[SUCCESS] Nitrox world backup archive generated successfully!" + core.NC,
			fmt.Sprintf("Archive File: %s%s%s", core.BOLD, backupFile, core.NC),
		})
	} else {
		tui.PrintInBox("Backup Failed", []string{
			core.RED + "[ERROR] Failed to generate backup: " + err.Error() + core.NC,
		})
	}
}

func listBackups(state map[string]string) {
	tui.ClearScreen()
	backupDir := "/var/backups/krotek/nitrox"
	installDir := state["INSTALL_DIR"]
	if installDir == "" {
		installDir = "/srv/nitrox"
	}
	customBackupDir := filepath.Join(installDir, "backups")

	dirs := []string{backupDir, customBackupDir, "/root"}
	var found []string

	for _, d := range dirs {
		if entries, err := os.ReadDir(d); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasPrefix(e.Name(), "nitrox") && strings.HasSuffix(e.Name(), ".tar.gz") {
					fullPath := filepath.Join(d, e.Name())
					fi, _ := e.Info()
					sz := float64(0)
					if fi != nil {
						sz = float64(fi.Size()) / (1024 * 1024)
					}
					found = append(found, fmt.Sprintf("%s (%s, %.2f MB)", fullPath, e.Name(), sz))
				}
			}
		}
	}

	if len(found) == 0 {
		tui.PrintInBox("No Backups Found", []string{
			"No Nitrox backup archives (.tar.gz) detected in:",
			"  - /var/backups/krotek/nitrox/",
			"  - " + customBackupDir + "/",
		})
	} else {
		tui.PrintInBox("Existing Nitrox World Backup Archives", found)
	}
}

func restoreBackupWizard(state map[string]string) {
	tui.ClearScreen()
	backupDir := "/var/backups/krotek/nitrox"
	installDir := state["INSTALL_DIR"]
	if installDir == "" {
		installDir = "/srv/nitrox"
	}
	customBackupDir := filepath.Join(installDir, "backups")

	dirs := []string{backupDir, customBackupDir}
	var archives []string

	for _, d := range dirs {
		if entries, err := os.ReadDir(d); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasPrefix(e.Name(), "nitrox") && strings.HasSuffix(e.Name(), ".tar.gz") {
					archives = append(archives, filepath.Join(d, e.Name()))
				}
			}
		}
	}

	if len(archives) == 0 {
		tui.PrintInBox("No Archives to Restore", tui.FormatInfoNotice("INFO", "No backup archives found. Create an instant backup first.", core.YELLOW))
		return
	}

	var menuOpts []string
	menuOpts = append(menuOpts, fmt.Sprintf("─── AVAILABLE BACKUPS (%d) ───", len(archives)))
	for _, a := range archives {
		info, sErr := os.Stat(a)
		if sErr == nil {
			sizeMB := float64(info.Size()) / (1024 * 1024)
			menuOpts = append(menuOpts, fmt.Sprintf("%s%s%s %s(%.1f MB)%s", core.CYAN, filepath.Base(a), core.NC, core.DIM, sizeMB, core.NC))
		} else {
			menuOpts = append(menuOpts, core.CYAN+filepath.Base(a)+core.NC)
		}
	}
	menuOpts = append(menuOpts, "─── NAVIGATION ───")
	menuOpts = append(menuOpts, core.Sym.ArrowLeft+" Cancel")

	_, selOpt, st := tui.SelectMenu("Select Backup Archive to Restore", menuOpts)
	cleanSel := core.StripANSI(selOpt)
	if st != 0 || strings.Contains(cleanSel, "Cancel") {
		return
	}

	var chosen string
	for _, a := range archives {
		if strings.HasPrefix(cleanSel, filepath.Base(a)) {
			chosen = a
			break
		}
	}
	if chosen == "" {
		return
	}
	if tui.AskYesNo(fmt.Sprintf("Confirm restoring world data from %s?", filepath.Base(chosen))) != 0 {
		return
	}

	tui.StartSpinner("Validating and restoring Nitrox world backup...")
	if err := system.ValidateSafeDirectoryPath(installDir); err != nil {
		tui.StopSpinner(1)
		tui.PrintInBox("Security Error", []string{core.RED + "[ERROR] Invalid install directory: " + err.Error() + core.NC})
		return
	}
	if err := system.ValidateTarGzArchive(chosen, 8*1024*1024*1024); err != nil {
		tui.StopSpinner(1)
		tui.PrintInBox("Security Error", []string{core.RED + "[ERROR] Unsafe or corrupt backup archive rejected: " + err.Error() + core.NC})
		return
	}
	_ = exec.Command("systemctl", "stop", "nitrox").Run()
	dataDir := filepath.Join(installDir, "data")
	_ = os.MkdirAll(dataDir, 0750)
	extractRoot := dataDir
	if listOut, lErr := exec.Command("tar", "-tzf", chosen).Output(); lErr == nil {
		for _, entry := range strings.Split(strings.TrimSpace(string(listOut)), "\n") {
			clean := strings.TrimPrefix(strings.TrimSpace(entry), "./")
			if clean == "" || clean == "." {
				continue
			}
			if strings.HasPrefix(clean, "srv/") || strings.HasPrefix(clean, "opt/") || strings.HasPrefix(clean, "etc/") {
				extractRoot = "/"
			}
			break
		}
	}
	if err := exec.Command("tar", "-xzf", chosen, "-C", extractRoot).Run(); err != nil {
		tui.StopSpinner(1)
		tui.PrintInBox("Restore Failed", []string{core.RED + "[ERROR] Failed to extract backup archive: " + err.Error() + core.NC})
		return
	}
	_ = filepath.WalkDir(installDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		_ = exec.Command("chown", "-h", "nitrox:nitrox", p).Run()
		return nil
	})
	_ = exec.Command("systemctl", "restart", "nitrox").Run()
	tui.StopSpinner(0)

	tui.PrintInBox("Restore Complete", []string{
		core.GREEN + "[SUCCESS] World data restored and service restarted!" + core.NC,
		fmt.Sprintf("Restored Archive: %s", chosen),
	})
	time.Sleep(1500 * time.Millisecond)
}

func installSelectionMenu() {
	options := []string{
		"─── HOST SYSTEM DEPLOYMENT ───",
		"Native Linux System Service (.NET Dedicated Daemon) (Recommended)",
		"─── CONTAINERIZED DEPLOYMENT ───",
		"Docker Containerized Stack (Isolated Compose Stack)",
		"─── NAVIGATION ───",
		"<- Back to Main Menu",
	}

	for {
		_, opt, status := tui.SelectMenu("KROTEK NITROX" + " " + core.Sym.EmDash + " " + "Deployment Mode Selection", options)
		if status == 2 || (status == 0 && strings.Contains(opt, "Back")) {
			return
		}

		cleanOpt := core.StripANSI(opt)
		switch {
		case strings.Contains(cleanOpt, "Native Linux"):
			install.FreshInstallWizard("native")
			return
		case strings.Contains(cleanOpt, "Docker Containerized"):
			install.FreshInstallWizard("docker")
			return
		}
	}
}

// noServiceDiagnosticMenu guides the user when Nitrox Subnautica Multiplayer Server is not yet installed or stopped.
func noServiceDiagnosticMenu() {
	for {
		options := []string{
			"─── DEPLOYMENT ───",
			"Install Nitrox Dedicated Server (Fresh Guided Setup Wizard)",
			"─── DOCUMENTATION & PREREQUISITES ───",
			"Subnautica Game Files Requirement & Upload Guide",
			"─── DIAGNOSTICS & SYSTEM AUDIT ───",
			"Run Deep System & Network Diagnostics",
			"─── EXIT ───",
			"Exit to Terminal",
		}

		_, opt, status := tui.SelectMenu("KROTEK NITROX" + " " + core.Sym.EmDash + " " + "Server Not Detected", options)
		if status == 2 || (status == 0 && (strings.Contains(opt, "Exit") || strings.Contains(opt, "Back"))) {
			return
		}

		cleanOpt := core.StripANSI(opt)
		switch {
		case strings.Contains(cleanOpt, "Install Nitrox Dedicated Server"):
			installSelectionMenu()
			return
		case strings.Contains(cleanOpt, "Subnautica Game Files"):
			showGameFilesCheckGuide(system.LoadState())
		case strings.Contains(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()
		}
	}
}

// dockerManagementMenu provides container lifecycle controls for containerized Nitrox stack.
func dockerManagementMenu(composeDir string) {
	for {
		badge := getDockerStatusBadge(composeDir)
		title := fmt.Sprintf("KROTEK NITROX [Docker Container] %s", badge)

		restartLabel := "Restart Docker Container"
		toggleLabel := "Start Docker Container (docker compose up)"
		if strings.Contains(badge, "ACTIVE") {
			toggleLabel = "Stop Docker Container (docker compose stop)"
		}

		options := []string{
			"─── SERVER STATUS & ASSETS ───",
			"Show Server Status & Live Telemetry",
			"Check & Upload Subnautica Game Files (Assets Guide)",
			"─── GAMEPLAY & SERVER CONFIGURATION ───",
			"Configure Server Parameters (server.cfg)",
			"World Saves & Backup Manager (.tar.gz)",
			"─── CONTAINER LIFECYCLE ───",
			restartLabel,
			toggleLabel,
			"View Container Logs & Output (Tail 50)",
			"─── MAINTENANCE & UPDATES ───",
			"Update Nitrox Container Image Stack",
			"Run Deep System & Network Diagnostics",
			"Run Guided Setup / Reinstallation Wizard",
			"─── REMOVAL & EXIT ───",
			"Uninstall Nitrox Server Stack",
			"Exit to Terminal",
		}

		_, opt, status := tui.SelectMenu(title, options)
		if status == 2 || (status == 0 && (strings.Contains(opt, "Exit") || strings.Contains(opt, "Back"))) {
			return
		}

		state := system.LoadState()
		cleanOpt := core.StripANSI(opt)
		switch {
		case strings.Contains(cleanOpt, "Show Server Status"):
			showServerStatusDetails(state)
		case strings.Contains(cleanOpt, "Check & Upload Subnautica Game Files"):
			showGameFilesCheckGuide(state)
		case strings.Contains(cleanOpt, "Configure Server Parameters"):
			editServerConfiguration(state)
		case strings.Contains(cleanOpt, "Restart Docker Container"):
			tui.ClearScreen()
			_ = exec.Command("docker", "compose", "--project-directory", composeDir, "restart").Run()
			tui.PrintInBox("Restart Container", []string{core.GREEN + "[SUCCESS] Nitrox Docker container restarted." + core.NC})
		case strings.Contains(cleanOpt, "Stop Docker Container"):
			tui.ClearScreen()
			_ = exec.Command("docker", "compose", "--project-directory", composeDir, "stop").Run()
			tui.PrintInBox("Stop Container", []string{core.YELLOW + "[STOPPED] Nitrox Docker container stopped." + core.NC})
		case strings.Contains(cleanOpt, "Start Docker Container"):
			tui.ClearScreen()
			_ = exec.Command("docker", "compose", "--project-directory", composeDir, "up", "-d").Run()
			tui.PrintInBox("Start Container", []string{core.GREEN + "[STARTED] Nitrox Docker container started." + core.NC})
		case strings.Contains(cleanOpt, "View Container Logs"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "compose", "--project-directory", composeDir, "logs", "--tail", "50").CombinedOutput()
			tui.PrintInBox("Container Logs (Tail 50)", tui.FormatStyledLogLines(string(out), "No container logs found."))
		case strings.Contains(cleanOpt, "World Saves & Backup Manager"):
			backupManagerMenu(state)
		case strings.Contains(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "Update Nitrox Container"):
			system.RunServiceUpdateFlow()
		case strings.Contains(cleanOpt, "Run Guided Setup"):
			install.FreshInstallWizard("docker")
			return
		case strings.Contains(cleanOpt, "Uninstall Nitrox Server"):
			install.UninstallWizard()
			return
		}
	}
}

// nativeManagementMenu provides direct host systemd controls, configuration test shortcuts, and port inspectors.
func nativeManagementMenu() {
	for {
		badge := getServiceStatusBadge()
		title := fmt.Sprintf("KROTEK NITROX [Native Service] %s", badge)

		restartLabel := "Restart Nitrox Service"
		toggleLabel := "Start Server Daemon (Start Service)"
		if strings.Contains(badge, "ACTIVE") {
			toggleLabel = "Stop Server Daemon (Stop Service)"
		}

		options := []string{
			"─── SERVER STATUS & ASSETS ───",
			"Show Server Status & Live Telemetry",
			"Check & Upload Subnautica Game Files (Assets Guide)",
			"─── GAMEPLAY & SERVER CONFIGURATION ───",
			"Configure Server Parameters (server.cfg)",
			"World Saves & Backup Manager (.tar.gz)",
			"─── SERVICE LIFECYCLE ───",
			restartLabel,
			toggleLabel,
			"View Service Logs & Live Console Journal",
			"─── MAINTENANCE & UPDATES ───",
			"Update Nitrox Server & Check Upgrades",
			"Run Deep System & Network Diagnostics",
			"Run Guided Setup / Reinstallation Wizard",
			"─── REMOVAL & EXIT ───",
			"Uninstall Nitrox Server Stack",
			"Exit to Terminal",
		}

		_, opt, status := tui.SelectMenu(title, options)
		if status == 2 || (status == 0 && (strings.Contains(opt, "Exit") || strings.Contains(opt, "Back"))) {
			return
		}

		state := system.LoadState()
		cleanOpt := core.StripANSI(opt)
		switch {
		case strings.Contains(cleanOpt, "Show Server Status"):
			showServerStatusDetails(state)
		case strings.Contains(cleanOpt, "Check & Upload Subnautica Game Files"):
			showGameFilesCheckGuide(state)
		case strings.Contains(cleanOpt, "Configure Server Parameters"):
			editServerConfiguration(state)
		case strings.Contains(cleanOpt, "Restart Nitrox Service"):
			tui.ClearScreen()
			_ = exec.Command("systemctl", "restart", "nitrox").Run()
			tui.PrintInBox("Restart Service", []string{core.GREEN + "[SUCCESS] Subnautica Nitrox service restarted." + core.NC})
		case strings.Contains(cleanOpt, "Stop Server Daemon"):
			tui.ClearScreen()
			_ = exec.Command("systemctl", "stop", "nitrox").Run()
			tui.PrintInBox("Stop Service", []string{core.YELLOW + "[STOPPED] Subnautica Nitrox service stopped." + core.NC})
		case strings.Contains(cleanOpt, "Start Server Daemon"):
			tui.ClearScreen()
			_ = exec.Command("systemctl", "start", "nitrox").Run()
			tui.PrintInBox("Start Service", []string{core.GREEN + "[STARTED] Subnautica Nitrox service started." + core.NC})
		case strings.Contains(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()
		case strings.Contains(cleanOpt, "World Saves & Backup Manager"):
			backupManagerMenu(state)
		case strings.Contains(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "Update Nitrox Server"):
			system.RunServiceUpdateFlow()
		case strings.Contains(cleanOpt, "Run Guided Setup"):
			install.FreshInstallWizard("native")
			return
		case strings.Contains(cleanOpt, "Uninstall Nitrox Server"):
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
			cd = "/etc/nitrox-docker"
		}
		dockerManagementMenu(cd)
	} else {
		nativeManagementMenu()
	}
}
