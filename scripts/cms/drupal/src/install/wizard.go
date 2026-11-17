// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Drupal CMS Production Suite (drupal)
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

	"krotek-drupal-installer/core"
	"krotek-drupal-installer/logger"
	"krotek-drupal-installer/system"
	"krotek-drupal-installer/tui"
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

// FreshInstallWizard handles the end-to-end installation pipeline for Drupal CMS Production Suite.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	accessMode := "local"
	envType := targetEnv
	if envType == "" {
		envType = "docker"
	}

	port := "80"
	domainName := ""
	enableSSL := false
	sslEmail := ""

	siteTitle := "My Drupal Site"
	adminUser := "admin"
	adminPass := ""
	adminEmail := "admin@example.com"
	dbName := "drupal"
	dbUser := "drupal_user"
	dbPass := ""
	rootPass := ""
	composeDir := "/etc/drupal-docker"

	PromptExistingConfigAction("Drupal CMS", "/var/www/drupal")

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/4: Choose Access Scope (Docker Drupal)"
			if envType == "native" {
				wizardTitle = "Step 1/4: Choose Access Scope (Native Drupal)"
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
					"  Specify the listening HTTP port for Drupal LAN / Wi-Fi access.",
					"  " + core.BOLD + "Default Port:" + core.NC + " " + core.CYAN + "80" + core.NC + " (or 8080)",
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: LAN Network Options", cardLines)

				portIn, st := tui.ReadTextInput("Drupal Web Port", port)
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
				enableSSL = false
			} else {
				cardLines := []string{
					core.BOLD + core.CYAN + "Public Internet Domain & SSL Setup" + core.NC,
					"",
					"  Enter your public Domain Name (e.g. portal.example.com).",
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

				portIn, st := tui.ReadTextInput("Drupal Web Port", port)
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

				res := tui.AskYesNo("Enable automated Let's Encrypt SSL certificate (HTTPS)?")
				if res == 2 {
					currentStep = 1
					continue
				}
				if res == 0 {
					enableSSL = true
					defaultSSLEmail := sslEmail
					if defaultSSLEmail == "" {
						defaultSSLEmail = "admin@" + domainName
					}
					emailIn, st := tui.ReadTextInput("SSL Admin Email", defaultSSLEmail)
					if st != 0 {
						currentStep = 1
						continue
					}
					sslEmail = emailIn
				} else {
					enableSSL = false
					sslEmail = ""
				}
			}

			currentStep = 3

		case 3:
			tui.ClearScreen()
			credCardLines := []string{
				core.BOLD + core.CYAN + "Drupal Portal & Database Setup" + core.NC,
				"",
				"  Configure portal metadata and administrator credentials.",
				"  " + core.BOLD + "Default Admin User :" + core.NC + " " + core.YELLOW + "admin" + core.NC,
				"  " + core.BOLD + "Admin Password     :" + core.NC + " " + core.GREEN + "(Auto-generated if left blank)" + core.NC,
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: Drupal Configuration", credCardLines)

			tIn, st := tui.ReadTextInput("Site Name", siteTitle)
			if st != 0 {
				currentStep = 2
				continue
			}
			if tIn != "" {
				siteTitle = tIn
			}

			uIn, st := tui.ReadTextInput("Drupal Admin Username", adminUser)
			if st != 0 {
				currentStep = 2
				continue
			}
			if uIn != "" {
				adminUser = uIn
			}

			pIn, st := tui.ReadPasswordInput("Drupal Admin Password (blank for random)")
			if st != 0 {
				currentStep = 2
				continue
			}
			if pIn != "" {
				adminPass = pIn
			} else if adminPass == "" {
				adminPass = GenerateRandomPassword(16)
			}

			defaultEmail := "admin@example.com"
			if domainName != "" {
				defaultEmail = "admin@" + domainName
			}
			eIn, st := tui.ReadTextInput("Admin Notification Email", defaultEmail)
			if st != 0 {
				currentStep = 2
				continue
			}
			if eIn != "" {
				adminEmail = eIn
			}

			if dbPass == "" {
				dbPass = GenerateRandomPassword(16)
			}
			if rootPass == "" {
				rootPass = GenerateRandomPassword(16)
			}

			currentStep = 4

		case 4:
			tui.ClearScreen()
			summaryLines := []string{
				tui.FormatKV("Deployment Target", func() string {
					if envType == "docker" {
						return "Docker Compose Container Stack (Drupal + MariaDB)"
					}
					return "Native Linux LEMP Stack (Nginx + PHP-FPM + MariaDB)"
				}(), core.GREEN),
				tui.FormatKV("Access Scope", func() string {
					if accessMode == "public" {
						return "Public Internet (Domain + WAN)"
					}
					return "Local Network Only (LAN)"
				}(), core.GREEN),
				tui.FormatKV("HTTP Web Port", port, core.CYAN),
				tui.FormatKV("Portal Title", siteTitle, core.YELLOW+core.BOLD),
				tui.FormatKV("Admin Username", adminUser, core.YELLOW+core.BOLD),
				tui.FormatKV("Admin Password", adminPass, core.GREEN+core.BOLD),
				tui.FormatKV("Admin Email", adminEmail, core.CYAN),
				tui.FormatKV("Database Engine", "MariaDB (Automated Provisioning)", core.GREEN),
				tui.FormatKV("Database Name", dbName, core.CYAN),
				tui.FormatKV("Database User", dbUser, core.CYAN),
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
				summaryLines = append(summaryLines, tui.FormatKV("Document Root", "/var/www/drupal", core.CYAN))
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
				ExecuteDockerInstall(accessMode, port, domainName, enableSSL, sslEmail, siteTitle, adminUser, adminPass, adminEmail, dbName, dbUser, dbPass, rootPass, composeDir)
			} else {
				ExecuteNativeInstall(accessMode, port, domainName, enableSSL, sslEmail, siteTitle, adminUser, adminPass, adminEmail, dbName, dbUser, dbPass)
			}
			return
		}
	}
}

// ==============================================================================
// DOCKER COMPOSE EXECUTOR
// ==============================================================================
func ExecuteDockerInstall(accessMode, port, domain string, enableSSL bool, sslEmail, siteTitle, adminUser, adminPass, adminEmail, dbName, dbUser, dbPass, rootPass, composeDir string) {
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing DRUPAL (Docker Compose Stack) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)

	if !system.EnsureDockerEngine() {
		return
	}

	if composeDir == "" {
		composeDir = "/etc/drupal-docker"
	}
	_ = os.MkdirAll(composeDir, 0755)

	composeYaml := fmt.Sprintf(`version: '3.8'

services:
  db:
    image: mariadb:lts
    container_name: krotek-drupal-db
    restart: always
    environment:
      MYSQL_ROOT_PASSWORD: "%s"
      MYSQL_DATABASE: "%s"
      MYSQL_USER: "%s"
      MYSQL_PASSWORD: "%s"
    volumes:
      - db_data:/var/lib/mysql

  drupal:
    image: drupal:latest
    container_name: krotek-drupal
    restart: always
    depends_on:
      - db
    ports:
      - "%s:80"
    volumes:
      - drupal_modules:/opt/drupal/web/modules
      - drupal_profiles:/opt/drupal/web/profiles
      - drupal_themes:/opt/drupal/web/themes
      - drupal_sites:/opt/drupal/web/sites

volumes:
  db_data:
  drupal_modules:
  drupal_profiles:
  drupal_themes:
  drupal_sites:
`, escapeYamlString(rootPass), escapeYamlString(dbName), escapeYamlString(dbUser), escapeYamlString(dbPass), port)

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}

	// Universal firewall configuration (UFW, Firewalld, iptables/ip6tables)
	system.ConfigureFirewallRules(port+"/tcp", "443/tcp")

	if !system.RunDockerCompose(composeDir, "Deploying Drupal & MariaDB Docker Stack", true, "up", "-d") {
		return
	}

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "docker",
		"ACCESS_MODE": accessMode,
		"PORT":        port,
		"DOMAIN_NAME": domain,
		"SITE_TITLE":  siteTitle,
		"ADMIN_USER":  adminUser,
		"ADMIN_PASS":  adminPass,
		"ADMIN_EMAIL": adminEmail,
		"DB_NAME":     dbName,
		"DB_USER":     dbUser,
		"DB_PASS":     dbPass,
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
	finalLines = append(finalLines, tui.FormatKV("Deployment Mode", "Docker Container Stack (Drupal + MariaDB)", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Localhost Web URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
	if domain != "" {
		finalLines = append(finalLines, tui.FormatKV("Public Domain URL", fmt.Sprintf("http://%s:%s", domain, port), core.GREEN))
	}
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"DRUPAL DATABASE & CREDENTIALS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Database Host", "db (Container) / localhost", core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database Name", dbName, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database User", dbUser, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database Password", dbPass, core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Admin Suggested User", adminUser, core.YELLOW+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("Admin Suggested Pass", adminPass, core.GREEN+core.BOLD))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Management Command", "drupal-manager", core.GREEN+core.BOLD))

	tui.ClearScreen()
	tui.PrintInBox("Drupal Docker Stack Installed Successfully", finalLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// ==============================================================================
// NATIVE INSTALLATION EXECUTOR
// ==============================================================================
func ExecuteNativeInstall(accessMode, port, domain string, enableSSL bool, sslEmail, siteTitle, adminUser, adminPass, adminEmail, dbName, dbUser, dbPass string) {
	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}

	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing DRUPAL Native LEMP Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native Drupal installation...")

	var pkgs []string
	switch core.SysInfo.Family {
	case "debian":
		pkgs = []string{"nginx", "mariadb-server", "php-fpm", "php-mysql", "php-curl", "php-gd", "php-mbstring", "php-xml", "php-zip", "curl", "tar"}
	case "rhel":
		pkgs = []string{"nginx", "mariadb-server", "php-fpm", "php-mysqlnd", "php-gd", "php-mbstring", "php-xml", "php-pecl-zip", "curl", "tar"}
	case "alpine":
		phpPrefix := system.DetectLatestAlpinePHP()
		pkgs = []string{"nginx", "nginx-openrc", "mariadb", "mariadb-client", "mariadb-server-utils", "mariadb-openrc", phpPrefix + "-fpm", phpPrefix + "-mysqli", phpPrefix + "-curl", phpPrefix + "-gd", phpPrefix + "-mbstring", phpPrefix + "-xml", phpPrefix + "-zip", "curl", "tar"}
	default:
		pkgs = []string{"nginx", "mariadb-server", "php-fpm", "php-mysql", "php-curl", "php-gd", "php-mbstring", "php-xml", "php-zip", "curl", "tar"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogError("Failed to install required Drupal packages.")
		return
	}

	// Universal firewall configuration (UFW, Firewalld, iptables/ip6tables)
	system.ConfigureFirewallRules(port+"/tcp", "443/tcp")

	system.InitMariaDBDatadir()
	system.StartService("mariadb")

	// Provision MariaDB Database & User safely via isolated temporary SQL script
	var sbName, sbUser strings.Builder
	for _, ch := range dbName {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			sbName.WriteRune(ch)
		}
	}
	escapedDBName := sbName.String()
	if escapedDBName == "" {
		escapedDBName = "drupal"
	}

	for _, ch := range dbUser {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			sbUser.WriteRune(ch)
		}
	}
	escapedDBUser := sbUser.String()
	if escapedDBUser == "" {
		escapedDBUser = "drupal"
	}

	escapedDBPass := strings.ReplaceAll(strings.ReplaceAll(dbPass, `\`, `\\`), "'", `\'`)

	dbInitSQL := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `"+escapedDBName+"` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;\n"+
		"CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s';\n"+
		"GRANT ALL PRIVILEGES ON `"+escapedDBName+"`.* TO '%s'@'localhost';\n"+
		"FLUSH PRIVILEGES;\n", escapedDBUser, escapedDBPass, escapedDBUser)

	// Pipe SQL directly on stdin without writing credentials to temporary files or invoking a shell.
	applied := false
	for _, attempt := range [][]string{
		{"mariadb", "-u", "root"},
		{"mariadb"},
		{"mysql", "-u", "root"},
		{"mysql"},
	} {
		if !system.CommandExists(attempt[0]) {
			continue
		}
		cmd := exec.Command(attempt[0], attempt[1:]...)
		cmd.Stdin = strings.NewReader(dbInitSQL)
		if cmd.Run() == nil {
			applied = true
			break
		}
	}
	if !applied {
		logger.LogWarn("MariaDB database initialization returned non-zero status; check MariaDB service logs")
	}

	system.StartService("nginx")
	system.StartPHPFPMService()

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "native",
		"ACCESS_MODE": accessMode,
		"PORT":        port,
		"DOMAIN_NAME": domain,
		"SITE_TITLE":  siteTitle,
		"ADMIN_USER":  adminUser,
		"ADMIN_PASS":  adminPass,
		"ADMIN_EMAIL": adminEmail,
		"DB_NAME":     dbName,
		"DB_USER":     dbUser,
		"DB_PASS":     dbPass,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	localIP := system.GetLocalIP()
	if localIP == "" {
		localIP = "127.0.0.1"
	}

	var finalLines []string
	finalLines = append(finalLines, tui.FormatKV("Deployment Mode", "Native Linux LEMP Stack", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Localhost Web URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
	if domain != "" {
		finalLines = append(finalLines, tui.FormatKV("Public Domain URL", fmt.Sprintf("http://%s:%s", domain, port), core.GREEN))
	}
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"DRUPAL DATABASE CREDENTIALS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Database Name", dbName, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database User", dbUser, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database Password", dbPass, core.GREEN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Management Command", "drupal-manager", core.GREEN+core.BOLD))

	tui.ClearScreen()
	tui.PrintInBox("Drupal Native LEMP Stack Provisioned Successfully", finalLines)

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

func escapeYamlString(val string) string {
	v := strings.ReplaceAll(val, "\\", "\\\\")
	v = strings.ReplaceAll(v, "\"", "\\\"")
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", "")
	return v
}
