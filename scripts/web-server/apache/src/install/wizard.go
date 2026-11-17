// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Apache HTTP Server 2.4+ (httpd) (apache)
//
// This package orchestrates automated pre-flight checks, package repository setup,
// configuration templating, service enablement (systemd/openrc), and firewall rules.
// Supports both interactive TUI step-by-step guidance and headless automation.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"krotek-apache-installer/core"
	"krotek-apache-installer/logger"
	"krotek-apache-installer/modules"
	"krotek-apache-installer/system"
	"krotek-apache-installer/tui"
)

// FreshInstallWizard handles the end-to-end installation pipeline for Apache HTTP Server 2.4+ (httpd).
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	accessMode := "local"
	envType := targetEnv // "native" or "docker"
	if envType == "" {
		envType = "native"
	}
	port := "80"
	domainName := ""
	sslStrat := system.SSLStrategy{Mode: "none"}
	wantPHP := false
	webDir := "/var/www/html"
	composeDir := "/etc/apache-docker"

	// ─── UNATTENDED / NON-INTERACTIVE AUTOMATION MODE ─────────────────────────
	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" || os.Getenv("NON_INTERACTIVE") == "1" {
		if envMode := os.Getenv("KROTEK_MODE"); envMode != "" {
			envType = envMode
		}
		if envPort := os.Getenv("KROTEK_PORT"); envPort != "" {
			port = envPort
		}
		if envDom := os.Getenv("KROTEK_DOMAIN"); envDom != "" {
			domainName = system.CleanDomain(envDom)
			accessMode = "public"
		}
		sslStrat = system.ParseSSLStrategyFromEnv(domainName)

		logger.LogInfo(fmt.Sprintf("Starting unattended installation: mode=%s", envType))
		if envType == "native" {
			ExecuteNativeInstall(accessMode, port, domainName, sslStrat, wantPHP, webDir)
		} else {
			ExecuteDockerInstall(accessMode, port, domainName, composeDir)
		}
		return
	}
	PromptExistingConfigAction("Apache HTTP Web Server", "/etc/apache2/apache2.conf", "/etc/httpd/conf/httpd.conf")

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/4: Choose Access Scope (Native APACHE)"
			if envType == "docker" {
				wizardTitle = "Step 1/4: Choose Access Scope (Docker APACHE)"
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
					"  Specify the APACHE listening web port for LAN / Wi-Fi access.",
					"  " + core.BOLD + "Default Port:" + core.NC + " " + core.CYAN + "80" + core.NC,
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: LAN Network Options", cardLines)

				portIn, st := tui.ReadTextInput("Apache Port", port)
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
				domainName = system.CleanDomain(domIn)
				domainName = system.CleanDomain(domIn)

				portIn, st := tui.ReadTextInput("Apache Web Port", port)
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

				strat, ok := system.PromptSSLStrategy(domainName)
				if !ok {
					currentStep = 1
					continue
				}
				sslStrat = strat
			}

			currentStep = 3

		case 3:
			tui.ClearScreen()
			if envType == "native" {
				phpPromptLines := []string{
					core.BOLD + core.CYAN + "PHP & Extension Stack (Optional)" + core.NC,
					"",
					"  Install PHP, libapache2-mod-php / PHP-FPM, and common modules.",
					"  Recommended if you plan to host dynamic sites, WordPress, or PHP apps.",
					"",
					core.BOLD + core.YELLOW + "Install PHP & Apache module stack?" + core.NC,
				}
				pIdx, _, pStatus := tui.SelectMenuWithContent("Step 3/4: PHP & Web Stack Configuration", phpPromptLines, []string{"YES", "NO"})
				if pStatus == 1 || pStatus == 2 {
					currentStep = 2
					continue
				}
				wantPHP = (pIdx == 0)

				tui.ClearScreen()
				webUser := "apache"
				if core.SysInfo.Family == "debian" {
					webUser = "www-data"
				}
				rootCardLines := []string{
					core.BOLD + core.CYAN + "Web Root Directory Configuration" + core.NC,
					"",
					"  Specify the root document directory where website files, HTML,",
					"  CSS, JavaScript assets, and scripts will be stored on this server.",
					"",
					"  " + core.BOLD + core.Sym.Pointer + " Default Location :" + core.NC + " " + core.CYAN + "/var/www/html" + core.NC,
					"  " + core.BOLD + core.Sym.Pointer + " Permissions      :" + core.NC + " " + core.GREEN + "Auto-configured for " + webUser + core.NC,
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: Web Root Directory Setup", rootCardLines)

				dirIn, st := tui.ReadTextInput("Web Root Directory", webDir)
				if st != 0 {
					currentStep = 2
					continue
				}
				webDir = dirIn
			} else if envType == "docker" {
				tui.ClearScreen()
				composeCardLines := []string{
					core.BOLD + core.CYAN + "Docker Compose Directory Configuration" + core.NC,
					"",
					"  Specify the directory where docker-compose.yml, Apache configuration",
					"  files, HTML files, and persistent volumes will be stored.",
					"",
					"  " + core.BOLD + core.Sym.Pointer + " Default Directory :" + core.NC + " " + core.CYAN + "/etc/apache-docker" + core.NC,
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
			}
			if accessMode == "public" {
				summaryLines = append(summaryLines, tui.FormatKV("SSL Strategy", func() string {
					switch sslStrat.Mode {
					case "certbot":
						return fmt.Sprintf("[CERTBOT] Let's Encrypt (%s)", sslStrat.Email)
					case "cloudflare":
						return "[CLOUDFLARE] Edge Proxy (Real-IP restoration)"
					case "custom":
						return fmt.Sprintf("[CUSTOM] Cert: %s", sslStrat.CertPath)
					default:
						return "[NONE] Plain HTTP Port 80"
					}
				}(), core.CYAN))
			}

			if envType == "native" {
				summaryLines = append(summaryLines, tui.FormatKV("Web Root Directory", webDir, core.CYAN))
				if wantPHP {
					summaryLines = append(summaryLines, tui.FormatKV("Install PHP Stack", "[YES]", core.GREEN))
				} else {
					summaryLines = append(summaryLines, tui.FormatKV("Install PHP Stack", "[NO]", core.YELLOW))
				}
			} else {
				summaryLines = append(summaryLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
			}
			summaryLines = append(summaryLines, tui.FormatKV("Automated Firewall", "[ENABLED] (Optimal Ports Opened)", core.CYAN))

			if accessMode == "public" {
				summaryLines = append(summaryLines, "")
				summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"REQUIREMENTS FOR PUBLIC ACCESS:"+core.NC)
				summaryLines = append(summaryLines, fmt.Sprintf("  1. DNS A Record       : %s%s%s -> Public IP", core.YELLOW, domainName, core.NC))
				summaryLines = append(summaryLines, fmt.Sprintf("  2. Router Forwarding  : Ports %s80%s & %s443%s to this server", core.CYAN, core.NC, core.CYAN, core.NC))
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

			if envType == "native" {
				ExecuteNativeInstall(accessMode, port, domainName, sslStrat, wantPHP, webDir)
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
func ExecuteNativeInstall(accessMode string, port string, domain string, sslStrat system.SSLStrategy, wantPHP bool, webDir string) {
	if sslStrat.Mode == "certbot" && domain != "" && system.IsLocalOrPrivateDomain(domain) {
		logger.LogWarn(fmt.Sprintf("Skipping Let's Encrypt for local/private domain '%s'; configuring plain HTTP.", domain))
		sslStrat.Mode = "none"
	}
	enableSSL := (sslStrat.Mode == "certbot" || sslStrat.Mode == "custom")

	if system.IsServiceInstalled() {
		system.StopService("apache2")
		system.StopService("httpd")
		time.Sleep(500 * time.Millisecond)
	}
	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}
	if enableSSL {
		if _, ok := system.ResolvePortConflict("443", "tcp", false); !ok {
			return
		}
	}

	// Mutual conflict check: if Docker Apache container is active
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-apache").Output(); err == nil && strings.TrimSpace(string(out)) == "running" {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "CONTAINER CONFLICT DETECTED" + core.NC,
			"",
			"  Docker APACHE Container 'krotek-apache' is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Native APACHE, the Docker container stack must be stopped",
			"  to free network ports.",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop Docker APACHE container stack to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Container Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Docker APACHE container and continue Native deployment?")
		if res != 0 {
			logger.LogInfo("Native installation cancelled by user due to container conflict.")
			return
		}

		if !system.RunStepOrAbort("Stopping conflicting Docker Apache container", "docker", "stop", "krotek-apache") {
			return
		}
	}

	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing APACHE Native Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)

	srvName := "apache2"
	switch core.SysInfo.Family {
	case "alpine":
		srvName = "apache2"
		if !system.RunStepOrAbort("Updating apk repositories", "apk", "update") {
			return
		}
		if !system.RunStepOrAbort("Installing Apache packages", "apk", "add", "apache2", "apache2-openrc", "curl") {
			return
		}
		if wantPHP {
			phpPrefix := system.DetectLatestAlpinePHP()
			if !system.RunStepOrAbort("Installing PHP & modules", "apk", "add", phpPrefix, phpPrefix+"-apache2", phpPrefix+"-fpm") {
				return
			}
		}
	case "debian":
		srvName = "apache2"
		if !system.RunStepOrAbort("Updating apt package lists", "apt-get", "update", "-y") {
			return
		}
		if !system.RunStepOrAbort("Installing Apache packages", // Dpkg options preserve existing configuration and prevent blocking terminal prompts in TUI & automation
			"apt-get", "install", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "apache2", "apache2-utils", "curl") {
			return
		}
		if wantPHP {
			if !system.RunStepOrAbort("Installing PHP & Apache module", // Dpkg options preserve existing configuration and prevent blocking terminal prompts in TUI & automation
				"apt-get", "install", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "php", "libapache2-mod-php", "php-cli", "php-common") {
				return
			}
		}
		if sslStrat.Mode == "certbot" {
			if !system.RunStepOrAbort("Installing Certbot SSL client", // Dpkg options preserve existing configuration and prevent blocking terminal prompts in TUI & automation
				"apt-get", "install", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "certbot", "python3-certbot-apache") {
				return
			}
		}
	case "rhel":
		srvName = "httpd"
		_ = system.RunStep("Refreshing DNF package cache", "dnf", "makecache")
		if !system.RunStepOrAbort("Installing Apache (httpd) packages", "dnf", "install", "-y", "httpd", "httpd-tools", "curl") {
			return
		}
		if wantPHP {
			if !system.RunStepOrAbort("Installing PHP & modules", "dnf", "install", "-y", "php", "php-cli", "php-common") {
				return
			}
		}
		if sslStrat.Mode == "certbot" {
			_ = exec.Command("dnf", "install", "-y", "epel-release").Run()
			if !system.RunStepOrAbort("Installing Certbot SSL client", "dnf", "install", "-y", "certbot", "python3-certbot-apache", "mod_ssl") {
				return
			}
		}
	}

	// Universal firewall hardening (UFW, Firewalld, raw iptables/ip6tables, and rules persistence)
	system.ConfigureFirewallRules(port, "80", "443")

	// Baseline configuration
	modules.EnsureValidMainConfig()

	if core.SysInfo.Family == "debian" {
		_ = exec.Command("a2enmod", "rewrite", "headers", "proxy", "proxy_http").Run()
		if enableSSL {
			_ = exec.Command("a2enmod", "ssl").Run()
		}
		if sslStrat.Mode == "cloudflare" {
			_ = exec.Command("a2enmod", "remoteip").Run()
		}
	}

	// Configure Listen port if custom
	if port != "80" {
		if core.SysInfo.Family == "debian" {
			if _, err := os.Lstat("/etc/apache2/ports.conf"); os.IsNotExist(err) || core.OverwriteExistingConfig {
				_ = system.WriteFileNoFollow("/etc/apache2/ports.conf", []byte(fmt.Sprintf("Listen %s\n\n<IfModule ssl_module>\n\tListen 443\n</IfModule>\n\n<IfModule mod_gnutls.c>\n\tListen 443\n</IfModule>\n", port)), 0644, "")
			}
		}
	}

	webUser := "apache"
	if core.SysInfo.Family == "debian" {
		webUser = "www-data"
	}
	activeWebDir := webDir
	if core.SysInfo.Family == "alpine" {
		activeWebDir = "/var/www/localhost/htdocs"
	}
	landingHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
		ServiceName:  "Apache HTTP Server",
		LogoText:     "A",
		HeroTitle:    "Apache HTTP Server Active",
		HeroDesc:     "Your robust, production-ready Apache web server is fully operational and tuned for production.",
		EnvType:      "native",
		Port:         port,
		WebDir:       activeWebDir,
		WebUser:      webUser,
		ManagerCmd:   "apache-manager",
		PHPInstalled: &wantPHP,
	})

	docRoots := []string{
		webDir,
		"/var/www/localhost/htdocs",
		"/var/www/htdocs",
	}
	for _, d := range docRoots {
		if system.IsSymlinkPath(d) {
			continue
		}
		_ = os.MkdirAll(d, 0755)
		system.SecureWebRootPermissions(d, webUser)
		idxFile := filepath.Join(d, "index.html")
		existingContent, readErr := os.ReadFile(idxFile)
		isDistroDefault := readErr == nil && (strings.Contains(string(existingContent), "It works!") ||
			strings.Contains(string(existingContent), "Apache2 Debian Default Page") ||
			strings.Contains(string(existingContent), "Apache2 Ubuntu Default Page") ||
			strings.Contains(string(existingContent), "Fedora Webserver"))
		if os.IsNotExist(readErr) || core.OverwriteExistingConfig || isDistroDefault {
			_ = system.WriteFileNoFollow(idxFile, []byte(landingHTML), 0644, webUser)
		}
		if wantPHP {
			probePath := filepath.Join(d, "php-status.php")
			_ = system.WriteFileNoFollow(probePath, []byte(system.GeneratePHPProbe()), 0644, webUser)
		}
	}
	modules.EnsureApacheWorking()

	// Sanitize domain input (strip http(s)://, trailing slashes, ports)
	if domain != "" {
		domain = system.CleanDomain(domain)
	}

	// VirtualHost configuration
	serverName := "localhost"
	if domain != "" {
		serverName = domain
	}

	if core.SysInfo.Family == "debian" {
		// Domain-specific vhost file; default block stays clean with ServerName _
		if domain != "" {
			vhostContent := fmt.Sprintf(`<VirtualHost *:%s>
    ServerName %s
    ServerAlias www.%s
    ServerAdmin webmaster@%s
    DocumentRoot %s

    <Directory %s>
        Options -Indexes +FollowSymLinks
        AllowOverride All
        Require all granted
    </Directory>

    ErrorLog ${APACHE_LOG_DIR}/%s-error.log
    CustomLog ${APACHE_LOG_DIR}/%s-access.log combined
</VirtualHost>
`, port, serverName, serverName, serverName, webDir, webDir, serverName, serverName)
			siteFile := fmt.Sprintf("/etc/apache2/sites-available/%s.conf", domain)
			if _, err := os.Lstat(siteFile); os.IsNotExist(err) || core.OverwriteExistingConfig {
				if err2 := system.WriteFileNoFollow(siteFile, []byte(vhostContent), 0644, ""); err2 == nil {
					logger.LogSuccess("Generated domain virtual host at " + siteFile)
				}
			}
			// Symlink into sites-enabled (remove stale link first)
			enabledLink := fmt.Sprintf("/etc/apache2/sites-enabled/%s.conf", domain)
			_ = os.Remove(enabledLink)
			_ = exec.Command("a2ensite", fmt.Sprintf("%s.conf", domain)).Run()
		} else {
			// Local mode: write default vhost
			vhostContent := fmt.Sprintf(`<VirtualHost *:%s>
    ServerName localhost
    ServerAdmin webmaster@localhost
    DocumentRoot %s

    <Directory %s>
        Options -Indexes +FollowSymLinks
        AllowOverride All
        Require all granted
    </Directory>

    ErrorLog ${APACHE_LOG_DIR}/error.log
    CustomLog ${APACHE_LOG_DIR}/access.log combined
</VirtualHost>
`, port, webDir, webDir)
			siteFile := "/etc/apache2/sites-available/000-default.conf"
			if _, err := os.Lstat(siteFile); os.IsNotExist(err) || core.OverwriteExistingConfig {
				_ = system.WriteFileNoFollow(siteFile, []byte(vhostContent), 0644, "")
			}
		}
	}

	// Syntax test before enabling/starting
	var syntaxCheck *exec.Cmd
	if core.SysInfo.Family == "debian" {
		syntaxCheck = exec.Command("apache2ctl", "-t")
	} else {
		syntaxCheck = exec.Command("httpd", "-t")
	}
	if syntaxCheck.Run() != nil {
		modules.EnsureValidMainConfig()
	}

	if !system.StartService(srvName) {
		logger.LogWarn("Initial Apache service start was delayed, attempting restart...")
		_ = system.RestartService(srvName)
	}

	// SSL automated provisioning
	sslFailed := false
	var sslErrLines []string
	if sslStrat.Mode == "certbot" && domain != "" {
		logger.LogInfo("Provisioning Let's Encrypt SSL certificate for " + domain + "...")
		certArgs := []string{
			"--apache",
			"-d", domain,
			"--non-interactive",
			"--agree-tos",
			"-m", sslStrat.Email,
			"--redirect",
		}
		if out, err := exec.Command("certbot", certArgs...).CombinedOutput(); err != nil {
			sslFailed = true
			enableSSL = false
			sslStrat.Mode = "none"
			logger.LogWarn(fmt.Sprintf("Certbot SSL acquisition failed for '%s'. Ensure DNS A record points to this server IP.\nOutput: %s", domain, strings.TrimSpace(string(out))))
			for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				cleanL := strings.TrimSpace(core.SanitizeTerminalText(l))
				if cleanL != "" {
					sslErrLines = append(sslErrLines, cleanL)
					if len(sslErrLines) >= 4 {
						break
					}
				}
			}
		} else {
			logger.LogSuccess("Let's Encrypt SSL certificate acquired successfully.")
		}
		system.ReloadService(srvName)
	} else if sslStrat.Mode == "cloudflare" {
		system.ConfigureCloudflareRealIP()
		system.ReloadService(srvName)
	}

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "native",
		"ACCESS_MODE": accessMode,
		"PORT":        port,
		"DOMAIN_NAME": domain,
		"WEB_DIR":     webDir,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	localIP := system.GetLocalIP()
	if localIP == "" {
		localIP = "127.0.0.1"
	}

	summaryLines := []string{
		tui.FormatKV("Deployment Mode", "Native Linux Stack", core.GREEN),
		tui.FormatKV("Local LAN Access URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN),
		tui.FormatKV("Localhost Access URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN),
	}
	if domain != "" {
		if enableSSL {
			summaryLines = append(summaryLines, tui.FormatKV("Public Web Domain URL", "https://"+domain, core.GREEN))
		} else {
			summaryLines = append(summaryLines, tui.FormatKV("Public Web Domain URL", fmt.Sprintf("http://%s:%s", domain, port), core.YELLOW))
		}
		if sslFailed {
			summaryLines = append(summaryLines, tui.FormatKV("SSL/TLS Mode", "NONE (Certbot Failed)", core.RED))
			summaryLines = append(summaryLines, core.YELLOW+"  SSL Notice: Let's Encrypt failed (check DNS A record & port 80); HTTP remains active."+core.NC)
			for _, el := range sslErrLines {
				summaryLines = append(summaryLines, "  "+core.RED+core.Sym.Pointer+" "+el+core.NC)
			}
			summaryLines = append(summaryLines, tui.FormatKV("Retry SSL", "sudo certbot --apache -d "+domain, core.GREEN))
		} else {
			summaryLines = append(summaryLines, tui.FormatKV("SSL/TLS Mode", strings.ToUpper(sslStrat.Mode), core.CYAN))
		}
	}
	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Web Root Directory", webDir, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "apache-manager", core.GREEN))

	tui.ClearScreen()
	tui.PrintInBox("APACHE Native Stack Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// ==============================================================================
// DOCKER COMPOSE EXECUTOR
// ==============================================================================
func ExecuteDockerInstall(accessMode, port, domain, composeDir string) {
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing APACHE (Docker Compose) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)

	if !system.EnsureDockerEngine() {
		return
	}

	// Mutual conflict check: if Native Apache service is running
	srvName := "apache2"
	if core.SysInfo.Family != "debian" {
		srvName = "httpd"
	}
	if exec.Command("systemctl", "is-active", "--quiet", srvName).Run() == nil {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "SERVICE CONFLICT DETECTED" + core.NC,
			"",
			"  Native APACHE Linux Service is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Docker APACHE, the Native service must be stopped & disabled",
			"  to free network ports.",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop & Disable Native APACHE service to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Service Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Native APACHE service and continue Docker deployment?")
		if res != 0 {
			logger.LogInfo("Docker installation cancelled by user due to service conflict.")
			return
		}

		system.StopService(srvName)
		_ = exec.Command("systemctl", "disable", srvName).Run()
	}

	if composeDir == "" {
		composeDir = "/etc/apache-docker"
	}
	htmlDir := filepath.Join(composeDir, "html")
	_ = os.MkdirAll(htmlDir, 0755)

	composeContent := fmt.Sprintf(`version: '3.8'

services:
  apache:
    image: httpd:alpine
    container_name: krotek-apache
    restart: always
    ports:
      - "%s:80"
    volumes:
      - ./html:/usr/local/apache2/htdocs/
`, port)

	_ = system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeContent), 0600, "")
	landingHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
		ServiceName: "Apache HTTP Server",
		LogoText:    "A",
		HeroTitle:   "Apache HTTP Server Active",
		HeroDesc:    "Your containerized Apache web server is fully operational and healthy.",
		EnvType:     "docker",
		Port:        port,
		WebDir:      filepath.Join(composeDir, "html"),
		ConfigPath:  filepath.Join(composeDir, "docker-compose.yml"),
		WebUser:     "daemon (container)",
		ManagerCmd:  "apache-manager",
	})
	_ = system.WriteFileNoFollow(filepath.Join(htmlDir, "index.html"), []byte(landingHTML), 0600, "")

	if !system.RunDockerCompose(composeDir, "Deploying Apache container stack", true, "up", "-d") {
		return
	}

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "docker",
		"ACCESS_MODE": accessMode,
		"PORT":        port,
		"DOMAIN_NAME": domain,
		"COMPOSE_DIR": composeDir,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	localIP := system.GetLocalIP()
	if localIP == "" {
		localIP = "127.0.0.1"
	}

	summaryLines := []string{
		tui.FormatKV("Deployment Mode", "Docker Container Stack", core.GREEN),
		tui.FormatKV("Local LAN Access URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN),
		tui.FormatKV("Localhost Access URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN),
	}
	if domain != "" {
		summaryLines = append(summaryLines, tui.FormatKV("Public Web Domain URL", fmt.Sprintf("http://%s:%s", domain, port), core.YELLOW))
	}
	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Docker Compose Dir", composeDir, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Web HTML Directory", htmlDir, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "apache-manager", core.GREEN))

	tui.ClearScreen()
	tui.PrintInBox("APACHE Docker Stack Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// PromptExistingConfigAction checks if prior configuration files exist on the host system.
// In interactive TUI mode, it prompts the administrator in English to decide whether to preserve existing custom settings
// or perform a clean factory default reset. In unattended mode (-y / CI/CD), it automatically preserves settings (safe default).
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
