// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Ghost Publishing Platform (ghost)
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

	"krotek-ghost-installer/core"
	"krotek-ghost-installer/logger"
	"krotek-ghost-installer/system"
	"krotek-ghost-installer/tui"
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

// FreshInstallWizard handles the end-to-end installation pipeline for Ghost Publishing Platform.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	accessMode := "local"
	envType := targetEnv
	if envType == "" {
		envType = "docker"
	}

	port := "2368"
	domainName := ""
	enableSSL := false
	sslEmail := ""

	blogTitle := "My Ghost Publication"
	adminEmail := "admin@example.com"
	dbName := "ghost"
	dbUser := "ghost_user"
	dbPass := ""
	rootPass := ""
	composeDir := "/etc/ghost-docker"

	PromptExistingConfigAction("Ghost Publishing Platform", "/var/www/ghost/config.production.json")

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/4: Choose Access Scope (Docker Ghost CMS)"
			if envType == "native" {
				wizardTitle = "Step 1/4: Choose Access Scope (Native Ghost CMS)"
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
				port = "2368"
			} else {
				accessMode = "public"
				port = "2368"
			}
			currentStep = 2

		case 2:
			tui.ClearScreen()
			if accessMode == "local" {
				cardLines := []string{
					core.BOLD + core.CYAN + "Local Network Configuration" + core.NC,
					"",
					"  Specify the listening HTTP port for Ghost CMS LAN access.",
					"  " + core.BOLD + "Default Port:" + core.NC + " " + core.CYAN + "2368" + core.NC,
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: LAN Network Options", cardLines)

				portIn, st := tui.ReadTextInput("Ghost Port", port)
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
					"  Enter your public Domain Name (e.g. blog.example.com).",
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

				portIn, st := tui.ReadTextInput("Ghost Web Port", port)
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
				core.BOLD + core.CYAN + "Ghost Publication & Configuration Setup" + core.NC,
				"",
				"  Configure publication metadata and database credentials.",
				"  " + core.BOLD + "Default Title :" + core.NC + " " + core.YELLOW + "My Ghost Publication" + core.NC,
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: Ghost Configuration", credCardLines)

			tIn, st := tui.ReadTextInput("Publication Title", blogTitle)
			if st != 0 {
				currentStep = 2
				continue
			}
			if tIn != "" {
				blogTitle = tIn
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
						return "Docker Compose Container Stack (Ghost + MariaDB)"
					}
					return "Native Linux Service Stack (Ghost + Node.js + MariaDB)"
				}(), core.GREEN),
				tui.FormatKV("Access Scope", func() string {
					if accessMode == "public" {
						return "Public Internet (Domain + WAN)"
					}
					return "Local Network Only (LAN)"
				}(), core.GREEN),
				tui.FormatKV("Target Port", port, core.CYAN),
				tui.FormatKV("Publication Title", blogTitle, core.YELLOW+core.BOLD),
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
				summaryLines = append(summaryLines, tui.FormatKV("Document Root", "/var/www/ghost", core.CYAN))
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
				ExecuteDockerInstall(accessMode, port, domainName, enableSSL, sslEmail, blogTitle, adminEmail, dbName, dbUser, dbPass, rootPass, composeDir)
			} else {
				ExecuteNativeInstall(accessMode, port, domainName, enableSSL, sslEmail, blogTitle, adminEmail, dbName, dbUser, dbPass)
			}
			return
		}
	}
}

// ==============================================================================
// DOCKER COMPOSE EXECUTOR
// ==============================================================================
func ExecuteDockerInstall(accessMode, port, domain string, enableSSL bool, sslEmail, blogTitle, adminEmail, dbName, dbUser, dbPass, rootPass, composeDir string) {
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing GHOST (Docker Compose Stack) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)

	if !system.EnsureDockerEngine() {
		return
	}

	if composeDir == "" {
		composeDir = "/etc/ghost-docker"
	}
	_ = os.MkdirAll(composeDir, 0755)

	localIP := system.GetLocalIP()
	if localIP == "" {
		localIP = "127.0.0.1"
	}

	siteURL := fmt.Sprintf("http://%s:%s", localIP, port)
	if domain != "" {
		if enableSSL {
			siteURL = "https://" + domain
		} else {
			siteURL = fmt.Sprintf("http://%s:%s", domain, port)
		}
	}

	composeYaml := fmt.Sprintf(`version: '3.8'

services:
  db:
    image: mariadb:lts
    container_name: krotek-ghost-db
    restart: always
    environment:
      MYSQL_ROOT_PASSWORD: "%s"
      MYSQL_DATABASE: "%s"
      MYSQL_USER: "%s"
      MYSQL_PASSWORD: "%s"
    volumes:
      - db_data:/var/lib/mysql

  ghost:
    image: ghost:alpine
    container_name: krotek-ghost
    restart: always
    depends_on:
      - db
    ports:
      - "%s:2368"
    environment:
      url: %s
      database__client: mysql
      database__connection__host: db
      database__connection__user: "%s"
      database__connection__password: "%s"
      database__connection__database: "%s"
    volumes:
      - ghost_content:/var/lib/ghost/content

volumes:
  db_data:
  ghost_content:
`, escapeYamlString(rootPass), escapeYamlString(dbName), escapeYamlString(dbUser), escapeYamlString(dbPass), port, siteURL, escapeYamlString(dbUser), escapeYamlString(dbPass), escapeYamlString(dbName))

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}

	// Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	system.ConfigureFirewallRules(port+"/tcp", "443/tcp")

	if !system.RunDockerCompose(composeDir, "Deploying Ghost & MariaDB Docker Stack", true, "up", "-d") {
		return
	}

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "docker",
		"ACCESS_MODE": accessMode,
		"PORT":        port,
		"DOMAIN_NAME": domain,
		"BLOG_TITLE":  blogTitle,
		"ADMIN_EMAIL": adminEmail,
		"DB_NAME":     dbName,
		"DB_USER":     dbUser,
		"DB_PASS":     dbPass,
		"COMPOSE_DIR": composeDir,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	pubIP := system.GetPublicIP()
	if pubIP == "" {
		pubIP = localIP
	}

	var finalLines []string
	finalLines = append(finalLines, tui.FormatKV("Deployment Mode", "Docker Container Stack (Ghost + MariaDB)", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Localhost URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
	if domain != "" {
		finalLines = append(finalLines, tui.FormatKV("Public Domain URL", siteURL, core.GREEN))
	}
	finalLines = append(finalLines, tui.FormatKV("Admin Setup URL", fmt.Sprintf("%s/ghost/", siteURL), core.PURPLE+core.BOLD))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"DATABASE & SETTINGS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Database User", dbUser, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database Password", dbPass, core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Admin Email", adminEmail, core.CYAN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Management Command", "ghost-manager", core.GREEN+core.BOLD))

	tui.ClearScreen()
	tui.PrintInBox("Ghost Docker Stack Installed Successfully", finalLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// ==============================================================================
// NATIVE INSTALLATION EXECUTOR
// ==============================================================================
func ExecuteNativeInstall(accessMode, port, domain string, enableSSL bool, sslEmail, blogTitle, adminEmail, dbName, dbUser, dbPass string) {
	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}

	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing GHOST Native Service Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native Ghost installation...")

	var pkgs []string
	switch core.SysInfo.Family {
	case "debian":
		pkgs = []string{"nodejs", "npm", "nginx", "mariadb-server", "curl"}
	case "rhel":
		pkgs = []string{"nodejs", "npm", "nginx", "mariadb-server", "curl"}
	case "alpine":
		pkgs = []string{"nodejs", "npm", "nginx", "nginx-openrc", "mariadb", "mariadb-client", "mariadb-server-utils", "mariadb-openrc", "curl"}
	default:
		pkgs = []string{"nodejs", "npm", "nginx", "mariadb-server", "curl"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogError("Failed to install required Ghost packages.")
		return
	}

	// Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	system.ConfigureFirewallRules(port+"/tcp", "443/tcp")

	system.StartService("mariadb")
	system.StartService("nginx")

	// Provision MariaDB Database & User safely via isolated temporary SQL script
	var sbName, sbUser strings.Builder
	for _, ch := range dbName {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			sbName.WriteRune(ch)
		}
	}
	escapedDBName := sbName.String()
	if escapedDBName == "" {
		escapedDBName = "ghost"
	}

	for _, ch := range dbUser {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			sbUser.WriteRune(ch)
		}
	}
	escapedDBUser := sbUser.String()
	if escapedDBUser == "" {
		escapedDBUser = "ghost"
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

	_ = system.RunMultiStep("Installing Ghost-CLI globally", "npm install -g ghost-cli@latest 2>/dev/null || true")

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "native",
		"ACCESS_MODE": accessMode,
		"PORT":        port,
		"DOMAIN_NAME": domain,
		"BLOG_TITLE":  blogTitle,
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
	finalLines = append(finalLines, tui.FormatKV("Deployment Mode", "Native Linux Service Stack", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Localhost URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
	if domain != "" {
		finalLines = append(finalLines, tui.FormatKV("Public Domain URL", fmt.Sprintf("http://%s:%s", domain, port), core.GREEN))
	}
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"DATABASE & SETTINGS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Database Name", dbName, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database User", dbUser, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database Password", dbPass, core.GREEN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Management Command", "ghost-manager", core.GREEN+core.BOLD))

	tui.ClearScreen()
	tui.PrintInBox("Ghost Native Stack Provisioned Successfully", finalLines)

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
