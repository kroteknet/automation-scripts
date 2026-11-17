// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE TUI DASHBOARD & MENUS
// Service: Apache HTTP Server 2.4+ (httpd) (apache)
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

	"krotek-apache-installer/core"
	"krotek-apache-installer/install"
	"krotek-apache-installer/modules"
	"krotek-apache-installer/system"
	"krotek-apache-installer/tui"
)

func noApacheDiagnosticMenu() {
	for {
		tui.ClearScreen()
		diagLines := []string{
			core.BOLD + core.YELLOW + "SYSTEM DIAGNOSTICS & ENVIRONMENT REPORT" + core.NC,
			"",
			tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
			tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			tui.FormatKV("Apache Service", core.RED+"[NOT INSTALLED]"+core.NC, core.RED),
			tui.FormatKV("Apache Binary Tool", system.GetToolBadge("apache2"), core.YELLOW),
			tui.FormatKV("Docker Engine Tool", system.GetToolBadge("docker"), core.YELLOW),
			"",
			core.BOLD + core.CYAN + "Status: Apache web server is not provisioned on this machine yet." + core.NC,
			"Choose an action below to deploy Apache or run system network diagnostics.",
		}

		options := []string{
			"Install Apache Docker (Containerized Stack - Recommended)",
			"Install Apache Native (Clean Installation + Firewall)",
			"Run Deep System & Network Diagnostics Scan",
			"Exit Diagnostics",
		}

		idx, _, status := tui.SelectMenuWithContent("apache-manager - System Diagnostics (Apache Not Found)", diagLines, options)
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

// managementMenu orchestrates the main control dashboard, inspecting service health
// and displaying native or Docker management options based on active deployment.
func managementMenu() {
	modules.EnsureApacheWorking()
	health := modules.DetectApacheHealth()

	if !health.Installed {
		noApacheDiagnosticMenu()
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
		composeDir := state["COMPOSE_DIR"]
		if composeDir == "" {
			composeDir = "/opt/apache-docker"
		}
		dockerManagementMenu(composeDir, health)
	} else {
		nativeManagementMenu(health)
	}
}

// nativeManagementMenu provides direct host systemd/OpenRC daemon controls,
// configuration test shortcuts, and port listening inspectors.
func nativeManagementMenu(health modules.ApacheHealth) {
	srvName := "apache2"
	mainConf := "/etc/apache2/apache2.conf"
	if core.SysInfo.Family != "debian" {
		srvName = "httpd"
		mainConf = "/etc/httpd/conf/httpd.conf"
	}

	for {
		health = modules.DetectApacheHealth()

		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart APACHE Service"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start APACHE Service  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		configLabel := fmt.Sprintf("Edit main config  (%s)", mainConf)
		if !health.SyntaxOK {
			configLabel = core.BOLD + core.RED + "Edit main config  ->  [SYNTAX ERROR DETECTED]" + core.NC
		}

		options := []string{
			"─── VIRTUAL HOSTS & DOMAINS ───",
			"Check Service Status & IP Info",
			"View Application Dashboard & IP Info",
			"Deploy / Reset KrotekNet Landing Page (/var/www/html)",
			"Add virtual host / domain",
			"Generate systemd background service (Node.js/Python)",
			"List configured sites",
			"Edit virtual host / domain config",
			"Enable / Disable site",
			"─── SERVICE MANAGEMENT ───",
			configLabel,
		}
		if !health.SyntaxOK {
			options = append(options, core.BOLD+core.YELLOW+"Restore Clean Default Config  ->  [AUTO-FIX SYNTAX ERROR]"+core.NC)
		}
		options = append(options,
			"Reload APACHE Service",
			restartLabel,
			"Update Apache HTTP Server & Check Upgrades (Safe Backup)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── LOGS & DIAGNOSTICS ───",
			"View Service Logs & System Journal",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System & Network Diagnostics",
			"Uninstall APACHE Server",
			"Exit",
		)

		titleStr := fmt.Sprintf("apache-manager  -  Management (Native: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "View Application Dashboard") || strings.HasPrefix(cleanOpt, "Check Service Status & IP Info"):
			tui.ClearScreen()
			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "Apache HTTP Server", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Local IP", system.GetLocalIP(), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Public IP", system.GetPublicIP(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("HTTP Port", "80", core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("HTTPS Port", "443", core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("Main Config", mainConf, core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Default Web Root", "/var/www/html", core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Service Name", srvName, core.GREEN))
			tui.PrintInBox("APACHE Dashboard Info", dbLines)

		case strings.HasPrefix(cleanOpt, "Deploy / Reset KrotekNet"):
			tui.ClearScreen()
			if tui.AskYesNo("Deploy / Overwrite KrotekNet Landing Page in /var/www/html/index.html?") != 0 {
				continue
			}
			webUser := "www-data"
			if core.SysInfo.Family != "debian" {
				webUser = "apache"
			}
			webDir := "/var/www/html"
			if core.SysInfo.Family == "alpine" {
				webDir = "/var/www/localhost/htdocs"
			}
			_ = os.MkdirAll(webDir, 0755)
			_ = exec.Command("chmod", "755", "/var").Run()
			_ = exec.Command("chmod", "755", "/var/www").Run()
			system.SecureWebRootPermissions(webDir, webUser)

			defaultHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
				ServiceName: "Apache HTTP Server",
				LogoText:    "A",
				HeroTitle:   "Apache HTTP Server Active",
				HeroDesc:    "Your robust, production-ready Apache web server is fully operational and tuned for production.",
				EnvType:     "native",
				Port:        "80",
				WebDir:      webDir,
				ConfigPath:  mainConf,
				WebUser:     webUser,
				ManagerCmd:  "apache-manager",
			})

			idxPath := filepath.Join(webDir, "index.html")
			_ = system.WriteFileNoFollow(idxPath, []byte(defaultHTML), 0644, webUser)

			system.ReloadService(srvName)

			var successLines []string
			successLines = append(successLines, core.BOLD+core.GREEN+core.TagOK() + " KrotekNet Landing Page deployed successfully!"+core.NC)
			successLines = append(successLines, "")
			successLines = append(successLines, tui.FormatKV("Document Root", webDir, core.CYAN))
			successLines = append(successLines, tui.FormatKV("Landing File", idxPath, core.GREEN))
			successLines = append(successLines, tui.FormatKV("File Ownership", webUser+":"+webUser, core.YELLOW))
			successLines = append(successLines, tui.FormatKV("Access URL", fmt.Sprintf("http://%s/", system.GetLocalIP()), core.CYAN+core.BOLD))
			tui.PrintInBox("Landing Page Deployed", successLines)

		case strings.HasPrefix(cleanOpt, "Add virtual host"):
			modules.DoAddDomain()

		case strings.HasPrefix(cleanOpt, "Generate systemd"):
			modules.GenerateSystemdServiceWizard()

		case strings.HasPrefix(cleanOpt, "List configured sites"):
			tui.ClearScreen()
			var siteInfo []string
			if core.SysInfo.Family == "debian" {
				siteInfo = append(siteInfo, core.BOLD+core.YELLOW+"SITES AVAILABLE (/etc/apache2/sites-available/):"+core.NC)
				availCount := 0
				if files, err := filepath.Glob("/etc/apache2/sites-available/*.conf"); err == nil {
					for _, f := range files {
						bname := filepath.Base(f)
						availCount++
						enabledPath := filepath.Join("/etc/apache2/sites-enabled", bname)
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
				siteInfo = append(siteInfo, core.BOLD+core.YELLOW+"CONFIGS IN /etc/httpd/conf.d/:"+core.NC)
				confCount := 0
				if files, err := filepath.Glob("/etc/httpd/conf.d/*.conf"); err == nil {
					for _, f := range files {
						bname := filepath.Base(f)
						confCount++
						siteInfo = append(siteInfo, fmt.Sprintf("  %s%s%s %s%s%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, bname, core.NC))
					}
				}
				if confCount == 0 {
					siteInfo = append(siteInfo, "  (none)")
				}
			}
			tui.PrintInBox("Configured APACHE Sites", siteInfo)

		case strings.HasPrefix(cleanOpt, "Edit virtual host"):
			tui.ClearScreen()
			searchDir := "/etc/apache2/sites-available"
			if core.SysInfo.Family != "debian" {
				searchDir = "/etc/httpd/conf.d"
			}

			var siteFiles []string
			var siteLabels []string
			if files, err := filepath.Glob(filepath.Join(searchDir, "*.conf")); err == nil {
				for _, f := range files {
					bname := filepath.Base(f)
					siteFiles = append(siteFiles, f)
					statusTag := ""
					if core.SysInfo.Family == "debian" {
						enabledPath := filepath.Join("/etc/apache2/sites-enabled", bname)
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
				sIdx, _, sSt := tui.SelectMenu("apache-manager - Select Config File", siteLabels)
				if sSt == 0 && sIdx < len(siteFiles) {
					selectedFile := siteFiles[sIdx]
					_ = tui.RunEditor(selectedFile)

					var testCmd *exec.Cmd
					if core.SysInfo.Family == "debian" {
						testCmd = exec.Command("apache2ctl", "-t")
					} else {
						testCmd = exec.Command("httpd", "-t")
					}

					var resInfo []string
					if testCmd.Run() == nil {
						if exec.Command("systemctl", "reload", srvName).Run() != nil {
							_ = exec.Command("rc-service", srvName, "reload").Run()
						}
						resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
						resInfo = append(resInfo, tui.FormatKV("Edited File", selectedFile, core.CYAN))
						resInfo = append(resInfo, tui.FormatKV("Apache Service", "Syntax OK & Service Reloaded", core.GREEN))
					} else {
						resInfo = append(resInfo, tui.FormatKV("Operation Status", "FAILED", core.RED))
						resInfo = append(resInfo, tui.FormatKV("Edited File", selectedFile, core.CYAN))
						resInfo = append(resInfo, tui.FormatKV("Apache Service", "Config Syntax Test Failed!", core.RED))
					}
					tui.ClearScreen()
					tui.PrintInBox("Edit Config Result", resInfo)
				}
			}

		case strings.HasPrefix(cleanOpt, "Enable / Disable"):
			tui.ClearScreen()
			if core.SysInfo.Family == "debian" {
				searchDir := "/etc/apache2/sites-available"
				var siteNames []string
				var siteOptions []string
				if files, err := filepath.Glob(filepath.Join(searchDir, "*.conf")); err == nil {
					for _, f := range files {
						bname := filepath.Base(f)
						siteNames = append(siteNames, bname)
						enabledPath := filepath.Join("/etc/apache2/sites-enabled", bname)
						if _, err := os.Lstat(enabledPath); err == nil {
							siteOptions = append(siteOptions, fmt.Sprintf("%s  %s-> [ENABLED - Click to Disable]%s", bname, core.GREEN, core.NC))
						} else {
							siteOptions = append(siteOptions, fmt.Sprintf("%s  %s-> [DISABLED - Click to Enable]%s", bname, core.YELLOW, core.NC))
						}
					}
				}

				if len(siteNames) == 0 {
					tui.PrintInBox("No Config Files Found", "No site configuration files found in "+searchDir+".")
				} else {
					siteOptions = append(siteOptions, "---", "Cancel / Back to Menu")
					tIdx, _, tSt := tui.SelectMenu("apache-manager - Enable / Disable Site", siteOptions)
					if tSt == 0 && tIdx < len(siteNames) {
						targetSite := siteNames[tIdx]
						enabledPath := filepath.Join("/etc/apache2/sites-enabled", targetSite)
						actionText := ""
						if _, err := os.Lstat(enabledPath); err == nil {
							_ = os.Remove(enabledPath)
							actionText = "DISABLED"
						} else {
							_ = os.Symlink(filepath.Join(searchDir, targetSite), enabledPath)
							actionText = "ENABLED"
						}

						var resInfo []string
						testCmd := exec.Command("apache2ctl", "-t")
						if testCmd.Run() == nil {
							_ = exec.Command("systemctl", "reload", "apache2").Run()
							resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
							resInfo = append(resInfo, tui.FormatKV("Target Site", targetSite, core.CYAN))
							resInfo = append(resInfo, tui.FormatKV("New Site Status", actionText, core.GREEN))
						} else {
							resInfo = append(resInfo, tui.FormatKV("Operation Status", "FAILED", core.RED))
							resInfo = append(resInfo, tui.FormatKV("Target Site", targetSite, core.CYAN))
							resInfo = append(resInfo, tui.FormatKV("Apache Service", "Config Syntax Test Failed!", core.RED))
						}
						tui.ClearScreen()
						tui.PrintInBox("Site Toggle Result", resInfo)
					}
				}
			} else {
				tui.PrintInBox("Enable/Disable Notice", "Enable/disable site is managed via files in /etc/httpd/conf.d/ on RHEL.")
			}

		case strings.HasPrefix(cleanOpt, "Restore Clean Default Config") || strings.HasPrefix(cleanOpt, "Restore Default"):
			tui.ClearScreen()
			askLines := []string{
				core.BOLD + core.YELLOW + "Restore Clean Production Default Configuration" + core.NC,
				"",
				"This action will replace the main configuration file:",
				"  " + core.CYAN + mainConf + core.NC,
				"with a tested, clean default production template.",
				"",
				"A backup of the existing config will be saved before overwriting.",
			}
			idx, _, st := tui.SelectMenuWithContent("Confirm Reset Configuration", askLines, []string{"YES, RESTORE DEFAULT CONFIG", "NO, CANCEL"})
			if st == 0 && idx == 0 {
				ts := time.Now().Format("2006-01-02_15-04-05")
				bakPath := fmt.Sprintf("%s.bak_%s", mainConf, ts)
				if rawConf, rErr := os.ReadFile(mainConf); rErr == nil {
					_ = system.WriteFileNoFollow(bakPath, rawConf, 0644, "")
				}
				_ = system.WriteFileNoFollow(mainConf, []byte(modules.GetDefaultMainConfig()), 0644, "")
				if exec.Command("systemctl", "restart", srvName).Run() != nil {
					_ = exec.Command("rc-service", srvName, "restart").Run()
				}

				var resInfo []string
				resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resInfo = append(resInfo, tui.FormatKV("Config File", mainConf, core.CYAN))
				resInfo = append(resInfo, tui.FormatKV("Backup Saved", bakPath, core.YELLOW))
				resInfo = append(resInfo, tui.FormatKV("Apache Service", "Config Restored & Restart Triggered", core.GREEN))
				tui.ClearScreen()
				tui.PrintInBox("Config Reset Result", resInfo)
			}

		case strings.HasPrefix(cleanOpt, "Edit main config"):
			tui.ClearScreen()
			_ = tui.RunEditor(mainConf)

			var testCmd *exec.Cmd
			if core.SysInfo.Family == "debian" {
				testCmd = exec.Command("apache2ctl", "-t")
			} else {
				testCmd = exec.Command("httpd", "-t")
			}

			var resInfo []string
			if testCmd.Run() == nil {
				if exec.Command("systemctl", "reload", srvName).Run() != nil {
					_ = exec.Command("rc-service", srvName, "reload").Run()
				}
				resInfo = append(resInfo, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resInfo = append(resInfo, tui.FormatKV("Config File", mainConf, core.CYAN))
				resInfo = append(resInfo, tui.FormatKV("Apache Service", "Syntax OK & Service Reloaded", core.GREEN))
			} else {
				resInfo = append(resInfo, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resInfo = append(resInfo, tui.FormatKV("Config File", mainConf, core.CYAN))
				resInfo = append(resInfo, tui.FormatKV("Apache Service", "Main Config Syntax Test Failed!", core.RED))
			}
			tui.ClearScreen()
			tui.PrintInBox("Main Config Result", resInfo)

		case strings.HasPrefix(cleanOpt, "Reload APACHE"):
			tui.ClearScreen()
			modules.EnsureApacheWorking()
			var resMsg []string
			var testCmd *exec.Cmd
			if core.SysInfo.Family == "debian" {
				testCmd = exec.Command("apache2ctl", "-t")
			} else {
				testCmd = exec.Command("httpd", "-t")
			}

			if testCmd.Run() == nil && (exec.Command("systemctl", "reload", srvName).Run() == nil || exec.Command("rc-service", srvName, "reload").Run() == nil) {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("Apache Service", "Reloaded successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resMsg = append(resMsg, tui.FormatKV("Apache Service", "Reload failed! Check syntax.", core.RED))
			}
			tui.PrintInBox("Reload APACHE Status", resMsg)

		case strings.HasPrefix(cleanOpt, "Restart APACHE") || strings.HasPrefix(cleanOpt, "Start APACHE"):
			tui.ClearScreen()
			modules.EnsureApacheWorking()
			var resMsg []string
			if exec.Command("systemctl", "restart", srvName).Run() == nil || exec.Command("systemctl", "start", srvName).Run() == nil || exec.Command("rc-service", srvName, "restart").Run() == nil || exec.Command("rc-service", srvName, "start").Run() == nil {
				health.IsRunning = true
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("Apache Service", "Started/Restarted successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
				resMsg = append(resMsg, tui.FormatKV("Apache Service", "Failed to start/restart!", core.RED))

				var testOut []byte
				if core.SysInfo.Family == "debian" {
					testOut, _ = exec.Command("apache2ctl", "-t").CombinedOutput()
				} else {
					testOut, _ = exec.Command("httpd", "-t").CombinedOutput()
				}
				if len(testOut) > 0 {
					resMsg = append(resMsg, "")
					resMsg = append(resMsg, core.BOLD+core.RED+"SYNTAX TEST OUTPUT:"+core.NC)
					for _, line := range strings.Split(strings.TrimSpace(string(testOut)), "\n") {
						if strings.TrimSpace(line) != "" {
							resMsg = append(resMsg, "  "+core.RED+line+core.NC)
						}
					}
				}
			}
			tui.PrintInBox("Restart APACHE Status", resMsg)

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()
		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("native")
			health = modules.DetectApacheHealth()
		case strings.HasPrefix(cleanOpt, "View Service Logs") || strings.HasPrefix(cleanOpt, "View Service"):
			modules.ShowLogsMenu()

		case strings.HasPrefix(cleanOpt, "Update ") || strings.Contains(cleanOpt, "Update & Check Upgrades"):
			system.RunServiceUpdateFlow()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallApache()
			return
		}
	}
}

// dockerManagementMenu provides container lifecycle controls (start/stop/restart/logs)
// for the containerized Apache HTTP Server 2.4+ (httpd) Docker Compose stack.
func dockerManagementMenu(composeDir string, health modules.ApacheHealth) {
	for {
		statusBadge := core.GREEN + "[ACTIVE]" + core.NC
		if !health.IsRunning {
			statusBadge = core.RED + "[STOPPED]" + core.NC
		}

		restartLabel := "Restart APACHE Container"
		if !health.IsRunning {
			restartLabel = core.BOLD + core.RED + "Start APACHE Container  ->  [CURRENTLY STOPPED - Click to Start]" + core.NC
		}

		options := []string{
			"─── DOCKER STACK MANAGEMENT ───",
			"Check Container Status (docker ps)",
			"View Application Dashboard & IP Info",
			fmt.Sprintf("Edit docker-compose.yml  (%s/docker-compose.yml)", composeDir),
			restartLabel,
			"View Container Logs (last 50 lines)",
			"Run Guided Setup / Reinstallation Wizard",
			"─── DIAGNOSTICS & MAINTENANCE ───",
			"Run Deep System & Network Diagnostics",
			"Uninstall APACHE Docker Stack",
			"Exit",
		}

		titleStr := fmt.Sprintf("apache-manager  -  Management (Docker: %s)", statusBadge)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "Check Container Status"):
			tui.ClearScreen()
			out, _ := exec.Command("docker", "ps", "--filter", "name=apache").CombinedOutput()
			tui.PrintInBox("Docker Container Status", tui.FormatDockerContainerStatus(string(out), "No running Docker containers found for apache."))

		case strings.HasPrefix(cleanOpt, "View Application Dashboard"):
			tui.ClearScreen()
			var dbLines []string
			dbLines = append(dbLines, tui.FormatKV("Application", "Apache HTTP Server (Docker)", core.GREEN+core.BOLD))
			dbLines = append(dbLines, tui.FormatKV("Local IP", system.GetLocalIP(), core.CYAN))
			dbLines = append(dbLines, tui.FormatKV("Public IP", system.GetPublicIP(), core.GREEN))
			dbLines = append(dbLines, tui.FormatKV("HTTP Port", "80 (Host Port)", core.YELLOW))
			dbLines = append(dbLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
			tui.PrintInBox("APACHE Dashboard Info", dbLines)

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
				}
			} else {
				resInfo = append(resInfo, tui.FormatKV("Operation Status", "SAVED", core.GREEN))
				resInfo = append(resInfo, tui.FormatKV("Compose File", composeFile, core.CYAN))
			}
			tui.ClearScreen()
			tui.PrintInBox("Docker Compose Edit Result", resInfo)

		case strings.HasPrefix(cleanOpt, "Restart APACHE") || strings.HasPrefix(cleanOpt, "Start APACHE"):
			tui.ClearScreen()
			var resMsg []string
			cmdStr := fmt.Sprintf("cd %s && docker compose restart 2>/dev/null || docker-compose restart 2>/dev/null || docker restart krotek-apache", composeDir)
			if system.RunMultiStep("Restarting Docker stack", cmdStr) {
				health.IsRunning = true
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
				resMsg = append(resMsg, tui.FormatKV("Docker Stack", "Restarted successfully", core.GREEN))
			} else {
				resMsg = append(resMsg, tui.FormatKV("Operation Status", "FAILED", core.RED))
			}
			tui.PrintInBox("Restart Docker Stack Result", resMsg)

		case strings.HasPrefix(cleanOpt, "View Container Logs"):
			tui.ClearScreen()
			var logLines []string
			cmdStr := fmt.Sprintf("docker logs --tail 50 krotek-apache 2>/dev/null || (cd %s && docker compose logs --tail 50 2>/dev/null)", composeDir)
			if out, err := exec.Command(system.GetShell(), "-c", cmdStr).CombinedOutput(); err == nil && len(out) > 0 {
				raw := strings.Split(string(out), "\n")
				code2xx := regexp.MustCompile(`" 2[0-9]{2} `)
				codeErr := regexp.MustCompile(`" [4-5][0-9]{2} `)
				for _, line := range raw {
					if strings.TrimSpace(line) != "" {
						line = code2xx.ReplaceAllString(line, fmt.Sprintf("\" %s$0%s ", core.GREEN, core.NC))
						line = codeErr.ReplaceAllString(line, fmt.Sprintf("\" %s$0%s ", core.RED, core.NC))
						logLines = append(logLines, line)
					}
				}
			}
			if len(logLines) == 0 {
				logLines = append(logLines, "No container logs found.")
			}
			if len(logLines) > 15 {
				logLines = logLines[len(logLines)-15:]
			}
			tui.PrintInBox("Docker Container Logs (Last 50 lines)", logLines)

		case strings.HasPrefix(cleanOpt, "Run Deep System"):
			modules.RunDeepDiagnostics()

		case strings.Contains(cleanOpt, "Setup / Reinstallation Wizard"):
			install.FreshInstallWizard("docker")
			health = modules.DetectApacheHealth()

		case strings.HasPrefix(cleanOpt, "Uninstall"):
			install.UninstallApache()
			return
		}
	}
}

// ==============================================================================
// UNINSTALLATION WIZARD
// ==============================================================================
