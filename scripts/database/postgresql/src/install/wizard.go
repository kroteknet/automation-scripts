// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: PostgreSQL Advanced Object-Relational Database (postgresql)
//
// This package orchestrates automated pre-flight checks, package repository setup,
// configuration templating, service enablement (systemd/openrc), and firewall rules.
// Supports both interactive TUI step-by-step guidance and headless automation.
// ==============================================================================

package install

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"krotek-postgresql-installer/core"
	"krotek-postgresql-installer/logger"
	"krotek-postgresql-installer/modules"
	"krotek-postgresql-installer/system"
	"krotek-postgresql-installer/tui"
)

// FreshInstallWizard handles the end-to-end installation pipeline for PostgreSQL Advanced Object-Relational Database.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	envType := targetEnv
	if envType == "" {
		envType = "native"
	}

	accessScope := "local" // "local" (127.0.0.1) or "network" (0.0.0.0)
	port := "5432"
	postgresPassword, pErr := modules.GenerateRandomPass(16)
	if pErr != nil {
		logger.LogError(fmt.Sprintf("Failed to generate initial postgres password: %v", pErr))
		tui.PrintInBox("Security Error", []string{core.RED + "[ERROR] CSPRNG failure generating superuser password." + core.NC})
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

	composeDir := "/etc/postgresql-docker"

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
			postgresPassword = envPass
		} else if envAdminPass := os.Getenv("KROTEK_ADMIN_PASS"); envAdminPass != "" {
			postgresPassword = envAdminPass
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

		logger.LogInfo(fmt.Sprintf("Starting Unattended PostgreSQL installation: mode=%s, port=%s, scope=%s", envType, port, accessScope))
		if envType == "native" {
			ExecuteNativeInstall(accessScope, port, postgresPassword, createAppDB, appDBName, appDBUser, appDBPass)
		} else {
			ExecuteDockerInstall(accessScope, port, postgresPassword, createAppDB, appDBName, appDBUser, appDBPass, composeDir)
		}
		return
	}

	PromptExistingConfigAction("PostgreSQL Database Server", "/etc/postgresql")

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/4: Choose Access Scope & Network Binding (PostgreSQL)"
			if envType == "docker" {
				wizardTitle = "Step 1/4: Choose Access Scope (Docker PostgreSQL Stack)"
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
				core.BOLD + core.CYAN + "PostgreSQL Network Port Configuration" + core.NC,
				"",
				"  Specify the TCP port for PostgreSQL database connections.",
				"  " + core.BOLD + "Default Port:" + core.NC + " " + core.CYAN + "5432" + core.NC,
				"  Automatic conflict detection will verify port availability.",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: Port Selection", cardLines)

			portIn, st := tui.ReadTextInput("PostgreSQL Port", port)
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
			cIdx, _, cStatus := tui.SelectMenu("Step 3/4: PostgreSQL 'postgres' User Credentials", credOptions)
			if cStatus == 1 || cStatus == 2 {
				currentStep = 2
				continue
			}

			if cIdx == 0 {
				if !isCustomRootPass {
					genPass, gErr := modules.GenerateRandomPass(16)
					if gErr != nil {
						logger.LogError(fmt.Sprintf("Failed to generate superuser password: %v", gErr))
						return
					}
					postgresPassword = genPass
				}
			} else {
				tui.ClearScreen()
				passCardLines := []string{
					core.BOLD + core.CYAN + "Administrative 'postgres' Password Setup" + core.NC,
					"",
					"  Enter a secure administrative password for the 'postgres' superuser.",
					"  Input is masked with bullets for privacy.",
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back", "Step 3/4: Superuser Password", passCardLines)

				passIn, pSt := modules.ReadPasswordInput("Enter 'postgres' User Password")
				if pSt != 0 || passIn == "" {
					continue
				}
				postgresPassword = passIn
				isCustomRootPass = true
			}

			// Step 3.2: Optional Application Database & User
			tui.ClearScreen()
			appPromptLines := []string{
				core.BOLD + core.CYAN + "Application Database & User Provisioning (Optional)" + core.NC,
				"",
				"  You can automatically create a dedicated database and non-superuser role",
				"  configured with ownership privileges for your application.",
				"",
				core.BOLD + core.YELLOW + "Provision a dedicated application database and user role now?" + core.NC,
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
					"  Provide database name and dedicated role credentials.",
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
				tui.FormatKV("Superuser Role", "postgres", core.CYAN),
				tui.FormatKV("Superuser Password", postgresPassword, core.YELLOW),
			}

			if createAppDB {
				summaryLines = append(summaryLines, "")
				summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"APPLICATION DATABASE:"+core.NC)
				summaryLines = append(summaryLines, tui.FormatKV("Database Name", appDBName, core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("Database Role", appDBUser, core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("Role Password", appDBPass, core.YELLOW))
			}

			if envType == "native" {
				summaryLines = append(summaryLines, "")
				summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PERFORMANCE & STORAGE:"+core.NC)
				summaryLines = append(summaryLines, tui.FormatKV("Data Directory", "/var/lib/postgresql", core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("Shared Buffers", "Auto-scaled (25% host RAM)", core.GREEN))
			} else {
				summaryLines = append(summaryLines, "")
				summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"DOCKER STORAGE & STACK:"+core.NC)
				summaryLines = append(summaryLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("Data Volume", "postgres_data -> /var/lib/postgresql/data", core.CYAN))
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
				ExecuteNativeInstall(accessScope, port, postgresPassword, createAppDB, appDBName, appDBUser, appDBPass)
			} else {
				ExecuteDockerInstall(accessScope, port, postgresPassword, createAppDB, appDBName, appDBUser, appDBPass, composeDir)
			}
			return
		}
	}
}

// ExecuteNativeInstall installs PostgreSQL server natively on the host OS.
func ExecuteNativeInstall(accessScope, port, postgresPassword string, createAppDB bool, appDBName, appDBUser, appDBPass string) {
	// Mutual conflict check: Check if Docker PostgreSQL container is active
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-postgres").Output(); err == nil && strings.TrimSpace(string(out)) == "running" {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "CONTAINER CONFLICT DETECTED" + core.NC,
			"",
			"  Docker PostgreSQL Container 'krotek-postgres' is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Native PostgreSQL, the Docker container stack must be stopped",
			"  to free network ports (Port " + port + ").",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop Docker PostgreSQL container stack to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Container Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Docker PostgreSQL container and continue Native deployment?")
		if res != 0 {
			logger.LogInfo("Native installation cancelled by user due to container conflict.")
			return
		}
		_ = exec.Command("docker", "stop", "krotek-postgres").Run()
	}

	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}

	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing PostgreSQL Server (Native System Service) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native PostgreSQL deployment...")

	// 1. Package Installation
	if core.SysInfo.Family == "alpine" {
		// Dynamically detect available PostgreSQL packages on Alpine without hardcoding version numbers
		apkCmd := `apk update -q && {
	if apk add --no-cache postgresql postgresql-contrib postgresql-common-openrc 2>/dev/null; then
		true
	elif apk add --no-cache postgresql postgresql-contrib postgresql-openrc 2>/dev/null; then
		true
	else
		LATEST_PG=$(apk search -q 'postgresql*' 2>/dev/null | grep -E '^postgresql[0-9]+$' | sort -V | tail -n 1)
		if [ -n "$LATEST_PG" ]; then
			apk add --no-cache "$LATEST_PG" "${LATEST_PG}-contrib" "${LATEST_PG}-openrc" postgresql-common-openrc 2>/dev/null || \
			apk add --no-cache "$LATEST_PG" "${LATEST_PG}-contrib" "${LATEST_PG}-openrc" 2>/dev/null || \
			apk add --no-cache "$LATEST_PG" "${LATEST_PG}-contrib"
		else
			apk add --no-cache postgresql postgresql-contrib
		fi
	fi
}`
		if !system.RunMultiStep("Installing PostgreSQL system packages", apkCmd) {
			logger.LogError("Failed to install PostgreSQL system packages on Alpine.")
			return
		}
	} else {
		var pkgs []string
		switch core.SysInfo.Family {
		case "debian":
			pkgs = []string{"postgresql", "postgresql-contrib"}
		case "rhel":
			pkgs = []string{"postgresql-server", "postgresql"}
		default:
			pkgs = []string{"postgresql", "postgresql-contrib"}
		}

		if !system.InstallPackages(pkgs...) {
			logger.LogError("Failed to install PostgreSQL system packages.")
			return
		}
	}

	// Ensure PostgreSQL system directories and permissions exist
	_ = os.MkdirAll("/var/lib/postgresql", 0750)
	_ = os.MkdirAll("/var/log/postgresql", 0755)
	_ = os.MkdirAll("/run/postgresql", 0775)
	_ = exec.Command("chown", "-R", "postgres:postgres", "/var/lib/postgresql", "/var/log/postgresql", "/run/postgresql").Run()

	// 2. RHEL/Alpine specific initdb & setup
	if core.SysInfo.Family == "rhel" {
		_ = exec.Command("postgresql-setup", "--initdb").Run()
	} else if core.SysInfo.Family == "alpine" {
		// Ensure OpenRC init script is available at /etc/init.d/postgresql
		if _, err := os.Stat("/etc/init.d/postgresql"); os.IsNotExist(err) {
			if matches, gErr := filepath.Glob("/etc/init.d/postgresql*"); gErr == nil {
				for _, s := range matches {
					if fi, statErr := os.Stat(s); statErr == nil && fi.Mode().IsRegular() && !system.IsSymlinkPath("/etc/init.d/postgresql") {
						_ = os.Symlink(s, "/etc/init.d/postgresql")
						break
					}
				}
			}
			if _, err := os.Stat("/etc/init.d/postgresql"); os.IsNotExist(err) {
				_ = exec.Command("apk", "add", "--no-cache", "postgresql-common-openrc").Run()
			}
		}

		// Initialize database cluster via OpenRC setup action or initdb
		setupCmd := "/etc/init.d/postgresql setup 2>/dev/null || rc-service postgresql setup 2>/dev/null || (" +
			"su -s /bin/sh postgres -c '/usr/bin/initdb -D /var/lib/postgresql/data' 2>/dev/null) || true"
		_ = system.RunMultiStep("Initializing PostgreSQL database cluster", setupCmd)
	}

	// 3. Service Enablement & Starting
	if !system.StartService("postgresql") {
		_ = system.StartService("postgres")
	}

	// Wait up to 10 seconds for PostgreSQL daemon to accept connections
	for i := 0; i < 10; i++ {
		if exec.Command(system.GetShell(), "-c", "pg_isready 2>/dev/null || su -s /bin/sh postgres -c 'pg_isready' 2>/dev/null").Run() == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// 4. Determine Dynamic shared_buffers (25% of system RAM, minimum 128M)
	sharedBuffers := "256MB"
	if memData, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(memData), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if totalKb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
						totalMb := totalKb / 1024
						bufMb := totalMb / 4
						if bufMb < 128 {
							bufMb = 128
						}
						sharedBuffers = fmt.Sprintf("%dMB", bufMb)
					}
				}
				break
			}
		}
	}

	// 5. Apply Configuration & Network Listeners via ALTER SYSTEM
	listenAddr := "localhost"
	if accessScope == "network" {
		listenAddr = "*"
	}

	alterCmd := fmt.Sprintf(`
ALTER SYSTEM SET listen_addresses = '%s';
ALTER SYSTEM SET port = %s;
ALTER SYSTEM SET shared_buffers = '%s';
ALTER SYSTEM SET work_mem = '16MB';
ALTER SYSTEM SET maintenance_work_mem = '64MB';
ALTER USER postgres WITH PASSWORD '%s';
`, listenAddr, port, sharedBuffers, strings.ReplaceAll(postgresPassword, "'", "''"))

	if createAppDB && appDBName != "" && appDBUser != "" {
		safeAppUser := regexp.MustCompile(`[^a-zA-Z0-9_]`).ReplaceAllString(appDBUser, "")
		safeDBName := regexp.MustCompile(`[^a-zA-Z0-9_]`).ReplaceAllString(appDBName, "")
		if safeAppUser != "" && safeDBName != "" {
			cleanPass := strings.ReplaceAll(appDBPass, "$krotek$", "")
			escapedAppPass := strings.ReplaceAll(cleanPass, "'", "''")
			alterCmd += fmt.Sprintf(`
DO $krotek$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '%s') THEN
    CREATE ROLE "%s" WITH LOGIN PASSWORD '%s';
  END IF;
END
$krotek$;
CREATE DATABASE "%s" OWNER "%s";
GRANT ALL PRIVILEGES ON DATABASE "%s" TO "%s";
`, safeAppUser, safeAppUser, escapedAppPass, safeDBName, safeAppUser, safeDBName, safeAppUser)
		}
	}

	tmpDir, err := os.MkdirTemp("", "krotek-pg-*")
	if err != nil {
		logger.LogError(fmt.Sprintf("Failed to create private temporary directory for PostgreSQL setup: %v", err))
		return
	}
	_ = os.Chmod(tmpDir, 0700)
	_ = exec.Command("chown", "postgres:postgres", tmpDir).Run()
	defer os.RemoveAll(tmpDir)

	sqlFile := filepath.Join(tmpDir, "init.sql")
	if wErr := system.WriteFileNoFollow(sqlFile, []byte(alterCmd), 0600, "postgres"); wErr != nil {
		logger.LogError(fmt.Sprintf("Failed to write PostgreSQL setup SQL safely: %v", wErr))
		return
	}
	defer os.Remove(sqlFile)

	portNum, portErr := strconv.Atoi(port)
	if portErr != nil || portNum < 1 || portNum > 65535 {
		portNum = 5432
	}
	portStr := strconv.Itoa(portNum)
	escapedSQLFile := strings.ReplaceAll(sqlFile, "'", "'\\''")
	for _, argv := range [][]string{
		{"su", "-s", "/bin/sh", "postgres", "-c", fmt.Sprintf("psql -f '%s'", escapedSQLFile)},
		{"su", "-s", "/bin/sh", "postgres", "-c", fmt.Sprintf("psql -p %s -f '%s'", portStr, escapedSQLFile)},
		{"psql", "-U", "postgres", "-p", portStr, "-f", sqlFile},
	} {
		if exec.Command(argv[0], argv[1:]...).Run() == nil {
			break
		}
	}

	// Allow LAN connections in pg_hba.conf if network mode
	if accessScope == "network" {
		_ = AppendPGHBALanRules("/etc/postgresql", "/var/lib/postgresql", "/var/lib/pgsql")
	}

	// Restart Service to apply changes
	if !system.RestartService("postgresql") {
		_ = system.RestartService("postgres")
	}

	// 6. Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	if accessScope == "network" {
		system.ConfigureFirewallRules(port)
	}

	// 7. Save State & Global Shortcut
	stateData := map[string]string{
		"ACTION":       "installed",
		"ENV_TYPE":     "native",
		"ACCESS_SCOPE": accessScope,
		"PORT":         port,
		"ADMIN_PASS":   postgresPassword,
		"APP_DB":       appDBName,
		"APP_USER":     appDBUser,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	// 8. Display Success Box
	localIP := system.GetLocalIP()
	summaryLines := []string{
		tui.FormatKV("Deployment Mode", "Native Linux System Service", core.GREEN),
		tui.FormatKV("Access Scope", func() string {
			if accessScope == "network" {
				return fmt.Sprintf("LAN Network (Bound to %s:%s)", localIP, port)
			}
			return fmt.Sprintf("Localhost Only (Bound to 127.0.0.1:%s)", port)
		}(), core.GREEN),
		tui.FormatKV("Localhost CLI Login", fmt.Sprintf("su - postgres -c 'psql -p %s'", port), core.CYAN),
	}

	if accessScope == "network" {
		summaryLines = append(summaryLines, tui.FormatKV("Remote Network Login", fmt.Sprintf("psql -h %s -p %s -U postgres", localIP, port), core.GREEN))
	}

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"CREDENTIALS & ROLES:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Superuser Role", "postgres", core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Superuser Password", postgresPassword, core.YELLOW+core.BOLD))

	if createAppDB {
		summaryLines = append(summaryLines, tui.FormatKV("App Database", appDBName, core.CYAN))
		summaryLines = append(summaryLines, tui.FormatKV("App Role / User", appDBUser, core.CYAN))
		summaryLines = append(summaryLines, tui.FormatKV("Role Password", appDBPass, core.YELLOW))
	}

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Data Root", "/var/lib/postgresql", core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Installer Log", core.LogFilePath, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "postgresql-manager", core.GREEN))

	tui.EnterAltScreen()
	tui.ClearScreen()
	tui.PrintInBox("PostgreSQL Native Stack Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Native PostgreSQL installation completed.")
}

// ExecuteDockerInstall provisions PostgreSQL via Docker Compose.
func ExecuteDockerInstall(accessScope, port, postgresPassword string, createAppDB bool, appDBName, appDBUser, appDBPass, composeDir string) {
	// Mutual conflict check: Check if Native PostgreSQL service is active
	isNativeActive := exec.Command("systemctl", "is-active", "--quiet", "postgresql").Run() == nil ||
		exec.Command("systemctl", "is-active", "--quiet", "postgres").Run() == nil ||
		exec.Command("rc-service", "postgresql", "status").Run() == nil ||
		exec.Command("pidof", "postgres").Run() == nil

	if isNativeActive {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "SERVICE CONFLICT DETECTED" + core.NC,
			"",
			"  Native PostgreSQL Linux Service is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Docker PostgreSQL, the Native service must be stopped & disabled",
			"  to free network ports (Port " + port + ").",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop & Disable Native PostgreSQL service to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Service Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Native PostgreSQL service and continue Docker deployment?")
		if res != 0 {
			logger.LogInfo("Docker installation cancelled by user due to service conflict.")
			return
		}
		_ = exec.Command("systemctl", "stop", "postgresql").Run()
		_ = exec.Command("systemctl", "stop", "postgres").Run()
		_ = exec.Command("systemctl", "disable", "postgresql").Run()
		_ = exec.Command("systemctl", "disable", "postgres").Run()
		_ = exec.Command("rc-service", "postgresql", "stop").Run()
		_ = exec.Command("rc-update", "del", "postgresql", "default").Run()
	}

	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}

	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing PostgreSQL (Docker Container Stack) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker PostgreSQL deployment...")

	if !system.EnsureDockerEngine() {
		return
	}

	_ = os.MkdirAll(composeDir, 0755)

	portBinding := fmt.Sprintf(`"%s:5432"`, port)
	if accessScope == "local" {
		portBinding = fmt.Sprintf(`"127.0.0.1:%s:5432"`, port)
	}

	envVars := fmt.Sprintf(`      POSTGRES_PASSWORD: "%s"`, postgresPassword)
	if createAppDB && appDBName != "" {
		envVars += fmt.Sprintf("\n      POSTGRES_DB: \"%s\"\n      POSTGRES_USER: \"%s\"",
			appDBName, appDBUser)
	}

	composeYaml := fmt.Sprintf(`version: '3.8'

services:
  postgresql:
    image: postgres:alpine
    container_name: krotek-postgres
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
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3

volumes:
  postgres_data:
    name: krotek_postgres_data
`, portBinding, envVars)

	composeFile := filepath.Join(composeDir, "docker-compose.yml")
	if err := system.WriteFileNoFollow(composeFile, []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write compose file: %v", err))
		return
	}

	if !system.RunDockerCompose(composeDir, "Deploying PostgreSQL Docker Stack", true, "up", "-d") {
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
		"ADMIN_PASS":   postgresPassword,
		"APP_DB":       appDBName,
		"APP_USER":     appDBUser,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	// Display Success Box
	localIP := system.GetLocalIP()
	summaryLines := []string{
		tui.FormatKV("Deployment Mode", "Docker Compose Container Stack", core.GREEN),
		tui.FormatKV("Container Name", "krotek-postgres (postgres:alpine)", core.GREEN),
		tui.FormatKV("Port Binding", func() string {
			if accessScope == "network" {
				return fmt.Sprintf("0.0.0.0:%s -> 5432/tcp", port)
			}
			return fmt.Sprintf("127.0.0.1:%s -> 5432/tcp", port)
		}(), core.GREEN),
		tui.FormatKV("Local Connection", fmt.Sprintf("psql -h 127.0.0.1 -p %s -U postgres", port), core.CYAN),
	}

	if accessScope == "network" {
		summaryLines = append(summaryLines, tui.FormatKV("Remote Connection", fmt.Sprintf("psql -h %s -p %s -U postgres", localIP, port), core.GREEN))
	}

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"CREDENTIALS & ROLES:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Superuser Role", "postgres", core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Superuser Password", postgresPassword, core.YELLOW+core.BOLD))

	if createAppDB {
		summaryLines = append(summaryLines, tui.FormatKV("App Database", appDBName, core.CYAN))
		summaryLines = append(summaryLines, tui.FormatKV("App Role / User", appDBUser, core.CYAN))
		summaryLines = append(summaryLines, tui.FormatKV("Role Password", appDBPass, core.YELLOW))
	}

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Docker Volume", "krotek_postgres_data", core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Installer Log", core.LogFilePath, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "postgresql-manager", core.GREEN))

	tui.EnterAltScreen()
	tui.ClearScreen()
	tui.PrintInBox("PostgreSQL Docker Stack Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Docker PostgreSQL installation completed.")
}

// AppendPGHBALanRules safely appends private LAN SCRAM-SHA-256 rules to every regular non-symlink
// pg_hba.conf under roots without invoking a shell.
func AppendPGHBALanRules(roots ...string) error {
	lanCIDRs := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
	for _, root := range roots {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d == nil {
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 || d.IsDir() || d.Name() != "pg_hba.conf" {
				return nil
			}
			if system.IsSymlinkPath(p) {
				return nil
			}
			raw, rErr := os.ReadFile(p)
			if rErr != nil {
				return nil
			}
			content := string(raw)
			var missing strings.Builder
			if len(raw) > 0 && raw[len(raw)-1] != '\n' {
				missing.WriteByte('\n')
			}
			added := false
			for _, cidr := range lanCIDRs {
				if !strings.Contains(content, cidr) {
					missing.WriteString(fmt.Sprintf("host all all %s scram-sha-256\n", cidr))
					added = true
				}
			}
			if added {
				_ = system.AppendFileNoFollow(p, []byte(missing.String()), 0640)
			}
			return nil
		})
	}
	return nil
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
