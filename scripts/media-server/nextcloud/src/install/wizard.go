// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Nextcloud Enterprise Cloud Collaboration (nextcloud)
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

	"krotek-nextcloud-installer/core"
	"krotek-nextcloud-installer/logger"
	"krotek-nextcloud-installer/system"
	"krotek-nextcloud-installer/tui"
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

// FreshInstallWizard handles the end-to-end installation pipeline for Nextcloud Enterprise Cloud Collaboration.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	accessMode := "local"
	envType := targetEnv
	if envType == "" {
		envType = "docker"
	}

	webserver := os.Getenv("KROTEK_WEBSERVER")
	if strings.ToLower(webserver) == "apache" || strings.ToLower(webserver) == "httpd" {
		webserver = "apache"
	} else {
		webserver = "nginx"
	}

	port := "80"
	domainName := ""
	enableSSL := false
	sslEmail := ""

	adminUser := "admin"
	adminPass := ""
	dbName := "nextcloud"
	dbUser := "nextcloud_user"
	dbPass := ""
	rootPass := ""
	composeDir := "/etc/nextcloud-docker"

	PromptExistingConfigAction("Nextcloud Cloud Storage", "/var/www/nextcloud/config/config.php")

	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
		if os.Getenv("KROTEK_PORT") != "" {
			port = os.Getenv("KROTEK_PORT")
		}
		if os.Getenv("KROTEK_ADMIN_USER") != "" {
			adminUser = os.Getenv("KROTEK_ADMIN_USER")
		}
		if os.Getenv("KROTEK_ADMIN_PASS") != "" {
			adminPass = os.Getenv("KROTEK_ADMIN_PASS")
		} else {
			adminPass = GenerateRandomPassword(16)
		}
		if dbPass == "" {
			dbPass = GenerateRandomPassword(16)
		}
		if rootPass == "" {
			rootPass = GenerateRandomPassword(16)
		}
		if envType == "docker" {
			ExecuteDockerInstall(accessMode, port, domainName, enableSSL, sslEmail, adminUser, adminPass, dbName, dbUser, dbPass, rootPass, composeDir)
		} else {
			ExecuteNativeInstall(accessMode, port, domainName, enableSSL, sslEmail, adminUser, adminPass, dbName, dbUser, dbPass, webserver)
		}
		return
	}

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/4: Choose Access Scope (Docker Nextcloud Hub)"
			if envType == "native" {
				wizardTitle = "Step 1/5: Choose Access Scope (Native Nextcloud Hub)"
			}

			idx, _, status := tui.SelectMenu(wizardTitle, []string{
				"Local Network Only (LAN / Private Cloud Storage)",
				"Public Internet Domain (Domain + WAN Remote Sync)",
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
			if envType == "native" {
				currentStep = 2
			} else {
				currentStep = 3
			}

		case 2:
			if envType == "docker" {
				currentStep = 3
				continue
			}
			tui.ClearScreen()
			wsCardLines := []string{
				core.BOLD + core.CYAN + "Native Web Server Engine Selection" + core.NC,
				"",
				"  Choose the HTTP web server daemon to serve Nextcloud Hub on this host:",
				"",
				"  " + core.BOLD + core.GREEN + "[1] NGINX (LEMP Stack - Recommended)" + core.NC,
				"      Fastest event-driven I/O, lowest memory footprint, and high concurrency.",
				"",
				"  " + core.BOLD + core.YELLOW + "[2] Apache HTTP Server (LAMP Stack)" + core.NC,
				"      Full native .htaccess rewrite support, dynamic modules, and proven stability.",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/5: Web Server Engine", wsCardLines)

			wsIdx, _, wsStatus := tui.SelectMenu("Select Web Server Engine", []string{
				"NGINX (LEMP Stack - High Concurrency & Modern Reverse Proxy)",
				"Apache HTTP Server (LAMP Stack - Native .htaccess & Mod_PHP/FPM)",
			})
			if wsStatus != 0 {
				currentStep = 1
				continue
			}
			if wsIdx == 0 {
				webserver = "nginx"
			} else {
				webserver = "apache"
			}
			currentStep = 3

		case 3:
			tui.ClearScreen()
			backTarget := 1
			stepLabel := "Step 2/4"
			if envType == "native" {
				backTarget = 2
				stepLabel = "Step 3/5"
			}

			if accessMode == "local" {
				cardLines := []string{
					core.BOLD + core.CYAN + "Local Network Configuration" + core.NC,
					"",
					"  Specify the listening HTTP port for Nextcloud LAN access.",
					"  " + core.BOLD + "Default Port:" + core.NC + " " + core.CYAN + "80" + core.NC + " (or 8080)",
				}
				helpMsg := fmt.Sprintf("[Esc]  ->  Back to Step %d", backTarget)
				tui.PrintInBox("--no-wait", "--help-str", helpMsg, fmt.Sprintf("%s: LAN Network Options", stepLabel), cardLines)

				portIn, st := tui.ReadTextInput("Nextcloud Web Port", port)
				if st != 0 {
					currentStep = backTarget
					continue
				}
				if resolvedPort, ok := system.ResolvePortConflict(portIn, "tcp", true); !ok {
					currentStep = backTarget
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
					"  Enter your public Domain Name (e.g. cloud.example.com).",
					"  Ensure your domain DNS points to this server's Public IP.",
				}
				helpMsg := fmt.Sprintf("[Esc]  ->  Back to Step %d", backTarget)
				tui.PrintInBox("--no-wait", "--help-str", helpMsg, fmt.Sprintf("%s: Public Domain Setup", stepLabel), cardLines)

				domIn, st := tui.ReadTextInput("Domain Name (FQDN)", domainName)
				if st != 0 {
					currentStep = backTarget
					continue
				}
				domainName = system.CleanDomain(domIn)

				portIn, st := tui.ReadTextInput("Nextcloud Web Port", port)
				if st != 0 {
					currentStep = backTarget
					continue
				}
				if resolvedPort, ok := system.ResolvePortConflict(portIn, "tcp", true); !ok {
					currentStep = backTarget
					continue
				} else {
					port = resolvedPort
				}

				res := tui.AskYesNo("Enable automated Let's Encrypt SSL certificate (HTTPS)?")
				if res == 2 {
					currentStep = backTarget
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
						currentStep = backTarget
						continue
					}
					sslEmail = emailIn
				} else {
					enableSSL = false
					sslEmail = ""
				}
			}

			currentStep = 4

		case 4:
			tui.ClearScreen()
			stepLabel := "Step 3/4"
			if envType == "native" {
				stepLabel = "Step 4/5"
			}
			credCardLines := []string{
				core.BOLD + core.CYAN + "Nextcloud Administrator & Database Setup" + core.NC,
				"",
				"  Configure master administrator credentials and database settings.",
				"  " + core.BOLD + "Default Admin User :" + core.NC + " " + core.YELLOW + "admin" + core.NC,
				"  " + core.BOLD + "Admin Password     :" + core.NC + " " + core.GREEN + "(Auto-generated if left blank)" + core.NC,
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 3", fmt.Sprintf("%s: Nextcloud Credentials", stepLabel), credCardLines)

			uIn, st := tui.ReadTextInput("Nextcloud Admin Username", adminUser)
			if st != 0 {
				currentStep = 3
				continue
			}
			if uIn != "" {
				adminUser = uIn
			}

			pIn, st := tui.ReadPasswordInput("Nextcloud Admin Password (blank for random)")
			if st != 0 {
				currentStep = 3
				continue
			}
			if pIn != "" {
				adminPass = pIn
			} else if adminPass == "" {
				adminPass = GenerateRandomPassword(16)
			}

			if dbPass == "" {
				dbPass = GenerateRandomPassword(16)
			}
			if rootPass == "" {
				rootPass = GenerateRandomPassword(16)
			}

			currentStep = 5

		case 5:
			tui.ClearScreen()
			stepLabel := "Step 4/4"
			if envType == "native" {
				stepLabel = "Step 5/5"
			}

			summaryLines := []string{
				tui.FormatKV("Deployment Target", func() string {
					if envType == "docker" {
						return "Docker Compose Container Stack (Nextcloud + MariaDB)"
					}
					if webserver == "apache" {
						return "Native Dedicated LAMP Stack (Apache + PHP-FPM + MariaDB)"
					}
					return "Native Dedicated LEMP Stack (NGINX + PHP-FPM + MariaDB)"
				}(), core.GREEN),
			}

			if envType == "native" {
				wsDisplay := "NGINX (High Concurrency Reverse Proxy)"
				if webserver == "apache" {
					wsDisplay = "Apache HTTP Server (Native .htaccess Enabled)"
				}
				summaryLines = append(summaryLines, tui.FormatKV("Web Server", wsDisplay, core.CYAN+core.BOLD))
			}

			summaryLines = append(summaryLines,
				tui.FormatKV("Access Scope", func() string {
					if accessMode == "public" {
						return "Public Internet (Domain + WAN)"
					}
					return "Local Network Only (LAN)"
				}(), core.GREEN),
				tui.FormatKV("HTTP Web Port", port, core.CYAN),
				tui.FormatKV("Admin Username", adminUser, core.YELLOW+core.BOLD),
				tui.FormatKV("Admin Password", adminPass, core.GREEN+core.BOLD),
				tui.FormatKV("Database Engine", "MariaDB (Automated Provisioning)", core.GREEN),
				tui.FormatKV("Database Name", dbName, core.CYAN),
				tui.FormatKV("Database User", dbUser, core.CYAN),
			)

			if domainName != "" {
				summaryLines = append(summaryLines, tui.FormatKV("Public Domain Name", domainName, core.YELLOW))
				if enableSSL {
					summaryLines = append(summaryLines, tui.FormatKV("Let's Encrypt SSL", fmt.Sprintf("[ENABLED] (%s)", sslEmail), core.GREEN))
				}
			}

			if envType == "docker" {
				summaryLines = append(summaryLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
			} else {
				summaryLines = append(summaryLines, tui.FormatKV("Document Root", "/var/www/nextcloud", core.CYAN))
			}

			summaryLines = append(summaryLines, tui.FormatKV("Automated Firewall", "[ENABLED] (Optimal Ports Opened)", core.CYAN))
			summaryLines = append(summaryLines, "")
			summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"Do you want to proceed with installation?"+core.NC)

			choices := []string{"YES", "NO"}

			idx, _, status := tui.SelectMenuWithContent(fmt.Sprintf("%s: Installation Summary", stepLabel), summaryLines, choices)
			if status == 2 || idx != 0 {
				logger.LogInfo("Installation aborted by user.")
				return
			}
			if status == 1 {
				currentStep = 4
				continue
			}

			if envType == "docker" {
				ExecuteDockerInstall(accessMode, port, domainName, enableSSL, sslEmail, adminUser, adminPass, dbName, dbUser, dbPass, rootPass, composeDir)
			} else {
				ExecuteNativeInstall(accessMode, port, domainName, enableSSL, sslEmail, adminUser, adminPass, dbName, dbUser, dbPass, webserver)
			}
			return
		}
	}
}

// ==============================================================================
// DOCKER COMPOSE EXECUTOR
// ==============================================================================
func ExecuteDockerInstall(accessMode, port, domain string, enableSSL bool, sslEmail, adminUser, adminPass, dbName, dbUser, dbPass, rootPass, composeDir string) {
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing NEXTCLOUD (Docker Compose Stack) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)

	if !system.EnsureDockerEngine() {
		return
	}

	if composeDir == "" {
		composeDir = "/etc/nextcloud-docker"
	}
	_ = os.MkdirAll(composeDir, 0755)

	localIP := system.GetLocalIP()
	if localIP == "" {
		localIP = "127.0.0.1"
	}

	trustedDomains := fmt.Sprintf("%s localhost", localIP)
	if domain != "" {
		trustedDomains = fmt.Sprintf("%s %s localhost", domain, localIP)
	}

	composeYaml := fmt.Sprintf(`version: '3.8'

services:
  db:
    image: mariadb:lts
    container_name: krotek-nextcloud-db
    restart: always
    environment:
      MYSQL_ROOT_PASSWORD: "%s"
      MYSQL_DATABASE: "%s"
      MYSQL_USER: "%s"
      MYSQL_PASSWORD: "%s"
    volumes:
      - db_data:/var/lib/mysql

  nextcloud:
    image: nextcloud:latest
    container_name: krotek-nextcloud
    restart: always
    depends_on:
      - db
    ports:
      - "%s:80"
    environment:
      MYSQL_HOST: db
      MYSQL_DATABASE: "%s"
      MYSQL_USER: "%s"
      MYSQL_PASSWORD: "%s"
      NEXTCLOUD_ADMIN_USER: "%s"
      NEXTCLOUD_ADMIN_PASSWORD: "%s"
      NEXTCLOUD_TRUSTED_DOMAINS: "%s"
    volumes:
      - nextcloud_data:/var/www/html

volumes:
  db_data:
  nextcloud_data:
`, escapeYamlString(rootPass), escapeYamlString(dbName), escapeYamlString(dbUser), escapeYamlString(dbPass), port, escapeYamlString(dbName), escapeYamlString(dbUser), escapeYamlString(dbPass), escapeYamlString(adminUser), escapeYamlString(adminPass), trustedDomains)

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}

	// Universal firewall hardening (UFW, Firewalld, raw iptables/ip6tables, rules persistence)
	system.ConfigureFirewallRules(port, "80", "443")

	if !system.RunDockerCompose(composeDir, "Deploying Nextcloud & MariaDB Docker Stack", true, "up", "-d") {
		return
	}

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "docker",
		"ACCESS_MODE": accessMode,
		"PORT":        port,
		"DOMAIN_NAME": domain,
		"ADMIN_USER":  adminUser,
		"ADMIN_PASS":  adminPass,
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
	finalLines = append(finalLines, tui.FormatKV("Deployment Mode", "Docker Container Stack (Nextcloud + MariaDB)", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Localhost Web URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
	if domain != "" {
		finalLines = append(finalLines, tui.FormatKV("Public Domain URL", fmt.Sprintf("http://%s:%s", domain, port), core.GREEN))
	}
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"NEXTCLOUD CREDENTIALS & ACCESS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Admin Username", adminUser, core.YELLOW+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("Admin Password", adminPass, core.GREEN+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("Database User", dbUser, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database Password", dbPass, core.GREEN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Management Command", "nextcloud-manager", core.GREEN+core.BOLD))

	tui.ClearScreen()
	tui.PrintInBox("Nextcloud Docker Stack Installed Successfully", finalLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// ==============================================================================
// NATIVE INSTALLATION EXECUTOR
// ==============================================================================
func ExecuteNativeInstall(accessMode, port, domain string, enableSSL bool, sslEmail, adminUser, adminPass, dbName, dbUser, dbPass, webserver string) {
	if webserver == "" {
		webserver = "nginx"
	}
	webserver = strings.ToLower(webserver)
	if webserver != "apache" && webserver != "httpd" {
		webserver = "nginx"
	} else {
		webserver = "apache"
	}

	// Deactivate opposing web server if active to prevent port 80 collisions
	if webserver == "nginx" {
		if system.IsServiceActive("apache2") {
			system.StopService("apache2")
			system.DisableService("apache2")
		}
		if system.IsServiceActive("httpd") {
			system.StopService("httpd")
			system.DisableService("httpd")
		}
	} else {
		if system.IsServiceActive("nginx") {
			system.StopService("nginx")
			system.DisableService("nginx")
		}
	}

	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}

	tui.ClearScreen()
	stackName := "LEMP Stack (NGINX)"
	if webserver == "apache" {
		stackName = "LAMP Stack (Apache)"
	}
	fmt.Printf("\r\n%s=== Installing NEXTCLOUD Native %s ===%s\r\n\r\n", core.BOLD+core.YELLOW, stackName, core.NC)
	logger.LogInfo("Starting Native Nextcloud installation with " + strings.ToUpper(webserver) + "...")

	var pkgs []string
	if webserver == "apache" {
		switch core.SysInfo.Family {
		case "debian":
			pkgs = []string{"apache2", "mariadb-server", "php", "php-fpm", "php-mysql", "php-curl", "php-gd", "php-mbstring", "php-xml", "php-zip", "php-intl", "php-bcmath", "php-gmp", "libapache2-mod-php", "bzip2", "unzip", "curl", "tar"}
		case "rhel":
			pkgs = []string{"httpd", "mariadb-server", "php", "php-fpm", "php-mysqlnd", "php-gd", "php-mbstring", "php-xml", "php-pecl-zip", "php-intl", "php-bcmath", "php-gmp", "bzip2", "unzip", "curl", "tar"}
		case "alpine":
			phpPrefix := system.DetectLatestAlpinePHP()
			pkgs = []string{"apache2", "apache2-openrc", "mariadb", "mariadb-client", "mariadb-server-utils", "mariadb-openrc", phpPrefix + "-apache2", phpPrefix + "-fpm", phpPrefix + "-mysqli", phpPrefix + "-curl", phpPrefix + "-gd", phpPrefix + "-mbstring", phpPrefix + "-xml", phpPrefix + "-zip", phpPrefix + "-intl", phpPrefix + "-bcmath", phpPrefix + "-gmp", "bzip2", "unzip", "curl", "tar"}
		default:
			pkgs = []string{"apache2", "mariadb-server", "php", "php-fpm", "php-mysql", "php-curl", "php-gd", "php-mbstring", "php-xml", "php-zip", "php-intl", "php-bcmath", "php-gmp", "bzip2", "unzip", "curl", "tar"}
		}
	} else {
		switch core.SysInfo.Family {
		case "debian":
			pkgs = []string{"nginx", "mariadb-server", "php-fpm", "php-mysql", "php-curl", "php-gd", "php-mbstring", "php-xml", "php-zip", "php-intl", "php-bcmath", "php-gmp", "bzip2", "unzip", "curl", "tar"}
		case "rhel":
			pkgs = []string{"nginx", "mariadb-server", "php-fpm", "php-mysqlnd", "php-gd", "php-mbstring", "php-xml", "php-pecl-zip", "php-intl", "php-bcmath", "php-gmp", "bzip2", "unzip", "curl", "tar"}
		case "alpine":
			phpPrefix := system.DetectLatestAlpinePHP()
			pkgs = []string{"nginx", "nginx-openrc", "mariadb", "mariadb-client", "mariadb-server-utils", "mariadb-openrc", phpPrefix + "-fpm", phpPrefix + "-mysqli", phpPrefix + "-curl", phpPrefix + "-gd", phpPrefix + "-mbstring", phpPrefix + "-xml", phpPrefix + "-zip", phpPrefix + "-intl", phpPrefix + "-bcmath", phpPrefix + "-gmp", "bzip2", "unzip", "curl", "tar"}
		default:
			pkgs = []string{"nginx", "mariadb-server", "php-fpm", "php-mysql", "php-curl", "php-gd", "php-mbstring", "php-xml", "php-zip", "php-intl", "php-bcmath", "php-gmp", "bzip2", "unzip", "curl", "tar"}
		}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogError("Failed to install required Nextcloud packages.")
		return
	}

	// Universal firewall hardening (UFW, Firewalld, raw iptables/ip6tables, rules persistence)
	system.ConfigureFirewallRules(port, "80", "443")

	system.InitMariaDBDatadir()
	system.StartService("mariadb")

	// Provision MariaDB Database & User safely via isolated temporary SQL script
	escapedDBName := strings.ReplaceAll(dbName, "`", "")
	escapedDBUser := strings.ReplaceAll(dbUser, "'", "''")
	escapedDBPass := strings.ReplaceAll(dbPass, "'", "''")

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

	// Determine Web User
	webUser := "www-data"
	if core.SysInfo.Family == "rhel" {
		if webserver == "apache" {
			webUser = "apache"
		} else {
			webUser = "nginx"
		}
	} else if core.SysInfo.Family == "alpine" {
		if webserver == "apache" {
			webUser = "apache"
		} else {
			webUser = "nginx"
		}
	}

	// Download & deploy Nextcloud core archive safely with zip-slip/tar-slip protections.
	if _, err := os.Stat("/var/www/nextcloud/version.php"); os.IsNotExist(err) || core.OverwriteExistingConfig {
		_ = os.MkdirAll("/var/www/nextcloud", 0755)
		ncTmpDir, ncErr := os.MkdirTemp("", "krotek-nc-dl-*")
		if ncErr != nil {
			logger.LogError(fmt.Sprintf("Failed to allocate secure download directory: %v", ncErr))
			return
		}
		_ = os.Chmod(ncTmpDir, 0700)
		defer os.RemoveAll(ncTmpDir)
		ncZip := filepath.Join(ncTmpDir, "nextcloud.zip")
		ncTarGz := filepath.Join(ncTmpDir, "nextcloud.tar.gz")
		if system.RunStep("Downloading Nextcloud core archive", "curl", "--proto", "=https", "--tlsv1.2", "-fsSL", "https://download.nextcloud.com/server/releases/latest.zip", "-o", ncZip) {
			if err := system.ExtractZipSafe(ncZip, "/var/www", 200000, 256*1024*1024, 2*1024*1024*1024); err != nil {
				logger.LogError(fmt.Sprintf("Nextcloud zip validation/extraction failed: %v", err))
				return
			}
		} else if system.RunStep("Downloading Nextcloud tarball fallback", "curl", "--proto", "=https", "--tlsv1.2", "-fsSL", "https://download.nextcloud.com/server/releases/latest.tar.gz", "-o", ncTarGz) {
			if err := system.ValidateTarGzArchive(ncTarGz, 2*1024*1024*1024); err != nil {
				logger.LogError(fmt.Sprintf("Nextcloud archive validation failed: %v", err))
				return
			}
			_ = system.RunStep("Extracting Nextcloud core archive", "tar", "-xzf", ncTarGz, "-C", "/var/www/")
		}
	}

	_ = os.MkdirAll("/var/www/nextcloud/data", 0770)
	_ = os.MkdirAll("/var/www/nextcloud/config", 0770)
	system.SecureWebRootPermissions("/var/www/nextcloud", webUser)
	for _, privDir := range []string{"/var/www/nextcloud/data", "/var/www/nextcloud/config"} {
		_ = filepath.WalkDir(privDir, func(p string, d os.DirEntry, wErr error) error {
			if wErr != nil || d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				_ = os.Chmod(p, 0770)
			} else if d.Type().IsRegular() {
				_ = os.Chmod(p, 0660)
			}
			return nil
		})
	}

	// Tune PHP memory, upload limits, and opcache
	system.OptimizePHPSettings()

	localIP := system.GetLocalIP()
	if localIP == "" {
		localIP = "127.0.0.1"
	}

	srvName := "_"
	if domain != "" {
		srvName = domain
	}

	if webserver == "apache" {
		apacheSvc := "apache2"
		if core.SysInfo.Family == "rhel" {
			apacheSvc = "httpd"
		}

		apacheVhost := fmt.Sprintf(`<VirtualHost *:%s>
    ServerName %s
    DocumentRoot /var/www/nextcloud

    <Directory /var/www/nextcloud/>
        Options +FollowSymlinks
        AllowOverride All
        Require all granted

        <IfModule mod_dav.c>
            Dav off
        </IfModule>

        SetEnv HOME /var/www/nextcloud
        SetEnv HTTP_HOME /var/www/nextcloud
    </Directory>

    ErrorLog /var/log/nextcloud_error.log
    CustomLog /var/log/nextcloud_access.log combined
</VirtualHost>
`, port, srvName)

		if core.SysInfo.Family == "debian" {
			_ = os.MkdirAll("/etc/apache2/sites-available", 0755)
			_ = system.WriteFileNoFollow("/etc/apache2/sites-available/nextcloud.conf", []byte(apacheVhost), 0644, "")
			_ = exec.Command("a2enmod", "rewrite", "headers", "env", "dir", "mime", "setenvif").Run()
			_ = exec.Command("a2ensite", "nextcloud.conf").Run()
			if port != "80" {
				portsConf, pErr := os.ReadFile("/etc/apache2/ports.conf")
				if pErr == nil && !strings.Contains(string(portsConf), "Listen "+port) {
					_ = system.AppendFileNoFollow("/etc/apache2/ports.conf", []byte(fmt.Sprintf("\nListen %s\n", port)), 0644)
				}
			}
		} else if core.SysInfo.Family == "rhel" {
			_ = os.MkdirAll("/etc/httpd/conf.d", 0755)
			_ = system.WriteFileNoFollow("/etc/httpd/conf.d/nextcloud.conf", []byte(apacheVhost), 0644, "")
		} else if core.SysInfo.Family == "alpine" {
			_ = os.MkdirAll("/etc/apache2/conf.d", 0755)
			_ = system.WriteFileNoFollow("/etc/apache2/conf.d/nextcloud.conf", []byte(apacheVhost), 0644, "")
		}

		system.StartService(apacheSvc)
		system.StartPHPFPMService()
		system.RestartService(apacheSvc)
	} else {
		// NGINX configuration
		nginxVhost := fmt.Sprintf(`server {
    listen %s;
    server_name %s;
    root /var/www/nextcloud;
    index index.php index.html;

    client_max_body_size 512M;
    fastcgi_buffers 64 4K;

    add_header X-Content-Type-Options nosniff;
    add_header X-XSS-Protection "1; mode=block";
    add_header X-Robots-Tag none;
    add_header X-Download-Options noopen;
    add_header X-Permitted-Cross-Domain-Policies none;
    add_header Referrer-Policy no-referrer;

    location = /robots.txt {
        allow all;
        log_not_found off;
        access_log off;
    }

    location = /.well-known/carddav {
        return 301 $scheme://$host:$server_port/remote.php/dav;
    }
    location = /.well-known/caldav {
        return 301 $scheme://$host:$server_port/remote.php/dav;
    }

    location / {
        rewrite ^ /index.php;
    }

    location ~ ^\/(?:build|tests|config|lib|3rdparty|templates|data)\/ {
        deny all;
    }
    location ~ ^\/(?:\.|autotest|occ|issue|indie|db_|console) {
        deny all;
    }

    location ~ ^\/(?:index|remote|public|cron|core\/ajax\/update|status|ocs\/v[12]|updater\/.+|oc[ms]-provider\/.+)\.php(?:$|\/) {
        fastcgi_split_path_info ^(.+?\.php)(\/.*)$;
        set $path_info $fastcgi_path_info;
        try_files $fastcgi_script_name =404;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_param PATH_INFO $path_info;
        fastcgi_param HTTPS $https if_not_empty;
        fastcgi_param modHeadersAvailable true;
        fastcgi_pass %s;
        fastcgi_intercept_errors on;
        fastcgi_request_buffering off;
    }

    location ~* \.(?:svg|gif|png|html|ttf|woff|ico|jpg|jpeg)$ {
        try_files $uri /index.php$request_uri;
        access_log off;
    }

    location ~ \.php$ {
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_pass %s;
    }
}
`, port, srvName, system.GetFastCGIPass(), system.GetFastCGIPass())

		_ = os.MkdirAll("/etc/nginx/conf.d", 0755)
		_ = system.WriteFileNoFollow("/etc/nginx/conf.d/nextcloud.conf", []byte(nginxVhost), 0644, "")
		if core.SysInfo.Family == "debian" {
			_ = os.Remove("/etc/nginx/sites-enabled/default")
		}

		system.StartService("nginx")
		system.StartPHPFPMService()
		system.RestartService("nginx")
	}

	// Automated Initial Setup via OCC CLI tool if available
	if _, err := os.Stat("/var/www/nextcloud/occ"); err == nil {
		occArgs := []string{
			"-s", "/bin/sh", webUser, "-c",
			`php /var/www/nextcloud/occ maintenance:install "$@" 2>/dev/null || true`,
			"--",
			"--database", "mysql",
			"--database-name", escapedDBName,
			"--database-user", escapedDBUser,
			"--database-pass", dbPass,
			"--admin-user", adminUser,
			"--admin-pass", adminPass,
			"--data-dir", "/var/www/nextcloud/data",
		}
		_ = system.RunStep("Executing Nextcloud initial database automated configuration", "su", occArgs...)

		suArgs1 := []string{"-s", "/bin/sh", webUser, "-c", `php /var/www/nextcloud/occ config:system:set "$@" 2>/dev/null || true`, "--", "trusted_domains", "1", "--value=" + localIP}
		_ = exec.Command("su", suArgs1...).Run()

		suArgs0 := []string{"-s", "/bin/sh", webUser, "-c", `php /var/www/nextcloud/occ config:system:set "$@" 2>/dev/null || true`, "--", "trusted_domains", "0", "--value=localhost"}
		_ = exec.Command("su", suArgs0...).Run()

		if domain != "" {
			suArgs2 := []string{"-s", "/bin/sh", webUser, "-c", `php /var/www/nextcloud/occ config:system:set "$@" 2>/dev/null || true`, "--", "trusted_domains", "2", "--value=" + domain}
			_ = exec.Command("su", suArgs2...).Run()
		}
		_ = exec.Command("chown", "-R", fmt.Sprintf("%s:%s", webUser, webUser), "/var/www/nextcloud").Run()
	}

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "native",
		"WEBSERVER":   webserver,
		"ACCESS_MODE": accessMode,
		"PORT":        port,
		"DOMAIN_NAME": domain,
		"ADMIN_USER":  adminUser,
		"ADMIN_PASS":  adminPass,
		"DB_NAME":     dbName,
		"DB_USER":     dbUser,
		"DB_PASS":     dbPass,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	pubIP := system.GetPublicIP()
	if pubIP == "" {
		pubIP = localIP
	}

	wsDisplayName := "NGINX (LEMP Stack)"
	if webserver == "apache" {
		wsDisplayName = "Apache HTTP Server (LAMP Stack)"
	}

	var finalLines []string
	finalLines = append(finalLines, tui.FormatKV("Deployment Mode", "Native Linux Dedicated Stack", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Web Server", wsDisplayName, core.GREEN+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Localhost Web URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
	if domain != "" {
		finalLines = append(finalLines, tui.FormatKV("Public Domain URL", fmt.Sprintf("http://%s:%s", domain, port), core.GREEN))
	}
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"NEXTCLOUD ADMIN & DATABASE CREDENTIALS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Admin Username", adminUser, core.YELLOW+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("Admin Password", adminPass, core.GREEN+core.BOLD))
	finalLines = append(finalLines, tui.FormatKV("Database Name", dbName, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database User", dbUser, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Database Password", dbPass, core.GREEN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Document Root", "/var/www/nextcloud", core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Management Command", "nextcloud-manager", core.GREEN+core.BOLD))

	tui.ClearScreen()
	tui.PrintInBox(fmt.Sprintf("Nextcloud Native Stack (%s) Installed Successfully", strings.ToUpper(webserver)), finalLines)
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
