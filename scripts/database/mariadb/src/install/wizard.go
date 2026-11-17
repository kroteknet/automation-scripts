// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: MariaDB Relational Database Server (mariadb)
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
	"strconv"
	"strings"
	"time"

	"krotek-mariadb-installer/core"
	"krotek-mariadb-installer/logger"
	"krotek-mariadb-installer/modules"
	"krotek-mariadb-installer/system"
	"krotek-mariadb-installer/tui"
)

// FreshInstallWizard handles the end-to-end installation pipeline for MariaDB Relational Database Server.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	envType := targetEnv
	if envType == "" {
		envType = "native"
	}

	accessScope := "local" // "local" (127.0.0.1) or "network" (0.0.0.0)
	port := "3306"
	rootPassword, pErr := modules.GenerateRandomPass(16)
	if pErr != nil {
		logger.LogError(fmt.Sprintf("Failed to generate initial root password: %v", pErr))
		tui.PrintInBox("Security Error", []string{core.RED + "[ERROR] CSPRNG failure generating root password." + core.NC})
		return
	}
	isCustomRootPass := false

	createAppDB := false
	appDBName := "app_db"
	appDBUser := "app_user"
	appDBPass, pErr := modules.GenerateRandomPass(16)
	if pErr != nil {
		logger.LogError(fmt.Sprintf("Failed to generate initial app password: %v", pErr))
		tui.PrintInBox("Security Error", []string{core.RED + "[ERROR] CSPRNG failure generating app password." + core.NC})
		return
	}

	composeDir := "/etc/mariadb-docker"

	// ─── UNATTENDED / NON-INTERACTIVE AUTOMATION MODE ─────────────────────────
	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" || os.Getenv("NON_INTERACTIVE") == "1" {
		if envScope := os.Getenv("KROTEK_BIND"); envScope != "" {
			if envScope == "0.0.0.0" || envScope == "network" {
				accessScope = "network"
			} else {
				accessScope = "local"
			}
		}
		if envPort := os.Getenv("KROTEK_PORT"); envPort != "" {
			port = envPort
		}
		if envPass := os.Getenv("KROTEK_ROOT_PASS"); envPass != "" {
			rootPassword = envPass
		} else if envAdminPass := os.Getenv("KROTEK_ADMIN_PASS"); envAdminPass != "" {
			rootPassword = envAdminPass
		}
		if envDB := os.Getenv("KROTEK_DB_NAME"); envDB != "" {
			createAppDB = true
			appDBName = envDB
			if envUser := os.Getenv("KROTEK_DB_USER"); envUser != "" {
				appDBUser = envUser
			}
			if envDBPass := os.Getenv("KROTEK_DB_PASS"); envDBPass != "" {
				appDBPass = envDBPass
			}
		}
		if envMode := os.Getenv("KROTEK_MODE"); envMode != "" {
			envType = envMode
		}

		logger.LogInfo(fmt.Sprintf("Starting Unattended MariaDB installation: mode=%s, port=%s, scope=%s", envType, port, accessScope))
		if envType == "native" {
			ExecuteNativeInstall(accessScope, port, rootPassword, createAppDB, appDBName, appDBUser, appDBPass)
		} else {
			ExecuteDockerInstall(accessScope, port, rootPassword, createAppDB, appDBName, appDBUser, appDBPass, composeDir)
		}
		return
	}

	PromptExistingConfigAction("MariaDB Database Server", "/etc/mysql/my.cnf", "/etc/my.cnf")

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/4: Choose Access Scope & Network Binding (MariaDB)"
			if envType == "docker" {
				wizardTitle = "Step 1/4: Choose Access Scope (Docker MariaDB Stack)"
			}

			options := []string{
				"Localhost Only (127.0.0.1 - Maximum Security for local stacks/apps)",
				"Local Network / LAN (0.0.0.0 - Allow remote DB connections across LAN)",
			}

			idx, _, status := tui.SelectMenu(wizardTitle, options)
			if status != 0 {
				return
			}
			if idx == 0 {
				accessScope = "local"
			} else {
				accessScope = "network"
			}
			currentStep = 2

		case 2:
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Database Network Port Configuration" + core.NC,
				"",
				"  Specify the TCP port for MariaDB database connections.",
				"  " + core.BOLD + "Default Port:" + core.NC + " " + core.CYAN + "3306" + core.NC,
				"  Automatic conflict detection will verify port availability.",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: Port Selection", cardLines)

			portIn, st := tui.ReadTextInput("MariaDB Port", port)
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
			currentStep = 3

		case 3:
			tui.ClearScreen()
			credOptions := []string{
				"Generate Secure Random Password (Recommended)",
				"Enter Custom Administrative Password",
			}
			cIdx, _, cStatus := tui.SelectMenu("Step 3/4: Database Root Administrative Credentials", credOptions)
			if cStatus == 1 || cStatus == 2 {
				currentStep = 2
				continue
			}

			if cIdx == 0 {
				if !isCustomRootPass {
					genPass, gErr := modules.GenerateRandomPass(16)
					if gErr != nil {
						logger.LogError(fmt.Sprintf("Failed to generate root password: %v", gErr))
						return
					}
					rootPassword = genPass
				}
			} else {
				tui.ClearScreen()
				passCardLines := []string{
					core.BOLD + core.CYAN + "Administrative Root Password Setup" + core.NC,
					"",
					"  Enter a secure administrative root password for MariaDB.",
					"  Input is masked with bullets for privacy.",
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back", "Step 3/4: Root Password", passCardLines)

				passIn, pSt := modules.ReadPasswordInput("Enter MariaDB Root Password")
				if pSt != 0 || passIn == "" {
					continue
				}
				rootPassword = passIn
				isCustomRootPass = true
			}

			// Step 3.2: Optional Application Database & User
			tui.ClearScreen()
			appPromptLines := []string{
				core.BOLD + core.CYAN + "Application Database & User Provisioning (Optional)" + core.NC,
				"",
				"  You can automatically create a dedicated database and non-root user",
				"  configured with full privileges for your application or web framework.",
				"",
				core.BOLD + core.YELLOW + "Provision a dedicated application database and user now?" + core.NC,
			}
			appIdx, _, appStatus := tui.SelectMenuWithContent("Step 3/4: Application Database Setup", appPromptLines, []string{"YES", "NO"})
			if appStatus == 1 || appStatus == 2 {
				currentStep = 2
				continue
			}

			createAppDB = (appIdx == 0)
			if createAppDB {
				tui.ClearScreen()
				appDBCardLines := []string{
					core.BOLD + core.CYAN + "Application Database Parameters" + core.NC,
					"",
					"  Provide database name and dedicated user credentials.",
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back", "Step 3/4: Application DB Parameters", appDBCardLines)

				dbNameIn, st := tui.ReadTextInput("Database Name", appDBName)
				if st != 0 {
					currentStep = 2
					continue
				}
				if dbNameIn != "" {
					appDBName = dbNameIn
				}

				dbUserIn, st2 := tui.ReadTextInput("Database Username", appDBUser)
				if st2 != 0 {
					currentStep = 2
					continue
				}
				if dbUserIn != "" {
					appDBUser = dbUserIn
				}

				dbPassIn, st3 := tui.ReadPasswordInput("Database User Password (blank for default)")
				if st3 != 0 {
					currentStep = 2
					continue
				}
				if dbPassIn != "" {
					appDBPass = dbPassIn
				}
			}

			currentStep = 4

		case 4:
			tui.ClearScreen()
			summaryLines := []string{
				tui.FormatKV("Deployment Target", func() string {
					if envType == "docker" {
						return "Docker Compose Container Stack"
					}
					return "Native Linux Service"
				}(), core.GREEN),
				tui.FormatKV("Access Scope", func() string {
					if accessScope == "network" {
						return "Local Network / LAN (0.0.0.0)"
					}
					return "Localhost Only (127.0.0.1)"
				}(), core.GREEN),
				tui.FormatKV("Database Port", port, core.CYAN),
				tui.FormatKV("Root User", "root", core.CYAN),
				tui.FormatKV("Root Password", rootPassword, core.YELLOW),
			}

			if createAppDB {
				summaryLines = append(summaryLines, "")
				summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"APPLICATION DATABASE:"+core.NC)
				summaryLines = append(summaryLines, tui.FormatKV("Database Name", appDBName, core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("Database User", appDBUser, core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("User Password", appDBPass, core.YELLOW))
			}

			if envType == "native" {
				summaryLines = append(summaryLines, "")
				summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PERFORMANCE & STORAGE:"+core.NC)
				summaryLines = append(summaryLines, tui.FormatKV("Data Directory", "/var/lib/mysql", core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("InnoDB Buffer Pool", "Auto-scaled to System RAM (50% RAM)", core.GREEN))
				summaryLines = append(summaryLines, tui.FormatKV("Config Path", "/etc/mysql/my.cnf", core.CYAN))
			} else {
				summaryLines = append(summaryLines, "")
				summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"DOCKER STORAGE & STACK:"+core.NC)
				summaryLines = append(summaryLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("Data Volume", "mariadb_data -> /var/lib/mysql", core.CYAN))
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
				ExecuteNativeInstall(accessScope, port, rootPassword, createAppDB, appDBName, appDBUser, appDBPass)
			} else {
				ExecuteDockerInstall(accessScope, port, rootPassword, createAppDB, appDBName, appDBUser, appDBPass, composeDir)
			}
			return
		}
	}
}

// ExecuteNativeInstall installs MariaDB server natively on the host OS.
func ExecuteNativeInstall(accessScope, port, rootPassword string, createAppDB bool, appDBName, appDBUser, appDBPass string) {
	// Mutual conflict check: Check if Docker MariaDB container is active
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-mariadb").Output(); err == nil && strings.TrimSpace(string(out)) == "running" {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "CONTAINER CONFLICT DETECTED" + core.NC,
			"",
			"  Docker MariaDB Container 'krotek-mariadb' is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Native MariaDB, the Docker container stack must be stopped",
			"  to free network ports (Port " + port + ").",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop Docker MariaDB container stack to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Container Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Docker MariaDB container and continue Native deployment?")
		if res != 0 {
			logger.LogInfo("Native installation cancelled by user due to container conflict.")
			return
		}
		_ = exec.Command("docker", "stop", "krotek-mariadb").Run()
	}

	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}

	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing MariaDB Server (Native System Service) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native MariaDB deployment...")

	// 1. Package Installation
	var pkgs []string
	switch core.SysInfo.Family {
	case "debian":
		pkgs = []string{"mariadb-server", "mariadb-client"}
	case "rhel":
		pkgs = []string{"mariadb-server", "mariadb"}
	case "alpine":
		pkgs = []string{"mariadb", "mariadb-client", "mariadb-server-utils", "mariadb-openrc"}
	default:
		pkgs = []string{"mariadb-server", "mariadb-client"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogError("Failed to install MariaDB system packages.")
		return
	}

	// 2. Alpine-specific data directory init
	if core.SysInfo.Family == "alpine" {
		if _, err := os.Stat("/var/lib/mysql/mysql"); os.IsNotExist(err) {
			_ = system.RunStep("Initializing MariaDB system database", "mariadb-install-db", "--user=mysql", "--datadir=/var/lib/mysql")
		}
		_ = os.MkdirAll("/run/mysqld", 0755)
		_ = exec.Command("chown", "-R", "mysql:mysql", "/var/lib/mysql", "/run/mysqld").Run()
	}

	// 3. Service Enablement
	if core.SysInfo.InitSystem == "systemd" {
		system.InitMariaDBDatadir()
		if !system.StartService("mariadb") {
			_ = system.StartService("mysqld")
		}
	} else {
		_ = os.MkdirAll("/run/mysqld", 0755)
		_ = exec.Command("chown", "-R", "mysql:mysql", "/run/mysqld").Run()
		_ = system.RunStep("Enabling MariaDB in OpenRC", "rc-update", "add", "mariadb", "default")
		_ = system.RunStep("Starting MariaDB in OpenRC", "rc-service", "mariadb", "restart")
		_ = exec.Command("rc-service", "mariadb", "start").Run()
	}

	// Wait up to 15s for MariaDB socket to accept connections
	for i := 0; i < 30; i++ {
		if exec.Command("mariadb-admin", "ping").Run() == nil ||
			exec.Command("mariadb-admin", "-u", "root", "ping").Run() == nil ||
			exec.Command("mysqladmin", "ping").Run() == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// 4. Determine Dynamic InnoDB Buffer Pool (50% of system RAM, minimum 256M)
	bufferPoolSize := "512M"
	if memData, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(memData), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if totalKb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
						totalMb := totalKb / 1024
						poolMb := totalMb / 2
						if poolMb < 256 {
							poolMb = 256
						}
						bufferPoolSize = fmt.Sprintf("%dM", poolMb)
					}
				}
				break
			}
		}
	}

	// 5. Write Hardened & Tuned Configurations
	bindAddr := "127.0.0.1"
	if accessScope == "network" {
		bindAddr = "0.0.0.0"
	}

	confDir := "/etc/mysql/conf.d"
	if core.SysInfo.Family != "debian" {
		confDir = "/etc/my.cnf.d"
	}
	_ = os.MkdirAll(confDir, 0755)

	if core.SysInfo.Family != "debian" {
		if _, err := os.Stat("/etc/my.cnf"); os.IsNotExist(err) {
			_ = system.WriteFileNoFollow("/etc/my.cnf", []byte("# Global MariaDB Configuration - Managed by Krotek\n[client-server]\n!includedir /etc/my.cnf.d\n"), 0644, "")
		}
	}

	perfConf := fmt.Sprintf(`# /etc/mysql/conf.d/performance.cnf
# Auto-tuned for Database performance by Krotek MariaDB Installer

[mysqld]
bind-address = %s
port = %s

# Dynamic Memory Tuning based on System RAM
innodb_buffer_pool_size = %s
innodb_buffer_pool_instances = 1
innodb_log_file_size = 128M
innodb_log_buffer_size = 16M
innodb_flush_log_at_trx_commit = 2
innodb_flush_method = O_DIRECT

# Resource Optimization
max_connections = 150
key_buffer_size = 64M
max_allowed_packet = 64M
thread_cache_size = 16
table_open_cache = 2048
`, bindAddr, port, bufferPoolSize)

	_ = system.WriteFileNoFollow(filepath.Join(confDir, "performance.cnf"), []byte(perfConf), 0644, "")

	// 6. Security Hardening & Root Password Configuration
	escapedRootPass := strings.ReplaceAll(strings.ReplaceAll(rootPassword, "\\", "\\\\"), "'", "''")
	hardeningSQL := fmt.Sprintf(`
ALTER USER 'root'@'localhost' IDENTIFIED BY '%s';
GRANT ALL PRIVILEGES ON *.* TO 'root'@'localhost' WITH GRANT OPTION;
DELETE FROM mysql.user WHERE User='';
DROP DATABASE IF EXISTS test;
DELETE FROM mysql.db WHERE Db='test' OR Db='test\_%%%%';
FLUSH PRIVILEGES;
`, escapedRootPass)

	if createAppDB && appDBName != "" && appDBUser != "" {
		escapedAppPass := strings.ReplaceAll(strings.ReplaceAll(appDBPass, "\\", "\\\\"), "'", "''")
		escapedAppUser := strings.ReplaceAll(strings.ReplaceAll(appDBUser, "\\", "\\\\"), "'", "''")
		escapedDBName := strings.ReplaceAll(strings.ReplaceAll(appDBName, "`", ""), ";", "")
		hardeningSQL += fmt.Sprintf(`
CREATE DATABASE IF NOT EXISTS `+"`%s`"+` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS '%s'@'%%%%' IDENTIFIED BY '%s';
GRANT ALL PRIVILEGES ON `+"`%s`"+`.* TO '%s'@'%%%%';
FLUSH PRIVILEGES;
`, escapedDBName, escapedAppUser, escapedAppPass, escapedDBName, escapedAppUser)
	}

	tmpDir, cnfFile, cErr := system.BuildMySQLClientCnf("root", rootPassword)
	if cErr != nil {
		logger.LogError(fmt.Sprintf("Failed to prepare secure credential directory: %v", cErr))
		return
	}
	defer os.RemoveAll(tmpDir)

	for _, bin := range []string{"mariadb", "mysql"} {
		if !system.CommandExists(bin) {
			continue
		}
		var applied bool
		for _, args := range [][]string{
			{"-u", "root"},
			{},
			{"--defaults-extra-file=" + cnfFile, "-u", "root"},
		} {
			c := exec.Command(bin, args...)
			c.Stdin = strings.NewReader(hardeningSQL)
			if c.Run() == nil {
				applied = true
				break
			}
		}
		if applied {
			break
		}
	}

	// 7. Restart Service to apply network bindings and tuning
	if core.SysInfo.InitSystem == "systemd" {
		system.RestartService("mariadb")
	} else {
		_ = system.RestartService("mariadb")
	}

	// 8. Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	if accessScope == "network" {
		system.ConfigureFirewallRules(port)
	}

	// 9. Save State & Global Shortcut
	stateData := map[string]string{
		"ACTION":       "installed",
		"ENV_TYPE":     "native",
		"ACCESS_SCOPE": accessScope,
		"PORT":         port,
		"ADMIN_PASS":   rootPassword,
		"APP_DB":       appDBName,
		"APP_USER":     appDBUser,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	// 10. Display Success Box
	localIP := system.GetLocalIP()
	summaryLines := []string{
		tui.FormatKV("Deployment Mode", "Native Linux System Service", core.GREEN),
		tui.FormatKV("Access Scope", func() string {
			if accessScope == "network" {
				return fmt.Sprintf("LAN Network (Bound to %s:%s)", localIP, port)
			}
			return fmt.Sprintf("Localhost Only (Bound to 127.0.0.1:%s)", port)
		}(), core.GREEN),
		tui.FormatKV("Localhost CLI Login", fmt.Sprintf("mariadb -u root -p -P %s", port), core.CYAN),
	}

	if accessScope == "network" {
		summaryLines = append(summaryLines, tui.FormatKV("Remote Network Login", fmt.Sprintf("mariadb -h %s -P %s -u root -p", localIP, port), core.GREEN))
	}

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"CREDENTIALS & SECURITY:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Administrative User", "root", core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Root Password", rootPassword, core.YELLOW+core.BOLD))

	if createAppDB {
		summaryLines = append(summaryLines, tui.FormatKV("App Database", appDBName, core.CYAN))
		summaryLines = append(summaryLines, tui.FormatKV("App User", appDBUser, core.CYAN))
		summaryLines = append(summaryLines, tui.FormatKV("App Password", appDBPass, core.YELLOW))
	}

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Performance Config", filepath.Join(confDir, "performance.cnf"), core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Data Directory", "/var/lib/mysql", core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Installer Log", core.LogFilePath, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "mariadb-manager", core.GREEN))

	tui.EnterAltScreen()
	tui.ClearScreen()
	tui.PrintInBox("MariaDB Native Stack Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Native MariaDB installation completed.")
}

// ExecuteDockerInstall provisions MariaDB via Docker Compose.
func ExecuteDockerInstall(accessScope, port, rootPassword string, createAppDB bool, appDBName, appDBUser, appDBPass, composeDir string) {
	// Mutual conflict check: Check if Native MariaDB service is active
	if exec.Command("systemctl", "is-active", "--quiet", "mariadb").Run() == nil || exec.Command("systemctl", "is-active", "--quiet", "mysqld").Run() == nil {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "SERVICE CONFLICT DETECTED" + core.NC,
			"",
			"  Native MariaDB / MySQL Linux Service is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Docker MariaDB, the Native service must be stopped & disabled",
			"  to free network ports (Port " + port + ").",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop & Disable Native MariaDB service to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Service Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Native MariaDB service and continue Docker deployment?")
		if res != 0 {
			logger.LogInfo("Docker installation cancelled by user due to service conflict.")
			return
		}
		_ = exec.Command("systemctl", "stop", "mariadb").Run()
		_ = exec.Command("systemctl", "stop", "mysqld").Run()
		_ = exec.Command("systemctl", "disable", "mariadb").Run()
		_ = exec.Command("systemctl", "disable", "mysqld").Run()
	}

	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}

	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing MariaDB (Docker Container Stack) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker MariaDB deployment...")

	if !system.EnsureDockerEngine() {
		return
	}

	_ = os.MkdirAll(composeDir, 0755)
	confDir := filepath.Join(composeDir, "conf.d")
	_ = os.MkdirAll(confDir, 0755)

	customCnf := `[mysqld]
character-set-server = utf8mb4
collation-server = utf8mb4_unicode_ci
max_connections = 150
innodb_buffer_pool_size = 512M
`
	_ = system.WriteFileNoFollow(filepath.Join(confDir, "custom.cnf"), []byte(customCnf), 0644, "")

	portBinding := fmt.Sprintf(`"%s:3306"`, port)
	if accessScope == "local" {
		portBinding = fmt.Sprintf(`"127.0.0.1:%s:3306"`, port)
	}

	envVars := fmt.Sprintf(`      MYSQL_ROOT_PASSWORD: "%s"`, rootPassword)
	if createAppDB && appDBName != "" {
		envVars += fmt.Sprintf("\n      MYSQL_DATABASE: \"%s\"\n      MYSQL_USER: \"%s\"\n      MYSQL_PASSWORD: \"%s\"",
			appDBName, appDBUser, appDBPass)
	}

	composeYaml := fmt.Sprintf(`version: '3.8'

services:
  mariadb:
    image: mariadb:lts
    container_name: krotek-mariadb
    restart: unless-stopped
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
    ports:
      - %s
    environment:
%s
    volumes:
      - mariadb_data:/var/lib/mysql
      - ./conf.d:/etc/mysql/conf.d:ro
    healthcheck:
      test: ["CMD-SHELL", "healthcheck.sh --connect --innodb_initialized || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3

volumes:
  mariadb_data:
    name: krotek_mariadb_data
`, portBinding, envVars)

	composeFile := filepath.Join(composeDir, "docker-compose.yml")
	if err := system.WriteFileNoFollow(composeFile, []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write compose file: %v", err))
		return
	}

	if !system.RunDockerCompose(composeDir, "Deploying MariaDB Docker Stack", true, "up", "-d") {
		return
	}

	// Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	if accessScope == "network" {
		system.ConfigureFirewallRules(port)
	}

	// Save State & Shortcut
	stateData := map[string]string{
		"ACTION":       "installed",
		"ENV_TYPE":     "docker",
		"COMPOSE_DIR":  composeDir,
		"ACCESS_SCOPE": accessScope,
		"PORT":         port,
		"ADMIN_PASS":   rootPassword,
		"APP_DB":       appDBName,
		"APP_USER":     appDBUser,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	// Display Success Box
	localIP := system.GetLocalIP()
	summaryLines := []string{
		tui.FormatKV("Deployment Mode", "Docker Compose Container Stack", core.GREEN),
		tui.FormatKV("Container Name", "krotek-mariadb (mariadb:11.4)", core.GREEN),
		tui.FormatKV("Port Binding", func() string {
			if accessScope == "network" {
				return fmt.Sprintf("0.0.0.0:%s -> 3306/tcp", port)
			}
			return fmt.Sprintf("127.0.0.1:%s -> 3306/tcp", port)
		}(), core.GREEN),
		tui.FormatKV("Local Connection", fmt.Sprintf("mariadb -h 127.0.0.1 -P %s -u root -p", port), core.CYAN),
	}

	if accessScope == "network" {
		summaryLines = append(summaryLines, tui.FormatKV("Remote Connection", fmt.Sprintf("mariadb -h %s -P %s -u root -p", localIP, port), core.GREEN))
	}

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"CREDENTIALS & SECURITY:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Administrative User", "root", core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Root Password", rootPassword, core.YELLOW+core.BOLD))

	if createAppDB {
		summaryLines = append(summaryLines, tui.FormatKV("App Database", appDBName, core.CYAN))
		summaryLines = append(summaryLines, tui.FormatKV("App User", appDBUser, core.CYAN))
		summaryLines = append(summaryLines, tui.FormatKV("App Password", appDBPass, core.YELLOW))
	}

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Docker Volume", "krotek_mariadb_data", core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Installer Log", core.LogFilePath, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "mariadb-manager", core.GREEN))

	tui.EnterAltScreen()
	tui.ClearScreen()
	tui.PrintInBox("MariaDB Docker Stack Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Docker MariaDB installation completed.")
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
