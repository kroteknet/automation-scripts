// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: vsftpd Secure FTP Daemon (vsftpd)
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

	"krotek-vsftpd-installer/core"
	"krotek-vsftpd-installer/logger"
	"krotek-vsftpd-installer/system"
	"krotek-vsftpd-installer/tui"
)

type VSFTPDConfig struct {
	Mode       string
	Port       string
	Username   string
	Password   string
	StorageDir string
	PasvMin    string
	PasvMax    string
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

// FreshInstallWizard handles the end-to-end installation pipeline for vsftpd Secure FTP Daemon.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(mode string) {
	cfg := VSFTPDConfig{
		Mode:       mode,
		Port:       "21",
		Username:   "ftpuser",
		Password:   generateRandomPassword(12),
		StorageDir: "/srv/ftp/upload",
		PasvMin:    "21100",
		PasvMax:    "21110",
	}

	PromptExistingConfigAction("vsftpd Secure FTP Server", "/etc/vsftpd.conf", "/etc/vsftpd/vsftpd.conf")

	step := 1
	for {
		switch step {
		case 1:
			// Step 1: Port Configuration
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "FTP Port Configuration" + core.NC,
				"",
				"  Specify primary FTP listening control port.",
				"  " + core.BOLD + "Default Port:" + core.NC + " 21",
				"  " + core.BOLD + "Passive Ports:" + core.NC + " 21100-21110",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Exit Setup", "Step 1/4: FTP Port Configuration", cardLines)

			portVal, st := tui.ReadTextInput("FTP Listening Port", cfg.Port)
			if st != 0 {
				return
			}
			portVal = strings.TrimSpace(portVal)
			if portVal == "" {
				portVal = "21"
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
			// Step 2: FTP User Credentials
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "FTP User Authentication Credentials" + core.NC,
				"",
				"  Create dedicated FTP account credentials for clients (FileZilla, WinSCP, etc).",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: User Credentials", cardLines)

			userVal, uSt := tui.ReadTextInput("FTP Username", cfg.Username)
			if uSt != 0 {
				step = 1
				continue
			}
			userVal = strings.TrimSpace(userVal)
			if userVal == "" || strings.ContainsAny(userVal, "\r\n:") {
				userVal = "ftpuser"
			}
			cfg.Username = userVal

			passVal, pSt := tui.ReadPasswordInput("FTP Password")
			if pSt != 0 {
				step = 1
				continue
			}
			passVal = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(passVal), "\r", ""), "\n", "")
			if passVal == "" {
				passVal = generateRandomPassword(12)
			}
			cfg.Password = passVal
			step = 3

		case 3:
			// Step 3: Storage Directory
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "FTP Storage & Upload Directory" + core.NC,
				"",
				"  Directory where uploaded files will be stored.",
				"  " + core.BOLD + "Default:" + core.NC + " /srv/ftp/upload",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: Storage Directory", cardLines)

			dirVal, dSt := tui.ReadTextInput("FTP Storage Directory Path", cfg.StorageDir)
			if dSt != 0 {
				step = 2
				continue
			}
			dirVal = strings.TrimSpace(dirVal)
			if dirVal == "" {
				dirVal = "/srv/ftp/upload"
			}
			if err := system.ValidateSafeDirectoryPath(dirVal); err != nil {
				tui.PrintInBox("Invalid Storage Directory", []string{
					core.RED + err.Error() + core.NC,
				})
				continue
			}
			cfg.StorageDir = dirVal
			step = 4

		case 4:
			// Step 4: Review Summary Card
			tui.ClearScreen()
			summary := []string{
				fmt.Sprintf("%sDeployment Mode:%s     %s", core.CYAN, core.NC, strings.ToUpper(cfg.Mode)),
				fmt.Sprintf("%sFTP Port:%s            %s/TCP (Passive: %s-%s)", core.CYAN, core.NC, cfg.Port, cfg.PasvMin, cfg.PasvMax),
				fmt.Sprintf("%sFTP Username:%s        %s", core.CYAN, core.NC, cfg.Username),
				fmt.Sprintf("%sFTP Password:%s        %s", core.CYAN, core.NC, cfg.Password),
				fmt.Sprintf("%sStorage Path:%s        %s", core.CYAN, core.NC, cfg.StorageDir),
				"",
				"Ready to install and configure VSFTPD server.",
			}
			tui.PrintInBox("--no-wait", "Step 4/4: Configuration Summary", summary)

			cOpts := []string{
				"Confirm & Proceed with Installation",
				"<- Change Configuration (Back)",
				"Cancel Installation",
			}
			cIdx, _, cSt := tui.SelectMenu("Confirm VSFTPD Deployment", cOpts)
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

func ExecuteDockerInstall(cfg VSFTPDConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing VSFTPD Docker Container Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker VSFTPD installation...")

	if !system.EnsureDockerEngine() {
		return
	}

	if err := system.ValidateSafeDirectoryPath(cfg.StorageDir); err != nil {
		logger.LogError("Aborting: " + err.Error())
		return
	}

	_ = os.MkdirAll(cfg.StorageDir, 0755)

	composeDir := "/etc/vsftpd-docker"
	_ = os.MkdirAll(composeDir, 0755)

	composeYaml := fmt.Sprintf(`version: '3.8'
services:
  vsftpd:
    image: fauria/vsftpd:latest
    container_name: krotek-vsftpd
    restart: unless-stopped
    environment:
      - FTP_USER=%s
      - FTP_PASS=%s
      - PASV_MIN_PORT=%s
      - PASV_MAX_PORT=%s
    volumes:
      - %s:/home/vsftpd/%s
    ports:
      - "%s:21"
      - "%s-%s:%s-%s"
`, cfg.Username, cfg.Password, cfg.PasvMin, cfg.PasvMax, cfg.StorageDir, cfg.Username, cfg.Port, cfg.PasvMin, cfg.PasvMax, cfg.PasvMin, cfg.PasvMax)

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}

	if !system.RunDockerCompose(composeDir, "Deploying VSFTPD Docker Stack", true, "up", "-d") {
		return
	}

	system.SaveState(map[string]string{
		"ENV_TYPE":    "docker",
		"COMPOSE_DIR": composeDir,
		"USERNAME":    cfg.Username,
		"PASSWORD":    cfg.Password,
		"STORAGE_DIR": cfg.StorageDir,
		"PORT":        cfg.Port,
		"PASV_MIN":    cfg.PasvMin,
		"PASV_MAX":    cfg.PasvMax,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("VSFTPD Docker deployment complete.")
	printFTPInstructions(cfg)
}

func ExecuteNativeInstall(cfg VSFTPDConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing VSFTPD Native Service ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native VSFTPD installation...")

	var pkgs []string
	switch core.SysInfo.Family {
	case "alpine":
		pkgs = []string{"vsftpd", "vsftpd-openrc"}
	case "debian":
		pkgs = []string{"vsftpd", "ssl-cert"}
	case "rhel":
		pkgs = []string{"vsftpd"}
	default:
		pkgs = []string{"vsftpd", "ssl-cert"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogError("Failed to install VSFTPD packages.")
		return
	}

	if err := system.ValidateSafeDirectoryPath(cfg.StorageDir); err != nil {
		logger.LogError("Aborting: " + err.Error())
		return
	}

	_ = os.MkdirAll(cfg.StorageDir, 0755)

	// Create user with storage home and disabled system shell login (FTP only)
	nologinShell := "/usr/sbin/nologin"
	if _, err := os.Stat("/usr/sbin/nologin"); err != nil {
		if _, err := os.Stat("/sbin/nologin"); err == nil {
			nologinShell = "/sbin/nologin"
		} else {
			nologinShell = "/bin/false"
		}
	}

	if system.CommandExists("useradd") {
		_ = exec.Command("useradd", "-m", "-d", cfg.StorageDir, "-s", nologinShell, cfg.Username).Run()
	} else {
		_ = exec.Command("adduser", "-h", cfg.StorageDir, "-s", nologinShell, "-D", cfg.Username).Run()
	}

	// Set password safely via chpasswd stdin without leaking into process table or shell
	chpasswdCmd := exec.Command("chpasswd")
	chpasswdCmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s\n", cfg.Username, cfg.Password))
	_ = chpasswdCmd.Run()
	system.SecureWebRootPermissions(cfg.StorageDir, cfg.Username)

	confPath := "/etc/vsftpd.conf"
	if _, err := os.Stat("/etc/vsftpd/vsftpd.conf"); err == nil {
		confPath = "/etc/vsftpd/vsftpd.conf"
	}

	// Backup original config
	_ = exec.Command("cp", "-a", confPath, confPath+".bak").Run()

	vsftpdConf := fmt.Sprintf(`listen=YES
listen_ipv6=NO
anonymous_enable=NO
local_enable=YES
write_enable=YES
check_shell=NO
local_umask=022
dirmessage_enable=YES
use_localtime=YES
xferlog_enable=YES
connect_from_port_20=YES
chroot_local_user=YES
allow_writeable_chroot=YES
listen_port=%s
pasv_enable=YES
pasv_min_port=%s
pasv_max_port=%s
secure_chroot_dir=/var/run/vsftpd/empty
pam_service_name=vsftpd
rsa_cert_file=/etc/ssl/certs/ssl-cert-snakeoil.pem
rsa_private_key_file=/etc/ssl/private/ssl-cert-snakeoil.key
ssl_enable=YES
allow_anon_ssl=NO
force_local_data_ssl=YES
force_local_logins_ssl=YES
ssl_tlsv1_2=YES
ssl_tlsv1_3=YES
`, cfg.Port, cfg.PasvMin, cfg.PasvMax)

	_ = os.MkdirAll("/var/run/vsftpd/empty", 0755)
	if _, err := os.Stat(confPath); os.IsNotExist(err) || core.OverwriteExistingConfig {
		_ = system.WriteFileNoFollow(confPath, []byte(vsftpdConf), 0644, "")
	}

	// Ensure self-signed certificate exists for vsftpd SSL
	if _, err := os.Stat("/etc/ssl/certs/ssl-cert-snakeoil.pem"); os.IsNotExist(err) {
		_ = os.MkdirAll("/etc/ssl/certs", 0755)
		_ = os.MkdirAll("/etc/ssl/private", 0700)
		_ = exec.Command("openssl", "req", "-x509", "-nodes", "-days", "3650", "-newkey", "rsa:2048",
			"-keyout", "/etc/ssl/private/ssl-cert-snakeoil.key",
			"-out", "/etc/ssl/certs/ssl-cert-snakeoil.pem",
			"-subj", "/CN=vsftpd/O=Krotek").Run()
	}

	system.StartService("vsftpd")
	system.RestartService("vsftpd")

	// Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	system.ConfigureFirewallRules(cfg.Port+"/tcp", fmt.Sprintf("%s:%s/tcp", cfg.PasvMin, cfg.PasvMax))

	system.SaveState(map[string]string{
		"ENV_TYPE":    "native",
		"CONF_PATH":   confPath,
		"USERNAME":    cfg.Username,
		"PASSWORD":    cfg.Password,
		"STORAGE_DIR": cfg.StorageDir,
		"PORT":        cfg.Port,
		"PASV_MIN":    cfg.PasvMin,
		"PASV_MAX":    cfg.PasvMax,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("VSFTPD Native setup finished successfully.")
	printFTPInstructions(cfg)
}

func printFTPInstructions(cfg VSFTPDConfig) {
	fmt.Printf("\r\n%s------------------------------------------------------------%s\r\n", core.GREEN, core.NC)
	fmt.Printf("%s[OK] VSFTPD Secure FTP Server Ready!%s Connect via FileZilla / Cyberduck:\r\n", core.BOLD+core.GREEN, core.NC)
	fmt.Printf("  %sHost:%s         <SERVER_IP> (Port: %s)\r\n", core.CYAN, core.NC, cfg.Port)
	fmt.Printf("  %sCredentials:%s  User: %s | Pass: %s\r\n", core.YELLOW, core.NC, cfg.Username, cfg.Password)
	fmt.Printf("  %sDirectory:%s    %s\r\n", core.CYAN, core.NC, cfg.StorageDir)
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
