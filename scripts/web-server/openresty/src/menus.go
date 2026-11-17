// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: OpenResty High-Performance Web Server & Reverse Proxy (openresty)
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

	"krotek-openresty-installer/core"
	"krotek-openresty-installer/install"
	"krotek-openresty-installer/modules"
	"krotek-openresty-installer/system"
	"krotek-openresty-installer/tui"
)

func openrestyCmd() string {
	if bin := system.GetOpenRestyBinPath(); bin != "" {
		return bin
	}
	return "openresty"
}

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	modules.EnsureOpenRestyWorking()
	health := modules.DetectOpenRestyHealth()

	if !health.Installed {
		noOpenRestyDiagnosticMenu()
		return
	}

	state := system.LoadState()
	composeDir := strings.TrimSpace(state["COMPOSE_DIR"])
	if composeDir == "" || system.ValidateSafeComposeDir(composeDir) != nil {
		composeDir = "/etc/openresty-docker"
	} else {
		composeDir = filepath.Clean(composeDir)
	}

	if health.EnvType == "docker" {
		dockerManagementMenu(composeDir, health)
	} else {
		nativeManagementMenu(health)
	}
}

// dockerManagementMenu provides container lifecycle controls (start/stop/restart/logs)
// for the containerized OpenResty High-Performance Web Server & Reverse Proxy Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.OpenRestyHealth) {
	if composeDir == "" || system.ValidateSafeComposeDir(composeDir) != nil {
		composeDir = "/etc/openresty-docker"
	} else {
		composeDir = filepath.Clean(composeDir)
	}
	for {
		health = modules.DetectOpenRestyHealth()
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart OpenResty Container"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start OpenResty Container  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		options := []string{
			"─── VIRTUAL HOSTS & DOMAINS ───",
			"View Application Dashboard & IP Info",
			"Add virtual host / domain",
			"List configured sites",
			"Edit virtual host / domain config",
			"─── DOCKER STACK MANAGEMENT ───",
			fmt.Sprintf("Edit docker-compose.yml  (%s/docker-compose.yml)", composeDir),
			"Reload OpenResty container",
			restartLabel,
			"View container logs (last 50 lines)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & System Journal",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System & Network Diagnostics",
			"Uninstall OpenResty Docker Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("openresty-manager  -  Management (Docker: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "View Application Dashboard"):
			tui.ClearScreen()
			state := system.LoadState()
			port := system.GetConfiguredPort()
			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "OpenResty (Docker Stack)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Local IP", system.GetLocalIP(), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Public IP", system.GetPublicIP(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("HTTP Port", port+" (Host Port)", core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
			if state["DOMAIN_NAME"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Configured Domain", state["DOMAIN_NAME"], core.GREEN))
			}
			if state["WEB_DIR"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Web Root Directory", state["WEB_DIR"], core.CYAN))
			}
			tui.PrintInBox("OpenResty Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Add virtual host"):
			modules.DoAddDomainDocker(composeDir)

		case strings.HasPrefix(cleanOpt, "List configured sites"):
			tui.ClearScreen()
			var siteInfo []string
			confDir := filepath.Join(composeDir, "conf.d")
			siteInfo = append(siteInfo, core.BOLD+core.YELLOW+fmt.Sprintf("CONFIGS IN DOCKER COMPOSE (%s/):", confDir)+core.NC)
			confCount := 0
			if files, err := filepath.Glob(filepath.Join(confDir, "*.conf")); err == nil {
				for _, f := range files {
					bname := filepath.Base(f)
					if bname == "" {
						continue
					}
					confCount++
					siteInfo = append(siteInfo, fmt.Sprintf("  %s%s%s %s%s%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, bname, core.NC))
				}
			}
			if confCount == 0 {
				siteInfo = append(siteInfo, "  (none found)")
			}
			siteInfo = append(siteInfo, "")
			siteInfo = append(siteInfo, core.BOLD+core.YELLOW+"DOCKER COMPOSE FILE:"+core.NC)
			siteInfo = append(siteInfo, fmt.Sprintf("  %s%s%s %s%s%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, filepath.Join(composeDir, "docker-compose.yml"), core.NC))
			tui.PrintInBox("Configured OpenResty Sites (Docker)", siteInfo)

		case strings.HasPrefix(cleanOpt, "Edit virtual host"):
			tui.ClearScreen()
			confDir := filepath.Join(composeDir, "conf.d")
			var siteFiles []string
			var siteLabels []string
			if files, err := filepath.Glob(filepath.Join(confDir, "*.conf")); err == nil {
				for _, f := range files {
					bname := filepath.Base(f)
					if bname == "" {
						continue
					}
					siteFiles = append(siteFiles, f)
					siteLabels = append(siteLabels, bname)
				}
			}

			if len(siteFiles) == 0 {
				tui.PrintInBox("No Config Files Found", "No site configuration files found in "+confDir+".")
			} else {
				siteLabels = append(siteLabels, "---", "Cancel / Back to Menu")
				sIdx, _, sSt := tui.SelectMenu("openresty-manager - Select Config File (Docker)", siteLabels)
				if sSt == 0 && sIdx < len(siteFiles) {
					selectedFile := siteFiles[sIdx]
					_ = tui.RunEditor(selectedFile)

					var resInfo []string
					if out, err := exec.Command("docker", "exec", "krotek-openresty", "openresty", "-t").CombinedOutput(); err == nil {
						_ = exec.Command("docker", "exec", "krotek-openresty", "openresty", "-s", "reload").Run()
						resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
						resInfo = append(resInfo, tui.FormatKV("Edited File", selectedFile, core.CYAN))
						resInfo = append(resInfo, tui.FormatKV("OpenResty Container", "Syntax OK & Reloaded", core.GREEN))
					} else {
						resInfo = append(resInfo, tui.FormatKV("Operation Status", "FAILED", core.RED))
						resInfo = append(resInfo, tui.FormatKV("Edited File", selectedFile, core.CYAN))
						resInfo = append(resInfo, tui.FormatKV("OpenResty Container", "Config Syntax Test Failed!", core.RED))
						if len(out) > 0 {
							resInfo = append(resInfo, core.DIM+core.SanitizeTerminalText(strings.TrimSpace(string(out)))+core.NC)
						}
					}
					tui.ClearScreen()
					tui.PrintInBox("Edit Config Result", resInfo)
				}
			}

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
					resInfo = append(resInfo, tui.FormatKV("Docker Container", "Recreation failed!", core.RED))
				}
			} else {
				resInfo = append(resInfo, tui.FormatKV("Operation Status", "SAVED", core.GREEN))
				resInfo = append(resInfo, tui.FormatKV("Compose File", composeFile, core.CYAN))
				resInfo = append(resInfo, tui.FormatKV("Docker Container", "No restart requested", core.YELLOW))
			}
			tui.ClearScreen()
			tui.PrintInBox("Docker Compose Edit Result", resInfo)

		case strings.HasPrefix(cleanOpt, "Reload OpenResty container"):
			tui.ClearScreen()
			var resMsg []string
			if exec.Command("docker", "exec", "krotek-openresty", "openresty", "-t").Run() == nil &&
				exec.Command("docker", "exec", "krotek-openresty", "openresty", "-s", "reload").Run() == nil {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("OpenResty Container", "Reloaded successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resMsg = append(resMsg, tui.FormatKV("OpenResty Container", "Reload failed! Container stopped or syntax error.", core.RED))
			}
			tui.PrintInBox("Reload OpenResty Container", resMsg)

		case strings.HasPrefix(cleanOpt, "Restart OpenResty") || strings.HasPrefix(cleanOpt, "Start OpenResty"):
			tui.ClearScreen()
			var resMsg []string
			if exec.Command("docker", "restart", "krotek-openresty").Run() == nil || exec.Command("docker", "start", "krotek-openresty").Run() == nil {
				health.IsRunning = true
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("OpenResty Container", "Started/Restarted successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resMsg = append(resMsg, tui.FormatKV("OpenResty Container", "Restart failed!", core.RED))
			}
			tui.PrintInBox("Restart OpenResty Container", resMsg)

		case strings.HasPrefix(cleanOpt, "View container logs"):
			tui.ClearScreen()
			var logLines []string
			if out, err := exec.Command("docker", "logs", "--tail", "50", "krotek-openresty").CombinedOutput(); err == nil && len(out) > 0 {
				raw := strings.Split(string(out), "\n")
				code2xx := regexp.MustCompile(`" 2[0-9]{2} `)
				codeErr := regexp.MustCompile(`" [4-5][0-9]{2} `)
				errReg := regexp.MustCompile(`\[error\]`)
				for _, line := range raw {
					line = core.SanitizeTerminalText(line)
					if strings.TrimSpace(line) != "" {
						line = code2xx.ReplaceAllString(line, fmt.Sprintf("\" %s$0%s ", core.GREEN, core.NC))
						line = codeErr.ReplaceAllString(line, fmt.Sprintf("\" %s$0%s ", core.RED, core.NC))
						line = errReg.ReplaceAllString(line, core.RED+"[error]"+core.NC)
						logLines = append(logLines, line)
					}
				}
			}
			if len(logLines) == 0 {
				logLines = append(logLines, "No container logs found for krotek-openresty.")
			}
			if len(logLines) > 15 {
				logLines = logLines[len(logLines)-15:]
			}
			tui.PrintInBox("Docker Container Logs (Last 50 lines)", logLines)

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("docker")
			health = modules.DetectOpenRestyHealth()
		case strings.HasPrefix(cleanOpt, "View Service Logs") || strings.HasPrefix(cleanOpt, "View Service"):
			modules.ShowLogsMenu()
		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallOpenResty()
			return
		}
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.OpenRestyHealth) {
	for {
		health = modules.DetectOpenRestyHealth()
		mainConf := system.GetOpenRestyMainConfigPath()
		availDir, enabledDir := system.GetOpenRestyVHostDirs()
		port := system.GetConfiguredPort()
		bin := openrestyCmd()

		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart OpenResty Service"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start OpenResty Service  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		configLabel := fmt.Sprintf("Edit main config  (%s)", mainConf)
		if !health.SyntaxOK {
			configLabel = core.BOLD + core.RED + fmt.Sprintf("Edit main config (%s) -> [SYNTAX ERROR DETECTED]", mainConf) + core.NC
		}

		options := []string{
			"─── VIRTUAL HOSTS & DOMAINS ───",
			"View Application Dashboard & IP Info",
			"Deploy / Reset KrotekNet Landing Page (/var/www/html)",
			"Add virtual host / domain (Presets: Node.js, SPA, PHP, Static)",
			"Generate systemd background service (Node.js/Python/Go)",
			"List configured sites",
			"Edit virtual host / domain config",
			"Enable / Disable site",
			"─── SERVICE MANAGEMENT ───",
			configLabel,
			"Reload OpenResty Service",
			restartLabel,
			"Update OpenResty & Check Upgrades (Safe Backup)",
			"Run Guided Setup / Reinstallation Wizard",

			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & System Journal",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System & Network Diagnostics",
			"Uninstall OpenResty Server",
			"Exit",
		}

		titleStr := fmt.Sprintf("openresty-manager  -  Management (Native: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "View Application Dashboard"):
			tui.ClearScreen()
			state := system.LoadState()
			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "OpenResty Web Platform", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Local IP", system.GetLocalIP(), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Public IP", system.GetPublicIP(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("HTTP Port", port, core.YELLOW))
			if state["ENABLE_SSL"] == "true" {
				dbLines = append(dbLines, tui.FormatKV("HTTPS Port", "443", core.YELLOW))
			} else {
				dbLines = append(dbLines, tui.FormatKV("HTTPS Port", "443 (TLS ready)", core.DIM))
			}
			dbLines = append(dbLines, tui.FormatKV("Main Config", mainConf, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("OpenResty Binary", bin, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Default Web Root", "/var/www/html", core.CYAN))
			if state["DOMAIN_NAME"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Configured Domain", state["DOMAIN_NAME"], core.GREEN))
			}
			if state["WEB_DIR"] != "" && state["WEB_DIR"] != "/var/www/html" {
				dbLines = append(dbLines, tui.FormatKV("Domain Web Root", state["WEB_DIR"], core.CYAN))
			}
			tui.PrintInBox("OpenResty Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Deploy / Reset KrotekNet"):
			tui.ClearScreen()
			if tui.AskYesNo("Deploy / Overwrite KrotekNet Landing Page in /var/www/html/index.html?") != 0 {
				continue
			}
			webUser := system.GetRuntimeWebUser()
			webDir := "/var/www/html"
			_ = os.MkdirAll(webDir, 0755)
			system.SecureWebRootPermissions(webDir, webUser)

			defaultHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
				ServiceName: "OpenResty Web Platform",
				LogoText:    "OR",
				HeroTitle:   "OpenResty Lua Platform Active",
				HeroDesc:    "Your high-performance dynamic web platform integrating enhanced Nginx core, LuaJIT, and OPM ecosystem is operational.",
				EnvType:     "native",
				Port:        port,
				WebDir:      webDir,
				ConfigPath:  mainConf,
				WebUser:     webUser,
				ManagerCmd:  "openresty-manager",
			})

			_ = os.Remove(filepath.Join(webDir, "index.nginx-debian.html"))
			idxPath := filepath.Join(webDir, "index.html")
			_ = system.WriteFileNoFollow(idxPath, []byte(defaultHTML), 0644, webUser)

			system.ReloadService("openresty")

			accessURL := fmt.Sprintf("http://%s/", system.GetLocalIP())
			if port != "" && port != "80" {
				accessURL = fmt.Sprintf("http://%s:%s/", system.GetLocalIP(), port)
			}

			var successLines []string
			successLines = append(successLines, core.BOLD+core.GREEN+core.TagOK() + " KrotekNet Landing Page deployed successfully!"+core.NC)
			successLines = append(successLines, "")
			successLines = append(successLines, tui.FormatKV("Document Root", webDir, core.CYAN))
			successLines = append(successLines, tui.FormatKV("Landing File", idxPath, core.GREEN))
			successLines = append(successLines, tui.FormatKV("File Ownership", webUser+":"+webUser, core.YELLOW))
			successLines = append(successLines, tui.FormatKV("Access URL", accessURL, core.CYAN+core.BOLD))
			tui.PrintInBox("Landing Page Deployed", successLines)

		case strings.HasPrefix(cleanOpt, "Add virtual host"):
			modules.DoAddDomain()

		case strings.HasPrefix(cleanOpt, "Generate systemd"):
			modules.GenerateSystemdServiceWizard()

		case strings.HasPrefix(cleanOpt, "List configured sites"):
			tui.ClearScreen()
			var siteInfo []string
			if core.SysInfo.Family == "debian" {
				siteInfo = append(siteInfo, core.BOLD+core.YELLOW+fmt.Sprintf("SITES AVAILABLE (%s/):", availDir)+core.NC)
				availCount := 0
				if files, err := filepath.Glob(filepath.Join(availDir, "*")); err == nil {
					for _, f := range files {
						fi, err := os.Stat(f)
						if err != nil || fi.IsDir() {
							continue
						}
						bname := filepath.Base(f)
						availCount++
						enabledPath := filepath.Join(enabledDir, bname)
						if _, err := os.Lstat(enabledPath); err == nil {
							siteInfo = append(siteInfo, fmt.Sprintf("  %s%s%s %s%-24s%s : %s[ENABLED]%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, bname, core.NC, core.GREEN, core.NC))
						} else {
							siteInfo = append(siteInfo, fmt.Sprintf("  %s%s%s %s%-24s%s : %s[DISABLED]%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, bname, core.NC, core.YELLOW, core.NC))
						}
					}
				}
				if availCount == 0 {
					siteInfo = append(siteInfo, "  (none)")
				}
			} else {
				siteInfo = append(siteInfo, core.BOLD+core.YELLOW+fmt.Sprintf("CONFIGS IN %s/:", availDir)+core.NC)
				confCount := 0
				if files, err := filepath.Glob(filepath.Join(availDir, "*.conf")); err == nil {
					for _, f := range files {
						fi, err := os.Stat(f)
						if err != nil || fi.IsDir() {
							continue
						}
						bname := filepath.Base(f)
						confCount++
						siteInfo = append(siteInfo, fmt.Sprintf("  %s%s%s %s%s%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, bname, core.NC))
					}
				}
				if confCount == 0 {
					siteInfo = append(siteInfo, "  (none)")
				}
			}
			tui.PrintInBox("Configured OpenResty Sites", siteInfo)

		case strings.HasPrefix(cleanOpt, "Edit virtual host"):
			tui.ClearScreen()
			globPattern := filepath.Join(availDir, "*.conf")
			if core.SysInfo.Family == "debian" {
				globPattern = filepath.Join(availDir, "*")
			}

			var siteFiles []string
			var siteLabels []string
			if files, err := filepath.Glob(globPattern); err == nil {
				for _, f := range files {
					fi, err := os.Stat(f)
					if err != nil || fi.IsDir() {
						continue
					}
					bname := filepath.Base(f)
					siteFiles = append(siteFiles, f)
					statusTag := ""
					if core.SysInfo.Family == "debian" {
						enabledPath := filepath.Join(enabledDir, bname)
						if _, err := os.Lstat(enabledPath); err == nil {
							statusTag = fmt.Sprintf("  %s[ENABLED]%s", core.GREEN, core.NC)
						} else {
							statusTag = fmt.Sprintf("  %s[DISABLED]%s", core.YELLOW, core.NC)
						}
					}
					siteLabels = append(siteLabels, bname+statusTag)
				}
			}

			if len(siteFiles) == 0 {
				tui.PrintInBox("No Config Files Found", "No site configuration files found in "+availDir+".")
			} else {
				siteLabels = append(siteLabels, "---", "Cancel / Back to Menu")
				sIdx, _, sSt := tui.SelectMenu("openresty-manager - Select Config File", siteLabels)
				if sSt == 0 && sIdx < len(siteFiles) {
					selectedFile := siteFiles[sIdx]
					_ = tui.RunEditor(selectedFile)

					var resInfo []string
					if out, err := exec.Command(bin, "-t").CombinedOutput(); err == nil {
						system.ReloadService("openresty")
						resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
						resInfo = append(resInfo, tui.FormatKV("Edited File", selectedFile, core.CYAN))
						resInfo = append(resInfo, tui.FormatKV("OpenResty Service", "Syntax OK & Service Reloaded", core.GREEN))
					} else {
						resInfo = append(resInfo, tui.FormatKV("Operation Status", "FAILED", core.RED))
						resInfo = append(resInfo, tui.FormatKV("Edited File", selectedFile, core.CYAN))
						resInfo = append(resInfo, tui.FormatKV("OpenResty Service", "Config Syntax Test Failed!", core.RED))
						resInfo = append(resInfo, "")
						resInfo = append(resInfo, core.BOLD+core.YELLOW+"SYNTAX ERROR DETAILS:"+core.NC)
						for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
							if cleanLine := core.SanitizeTerminalText(line); cleanLine != "" {
								resInfo = append(resInfo, "  "+core.RED+cleanLine+core.NC)
							}
						}
					}
					tui.ClearScreen()
					tui.PrintInBox("Edit Config Result", resInfo)
				}
			}

		case strings.HasPrefix(cleanOpt, "Edit main config"):
			tui.ClearScreen()
			_ = tui.RunEditor(mainConf)

			var resInfo []string
			if out, err := exec.Command(bin, "-t").CombinedOutput(); err == nil {
				system.ReloadService("openresty")
				resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resInfo = append(resInfo, tui.FormatKV("Config File", mainConf, core.CYAN))
				resInfo = append(resInfo, tui.FormatKV("OpenResty Service", "Syntax OK & Service Reloaded", core.GREEN))
			} else {
				resInfo = append(resInfo, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resInfo = append(resInfo, tui.FormatKV("Config File", mainConf, core.CYAN))
				resInfo = append(resInfo, tui.FormatKV("OpenResty Service", "Main Config Syntax Test Failed!", core.RED))
				if len(out) > 0 {
					resInfo = append(resInfo, core.RED+core.SanitizeTerminalText(strings.TrimSpace(string(out)))+core.NC)
				}
			}
			tui.ClearScreen()
			tui.PrintInBox("Main Config Result", resInfo)

		case strings.HasPrefix(cleanOpt, "Enable / Disable"):
			tui.ClearScreen()
			if core.SysInfo.Family == "debian" {
				var availFiles []string
				var siteNames []string
				if files, err := filepath.Glob(filepath.Join(availDir, "*")); err == nil {
					for _, f := range files {
						fi, err := os.Stat(f)
						if err != nil || fi.IsDir() {
							continue
						}
						bname := filepath.Base(f)
						siteNames = append(siteNames, bname)
						enabledPath := filepath.Join(enabledDir, bname)
						statusTag := core.YELLOW + "[DISABLED - Click to Enable]" + core.NC
						if _, err := os.Lstat(enabledPath); err == nil {
							statusTag = core.GREEN + "[ENABLED - Click to Disable]" + core.NC
						}
						availFiles = append(availFiles, fmt.Sprintf("%s  ->  %s", bname, statusTag))
					}
				}
				if len(availFiles) == 0 {
					tui.PrintInBox("Sites Toggle", "No site files available to toggle in "+availDir+".")
				} else {
					availFiles = append(availFiles, "---", "Cancel / Back")
					sIdx, _, sSt := tui.SelectMenu("Toggle Virtual Host Active State", availFiles)
					if sSt == 0 && sIdx < len(siteNames) {
						targetSite := siteNames[sIdx]
						enabledPath := filepath.Join(enabledDir, targetSite)
						availPath := filepath.Join(availDir, targetSite)
						_ = os.MkdirAll(enabledDir, 0755)
						var resInfo []string
						if _, err := os.Lstat(enabledPath); err == nil {
							_ = os.Remove(enabledPath)
							resInfo = append(resInfo, tui.FormatKV("Site Status", fmt.Sprintf("Disabled (%s)", targetSite), core.YELLOW))
						} else {
							_ = os.Symlink(availPath, enabledPath)
							resInfo = append(resInfo, tui.FormatKV("Site Status", fmt.Sprintf("Enabled (%s)", targetSite), core.GREEN))
						}
						if exec.Command(bin, "-t").Run() == nil {
							system.ReloadService("openresty")
							resInfo = append(resInfo, tui.FormatKV("OpenResty Service", "Syntax OK & Reloaded", core.GREEN))
						} else {
							resInfo = append(resInfo, tui.FormatKV("OpenResty Service", "Config Syntax Warning!", core.RED))
						}
						tui.ClearScreen()
						tui.PrintInBox("Site Toggle Result", resInfo)
					}
				}
			} else {
				tui.PrintInBox("Enable/Disable Notice", fmt.Sprintf("Enable/disable site is managed via *.conf files in %s/ on RHEL and Alpine.", availDir))
			}

		case strings.HasPrefix(cleanOpt, "Reload OpenResty Service"):
			tui.ClearScreen()
			modules.EnsureOpenRestyWorking()
			var resMsg []string
			if exec.Command(bin, "-t").Run() == nil && system.ReloadService("openresty") {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("OpenResty Service", "Reloaded successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resMsg = append(resMsg, tui.FormatKV("OpenResty Service", fmt.Sprintf("Reload failed! Run '%s -t'", bin), core.RED))
			}
			tui.PrintInBox("Reload OpenResty Status", resMsg)

		case strings.HasPrefix(cleanOpt, "Restart OpenResty") || strings.HasPrefix(cleanOpt, "Start OpenResty"):
			tui.ClearScreen()
			modules.EnsureOpenRestyWorking()
			system.EnsureOpenRestyRuntimeDirs(system.GetRuntimeWebUser())
			var resMsg []string
			success := false
			if !health.IsRunning {
				success = system.StartService("openresty")
			}
			if !success {
				success = system.RestartService("openresty")
			}
			if success {
				health = modules.DetectOpenRestyHealth()
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("OpenResty Service", "Started/Restarted successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resMsg = append(resMsg, tui.FormatKV("OpenResty Service", "Failed to start/restart service!", core.RED))
				if core.LastServiceError != "" {
					resMsg = append(resMsg, "")
					resMsg = append(resMsg, core.BOLD+core.RED+"LAST JOURNAL / DAEMON ERROR LOGS:"+core.NC)
					for _, line := range strings.Split(core.LastServiceError, "\n") {
						trimmed := core.SanitizeTerminalText(strings.TrimSpace(line))
						if trimmed != "" {
							if len(trimmed) > 85 {
								trimmed = trimmed[:85] + "..."
							}
							resMsg = append(resMsg, "  "+trimmed)
						}
					}
				} else {
					resMsg = append(resMsg, tui.FormatKV("Diagnostics Tip", "Run 'sudo journalctl -xeu openresty.service' for full log", core.YELLOW))
				}
			}
			tui.PrintInBox("Restart OpenResty Status", resMsg)

		case strings.HasPrefix(cleanOpt, "Update OpenResty") || strings.HasPrefix(cleanOpt, "Update Service"):
			system.RunServiceUpdateFlow()

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			health = modules.DetectOpenRestyHealth()
		case strings.HasPrefix(cleanOpt, "View Service Logs") || strings.HasPrefix(cleanOpt, "View Service"):
			modules.ShowLogsMenu()
		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallOpenResty()
			return
		}
	}
}

// ==============================================================================
// STEP-BY-STEP INTERACTIVE SETUP WIZARD
// ==============================================================================
// noOpenRestyDiagnosticMenu runs when no active or configured OpenResty
// installation is detected, offering binary adoption, Docker/native wizards,
// and deep diagnostics.

func noOpenRestyDiagnosticMenu() {
	for {
		tui.ClearScreen()
		binPath := system.GetOpenRestyBinPath()
		hasBinary := binPath != ""
		statusText := "Status: OpenResty web platform is not provisioned on this machine yet."
		if hasBinary {
			statusText = "Status: OpenResty binary detected on machine. You can adopt it or run fresh setup."
		}

		binBadge := core.RED + "[NOT INSTALLED]" + core.NC
		if hasBinary {
			binBadge = core.GREEN + "[FOUND]" + core.NC + " (" + binPath + ")"
		}

		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("OpenResty Service", func() string {
				if hasBinary {
					return core.YELLOW + "[UNREGISTERED / STOPPED]" + core.NC
				}
				return core.RED + "[NOT INSTALLED]" + core.NC
			}(), core.RED),
			tui.FormatKV("OpenResty Binary", binBadge, core.YELLOW),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + statusText + core.NC,
			"Choose an action below to deploy OpenResty or run system network diagnostics.",
		}

		var options []string
		if hasBinary {
			options = append(options, "Adopt Existing OpenResty & Enter Management Panel")
		}
		options = append(options,
			"Install OpenResty Docker (Containerized Stack - Recommended)",
			"Install OpenResty Native (Clean Installation + Firewall)",
			"Run Deep System & Network Diagnostics Scan",
			"Exit Diagnostics",
		)

		_, selectedOpt, status := tui.SelectMenuWithContent("openresty-manager - System Diagnostics", diagLines, options)
		cleanOpt := core.StripANSI(selectedOpt)
		if status == 2 || (status == 0 && cleanOpt == "Exit Diagnostics") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		switch {
		case strings.HasPrefix(cleanOpt, "Adopt Existing"):
			system.SaveState(map[string]string{
				"ACTION":   "adopted",
				"ENV_TYPE": "native",
				"PORT":     system.GetConfiguredPort(),
			})
			managementMenu()
			return
		case strings.HasPrefix(cleanOpt, "Install OpenResty Docker"):
			install.FreshInstallWizard("docker")
			return
		case strings.HasPrefix(cleanOpt, "Install OpenResty Native"):
			install.FreshInstallWizard("native")
			return
		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()
		}
	}
}
