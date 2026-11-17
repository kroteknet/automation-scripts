// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: Minecraft PaperMC Dedicated Game Server (minecraft)
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
	"regexp"
	"strings"
	"time"

	"krotek-minecraft-installer/core"
	"krotek-minecraft-installer/install"
	"krotek-minecraft-installer/modules"
	"krotek-minecraft-installer/system"
	"krotek-minecraft-installer/tui"
)

func noMinecraftDiagnosticMenu() {
	for {
		tui.ClearScreen()
		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("Minecraft Service", core.RED+"[NOT INSTALLED]"+core.NC, core.RED),
			tui.FormatKV("Java Runtime Engine", system.GetToolBadge("java"), core.YELLOW),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + "Status: Minecraft Dedicated Game Server is not provisioned on this machine yet." + core.NC,
			"Choose an action below to deploy Minecraft or run system network diagnostics.",
		}

		options := []string{
			"Install Minecraft Docker (Containerized Stack) - Recommended)",
			"Install Minecraft Native (Guided PaperMC/Purpur Wizard + Firewall)",
			"Run Deep System & Network Diagnostics Scan",
			"Exit Diagnostics",
		}

		idx, _, status := tui.SelectMenuWithContent("minecraft-manager - System Diagnostics (Minecraft Not Found)", diagLines, options)
		if status == 2 || (status == 0 && idx == 3) {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		switch idx {
		case 0:
			install.StartGuidedInstall("docker")
			if system.IsServiceInstalled() {
				managementMenu()
				return
			}
		case 1:
			install.StartGuidedInstall("native")
			if system.IsServiceInstalled() {
				managementMenu()
				return
			}
		case 2:
			modules.RunDeepDiagnostics()
		}
	}
}

func showMinecraftDashboard(isDocker bool) {
	tui.ClearScreen()
	state := system.LoadState()
	health := modules.DetectMinecraftHealth()

	serverDir := system.GetMinecraftDataDir()

	onlineMode := "false"
	serverPort := state["SERVER_PORT"]
	if serverPort == "" {
		serverPort = core.DefaultServerPort
	}
	worldName := "world"
	maxPlayers := "20"
	motd := "A Minecraft Server"

	if data, err := os.ReadFile(serverDir + "/server.properties"); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "online-mode=") {
				onlineMode = strings.TrimPrefix(l, "online-mode=")
			} else if strings.HasPrefix(l, "server-port=") {
				serverPort = strings.TrimPrefix(l, "server-port=")
			} else if strings.HasPrefix(l, "level-name=") {
				worldName = strings.TrimPrefix(l, "level-name=")
			} else if strings.HasPrefix(l, "max-players=") {
				maxPlayers = strings.TrimPrefix(l, "max-players=")
			} else if strings.HasPrefix(l, "motd=") {
				motd = strings.TrimPrefix(l, "motd=")
			}
		}
	}

	engine := state["ENGINE"]
	if engine == "" {
		engine = "PaperMC"
	}
	mcVer := state["MC_VERSION"]
	if mcVer == "" {
		mcVer = health.VersionText
	}
	ramAlloc := state["ALLOCATED_RAM"]
	if ramAlloc == "" {
		ramAlloc = "2G"
	}

	localIP := system.GetLocalIP()
	publicIP := system.GetPublicIP()

	var dbLines []string
	dbLines = append(dbLines, tui.FormatKV("Application", "Minecraft Java Dedicated Game Server", core.GREEN+core.BOLD))
	dbLines = append(dbLines, tui.FormatKV("Runtime Status", func() string {
		if health.IsRunning {
			return core.GREEN + "[ACTIVE / RUNNING]"
		}
		return core.RED + "[STOPPED / INACTIVE]"
	}(), core.GREEN))
	dbLines = append(dbLines, tui.FormatKV("Engine & Version", fmt.Sprintf("%s %s", engine, mcVer), core.YELLOW))
	dbLines = append(dbLines, tui.FormatKV("Local Network IP", fmt.Sprintf("%s:%s", localIP, serverPort), core.CYAN))
	dbLines = append(dbLines, tui.FormatKV("Public Internet IP", fmt.Sprintf("%s:%s", publicIP, serverPort), core.GREEN))
	dbLines = append(dbLines, tui.FormatKV("Direct Connect URL", fmt.Sprintf("%s:%s", localIP, serverPort), core.BOLD+core.YELLOW))

	if health.IsRunning {
		if slp, err := modules.QueryServerListPing("127.0.0.1", serverPort); err == nil {
			dbLines = append(dbLines, tui.FormatKV("Live Online Players", modules.FormatPlayerList(slp), core.BOLD+core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("Ping Latency", fmt.Sprintf("%d ms", slp.LatencyMS), core.GREEN))
		}
	}

	dbLines = append(dbLines, tui.FormatKV("Memory (JVM Heap)", fmt.Sprintf("-Xms1G -Xmx%s", ramAlloc), core.CYAN))
	dbLines = append(dbLines, tui.FormatKV("Online Mode", func() string {
		if onlineMode == "true" {
			return core.GREEN + "TRUE (Premium Accounts Only)"
		}
		return core.YELLOW + "FALSE (Offline / LAN Allowed)"
	}(), core.CYAN))
	dbLines = append(dbLines, tui.FormatKV("Server World Name", worldName, core.CYAN))
	dbLines = append(dbLines, tui.FormatKV("Player Slots", maxPlayers, core.CYAN))
	dbLines = append(dbLines, tui.FormatKV("Server MOTD", motd, core.CYAN))
	dbLines = append(dbLines, tui.FormatKV("Server Directory", serverDir, core.CYAN))
	dbLines = append(dbLines, tui.FormatKV("Backup Storage", core.BackupDir, core.CYAN))
	if !isDocker {
		dbLines = append(dbLines, tui.FormatKV("Service Unit", "systemctl status minecraft", core.CYAN))
	} else {
		dbLines = append(dbLines, tui.FormatKV("Docker Container", core.DockerContainerName, core.CYAN))
	}
	if _, err := os.Stat(filepath.Join(serverDir, "plugins", "BlueMap.jar")); err == nil {
		dbLines = append(dbLines, tui.FormatKV("3D Live Web Map", fmt.Sprintf("http://%s:%s", localIP, core.BlueMapPort), core.BOLD+core.GREEN))
	}
	dbLines = append(dbLines, "")
	dbLines = append(dbLines, core.BOLD+"Client Join Instruction:"+core.NC)
	dbLines = append(dbLines, fmt.Sprintf("  Multiplayer" + " " + core.Sym.ArrowRight + " " + "Direct Connection" + " " + core.Sym.ArrowRight + " " + "%s%s:%s%s", core.BOLD+core.YELLOW, localIP, serverPort, core.NC))

	tui.PrintInBox("Minecraft Server Dashboard", dbLines)
}

func showServerConsoleLogs(logPath string) {
	tui.ClearScreen()
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		out, _ := exec.Command("journalctl", "-u", "minecraft", "-n", "50", "--no-pager").CombinedOutput()
		tui.PrintInBox("Minecraft Console Logs (journalctl)", tui.FormatStyledLogLines(string(out), "No journalctl logs found for minecraft service."))
		return
	}

	out, err := exec.Command("tail", "-n", "50", logPath).CombinedOutput()
	if err != nil {
		tui.PrintInBox("Logs Error", []string{core.RED + "Could not read log file: " + logPath + core.NC})
		return
	}

	lines := strings.Split(string(out), "\n")
	var coloredLines []string
	for _, l := range lines {
		if strings.Contains(l, "WARN") {
			coloredLines = append(coloredLines, core.YELLOW+l+core.NC)
		} else if strings.Contains(l, "ERROR") {
			coloredLines = append(coloredLines, core.RED+l+core.NC)
		} else if strings.Contains(l, "INFO") {
			coloredLines = append(coloredLines, core.CYAN+l+core.NC)
		} else {
			coloredLines = append(coloredLines, l)
		}
	}
	tui.PrintInBox(fmt.Sprintf("Live Server Logs" + " " + core.Sym.EmDash + " " + "%s (Tail 50)", filepath.Base(logPath)), coloredLines)
}

func configureServerProperties(propPath string) {
	modified := false
	for {
		props := make(map[string]string)
		if data, err := os.ReadFile(propPath); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						props[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
					}
				}
			}
		}

		onlineMode := props["online-mode"]
		if onlineMode == "" {
			onlineMode = "false"
		}
		motd := props["motd"]
		difficulty := props["difficulty"]
		if difficulty == "" {
			difficulty = "easy"
		}
		maxPlayers := props["max-players"]
		if maxPlayers == "" {
			maxPlayers = "20"
		}
		pvp := props["pvp"]
		if pvp == "" {
			pvp = "true"
		}

		options := []string{
			fmt.Sprintf("Online Mode: %s", func() string {
				if onlineMode == "true" {
					return core.GREEN + "TRUE (Premium Only)" + core.NC
				}
				return core.YELLOW + "FALSE (Offline / Cracked Allowed)" + core.NC
			}()),
			fmt.Sprintf("Server MOTD: %s", motd),
			fmt.Sprintf("Difficulty: %s%s%s", core.CYAN, difficulty, core.NC),
			fmt.Sprintf("Max Players: %s%s%s", core.CYAN, maxPlayers, core.NC),
			fmt.Sprintf("PVP Enabled: %s%s%s", core.CYAN, pvp, core.NC),
			"Open Full server.properties in Text Editor (Nano/Vim)",
			"Back to Main Menu",
		}

		idx, opt, st := tui.SelectMenu("KROTEK MINECRAFT" + " " + core.Sym.EmDash + " " + "server.properties Configuration", options)
		if st == 2 || strings.Contains(opt, "Back to Main Menu") {
			if modified {
				_, rOpt, _ := tui.SelectMenu("Configuration Changed" + " " + core.Sym.EmDash + " " + "Restart Server?", []string{
					"Restart Minecraft Server now to apply changes (Recommended)",
					"Don't restart yet",
				})
				if strings.Contains(rOpt, "Restart") {
					_, _ = system.RestartMinecraftService()
				}
			}
			return
		}

		switch idx {
		case 0:
			if onlineMode == "true" {
				updateProperty(propPath, "online-mode", "false")
			} else {
				updateProperty(propPath, "online-mode", "true")
			}
			modified = true
			// Show a brief explanation so beginners understand the setting
			newMode := "false"
			if onlineMode != "true" {
				newMode = "true"
			}
			explain := "Offline mode: players with any Minecraft copy can join (including cracked clients)."
			if newMode == "true" {
				explain = "Online mode: only players with a genuine paid Minecraft account can join."
			}
			tui.PrintInBox("Online Mode Changed", []string{
				fmt.Sprintf("%s[OK] online-mode set to %s.%s", core.GREEN, newMode, core.NC),
				explain,
				"Restart the server to apply this change.",
			})
		case 1:
			newMotd, st := tui.ReadTextInput("Enter new Server MOTD", motd)
			if st == 0 && newMotd != "" {
				updateProperty(propPath, "motd", newMotd)
				modified = true
			}
		case 2:
			dOpts := []string{"peaceful", "easy", "normal", "hard", "Back"}
			dIdx, dOpt, dSt := tui.SelectMenu("Select Server Difficulty", dOpts)
			if dSt == 0 && !strings.Contains(dOpt, "Back") {
				updateProperty(propPath, "difficulty", dOpts[dIdx])
				modified = true
			}
		case 3:
			newMax, st := tui.ReadTextInput("Enter Max Players limit", maxPlayers)
			if st == 0 && newMax != "" {
				updateProperty(propPath, "max-players", newMax)
				modified = true
			}
		case 4:
			if pvp == "true" {
				updateProperty(propPath, "pvp", "false")
			} else {
				updateProperty(propPath, "pvp", "true")
			}
			modified = true
		case 5:
			_ = tui.RunEditor(propPath)
			modified = true
		}
	}
}

func updateProperty(filePath, key, val string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+"=") {
			lines[i] = key + "=" + val
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, key+"="+val)
	}
	_ = system.WriteFileNoFollow(filePath, []byte(strings.Join(lines, "\n")), 0644, "")
}

func changeNativeRAM() {
	newRAM, st := install.SelectRAM()
	if st != 0 || newRAM == "" {
		return
	}

	servicePath := "/etc/systemd/system/minecraft.service"
	content, err := os.ReadFile(servicePath)
	if err != nil {
		tui.PrintInBox("Error", []string{core.RED + "Could not read " + servicePath + core.NC})
		return
	}

	minRAM := "1G"
	if strings.HasPrefix(newRAM, "1G") || strings.HasSuffix(newRAM, "M") {
		minRAM = "512M"
	}

	re := regexp.MustCompile(`-Xms\S+\s+-Xmx\S+`)
	updated := re.ReplaceAllString(string(content), fmt.Sprintf("-Xms%s -Xmx%s", minRAM, newRAM))
	if err := system.WriteFileNoFollow(servicePath, []byte(updated), 0644, ""); err != nil {
		tui.PrintInBox("Error", []string{core.RED + "Failed to update " + servicePath + core.NC})
		return
	}

	_ = exec.Command("systemctl", "daemon-reload").Run()
	_ = exec.Command("systemctl", "restart", "minecraft").Run()

	state := system.LoadState()
	state["ALLOCATED_RAM"] = newRAM
	system.SaveState(state)

	tui.PrintInBox("RAM Updated", []string{
		core.GREEN + "[SUCCESS] Minecraft Server RAM updated to " + newRAM + core.NC,
		fmt.Sprintf("Memory bounds: -Xms%s (initial) -Xmx%s (maximum).", minRAM, newRAM),
		"Service has been reloaded and restarted with new memory allocation.",
	})
}

func fixFirewallNetwork() {
	tui.ClearScreen()
	tui.StartSpinner("Applying universal firewall rules for Minecraft ports...")

	state := system.LoadState()
	port := state["SERVER_PORT"]
	if port == "" {
		port = core.DefaultServerPort
	}

	var results []string
	results = append(results, core.BOLD+core.YELLOW+"=== MINECRAFT FIREWALL & NETWORKING AUTO-FIX ==="+core.NC, "")

	applied := 0
	if system.CommandExists("iptables") {
		_ = exec.Command("iptables", "-I", "INPUT", "-p", "tcp", "--dport", port, "-j", "ACCEPT").Run()
		_ = exec.Command("iptables", "-I", "INPUT", "-p", "udp", "--dport", core.BedrockPort, "-j", "ACCEPT").Run()
		results = append(results, core.GREEN+core.TagOK() + " iptables: Port "+port+" TCP allowed (Java)"+core.NC)
		results = append(results, core.GREEN+core.TagOK() + " iptables: Port "+core.BedrockPort+" UDP allowed (Bedrock crossplay)"+core.NC)
		applied++
	}
	if system.CommandExists("ip6tables") {
		_ = exec.Command("ip6tables", "-I", "INPUT", "-p", "tcp", "--dport", port, "-j", "ACCEPT").Run()
		_ = exec.Command("ip6tables", "-I", "INPUT", "-p", "udp", "--dport", core.BedrockPort, "-j", "ACCEPT").Run()
		results = append(results, core.GREEN+core.TagOK() + " ip6tables: Port "+port+" TCP allowed in IPv6 INPUT chain"+core.NC)
		applied++
	}
	if system.CommandExists("nft") {
		_ = exec.Command("nft", "add", "rule", "inet", "filter", "input", "tcp", "dport", port, "accept").Run()
		_ = exec.Command("nft", "add", "rule", "inet", "filter", "input", "udp", "dport", core.BedrockPort, "accept").Run()
		results = append(results, core.GREEN+core.TagOK() + " nftables: Rules added for TCP "+port+" and UDP "+core.BedrockPort+core.NC)
		applied++
	}
	if system.CommandExists("ufw") {
		_ = exec.Command("ufw", "allow", port+"/tcp").Run()
		_ = exec.Command("ufw", "allow", core.BedrockPort+"/udp").Run()
		results = append(results, core.GREEN+core.TagOK() + " ufw: Ports "+port+"/tcp and "+core.BedrockPort+"/udp allowed"+core.NC)
		applied++
	}
	if system.CommandExists("firewall-cmd") {
		_ = exec.Command("firewall-cmd", "--add-port="+port+"/tcp", "--permanent").Run()
		_ = exec.Command("firewall-cmd", "--add-port="+core.BedrockPort+"/udp", "--permanent").Run()
		_ = exec.Command("firewall-cmd", "--reload").Run()
		results = append(results, core.GREEN+core.TagOK() + " firewalld: Ports "+port+"/tcp and "+core.BedrockPort+"/udp allowed"+core.NC)
		applied++
	}

	if applied == 0 {
		results = append(results, core.CYAN+"[INFO] No active software firewall daemon detected."+core.NC)
	}

	serverIP := system.GetLocalIP()
	results = append(results, "", core.BOLD+"LAN Connection Addresses for Minecraft Clients:"+core.NC)
	results = append(results, fmt.Sprintf("  Java Edition   " + " " + core.Sym.ArrowRight + " " + "Direct Connection" + " " + core.Sym.ArrowRight + " " + "%s%s:%s%s", core.BOLD+core.YELLOW, serverIP, port, core.NC))
	results = append(results, fmt.Sprintf("  Bedrock Edition" + " " + core.Sym.ArrowRight + " " + "Direct Connection" + " " + core.Sym.ArrowRight + " " + "%s%s:%s%s", core.BOLD+core.CYAN, serverIP, core.BedrockPort, core.NC))

	tui.StopSpinner(0)
	tui.PrintInBox("Firewall Configuration Result", results)
}

func isModEngine() bool {
	state := system.LoadState()
	engType := strings.ToLower(strings.TrimSpace(state["ENGINE_TYPE"]))
	eng := strings.ToLower(strings.TrimSpace(state["ENGINE"]))

	if engType == "fabric" || engType == "neoforge" || engType == "forge" {
		return true
	}
	if strings.Contains(eng, "fabric") || strings.Contains(eng, "neoforge") || strings.Contains(eng, "forge") {
		return true
	}
	if engType != "" || eng != "" {
		return false
	}

	dataDir := system.GetMinecraftDataDir()
	if entries, err := os.ReadDir(filepath.Join(dataDir, "mods")); err == nil && len(entries) > 0 {
		return true
	}
	return false
}

func isPluginEngine() bool {
	state := system.LoadState()
	engType := strings.ToLower(strings.TrimSpace(state["ENGINE_TYPE"]))
	eng := strings.ToLower(strings.TrimSpace(state["ENGINE"]))

	if engType == "paper" || engType == "purpur" || engType == "spigot" {
		return true
	}
	if strings.Contains(eng, "paper") || strings.Contains(eng, "purpur") || strings.Contains(eng, "spigot") {
		return true
	}
	if engType == "vanilla" || strings.Contains(eng, "vanilla") {
		return false
	}
	if isModEngine() {
		return false
	}

	dataDir := system.GetMinecraftDataDir()
	if _, err := os.Stat(filepath.Join(dataDir, "plugins")); err == nil {
		return true
	}

	return true
}

func backupAndScheduleSubmenu(isDocker bool) {
	for {
		options := []string{
			"─── WORLD BACKUPS & SNAPSHOTS ───",
			"Create Full World Backup (.tar.gz)",
			"List & Manage World Backups",
			"─── AUTOMATION ───",
			"Automated Scheduled Tasks (Nightly Backup & Restart)",
			"─── NAVIGATION ───",
			"Back to Main Menu",
		}

		titleStr := "minecraft-manager  -  Backups & Automation"
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)
		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Back to Main Menu") {
			return
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "Create Full World Backup"):
			modules.CreateWorldBackup(isDocker)

		case strings.HasPrefix(cleanOpt, "List & Manage World Backups"):
			modules.ManageBackups(isDocker)

		case strings.HasPrefix(cleanOpt, "Automated Scheduled Tasks"):
			modules.ManageScheduledTasks()
		}
	}
}

func advancedMaintenanceSubmenu(isDocker bool, composeDir string) {
	for {
		stopLabel := "Stop Minecraft Service"
		if isDocker {
			stopLabel = "Stop Docker Container"
		}

		var options []string
		options = append(options, "─── UPDATES & ENVIRONMENT ───")
		options = append(options, "Update Minecraft Server & Check Upgrades (Safe Backup)")
		if isDocker {
			options = append(options, "Edit docker-compose.yml (RAM & Environment)")
		} else {
			options = append(options, "Change RAM Allocation Settings")
		}
		options = append(options, "Fix Firewall & LAN Network Access (Open Ports)")
		options = append(options, "Run Deep System & Network Diagnostics")

		options = append(options, "─── LIFECYCLE CONTROLS ───")
		options = append(options, stopLabel)

		options = append(options, "─── DANGEROUS / MAINTENANCE ───")
		options = append(options, "Run Guided Setup / Reinstallation Wizard")
		options = append(options, "Uninstall MINECRAFT Server Stack")

		options = append(options, "─── NAVIGATION ───")
		options = append(options, "Back to Main Menu")

		titleStr := "minecraft-manager  -  Advanced Maintenance & System Tools"
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)
		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Back to Main Menu") {
			return
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.HasPrefix(cleanOpt, "Edit docker-compose.yml"):
			_ = tui.RunEditor(filepath.Join(composeDir, "docker-compose.yml"))

		case strings.HasPrefix(cleanOpt, "Change RAM Allocation"):
			changeNativeRAM()

		case strings.HasPrefix(cleanOpt, "Fix Firewall"):
			fixFirewallNetwork()

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()

		case strings.HasPrefix(cleanOpt, "Stop Docker Container"):
			tui.ClearScreen()
			_, _ = system.StopMinecraftService()
			tui.PrintInBox("Stop Container", []string{core.YELLOW + "[SUCCESS] Docker container stopped." + core.NC})

		case strings.HasPrefix(cleanOpt, "Stop Minecraft Service"):
			tui.ClearScreen()
			_, _ = system.StopMinecraftService()
			tui.PrintInBox("Stop Service", []string{core.YELLOW + "[SUCCESS] Minecraft service stopped." + core.NC})

		case strings.Contains(cleanOpt, "Guided Setup"):
			if isDocker {
				install.StartGuidedInstall("docker")
			} else {
				install.StartGuidedInstall("native")
			}
			return

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallMinecraft()
			return
		}
	}
}

func showLogsSubmenu(isDocker bool, composeDir string) {
	for {
		tailLabel := "View Container Logs (Tail 50)"
		if !isDocker {
			tailLabel = "View Live Server Console Logs (Tail 50)"
		}

		options := []string{
			"─── LOG TARGETS ───",
			tailLabel,
			"View Service Logs & System Journal",
			"View Server Crash Reports (/crash-reports/)",
			"─── NAVIGATION ───",
			"Back to Main Menu",
		}

		titleStr := "minecraft-manager  -  Server Logs & Journal"
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)
		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Back to Main Menu") {
			return
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "View Container Logs"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "compose", "--project-directory", composeDir, "logs", "--tail", "50").CombinedOutput()
			tui.PrintInBox("Container Logs (Tail 50)", tui.FormatStyledLogLines(string(out), "No container logs found."))

		case strings.HasPrefix(cleanOpt, "View Live Server Console"):
			showServerConsoleLogs(filepath.Join(core.NativeServerDir, "logs", "latest.log"))

		case strings.HasPrefix(cleanOpt, "View Service Logs"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "View Server Crash Reports"):
			modules.ShowCrashReports()
		}
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.MinecraftHealth) {
	propPath := filepath.Join(core.NativeServerDir, "server.properties")

	for {
		health = modules.DetectMinecraftHealth()

		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart Minecraft Service"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start Minecraft Service  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		var options []string
		options = append(options, "─── SERVER DASHBOARD & LIVE CONTROLS ───")
		options = append(options, "View Server Dashboard & Connection Info")
		options = append(options, "Execute Server Console Command (RCON)")
		options = append(options, "View Server Logs & System Journal")

		options = append(options, "─── GAMEPLAY & SERVER CONFIGURATION ───")
		options = append(options, "Configure server.properties (GUI Settings)")
		options = append(options, "Optimize Engine Performance (Tick & Entity Tuning)")
		options = append(options, "Manage Server Operators & Whitelist Access")
		if isPluginEngine() {
			options = append(options, "Plugin & Addon Manager (1-Click Installers)")
		}
		if isModEngine() {
			options = append(options, "Mod Manager (Fabric / NeoForge Mods)")
		}

		options = append(options, "─── BACKUPS & SCHEDULES ───")
		options = append(options, "World Backups & Automated Schedules")

		options = append(options, "─── SERVICE MANAGEMENT ───")
		options = append(options, restartLabel)
		options = append(options, "Advanced Maintenance & System Tools")
		options = append(options, "Exit")

		titleStr := fmt.Sprintf("minecraft-manager  -  Management (Native: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "View Server Dashboard"):
			showMinecraftDashboard(false)

		case strings.HasPrefix(cleanOpt, "Execute Server Console"):
			modules.InteractiveConsoleCLI()

		case strings.HasPrefix(cleanOpt, "View Server Logs"):
			showLogsSubmenu(false, "")

		case strings.HasPrefix(cleanOpt, "Configure server.properties"):
			configureServerProperties(propPath)

		case strings.HasPrefix(cleanOpt, "Optimize Engine Performance"):
			modules.ShowPerformanceTuningMenu()

		case strings.HasPrefix(cleanOpt, "Manage Server Operators"):
			modules.ManagePlayers()

		case strings.HasPrefix(cleanOpt, "Plugin & Addon Manager"):
			modules.ManagePluginsMenu()

		case strings.HasPrefix(cleanOpt, "Mod Manager"):
			modules.ManageModsMenu()

		case strings.HasPrefix(cleanOpt, "World Backups & Automated Schedules"):
			backupAndScheduleSubmenu(false)

		case strings.HasPrefix(cleanOpt, "Restart Minecraft Service") || strings.HasPrefix(cleanOpt, "Start Minecraft Service"):
			tui.ClearScreen()
			tui.StartSpinner("Restarting Minecraft service...")
			ok, err := system.RestartMinecraftService()
			time.Sleep(2 * time.Second)
			tui.StopSpinner(0)
			if ok && system.IsMinecraftActive() {
				tui.PrintInBox("Service Started", []string{core.GREEN + core.TagOK() + " Minecraft server is running." + core.NC})
			} else {
				errMsg := "The server failed to start or exited immediately after launching."
				if err != nil {
					errMsg += " Reason: " + err.Error()
				}
				jOut, _ := system.GetMinecraftServiceLogs(15)
				lines := []string{
					core.RED + "[ERROR] " + errMsg + core.NC,
					"",
					"Common causes:",
					"  - A plugin is incompatible with the current server version",
					"  - Not enough RAM allocated (try increasing it in 'Advanced Maintenance & System Tools')",
					"  - The server port is already in use by another process",
					"",
				}
				lines = append(lines, jOut...)
				tui.PrintInBox("Server Failed to Start", lines)
			}

		case strings.HasPrefix(cleanOpt, "Advanced Maintenance"):
			advancedMaintenanceSubmenu(false, "")
		}
	}
}

// dockerManagementMenu provides container lifecycle controls (start/stop/restart/logs)
// for the containerized Minecraft PaperMC Dedicated Game Server Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.MinecraftHealth) {
	for {
		health = modules.DetectMinecraftHealth()

		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart Docker Container"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start Docker Container  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		var options []string
		options = append(options, "─── SERVER DASHBOARD & LIVE CONTROLS ───")
		options = append(options, "View Server Dashboard & Connection Info")
		options = append(options, "Execute Server Console Command (RCON)")
		options = append(options, "View Server Logs & System Journal")

		options = append(options, "─── GAMEPLAY & SERVER CONFIGURATION ───")
		options = append(options, "Configure server.properties (GUI Settings)")
		options = append(options, "Optimize Engine Performance (Tick & Entity Tuning)")
		options = append(options, "Manage Server Operators & Whitelist Access")
		if isPluginEngine() {
			options = append(options, "Plugin & Addon Manager (1-Click Installers)")
		}
		if isModEngine() {
			options = append(options, "Mod Manager (Fabric / NeoForge Mods)")
		}

		options = append(options, "─── BACKUPS & SCHEDULES ───")
		options = append(options, "World Backups & Automated Schedules")

		options = append(options, "─── CONTAINER LIFECYCLE ───")
		options = append(options, restartLabel)
		options = append(options, "Advanced Maintenance & System Tools")
		options = append(options, "Back / Exit")

		titleStr := fmt.Sprintf("minecraft-manager  -  Management (Docker: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Back / Exit") {
			return
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "View Server Dashboard"):
			showMinecraftDashboard(true)

		case strings.HasPrefix(cleanOpt, "Execute Server Console"):
			modules.InteractiveConsoleCLI()

		case strings.HasPrefix(cleanOpt, "View Server Logs"):
			showLogsSubmenu(true, composeDir)

		case strings.HasPrefix(cleanOpt, "Configure server.properties"):
			configureServerProperties(filepath.Join(core.DockerDataDir, "server.properties"))

		case strings.HasPrefix(cleanOpt, "Optimize Engine Performance"):
			modules.ShowPerformanceTuningMenu()

		case strings.HasPrefix(cleanOpt, "Manage Server Operators"):
			modules.ManagePlayers()

		case strings.HasPrefix(cleanOpt, "Plugin & Addon Manager"):
			modules.ManagePluginsMenu()

		case strings.HasPrefix(cleanOpt, "Mod Manager"):
			modules.ManageModsMenu()

		case strings.HasPrefix(cleanOpt, "World Backups & Automated Schedules"):
			backupAndScheduleSubmenu(true)

		case strings.HasPrefix(cleanOpt, "Restart Docker") || strings.HasPrefix(cleanOpt, "Start Docker"):
			tui.ClearScreen()
			tui.StartSpinner("Restarting Minecraft Docker container...")
			_, _ = system.RestartMinecraftService()
			time.Sleep(2 * time.Second)
			tui.StopSpinner(0)
			tui.PrintInBox("Restart Container", []string{core.GREEN + "[SUCCESS] Docker container restarted." + core.NC})

		case strings.HasPrefix(cleanOpt, "Advanced Maintenance"):
			advancedMaintenanceSubmenu(true, composeDir)
		}
	}
}

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	health := modules.DetectMinecraftHealth()

	if !health.Installed {
		noMinecraftDiagnosticMenu()
		return
	}

	state := system.LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		if health.EnvType != "" {
			envType = health.EnvType
		} else if _, err := os.Stat("/etc/minecraft-docker/docker-compose.yml"); err == nil {
			envType = "docker"
		} else {
			envType = "native"
		}
	}

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd == "" {
			cd = "/etc/minecraft-docker"
		}
		dockerManagementMenu(cd, health)
	} else {
		nativeManagementMenu(health)
	}
}
