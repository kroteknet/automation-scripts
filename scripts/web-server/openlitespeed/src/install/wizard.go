// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: OpenLiteSpeed High-Performance Web Server (openlitespeed)
//
// This package orchestrates automated pre-flight checks, package repository setup,
// configuration templating, service enablement (systemd/openrc), and firewall rules.
// Supports both interactive TUI step-by-step guidance and headless automation.
// ==============================================================================

package install

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"krotek-openlitespeed-installer/core"
	"krotek-openlitespeed-installer/logger"
	"krotek-openlitespeed-installer/modules"
	"krotek-openlitespeed-installer/system"
	"krotek-openlitespeed-installer/tui"
)

func GenerateRandomPassword(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%"
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			logger.LogError("Cryptographic randomness failure: unable to generate secure admin password")
			os.Exit(1)
			return ""
		}
		result[i] = chars[n.Int64()]
	}
	return string(result)
}

// FreshInstallWizard handles the end-to-end installation pipeline for OpenLiteSpeed High-Performance Web Server.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	accessMode := "local"
	envType := targetEnv
	if envType == "" {
		envType = "native"
	}

	if envType == "native" && core.SysInfo.Family == "alpine" {
		envType = "docker"
		tui.ClearScreen()
		alpineNotice := []string{
			core.BOLD + core.YELLOW + "ALPINE LINUX COMPATIBILITY NOTICE" + core.NC,
			"",
			"  OpenLiteSpeed does not provide official native apk packages",
			"  for Alpine Linux (due to musl libc standard library).",
			"",
			"  " + core.GREEN + "Automatic Resolution:" + core.NC,
			"  Deployment target has been switched to the official containerized",
			"  OpenLiteSpeed Docker Stack for maximum performance and stability.",
		}
		tui.PrintInBox("Alpine Linux Notice", alpineNotice)
	}

	webPort := "8088"
	adminPort := "7080"
	adminUser := "admin"
	adminPass := ""
	domainName := ""
	enableSSL := false
	sslEmail := ""
	composeDir := "/etc/openlitespeed-docker"

	// ─── UNATTENDED / NON-INTERACTIVE AUTOMATION MODE ─────────────────────────
	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" || os.Getenv("NON_INTERACTIVE") == "1" {
		if envPort := os.Getenv("KROTEK_PORT"); envPort != "" {
			webPort = envPort
		}
		if envAdminPort := os.Getenv("KROTEK_ADMIN_PORT"); envAdminPort != "" {
			adminPort = envAdminPort
		}
		if envPass := os.Getenv("KROTEK_ADMIN_PASS"); envPass != "" {
			adminPass = envPass
		} else if adminPass == "" {
			adminPass = GenerateRandomPassword(14)
		}
		if envDomain := os.Getenv("KROTEK_DOMAIN"); envDomain != "" {
			domainName = envDomain
			accessMode = "public"
		}
		if envType == "docker" {
			ExecuteDockerInstall(accessMode, webPort, adminPort, adminUser, adminPass, domainName, composeDir)
		} else {
			ExecuteNativeInstall(accessMode, webPort, adminPort, adminUser, adminPass, domainName, enableSSL, sslEmail)
		}
		return
	}

	PromptExistingConfigAction("OpenLiteSpeed Web Server", "/usr/local/lsws/conf/httpd_config.conf")

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/4: Choose Access Scope (Native OpenLiteSpeed)"
			if envType == "docker" {
				wizardTitle = "Step 1/4: Choose Access Scope (Docker OpenLiteSpeed)"
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
			} else {
				accessMode = "public"
			}
			currentStep = 2

		case 2:
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "OpenLiteSpeed Network Ports Configuration" + core.NC,
				"",
				"  Specify the HTTP Web Port and WebAdmin Console Port.",
				"  " + core.BOLD + "Default Web Port      :" + core.NC + " " + core.CYAN + "8088" + core.NC + " (or 80)",
				"  " + core.BOLD + "Default WebAdmin Port :" + core.NC + " " + core.PURPLE + "7080" + core.NC + " (Admin Web GUI)",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: Network Ports Setup", cardLines)

			portIn, st := tui.ReadTextInput("HTTP Web Port", webPort)
			if st != 0 {
				currentStep = 1
				continue
			}
			if resolvedPort, ok := system.ResolvePortConflict(portIn, "tcp", true); !ok {
				currentStep = 1
				continue
			} else {
				webPort = resolvedPort
			}

			adminPortIn, st2 := tui.ReadTextInput("WebAdmin Console Port", adminPort)
			if st2 != 0 {
				currentStep = 1
				continue
			}
			if resolvedAdminPort, ok := system.ResolvePortConflict(adminPortIn, "tcp", true); !ok {
				currentStep = 1
				continue
			} else {
				adminPort = resolvedAdminPort
			}

			currentStep = 3

		case 3:
			tui.ClearScreen()
			credCardLines := []string{
				core.BOLD + core.CYAN + "OpenLiteSpeed WebAdmin Console Credentials" + core.NC,
				"",
				"  Specify the WebAdmin administrator username and secure password.",
				"  " + core.BOLD + "Default Username :" + core.NC + " " + core.YELLOW + "admin" + core.NC,
				"  " + core.BOLD + "Password         :" + core.NC + " " + core.GREEN + "(Auto-generated if left blank)" + core.NC,
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: Admin Credentials & Domain", credCardLines)

			uIn, st := tui.ReadTextInput("Admin Username", adminUser)
			if st != 0 {
				currentStep = 2
				continue
			}
			if uIn != "" {
				adminUser = uIn
			}

			pIn, st2 := tui.ReadPasswordInput("Admin Password (blank for random)")
			if st2 != 0 {
				currentStep = 2
				continue
			}
			if pIn != "" {
				adminPass = pIn
			} else if adminPass == "" {
				adminPass = GenerateRandomPassword(14)
			}

			if accessMode == "public" {
				domIn, st3 := tui.ReadTextInput("Domain Name (FQDN)", domainName)
				if st3 != 0 {
					currentStep = 2
					continue
				}
				domainName = system.CleanDomain(domIn)
				domainName = system.CleanDomain(domIn)

				res := tui.AskYesNo("Enable automated Let's Encrypt SSL certificate (HTTPS)?")
				if res == 2 {
					currentStep = 2
					continue
				}
				if res == 0 {
					enableSSL = true
					defaultSSLEmail := sslEmail
					if defaultSSLEmail == "" {
						defaultSSLEmail = "admin@" + domainName
					}
					emailIn, st4 := tui.ReadTextInput("SSL Admin Email", defaultSSLEmail)
					if st4 != 0 {
						currentStep = 2
						continue
					}
					sslEmail = emailIn
				} else {
					enableSSL = false
					sslEmail = ""
				}
			} else {
				domainName = ""
				enableSSL = false
			}

			currentStep = 4

		case 4:
			tui.ClearScreen()
			summaryLines := []string{
				tui.FormatKV("Deployment Target", func() string {
					if envType == "docker" {
						return "Docker Compose Container Stack"
					}
					return "Native Linux Service (lsws)"
				}(), core.GREEN),
				tui.FormatKV("Access Scope", func() string {
					if accessMode == "public" {
						return "Public Internet (Domain + WAN)"
					}
					return "Local Network Only (LAN)"
				}(), core.GREEN),
				tui.FormatKV("HTTP Web Port", webPort, core.CYAN),
				tui.FormatKV("WebAdmin Port", adminPort, core.PURPLE+core.BOLD),
				tui.FormatKV("WebAdmin User", adminUser, core.YELLOW+core.BOLD),
				tui.FormatKV("WebAdmin Password", adminPass, core.GREEN+core.BOLD),
			}

			if domainName != "" {
				summaryLines = append(summaryLines, tui.FormatKV("Public Domain Name", domainName, core.YELLOW))
				if enableSSL {
					summaryLines = append(summaryLines, tui.FormatKV("Let's Encrypt SSL", fmt.Sprintf("[ENABLED] (%s)", sslEmail), core.GREEN))
				}
			}

			if envType == "docker" {
				summaryLines = append(summaryLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
			} else {
				summaryLines = append(summaryLines, tui.FormatKV("Install Directory", "/usr/local/lsws", core.CYAN))
			}

			summaryLines = append(summaryLines, tui.FormatKV("Automated Firewall", "[ENABLED] (Optimal Ports Opened)", core.CYAN))
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

			if envType == "docker" {
				ExecuteDockerInstall(accessMode, webPort, adminPort, adminUser, adminPass, domainName, composeDir)
			} else {
				ExecuteNativeInstall(accessMode, webPort, adminPort, adminUser, adminPass, domainName, enableSSL, sslEmail)
			}
			return
		}
	}
}

// ==============================================================================
// NATIVE INSTALLATION EXECUTOR
// ==============================================================================
func ExecuteNativeInstall(accessMode, webPort, adminPort, adminUser, adminPass, domain string, enableSSL bool, sslEmail string) {
	if core.SysInfo.Family == "alpine" {
		tui.PrintInBox("OPENLITESPEED Native" + " " + core.Sym.EmDash + " " + "Alpine Linux Not Supported", []string{
			core.BOLD + core.RED + "Native OPENLITESPEED service is not supported on Alpine Linux." + core.NC,
			"Alpine Linux lacks required systemd/glibc dependencies.",
			"",
			core.BOLD + core.GREEN + "Recommendation:" + core.NC + " Use the Docker container deployment mode instead.",
		})
		logger.LogError("Native OPENLITESPEED is unsupported on Alpine Linux. Aborting.")
		return
	}
	if resolvedPort, ok := system.ResolvePortConflict(webPort, "tcp", true); !ok {
		return
	} else {
		webPort = resolvedPort
	}
	if resolvedAdminPort, ok := system.ResolvePortConflict(adminPort, "tcp", true); !ok {
		return
	} else {
		adminPort = resolvedAdminPort
	}

	// Mutual conflict check: if Docker OpenLiteSpeed container is running
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-openlitespeed").Output(); err == nil && strings.TrimSpace(string(out)) == "running" {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "CONTAINER CONFLICT DETECTED" + core.NC,
			"",
			"  Docker OpenLiteSpeed Container 'krotek-openlitespeed' is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Native OpenLiteSpeed, the Docker container stack must be stopped",
			"  to free network ports.",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop Docker OpenLiteSpeed container stack to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Container Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Docker OpenLiteSpeed container and continue Native deployment?")
		if res != 0 {
			logger.LogInfo("Native installation cancelled by user due to container conflict.")
			return
		}

		if !system.RunStepOrAbort("Stopping conflicting Docker OpenLiteSpeed container", "docker", "stop", "krotek-openlitespeed") {
			return
		}
	}

	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing OpenLiteSpeed Web Server Native Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native OpenLiteSpeed installation...")

	// Sanitize domain input (strip http(s)://, trailing slashes, ports)
	if domain != "" {
		domain = system.CleanDomain(domain)
	}

	if core.SysInfo.Family == "debian" {
		if !system.RunStepOrAbort("Updating apt package lists", "apt-get", "update", "-y") {
			return
		}
		if !system.RunStepOrAbort("Installing repository setup dependencies", // Dpkg options preserve existing configuration and prevent blocking terminal prompts in TUI & automation
			"apt-get", "install", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "wget", "curl", "ca-certificates") {
			return
		}
		repoCmd := "TMP_D=$(mktemp -d /tmp/krotek-ols-XXXXXX) && chmod 700 \"$TMP_D\" && (curl --proto '=https' --tlsv1.2 -fsSL https://repo.litespeed.sh -o \"$TMP_D/repo.sh\" || curl --proto '=https' --tlsv1.2 -fsSL https://rpms.litespeedtech.com/debian/enable_lst_debian_repo.sh -o \"$TMP_D/repo.sh\") && bash \"$TMP_D/repo.sh\"; rm -rf \"$TMP_D\"; sed -i 's/trixie/bookworm/g' /etc/apt/sources.list.d/lst_debian_repo.list 2>/dev/null || true; apt-get update -y -qq"
		if !system.RunMultiStepOrAbort("Configuring LiteSpeed official repository", repoCmd) {
			return
		}
		if !system.RunStep("Installing OpenLiteSpeed package", // Dpkg options preserve existing configuration and prevent blocking terminal prompts in TUI & automation
			"apt-get", "install", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "openlitespeed") {
			logger.LogWarn("Native package installation failed. Swapping to OpenLiteSpeed Docker Stack...")
			_ = os.Remove("/etc/apt/sources.list.d/lst_debian_repo.list")
			_ = exec.Command("apt-get", "update", "-qq").Run()
			time.Sleep(2 * time.Second)
			ExecuteDockerInstall(accessMode, webPort, adminPort, adminUser, adminPass, domain, "/etc/openlitespeed-docker")
			return
		}
	} else if core.SysInfo.Family == "rhel" {
		_ = system.RunStep("Refreshing DNF package cache", "dnf", "makecache")
		if !system.RunStepOrAbort("Installing prerequisites", "dnf", "install", "-y", "wget", "curl") {
			return
		}
		if !system.RunMultiStepOrAbort("Configuring LiteSpeed official repository", "TMP_D=$(mktemp -d /tmp/krotek-ols-XXXXXX) && chmod 700 \"$TMP_D\" && curl --proto '=https' --tlsv1.2 -fsSL https://repo.litespeed.sh -o \"$TMP_D/repo.sh\" && bash \"$TMP_D/repo.sh\"; RET=$?; rm -rf \"$TMP_D\"; exit $RET") {
			return
		}
		if !system.RunStep("Installing OpenLiteSpeed package", "dnf", "install", "-y", "openlitespeed") {
			logger.LogWarn("Native package installation failed on RHEL. Swapping to Docker Stack...")
			time.Sleep(2 * time.Second)
			ExecuteDockerInstall(accessMode, webPort, adminPort, adminUser, adminPass, domain, "/etc/openlitespeed-docker")
			return
		}
	} else if core.SysInfo.Family == "alpine" {
		logger.LogWarn("OpenLiteSpeed does not provide official native packages for Alpine Linux (musl libc).")
		logger.LogInfo("Automatically deploying high-performance OpenLiteSpeed Docker Stack...")
		time.Sleep(2 * time.Second)
		ExecuteDockerInstall(accessMode, webPort, adminPort, adminUser, adminPass, domain, "/etc/openlitespeed-docker")
		return
	}

	// Universal firewall hardening (UFW, Firewalld, raw iptables/ip6tables, rules persistence)
	system.ConfigureFirewallRules(webPort, adminPort, "80", "443")

	_ = system.RunMultiStep("Starting OpenLiteSpeed service", "systemctl daemon-reload 2>/dev/null || true; systemctl restart lsws 2>/dev/null || systemctl restart openlitespeed 2>/dev/null || /usr/local/lsws/bin/lswsctrl start 2>/dev/null || true")
	_ = system.RunMultiStep("Enabling OpenLiteSpeed on boot", "systemctl enable lsws 2>/dev/null || systemctl enable openlitespeed 2>/dev/null || true")

	// Set WebAdmin Admin User & Password via admpass.sh stdin (never expose password in argv)
	admpassProc := exec.Command("/usr/local/lsws/admin/misc/admpass.sh")
	admpassProc.Stdin = strings.NewReader(adminUser + "\n" + adminPass + "\n" + adminPass + "\n")
	_ = admpassProc.Run()

	// Deploy unified landing page
	landingHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
		ServiceName: "OpenLiteSpeed Web Server",
		LogoText:    "OLS",
		HeroTitle:   "OpenLiteSpeed Event Engine Active",
		HeroDesc:    "Your high-velocity, event-driven HTTP/3 & QUIC web server with LiteSpeed Cache is fully operational.",
		EnvType:     "native",
		Port:        webPort,
		AdminPort:   adminPort,
		WebDir:      "/usr/local/lsws/Example/html",
		ConfigPath:  "/usr/local/lsws/conf/httpd_config.conf",
		WebUser:     "nobody / lsadm",
		ManagerCmd:  "openlitespeed-manager",
	})
	_ = os.MkdirAll("/usr/local/lsws/Example/html", 0755)
	_ = system.WriteFileNoFollow("/usr/local/lsws/Example/html/index.html", []byte(landingHTML), 0644, "")
	_ = os.MkdirAll("/var/www/html", 0755)
	_ = system.WriteFileNoFollow("/var/www/html/index.html", []byte(landingHTML), 0644, "")
	modules.EnsureOpenLiteSpeedWorking()

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "native",
		"ACCESS_MODE": accessMode,
		"PORT":        webPort,
		"ADMIN_PORT":  adminPort,
		"ADMIN_USER":  adminUser,
		"ADMIN_PASS":  adminPass,
		"DOMAIN_NAME": domain,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	localIP := system.GetLocalIP()
	if localIP == "" {
		localIP = "127.0.0.1"
	}
	pubIP := system.GetPublicIP()
	if pubIP == "" {
		pubIP = localIP
	}

	var finalLines []string
	finalLines = append(finalLines, tui.FormatKV("Deployment Mode", "Native Linux Service (lsws)", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, webPort), core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Localhost Web URL", fmt.Sprintf("http://localhost:%s", webPort), core.GREEN))
	if domain != "" {
		finalLines = append(finalLines, tui.FormatKV("Domain Web URL", fmt.Sprintf("http://%s:%s", domain, webPort), core.GREEN))
	}
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"OPENLITESPEED ACCESS & SECURED CREDENTIALS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("WebAdmin Console", fmt.Sprintf("https://%s:%s", pubIP, adminPort), core.PURPLE+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("WebAdmin Username", adminUser, core.YELLOW+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("WebAdmin Password", adminPass, core.GREEN+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("Main Config File", "/usr/local/lsws/conf/httpd_config.conf", core.CYAN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("OLS Root Directory", "/usr/local/lsws", core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Management Command", "openlitespeed-manager", core.GREEN+core.BOLD))

	tui.ClearScreen()
	tui.PrintInBox("OpenLiteSpeed Native Stack Installed Successfully", finalLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// ==============================================================================
// DOCKER COMPOSE EXECUTOR
// ==============================================================================
func ExecuteDockerInstall(accessMode, webPort, adminPort, adminUser, adminPass, domain, composeDir string) {
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing OPENLITESPEED (Docker Compose) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)

	// Sanitize domain input (strip http(s)://, trailing slashes, ports)
	if domain != "" {
		domain = system.CleanDomain(domain)
	}

	if !system.EnsureDockerEngine() {
		return
	}

	if resolvedPort, ok := system.ResolvePortConflict(webPort, "tcp", true); !ok {
		return
	} else {
		webPort = resolvedPort
	}
	if resolvedAdminPort, ok := system.ResolvePortConflict(adminPort, "tcp", true); !ok {
		return
	} else {
		adminPort = resolvedAdminPort
	}

	// Clean up any stale conflicting container
	_ = exec.Command("docker", "rm", "-f", "krotek-openlitespeed").Run()

	// Mutual conflict check: if native lsws service is running
	if exec.Command("systemctl", "is-active", "--quiet", "lsws").Run() == nil || exec.Command("systemctl", "is-active", "--quiet", "openlitespeed").Run() == nil {
		if !core.AutoYes && os.Getenv("KROTEK_NON_INTERACTIVE") != "1" {
			tui.ClearScreen()
			conflictCard := []string{
				core.BOLD + core.YELLOW + "SERVICE CONFLICT DETECTED" + core.NC,
				"",
				"  Native OpenLiteSpeed Service is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
				"  To deploy Docker OpenLiteSpeed, the Native service must be stopped & disabled",
				"  to free network ports.",
				"",
				core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop & Disable Native OpenLiteSpeed service?",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Service Conflict Notice", conflictCard)

			res := tui.AskYesNo("Stop Native OpenLiteSpeed service and continue Docker deployment?")
			if res != 0 {
				logger.LogInfo("Docker installation cancelled by user due to service conflict.")
				return
			}
		}

		_ = exec.Command("systemctl", "stop", "lsws").Run()
		_ = exec.Command("systemctl", "stop", "openlitespeed").Run()
		_ = exec.Command("systemctl", "disable", "lsws").Run()
		_ = exec.Command("pkill", "-9", "-x", "litespeed").Run()
	}

	if composeDir == "" {
		composeDir = "/etc/openlitespeed-docker"
	}
	_ = os.MkdirAll(composeDir, 0755)
	htmlDir := filepath.Join(composeDir, "html")
	_ = os.MkdirAll(htmlDir, 0755)

	landingHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
		ServiceName: "OpenLiteSpeed Web Server",
		LogoText:    "OLS",
		HeroTitle:   "OpenLiteSpeed Event Engine Active",
		HeroDesc:    "Your containerized OpenLiteSpeed web server is fully operational and healthy.",
		EnvType:     "docker",
		Port:        webPort,
		AdminPort:   adminPort,
		WebDir:      htmlDir,
		ConfigPath:  filepath.Join(composeDir, "docker-compose.yml"),
		WebUser:     "nobody (container)",
		ManagerCmd:  "openlitespeed-manager",
	})
	_ = system.WriteFileNoFollow(filepath.Join(htmlDir, "index.html"), []byte(landingHTML), 0644, "")

	// Build port mappings without duplicate host port collisions
	portList := []string{
		fmt.Sprintf(`      - "%s:80"`, webPort),
		fmt.Sprintf(`      - "%s:7080"`, adminPort),
	}
	chk443 := system.CheckPortConflict("443", "tcp")
	if !chk443.InUse {
		portList = append(portList, `      - "443:443"`, `      - "443:443/udp"`)
	}
	portsBlock := strings.Join(portList, "\n")

	composeYaml := fmt.Sprintf(`version: '3.8'
services:
  openlitespeed:
    image: litespeedtech/openlitespeed:latest
    container_name: krotek-openlitespeed
    restart: always
    environment:
      - OLS_PASSWORD=%s
    ports:
%s
    volumes:
      - ./html:/var/www/vhosts/localhost/html
`, adminPass, portsBlock)

	_ = system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, "")

	// Kernel network preparation for Docker container bridge
	_ = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
	_ = exec.Command("modprobe", "br_netfilter").Run()
	_ = exec.Command("modprobe", "ip_tables").Run()
	_ = exec.Command("modprobe", "iptable_nat").Run()

	if !system.RunDockerCompose(composeDir, "Deploying OPENLITESPEED Docker Stack", false, "up", "-d") {
		logger.LogWarn("Container network setup failed. Resetting Docker daemon network state...")
		_ = exec.Command("docker", "rm", "-f", "krotek-openlitespeed").Run()
		_ = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
		_ = exec.Command("rc-service", "docker", "restart").Run()
		_ = exec.Command("systemctl", "restart", "docker").Run()
		time.Sleep(3 * time.Second)

		if !system.RunDockerCompose(composeDir, "Deploying OPENLITESPEED Docker Stack (Retry)", true, "up", "-d") {
			return
		}
	}

	time.Sleep(2 * time.Second)
	modules.EnsureOpenLiteSpeedWorking()

	// Universal firewall hardening (UFW, Firewalld, raw iptables/ip6tables, rules persistence)
	system.ConfigureFirewallRules(webPort, adminPort, "80", "443")

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "docker",
		"ACCESS_MODE": accessMode,
		"PORT":        webPort,
		"ADMIN_PORT":  adminPort,
		"ADMIN_USER":  adminUser,
		"ADMIN_PASS":  adminPass,
		"DOMAIN_NAME": domain,
		"COMPOSE_DIR": composeDir,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	localIP := system.GetLocalIP()
	if localIP == "" {
		localIP = "127.0.0.1"
	}
	pubIP := system.GetPublicIP()
	if pubIP == "" {
		pubIP = localIP
	}

	var finalLines []string
	finalLines = append(finalLines, tui.FormatKV("Deployment Mode", "Docker Container Stack", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, webPort), core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Localhost Web URL", fmt.Sprintf("http://localhost:%s", webPort), core.GREEN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"OPENLITESPEED ACCESS & SECURED CREDENTIALS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("WebAdmin Console", fmt.Sprintf("https://%s:%s", pubIP, adminPort), core.PURPLE+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("WebAdmin Username", adminUser, core.YELLOW+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("WebAdmin Password", adminPass, core.GREEN+core.BOLD))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Management Command", "openlitespeed-manager", core.GREEN+core.BOLD))

	tui.ClearScreen()
	tui.PrintInBox("OpenLiteSpeed Docker Stack Installed Successfully", finalLines)

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
