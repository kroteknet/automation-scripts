// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: WordPress Web Publishing Stack (wordpress)
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

	"krotek-wordpress-installer/core"
	"krotek-wordpress-installer/logger"
	"krotek-wordpress-installer/system"
	"krotek-wordpress-installer/tui"
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

// FreshInstallWizard handles the end-to-end installation pipeline for WordPress Web Publishing Stack.
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

	siteTitle := "My WordPress Site"
	adminUser := "admin"
	adminPass := ""
	adminEmail := "admin@example.com"
	dbName := "wordpress"
	dbUser := "wp_user"
	dbPass := ""
	rootPass := ""
	composeDir := "/etc/wordpress-docker"

	PromptExistingConfigAction("WordPress CMS", "/var/www/wordpress/wp-config.php")

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/4: Choose Access Scope (Docker WordPress)"
			if envType == "native" {
				wizardTitle = "Step 1/4: Choose Access Scope (Native WordPress)"
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
					"  Specify the listening HTTP port for WordPress LAN / Wi-Fi access.",
					"  " + core.BOLD + "Default Port:" + core.NC + " " + core.CYAN + "80" + core.NC + " (or 8080)",
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: LAN Network Options", cardLines)

				portIn, st := tui.ReadTextInput("WordPress Web Port", port)
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

				portIn, st := tui.ReadTextInput("WordPress Web Port", port)
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
				core.BOLD + core.CYAN + "WordPress Site & Administrator Setup" + core.NC,
				"",
				"  Configure site metadata and initial administrator credentials.",
				"  " + core.BOLD + "Default Admin User :" + core.NC + " " + core.YELLOW + "admin" + core.NC,
				"  " + core.BOLD + "Admin Password     :" + core.NC + " " + core.GREEN + "(Auto-generated if left blank)" + core.NC,
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: WordPress Configuration", credCardLines)

			tIn, st := tui.ReadTextInput("Website Title", siteTitle)
			if st != 0 {
				currentStep = 2
				continue
			}
			if tIn != "" {
				siteTitle = tIn
			}

			uIn, st := tui.ReadTextInput("WordPress Admin Username", adminUser)
			if st != 0 {
				currentStep = 2
				continue
			}
			if uIn != "" {
				adminUser = uIn
			}

			pIn, st := tui.ReadPasswordInput("WordPress Admin Password (blank for random)")
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
						return "Docker Compose Container Stack (WordPress + MariaDB)"
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
				tui.FormatKV("Site Title", siteTitle, core.YELLOW+core.BOLD),
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
				summaryLines = append(summaryLines, tui.FormatKV("Document Root", "/var/www/wordpress", core.CYAN))
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
	fmt.Printf("\r\n%s=== Installing WORDPRESS (Docker Compose Stack) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)

	if !system.EnsureDockerEngine() {
		return
	}

	// Mutual conflict check: if native web servers are running on the port
	for _, srv := range []string{"nginx", "apache2", "httpd"} {
		if exec.Command("systemctl", "is-active", "--quiet", srv).Run() == nil {
			tui.ClearScreen()
			conflictCard := []string{
				core.BOLD + core.YELLOW + "SERVICE CONFLICT DETECTED" + core.NC,
				"",
				fmt.Sprintf("  Native Linux Service '%s' is currently %s[ACTIVE / RUNNING]%s.", srv, core.GREEN, core.NC),
				"  To deploy Docker WordPress, the conflicting service must be stopped",
				"  to free network ports.",
				"",
				core.BOLD + core.CYAN + "Action Required:" + core.NC + fmt.Sprintf(" Stop & Disable %s service?", srv),
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Service Conflict Notice", conflictCard)

			res := tui.AskYesNo(fmt.Sprintf("Stop Native %s service and continue Docker deployment?", srv))
			if res != 0 {
				logger.LogInfo("Docker installation cancelled by user due to service conflict.")
				return
			}

			system.StopService(srv)
			_ = exec.Command("systemctl", "disable", srv).Run()
		}
	}

	if composeDir == "" {
		composeDir = "/etc/wordpress-docker"
	}
	_ = os.MkdirAll(composeDir, 0755)

	composeYaml := fmt.Sprintf(`version: '3.8'

services:
  db:
    image: mariadb:lts
    container_name: krotek-wordpress-db
    restart: always
    environment:
      MYSQL_ROOT_PASSWORD: "%s"
      MYSQL_DATABASE: "%s"
      MYSQL_USER: "%s"
      MYSQL_PASSWORD: "%s"
    volumes:
      - db_data:/var/lib/mysql

  wordpress:
    image: wordpress:latest
    container_name: krotek-wordpress
    restart: always
    depends_on:
      - db
    ports:
      - "%s:80"
    environment:
      WORDPRESS_DB_HOST: db:3306
      WORDPRESS_DB_NAME: "%s"
      WORDPRESS_DB_USER: "%s"
      WORDPRESS_DB_PASSWORD: "%s"
    volumes:
      - wp_data:/var/www/html

volumes:
  db_data:
  wp_data:
`, escapeYamlString(rootPass), escapeYamlString(dbName), escapeYamlString(dbUser), escapeYamlString(dbPass), port, escapeYamlString(dbName), escapeYamlString(dbUser), escapeYamlString(dbPass))

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}

	// Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	system.ConfigureFirewallRules(port+"/tcp", "443/tcp")

	if !system.RunDockerCompose(composeDir, "Deploying WordPress & MariaDB Docker Stack", true, "up", "-d") {
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
	finalLines = append(finalLines, tui.FormatKV("Deployment Mode", "Docker Container Stack (WordPress + MariaDB)", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Localhost Web URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
	if domain != "" {
		finalLines = append(finalLines, tui.FormatKV("Public Domain URL", fmt.Sprintf("http://%s:%s", domain, port), core.GREEN))
	}
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"WORDPRESS CREDENTIALS & ACCESS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Admin Username", adminUser, core.YELLOW+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("Admin Password", adminPass, core.GREEN+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("Admin Email", adminEmail, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database User", dbUser, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database Password", dbPass, core.GREEN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Management Command", "wordpress-manager", core.GREEN+core.BOLD))

	tui.ClearScreen()
	tui.PrintInBox("WordPress Docker Stack Installed Successfully", finalLines)

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
	fmt.Printf("\r\n%s=== Installing WORDPRESS Native LEMP Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native WordPress installation...")

	var pkgs []string
	switch core.SysInfo.Family {
	case "debian":
		pkgs = []string{"nginx", "mariadb-server", "php-fpm", "php-mysql", "php-curl", "php-gd", "php-mbstring", "php-xml", "php-zip", "tar", "curl"}
	case "rhel":
		pkgs = []string{"nginx", "mariadb-server", "php-fpm", "php-mysqlnd", "php-gd", "php-mbstring", "php-xml", "php-pecl-zip", "tar", "curl"}
	case "alpine":
		phpPrefix := system.DetectLatestAlpinePHP()
		pkgs = []string{"nginx", "nginx-openrc", "mariadb", "mariadb-client", "mariadb-server-utils", "mariadb-openrc", phpPrefix + "-fpm", phpPrefix + "-mysqli", phpPrefix + "-curl", phpPrefix + "-gd", phpPrefix + "-mbstring", phpPrefix + "-xml", phpPrefix + "-zip", "tar", "curl"}
	default:
		pkgs = []string{"nginx", "mariadb-server", "php-fpm", "php-mysql", "php-curl", "php-gd", "php-mbstring", "php-xml", "php-zip", "tar", "curl"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogError("Failed to install required WordPress packages.")
		return
	}

	// Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
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
		escapedDBName = "wordpress"
	}

	for _, ch := range dbUser {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			sbUser.WriteRune(ch)
		}
	}
	escapedDBUser := sbUser.String()
	if escapedDBUser == "" {
		escapedDBUser = "wordpress"
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

	// Download & extract WordPress safely without shell pipe or unvalidated archive extraction.
	_ = os.MkdirAll("/var/www/wordpress", 0755)
	if dlTmp, dlErr := os.MkdirTemp("", "krotek-wp-dl-*"); dlErr != nil {
		logger.LogError(fmt.Sprintf("Failed to allocate secure download directory: %v", dlErr))
		return
	} else {
		_ = os.Chmod(dlTmp, 0700)
		defer os.RemoveAll(dlTmp)
		tarPath := filepath.Join(dlTmp, "latest.tar.gz")
		if system.RunStep("Downloading WordPress core", "curl", "--proto", "=https", "--tlsv1.2", "-fsSL", "https://wordpress.org/latest.tar.gz", "-o", tarPath) {
			if err := system.ValidateTarGzArchive(tarPath, 512*1024*1024); err != nil {
				logger.LogError(fmt.Sprintf("WordPress archive validation failed: %v", err))
				return
			}
			_ = system.RunStep("Extracting WordPress core", "tar", "-xzf", tarPath, "-C", "/var/www/")
		}
	}

	webUser := "www-data"
	if core.SysInfo.Family == "rhel" || core.SysInfo.Family == "alpine" {
		webUser = "nginx"
	}
	system.SecureWebRootPermissions("/var/www/wordpress", webUser)

	// Create wp-config.php with properly escaped database credentials
	cleanWpDBName := strings.ReplaceAll(strings.ReplaceAll(dbName, "\\", "\\\\"), "'", "\\'")
	cleanWpDBUser := strings.ReplaceAll(strings.ReplaceAll(dbUser, "\\", "\\\\"), "'", "\\'")
	cleanWpDBPass := strings.ReplaceAll(strings.ReplaceAll(dbPass, "\\", "\\\\"), "'", "\\'")

	wpConfig := fmt.Sprintf(`<?php
define( 'DB_NAME', '%s' );
define( 'DB_USER', '%s' );
define( 'DB_PASSWORD', '%s' );
define( 'DB_HOST', 'localhost' );
define( 'DB_CHARSET', 'utf8mb4' );
define( 'DB_COLLATE', '' );

define( 'AUTH_KEY',         '%s' );
define( 'SECURE_AUTH_KEY',  '%s' );
define( 'LOGGED_IN_KEY',    '%s' );
define( 'NONCE_KEY',        '%s' );
define( 'AUTH_SALT',        '%s' );
define( 'SECURE_AUTH_SALT', '%s' );
define( 'LOGGED_IN_SALT',   '%s' );
define( 'NONCE_SALT',       '%s' );

$table_prefix = 'wp_';
define( 'WP_DEBUG', false );

if ( ! defined( 'ABSPATH' ) ) {
    define( 'ABSPATH', __DIR__ . '/' );
}
require_once ABSPATH . 'wp-settings.php';
`, cleanWpDBName, cleanWpDBUser, cleanWpDBPass,
		GenerateRandomPassword(48), GenerateRandomPassword(48), GenerateRandomPassword(48), GenerateRandomPassword(48),
		GenerateRandomPassword(48), GenerateRandomPassword(48), GenerateRandomPassword(48), GenerateRandomPassword(48))

	if err := system.WriteFileNoFollow("/var/www/wordpress/wp-config.php", []byte(wpConfig), 0600, webUser); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write wp-config.php: %v", err))
		return
	}

	// Nginx configuration
	srvName := "_"
	if domain != "" {
		srvName = fmt.Sprintf("%s www.%s", domain, domain)
	}

	nginxVhost := fmt.Sprintf(`server {
    listen %s;
    server_name %s;
    root /var/www/wordpress;
    index index.php index.html;

    location / {
        try_files $uri $uri/ /index.php?$args;
    }

    location ~ \.php$ {
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_pass %s;
    }

    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg)$ {
        expires max;
        log_not_found off;
    }
}
`, port, srvName, system.GetFastCGIPass())

	_ = system.WriteFileNoFollow("/etc/nginx/conf.d/wordpress.conf", []byte(nginxVhost), 0644, "")
	if core.SysInfo.Family == "debian" {
		_ = os.Remove("/etc/nginx/sites-enabled/default")
	}

	system.StartService("nginx")
	system.StartPHPFPMService()
	system.RestartService("nginx")

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
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"WORDPRESS CREDENTIALS & ACCESS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Admin Username", adminUser, core.YELLOW+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("Admin Password", adminPass, core.GREEN+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("Admin Email", adminEmail, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database Name", dbName, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database User", dbUser, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database Password", dbPass, core.GREEN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Document Root", "/var/www/wordpress", core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Management Command", "wordpress-manager", core.GREEN+core.BOLD))

	tui.ClearScreen()
	tui.PrintInBox("WordPress Native LEMP Stack Installed Successfully", finalLines)

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
