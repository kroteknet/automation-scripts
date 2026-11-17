// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: OpenResty High-Performance Web & Dynamic Lua Platform (openresty)
// File:    web-server/openresty/src/install/wizard.go
//
// Orchestrates automated pre-flight checks, package repository setup across
// Debian/Ubuntu, RHEL/AlmaLinux/Rocky/Fedora, and Alpine Linux, configuration
// templating with LuaJIT directives, service enablement (systemd/OpenRC/WSL),
// firewall configuration, and Docker Compose deployment.
// ==============================================================================

package install

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"krotek-openresty-installer/core"
	"krotek-openresty-installer/logger"
	"krotek-openresty-installer/modules"
	"krotek-openresty-installer/system"
	"krotek-openresty-installer/tui"
)

// FreshInstallWizard handles the end-to-end installation pipeline for OpenResty.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_DOMAIN, KROTEK_PHP, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	accessMode := "local"
	envType := targetEnv // "native" or "docker"
	port := "80"
	domainName := ""
	sslStrat := system.SSLStrategy{Mode: "none"}
	wantPHP := false
	ncDir := "/var/www/html"
	composeDir := "/etc/openresty-docker"

	// ─── UNATTENDED / NON-INTERACTIVE AUTOMATION MODE ─────────────────────────
	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" || os.Getenv("NON_INTERACTIVE") == "1" {
		if envPort := os.Getenv("KROTEK_PORT"); envPort != "" {
			port = envPort
		}
		if envDomain := strings.TrimSpace(os.Getenv("KROTEK_DOMAIN")); envDomain != "" {
			cleanDom := system.CleanDomain(envDomain)
			if !system.ValidateDomainName(cleanDom) {
				logger.LogError(fmt.Sprintf("Invalid KROTEK_DOMAIN value '%s'; aborting installation.", envDomain))
				system.HandleInstallFailure("Validating KROTEK_DOMAIN (" + envDomain + ")")
				return
			}
			domainName = cleanDom
			accessMode = "public"
			ncDir = "/var/www/" + domainName
			sslStrat = system.ParseSSLStrategyFromEnv(domainName)
		}
		if envWebDir := strings.TrimSpace(os.Getenv("KROTEK_WEB_DIR")); envWebDir != "" {
			if err := system.ValidateSafeDirectoryPath(envWebDir); err != nil || system.IsSymlinkPath(envWebDir) {
				logger.LogError(fmt.Sprintf("Unsafe KROTEK_WEB_DIR value '%s'; aborting installation.", envWebDir))
				system.HandleInstallFailure("Validating KROTEK_WEB_DIR (" + envWebDir + ")")
				return
			}
			ncDir = filepath.Clean(envWebDir)
		}
		if os.Getenv("KROTEK_PHP") == "1" || os.Getenv("KROTEK_PHP") == "true" {
			wantPHP = true
		}
		if envMode := strings.ToLower(strings.TrimSpace(os.Getenv("KROTEK_MODE"))); envMode != "" {
			if envMode != "native" && envMode != "docker" {
				logger.LogError(fmt.Sprintf("Invalid KROTEK_MODE value '%s'; aborting installation.", envMode))
				system.HandleInstallFailure("Validating KROTEK_MODE (" + envMode + ")")
				return
			}
			envType = envMode
		}

		logger.LogInfo(fmt.Sprintf("Starting Unattended OpenResty installation: mode=%s, port=%s, domain=%s, ssl_mode=%s, php=%t", envType, port, domainName, sslStrat.Mode, wantPHP))
		modules.CheckConflicts()
		if envType == "native" {
			ExecuteNativeInstall(accessMode, port, domainName, sslStrat, wantPHP, ncDir)
		} else {
			ExecuteDockerInstall(accessMode, port, domainName, composeDir)
		}
		return
	}

	PromptExistingConfigAction("OpenResty Web Platform", system.GetOpenRestyMainConfigPath())

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/4: Choose Access Scope (Native OpenResty)"
			if envType == "docker" {
				wizardTitle = "Step 1/4: Choose Access Scope (Docker OpenResty)"
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
					"  Specify the OpenResty listening web port for LAN / Wi-Fi access.",
					"  " + core.BOLD + "Default Port:" + core.NC + " " + core.CYAN + "80" + core.NC,
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: LAN Network Options", cardLines)

				portIn, st := tui.ReadTextInput("OpenResty Port", port)
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

				portIn, st := tui.ReadTextInput("OpenResty Web Port", port)
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
					"  Recommended if you plan to host dynamic PHP sites alongside OpenResty/Lua.",
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
				webUser := system.GetRuntimeWebUser()
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
				dirIn = strings.TrimSpace(dirIn)
				if err := system.ValidateSafeDirectoryPath(dirIn); err != nil || system.IsSymlinkPath(dirIn) {
					errMsg := "Web root directory must not be a symbolic link."
					if err != nil {
						errMsg = err.Error()
					}
					tui.PrintInBox("Invalid Web Root Directory", []string{
						core.RED + errMsg + core.NC,
					})
					currentStep = 3
					continue
				}
				ncDir = filepath.Clean(dirIn)
			} else if envType == "docker" {
				tui.ClearScreen()
				composeCardLines := []string{
					core.BOLD + core.CYAN + "Docker Compose Directory Configuration" + core.NC,
					"",
					"  Specify the directory where docker-compose.yml, OpenResty configuration",
					"  files, HTML files, and persistent volumes will be stored.",
					"",
					"  " + core.BOLD + core.Sym.Pointer + " Default Directory :" + core.NC + " " + core.CYAN + "/etc/openresty-docker" + core.NC,
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: Docker Compose Directory Setup", composeCardLines)

				cDirIn, st := tui.ReadTextInput("Compose Directory", composeDir)
				if st != 0 {
					currentStep = 2
					continue
				}
				cDirIn = strings.TrimSpace(cDirIn)
				if err := system.ValidateSafeComposeDir(cDirIn); err != nil {
					tui.PrintInBox("Invalid Compose Directory", []string{
						core.RED + err.Error() + core.NC,
					})
					currentStep = 3
					continue
				}
				composeDir = filepath.Clean(cDirIn)
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

func ExecuteNativeInstall(accessMode string, port string, domain string, sslStrat system.SSLStrategy, wantPHP bool, webDir string) {
	if sslStrat.Mode == "certbot" && domain != "" && system.IsLocalOrPrivateDomain(domain) {
		logger.LogWarn(fmt.Sprintf("Skipping Let's Encrypt for local/private domain '%s'; configuring plain HTTP.", domain))
		sslStrat.Mode = "none"
	}
	if strings.TrimSpace(domain) != "" {
		cleanedDom := system.CleanDomain(domain)
		if !system.ValidateDomainName(cleanedDom) {
			logger.LogError(fmt.Sprintf("Invalid domain '%s' passed to ExecuteNativeInstall.", domain))
			system.HandleInstallFailure("Validating domain (" + domain + ")")
			return
		}
		domain = cleanedDom
	}
	if strings.TrimSpace(webDir) != "" {
		if err := system.ValidateSafeDirectoryPath(webDir); err != nil || system.IsSymlinkPath(webDir) {
			logger.LogError(fmt.Sprintf("Unsafe web root directory '%s' passed to ExecuteNativeInstall.", webDir))
			system.HandleInstallFailure("Validating Web Root Directory (" + webDir + ")")
			return
		}
		webDir = filepath.Clean(webDir)
	}

	if system.IsOpenRestyInstalled() && system.IsOpenRestyRunning() {
		system.StopService("openresty")
		time.Sleep(400 * time.Millisecond)
	}

	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
			system.HandleInstallFailure("Resolving HTTP port conflict (" + port + ")")
		}
		return
	} else {
		port = resolvedPort
	}
	if sslStrat.Mode == "certbot" || sslStrat.Mode == "custom" {
		if _, ok := system.ResolvePortConflict("443", "tcp", false); !ok {
			if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
				system.HandleInstallFailure("Resolving HTTPS port conflict (443)")
			}
			return
		}
	}
	if system.IsOpenRestyDockerRunning() {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "CONTAINER CONFLICT DETECTED" + core.NC,
			"",
			"  Docker OpenResty Container 'krotek-openresty' is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Native OpenResty, the Docker container stack must be stopped",
			"  to free network ports.",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop Docker OpenResty container stack to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Container Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Docker OpenResty container and continue Native deployment?")
		if res != 0 {
			logger.LogInfo("Native installation cancelled by user due to container conflict.")
			return
		}

		if !system.RunStepOrAbort("Stopping conflicting Docker OpenResty container", "docker", "stop", "krotek-openresty") {
			return
		}
	}

	// Capture existing configuration presence BEFORE OS packages write stock distro defaults.
	preMainConf := system.GetOpenRestyMainConfigPath()
	_, preMainErr := os.Stat(preMainConf)
	hadMainConfBefore := preMainErr == nil

	preDefaultVhost := system.GetOpenRestyDefaultVHostPath()
	_, preVhostErr := os.Stat(preDefaultVhost)
	hadDefaultVhostBefore := preVhostErr == nil

	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing OpenResty Native Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)

	switch core.SysInfo.Family {
	case "alpine":
		if !system.SetupOpenRestyRepository() {
			system.HandleInstallFailure("Configuring Alpine APK Community Repository")
			return
		}
		if !system.RunStepOrAbort("Installing OpenResty packages", "apk", "add", "openresty", "openresty-openrc", "curl", "bash") {
			return
		}
		if wantPHP {
			if _, ok := system.InstallPHPStack(); !ok {
				system.HandleInstallFailure("Installing PHP-FPM stack")
				return
			}
		}
	case "debian":
		if !system.SetupOpenRestyRepository() {
			system.HandleInstallFailure("Configuring Official OpenResty APT Repository")
			return
		}
		aptArgs := append([]string{"install", "-y"}, system.GetDpkgConfigFlags()...)
		aptArgs = append(aptArgs, "openresty", "openresty-resty", "openresty-opm", "curl")
		if !system.RunStepOrAbort("Installing OpenResty packages", "apt-get", aptArgs...) {
			return
		}
		if wantPHP {
			if _, ok := system.InstallPHPStack(); !ok {
				system.HandleInstallFailure("Installing PHP-FPM stack")
				return
			}
		}
		if sslStrat.Mode == "certbot" {
			if !system.RunStepOrAbort("Installing Certbot SSL client", "apt-get", "install", "-y", "certbot", "python3-certbot-nginx") {
				return
			}
		}
	case "rhel":
		if !system.SetupOpenRestyRepository() {
			system.HandleInstallFailure("Configuring Official OpenResty RPM Repository")
			return
		}
		if !system.RunRHELStepOrAbort("Installing OpenResty packages", "install", "-y", "openresty", "openresty-resty", "openresty-opm", "curl") {
			return
		}
		if wantPHP {
			if _, ok := system.InstallPHPStack(); !ok {
				system.HandleInstallFailure("Installing PHP-FPM stack")
				return
			}
		}
		if sslStrat.Mode == "certbot" {
			_, _, _ = system.ExecRHELCmd("", false, false, "install", "-y", "epel-release")
			if !system.RunRHELStepOrAbort("Installing Certbot SSL client", "install", "-y", "certbot", "python3-certbot-nginx") {
				return
			}
		}
	}

	system.EnsureOpenRestyBinarySymlinks()
	webUser := system.GetRuntimeWebUser()
	system.EnsureOpenRestyRuntimeDirs(webUser)

	// Start PHP-FPM before generating VirtualHost FastCGI blocks so socket path discovery is accurate
	if wantPHP {
		tui.StartSpinner("Optimizing PHP & OPcache Settings")
		system.OptimizePHPSettings()
		time.Sleep(150 * time.Millisecond)
		tui.StopSpinner(0)
		system.StartPHPFPMService()
	}

	// Soft firewall configuration (UFW, Firewalld, and Oracle Cloud iptables)
	system.ConfigureFirewallRules(port)
	if sslStrat.Mode == "certbot" || sslStrat.Mode == "custom" {
		system.ConfigureFirewallRules("443")
	}

	ApplyDefaultOpenRestyConfig(hadMainConfBefore)

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
	mainConfPath := system.GetOpenRestyMainConfigPath()

	// 1. Prepare default web directory (/var/www/html)
	_ = os.MkdirAll(defaultWebDir, 0755)
	system.SecureWebRootPermissions(defaultWebDir, webUser)

	defaultLandingHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
		ServiceName:  "OpenResty Web Platform",
		LogoText:     "OR",
		HeroTitle:    "OpenResty Lua Platform Active",
		HeroDesc:     "Your high-performance dynamic web platform integrating enhanced Nginx core, LuaJIT, and OPM ecosystem is operational.",
		EnvType:      "native",
		Port:         port,
		WebDir:       defaultWebDir,
		ConfigPath:   mainConfPath,
		WebUser:      webUser,
		ManagerCmd:   "openresty-manager",
		PHPInstalled: &wantPHP,
	})
	defaultIndexFile := filepath.Join(defaultWebDir, "index.html")
	existingIdxBytes, idxReadErr := os.ReadFile(defaultIndexFile)
	isDefaultPlaceholder := idxReadErr != nil ||
		strings.Contains(string(existingIdxBytes), "Welcome to OpenResty!") ||
		strings.Contains(string(existingIdxBytes), "Welcome to nginx!") ||
		strings.Contains(string(existingIdxBytes), "If you see this page, the nginx web server")
	if core.OverwriteExistingConfig || isDefaultPlaceholder {
		_ = os.Remove(filepath.Join(defaultWebDir, "index.nginx-debian.html"))
		_ = system.WriteFileNoFollow(defaultIndexFile, []byte(defaultLandingHTML), 0644, webUser)
	} else {
		logger.LogInfo("Preserving existing " + defaultIndexFile)
	}

	// 2. Prepare domain web directory if configured (/var/www/<domain>)
	if cleanDomain != "" {
		if err := system.ValidateSafeDirectoryPath(domainWebDir); err != nil || system.IsSymlinkPath(domainWebDir) {
			logger.LogError(fmt.Sprintf("Unsafe domain web root '%s'.", domainWebDir))
			system.HandleInstallFailure("Validating Domain Web Root (" + domainWebDir + ")")
			return
		}
		_ = os.MkdirAll(domainWebDir, 0755)
		system.SecureWebRootPermissions(domainWebDir, webUser)

		domainLandingHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
			ServiceName:  "OpenResty Web Platform",
			LogoText:     "OR",
			HeroTitle:    cleanDomain + " Active",
			HeroDesc:     "Production-grade OpenResty LuaJIT & HTTP/2 web server is operational for " + cleanDomain + ".",
			EnvType:      "native",
			Port:         port,
			WebDir:       domainWebDir,
			ConfigPath:   mainConfPath,
			WebUser:      webUser,
			ManagerCmd:   "openresty-manager",
			PHPInstalled: &wantPHP,
		})
		domainIndexFile := filepath.Join(domainWebDir, "index.html")
		if core.OverwriteExistingConfig || func() bool { _, err := os.Stat(domainIndexFile); return os.IsNotExist(err) }() {
			_ = system.WriteFileNoFollow(domainIndexFile, []byte(domainLandingHTML), 0644, webUser)
		} else {
			logger.LogInfo("Preserving existing " + domainIndexFile)
		}
	}

	defaultConf := system.GetOpenRestyDefaultVHostPath()
	availDir, enabledDir := system.GetOpenRestyVHostDirs()

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
<head><meta charset="UTF-8"><title>PHP Active &bull; OpenResty</title>
<style>body{font-family:system-ui,sans-serif;text-align:center;padding:50px;background:#0b0f17;color:#f8fafc;}h1{color:#38bdf8;}.badge{display:inline-block;padding:6px 14px;background:rgba(56,189,248,0.15);border:1px solid rgba(56,189,248,0.3);color:#38bdf8;border-radius:9999px;margin:10px;font-weight:600;}</style>
</head>
<body><h1>OpenResty Engine Active</h1><div class="badge">PHP <?php echo htmlspecialchars(phpversion(), ENT_QUOTES, 'UTF-8'); ?> Active</div><p style="color:#94a3b8;">Managed by OpenResty Toolkit.</p></body>
</html>`
			phpIndexPath := filepath.Join(td, "index.php")
			if core.OverwriteExistingConfig || func() bool { _, err := os.Stat(phpIndexPath); return os.IsNotExist(err) }() {
				_ = system.WriteFileNoFollow(phpIndexPath, []byte(phpSafeFile), 0644, webUser)
			} else {
				logger.LogInfo("Preserving existing " + phpIndexPath)
			}
		}
	}

	listenDefault := system.ListenDirectives(port, true, "")
	listenDomain := system.ListenDirectives(port, false, "")
	listenHTTPS := system.ListenDirectives("443", false, "ssl http2")

	vhostContent := fmt.Sprintf(`server {
%s    server_name _;

    root %s;
    index %s;

    location / {
        try_files $uri $uri/ /index.html =404;
    }
%s}
`, listenDefault, defaultWebDir, indexFiles, phpLocationBlock)

	shouldWriteDefaultVhost := !hadDefaultVhostBefore || core.OverwriteExistingConfig
	if !shouldWriteDefaultVhost {
		// Replace Alpine/distro stock 404 stub even if it existed with generic default content
		if vData, err := os.ReadFile(defaultConf); err == nil {
			vStr := string(vData)
			if strings.Contains(vStr, "return 404;") && strings.Contains(vStr, "default_server") {
				shouldWriteDefaultVhost = true
			}
		}
	}
	if !shouldWriteDefaultVhost {
		logger.LogInfo(fmt.Sprintf("Preserving existing default virtual host at %s", defaultConf))
	}

	if shouldWriteDefaultVhost {
		_ = os.MkdirAll(availDir, 0755)
		_ = os.MkdirAll(enabledDir, 0755)
		if core.SysInfo.Family == "alpine" {
			_ = os.Remove("/etc/nginx/conf.d/default.conf")
			_ = os.Remove("/etc/nginx/http.d/default.conf")
		}
		_ = system.WriteFileNoFollow(defaultConf, []byte(vhostContent), 0644, "")
		if core.SysInfo.Family == "debian" {
			enabledDefault := filepath.Join(enabledDir, "default")
			_ = os.Remove(enabledDefault)
			_ = os.Symlink(defaultConf, enabledDefault)
		}
	}

	if cleanDomain != "" {
		domainConf, enabledLink := system.GetOpenRestyDomainVHostPaths(cleanDomain)
		shouldWriteDomainConf := true
		if !core.OverwriteExistingConfig {
			if _, err := os.Stat(domainConf); err == nil {
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
%s    server_name %s www.%s;
    return 301 https://$host$request_uri;
}

server {
%s    server_name %s www.%s;

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
`, listenDomain, cleanDomain, cleanDomain, listenHTTPS, cleanDomain, cleanDomain, domainWebDir, domainIndexFiles, system.GetCustomSSLBlock(sslStrat.CertPath, sslStrat.KeyPath), domainPhpBlock)

			case "cloudflare":
				domainVhostContent = fmt.Sprintf(`server {
%s    server_name %s www.%s;

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
`, listenDomain, cleanDomain, cleanDomain, domainWebDir, domainIndexFiles, system.GetCloudflareRealIPBlock(), domainPhpBlock)

			default:
				domainVhostContent = fmt.Sprintf(`server {
%s    server_name %s www.%s;

    root %s;
    index %s;

    # Gzip Compression
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;

    location / {
        try_files $uri $uri/ =404;
    }
%s}
`, listenDomain, cleanDomain, cleanDomain, domainWebDir, domainIndexFiles, domainPhpBlock)
			}

			_ = os.MkdirAll(filepath.Dir(domainConf), 0755)
			if err := system.WriteFileNoFollow(domainConf, []byte(domainVhostContent), 0644, ""); err != nil {
				logger.LogError(fmt.Sprintf("Failed to write domain configuration to %s: %v", domainConf, err))
			} else {
				logger.LogSuccess(fmt.Sprintf("Generated domain virtual host at %s", domainConf))
			}

			if enabledLink != "" {
				_ = os.MkdirAll(filepath.Dir(enabledLink), 0755)
				_ = os.Remove(enabledLink)
				if err := os.Symlink(domainConf, enabledLink); err != nil {
					logger.LogError(fmt.Sprintf("Failed to symlink %s -> %s: %v", domainConf, enabledLink, err))
				} else {
					logger.LogSuccess(fmt.Sprintf("Enabled domain virtual host: %s", enabledLink))
				}
			}
		}
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

	bin := system.GetOpenRestyBinPath()
	if bin == "" {
		bin = "openresty"
	}
	if !system.RunStepOrAbort("Testing OpenResty configuration syntax", bin, "-t") {
		return
	}

	tui.StartSpinner("Enabling & Starting OpenResty Service")
	started := system.StartService("openresty")
	if started {
		tui.StopSpinner(0)
	} else {
		tui.StopSpinner(1)
		logger.LogError(fmt.Sprintf("Failed to start OpenResty service. Details: %s", core.LastServiceError))
		system.HandleInstallFailure("Enabling & Starting OpenResty Service")
		return
	}

	modules.EnsureOpenRestyWorking()
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
		certArgs := []string{"--nginx", "-d", cleanDomain, "--non-interactive", "--agree-tos", "-m", certEmail, "--redirect"}
		confDir := system.GetOpenRestyConfDir()
		if confDir != "/etc/nginx" {
			certArgs = append(certArgs, "--nginx-server-root", confDir, "--nginx-ctl", bin)
		}
		if !system.RunStep("Obtaining Let's Encrypt SSL Certificate", "certbot", certArgs...) {
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
	summaryLines = append(summaryLines, tui.FormatKV("Main Configuration", mainConfPath, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Web Root Directory", targetWebDir, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "openresty-manager", core.GREEN))

	tui.ClearScreen()
	tui.PrintInBox("OpenResty Native Stack Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// ==============================================================================
// DOCKER COMPOSE EXECUTOR
// ==============================================================================

func isHostPortBound(port string) bool {
	pTrim := strings.TrimSpace(port)
	if pNum, err := strconv.Atoi(pTrim); err != nil || pNum < 1 || pNum > 65535 {
		return false
	}
	suffix := ":" + pTrim
	var out []byte
	var err error
	if system.CommandExists("ss") {
		out, err = exec.Command("ss", "-tuln").Output()
	} else if system.CommandExists("netstat") {
		out, err = exec.Command("netstat", "-tuln").Output()
	}
	if err == nil && len(out) > 0 {
		for _, field := range strings.Fields(string(out)) {
			if strings.HasSuffix(field, suffix) {
				return true
			}
		}
	}
	return false
}

func ExecuteDockerInstall(accessMode, port, domain, composeDir string) {
	if strings.TrimSpace(domain) != "" {
		cleanedDom := system.CleanDomain(domain)
		if !system.ValidateDomainName(cleanedDom) {
			logger.LogError(fmt.Sprintf("Invalid domain '%s' passed to ExecuteDockerInstall.", domain))
			system.HandleInstallFailure("Validating domain (" + domain + ")")
			return
		}
		domain = cleanedDom
	}
	if strings.TrimSpace(composeDir) == "" {
		composeDir = "/etc/openresty-docker"
	}
	composeDir = filepath.Clean(strings.TrimSpace(composeDir))
	if err := system.ValidateSafeComposeDir(composeDir); err != nil {
		logger.LogError(fmt.Sprintf("Unsafe Docker Compose directory '%s': %v", composeDir, err))
		system.HandleInstallFailure("Validating Docker Compose Directory (" + composeDir + ")")
		return
	}

	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing OpenResty (Docker Compose) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)

	if !system.EnsureDockerEngine() {
		system.HandleInstallFailure("Ensuring Docker Engine & Docker Compose")
		return
	}

	// Only stop native OpenResty if OpenResty itself is running (never host vanilla nginx)
	if system.IsOpenRestyRunning() && !system.IsOpenRestyDockerRunning() {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "SERVICE CONFLICT DETECTED" + core.NC,
			"",
			"  Native OpenResty Linux Service is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Docker OpenResty, the Native service must be stopped & disabled",
			"  to free network ports.",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop & Disable Native OpenResty service to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Service Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Native OpenResty service and continue Docker deployment?")
		if res != 0 {
			logger.LogInfo("Docker installation cancelled by user due to service conflict.")
			return
		}

		system.StopService("openresty")
		_ = exec.Command("systemctl", "disable", "openresty").Run()
		_ = exec.Command("rc-update", "del", "openresty", "default").Run()
		_ = exec.Command("update-rc.d", "openresty", "disable").Run()
	}

	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
			system.HandleInstallFailure("Resolving Docker HTTP port conflict (" + port + ")")
		}
		return
	} else {
		port = resolvedPort
	}

	if composeDir == "" {
		composeDir = "/etc/openresty-docker"
	}
	confDir := filepath.Join(composeDir, "conf.d")
	htmlDir := filepath.Join(composeDir, "html")
	_ = os.MkdirAll(confDir, 0755)
	_ = os.MkdirAll(htmlDir, 0755)

	portsSection := fmt.Sprintf("      - \"%s:80\"", port)
	if !isHostPortBound("443") {
		portsSection += "\n      - \"443:443\""
	}

	composeContent := fmt.Sprintf(`services:
  openresty:
    image: openresty/openresty:alpine
    container_name: krotek-openresty
    restart: unless-stopped
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
    ports:
%s
    volumes:
      - ./html:/usr/local/openresty/nginx/html
      - ./conf.d:/etc/nginx/conf.d
    healthcheck:
      test: ["CMD-SHELL", "wget -q --spider http://127.0.0.1/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`, portsSection)

	_ = system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeContent), 0600, "")

	defaultConfTpl := fmt.Sprintf(`server {
%s    server_name _;

    root /usr/local/openresty/nginx/html;
    index index.html index.htm;

    location / {
        try_files $uri $uri/ =404;
    }
}
`, system.ListenDirectives("80", true, ""))
	_ = system.WriteFileNoFollow(filepath.Join(confDir, "default.conf"), []byte(defaultConfTpl), 0644, "")

	cleanDomain := system.CleanDomain(domain)
	targetDockerWebDir := htmlDir
	if cleanDomain != "" {
		targetDockerWebDir = filepath.Join(htmlDir, cleanDomain)
		_ = os.MkdirAll(targetDockerWebDir, 0755)

		dockerDomainConf := filepath.Join(confDir, cleanDomain+".conf")
		shouldWriteDomainConf := true
		if !core.OverwriteExistingConfig {
			if _, err := os.Stat(dockerDomainConf); err == nil {
				shouldWriteDomainConf = false
				logger.LogInfo(fmt.Sprintf("Preserving existing Docker domain virtual host at %s", dockerDomainConf))
			}
		}
		if shouldWriteDomainConf {
			domainConfTpl := fmt.Sprintf(`server {
    listen 80;
    server_name %s www.%s;

    root /usr/local/openresty/nginx/html/%s;
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
				ServiceName: "OpenResty Web Platform",
				LogoText:    "OR",
				HeroTitle:   cleanDomain + " Active",
				HeroDesc:    "Your containerized OpenResty Web Platform stack is running and ready to serve traffic for " + cleanDomain + ".",
				EnvType:     "docker",
				Port:        port,
				WebDir:      targetDockerWebDir,
				ConfigPath:  dockerDomainConf,
				WebUser:     "nobody (container)",
				ManagerCmd:  "openresty-manager",
			})
			domainIndexFile := filepath.Join(targetDockerWebDir, "index.html")
			if core.OverwriteExistingConfig || func() bool { _, err := os.Stat(domainIndexFile); return os.IsNotExist(err) }() {
				_ = system.WriteFileNoFollow(domainIndexFile, []byte(domainLandingHTML), 0644, "")
			}
		}
	}

	dockerLandingHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
		ServiceName: "OpenResty Web Platform",
		LogoText:    "OR",
		HeroTitle:   "OpenResty Docker Stack Active",
		HeroDesc:    "Your containerized OpenResty Web Platform stack is running and ready to serve traffic.",
		EnvType:     "docker",
		Port:        port,
		WebDir:      composeDir + "/html",
		ConfigPath:  composeDir + "/conf.d/default.conf",
		WebUser:     "nobody (container)",
		ManagerCmd:  "openresty-manager",
	})
	_ = system.WriteFileNoFollow(filepath.Join(htmlDir, "index.html"), []byte(dockerLandingHTML), 0644, "")

	if !system.RunDockerCompose(composeDir, "Deploying OpenResty container stack", true, "up", "-d") {
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
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "openresty-manager", core.GREEN))

	tui.ClearScreen()
	tui.PrintInBox("OpenResty Docker Stack Started Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// ApplyDefaultOpenRestyConfig writes the primary OpenResty nginx.conf tailored for
// Debian/Ubuntu, RHEL/AlmaLinux/Rocky/Fedora, or Alpine Linux with full LuaJIT
// package path directives and vhost directory includes.
func ApplyDefaultOpenRestyConfig(hadMainConfBefore bool) {
	mainConf := system.GetOpenRestyMainConfigPath()
	confDir := system.GetOpenRestyConfDir()

	if hadMainConfBefore && !core.OverwriteExistingConfig {
		logger.LogInfo(fmt.Sprintf("Preserving existing %s configuration.", mainConf))
		return
	}

	webUser := system.GetRuntimeWebUser()
	system.EnsureOpenRestyRuntimeDirs(webUser)

	pidPath := system.GetOpenRestyPidPath()
	mimeTypesPath := system.GetOpenRestyMimeTypesPath()
	luaDirectives := system.GetLuaPackageDirectives()

	availDir, enabledDir := system.GetOpenRestyVHostDirs()
	_ = os.MkdirAll(filepath.Join(confDir, "conf.d"), 0755)
	_ = os.MkdirAll(availDir, 0755)
	_ = os.MkdirAll(enabledDir, 0755)

	var includeSection string
	switch core.SysInfo.Family {
	case "debian":
		includeSection = fmt.Sprintf("    include %s/conf.d/*.conf;\n    include %s/sites-enabled/*;", confDir, confDir)
	case "alpine":
		includeSection = "    include /etc/nginx/http.d/*.conf;"
	default:
		includeSection = fmt.Sprintf("    include %s/conf.d/*.conf;", confDir)
	}

	config := fmt.Sprintf(`# %s
# Configured by Krotek OpenResty Installer

user %s;
worker_processes auto;
pid %s;
error_log /var/log/openresty/error.log warn;

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

    include %s;
    default_type application/octet-stream;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers on;

    access_log /var/log/openresty/access.log;
    error_log /var/log/openresty/error.log warn;

    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;

%s
%s
}
`, mainConf, webUser, pidPath, mimeTypesPath, luaDirectives, includeSection)

	_ = os.MkdirAll(filepath.Dir(mainConf), 0755)
	_ = system.WriteFileNoFollow(mainConf, []byte(config), 0644, "")
}

// PromptExistingConfigAction checks if prior configuration files exist on the host system.
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
