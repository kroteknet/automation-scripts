// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Webmin System Administration Control Panel (webmin)
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

	"krotek-webmin-installer/core"
	"krotek-webmin-installer/logger"
	"krotek-webmin-installer/system"
	"krotek-webmin-installer/tui"
)

type WebminConfig struct {
	Mode      string
	Port      string
	SSL       bool
	AdminPass string
}

func generateRandomPassword(length int) string {
	bytes := make([]byte, length/2)
	for i := 0; i < 5; i++ {
		if _, err := rand.Read(bytes); err == nil {
			return hex.EncodeToString(bytes) + "W1!"
		}
	}
	logger.LogError("Cryptographic randomness failure: unable to generate secure admin password")
	os.Exit(1)
	return ""
}

// FreshInstallWizard handles the end-to-end installation pipeline for Webmin System Administration Control Panel.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(mode string) {
	cfg := WebminConfig{
		Mode:      mode,
		Port:      "10000",
		SSL:       true,
		AdminPass: generateRandomPassword(12),
	}

	PromptExistingConfigAction("Webmin System Administration Panel", "/etc/webmin/miniserv.conf")

	step := 1
	for {
		switch step {
		case 1:
			// Step 1: Panel Port
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Webmin Web Interface Port Configuration" + core.NC,
				"",
				"  Specify the port for Webmin system administration GUI.",
				"  " + core.BOLD + "Default Port:" + core.NC + " 10000",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Exit Setup", "Step 1/4: Webmin Port", cardLines)

			portVal, st := tui.ReadTextInput("Webmin GUI Port", cfg.Port)
			if st != 0 {
				return
			}
			portVal = strings.TrimSpace(portVal)
			if portVal == "" {
				portVal = "10000"
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
			// Step 2: SSL Mode
			tui.ClearScreen()
			sslOpts := []string{
				"Enable SSL / HTTPS Encryption (Self-Signed / Secure - Recommended)",
				"Plain HTTP Mode (No Encryption)",
				"<- Back to Port Selection",
			}
			sIdx, _, sSt := tui.SelectMenu("Step 2/4: SSL / HTTPS Mode", sslOpts)
			if sSt != 0 || sIdx == 2 {
				continue
			}
			cfg.SSL = (sIdx == 0)
			step = 3

		case 3:
			// Step 3: Admin Password
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Webmin Root / Admin Password" + core.NC,
				"",
				"  Webmin authenticates using the system root account or webmin admin.",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: Admin Authentication", cardLines)

			passVal, pSt := tui.ReadPasswordInput("Webmin Admin / Root Password (blank for random)")
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
			proto := "http"
			if cfg.SSL {
				proto = "https"
			}
			summary := []string{
				fmt.Sprintf("%sDeployment Mode:%s     %s", core.CYAN, core.NC, strings.ToUpper(cfg.Mode)),
				fmt.Sprintf("%sWebmin Port:%s         %s", core.CYAN, core.NC, cfg.Port),
				fmt.Sprintf("%sDashboard URL:%s       %s://<SERVER_IP>:%s", core.CYAN, core.NC, proto, cfg.Port),
				fmt.Sprintf("%sSSL Encryption:%s      %v", core.CYAN, core.NC, cfg.SSL),
				fmt.Sprintf("%sRoot/Admin Password:%s %s", core.CYAN, core.NC, cfg.AdminPass),
				"",
				"Ready to install Webmin system administration panel.",
			}
			tui.PrintInBox("--no-wait", "Step 4/4: Configuration Summary", summary)

			cOpts := []string{
				"Confirm & Proceed with Installation",
				"<- Change Configuration (Back)",
				"Cancel Installation",
			}
			cIdx, _, cSt := tui.SelectMenu("Confirm Webmin Deployment", cOpts)
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
	system.ConfigureFirewallRules(port)
}

func ExecuteDockerInstall(cfg WebminConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing Webmin Docker Container Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker Webmin installation...")

	if !system.EnsureDockerEngine() {
		return
	}

	composeDir := "/etc/webmin-docker"
	_ = os.MkdirAll(composeDir, 0755)

	composeYaml := fmt.Sprintf(`version: '3.8'
services:
  webmin:
    image: sameersbn/webmin:latest
    container_name: krotek-webmin
    restart: unless-stopped
    ports:
      - "%s:10000"
    environment:
      - ROOT_PASSWORD="%s"
    volumes:
      - %s/data:/var/lib/webmin
`, cfg.Port, escapeYamlString(cfg.AdminPass), composeDir)

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}
	configureFirewall(cfg.Port)

	if !system.RunDockerCompose(composeDir, "Deploying Webmin Docker Stack", true, "up", "-d") {
		return
	}

	system.SaveState(map[string]string{
		"ENV_TYPE":    "docker",
		"COMPOSE_DIR": composeDir,
		"PORT":        cfg.Port,
		"ADMIN_PASS":  cfg.AdminPass,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("Webmin Docker deployment complete.")
	printWebminInstructions(cfg)
}

func ExecuteNativeInstall(cfg WebminConfig) {
	if core.SysInfo.Family == "alpine" {
		tui.PrintInBox("WEBMIN Native" + " " + core.Sym.EmDash + " " + "Alpine Linux Not Supported", []string{
			core.BOLD + core.RED + "Native WEBMIN service is not supported on Alpine Linux." + core.NC,
			"Alpine Linux lacks required systemd/glibc dependencies.",
			"",
			core.BOLD + core.GREEN + "Recommendation:" + core.NC + " Use the Docker container deployment mode instead.",
		})
		logger.LogError("Native WEBMIN is unsupported on Alpine Linux. Aborting.")
		return
	}
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing Webmin Native Service ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native Webmin installation...")

	var pkgs []string
	switch core.SysInfo.Family {
	case "debian":
		pkgs = []string{"curl", "wget", "perl", "libnet-ssleay-perl", "openssl", "libauthen-pam-perl"}
	case "rhel":
		_ = exec.Command("dnf", "install", "-y", "epel-release").Run()
		_ = exec.Command("yum", "install", "-y", "epel-release").Run()
		pkgs = []string{"curl", "wget", "perl", "perl-Net-SSLeay", "openssl", "perl-Authen-PAM"}
	default:
		pkgs = []string{"curl", "wget", "perl", "openssl"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogWarn("Package installation had warnings, proceeding...")
	}

	configureFirewall(cfg.Port)

	// Add Webmin repository & install Webmin
	if core.SysInfo.Family == "debian" {
		wmTmpDir, wmErr := os.MkdirTemp("", "krotek-webmin-*")
		if wmErr != nil {
			logger.LogError("Failed to create secure temporary directory: " + wmErr.Error())
			return
		}
		_ = os.Chmod(wmTmpDir, 0700)
		defer os.RemoveAll(wmTmpDir)
		repoSetup := filepath.Join(wmTmpDir, "setup-repos.sh")
		if exec.Command("curl", "--proto", "=https", "--tlsv1.2", "-fsSL", "https://raw.githubusercontent.com/webmin/webmin/master/setup-repos.sh", "-o", repoSetup).Run() == nil {
			_ = exec.Command("sh", repoSetup, "-f").Run()
		}
		_ = exec.Command( // Dpkg options preserve existing configuration and prevent blocking terminal prompts in TUI & automation
			"apt-get", "install", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "--install-recommends", "webmin").Run()
	} else if core.SysInfo.Family == "rhel" {
		_ = exec.Command("curl", "--proto", "=https", "--tlsv1.2", "-fsSL", "-o", "/etc/yum.repos.d/webmin.repo", "https://raw.githubusercontent.com/webmin/webmin/master/webmin.repo").Run()
		_ = exec.Command("dnf", "install", "-y", "webmin").Run()
		_ = exec.Command("yum", "install", "-y", "webmin").Run()
	}

	// Update webmin port & SSL setting if configured
	miniservConf := "/etc/webmin/miniserv.conf"
	if _, err := os.Stat(miniservConf); err == nil {
		if content, err := os.ReadFile(miniservConf); err == nil {
			lines := strings.Split(string(content), "\n")
			var newLines []string
			for _, line := range lines {
				if strings.HasPrefix(line, "port=") {
					newLines = append(newLines, fmt.Sprintf("port=%s", cfg.Port))
				} else if strings.HasPrefix(line, "listen=") {
					newLines = append(newLines, fmt.Sprintf("listen=%s", cfg.Port))
				} else if strings.HasPrefix(line, "ssl=") {
					if cfg.SSL {
						newLines = append(newLines, "ssl=1")
					} else {
						newLines = append(newLines, "ssl=0")
					}
				} else {
					newLines = append(newLines, line)
				}
			}
			_ = system.WriteFileNoFollow(miniservConf, []byte(strings.Join(newLines, "\n")), 0600, "")
		}
	}

	// Set root password for webmin
	passCmd := exec.Command("perl", "-e", `
		my $dir = $ARGV[0]; my $user = $ARGV[1]; my $pass = $ENV{"WEBMIN_ADMIN_PASS"};
		system("/usr/share/webmin/changepass.pl", $dir, $user, $pass);
	`, "/etc/webmin", "root")
	passCmd.Env = append(os.Environ(), "WEBMIN_ADMIN_PASS="+cfg.AdminPass)
	_ = passCmd.Run()

	system.StartService("webmin")
	system.RestartService("webmin")

	system.SaveState(map[string]string{
		"ENV_TYPE":   "native",
		"PORT":       cfg.Port,
		"ADMIN_PASS": cfg.AdminPass,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("Webmin Native setup finished successfully.")
	printWebminInstructions(cfg)
}

func printWebminInstructions(cfg WebminConfig) {
	proto := "http"
	if cfg.SSL {
		proto = "https"
	}
	fmt.Printf("\r\n%s------------------------------------------------------------%s\r\n", core.GREEN, core.NC)
	fmt.Printf("%s[OK] Webmin Administration Panel Ready!%s\r\n", core.BOLD+core.GREEN, core.NC)
	fmt.Printf("  %sPanel URL:%s    %s://<SERVER_IP>:%s\r\n", core.CYAN, core.NC, proto, cfg.Port)
	fmt.Printf("  %sCredentials:%s  User: root | Pass: %s\r\n", core.YELLOW, core.NC, cfg.AdminPass)
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

func escapeYamlString(val string) string {
	v := strings.ReplaceAll(val, "\\", "\\\\")
	v = strings.ReplaceAll(v, "\"", "\\\"")
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", "")
	return v
}
