// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Redis In-Memory Data Structure Store (redis)
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
	"regexp"
	"strconv"
	"strings"
	"time"

	"krotek-redis-installer/core"
	"krotek-redis-installer/logger"
	"krotek-redis-installer/modules"
	"krotek-redis-installer/system"
	"krotek-redis-installer/tui"
)

// FreshInstallWizard handles the end-to-end installation pipeline for Redis In-Memory Data Structure Store.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	envType := targetEnv
	if envType == "" {
		envType = "native"
	}

	accessScope := "local" // "local" (127.0.0.1) or "network" (0.0.0.0)
	port := "6379"
	requirePass := true
	redisPassword, pErr := modules.GenerateRandomPass(20)
	if pErr != nil {
		logger.LogError(fmt.Sprintf("Failed to generate initial Redis password: %v", pErr))
		tui.PrintInBox("Security Error", []string{core.RED + "[ERROR] CSPRNG failure generating Redis password." + core.NC})
		return
	}
	isCustomPass := false

	composeDir := "/etc/redis-docker"

	// ─── UNATTENDED / NON-INTERACTIVE AUTOMATION MODE ─────────────────────────
	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" || os.Getenv("NON_INTERACTIVE") == "1" {
		if envMode := os.Getenv("KROTEK_MODE"); envMode != "" {
			envType = envMode
		}
		if envPort := os.Getenv("KROTEK_PORT"); envPort != "" {
			port = envPort
		}
		logger.LogInfo(fmt.Sprintf("Starting unattended installation: mode=%s", envType))
		if envType == "native" {
			ExecuteNativeInstall(accessScope, port, requirePass, redisPassword)
		} else {
			ExecuteDockerInstall(accessScope, port, requirePass, redisPassword, composeDir)
		}
		return
	}
	PromptExistingConfigAction("Redis In-Memory Data Store", "/etc/redis/redis.conf", "/etc/redis.conf")

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/4: Choose Access Scope & Network Binding (Redis)"
			if envType == "docker" {
				wizardTitle = "Step 1/4: Choose Access Scope (Docker Redis Stack)"
			}

			options := []string{
				"Localhost Only (127.0.0.1 - Maximum Security for local caching/queues)",
				"Local Network / LAN (0.0.0.0 - Allow remote cache/pubsub access across LAN)",
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
				core.BOLD + core.CYAN + "Redis Network Port Configuration" + core.NC,
				"",
				"  Specify the TCP listening port for Redis cache and data connections.",
				"  " + core.BOLD + "Default Port:" + core.NC + " " + core.CYAN + "6379" + core.NC,
				"  Automatic conflict detection will verify port availability.",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: Port Selection", cardLines)

			portIn, st := tui.ReadTextInput("Redis Port", port)
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
			authPromptLines := []string{
				core.BOLD + core.CYAN + "Redis Authentication & Security (requirepass)" + core.NC,
				"",
				"  Redis is extremely fast, making unauthenticated instances vulnerable",
				"  to brute-force attacks and unauthorized data manipulation.",
				"",
				core.BOLD + core.YELLOW + "Require client authentication password?" + core.NC,
			}
			aIdx, _, aStatus := tui.SelectMenuWithContent("Step 3/4: Authentication Policy", authPromptLines, []string{
				"YES (Recommended - Strong Password)",
				"NO (Passwordless - Only safe for local isolated environments)",
			})
			if aStatus == 1 || aStatus == 2 {
				currentStep = 2
				continue
			}

			requirePass = (aIdx == 0)
			if accessScope == "network" && !requirePass {
				tui.ClearScreen()
				tui.PrintInBox("Security Restriction", []string{
					core.RED + core.BOLD + "[CRITICAL SECURITY REQUIREMENT]" + core.NC,
					"",
					"  Redis bound to 0.0.0.0 (Local Network / Internet) WITHOUT a password",
					"  is extremely vulnerable to remote code execution and SSH key injection.",
					"",
					core.YELLOW + "  Password authentication is mandatory for network-accessible Redis instances." + core.NC,
				})
				tui.ReadKey()
				requirePass = true
			}

			if requirePass {
				tui.ClearScreen()
				passOptions := []string{
					"Generate Secure Random Password (Recommended)",
					"Enter Custom Password",
				}
				pIdx, _, pStatus := tui.SelectMenu("Step 3/4: Password Configuration", passOptions)
				if pStatus == 1 || pStatus == 2 {
					currentStep = 2
					continue
				}

				if pIdx == 0 {
					if !isCustomPass {
						genPass, gErr := modules.GenerateRandomPass(20)
						if gErr != nil {
							logger.LogError(fmt.Sprintf("Failed to generate Redis password: %v", gErr))
							return
						}
						redisPassword = genPass
					}
				} else {
					tui.ClearScreen()
					passCardLines := []string{
						core.BOLD + core.CYAN + "Custom Redis Authentication Password" + core.NC,
						"",
						"  Enter password for Redis 'AUTH' command.",
					}
					tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back", "Step 3/4: Custom Password", passCardLines)

					passIn, pSt := modules.ReadPasswordInput("Enter Redis Password")
					if pSt != 0 || passIn == "" {
						continue
					}
					redisPassword = passIn
					isCustomPass = true
				}
			} else {
				redisPassword = ""
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
				tui.FormatKV("Redis Port", port, core.CYAN),
				tui.FormatKV("Authentication", func() string {
					if requirePass {
						return core.GREEN + "[ENABLED] (Password Protected)" + core.NC
					}
					return core.YELLOW + "[DISABLED] (Passwordless Access)" + core.NC
				}(), core.GREEN),
			}

			if requirePass {
				summaryLines = append(summaryLines, tui.FormatKV("Redis Password", redisPassword, core.YELLOW))
			}

			if envType == "native" {
				summaryLines = append(summaryLines, "")
				summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PERFORMANCE & STORAGE:"+core.NC)
				summaryLines = append(summaryLines, tui.FormatKV("Dump Path", "/var/lib/redis/dump.rdb", core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("MaxMemory Policy", "volatile-lru (Auto-scaled to 25% RAM)", core.GREEN))
				summaryLines = append(summaryLines, tui.FormatKV("Config Path", "/etc/redis/redis.conf", core.CYAN))
			} else {
				summaryLines = append(summaryLines, "")
				summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"DOCKER STORAGE & STACK:"+core.NC)
				summaryLines = append(summaryLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("Data Volume", "redis_data -> /data", core.CYAN))
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
				ExecuteNativeInstall(accessScope, port, requirePass, redisPassword)
			} else {
				ExecuteDockerInstall(accessScope, port, requirePass, redisPassword, composeDir)
			}
			return
		}
	}
}

// ExecuteNativeInstall installs Redis natively on the host OS.
func ExecuteNativeInstall(accessScope, port string, requirePass bool, redisPassword string) {
	// Mutual conflict check: Check if Docker Redis container is active
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-redis-server").Output(); err == nil && strings.TrimSpace(string(out)) == "running" {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "CONTAINER CONFLICT DETECTED" + core.NC,
			"",
			"  Docker Redis Container 'krotek-redis-server' is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Native Redis, the Docker container stack must be stopped",
			"  to free network ports (Port " + port + ").",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop Docker Redis container stack to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Container Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Docker Redis container and continue Native deployment?")
		if res != 0 {
			logger.LogInfo("Native installation cancelled by user due to container conflict.")
			return
		}
		_ = exec.Command("docker", "stop", "krotek-redis-server").Run()
	}

	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}

	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing Redis In-Memory Store (Native Service) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native Redis deployment...")

	// 1. Package Installation
	var pkgs []string
	switch core.SysInfo.Family {
	case "alpine":
		pkgs = []string{"redis", "redis-openrc"}
	case "debian":
		pkgs = []string{"redis-server", "redis-tools"}
	case "rhel":
		pkgs = []string{"redis"}
	default:
		pkgs = []string{"redis-server", "redis-tools"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogError("Failed to install Redis system packages.")
		return
	}

	// 2. Service Enablement
	if !system.StartService("redis-server") {
		_ = system.StartService("redis")
	}

	time.Sleep(2 * time.Second)

	// 3. Determine Dynamic maxmemory (25% of system RAM, minimum 128MB)
	maxMemory := "256mb"
	if memData, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(memData), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if totalKb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
						totalMb := totalKb / 1024
						memMb := totalMb / 4
						if memMb < 128 {
							memMb = 128
						}
						maxMemory = fmt.Sprintf("%dmb", memMb)
					}
				}
				break
			}
		}
	}

	// 4. Configure Redis Settings in redis.conf
	bindAddr := "127.0.0.1 -::1"
	if accessScope == "network" {
		bindAddr = "* -::*"
	}

	confFiles := []string{
		"/etc/redis/redis.conf",
		"/etc/redis.conf",
	}
	for _, cf := range confFiles {
		if data, err := os.ReadFile(cf); err == nil {
			content := string(data)
			// Adjust port
			content = regexp.MustCompile(`(?mi)^port\s+[0-9]+`).ReplaceAllString(content, fmt.Sprintf("port %s", port))
			// Adjust bind
			content = regexp.MustCompile(`(?mi)^bind\s+.*`).ReplaceAllString(content, fmt.Sprintf("bind %s", bindAddr))
			// Adjust requirepass safely
			if requirePass && redisPassword != "" {
				cleanPass := sanitizeRedisPassword(redisPassword)
				passDirective := fmt.Sprintf("requirepass \"%s\"", cleanPass)
				if strings.Contains(content, "requirepass") {
					content = regexp.MustCompile(`(?mi)^#?\s*requirepass\s+.*`).ReplaceAllString(content, passDirective)
				} else {
					content += fmt.Sprintf("\n%s\n", passDirective)
				}
			}
			// Adjust maxmemory
			content += fmt.Sprintf("\nmaxmemory %s\nmaxmemory-policy volatile-lru\n", maxMemory)

			_ = system.WriteFileNoFollow(cf, []byte(content), 0640, "")
		}
	}

	// 5. Restart Service to apply changes
	if !system.RestartService("redis-server") {
		_ = system.RestartService("redis")
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
		"ADMIN_PASS":   redisPassword,
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
		tui.FormatKV("Local CLI Command", func() string {
			if requirePass {
				return fmt.Sprintf("redis-cli -p %s -a '%s'", port, redisPassword)
			}
			return fmt.Sprintf("redis-cli -p %s", port)
		}(), core.CYAN),
	}

	if accessScope == "network" {
		summaryLines = append(summaryLines, tui.FormatKV("Remote CLI Command", func() string {
			if requirePass {
				return fmt.Sprintf("redis-cli -h %s -p %s -a '%s'", localIP, port, redisPassword)
			}
			return fmt.Sprintf("redis-cli -h %s -p %s", localIP, port)
		}(), core.GREEN))
	}

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"SECURITY & MEMORY:"+core.NC)
	if requirePass {
		summaryLines = append(summaryLines, tui.FormatKV("Redis Password", redisPassword, core.YELLOW+core.BOLD))
	} else {
		summaryLines = append(summaryLines, tui.FormatKV("Authentication", "[NONE] (Unprotected)", core.RED))
	}
	summaryLines = append(summaryLines, tui.FormatKV("Memory Limit", maxMemory, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Eviction Policy", "volatile-lru", core.CYAN))

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Data Storage", "/var/lib/redis", core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Installer Log", core.LogFilePath, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "redis-manager", core.GREEN))

	tui.EnterAltScreen()
	tui.ClearScreen()
	tui.PrintInBox("Redis Native Stack Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Native Redis installation completed.")
}

// ExecuteDockerInstall provisions Redis via Docker Compose.
func ExecuteDockerInstall(accessScope, port string, requirePass bool, redisPassword, composeDir string) {
	// Mutual conflict check: Check if Native Redis service is active
	if exec.Command("systemctl", "is-active", "--quiet", "redis-server").Run() == nil || exec.Command("systemctl", "is-active", "--quiet", "redis").Run() == nil {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "SERVICE CONFLICT DETECTED" + core.NC,
			"",
			"  Native Redis Linux Service is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Docker Redis, the Native service must be stopped & disabled",
			"  to free network ports (Port " + port + ").",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop & Disable Native Redis service to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Service Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Native Redis service and continue Docker deployment?")
		if res != 0 {
			logger.LogInfo("Docker installation cancelled by user due to service conflict.")
			return
		}
		_ = exec.Command("systemctl", "stop", "redis-server").Run()
		_ = exec.Command("systemctl", "stop", "redis").Run()
		_ = exec.Command("systemctl", "disable", "redis-server").Run()
		_ = exec.Command("systemctl", "disable", "redis").Run()
	}

	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}

	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing Redis (Docker Container Stack) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker Redis deployment...")

	if !system.EnsureDockerEngine() {
		return
	}

	_ = os.MkdirAll(composeDir, 0755)

	portBinding := fmt.Sprintf(`"%s:6379"`, port)
	if accessScope == "local" {
		portBinding = fmt.Sprintf(`"127.0.0.1:%s:6379"`, port)
	}

	redisConfContent := BuildRedisDockerConf(requirePass, redisPassword)
	redisConfFile := filepath.Join(composeDir, "redis.conf")
	if err := system.WriteFileNoFollow(redisConfFile, []byte(redisConfContent), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write Redis container config: %v", err))
		return
	}

	composeYaml := fmt.Sprintf(`version: '3.8'

services:
  redis:
    image: redis:alpine
    container_name: krotek-redis-server
    restart: unless-stopped
    command: redis-server /usr/local/etc/redis/redis.conf
    ports:
      - %s
    volumes:
      - redis_data:/data
      - ./redis.conf:/usr/local/etc/redis/redis.conf:ro

volumes:
  redis_data:
    name: krotek_redis_data
`, portBinding)

	composeFile := filepath.Join(composeDir, "docker-compose.yml")
	if err := system.WriteFileNoFollow(composeFile, []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write compose file: %v", err))
		return
	}

	if !system.RunDockerCompose(composeDir, "Deploying Redis Docker Stack", true, "up", "-d") {
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
		"ADMIN_PASS":   redisPassword,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	// Display Success Box
	localIP := system.GetLocalIP()
	summaryLines := []string{
		tui.FormatKV("Deployment Mode", "Docker Compose Container Stack", core.GREEN),
		tui.FormatKV("Container Name", "krotek-redis-server (redis:alpine)", core.GREEN),
		tui.FormatKV("Port Binding", func() string {
			if accessScope == "network" {
				return fmt.Sprintf("0.0.0.0:%s -> 6379/tcp", port)
			}
			return fmt.Sprintf("127.0.0.1:%s -> 6379/tcp", port)
		}(), core.GREEN),
		tui.FormatKV("Local Connection", func() string {
			if requirePass {
				return fmt.Sprintf("redis-cli -h 127.0.0.1 -p %s -a '%s'", port, redisPassword)
			}
			return fmt.Sprintf("redis-cli -h 127.0.0.1 -p %s", port)
		}(), core.CYAN),
	}

	if accessScope == "network" {
		summaryLines = append(summaryLines, tui.FormatKV("Remote Connection", func() string {
			if requirePass {
				return fmt.Sprintf("redis-cli -h %s -p %s -a '%s'", localIP, port, redisPassword)
			}
			return fmt.Sprintf("redis-cli -h %s -p %s", localIP, port)
		}(), core.GREEN))
	}

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"SECURITY & STORAGE:"+core.NC)
	if requirePass {
		summaryLines = append(summaryLines, tui.FormatKV("Redis Password", redisPassword, core.YELLOW+core.BOLD))
	} else {
		summaryLines = append(summaryLines, tui.FormatKV("Authentication", "[NONE] (Unprotected)", core.RED))
	}
	summaryLines = append(summaryLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Docker Volume", "krotek_redis_data", core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Installer Log", core.LogFilePath, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "redis-manager", core.GREEN))

	tui.EnterAltScreen()
	tui.ClearScreen()
	tui.PrintInBox("Redis Docker Stack Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Docker Redis installation completed.")
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

// BuildRedisDockerConf renders a mounted redis.conf so requirepass is never exposed on container CLI args.
func BuildRedisDockerConf(requirePass bool, redisPassword string) string {
	var b strings.Builder
	b.WriteString("save 60 1\nloglevel warning\ndir /data\n")
	if requirePass && redisPassword != "" {
		b.WriteString(fmt.Sprintf("requirepass \"%s\"\n", sanitizeRedisPassword(redisPassword)))
	}
	return b.String()
}

func sanitizeRedisPassword(pass string) string {
	clean := strings.ReplaceAll(pass, "\n", "")
	clean = strings.ReplaceAll(clean, "\r", "")
	clean = strings.ReplaceAll(clean, "\\", "\\\\")
	clean = strings.ReplaceAll(clean, "\"", "\\\"")
	return clean
}
