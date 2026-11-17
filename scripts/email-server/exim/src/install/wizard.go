// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Exim Mail Transfer Agent (exim)
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
	"strings"

	"krotek-exim-installer/core"
	"krotek-exim-installer/logger"
	"krotek-exim-installer/system"
	"krotek-exim-installer/tui"
)

type EximConfig struct {
	Mode       string
	MailDomain string
	ServerType string // "internet" or "smarthost"
	RelayHost  string
	Port       string
}

// FreshInstallWizard handles the end-to-end installation pipeline for Exim Mail Transfer Agent.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(mode string) {
	detectedHostname, _ := os.Hostname()
	if detectedHostname == "" {
		detectedHostname = "mail.example.com"
	}

	cfg := EximConfig{
		Mode:       mode,
		MailDomain: detectedHostname,
		ServerType: "internet",
		RelayHost:  "",
		Port:       "25",
	}

	PromptExistingConfigAction("Exim MTA Mail Server", "/etc/exim4")

	step := 1
	for {
		switch step {
		case 1:
			// Step 1: Mail Domain
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Exim4 MTA Mail Domain Configuration" + core.NC,
				"",
				"  Specify primary mail domain name (FQDN).",
				"  " + core.BOLD + "Detected Hostname:" + core.NC + " " + detectedHostname,
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Exit Setup", "Step 1/4: Mail Domain", cardLines)

			domVal, st := tui.ReadTextInput("Primary Mail Domain", cfg.MailDomain)
			if st != 0 {
				return
			}
			domVal = strings.TrimSpace(domVal)
			if domVal == "" {
				domVal = detectedHostname
			}
			cfg.MailDomain = domVal
			step = 2

		case 2:
			// Step 2: Server Delivery Mode
			tui.ClearScreen()
			opts := []string{
				"Internet Site (Direct Send & Receive over SMTP)",
				"Mail Sent by Smarthost (Received via SMTP/fetchmail)",
				"<- Back to Mail Domain",
			}
			idx, _, st := tui.SelectMenu("Step 2/4: Delivery Configuration Profile", opts)
			if st != 0 || idx == 2 {
				step = 1
				continue
			}
			if idx == 0 {
				cfg.ServerType = "internet"
				cfg.RelayHost = ""
				step = 4
			} else {
				cfg.ServerType = "smarthost"
				step = 3
			}

		case 3:
			// Step 3: Smarthost
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Outgoing Smarthost / Relay Address" + core.NC,
				"",
				"  Specify target SMTP relay (e.g. smtp.mailgun.org::587).",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Profile", "Step 3/4: Smarthost", cardLines)

			relayVal, rSt := tui.ReadTextInput("Smarthost Address", cfg.RelayHost)
			if rSt != 0 {
				step = 2
				continue
			}
			cfg.RelayHost = strings.TrimSpace(relayVal)
			step = 4

		case 4:
			// Step 4: Review Summary Card
			tui.ClearScreen()
			relayDisplay := "None (Direct Internet Site)"
			if cfg.RelayHost != "" {
				relayDisplay = cfg.RelayHost
			}
			summary := []string{
				fmt.Sprintf("%sDeployment Mode:%s     %s", core.CYAN, core.NC, strings.ToUpper(cfg.Mode)),
				fmt.Sprintf("%sPrimary Domain:%s      %s", core.CYAN, core.NC, cfg.MailDomain),
				fmt.Sprintf("%sDelivery Profile:%s    %s", core.CYAN, core.NC, strings.ToUpper(cfg.ServerType)),
				fmt.Sprintf("%sSmarthost Relay:%s    %s", core.CYAN, core.NC, relayDisplay),
				fmt.Sprintf("%sSMTP Inbound Port:%s  %s/TCP", core.CYAN, core.NC, cfg.Port),
				"",
				"Ready to install Exim4 MTA service.",
			}
			tui.PrintInBox("--no-wait", "Step 4/4: Configuration Summary", summary)

			cOpts := []string{
				"Confirm & Proceed with Installation",
				"<- Change Configuration (Back)",
				"Cancel Installation",
			}
			cIdx, _, cSt := tui.SelectMenu("Confirm Exim4 Deployment", cOpts)
			if cSt != 0 || cIdx == 2 {
				return
			}
			if cIdx == 1 {
				if cfg.ServerType == "smarthost" {
					step = 3
				} else {
					step = 2
				}
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
	ports := []string{port, "587", "465"}
	system.ConfigureFirewallRules(ports...)
}

func ExecuteDockerInstall(cfg EximConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing Exim Docker Container Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker Exim installation...")

	if !system.EnsureDockerEngine() {
		return
	}

	composeDir := "/etc/exim-docker"
	_ = os.MkdirAll(composeDir, 0755)

	composeYaml := fmt.Sprintf(`version: '3.8'
services:
  exim:
    image: devture/exim-relay:latest
    container_name: krotek-exim
    restart: unless-stopped
    ports:
      - "%s:25"
      - "587:587"
    environment:
      - PRIMARY_HOST="%s"
      - SMARTHOST="%s"
`, cfg.Port, cfg.MailDomain, cfg.RelayHost)

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}
	configureFirewall(cfg.Port)

	if !system.RunDockerCompose(composeDir, "Deploying Exim Docker Stack", true, "up", "-d") {
		return
	}

	system.SaveState(map[string]string{
		"ENV_TYPE":    "docker",
		"COMPOSE_DIR": composeDir,
		"MAIL_DOMAIN": cfg.MailDomain,
		"PORT":        cfg.Port,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("Exim Docker deployment complete.")
	printEximInstructions(cfg)
}

func ExecuteNativeInstall(cfg EximConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing Exim4 Native Service ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native Exim4 installation...")

	// Pre-seed debconf for debian safely via stdin
	if core.SysInfo.Family == "debian" {
		cfgType := "internet"
		if cfg.ServerType == "smarthost" {
			cfgType = "smarthost"
		}
		cmd1 := exec.Command("debconf-set-selections")
		cmd1.Stdin = strings.NewReader(fmt.Sprintf("exim4-config exim4/dc_eximconfig_configtype string \"%s\"\n", cfgType))
		_ = cmd1.Run()

		cmd2 := exec.Command("debconf-set-selections")
		cmd2.Stdin = strings.NewReader(fmt.Sprintf("exim4-config exim4/mailname string \"%s\"\n", cfg.MailDomain))
		_ = cmd2.Run()
	}

	var pkgs []string
	switch core.SysInfo.Family {
	case "alpine":
		pkgs = []string{"exim", "exim-openrc"}
	case "debian":
		pkgs = []string{"exim4", "exim4-daemon-heavy"}
	case "rhel":
		_ = exec.Command("dnf", "install", "-y", "epel-release").Run()
		_ = exec.Command("yum", "install", "-y", "epel-release").Run()
		pkgs = []string{"exim"}
	default:
		pkgs = []string{"exim4", "exim4-daemon-heavy"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogWarn("Package installation had warnings, proceeding...")
	}

	configureFirewall(cfg.Port)

	_ = system.WriteFileNoFollow("/etc/mailname", []byte(cfg.MailDomain+"\n"), 0600, "")
	if core.SysInfo.Family == "debian" {
		_ = exec.Command("update-exim4.conf").Run()
	}
	if !system.StartService("exim4") {
		_ = system.StartService("exim")
	}
	if !system.RestartService("exim4") {
		_ = system.RestartService("exim")
	}

	system.SaveState(map[string]string{
		"ENV_TYPE":    "native",
		"MAIL_DOMAIN": cfg.MailDomain,
		"PORT":        cfg.Port,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("Exim4 Native setup finished successfully.")
	printEximInstructions(cfg)
}

func printEximInstructions(cfg EximConfig) {
	fmt.Printf("\r\n%s------------------------------------------------------------%s\r\n", core.GREEN, core.NC)
	fmt.Printf("%s%s Exim4 MTA Service Active!%s\r\n", core.BOLD+core.GREEN, core.TagOK(), core.NC)
	fmt.Printf("  %sDomain:%s   %s\r\n", core.CYAN, core.NC, cfg.MailDomain)
	fmt.Printf("  %sPort:%s     %s/TCP\r\n", core.CYAN, core.NC, cfg.Port)
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
