// ==============================================================================
// KROTEK SCRIPT HUB — VIRTUALHOST & SERVER BLOCK MANAGER
// Service: NGINX (web-server)
// File:    web-server/nginx/src/modules/vhosts.go
//
// Manages website domain configurations, reverse proxy upstream blocks, directory permissions, and Let's Encrypt SSL/TLS certificate directives.
// ==============================================================================

package modules

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"krotek-nginx-installer/core"
	"krotek-nginx-installer/logger"
	"krotek-nginx-installer/system"
	"krotek-nginx-installer/tui"
)

// CheckConflicts verifies whether conflicting web servers (e.g. Apache) or occupied ports are active on the host.
func CheckConflicts() {
	apacheSvc := "apache2"
	if core.SysInfo.Family == "rhel" {
		apacheSvc = "httpd"
	}

	if system.CommandExists(apacheSvc) || (system.IsSystemdRunning() && exec.Command("systemctl", "is-active", "--quiet", apacheSvc).Run() == nil) || exec.Command("service", apacheSvc, "status").Run() == nil || exec.Command("pidof", apacheSvc).Run() == nil {
		res := tui.AskYesNo(fmt.Sprintf("Apache detected (%s)! Stop and disable Apache to free ports for NGINX?", apacheSvc))
		tui.ClearScreen()
		if res == 0 {
			system.RunMultiStep("Stopping Apache", fmt.Sprintf("systemctl stop %s 2>/dev/null || service %s stop 2>/dev/null || /etc/init.d/%s stop 2>/dev/null || true", apacheSvc, apacheSvc, apacheSvc))
			system.RunMultiStep("Disabling Apache", fmt.Sprintf("systemctl disable %s 2>/dev/null || update-rc.d %s disable 2>/dev/null || true", apacheSvc, apacheSvc))
		} else {
			logger.LogWarn("Left Apache running. This may cause a port 80/443 conflict.")
		}
	}

	var netOut []byte
	if system.CommandExists("ss") {
		netOut, _ = exec.Command("ss", "-tuln").Output()
	} else if system.CommandExists("netstat") {
		netOut, _ = exec.Command("netstat", "-tuln").Output()
	}
	if len(netOut) > 0 {
		outStr := string(netOut)
		if strings.Contains(outStr, ":80 ") || strings.Contains(outStr, ":80\t") || strings.Contains(outStr, ":::80 ") ||
			strings.Contains(outStr, ":443 ") || strings.Contains(outStr, ":443\t") || strings.Contains(outStr, ":::443 ") {
			res := tui.AskYesNo("Port 80 or 443 is currently in use! Continue anyway?")
			tui.ClearScreen()
			if res != 0 {
				logger.LogInfo("Installation cancelled by user.")
				tui.LeaveAltScreen()
				os.Exit(0)
			}
		}
	}
}

// ==============================================================================
// DOMAIN & VIRTUAL HOST CREATOR
// ==============================================================================
func DoAddDomain() {
	tui.ClearScreen()
	cardLines := []string{
		core.BOLD + core.CYAN + "Add New Virtual Host / Domain Configuration" + core.NC,
		"",
		"  Specify the primary domain name you want to configure for NGINX.",
		"  " + core.BOLD + "Examples:" + core.NC + " " + core.CYAN + "next-app.com" + core.NC + ", " + core.CYAN + "api.service.org" + core.NC + ", " + core.CYAN + "sub.domain.io" + core.NC,
	}
	tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel & Return to Menu", "Add Virtual Host / Domain", cardLines)

	rawDomain, st := tui.ReadTextInput("Enter domain name", "")
	if st != 0 || rawDomain == "" {
		logger.LogInfo("Cancelled by user.")
		return
	}
	domainName := system.CleanDomain(rawDomain)
	if !system.ValidateDomainName(domainName) {
		tui.PrintInBox("Validation Error", []string{
			core.RED + "Invalid domain name syntax. Must be a valid FQDN (e.g. example.com)." + core.NC,
			"",
			"Entered: " + rawDomain,
		})
		return
	}

	preset, targetOrPath, okP := PromptVHostPreset(domainName)
	if !okP {
		logger.LogInfo("Domain addition cancelled by user.")
		return
	}

	sslStrat, okS := system.PromptSSLStrategy(domainName)
	if !okS {
		logger.LogInfo("Domain addition cancelled by user.")
		return
	}

	vhostFile := fmt.Sprintf("/etc/nginx/conf.d/%s.conf", domainName)
	if core.SysInfo.Family == "debian" {
		vhostFile = fmt.Sprintf("/etc/nginx/sites-available/%s", domainName)
	} else if core.SysInfo.Family == "alpine" {
		vhostFile = fmt.Sprintf("/etc/nginx/http.d/%s.conf", domainName)
	}

	if !system.RunStepOrAbort("Generating NGINX configuration", "true") {
		return
	}

	// Prepare document root or SELinux policies
	if preset.Type == PresetNodeJS || preset.Type == PresetPython {
		if system.CommandExists("getenforce") {
			if out, err := exec.Command("getenforce").Output(); err == nil && strings.TrimSpace(string(out)) == "Enforcing" {
				_ = system.RunStepOrAbort("Configuring SELinux for network connections", "setsebool", "-P", "httpd_can_network_connect", "1")
			}
		}
	} else {
		if err := system.ValidateSafeDirectoryPath(targetOrPath); err != nil {
			logger.LogError("Aborting: " + err.Error())
			return
		}
		if system.IsSymlinkPath(targetOrPath) {
			logger.LogError("Aborting: document root must not be a symbolic link: " + targetOrPath)
			return
		}
		_ = os.MkdirAll(targetOrPath, 0755)
		webUser := "nginx"
		if core.SysInfo.Family == "debian" {
			webUser = "www-data"
		}
		system.SecureWebRootPermissions(targetOrPath, webUser)

		indexPath := filepath.Join(targetOrPath, "index.html")
		if _, err := os.Lstat(indexPath); os.IsNotExist(err) {
			_ = system.WriteFileNoFollow(indexPath, []byte("<h1>Welcome to "+domainName+"</h1>"), 0644, webUser)
		}

		if preset.Type == PresetPHP {
			testPHP := fmt.Sprintf(`<?php
header('Content-Type: text/html; charset=utf-8');
echo "<h1>Welcome to %s</h1><p>PHP " . phpversion() . " is active and running under NGINX.</p>";
`, domainName)
			phpIndexPath := filepath.Join(targetOrPath, "index.php")
			if _, err := os.Lstat(phpIndexPath); os.IsNotExist(err) {
				_ = system.WriteFileNoFollow(phpIndexPath, []byte(testPHP), 0644, webUser)
			}
		}
	}

	// Ensure port 80 and 443 are universally open across firewalls
	system.ConfigureFirewallRules("80", "443")

	vhostTpl := GenerateTemplateVHost(preset, domainName, targetOrPath, sslStrat)
	if vhostTpl == "" {
		logger.LogError("Aborting: generated NGINX virtual host configuration is invalid.")
		return
	}

	_ = os.MkdirAll(filepath.Dir(vhostFile), 0755)
	if err := system.WriteFileNoFollow(vhostFile, []byte(vhostTpl), 0644, ""); err != nil {
		logger.LogError("Failed to write virtual host config: " + err.Error())
		return
	}
	if system.CommandExists("restorecon") {
		system.RunStep("Fixing SELinux context", "restorecon", "-v", vhostFile)
	}

	if core.SysInfo.Family == "debian" {
		_ = os.MkdirAll("/etc/nginx/sites-enabled", 0755)
		enabledPath := filepath.Join("/etc/nginx/sites-enabled", filepath.Base(vhostFile))
		_ = os.Remove(enabledPath)
		_ = os.Symlink(vhostFile, enabledPath)
	}

	if system.CommandExists("nginx") {
		if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
			_ = os.Remove(vhostFile)
			if core.SysInfo.Family == "debian" {
				_ = os.Remove(fmt.Sprintf("/etc/nginx/sites-enabled/%s", domainName))
			}
			tui.PrintInBox("NGINX Configuration Error", []string{
				core.RED + "Configuration syntax check failed! Vhost was removed to prevent server crash:" + core.NC,
				"",
				core.SanitizeTerminalText(string(out)),
			})
			return
		}
	}
	system.ReloadService("nginx")

	if preset.Type == PresetPHP {
		system.InstallPHPStack()
		system.StartPHPFPMService()
		system.OptimizePHPSettings()
	}

	sslFailed := false
	var sslErrLines []string

	if sslStrat.Mode == "certbot" {
		if system.IsLocalOrPrivateDomain(domainName) {
			logger.LogWarn(fmt.Sprintf("Skipping Let's Encrypt for local/private domain '%s'; configuring plain HTTP.", domainName))
			sslStrat.Mode = "none"
		} else {
			certbotReady := system.CommandExists("certbot")
			if !certbotReady {
				if core.SysInfo.Family == "debian" {
					certbotReady = system.RunStep("Installing Certbot", "apt-get", "install", "-y", "certbot", "python3-certbot-nginx")
				} else if core.SysInfo.Family == "rhel" {
					if system.RunStep("Installing EPEL repository", "dnf", "install", "-y", "epel-release") {
						certbotReady = system.RunStep("Installing Certbot (EPEL)", "dnf", "install", "-y", "certbot", "python3-certbot-nginx")
					}
				} else if core.SysInfo.Family == "alpine" {
					certbotReady = system.RunStep("Installing Certbot", "apk", "add", "--no-cache", "certbot", "certbot-nginx")
				}
			}

			if !certbotReady {
				sslFailed = true
				sslErrLines = system.ExtractRecentLogErrors(core.LogFilePath, 4)
				sslStrat.Mode = "none"
			} else {
				logger.LogWarn("Ensuring domain points to this server's IP address (A record) before generation!")
				adminEmail := sslStrat.Email
				if adminEmail == "" || strings.HasPrefix(adminEmail, "-") {
					adminEmail = "admin@" + domainName
				}
				certArgs := []string{"--nginx", "-d", domainName}
				if addrs, err := net.LookupHost("www." + domainName); err == nil && len(addrs) > 0 {
					certArgs = append(certArgs, "-d", "www."+domainName)
				}
				certArgs = append(certArgs, "--non-interactive", "--agree-tos", "-m", adminEmail, "--redirect")

				if !system.RunStep("Generating SSL Certificate (Let's Encrypt)", "certbot", certArgs...) {
					sslFailed = true
					sslErrLines = system.ExtractRecentLogErrors(core.LogFilePath, 4)
					logger.LogWarn(fmt.Sprintf("Let's Encrypt SSL acquisition failed for '%s'. Virtual host remains active on HTTP port 80.", domainName))
					sslStrat.Mode = "none"
				} else {
					if rawConf, rErr := os.ReadFile(vhostFile); rErr == nil {
						updatedConf := strings.ReplaceAll(string(rawConf), "listen 443 ssl;", "listen 443 ssl http2;")
						_ = system.WriteFileNoFollow(vhostFile, []byte(updatedConf), 0644, "")
					}
					system.ReloadService("nginx")
				}
			}
		}
	} else if sslStrat.Mode == "cloudflare" {
		logger.LogSuccess("Cloudflare Real-IP rules active. Local SSL termination delegated to Cloudflare Edge.")
	} else if sslStrat.Mode == "custom" {
		logger.LogSuccess(fmt.Sprintf("Custom SSL certificate installed from %s", sslStrat.CertPath))
	}

	statusVal := "SUCCESS"
	statusColor := core.GREEN
	sslDisplay := strings.ToUpper(sslStrat.Mode)
	sslColor := core.GREEN
	if sslStrat.Mode == "none" {
		sslColor = core.DIM
	}
	if sslFailed {
		statusVal = "PARTIAL (HTTP Active, SSL Failed)"
		statusColor = core.YELLOW
		sslDisplay = "NONE (Certbot Failed)"
		sslColor = core.RED
	}

	addRes := []string{
		tui.FormatKV("Operation Status", statusVal, statusColor),
		tui.FormatKV("Target Domain", domainName, core.CYAN),
		tui.FormatKV("Template Preset", preset.Title, core.CYAN),
		tui.FormatKV("Configuration File", vhostFile, core.YELLOW),
		tui.FormatKV("SSL/TLS Mode", sslDisplay, sslColor),
	}
	if preset.Type == PresetNodeJS || preset.Type == PresetPython {
		addRes = append(addRes, tui.FormatKV("Backend Proxy Target", targetOrPath, core.CYAN))
	} else {
		addRes = append(addRes, tui.FormatKV("Web Root Directory", targetOrPath, core.CYAN))
	}
	if sslFailed {
		addRes = append(addRes, "")
		addRes = append(addRes, core.BOLD+core.YELLOW+"SSL CERTIFICATE NOTICE:"+core.NC)
		addRes = append(addRes, "  "+core.YELLOW+"Let's Encrypt could not issue a certificate (check DNS A record & port 80)."+core.NC)
		for _, el := range sslErrLines {
			addRes = append(addRes, "  "+core.RED+core.Sym.Pointer+" "+el+core.NC)
		}
		if core.LogFilePath != "" {
			addRes = append(addRes, tui.FormatKV("Log File", "cat "+core.LogFilePath, core.CYAN))
		}
		addRes = append(addRes, tui.FormatKV("Retry SSL", "sudo certbot --nginx -d "+domainName, core.GREEN))
	}
	tui.ClearScreen()
	tui.PrintInBox("Add Virtual Host / Domain Result", addRes)

	if preset.Type == PresetNodeJS || preset.Type == PresetPython {
		fmt.Print("\r\n")
		if tui.AskYesNo("Generate systemd background service to keep your backend app running 24/7?") == 0 {
			GenerateSystemdServiceWizard()
		}
	}
}

// ==============================================================================
// NGINX MANAGEMENT DASHBOARD
// ==============================================================================
// ==============================================================================
// NGINX HEALTH DETECTOR & DEEP DIAGNOSTIC ENGINE
// ==============================================================================

func DoAddDomainDocker(composeDir string) {
	tui.ClearScreen()
	cardLines := []string{
		core.BOLD + core.CYAN + "Add New Virtual Host / Domain (Docker)" + core.NC,
		"",
		"  Specify the primary domain name you want to configure for Docker NGINX.",
		"  " + core.BOLD + "Examples:" + core.NC + " " + core.CYAN + "next-app.com" + core.NC + ", " + core.CYAN + "api.service.org" + core.NC + ", " + core.CYAN + "sub.domain.io" + core.NC,
	}
	tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel & Return to Menu", "Add Virtual Host / Domain", cardLines)

	rawDomain, st := tui.ReadTextInput("Enter domain name", "")
	if st != 0 || rawDomain == "" {
		logger.LogInfo("Cancelled by user.")
		return
	}
	domainName := system.CleanDomain(rawDomain)
	if !system.ValidateDomainName(domainName) {
		tui.PrintInBox("Validation Error", []string{
			core.RED + "Invalid domain name syntax. Must be a valid FQDN (e.g. example.com)." + core.NC,
			"",
			"Entered: " + rawDomain,
		})
		return
	}

	wantProxy := false
	proxyTarget := ""
	tui.ClearScreen()
	res := tui.AskYesNo("Configure this domain as a Reverse Proxy?")
	if res == 2 || res == -1 {
		return
	}
	if res == 0 {
		wantProxy = true
		tui.ClearScreen()
		proxyCard := []string{
			core.BOLD + core.CYAN + "Reverse Proxy Target Configuration" + core.NC,
			"",
			"  Enter the target backend service URL and port to forward traffic to.",
			"  " + core.BOLD + "Examples:" + core.NC + " " + core.CYAN + "http://host.docker.internal:3000" + core.NC + ", " + core.CYAN + "http://172.17.0.1:8080" + core.NC,
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel & Return to Menu", "Reverse Proxy Target", proxyCard)
		pt, st := tui.ReadTextInput("Enter proxy target URL", "http://host.docker.internal:3000")
		if st != 0 || pt == "" {
			logger.LogInfo("Cancelled by user.")
			return
		}
		if !validateProxyTarget(pt) {
			tui.PrintInBox("Validation Error", []string{core.RED + "Invalid proxy target URL. Must start with http:// or https:// and cannot contain newlines or semicolons." + core.NC})
			return
		}
		proxyTarget = pt
	}

	if err := system.ValidateSafeComposeDir(composeDir); err != nil {
		logger.LogError("Invalid Docker Compose directory: " + err.Error())
		return
	}
	confDir := filepath.Join(composeDir, "conf.d")
	_ = os.MkdirAll(confDir, 0755)
	vhostFile := filepath.Join(confDir, domainName+".conf")

	vhostTpl := fmt.Sprintf(`server {
    listen 80;
    server_name %s www.%s;

    # Gzip Compression
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;

`, domainName, domainName)

	if wantProxy {
		vhostTpl += fmt.Sprintf(`    location / {
        proxy_pass %s;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`, proxyTarget)
	} else {
		siteRoot := "/usr/share/nginx/html/" + domainName
		hostDomainHtml := filepath.Join(composeDir, "html", domainName)
		_ = os.MkdirAll(hostDomainHtml, 0755)
		indexPath := filepath.Join(hostDomainHtml, "index.html")
		if _, err := os.Lstat(indexPath); os.IsNotExist(err) {
			_ = system.WriteFileNoFollow(indexPath, []byte("<h1>Welcome to "+domainName+"</h1>"), 0644, "")
		}
		vhostTpl += fmt.Sprintf(`    root %s;
    index index.html index.htm;

    location / {
        try_files $uri $uri/ =404;
    }
}
`, siteRoot)
	}

	_ = system.WriteFileNoFollow(vhostFile, []byte(vhostTpl), 0644, "")

	tui.ClearScreen()
	var addRes []string
	if exec.Command("docker", "exec", "krotek-nginx", "nginx", "-t").Run() == nil {
		_ = exec.Command("docker", "exec", "krotek-nginx", "nginx", "-s", "reload").Run()
		addRes = append(addRes, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
		addRes = append(addRes, tui.FormatKV("Target Domain", domainName, core.CYAN))
		addRes = append(addRes, tui.FormatKV("Configuration File", vhostFile, core.YELLOW))
		if !wantProxy {
			addRes = append(addRes, tui.FormatKV("Web Root Directory", filepath.Join(composeDir, "html", domainName), core.CYAN))
		}
		addRes = append(addRes, tui.FormatKV("Nginx Container", "Syntax OK & Service Reloaded", core.GREEN))
	} else {
		addRes = append(addRes, tui.FormatKV("Operation Status", "WARNING", core.YELLOW))
		addRes = append(addRes, tui.FormatKV("Target Domain", domainName, core.CYAN))
		addRes = append(addRes, tui.FormatKV("Configuration File", vhostFile, core.YELLOW))
		if !wantProxy {
			addRes = append(addRes, tui.FormatKV("Web Root Directory", filepath.Join(composeDir, "html", domainName), core.CYAN))
		}
		addRes = append(addRes, tui.FormatKV("Nginx Container", "Syntax test failed or container offline", core.RED))
	}
	tui.PrintInBox("Add Virtual Host Result (Docker)", addRes)
}

func validateDomainName(domain string) bool {
	return system.ValidateDomainName(domain)
}

func validateProxyTarget(target string) bool {
	return system.ValidateProxyTarget(target)
}
