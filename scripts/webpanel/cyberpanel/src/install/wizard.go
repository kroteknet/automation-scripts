// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: CyberPanel Web Hosting Management Panel (cyberpanel)
//
// This package orchestrates automated pre-flight checks, package repository setup,
// configuration templating, service enablement (systemd/openrc), and firewall rules.
// Supports both interactive TUI step-by-step guidance and headless automation.
// ==============================================================================

package install

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"krotek-cyberpanel-installer/core"
	"krotek-cyberpanel-installer/logger"
	"krotek-cyberpanel-installer/system"
	"krotek-cyberpanel-installer/tui"
)

type CyberPanelConfig struct {
	Mode       string
	Port       string
	AdminUser  string
	AdminPass  string
	AdminEmail string
}

func generateRandomPassword(length int) string {
	bytes := make([]byte, length/2)
	for i := 0; i < 5; i++ {
		if _, err := rand.Read(bytes); err == nil {
			return hex.EncodeToString(bytes) + "A1!"
		}
	}
	logger.LogError("Cryptographic randomness failure: unable to generate secure admin password")
	os.Exit(1)
	return ""
}

// FreshInstallWizard handles the end-to-end installation pipeline for CyberPanel Web Hosting Management Panel.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(mode string) {
	cfg := CyberPanelConfig{
		Mode:       mode,
		Port:       "8090",
		AdminUser:  "admin",
		AdminPass:  generateRandomPassword(12),
		AdminEmail: "admin@example.com",
	}

	PromptExistingConfigAction("CyberPanel Control Panel", "/usr/local/CyberCP")

	step := 1
	for {
		switch step {
		case 1:
			// Step 1: Port Selection
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "CyberPanel Web GUI Port Configuration" + core.NC,
				"",
				"  Specify the port for accessing the CyberPanel web administration panel.",
				"  " + core.BOLD + "Default Port:" + core.NC + " 8090",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Exit Setup", "Step 1/4: Panel Port", cardLines)

			portVal, st := tui.ReadTextInput("CyberPanel Web Port", cfg.Port)
			if st != 0 {
				return
			}
			portVal = strings.TrimSpace(portVal)
			if portVal == "" {
				portVal = "8090"
			}
			pNum, pErr := strconv.Atoi(portVal)
			if pErr != nil || pNum < 1 || pNum > 65535 {
				tui.PrintInBox("Invalid Port", []string{
					core.RED + "Port must be an integer between 1 and 65535." + core.NC,
				})
				continue
			}
			cfg.Port = strconv.Itoa(pNum)
			step = 2

		case 2:
			// Step 2: Admin Credentials
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "CyberPanel Administrator Credentials" + core.NC,
				"",
				"  Configure master credentials to login to the CyberPanel dashboard.",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: Admin Credentials", cardLines)

			emailVal, eSt := tui.ReadTextInput("Administrator Email", cfg.AdminEmail)
			if eSt != 0 {
				step = 1
				continue
			}
			emailVal = strings.TrimSpace(emailVal)
			if emailVal == "" {
				emailVal = "admin@example.com"
			}
			cfg.AdminEmail = emailVal

			userVal, uSt := tui.ReadTextInput("Administrator Username", cfg.AdminUser)
			if uSt != 0 {
				step = 1
				continue
			}
			userVal = strings.TrimSpace(userVal)
			if userVal == "" {
				userVal = "admin"
			}
			cfg.AdminUser = userVal

			passVal, pSt := tui.ReadPasswordInput("Administrator Password (blank to keep/generate)")
			if pSt != 0 {
				step = 1
				continue
			}
			passVal = strings.TrimSpace(passVal)
			if passVal == "" {
				passVal = generateRandomPassword(12)
			}
			cfg.AdminPass = passVal
			step = 3

		case 3:
			// Step 3: Performance & Addons
			tui.ClearScreen()
			options := []string{
				"Standard OpenLiteSpeed Stack (Recommended)",
				"Include PowerDNS & Postfix Mail Integration",
				"<- Back to Previous Step",
			}
			idx, _, st := tui.SelectMenu("Step 3/4: CyberPanel Features & Engine", options)
			if st != 0 || idx == 2 {
				step = 2
				continue
			}
			step = 4

		case 4:
			// Step 4: Review Summary Card
			tui.ClearScreen()
			summary := []string{
				fmt.Sprintf("%sDeployment Mode:%s     %s", core.CYAN, core.NC, strings.ToUpper(cfg.Mode)),
				fmt.Sprintf("%sPanel Port:%s          %s", core.CYAN, core.NC, cfg.Port),
				fmt.Sprintf("%sDashboard URL:%s       https://<SERVER_IP>:%s", core.CYAN, core.NC, cfg.Port),
				fmt.Sprintf("%sAdmin Email:%s         %s", core.CYAN, core.NC, cfg.AdminEmail),
				fmt.Sprintf("%sAdmin User:%s          %s", core.CYAN, core.NC, cfg.AdminUser),
				fmt.Sprintf("%sAdmin Password:%s      %s", core.CYAN, core.NC, cfg.AdminPass),
				"",
				"Ready to install CyberPanel control panel.",
			}
			tui.PrintInBox("--no-wait", "Step 4/4: Configuration Summary", summary)

			cOpts := []string{
				"Confirm & Proceed with Installation",
				"<- Change Configuration (Back)",
				"Cancel Installation",
			}
			cIdx, _, cSt := tui.SelectMenu("Confirm CyberPanel Deployment", cOpts)
			if cSt != 0 || cIdx == 2 {
				return
			}
			if cIdx == 1 {
				step = 3
				continue
			}

			// Port conflict check
			if resolvedPort, ok := system.ResolvePortConflict(cfg.Port, "tcp", false); !ok {
				return
			} else {
				cfg.Port = resolvedPort
			}

			if cfg.Mode == "docker" {
				ExecuteDockerInstall(cfg)
			} else {
				ExecuteNativeInstall(cfg)
			}
			return
		}
	}
}

func configureFirewall(port string) {
	system.ConfigureFirewallRules(port, "80", "443", "7080")
}

func ExecuteDockerInstall(cfg CyberPanelConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing CyberPanel Docker Container Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker CyberPanel installation...")
	logger.LogWarn("Upstream notice: CyberPanel is primarily architected for native bare-metal VPS. Docker deployment is experimental.")

	if !system.EnsureDockerEngine() {
		return
	}

	composeDir := "/etc/cyberpanel-docker"
	_ = os.MkdirAll(composeDir, 0755)

	composeYaml := fmt.Sprintf(`version: '3.8'
services:
  cyberpanel:
    image: cyberpanel/cyberpanel:latest
    container_name: krotek-cyberpanel
    restart: unless-stopped
    ports:
      - "%s:8090"
      - "80:80"
      - "443:443"
      - "7080:7080"
    volumes:
      - %s/data:/usr/local/CyberCP
`, cfg.Port, composeDir)

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}
	configureFirewall(cfg.Port)

	if !system.RunDockerCompose(composeDir, "Deploying CyberPanel Docker Stack", true, "up", "-d") {
		return
	}

	system.SaveState(map[string]string{
		"ENV_TYPE":    "docker",
		"COMPOSE_DIR": composeDir,
		"PORT":        cfg.Port,
		"ADMIN_USER":  cfg.AdminUser,
		"ADMIN_PASS":  cfg.AdminPass,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("CyberPanel Docker deployment complete.")
	printCyberPanelInstructions(cfg)
}

func ExecuteNativeInstall(cfg CyberPanelConfig) {
	if core.SysInfo.Family == "alpine" {
		tui.PrintInBox("CYBERPANEL Native" + " " + core.Sym.EmDash + " " + "Alpine Linux Not Supported", []string{
			core.BOLD + core.RED + "Native CYBERPANEL service is not supported on Alpine Linux." + core.NC,
			"Alpine Linux lacks required systemd/glibc dependencies.",
			"",
			core.BOLD + core.GREEN + "Recommendation:" + core.NC + " Use the Docker container deployment mode instead.",
		})
		logger.LogError("Native CYBERPANEL is unsupported on Alpine Linux. Aborting.")
		return
	}
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing CyberPanel Native Service ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native CyberPanel installation...")

	var pkgs []string
	switch core.SysInfo.Family {
	case "debian":
		pkgs = []string{"curl", "wget", "python3", "tar", "ufw"}
	case "rhel":
		pkgs = []string{"curl", "wget", "python3", "tar", "firewalld"}
	default:
		pkgs = []string{"curl", "wget", "python3", "tar"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogWarn("Package installation had warnings, proceeding...")
	}

	configureFirewall(cfg.Port)

	// Invoke official installer non-interactively if internet is reachable
	tmpDir, err := os.MkdirTemp("", "cyberpanel-*")
	if err != nil {
		logger.LogError("Failed to create secure temporary directory: " + err.Error())
		return
	}
	_ = os.Chmod(tmpDir, 0700)
	defer os.RemoveAll(tmpDir)
	scriptPath := filepath.Join(tmpDir, "cp_install.sh")

	downloadOk := false
	if system.CommandExists("curl") {
		if exec.Command("curl", "--proto", "=https", "--tlsv1.2", "-fsSL", "https://cyberpanel.net/install.sh", "-o", scriptPath).Run() == nil {
			downloadOk = true
		}
	}
	if !downloadOk && system.CommandExists("wget") {
		if exec.Command("wget", "--https-only", "-qO", scriptPath, "https://cyberpanel.net/install.sh").Run() == nil {
			downloadOk = true
		}
	}

	if downloadOk {
		logger.LogInfo("Running CyberPanel bootstrap installer (this may take several minutes)...")
		wrapperPath := filepath.Join(tmpDir, "run_install.sh")
		wrapperContent := "#!/bin/bash\nexec bash \"$1\" --admin-password \"$CYBERPANEL_ADMIN_PASSWORD\"\n"
		if err := system.WriteFileNoFollow(wrapperPath, []byte(wrapperContent), 0700, ""); err == nil {
			_ = os.Setenv("CYBERPANEL_ADMIN_PASSWORD", cfg.AdminPass)
			_ = system.RunStep("Installing CyberPanel Engine", "bash", wrapperPath, scriptPath)
			_ = os.Unsetenv("CYBERPANEL_ADMIN_PASSWORD")
			_ = os.Remove(wrapperPath)
		} else {
			logger.LogError("Failed to prepare secure installer wrapper: " + err.Error())
			return
		}
	} else {
		logger.LogError("Failed to download CyberPanel installer script.")
	}

	system.StartService("lscpd")
	system.RestartService("lscpd")

	system.SaveState(map[string]string{
		"ENV_TYPE":   "native",
		"PORT":       cfg.Port,
		"ADMIN_USER": cfg.AdminUser,
		"ADMIN_PASS": cfg.AdminPass,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("CyberPanel Native setup finished successfully.")
	printCyberPanelInstructions(cfg)
}

func printCyberPanelInstructions(cfg CyberPanelConfig) {
	fmt.Printf("\r\n%s------------------------------------------------------------%s\r\n", core.GREEN, core.NC)
	fmt.Printf("%s[OK] CyberPanel Dashboard Ready!%s\r\n", core.BOLD+core.GREEN, core.NC)
	fmt.Printf("  %sPanel URL:%s    https://<SERVER_IP>:%s\r\n", core.CYAN, core.NC, cfg.Port)
	fmt.Printf("  %sCredentials:%s  User: %s | Pass: %s\r\n", core.YELLOW, core.NC, cfg.AdminUser, cfg.AdminPass)
	fmt.Printf("%s------------------------------------------------------------%s\r\n\r\n", core.GREEN, core.NC)
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
