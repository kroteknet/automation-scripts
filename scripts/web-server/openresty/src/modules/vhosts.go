// ==============================================================================
// KROTEK SCRIPT HUB — VIRTUALHOST & SERVER BLOCK MANAGER
// Service: OpenResty (web-server)
// File:    web-server/openresty/src/modules/vhosts.go
//
// Manages website domain configurations, reverse proxy upstream blocks, directory
// permissions, and Let's Encrypt SSL/TLS certificate directives across distributions.
// ==============================================================================

package modules

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"krotek-openresty-installer/core"
	"krotek-openresty-installer/logger"
	"krotek-openresty-installer/system"
	"krotek-openresty-installer/tui"
)

func isApacheConflictActiveOrEnabled(apacheSvc string) bool {
	if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
		if exec.Command("rc-service", apacheSvc, "status").Run() == nil {
			return true
		}
		if out, err := exec.Command("rc-update", "show", "default").Output(); err == nil {
			for _, field := range strings.Fields(string(out)) {
				if field == apacheSvc {
					return true
				}
			}
		}
	}
	if system.IsSystemdRunning() {
		if exec.Command("systemctl", "is-active", "--quiet", apacheSvc).Run() == nil ||
			exec.Command("systemctl", "is-enabled", "--quiet", apacheSvc).Run() == nil {
			return true
		}
		// Fallback to sysvinit unit check when init script exists without systemd unit
		if exec.Command("service", apacheSvc, "status").Run() == nil {
			return true
		}
	} else {
		if exec.Command("service", apacheSvc, "status").Run() == nil {
			return true
		}
	}
	if exec.Command("pidof", apacheSvc).Run() == nil {
		return true
	}
	return false
}

// CheckConflicts verifies whether conflicting web servers (e.g. Apache) or occupied target ports are active on the host.
func CheckConflicts() {
	apacheSvc := "apache2"
	if core.SysInfo.Family == "rhel" {
		apacheSvc = "httpd"
	}

	if isApacheConflictActiveOrEnabled(apacheSvc) {
		res := tui.AskYesNo(fmt.Sprintf("Apache detected (%s)! Stop and disable Apache to free ports for OpenResty?", apacheSvc))
		tui.ClearScreen()
		if res == 0 {
			tui.StartSpinner("Stopping Apache")
			_ = exec.Command("systemctl", "stop", apacheSvc).Run()
			_ = exec.Command("rc-service", apacheSvc, "stop").Run()
			_ = exec.Command("service", apacheSvc, "stop").Run()
			_ = exec.Command("/etc/init.d/"+apacheSvc, "stop").Run()
			tui.StopSpinner(0)

			tui.StartSpinner("Disabling Apache")
			_ = exec.Command("systemctl", "disable", apacheSvc).Run()
			_ = exec.Command("rc-update", "del", apacheSvc, "default").Run()
			_ = exec.Command("update-rc.d", apacheSvc, "disable").Run()
			tui.StopSpinner(0)
		} else {
			logger.LogWarn("Left Apache running. This may cause a port conflict if listening on the same port.")
		}
	}

	targetPort := strings.TrimSpace(os.Getenv("KROTEK_PORT"))
	if pNum, err := strconv.Atoi(targetPort); err != nil || pNum < 1 || pNum > 65535 {
		targetPort = "80"
	} else {
		targetPort = strconv.Itoa(pNum)
	}

	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
		return
	}

	var netOut []byte
	var netErr error
	if system.CommandExists("ss") {
		netOut, netErr = exec.Command("ss", "-tuln").Output()
	} else if system.CommandExists("netstat") {
		netOut, netErr = exec.Command("netstat", "-tuln").Output()
	}
	if netErr == nil && len(netOut) > 0 {
		portInUse := false
		for _, field := range strings.Fields(string(netOut)) {
			if strings.HasSuffix(field, ":"+targetPort) || (targetPort == "80" && strings.HasSuffix(field, ":443")) {
				portInUse = true
				break
			}
		}
		if portInUse {
			res := tui.AskYesNo(fmt.Sprintf("Port %s is currently in use! Continue anyway?", targetPort))
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
		"  Specify the primary domain name you want to configure for OpenResty.",
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

	vhostFile, enabledLink := system.GetOpenRestyDomainVHostPaths(domainName)
	_ = os.MkdirAll(filepath.Dir(vhostFile), 0755)
	if enabledLink != "" {
		_ = os.MkdirAll(filepath.Dir(enabledLink), 0755)
	}

	if !system.RunStepOrAbort("Generating OpenResty VirtualHost configuration", "true") {
		return
	}

	webUser := system.GetRuntimeWebUser()

	// Prepare document root or SELinux policies
	if preset.Type == PresetNodeJS || preset.Type == PresetPython {
		if system.CommandExists("getenforce") {
			if out, err := exec.Command("getenforce").Output(); err == nil && strings.TrimSpace(string(out)) == "Enforcing" {
				_ = system.RunStepOrAbort("Configuring SELinux for network connections", "setsebool", "-P", "httpd_can_network_connect", "1")
			}
		}
	} else {
		if err := system.ValidateSafeDirectoryPath(targetOrPath); err != nil || system.IsSymlinkPath(targetOrPath) {
			if err != nil {
				logger.LogError("Aborting: " + err.Error())
			} else {
				logger.LogError("Aborting: web root directory must not be a symbolic link.")
			}
			return
		}
		targetOrPath = filepath.Clean(strings.TrimSpace(targetOrPath))
		_ = os.MkdirAll(targetOrPath, 0755)
		system.SecureWebRootPermissions(targetOrPath, webUser)

		indexPath := filepath.Join(targetOrPath, "index.html")
		if _, err := os.Lstat(indexPath); os.IsNotExist(err) {
			_ = system.WriteFileNoFollow(indexPath, []byte("<h1>Welcome to "+domainName+"</h1>"), 0644, webUser)
		}

		if preset.Type == PresetPHP {
			system.InstallPHPStack()
			system.OptimizePHPSettings()
			system.StartPHPFPMService()

			testPHP := fmt.Sprintf(`<?php
header('Content-Type: text/html; charset=utf-8');
echo "<h1>Welcome to %s</h1><p>PHP " . phpversion() . " is active and running under OpenResty.</p>";
`, domainName)
			phpIndexPath := filepath.Join(targetOrPath, "index.php")
			if _, err := os.Lstat(phpIndexPath); os.IsNotExist(err) {
				_ = system.WriteFileNoFollow(phpIndexPath, []byte(testPHP), 0644, webUser)
			}
		}
	}

	// Ensure active HTTP port and HTTPS 443 are open across firewalls
	httpPort := system.GetConfiguredPort()
	system.ConfigureFirewallRules(httpPort, "443")

	vhostTpl := GenerateTemplateVHost(preset, domainName, targetOrPath, sslStrat)

	_ = system.WriteFileNoFollow(vhostFile, []byte(vhostTpl), 0644, "")
	if system.CommandExists("restorecon") {
		system.RunStep("Fixing SELinux context", "restorecon", "-v", vhostFile)
	}

	if enabledLink != "" {
		_ = os.Remove(enabledLink)
		if err := os.Symlink(vhostFile, enabledLink); err != nil {
			logger.LogWarn(fmt.Sprintf("Failed to symlink %s -> %s: %v", vhostFile, enabledLink, err))
		}
	}

	bin := system.GetOpenRestyBinPath()
	if bin == "" {
		bin = "openresty"
	}
	if out, err := exec.Command(bin, "-t").CombinedOutput(); err != nil {
		_ = os.Remove(vhostFile)
		if enabledLink != "" {
			_ = os.Remove(enabledLink)
		}
		tui.PrintInBox("OpenResty Configuration Error", []string{
			core.RED + "Configuration syntax check failed! Vhost was removed to prevent server crash:" + core.NC,
			"",
			core.SanitizeTerminalText(string(out)),
		})
		return
	}
	system.ReloadService("openresty")

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
					_, _, _ = system.ExecRHELCmd("", false, false, "install", "-y", "epel-release")
					certbotReady = system.RunRHELStep("Installing Certbot (EPEL)", true, "install", "-y", "certbot", "python3-certbot-nginx")
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
				confDir := system.GetOpenRestyConfDir()
				if confDir != "/etc/nginx" {
					certArgs = append(certArgs, "--nginx-server-root", confDir, "--nginx-ctl", bin)
				}
				if !system.RunStep("Generating SSL Certificate (Let's Encrypt)", "certbot", certArgs...) {
					sslFailed = true
					sslErrLines = system.ExtractRecentLogErrors(core.LogFilePath, 4)
					logger.LogWarn(fmt.Sprintf("Let's Encrypt SSL acquisition failed for '%s'. Virtual host remains active on HTTP port 80.", domainName))
					sslStrat.Mode = "none"
				} else {
					system.ReloadService("openresty")
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
		if tui.AskYesNo("Generate systemd/OpenRC background service to keep your backend app running 24/7?") == 0 {
			GenerateSystemdServiceWizard()
		}
	}
}

func DoAddDomainDocker(composeDir string) {
	if strings.TrimSpace(composeDir) == "" {
		composeDir = "/etc/openresty-docker"
	}
	composeDir = filepath.Clean(strings.TrimSpace(composeDir))
	if err := system.ValidateSafeComposeDir(composeDir); err != nil {
		tui.PrintInBox("Validation Error", []string{core.RED + err.Error() + core.NC})
		return
	}
	tui.ClearScreen()
	cardLines := []string{
		core.BOLD + core.CYAN + "Add New Virtual Host / Domain (Docker)" + core.NC,
		"",
		"  Specify the primary domain name you want to configure for Docker OpenResty.",
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
		siteRoot := "/usr/local/openresty/nginx/html/" + domainName
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
	if exec.Command("docker", "exec", "krotek-openresty", "openresty", "-t").Run() == nil || exec.Command("docker", "exec", "krotek-openresty", "nginx", "-t").Run() == nil {
		if exec.Command("docker", "exec", "krotek-openresty", "openresty", "-s", "reload").Run() != nil {
			_ = exec.Command("docker", "exec", "krotek-openresty", "nginx", "-s", "reload").Run()
		}
		addRes = append(addRes, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
		addRes = append(addRes, tui.FormatKV("Target Domain", domainName, core.CYAN))
		addRes = append(addRes, tui.FormatKV("Configuration File", vhostFile, core.YELLOW))
		if !wantProxy {
			addRes = append(addRes, tui.FormatKV("Web Root Directory", filepath.Join(composeDir, "html", domainName), core.CYAN))
		}
		addRes = append(addRes, tui.FormatKV("OpenResty Container", "Syntax OK & Service Reloaded", core.GREEN))
	} else {
		addRes = append(addRes, tui.FormatKV("Operation Status", "WARNING", core.YELLOW))
		addRes = append(addRes, tui.FormatKV("Target Domain", domainName, core.CYAN))
		addRes = append(addRes, tui.FormatKV("Configuration File", vhostFile, core.YELLOW))
		if !wantProxy {
			addRes = append(addRes, tui.FormatKV("Web Root Directory", filepath.Join(composeDir, "html", domainName), core.CYAN))
		}
		addRes = append(addRes, tui.FormatKV("OpenResty Container", "Syntax test failed or container offline", core.RED))
	}
	tui.PrintInBox("Add Virtual Host Result (Docker)", addRes)
}

func validateDomainName(domain string) bool {
	return system.ValidateDomainName(domain)
}

func validateProxyTarget(target string) bool {
	return system.ValidateProxyTarget(target)
}
