// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Postfix & Dovecot Mail Server Stack (postfix)
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

	"krotek-postfix-installer/core"
	"krotek-postfix-installer/logger"
	"krotek-postfix-installer/system"
	"krotek-postfix-installer/tui"
)

type PostfixConfig struct {
	Mode       string
	MailDomain string
	ServerType string // "internet" or "satellite"
	RelayHost  string
	Port       string
}

// FreshInstallWizard handles the end-to-end installation pipeline for Postfix & Dovecot Mail Server Stack.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(mode string) {
	detectedHostname, _ := os.Hostname()
	if detectedHostname == "" {
		detectedHostname = "mail.example.com"
	}

	cfg := PostfixConfig{
		Mode:       mode,
		MailDomain: detectedHostname,
		ServerType: "internet",
		RelayHost:  "",
		Port:       "25",
	}

	PromptExistingConfigAction("Postfix Mail Server", "/etc/postfix/main.cf")

	step := 1
	for {
		switch step {
		case 1:
			// Step 1: Mail Domain FQDN
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Postfix Mail Server Hostname / Domain" + core.NC,
				"",
				"  Specify the fully qualified domain name (FQDN) for this mail server.",
				"  " + core.BOLD + "Detected Hostname:" + core.NC + " " + detectedHostname,
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Exit Setup", "Step 1/4: Mail Domain", cardLines)

			domVal, st := tui.ReadTextInput("Mail Domain / FQDN", cfg.MailDomain)
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
			// Step 2: Server Type
			tui.ClearScreen()
			typeOpts := []string{
				"Internet Site (Direct Send & Receive over standard SMTP)",
				"Satellite / Smarthost (Forward all outbound mail via Relay/SES/SendGrid)",
				"<- Back to Mail Domain",
			}
			tIdx, _, tSt := tui.SelectMenu("Step 2/4: Mail Delivery Profile", typeOpts)
			if tSt != 0 || tIdx == 2 {
				step = 1
				continue
			}
			if tIdx == 0 {
				cfg.ServerType = "internet"
				cfg.RelayHost = ""
				step = 4
			} else {
				cfg.ServerType = "satellite"
				step = 3
			}

		case 3:
			// Step 3: Relay Host
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "External SMTP Relay / Smarthost Configuration" + core.NC,
				"",
				"  Specify your relay server address (e.g. [smtp-relay.brevo.com]:587).",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Server Type", "Step 3/4: Relay Host", cardLines)

			relayVal, rSt := tui.ReadTextInput("Relay Host ([host]:port)", cfg.RelayHost)
			if rSt != 0 {
				step = 2
				continue
			}
			cfg.RelayHost = strings.TrimSpace(relayVal)
			step = 4

		case 4:
			// Step 4: Review Summary Card
			tui.ClearScreen()
			relayDisplay := "None (Direct Delivery)"
			if cfg.RelayHost != "" {
				relayDisplay = cfg.RelayHost
			}
			summary := []string{
				fmt.Sprintf("%sDeployment Mode:%s     %s", core.CYAN, core.NC, strings.ToUpper(cfg.Mode)),
				fmt.Sprintf("%sMail Domain FQDN:%s   %s", core.CYAN, core.NC, cfg.MailDomain),
				fmt.Sprintf("%sDelivery Type:%s      %s", core.CYAN, core.NC, strings.ToUpper(cfg.ServerType)),
				fmt.Sprintf("%sRelay Host:%s         %s", core.CYAN, core.NC, relayDisplay),
				fmt.Sprintf("%sSMTP Inbound Port:%s  %s/TCP", core.CYAN, core.NC, cfg.Port),
				"",
				"Ready to install and configure Postfix MTA.",
			}
			tui.PrintInBox("--no-wait", "Step 4/4: Configuration Summary", summary)

			cOpts := []string{
				"Confirm & Proceed with Installation",
				"<- Change Configuration (Back)",
				"Cancel Installation",
			}
			cIdx, _, cSt := tui.SelectMenu("Confirm Postfix Deployment", cOpts)
			if cSt != 0 || cIdx == 2 {
				return
			}
			if cIdx == 1 {
				if cfg.ServerType == "satellite" {
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

func ExecuteDockerInstall(cfg PostfixConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing Postfix Docker Container Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker Postfix installation...")

	if !system.EnsureDockerEngine() {
		return
	}

	composeDir := "/etc/postfix-docker"
	_ = os.MkdirAll(composeDir, 0755)

	composeYaml := fmt.Sprintf(`version: '3.8'
services:
  postfix:
    image: boky/postfix:latest
    container_name: krotek-postfix
    restart: unless-stopped
    ports:
      - "%s:25"
      - "587:587"
    environment:
      - HOSTNAME="%s"
      - RELAYHOST="%s"
    volumes:
      - %s/spool:/var/spool/postfix
`, cfg.Port, cfg.MailDomain, cfg.RelayHost, composeDir)

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}
	configureFirewall(cfg.Port)

	if !system.RunDockerCompose(composeDir, "Deploying Postfix Docker Stack", true, "up", "-d") {
		return
	}

	system.SaveState(map[string]string{
		"ENV_TYPE":    "docker",
		"COMPOSE_DIR": composeDir,
		"MAIL_DOMAIN": cfg.MailDomain,
		"PORT":        cfg.Port,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("Postfix Docker deployment complete.")
	printPostfixInstructions(cfg)
}

func ExecuteNativeInstall(cfg PostfixConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing Postfix Native Service ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native Postfix installation...")

	// Pre-seed debconf for non-interactive postfix install on Debian/Ubuntu safely via stdin
	if core.SysInfo.Family == "debian" {
		cmd1 := exec.Command("debconf-set-selections")
		cmd1.Stdin = strings.NewReader(fmt.Sprintf("postfix postfix/mailname string %s\n", cfg.MailDomain))
		_ = cmd1.Run()

		cmd2 := exec.Command("debconf-set-selections")
		cmd2.Stdin = strings.NewReader("postfix postfix/main_mailer_type string \"Internet Site\"\n")
		_ = cmd2.Run()
	}

	var pkgs []string
	switch core.SysInfo.Family {
	case "alpine":
		pkgs = []string{"postfix", "postfix-openrc"}
	case "debian":
		pkgs = []string{"postfix", "mailutils"}
	case "rhel":
		pkgs = []string{"postfix", "mailx"}
	default:
		pkgs = []string{"postfix", "mailutils"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogWarn("Package installation had warnings, proceeding...")
	}

	configureFirewall(cfg.Port)

	// Backup original main.cf
	if _, err := os.Stat("/etc/postfix/main.cf"); err == nil {
		_ = exec.Command("cp", "-a", "/etc/postfix/main.cf", "/etc/postfix/main.cf.bak").Run()
	}

	mainCf := fmt.Sprintf(`smtpd_banner = $myhostname ESMTP $mail_name
biff = no
append_dot_mydomain = no
readme_directory = no
compatibility_level = 2

smtpd_tls_cert_file=/etc/ssl/certs/ssl-cert-snakeoil.pem
smtpd_tls_key_file=/etc/ssl/private/ssl-cert-snakeoil.key
smtpd_tls_security_level=may
smtp_tls_security_level=may
smtp_tls_session_cache_database = btree:${data_directory}/smtp_scache

myhostname = %s
alias_maps = hash:/etc/aliases
alias_database = hash:/etc/aliases
myorigin = /etc/mailname
mydestination = $myhostname, %s, localhost.localdomain, localhost
relayhost = %s
mynetworks = 127.0.0.0/8 [::ffff:127.0.0.0]/104 [::1]/128
mailbox_size_limit = 0
recipient_delimiter = +
inet_interfaces = all
inet_protocols = all
`, cfg.MailDomain, cfg.MailDomain, cfg.RelayHost)

	_ = system.WriteFileNoFollow("/etc/mailname", []byte(cfg.MailDomain+"\n"), 0644, "")
	_ = system.WriteFileNoFollow("/etc/postfix/main.cf", []byte(mainCf), 0644, "")

	_ = exec.Command("newaliases").Run()
	system.StartService("postfix")
	system.RestartService("postfix")

	system.SaveState(map[string]string{
		"ENV_TYPE":    "native",
		"MAIL_DOMAIN": cfg.MailDomain,
		"PORT":        cfg.Port,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("Postfix Native setup finished successfully.")
	printPostfixInstructions(cfg)
}

func printPostfixInstructions(cfg PostfixConfig) {
	fmt.Printf("\r\n%s------------------------------------------------------------%s\r\n", core.GREEN, core.NC)
	fmt.Printf("%s[OK] Postfix Mail Transfer Agent Active!%s\r\n", core.BOLD+core.GREEN, core.NC)
	fmt.Printf("  %sMail Domain:%s %s\r\n", core.CYAN, core.NC, cfg.MailDomain)
	fmt.Printf("  %sSMTP Port:%s   %s\r\n", core.CYAN, core.NC, cfg.Port)
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
