// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Mail-in-a-Box All-In-One Mail Server (mailinabox)
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
	"regexp"
	"strings"

	"krotek-mailinabox-installer/core"
	"krotek-mailinabox-installer/logger"
	"krotek-mailinabox-installer/system"
	"krotek-mailinabox-installer/tui"
)

type MailInABoxConfig struct {
	Mode       string
	MailDomain string
	AdminEmail string
	AdminPass  string
	Port       string
}

func generateRandomPassword(length int) string {
	bytes := make([]byte, length/2)
	for i := 0; i < 5; i++ {
		if _, err := rand.Read(bytes); err == nil {
			return hex.EncodeToString(bytes) + "M1!"
		}
	}
	logger.LogError("Cryptographic randomness failure: unable to generate secure admin password")
	os.Exit(1)
	return ""
}

// FreshInstallWizard handles the end-to-end installation pipeline for Mail-in-a-Box All-In-One Mail Server.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(mode string) {
	detectedHostname, _ := os.Hostname()
	if detectedHostname == "" {
		detectedHostname = "box.example.com"
	}

	cfg := MailInABoxConfig{
		Mode:       mode,
		MailDomain: detectedHostname,
		AdminEmail: "me@" + detectedHostname,
		AdminPass:  generateRandomPassword(12),
		Port:       "25",
	}

	PromptExistingConfigAction("Mail-in-a-Box Mail Server", "/root/mailinabox")

	step := 1
	for {
		switch step {
		case 1:
			// Step 1: Mail Domain
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Mail-in-a-Box Server Domain Name" + core.NC,
				"",
				"  Enter your mail server hostname / FQDN (e.g. box.yourdomain.com).",
				"  " + core.BOLD + "Detected Hostname:" + core.NC + " " + detectedHostname,
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Exit Setup", "Step 1/4: Mail Domain FQDN", cardLines)

			domVal, st := tui.ReadTextInput("Mail Server Domain FQDN", cfg.MailDomain)
			if st != 0 {
				return
			}
			domVal = strings.TrimSpace(domVal)
			if domVal == "" {
				domVal = detectedHostname
			}
			cfg.MailDomain = domVal
			cfg.AdminEmail = "admin@" + domVal
			step = 2

		case 2:
			// Step 2: Admin Email
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Mail-in-a-Box Master Admin Email Address" + core.NC,
				"",
				"  This will be the initial administrator mailbox account.",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: Admin Email", cardLines)

			emVal, eSt := tui.ReadTextInput("Admin Email Address", cfg.AdminEmail)
			if eSt != 0 {
				step = 1
				continue
			}
			emVal = strings.TrimSpace(emVal)
			if emVal == "" {
				emVal = "admin@" + cfg.MailDomain
			}
			cfg.AdminEmail = emVal
			step = 3

		case 3:
			// Step 3: Admin Password
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Administrator Account Password" + core.NC,
				"",
				"  Master password for webmail and administrator control panel.",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: Admin Password", cardLines)

			passVal, pSt := tui.ReadPasswordInput("Admin Password (blank to keep)")
			if pSt != 0 {
				step = 2
				continue
			}
			passVal = strings.TrimSpace(passVal)
			if passVal == "" {
				passVal = generateRandomPassword(12)
			}
			cfg.AdminPass = passVal
			step = 4

		case 4:
			// Step 4: Review Summary Card
			tui.ClearScreen()
			summary := []string{
				fmt.Sprintf("%sDeployment Mode:%s     %s", core.CYAN, core.NC, strings.ToUpper(cfg.Mode)),
				fmt.Sprintf("%sMail Domain:%s         %s", core.CYAN, core.NC, cfg.MailDomain),
				fmt.Sprintf("%sAdmin Email:%s         %s", core.CYAN, core.NC, cfg.AdminEmail),
				fmt.Sprintf("%sAdmin Password:%s      %s", core.CYAN, core.NC, cfg.AdminPass),
				fmt.Sprintf("%sControl Panel:%s       https://%s/admin", core.CYAN, core.NC, cfg.MailDomain),
				fmt.Sprintf("%sWebmail URL:%s         https://%s/mail", core.CYAN, core.NC, cfg.MailDomain),
				"",
				"Ready to install Mail-in-a-Box all-in-one suite.",
			}
			tui.PrintInBox("--no-wait", "Step 4/4: Configuration Summary", summary)

			cOpts := []string{
				"Confirm & Proceed with Installation",
				"<- Change Configuration (Back)",
				"Cancel Installation",
			}
			cIdx, _, cSt := tui.SelectMenu("Confirm Mail-in-a-Box Deployment", cOpts)
			if cSt != 0 || cIdx == 2 {
				return
			}
			if cIdx == 1 {
				step = 3
				continue
			}

			// Port conflict checks
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

func configureFirewall() {
	ports := []string{"25/tcp", "587/tcp", "465/tcp", "993/tcp", "995/tcp", "80/tcp", "443/tcp", "53/tcp", "53/udp", "4190/tcp"}
	system.ConfigureFirewallRules(ports...)
}

func ExecuteDockerInstall(cfg MailInABoxConfig) {
	tui.PrintInBox("Mail-in-a-Box" + " " + core.Sym.EmDash + " " + "Docker Not Supported", []string{
		core.BOLD + core.YELLOW + "Mail-in-a-Box does not have an official Docker image on Docker Hub." + core.NC,
		"The upstream developers design Mail-in-a-Box strictly for dedicated standalone servers.",
		"",
		core.BOLD + core.GREEN + "Recommendation:" + core.NC + " Install Mail-in-a-Box Native on a dedicated Ubuntu LTS machine.",
	})
	logger.LogError("Docker mode is unsupported for Mail-in-a-Box.")
	return
}

func ExecuteNativeInstall(cfg MailInABoxConfig) {
	if core.SysInfo.Distro != "ubuntu" {
		tui.PrintInBox("MAILINABOX Native" + " " + core.Sym.EmDash + " " + "Ubuntu Strictly Required", []string{
			core.BOLD + core.RED + "Mail-in-a-Box strictly requires a clean installation of Ubuntu LTS (22.04)." + core.NC,
			fmt.Sprintf("Detected OS: %s (Unsupported by Mail-in-a-Box setup.sh).", system.GetOSPrettyVersion()),
			"",
			core.BOLD + core.GREEN + "Requirement:" + core.NC + " Please run this installer on a fresh Ubuntu VPS.",
		})
		logger.LogError("Native MAILINABOX is unsupported on non-Ubuntu systems. Aborting.")
		return
	}
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing Mail-in-a-Box Native Service ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native Mail-in-a-Box installation...")

	var pkgs []string
	switch core.SysInfo.Family {
	case "debian":
		pkgs = []string{"curl", "git", "python3", "ufw"}
	default:
		pkgs = []string{"curl", "git", "python3"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogWarn("Package installation had warnings, proceeding...")
	}

	configureFirewall()

	cleanDomain := regexp.MustCompile(`[^a-zA-Z0-9.-]`).ReplaceAllString(strings.TrimSpace(cfg.MailDomain), "")
	if cleanDomain == "" {
		cleanDomain = "box.example.com"
	}
	cfg.MailDomain = cleanDomain

	tmpDir, tErr := os.MkdirTemp("", "krotek-miab-*")
	if tErr != nil {
		logger.LogError(fmt.Sprintf("Failed to allocate secure temporary directory: %v", tErr))
		return
	}
	_ = os.Chmod(tmpDir, 0700)
	defer os.RemoveAll(tmpDir)
	setupPath := filepath.Join(tmpDir, "setup.sh")
	if !system.RunStepOrAbort("Downloading Mail-in-a-Box setup script", "curl", "--proto", "=https", "--tlsv1.2", "-fsSL", "https://mailinabox.email/setup.sh", "-o", setupPath) {
		return
	}
	if info, sErr := os.Stat(setupPath); sErr != nil || info.Size() == 0 {
		logger.LogError("Downloaded Mail-in-a-Box setup script is empty or missing.")
		return
	}
	logger.LogInfo("Running Mail-in-a-Box setup script...")
	setupCmd := exec.Command("bash", setupPath)
	setupCmd.Env = append(os.Environ(), "PRIMARY_HOSTNAME="+cleanDomain)
	setupCmd.Stdout = os.Stdout
	setupCmd.Stderr = os.Stderr
	if err := setupCmd.Run(); err != nil {
		logger.LogError(fmt.Sprintf("Mail-in-a-Box setup failed: %v", err))
		return
	}

	system.StartService("mailinabox")

	system.SaveState(map[string]string{
		"ENV_TYPE":    "native",
		"MAIL_DOMAIN": cfg.MailDomain,
		"ADMIN_EMAIL": cfg.AdminEmail,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("Mail-in-a-Box Native setup finished successfully.")
	printMailinaboxInstructions(cfg)
}

func printMailinaboxInstructions(cfg MailInABoxConfig) {
	fmt.Printf("\r\n%s------------------------------------------------------------%s\r\n", core.GREEN, core.NC)
	fmt.Printf("%s[OK] Mail-in-a-Box Suite Ready!%s\r\n", core.BOLD+core.GREEN, core.NC)
	fmt.Printf("  %sAdmin Panel:%s https://%s/admin\r\n", core.CYAN, core.NC, cfg.MailDomain)
	fmt.Printf("  %sWebmail:%s     https://%s/mail\r\n", core.CYAN, core.NC, cfg.MailDomain)
	fmt.Printf("  %sAdmin Login:%s %s | Pass: %s\r\n", core.YELLOW, core.NC, cfg.AdminEmail, cfg.AdminPass)
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
