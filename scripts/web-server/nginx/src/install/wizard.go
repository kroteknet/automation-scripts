// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Nginx High-Performance Web Server & Reverse Proxy (nginx)
//
// This package orchestrates automated pre-flight checks, package repository setup,
// configuration templating, service enablement (systemd/openrc), and firewall rules.
// Supports both interactive TUI step-by-step guidance and headless automation.
// ==============================================================================

package install

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"krotek-nginx-installer/core"
	"krotek-nginx-installer/logger"
	"krotek-nginx-installer/modules"
	"krotek-nginx-installer/system"
	"krotek-nginx-installer/tui"
)

// FreshInstallWizard handles the end-to-end installation pipeline for Nginx High-Performance Web Server & Reverse Proxy.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	accessMode := "local"
	envType := targetEnv // "native" or "docker"
	port := "80"
	domainName := ""
	sslStrat := system.SSLStrategy{Mode: "none"}
	wantPHP := false
	ncDir := "/var/www/html"
	composeDir := "/etc/nginx-docker"

	// ─── UNATTENDED / NON-INTERACTIVE AUTOMATION MODE ─────────────────────────
	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" || os.Getenv("NON_INTERACTIVE") == "1" {
		if envPort := os.Getenv("KROTEK_PORT"); envPort != "" {
			port = envPort
		}
		if envDomain := os.Getenv("KROTEK_DOMAIN"); envDomain != "" {
			domainName = system.CleanDomain(envDomain)
			if domainName != "" {
				accessMode = "public"
				if ncDir == "/var/www/html" {
					ncDir = "/var/www/" + domainName
				}
				sslStrat = system.ParseSSLStrategyFromEnv(domainName)
			}
		}
		if os.Getenv("KROTEK_PHP") == "1" || os.Getenv("KROTEK_PHP") == "true" {
			wantPHP = true
		}
		if envMode := os.Getenv("KROTEK_MODE"); envMode != "" {
			envType = envMode
		}

		logger.LogInfo(fmt.Sprintf("Starting Unattended NGINX installation: mode=%s, port=%s, domain=%s, ssl_mode=%s", envType, port, domainName, sslStrat.Mode))
		modules.CheckConflicts()
		if envType == "native" {
			ExecuteNativeInstall(accessMode, port, domainName, sslStrat, wantPHP, ncDir)
		} else {
			ExecuteDockerInstall(accessMode, port, domainName, composeDir)
		}
		return
	}

	PromptExistingConfigAction("NGINX Web Server", "/etc/nginx/nginx.conf")

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/4: Choose Access Scope (Native NGINX)"
			if envType == "docker" {
				wizardTitle = "Step 1/4: Choose Access Scope (Docker NGINX)"
			}

			idx, _, status := tui.SelectMenu(wizardTitle, []string{
				"Local Network Only (LAN / Private Server)",
				"Public Access (Domain + Internet Access)",
			})
			if status != 0 {
				return
			}
			if idx == 0 {
				accessMode = "local"
				port = "80"
				domainName = ""
				sslStrat = system.SSLStrategy{Mode: "none"}
				ncDir = "/var/www/html"
			} else {
				accessMode = "public"
				port = "80"
			}
			currentStep = 2

		case 2:
			tui.ClearScreen()
			if accessMode == "local" {
				cardLines := []string{
					core.BOLD + core.CYAN + "Local Network Configuration" + core.NC,
					"",
					"  Specify the NGINX listening web port for LAN / Wi-Fi access.",
					"  " + core.BOLD + "Default Port:" + core.NC + " " + core.CYAN + "80" + core.NC,
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: LAN Network Options", cardLines)

				portIn, st := tui.ReadTextInput("Nginx Port", port)
				if st != 0 {
					currentStep = 1
					continue
				}
				if resolvedPort, ok := system.ResolvePortConflict(portIn, "tcp", true); !ok {
					currentStep = 1
					continue
				} else {
					port = resolvedPort
				}
				domainName = ""
				sslStrat = system.SSLStrategy{Mode: "none"}
				ncDir = "/var/www/html"
			} else {
				cardLines := []string{
					core.BOLD + core.CYAN + "Public Internet Domain & SSL Setup" + core.NC,
					"",
					"  Enter your public Domain Name (e.g. example.com).",
					"  Ensure your domain DNS points to this server's Public IP.",
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: Public Domain Setup", cardLines)

				domIn, st := tui.ReadTextInput("Domain Name (FQDN)", domainName)
				if st != 0 {
					currentStep = 1
					continue
				}
				if domIn == "" {
					currentStep = 2
					continue
				}
				cleanedDom := system.CleanDomain(domIn)
				if !system.ValidateDomainName(cleanedDom) {
					tui.PrintInBox("Invalid Domain Name", []string{
						core.RED + "The domain name you entered is invalid:" + core.NC,
						"  " + core.BOLD + domIn + core.NC,
						"",
						"Please enter a valid FQDN format, e.g.:",
						"  " + core.CYAN + "example.com" + core.NC + " or " + core.CYAN + "sub.example.com" + core.NC,
						"(Protocols like http:// and trailing slashes are automatically stripped)",
					})
					currentStep = 2
					continue
				}
				domainName = cleanedDom
				if ncDir == "/var/www/html" || strings.HasPrefix(ncDir, "/var/www/") {
					ncDir = "/var/www/" + domainName
				}

				portIn, st := tui.ReadTextInput("Nginx Web Port", port)
				if st != 0 {
					currentStep = 2
					continue
				}
				if resolvedPort, ok := system.ResolvePortConflict(portIn, "tcp", true); !ok {
					currentStep = 2
					continue
				} else {
					port = resolvedPort
				}

				strat, ok := system.PromptSSLStrategy(domainName)
				if !ok {
					currentStep = 2
					continue
				}
				sslStrat = strat
			}

			currentStep = 3

		case 3:
			tui.ClearScreen()
			if envType == "native" {
				// Step 3.1: PHP Stack Option
				phpPromptLines := []string{
					core.BOLD + core.CYAN + "PHP & Extension Stack (Optional)" + core.NC,
					"",
					"  Install PHP, PHP-FPM, and common extensions (mbstring, xml, curl, mysql).",
					"  Recommended if you plan to host dynamic sites, WordPress, or PHP apps.",
					"",
					core.BOLD + core.YELLOW + "Install PHP & PHP-FPM extension stack?" + core.NC,
				}
				pIdx, _, pStatus := tui.SelectMenuWithContent("Step 3/4: PHP & Web Stack Configuration", phpPromptLines, []string{"YES", "NO"})
				if pStatus == 1 || pStatus == 2 {
					currentStep = 2
					continue
				}
				wantPHP = (pIdx == 0)

				// Step 3.2: Web Root Directory
				tui.ClearScreen()
				webUser := "nginx"
				if core.SysInfo.Family == "debian" {
					webUser = "www-data"
				}
				rootCardLines := []string{
					core.BOLD + core.CYAN + "Web Root Directory Configuration" + core.NC,
					"",
					"  Specify the root document directory where website files, HTML,",
					"  CSS, JavaScript assets, and scripts will be stored on this server.",
					"",
					"  " + core.BOLD + core.Sym.Pointer + " Default Location :" + core.NC + " " + core.CYAN + ncDir + core.NC,
					"  " + core.BOLD + core.Sym.Pointer + " Permissions      :" + core.NC + " " + core.GREEN + "Auto-configured for " + webUser + core.NC,
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to PHP Selection", "Step 3/4: Web Root Directory Setup", rootCardLines)

				dirIn, st := tui.ReadTextInput("Web Root Directory", ncDir)
				if st != 0 {
					currentStep = 3
					continue
				}
				ncDir = dirIn
			} else if envType == "docker" {
				tui.ClearScreen()
				composeCardLines := []string{
					core.BOLD + core.CYAN + "Docker Compose Directory Configuration" + core.NC,
					"",
					"  Specify the directory where docker-compose.yml, NGINX configuration",
					"  files, HTML files, and persistent volumes will be stored.",
					"",
					"  " + core.BOLD + core.Sym.Pointer + " Default Directory :" + core.NC + " " + core.CYAN + "/etc/nginx-docker" + core.NC,
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: Docker Compose Directory Setup", composeCardLines)

				cDirIn, st := tui.ReadTextInput("Compose Directory", composeDir)
				if st != 0 {
					currentStep = 2
					continue
				}
				composeDir = cDirIn
			}

			currentStep = 4

		case 4:
			tui.ClearScreen()
			summaryLines := []string{
				tui.FormatKV("Access Scope", func() string {
					if accessMode == "public" {
						return "Public Internet (Domain + WAN)"
					}
					return "Local Network Only (LAN)"
				}(), core.GREEN),
				tui.FormatKV("Environment Stack", func() string {
					if envType == "docker" {
						return "Docker Compose Container Stack"
					}
					return "Native Linux Service"
				}(), core.GREEN),
				tui.FormatKV("Target Web Port", port, core.CYAN),
			}

			if domainName != "" {
				summaryLines = append(summaryLines, tui.FormatKV("Public Domain Name", domainName, core.YELLOW))
				switch sslStrat.Mode {
				case "certbot":
					summaryLines = append(summaryLines, tui.FormatKV("SSL / TLS Strategy", fmt.Sprintf("Let's Encrypt (Certbot: %s)", sslStrat.Email), core.GREEN))
				case "cloudflare":
					summaryLines = append(summaryLines, tui.FormatKV("SSL / TLS Strategy", "Cloudflare Edge Proxy (Real-IP Enabled)", core.YELLOW))
				case "custom":
					summaryLines = append(summaryLines, tui.FormatKV("SSL / TLS Strategy", fmt.Sprintf("Custom Certificate (%s)", filepath.Base(sslStrat.CertPath)), core.CYAN))
				default:
					summaryLines = append(summaryLines, tui.FormatKV("SSL / TLS Strategy", "Plain HTTP (Disabled)", core.DIM))
				}
			}

			if envType == "native" {
				summaryLines = append(summaryLines, tui.FormatKV("Web Root Directory", ncDir, core.CYAN))
				if wantPHP {
					summaryLines = append(summaryLines, tui.FormatKV("Install PHP-FPM", "[YES]", core.GREEN))
				} else {
					summaryLines = append(summaryLines, tui.FormatKV("Install PHP-FPM", "[NO]", core.YELLOW))
				}
			} else {
				summaryLines = append(summaryLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
			}

			if accessMode == "public" {
				summaryLines = append(summaryLines, "")
				summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"REQUIREMENTS FOR PUBLIC ACCESS:"+core.NC)
				if sslStrat.Mode == "cloudflare" {
					summaryLines = append(summaryLines, fmt.Sprintf("  1. Cloudflare DNS     : %s%s%s proxied (Orange Cloud active)", core.YELLOW, domainName, core.NC))
					summaryLines = append(summaryLines, fmt.Sprintf("  2. Router Forwarding  : Port %s80%s forwarded to this server", core.CYAN, core.NC))
				} else if sslStrat.Mode == "certbot" || sslStrat.Mode == "custom" {
					summaryLines = append(summaryLines, fmt.Sprintf("  1. DNS A Record       : %s%s%s -> Public IP", core.YELLOW, domainName, core.NC))
					summaryLines = append(summaryLines, fmt.Sprintf("  2. Router Forwarding  : Ports %s80%s & %s443%s forwarded to this server", core.CYAN, core.NC, core.CYAN, core.NC))
				} else {
					summaryLines = append(summaryLines, fmt.Sprintf("  1. DNS A Record       : %s%s%s -> Public IP", core.YELLOW, domainName, core.NC))
					summaryLines = append(summaryLines, fmt.Sprintf("  2. Router Forwarding  : Port %s80%s forwarded to this server", core.CYAN, core.NC))
				}
			}

			summaryLines = append(summaryLines, "")
			summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"Do you want to proceed with installation?"+core.NC)

			choices := []string{"YES", "NO"}

			idx, _, status := tui.SelectMenuWithContent("Step 4/4: Installation Summary", summaryLines, choices)
			if status == 2 || idx != 0 {
				logger.LogInfo("Installation aborted by user.")
				return
			}
			if status == 1 {
				currentStep = 3
				continue
			}

			modules.CheckConflicts()

			if envType == "native" {
				ExecuteNativeInstall(accessMode, port, domainName, sslStrat, wantPHP, ncDir)
			} else {
				ExecuteDockerInstall(accessMode, port, domainName, composeDir)
			}
			return
		}
	}
}

// ==============================================================================
// NATIVE INSTALLATION EXECUTOR
// ==============================================================================
// executeNativeInstall installs the service directly on the host OS.
// Pre-cleans conflicting services and runs the official installer script.
func ExecuteNativeInstall(accessMode string, port string, domain string, sslStrat system.SSLStrategy, wantPHP bool, webDir string) {
	if sslStrat.Mode == "certbot" && domain != "" && system.IsLocalOrPrivateDomain(domain) {
		logger.LogWarn(fmt.Sprintf("Skipping Let's Encrypt for local/private domain '%s'; configuring plain HTTP.", domain))
		sslStrat.Mode = "none"
	}
	if system.IsServiceInstalled() {
		system.StopService("nginx")
		time.Sleep(500 * time.Millisecond)
	}

	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}
	if sslStrat.Mode == "certbot" || sslStrat.Mode == "custom" {
		if _, ok := system.ResolvePortConflict("443", "tcp", false); !ok {
			return
		}
	}
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-nginx").Output(); err == nil && strings.TrimSpace(string(out)) == "running" {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "CONTAINER CONFLICT DETECTED" + core.NC,
			"",
			"  Docker NGINX Container 'krotek-nginx' is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Native NGINX, the Docker container stack must be stopped",
			"  to free network ports (Port 80 / 443).",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop Docker NGINX container stack to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Container Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Docker NGINX container and continue Native deployment?")
		if res != 0 {
			logger.LogInfo("Native installation cancelled by user due to container conflict.")
			return
		}

		if !system.RunStepOrAbort("Stopping conflicting Docker Nginx container", "docker", "stop", "krotek-nginx") {
			return
		}
	}

	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing NGINX Native Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)

	switch core.SysInfo.Family {
	case "alpine":
		if !system.RunStepOrAbort("Updating apk repositories", "apk", "update") {
			return
		}
		if !system.RunStepOrAbort("Installing Nginx package", "apk", "add", "nginx", "nginx-openrc", "curl", "bash") {
			return
		}
		if wantPHP {
			if _, ok := system.InstallPHPStack(); !ok {
				return
			}
		}
		if sslStrat.Mode == "certbot" {
			if !system.RunStepOrAbort("Installing Certbot SSL client", "apk", "add", "--no-cache", "certbot", "certbot-nginx") {
				return
			}
		}
	case "debian":
		if !system.RunStepOrAbort("Updating apt package lists", "apt-get", "update", "-y") {
			return
		}
		aptArgs := append([]string{"install", "-y"}, system.GetDpkgConfigFlags()...)
		aptArgs = append(aptArgs, "nginx", "curl")
		if !system.RunStepOrAbort("Installing Nginx packages", "apt-get", aptArgs...) {
			return
		}
		if wantPHP {
			if _, ok := system.InstallPHPStack(); !ok {
				return
			}
		}
		if sslStrat.Mode == "certbot" {
			if !system.RunStepOrAbort("Installing Certbot SSL client", "apt-get", "install", "-y", "certbot", "python3-certbot-nginx") {
				return
			}
		}
	case "rhel":
		_ = system.RunStep("Refreshing DNF package cache", "dnf", "makecache")
		_ = exec.Command("dnf", "install", "-y", "epel-release").Run()
		if !system.RunStepOrAbort("Installing Nginx packages", "dnf", "install", "-y", "nginx", "curl") {
			return
		}
		if wantPHP {
			if _, ok := system.InstallPHPStack(); !ok {
				return
			}
		}
		if sslStrat.Mode == "certbot" {
			if !system.RunStepOrAbort("Installing Certbot SSL client", "dnf", "install", "-y", "certbot", "python3-certbot-nginx") {
				return
			}
		}
	}

	if wantPHP {
		tui.StartSpinner("Optimizing PHP & OPcache Settings")
		system.OptimizePHPSettings()
		time.Sleep(150 * time.Millisecond)
		tui.StopSpinner(0)
	}

	// Soft firewall configuration (UFW, Firewalld, and Oracle Cloud iptables)
	system.ConfigureFirewallRules(port)
	if sslStrat.Mode == "certbot" || sslStrat.Mode == "custom" {
		system.ConfigureFirewallRules("443")
	}

	ApplyDefaultNginxConfig()

	cleanDomain := system.CleanDomain(domain)
	domainWebDir := webDir
	if cleanDomain != "" {
		if domainWebDir == "" || domainWebDir == "/var/www/html" {
			domainWebDir = "/var/www/" + cleanDomain
		}
	} else {
		if domainWebDir == "" {
			domainWebDir = "/var/www/html"
		}
	}
	defaultWebDir := "/var/www/html"

	webUser := "nginx"
	if core.SysInfo.Family == "debian" {
		webUser = "www-data"
	}

	// 1. Prepare default fallback web directory (/var/www/html)
	_ = os.MkdirAll(defaultWebDir, 0755)
	_ = exec.Command("chmod", "755", "/var").Run()
	_ = exec.Command("chmod", "755", "/var/www").Run()
	system.SecureWebRootPermissions(defaultWebDir, webUser)

	defaultLandingHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
		ServiceName:  "NGINX Web Server",
		LogoText:     "N",
		HeroTitle:    "NGINX High-Performance Engine Active",
		HeroDesc:     "Your production-grade, event-driven HTTP/2 & reverse proxy web server is fully operational.",
		EnvType:      "native",
		Port:         port,
		WebDir:       defaultWebDir,
		ConfigPath:   "/etc/nginx/nginx.conf",
		WebUser:      webUser,
		ManagerCmd:   "nginx-manager",
		PHPInstalled: &wantPHP,
	})
	defaultIndexFile := filepath.Join(defaultWebDir, "index.html")
	if core.OverwriteExistingConfig || func() bool { _, err := os.Lstat(defaultIndexFile); return os.IsNotExist(err) }() {
		_ = os.Remove(filepath.Join(defaultWebDir, "index.nginx-debian.html"))
		_ = system.WriteFileNoFollow(defaultIndexFile, []byte(defaultLandingHTML), 0644, webUser)
	} else {
		logger.LogInfo("Preserving existing " + defaultIndexFile)
	}

	// 2. Prepare domain web directory if configured (/var/www/<domain>)
	if cleanDomain != "" {
		if !system.IsSymlinkPath(domainWebDir) {
			_ = os.MkdirAll(domainWebDir, 0755)
			system.SecureWebRootPermissions(domainWebDir, webUser)
		}

		domainLandingHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
			ServiceName:  "NGINX Web Server",
			LogoText:     "N",
			HeroTitle:    cleanDomain + " Active",
			HeroDesc:     "Production-grade NGINX HTTP/2 & reverse proxy web server is operational for " + cleanDomain + ".",
			EnvType:      "native",
			Port:         port,
			WebDir:       domainWebDir,
			ConfigPath:   "/etc/nginx/nginx.conf",
			WebUser:      webUser,
			ManagerCmd:   "nginx-manager",
			PHPInstalled: &wantPHP,
		})
		domainIndexFile := filepath.Join(domainWebDir, "index.html")
		if core.OverwriteExistingConfig || func() bool { _, err := os.Lstat(domainIndexFile); return os.IsNotExist(err) }() {
			_ = system.WriteFileNoFollow(domainIndexFile, []byte(domainLandingHTML), 0644, webUser)
		} else {
			logger.LogInfo("Preserving existing " + domainIndexFile)
		}
	}

	defaultConf := "/etc/nginx/conf.d/default.conf"
	if core.SysInfo.Family == "debian" {
		defaultConf = "/etc/nginx/sites-available/default"
	} else if core.SysInfo.Family == "alpine" {
		defaultConf = "/etc/nginx/http.d/default.conf"
	}

	serverName := "_"

	indexFiles := "index.html index.htm"
	phpLocationBlock := ""
	if wantPHP {
		indexFiles = "index.html index.php index.htm"
		phpLocationBlock = system.GetPHPFastCGIBlock()

		targetDirs := []string{defaultWebDir}
		if cleanDomain != "" && domainWebDir != defaultWebDir {
			targetDirs = append(targetDirs, domainWebDir)
		}

		for _, td := range targetDirs {
			probePath := filepath.Join(td, "php-status.php")
			_ = system.WriteFileNoFollow(probePath, []byte(system.GeneratePHPProbe()), 0644, webUser)

			phpSafeFile := `<?php
if (file_exists(__DIR__ . '/index.html')) {
    header('Location: index.html');
    exit;
}
header('Content-Type: text/html; charset=utf-8');
?>
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"><title>PHP Active &bull; NGINX</title>
<style>body{font-family:system-ui,sans-serif;text-align:center;padding:50px;background:#0b0f17;color:#f8fafc;}h1{color:#38bdf8;}.badge{display:inline-block;padding:6px 14px;background:rgba(56,189,248,0.15);border:1px solid rgba(56,189,248,0.3);color:#38bdf8;border-radius:9999px;margin:10px;font-weight:600;}</style>
</head>
<body><h1>NGINX Engine Active</h1><div class="badge">PHP <?php echo htmlspecialchars(phpversion(), ENT_QUOTES, 'UTF-8'); ?> Active</div><p style="color:#94a3b8;">Managed by NGINX Toolkit.</p></body>
</html>`
			phpIndexPath := filepath.Join(td, "index.php")
			if core.OverwriteExistingConfig || func() bool { _, err := os.Lstat(phpIndexPath); return os.IsNotExist(err) }() {
				_ = system.WriteFileNoFollow(phpIndexPath, []byte(phpSafeFile), 0644, webUser)
			} else {
				logger.LogInfo("Preserving existing " + phpIndexPath)
			}
		}
	}

	vhostContent := fmt.Sprintf(`server {
    listen %s default_server;
    listen [::]:%s default_server;
    server_name %s;

    root %s;
    index %s;

    location / {
        try_files $uri $uri/ /index.html =404;
    }
%s}
`, port, port, serverName, defaultWebDir, indexFiles, phpLocationBlock)

	shouldWriteDefaultVhost := true
	if !core.OverwriteExistingConfig {
		if _, err := os.Stat(defaultConf); err == nil {
			shouldWriteDefaultVhost = false
			logger.LogInfo(fmt.Sprintf("Preserving existing default virtual host at %s", defaultConf))
		} else if core.SysInfo.Family == "debian" {
			if entries, err := os.ReadDir("/etc/nginx/sites-enabled"); err == nil && len(entries) > 0 {
				shouldWriteDefaultVhost = false
				logger.LogInfo("Preserving existing sites-enabled virtual hosts")
			}
		}
	}

	if shouldWriteDefaultVhost {
		_ = os.Remove("/etc/nginx/http.d/default.conf")
		_ = os.Remove("/etc/nginx/conf.d/default.conf")
		_ = os.Remove("/etc/nginx/sites-enabled/default")
		_ = os.Remove("/etc/nginx/sites-available/default")

		_ = os.MkdirAll(filepath.Dir(defaultConf), 0755)
		_ = system.WriteFileNoFollow(defaultConf, []byte(vhostContent), 0644, "")
		if core.SysInfo.Family == "debian" {
			_ = os.MkdirAll("/etc/nginx/sites-enabled", 0755)
			_ = os.Remove("/etc/nginx/sites-enabled/default")
			_ = os.Symlink(defaultConf, "/etc/nginx/sites-enabled/default")
		}
	}

	if cleanDomain != "" {
		domainConf := fmt.Sprintf("/etc/nginx/conf.d/%s.conf", cleanDomain)
		if core.SysInfo.Family == "debian" {
			domainConf = fmt.Sprintf("/etc/nginx/sites-available/%s", cleanDomain)
		} else if core.SysInfo.Family == "alpine" {
			domainConf = fmt.Sprintf("/etc/nginx/http.d/%s.conf", cleanDomain)
		}
		shouldWriteDomainConf := true
		if !core.OverwriteExistingConfig {
			if _, err := os.Lstat(domainConf); err == nil {
				shouldWriteDomainConf = false
				logger.LogInfo(fmt.Sprintf("Preserving existing domain virtual host at %s", domainConf))
			}
		}
		if shouldWriteDomainConf {
			domainIndexFiles := "index.html index.htm"
			domainPhpBlock := ""
			if wantPHP {
				domainIndexFiles = "index.html index.php index.htm"
				domainPhpBlock = system.GetPHPFastCGIBlock()
			}

			var domainVhostContent string
			switch sslStrat.Mode {
			case "custom":
				domainVhostContent = fmt.Sprintf(`server {
    listen %s;
    listen [::]:%s;
    server_name %s www.%s;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name %s www.%s;

    root %s;
    index %s;

%s
    # Gzip Compression
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;

    location / {
        try_files $uri $uri/ =404;
    }
%s}
`, port, port, cleanDomain, cleanDomain, cleanDomain, cleanDomain, domainWebDir, domainIndexFiles, system.GetCustomSSLBlock(sslStrat.CertPath, sslStrat.KeyPath), domainPhpBlock)

			case "cloudflare":
				domainVhostContent = fmt.Sprintf(`server {
    listen %s;
    listen [::]:%s;
    server_name %s www.%s;

    root %s;
    index %s;

%s
    # Gzip Compression
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;

    location / {
        try_files $uri $uri/ =404;
    }
%s}
`, port, port, cleanDomain, cleanDomain, domainWebDir, domainIndexFiles, system.GetCloudflareRealIPBlock(), domainPhpBlock)

			default:
				domainVhostContent = fmt.Sprintf(`server {
    listen %s;
    listen [::]:%s;
    server_name %s www.%s;

    root %s;
    index %s;

    # Gzip Compression
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;

    location / {
        try_files $uri $uri/ =404;
    }
%s}
`, port, port, cleanDomain, cleanDomain, domainWebDir, domainIndexFiles, domainPhpBlock)
			}

			_ = os.MkdirAll(filepath.Dir(domainConf), 0755)
			if err := system.WriteFileNoFollow(domainConf, []byte(domainVhostContent), 0644, ""); err != nil {
				logger.LogError(fmt.Sprintf("Failed to write domain configuration to %s: %v", domainConf, err))
			} else {
				logger.LogSuccess(fmt.Sprintf("Generated domain virtual host at %s", domainConf))
			}

			if core.SysInfo.Family == "debian" {
				_ = os.MkdirAll("/etc/nginx/sites-enabled", 0755)
				enabledLink := fmt.Sprintf("/etc/nginx/sites-enabled/%s", cleanDomain)
				_ = os.Remove(enabledLink)
				if err := os.Symlink(domainConf, enabledLink); err != nil {
					logger.LogError(fmt.Sprintf("Failed to symlink %s -> %s: %v", domainConf, enabledLink, err))
				} else {
					logger.LogSuccess(fmt.Sprintf("Enabled domain virtual host: %s", enabledLink))
				}
			}
		}
	}

	if !system.RunStepOrAbort("Testing Nginx configuration syntax", "nginx", "-t") {
		return
	}

	_ = os.MkdirAll("/run/nginx", 0755)
	_ = os.MkdirAll("/var/log/nginx", 0755)
	if core.SysInfo.Family == "alpine" {
		_ = exec.Command("chown", "-R", "nginx:nginx", "/run/nginx").Run()
		_ = exec.Command("chmod", "775", "/run/nginx").Run()
	}

	startCmd := "systemctl enable nginx 2>/dev/null; systemctl restart nginx 2>/dev/null || systemctl start nginx 2>/dev/null || service nginx restart 2>/dev/null || service nginx start 2>/dev/null || /etc/init.d/nginx restart 2>/dev/null || /etc/init.d/nginx start 2>/dev/null || nginx 2>/dev/null"
	if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
		_ = exec.Command("rc-service", "nginx", "zap").Run()
		_ = exec.Command("rc-update", "add", "nginx", "default").Run()
		startCmd = "rc-service nginx zap 2>/dev/null; rc-service nginx restart 2>/dev/null || rc-service nginx start 2>/dev/null || /etc/init.d/nginx restart 2>/dev/null || /etc/init.d/nginx start 2>/dev/null || nginx 2>/dev/null"
	}

	if !system.RunMultiStepOrAbort("Enabling & Starting Nginx service", startCmd) {
		return
	}
	modules.EnsureNginxWorking()
	if wantPHP {
		system.StartPHPFPMService()
	}

	sslFailed := false
	var sslErrLines []string
	if sslStrat.Mode == "certbot" && cleanDomain != "" {
		certEmail := sslStrat.Email
		if certEmail == "" {
			certEmail = "admin@" + cleanDomain
		}
		if !system.RunStep("Obtaining Let's Encrypt SSL Certificate", "certbot", "--nginx", "-d", cleanDomain, "--non-interactive", "--agree-tos", "-m", certEmail, "--redirect") {
			sslFailed = true
			sslErrLines = system.ExtractRecentLogErrors(core.LogFilePath, 4)
			logger.LogWarn(fmt.Sprintf("Certbot SSL acquisition failed for '%s'. Ensure DNS A record points to this server, then run: sudo certbot --nginx -d %s", cleanDomain, cleanDomain))
			sslStrat.Mode = "none"
		} else {
			logger.LogSuccess("Let's Encrypt SSL certificate configured successfully!")
		}
	} else if sslStrat.Mode == "cloudflare" {
		logger.LogSuccess("Cloudflare Real-IP rules active. Local SSL termination delegated to Cloudflare Edge.")
	} else if sslStrat.Mode == "custom" {
		logger.LogSuccess(fmt.Sprintf("Custom SSL certificate installed from %s", sslStrat.CertPath))
	}

	targetWebDir := defaultWebDir
	if cleanDomain != "" {
		targetWebDir = domainWebDir
	}

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "native",
		"ACCESS_MODE": accessMode,
		"PORT":        port,
		"DOMAIN_NAME": cleanDomain,
		"ENABLE_SSL":  fmt.Sprintf("%t", sslStrat.Mode != "none"),
		"SSL_MODE":    sslStrat.Mode,
		"WEB_DIR":     targetWebDir,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	localIP := "127.0.0.1"
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				localIP = ipnet.IP.String()
				break
			}
		}
	}

	summaryLines := []string{
		tui.FormatKV("Deployment Mode", "Native Linux Stack", core.GREEN),
		tui.FormatKV("Local LAN Access URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN),
		tui.FormatKV("Localhost Access URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN),
	}
	if cleanDomain != "" {
		if sslStrat.Mode == "certbot" || sslStrat.Mode == "custom" {
			summaryLines = append(summaryLines, tui.FormatKV("Public Web Domain URL", "https://"+cleanDomain, core.GREEN))
		} else {
			summaryLines = append(summaryLines, tui.FormatKV("Public Web Domain URL", fmt.Sprintf("http://%s:%s", cleanDomain, port), core.YELLOW))
		}
		if sslFailed {
			summaryLines = append(summaryLines, tui.FormatKV("SSL/TLS Mode", "NONE (Certbot Failed)", core.RED))
			summaryLines = append(summaryLines, core.YELLOW+"  SSL Notice: Let's Encrypt failed (check DNS A record & port 80); HTTP remains active."+core.NC)
			for _, el := range sslErrLines {
				summaryLines = append(summaryLines, "  "+core.RED+core.Sym.Pointer+" "+el+core.NC)
			}
			summaryLines = append(summaryLines, tui.FormatKV("Retry SSL", "sudo certbot --nginx -d "+cleanDomain, core.GREEN))
		} else {
			summaryLines = append(summaryLines, tui.FormatKV("SSL/TLS Mode", strings.ToUpper(sslStrat.Mode), core.CYAN))
		}
	}
	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Web Root Directory", targetWebDir, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "nginx-manager", core.GREEN))

	tui.ClearScreen()
	tui.PrintInBox("NGINX Native Stack Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// ==============================================================================
// DOCKER COMPOSE EXECUTOR
// ==============================================================================
// executeDockerInstall deploys the service as a Docker container stack.
// It installs Docker if absent, writes a docker-compose.yml, starts the stack,
// persists state, and displays a post-install summary.
func ExecuteDockerInstall(accessMode, port, domain, composeDir string) {
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing NGINX (Docker Compose) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)

	if !system.EnsureDockerEngine() {
		return
	}

	// Check if Native Nginx service is running and ask user before stopping it
	if (system.IsSystemdRunning() && exec.Command("systemctl", "is-active", "--quiet", "nginx").Run() == nil) ||
		exec.Command("service", "nginx", "status").Run() == nil ||
		exec.Command("pidof", "nginx").Run() == nil {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "SERVICE CONFLICT DETECTED" + core.NC,
			"",
			"  Native NGINX Linux Service is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Docker NGINX, the Native service must be stopped & disabled",
			"  to free network ports (Port 80 / 443).",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop & Disable Native NGINX service to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Service Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Native NGINX service and continue Docker deployment?")
		if res != 0 {
			logger.LogInfo("Docker installation cancelled by user due to service conflict.")
			return
		}

		system.StopService("nginx")
		_ = exec.Command("systemctl", "disable", "nginx").Run()
		_ = exec.Command("update-rc.d", "nginx", "disable").Run()
	}

	if composeDir == "" {
		composeDir = "/etc/nginx-docker"
	}
	confDir := filepath.Join(composeDir, "conf.d")
	htmlDir := filepath.Join(composeDir, "html")
	_ = os.MkdirAll(confDir, 0755)
	_ = os.MkdirAll(htmlDir, 0755)

	composeContent := fmt.Sprintf(`version: '3.8'

services:
  nginx:
    image: nginx:alpine
    container_name: krotek-nginx
    restart: unless-stopped
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
    ports:
      - "%s:80"
      - "443:443"
    volumes:
      - ./html:/usr/share/nginx/html
      - ./conf.d:/etc/nginx/conf.d
    healthcheck:
      test: ["CMD-SHELL", "wget -q --spider http://localhost/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`, port)

	_ = system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeContent), 0600, "")

	defaultConfTpl := `server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _;

    root /usr/share/nginx/html;
    index index.html index.htm;

    location / {
        try_files $uri $uri/ =404;
    }
}
`
	_ = system.WriteFileNoFollow(filepath.Join(confDir, "default.conf"), []byte(defaultConfTpl), 0644, "")

	cleanDomain := system.CleanDomain(domain)
	targetDockerWebDir := htmlDir
	if cleanDomain != "" {
		targetDockerWebDir = filepath.Join(htmlDir, cleanDomain)
		_ = os.MkdirAll(targetDockerWebDir, 0755)

		dockerDomainConf := filepath.Join(confDir, cleanDomain+".conf")
		shouldWriteDomainConf := true
		if !core.OverwriteExistingConfig {
			if _, err := os.Lstat(dockerDomainConf); err == nil {
				shouldWriteDomainConf = false
				logger.LogInfo(fmt.Sprintf("Preserving existing Docker domain virtual host at %s", dockerDomainConf))
			}
		}
		if shouldWriteDomainConf {
			domainConfTpl := fmt.Sprintf(`server {
    listen 80;
    server_name %s www.%s;

    root /usr/share/nginx/html/%s;
    index index.html index.htm;

    # Gzip Compression
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;

    location / {
        try_files $uri $uri/ =404;
    }
}
`, cleanDomain, cleanDomain, cleanDomain)
			if err := system.WriteFileNoFollow(dockerDomainConf, []byte(domainConfTpl), 0644, ""); err != nil {
				logger.LogError(fmt.Sprintf("Failed to write Docker domain configuration to %s: %v", dockerDomainConf, err))
			} else {
				logger.LogSuccess(fmt.Sprintf("Generated Docker domain configuration at %s", dockerDomainConf))
			}

			domainLandingHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
				ServiceName: "NGINX Web Server",
				LogoText:    "N",
				HeroTitle:   cleanDomain + " Active",
				HeroDesc:    "Your containerized NGINX web server stack is running and ready to serve traffic for " + cleanDomain + ".",
				EnvType:     "docker",
				Port:        port,
				WebDir:      targetDockerWebDir,
				ConfigPath:  dockerDomainConf,
				WebUser:     "root",
				ManagerCmd:  "nginx-manager",
			})
			domainIndexFile := filepath.Join(targetDockerWebDir, "index.html")
			if core.OverwriteExistingConfig || func() bool { _, err := os.Lstat(domainIndexFile); return os.IsNotExist(err) }() {
				_ = system.WriteFileNoFollow(domainIndexFile, []byte(domainLandingHTML), 0644, "")
			}
		}
	}

	dockerLandingHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
		ServiceName: "NGINX Web Server",
		LogoText:    "N",
		HeroTitle:   "Nginx Docker Stack Active",
		HeroDesc:    "Your containerized NGINX web server stack is running and ready to serve traffic.",
		EnvType:     "docker",
		Port:        port,
		WebDir:      composeDir + "/html",
		ConfigPath:  composeDir + "/conf.d/default.conf",
		WebUser:     "root",
		ManagerCmd:  "nginx-manager",
	})
	_ = system.WriteFileNoFollow(filepath.Join(htmlDir, "index.html"), []byte(dockerLandingHTML), 0644, "")

	if !system.RunDockerCompose(composeDir, "Deploying Nginx container stack", true, "up", "-d") {
		return
	}

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "docker",
		"ACCESS_MODE": accessMode,
		"PORT":        port,
		"DOMAIN_NAME": cleanDomain,
		"COMPOSE_DIR": composeDir,
		"WEB_DIR":     targetDockerWebDir,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	localIP := "127.0.0.1"
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				localIP = ipnet.IP.String()
				break
			}
		}
	}

	summaryLines := []string{
		tui.FormatKV("Deployment Mode", "Docker Compose Container Stack", core.GREEN),
		tui.FormatKV("Local LAN Access URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN),
		tui.FormatKV("Localhost Access URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN),
	}
	if cleanDomain != "" {
		summaryLines = append(summaryLines, tui.FormatKV("Public Web Domain URL", fmt.Sprintf("http://%s:%s", cleanDomain, port), core.YELLOW))
	}
	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Web Root Directory", targetDockerWebDir, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Container Management", fmt.Sprintf("cd %s && docker compose ps", composeDir), core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "nginx-manager", core.GREEN))

	tui.ClearScreen()
	tui.PrintInBox("NGINX Docker Stack Started Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// commandExists checks whether a given command is available in the system PATH.

func ApplyDefaultNginxConfig() {
	if !core.OverwriteExistingConfig {
		if _, err := os.Stat("/etc/nginx/nginx.conf"); err == nil {
			logger.LogInfo("Preserving existing /etc/nginx/nginx.conf configuration.")
			return
		}
	}
	nginxUser := "nginx"
	if core.SysInfo.Family == "debian" {
		nginxUser = "www-data"
	}

	pidPath := "/run/nginx.pid"
	_ = os.MkdirAll("/run/nginx", 0755)
	_ = os.MkdirAll("/var/log/nginx", 0755)
	if core.SysInfo.Family == "alpine" {
		_ = exec.Command("chown", "-R", "nginx:nginx", "/run/nginx").Run()
		_ = exec.Command("chmod", "775", "/run/nginx").Run()
		if data, err := os.ReadFile("/etc/init.d/nginx"); err == nil {
			if strings.Contains(string(data), "/run/nginx/nginx.pid") {
				pidPath = "/run/nginx/nginx.pid"
			}
		}
	}

	// Clean up default 404 dummy configs shipped by distros (especially Alpine's /etc/nginx/http.d/default.conf)
	_ = os.Remove("/etc/nginx/http.d/default.conf")
	_ = os.Remove("/etc/nginx/conf.d/default.conf")
	_ = os.Remove("/etc/nginx/sites-enabled/default")
	_ = os.Remove("/etc/nginx/sites-available/default")

	config := fmt.Sprintf(`# /etc/nginx/nginx.conf
# Tuned by Krotek NGINX Installer (Go Native Engine)

user %s;
worker_processes auto;
pid %s;

events {
    worker_connections 1024;
    multi_accept on;
}

http {
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;
    server_tokens off;

    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers on;

    access_log /var/log/nginx/access.log;
    error_log /var/log/nginx/error.log warn;

    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;

    include /etc/nginx/conf.d/*.conf;
    include /etc/nginx/sites-enabled/*;
    include /etc/nginx/http.d/*.conf;
}
`, nginxUser, pidPath)
	_ = os.MkdirAll("/etc/nginx/conf.d", 0755)
	_ = os.MkdirAll("/etc/nginx/sites-enabled", 0755)
	_ = system.WriteFileNoFollow("/etc/nginx/nginx.conf", []byte(config), 0644, "")
}

// PromptExistingConfigAction checks if prior configuration files exist on the host system.
// In interactive TUI mode, it prompts the administrator to choose whether to preserve existing custom settings
// or perform a clean factory default reset. In unattended mode (-y), it automatically preserves settings (safe default).
func PromptExistingConfigAction(serviceTitle string, configPaths ...string) {
	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
		return
	}

	hasExisting := false
	for _, p := range configPaths {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				hasExisting = true
				break
			}
		}
	}
	if !hasExisting {
		return
	}

	tui.ClearScreen()
	cardLines := []string{
		core.BOLD + core.YELLOW + "EXISTING CONFIGURATION DETECTED" + core.NC,
		"",
		fmt.Sprintf("  Prior configuration files were detected for %s on this host.", serviceTitle),
		"  Choose how the deployment wizard should handle existing settings:",
		"",
		"  " + core.GREEN + "[1] Keep Existing Configuration (Recommended)" + core.NC,
		"      Preserves all existing configuration, virtual hosts, and custom data intact.",
		"",
		"  " + core.YELLOW + "[2] Clean Reset to Factory Default (Overwrite)" + core.NC,
		"      Overwrites configuration with fresh, production-hardened defaults.",
	}

	choices := []string{
		"Keep Existing Configuration (Preserve custom settings & virtual hosts)",
		"Clean Reset to Factory Default (Overwrite with fresh configurations)",
	}

	idx, _, status := tui.SelectMenuWithContent("Configuration Conflict Handler", cardLines, choices)
	if status == 2 {
		tui.LeaveAltScreen()
		logger.LogInfo("Setup cancelled by user at configuration conflict prompt.")
		os.Exit(0)
	}
	if status == 0 && idx == 1 {
		core.OverwriteExistingConfig = true
		logger.LogInfo("User selected: Clean reset to factory default configuration.")
	} else {
		core.OverwriteExistingConfig = false
		logger.LogInfo("User selected: Preserve existing configuration.")
	}
}

// ==============================================================================
// MULTI-STEP UNINSTALLATION WIZARD
// ==============================================================================
