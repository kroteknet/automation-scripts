// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: Nginx High-Performance Web Server & Reverse Proxy (nginx)
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

	"krotek-nginx-installer/core"
	"krotek-nginx-installer/install"
	"krotek-nginx-installer/modules"
	"krotek-nginx-installer/system"
	"krotek-nginx-installer/tui"
)

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	modules.EnsureNginxWorking()
	health := modules.DetectNginxHealth()

	if !health.Installed {
		noNginxDiagnosticMenu()
		return
	}

	state := system.LoadState()
	composeDir := state["COMPOSE_DIR"]
	if composeDir == "" {
		composeDir = "/etc/nginx-docker"
	}

	if health.EnvType == "docker" {
		dockerManagementMenu(composeDir, health)
	} else {
		nativeManagementMenu(health)
	}
}

// dockerManagementMenu provides container lifecycle controls (start/stop/restart/logs)
// for the containerized Nginx High-Performance Web Server & Reverse Proxy Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.NginxHealth) {
	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart NGINX Container"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start NGINX Container  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		options := []string{
			"─── VIRTUAL HOSTS & DOMAINS ───",
			"View Application Dashboard & IP Info",
			"Add virtual host / domain",
			"List configured sites",
			"Edit virtual host / domain config",
			"─── DOCKER STACK MANAGEMENT ───",
			fmt.Sprintf("Edit docker-compose.yml  (%s/docker-compose.yml)", composeDir),
			"Reload NGINX container",
			restartLabel,
			"View container logs (last 50 lines)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & System Journal",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System & Network Diagnostics",
			"Uninstall NGINX Docker Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("nginx-manager  -  Management (Docker: %s)", statusBadge)
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
			dbLines = append(dbLines, tui.FormatKV("Application", "NGINX (Docker Stack)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Local IP", system.GetLocalIP(), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Public IP", system.GetPublicIP(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("HTTP Port", "80 (Host Port)", core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
			if state["DOMAIN_NAME"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Configured Domain", state["DOMAIN_NAME"], core.GREEN))
			}
			if state["WEB_DIR"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Web Root Directory", state["WEB_DIR"], core.CYAN))
			}
			tui.PrintInBox("NGINX Dashboard Info", dbLines)

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
			tui.PrintInBox("Configured NGINX Sites (Docker)", siteInfo)

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
				sIdx, _, sSt := tui.SelectMenu("nginx-manager - Select Config File (Docker)", siteLabels)
				if sSt == 0 && sIdx < len(siteFiles) {
					selectedFile := siteFiles[sIdx]
					_ = tui.RunEditor(selectedFile)

					var resInfo []string
					if exec.Command("docker", "exec", "krotek-nginx", "nginx", "-t").Run() == nil {
						_ = exec.Command("docker", "exec", "krotek-nginx", "nginx", "-s", "reload").Run()
						resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
						resInfo = append(resInfo, tui.FormatKV("Edited File", selectedFile, core.CYAN))
						resInfo = append(resInfo, tui.FormatKV("Nginx Container", "Syntax OK & Reloaded", core.GREEN))
					} else {
						resInfo = append(resInfo, tui.FormatKV("Operation Status", "FAILED", core.RED))
						resInfo = append(resInfo, tui.FormatKV("Edited File", selectedFile, core.CYAN))
						resInfo = append(resInfo, tui.FormatKV("Nginx Container", "Config Syntax Test Failed!", core.RED))
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
				cmdStr := fmt.Sprintf("cd %s && docker compose up -d 2>/dev/null || docker-compose up -d", composeDir)
				if system.RunMultiStep("Recreating Docker stack", cmdStr) {
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

		case strings.HasPrefix(cleanOpt, "Reload NGINX container"):
			tui.ClearScreen()
			var resMsg []string
			if exec.Command("docker", "exec", "krotek-nginx", "nginx", "-s", "reload").Run() == nil {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("Nginx Container", "Reloaded successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resMsg = append(resMsg, tui.FormatKV("Nginx Container", "Reload failed! Container stopped or syntax error.", core.RED))
			}
			tui.PrintInBox("Reload NGINX Container", resMsg)

		case strings.HasPrefix(cleanOpt, "Restart NGINX") || strings.HasPrefix(cleanOpt, "Start NGINX"):
			tui.ClearScreen()
			var resMsg []string
			if exec.Command("docker", "restart", "krotek-nginx").Run() == nil || exec.Command("docker", "start", "krotek-nginx").Run() == nil {
				health.IsRunning = true
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("Nginx Container", "Started/Restarted successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resMsg = append(resMsg, tui.FormatKV("Nginx Container", "Restart failed!", core.RED))
			}
			tui.PrintInBox("Restart NGINX Container", resMsg)

		case strings.HasPrefix(cleanOpt, "View container logs"):
			tui.ClearScreen()
			var logLines []string
			if out, err := exec.Command("docker", "logs", "--tail", "50", "krotek-nginx").CombinedOutput(); err == nil && len(out) > 0 {
				raw := strings.Split(string(out), "\n")
				code2xx := regexp.MustCompile(`" 2[0-9]{2} `)
				codeErr := regexp.MustCompile(`" [4-5][0-9]{2} `)
				errReg := regexp.MustCompile(`\[error\]`)
				for _, line := range raw {
					if strings.TrimSpace(line) != "" {
						line = code2xx.ReplaceAllString(line, fmt.Sprintf("\" %s$0%s ", core.GREEN, core.NC))
						line = codeErr.ReplaceAllString(line, fmt.Sprintf("\" %s$0%s ", core.RED, core.NC))
						line = errReg.ReplaceAllString(line, core.RED+"[error]"+core.NC)
						logLines = append(logLines, line)
					}
				}
			}
			if len(logLines) == 0 {
				logLines = append(logLines, "No container logs found for krotek-nginx.")
			}
			if len(logLines) > 15 {
				logLines = logLines[len(logLines)-15:]
			}
			tui.PrintInBox("Docker Container Logs (Last 50 lines)", logLines)

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("docker")
			health = modules.DetectNginxHealth()
		case strings.HasPrefix(cleanOpt, "View Service Logs") || strings.HasPrefix(cleanOpt, "View Service"):
			modules.ShowLogsMenu()
		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallNginx()
			return
		}
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.NginxHealth) {
	for {
		health = modules.DetectNginxHealth()
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart NGINX Service"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start NGINX Service  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		configLabel := "Edit main config  (/etc/nginx/nginx.conf)"
		if !health.SyntaxOK {
			configLabel = core.BOLD + core.RED + "Edit main config  ->  [SYNTAX ERROR DETECTED]" + core.NC
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
			"Reload NGINX Service",
			restartLabel,
			"Update NGINX & Check Upgrades (Safe Backup)",
			"Run Guided Setup / Reinstallation Wizard",

			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & System Journal",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System & Network Diagnostics",
			"Uninstall NGINX Server",
			"Exit",
		}

		titleStr := fmt.Sprintf("nginx-manager  -  Management (Native: %s)", statusBadge)
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
			dbLines = append(dbLines, tui.FormatKV("Application", "NGINX Web Server", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Local IP", system.GetLocalIP(), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Public IP", system.GetPublicIP(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("HTTP Port", "80", core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("HTTPS Port", "443", core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("Main Config", "/etc/nginx/nginx.conf", core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Default Web Root", "/var/www/html", core.CYAN))
			if state["DOMAIN_NAME"] != "" {
				dbLines = append(dbLines, tui.FormatKV("Configured Domain", state["DOMAIN_NAME"], core.GREEN))
			}
			if state["WEB_DIR"] != "" && state["WEB_DIR"] != "/var/www/html" {
				dbLines = append(dbLines, tui.FormatKV("Domain Web Root", state["WEB_DIR"], core.CYAN))
			}
			tui.PrintInBox("NGINX Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Deploy / Reset KrotekNet"):
			tui.ClearScreen()
			if tui.AskYesNo("Deploy / Overwrite KrotekNet Landing Page in /var/www/html/index.html?") != 0 {
				continue
			}
			webUser := "www-data"
			if core.SysInfo.Family != "debian" {
				webUser = "nginx"
			}
			webDir := "/var/www/html"
			_ = os.MkdirAll(webDir, 0755)
			_ = exec.Command("chmod", "755", "/var").Run()
			_ = exec.Command("chmod", "755", "/var/www").Run()
			system.SecureWebRootPermissions(webDir, webUser)

			defaultHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
				ServiceName: "NGINX Web Server",
				LogoText:    "N",
				HeroTitle:   "NGINX High-Performance Engine Active",
				HeroDesc:    "Your production-grade, event-driven HTTP/2 & reverse proxy web server is fully operational.",
				EnvType:     "native",
				Port:        "80",
				WebDir:      webDir,
				ConfigPath:  "/etc/nginx/nginx.conf",
				WebUser:     webUser,
				ManagerCmd:  "nginx-manager",
			})

			_ = os.Remove(filepath.Join(webDir, "index.nginx-debian.html"))
			idxPath := filepath.Join(webDir, "index.html")
			_ = system.WriteFileNoFollow(idxPath, []byte(defaultHTML), 0644, webUser)

			system.ReloadService("nginx")

			var successLines []string
			successLines = append(successLines, core.BOLD+core.GREEN+core.TagOK()+" KrotekNet Landing Page deployed successfully!"+core.NC)
			successLines = append(successLines, "")
			successLines = append(successLines, tui.FormatKV("Document Root", webDir, core.CYAN))
			successLines = append(successLines, tui.FormatKV("Landing File", idxPath, core.GREEN))
			successLines = append(successLines, tui.FormatKV("File Ownership", webUser+":"+webUser, core.YELLOW))
			successLines = append(successLines, tui.FormatKV("Access URL", fmt.Sprintf("http://%s/", system.GetLocalIP()), core.CYAN+core.BOLD))
			successLines = append(successLines, "")
			successLines = append(successLines, core.DIM+"Debian default placeholder (index.nginx-debian.html) has been removed."+core.NC)
			tui.PrintInBox("Landing Page Deployed", successLines)

		case strings.HasPrefix(cleanOpt, "Add virtual host"):
			modules.DoAddDomain()

		case strings.HasPrefix(cleanOpt, "Generate systemd"):
			modules.GenerateSystemdServiceWizard()

		case strings.HasPrefix(cleanOpt, "List configured sites"):
			tui.ClearScreen()
			var siteInfo []string
			if core.SysInfo.Family == "debian" {
				siteInfo = append(siteInfo, core.BOLD+core.YELLOW+"SITES AVAILABLE (/etc/nginx/sites-available/):"+core.NC)
				availCount := 0
				if files, err := filepath.Glob("/etc/nginx/sites-available/*"); err == nil {
					for _, f := range files {
						bname := filepath.Base(f)
						if bname == "" {
							continue
						}
						availCount++
						enabledPath := filepath.Join("/etc/nginx/sites-enabled", bname)
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
				headerTitle := "CONFIGS IN /etc/nginx/conf.d/:"
				searchDir := "/etc/nginx/conf.d"
				if core.SysInfo.Family == "alpine" {
					headerTitle = "CONFIGS IN /etc/nginx/http.d/:"
					searchDir = "/etc/nginx/http.d"
				}
				siteInfo = append(siteInfo, core.BOLD+core.YELLOW+headerTitle+core.NC)
				confCount := 0
				if files, err := filepath.Glob(filepath.Join(searchDir, "*.conf")); err == nil {
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
					siteInfo = append(siteInfo, "  (none)")
				}
			}
			tui.PrintInBox("Configured NGINX Sites", siteInfo)

		case strings.HasPrefix(cleanOpt, "Edit virtual host"):
			tui.ClearScreen()
			searchDir := "/etc/nginx/conf.d"
			if core.SysInfo.Family == "debian" {
				searchDir = "/etc/nginx/sites-available"
			} else if core.SysInfo.Family == "alpine" {
				searchDir = "/etc/nginx/http.d"
			}

			var siteFiles []string
			var siteLabels []string
			if files, err := filepath.Glob(filepath.Join(searchDir, "*")); err == nil {
				for _, f := range files {
					bname := filepath.Base(f)
					if bname == "" {
						continue
					}
					siteFiles = append(siteFiles, f)
					statusTag := ""
					if core.SysInfo.Family == "debian" {
						enabledPath := filepath.Join("/etc/nginx/sites-enabled", bname)
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
				tui.PrintInBox("No Config Files Found", "No site configuration files found in "+searchDir+".")
			} else {
				siteLabels = append(siteLabels, "---", "Cancel / Back to Menu")
				sIdx, _, sSt := tui.SelectMenu("nginx-manager - Select Config File", siteLabels)
				if sSt == 0 && sIdx < len(siteFiles) {
					selectedFile := siteFiles[sIdx]
					_ = tui.RunEditor(selectedFile)

					var resInfo []string
					if out, err := exec.Command("nginx", "-t").CombinedOutput(); err == nil {
						system.ReloadService("nginx")
						resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
						resInfo = append(resInfo, tui.FormatKV("Edited File", selectedFile, core.CYAN))
						resInfo = append(resInfo, tui.FormatKV("Nginx Service", "Syntax OK & Service Reloaded", core.GREEN))
					} else {
						resInfo = append(resInfo, tui.FormatKV("Operation Status", "FAILED", core.RED))
						resInfo = append(resInfo, tui.FormatKV("Edited File", selectedFile, core.CYAN))
						resInfo = append(resInfo, tui.FormatKV("Nginx Service", "Config Syntax Test Failed!", core.RED))
						resInfo = append(resInfo, "")
						resInfo = append(resInfo, core.BOLD+core.YELLOW+"SYNTAX ERROR DETAILS:"+core.NC)
						for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
							if line != "" {
								resInfo = append(resInfo, "  "+core.RED+line+core.NC)
							}
						}
					}
					tui.ClearScreen()
					tui.PrintInBox("Edit Config Result", resInfo)
				}
			}

		case strings.HasPrefix(cleanOpt, "Edit main config"):
			tui.ClearScreen()
			_ = tui.RunEditor("/etc/nginx/nginx.conf")

			var resInfo []string
			if exec.Command("nginx", "-t").Run() == nil {
				system.ReloadService("nginx")
				resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resInfo = append(resInfo, tui.FormatKV("Config File", "/etc/nginx/nginx.conf", core.CYAN))
				resInfo = append(resInfo, tui.FormatKV("Nginx Service", "Syntax OK & Service Reloaded", core.GREEN))
			} else {
				resInfo = append(resInfo, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resInfo = append(resInfo, tui.FormatKV("Config File", "/etc/nginx/nginx.conf", core.CYAN))
				resInfo = append(resInfo, tui.FormatKV("Nginx Service", "Main Config Syntax Test Failed!", core.RED))
			}
			tui.ClearScreen()
			tui.PrintInBox("Main Config Result", resInfo)

		case strings.HasPrefix(cleanOpt, "Enable / Disable"):
			tui.ClearScreen()
			if core.SysInfo.Family == "debian" {
				searchDir := "/etc/nginx/sites-available"
				var siteNames []string
				var siteOptions []string
				if files, err := filepath.Glob(filepath.Join(searchDir, "*")); err == nil {
					for _, f := range files {
						bname := filepath.Base(f)
						if bname == "" {
							continue
						}
						siteNames = append(siteNames, bname)
						enabledPath := filepath.Join("/etc/nginx/sites-enabled", bname)
						if _, err := os.Lstat(enabledPath); err == nil {
							siteOptions = append(siteOptions, fmt.Sprintf("%s  %s-> [ENABLED - Click to Disable]%s", bname, core.GREEN, core.NC))
						} else {
							siteOptions = append(siteOptions, fmt.Sprintf("%s  %s-> [DISABLED - Click to Enable]%s", bname, core.YELLOW, core.NC))
						}
					}
				}
				var availFiles []string
				if files, err := filepath.Glob("/etc/nginx/sites-available/*"); err == nil {
					for _, f := range files {
						bname := filepath.Base(f)
						if bname == "" {
							continue
						}
						enabledPath := filepath.Join("/etc/nginx/sites-enabled", bname)
						statusTag := core.RED + "[DISABLED]" + core.NC
						if _, err := os.Lstat(enabledPath); err == nil {
							statusTag = core.GREEN + "[ENABLED]" + core.NC
						}
						availFiles = append(availFiles, fmt.Sprintf("%s  ->  %s", bname, statusTag))
					}
				}
				if len(availFiles) == 0 {
					tui.PrintInBox("Sites Toggle", "No site files available to toggle.")
				} else {
					availFiles = append(availFiles, "---", "Cancel / Back")
					sIdx, _, sSt := tui.SelectMenu("Toggle Virtual Host Active State", availFiles)
					if sSt == 0 && sIdx < len(availFiles)-2 {
						targetSite := strings.TrimSpace(strings.Split(core.StripANSI(availFiles[sIdx]), "->")[0])
						enabledPath := filepath.Join("/etc/nginx/sites-enabled", targetSite)
						availPath := filepath.Join("/etc/nginx/sites-available", targetSite)
						var resInfo []string
						if _, err := os.Lstat(enabledPath); err == nil {
							_ = os.Remove(enabledPath)
							resInfo = append(resInfo, tui.FormatKV("Site Status", fmt.Sprintf("Disabled (%s)", targetSite), core.YELLOW))
						} else {
							_ = os.Symlink(availPath, enabledPath)
							resInfo = append(resInfo, tui.FormatKV("Site Status", fmt.Sprintf("Enabled (%s)", targetSite), core.GREEN))
						}
						if exec.Command("nginx", "-t").Run() == nil {
							system.ReloadService("nginx")
							resInfo = append(resInfo, tui.FormatKV("Nginx Service", "Syntax OK & Reloaded", core.GREEN))
						} else {
							resInfo = append(resInfo, tui.FormatKV("Nginx Service", "Config Syntax Warning!", core.RED))
						}
						tui.ClearScreen()
						tui.PrintInBox("Site Toggle Result", resInfo)
					}
				}
			} else {
				tui.PrintInBox("Enable/Disable Notice", "Enable/disable site is managed via files in /etc/nginx/conf.d/ on RHEL and Alpine.")
			}

		case strings.HasPrefix(cleanOpt, "Reload NGINX Service"):
			tui.ClearScreen()
			modules.EnsureNginxWorking()
			var resMsg []string
			if exec.Command("nginx", "-t").Run() == nil && system.ReloadService("nginx") {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("Nginx Service", "Reloaded successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resMsg = append(resMsg, tui.FormatKV("Nginx Service", "Reload failed! Run 'nginx -t'", core.RED))
			}
			tui.PrintInBox("Reload NGINX Status", resMsg)

		case strings.HasPrefix(cleanOpt, "Restart NGINX") || strings.HasPrefix(cleanOpt, "Start NGINX"):
			tui.ClearScreen()
			modules.EnsureNginxWorking()
			var resMsg []string
			_ = os.MkdirAll("/run/nginx", 0755)
			_ = os.MkdirAll("/var/log/nginx", 0755)
			if core.SysInfo.Family == "alpine" {
				_ = exec.Command("chown", "-R", "nginx:nginx", "/run/nginx").Run()
				_ = exec.Command("chmod", "775", "/run/nginx").Run()
			} else if core.SysInfo.Family == "debian" {
				_ = exec.Command("chown", "-R", "www-data:adm", "/var/log/nginx").Run()
			}
			success := false
			if !health.IsRunning {
				success = system.StartService("nginx")
			}
			if !success {
				success = system.RestartService("nginx")
			}
			if success {
				health = modules.DetectNginxHealth()
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("Nginx Service", "Started/Restarted successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resMsg = append(resMsg, tui.FormatKV("Nginx Service", "Failed to start/restart service!", core.RED))
				if core.LastServiceError != "" {
					resMsg = append(resMsg, "")
					resMsg = append(resMsg, core.BOLD+core.RED+"LAST JOURNAL / DAEMON ERROR LOGS:"+core.NC)
					for _, line := range strings.Split(core.LastServiceError, "\n") {
						trimmed := core.SanitizeTerminalText(strings.TrimSpace(line))
						if trimmed != "" {
							if len(trimmed) > 85 {
								trimmed = trimmed[:85] + "..."
							}
							resMsg = append(resMsg, "  "+core.StripANSI(trimmed))
						}
					}
				} else {
					resMsg = append(resMsg, tui.FormatKV("Diagnostics Tip", "Run 'sudo journalctl -xeu nginx.service' for full log", core.YELLOW))
				}
			}
			tui.PrintInBox("Restart NGINX Status", resMsg)

		case strings.HasPrefix(cleanOpt, "Update NGINX") || strings.HasPrefix(cleanOpt, "Update Service"):
			system.RunServiceUpdateFlow()

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			health = modules.DetectNginxHealth()
		case strings.HasPrefix(cleanOpt, "View Service Logs") || strings.HasPrefix(cleanOpt, "View Service"):
			modules.ShowLogsMenu()
		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallNginx()
			return
		}
	}
}

// ==============================================================================
// STEP-BY-STEP INTERACTIVE SETUP WIZARD
// ==============================================================================
// freshInstallWizard runs the interactive installation setup wizard.
// It guides the user through configuration preset selection, credential setup,
// and a final confirmation summary before dispatching to the appropriate installer.

func noNginxDiagnosticMenu() {
	for {
		tui.ClearScreen()
		hasBinary := system.CommandExists("nginx")
		statusText := "Status: NGINX web server is not provisioned on this machine yet."
		if hasBinary {
			statusText = "Status: NGINX binary detected on machine. You can adopt it or run fresh setup."
		}

		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("Nginx Service", func() string {
				if hasBinary {
					return core.YELLOW + "[UNREGISTERED / STOPPED]" + core.NC
				}
				return core.RED + "[NOT INSTALLED]" + core.NC
			}(), core.RED),
			tui.FormatKV("Nginx Binary Tool", system.GetToolBadge("nginx"), core.YELLOW),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + statusText + core.NC,
			"Choose an action below to deploy NGINX or run system network diagnostics.",
		}

		var options []string
		if hasBinary {
			options = append(options, "Adopt Existing NGINX & Enter Management Panel")
		}
		options = append(options,
			"Install NGINX Docker (Containerized Stack - Recommended)",
			"Install NGINX Native (Clean Installation + Firewall)",
			"Run Deep System & Network Diagnostics Scan",
			"Exit Diagnostics",
		)

		_, selectedOpt, status := tui.SelectMenuWithContent("nginx-manager - System Diagnostics", diagLines, options)
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
			})
			managementMenu()
			return
		case strings.HasPrefix(cleanOpt, "Install NGINX Docker"):
			install.FreshInstallWizard("docker")
			return
		case strings.HasPrefix(cleanOpt, "Install NGINX Native"):
			install.FreshInstallWizard("native")
			return
		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()
		}
	}
}
