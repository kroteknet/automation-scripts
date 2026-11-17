// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Samba SMB/CIFS Network File Sharing (samba)
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
	"strings"

	"krotek-samba-installer/core"
	"krotek-samba-installer/logger"
	"krotek-samba-installer/system"
	"krotek-samba-installer/tui"
)

type SambaConfig struct {
	Mode       string
	ShareName  string
	SharePath  string
	AccessType string // "guest" or "auth"
	Username   string
	Password   string
	Port       string
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

// FreshInstallWizard handles the end-to-end installation pipeline for Samba SMB/CIFS Network File Sharing.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(mode string) {
	cfg := SambaConfig{
		Mode:       mode,
		ShareName:  "share",
		SharePath:  "/srv/samba/share",
		AccessType: "auth",
		Username:   "smbuser",
		Password:   generateRandomPassword(12),
		Port:       "445",
	}

	PromptExistingConfigAction("Samba SMB File Sharing", "/etc/samba/smb.conf")

	step := 1
	for {
		switch step {
		case 1:
			// Step 1: Share Directory
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Samba Storage Directory" + core.NC,
				"",
				"  Specify the directory on this host to share across the Windows/LAN network.",
				"  This path will be created automatically if it does not exist.",
				"  " + core.BOLD + "Default:" + core.NC + " /srv/samba/share",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Exit Setup", "Step 1/4: Samba Share Directory", cardLines)

			val, st := tui.ReadTextInput("Share Directory Path", cfg.SharePath)
			if st != 0 {
				return
			}
			val = strings.TrimSpace(val)
			if val == "" {
				val = "/srv/samba/share"
			}
			if err := system.ValidateSafeDirectoryPath(val); err != nil {
				tui.PrintInBox("Invalid Share Directory", []string{
					core.RED + err.Error() + core.NC,
				})
				continue
			}
			cfg.SharePath = val
			step = 2

		case 2:
			// Step 2: Share Name & Access Type
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Share Name & Access Scope" + core.NC,
				"",
				"  Define the network share name visible in Windows Explorer / Network.",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: Share Name", cardLines)

			nameVal, st := tui.ReadTextInput("Network Share Name", cfg.ShareName)
			if st != 0 {
				step = 1
				continue
			}
			nameVal = strings.TrimSpace(nameVal)
			if nameVal == "" {
				nameVal = "share"
			}
			cfg.ShareName = nameVal

			// Select Access Type
			tui.ClearScreen()
			secOpts := []string{
				"Guest / Public Access (No Password - Anonymous Read/Write)",
				"Authenticated Access (Protected with Username & Password)",
				"<- Back to Share Name",
			}
			sIdx, _, sSt := tui.SelectMenu("Step 2/4: Choose Access Security", secOpts)
			if sSt != 0 || sIdx == 2 {
				continue
			}
			if sIdx == 0 {
				cfg.AccessType = "guest"
				step = 4
			} else {
				cfg.AccessType = "auth"
				step = 3
			}

		case 3:
			// Step 3: User Credentials
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Samba User Authentication Credentials" + core.NC,
				"",
				"  Create a dedicated credentials pair for connecting to the share.",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: User Credentials", cardLines)

			userVal, uSt := tui.ReadTextInput("Samba Username", cfg.Username)
			if uSt != 0 {
				step = 2
				continue
			}
			userVal = strings.TrimSpace(userVal)
			if userVal == "" {
				userVal = "smbuser"
			}
			cfg.Username = userVal

			passVal, pSt := tui.ReadPasswordInput("Samba Password")
			if pSt != 0 {
				step = 2
				continue
			}
			passVal = strings.TrimSpace(passVal)
			if passVal == "" {
				passVal = generateRandomPassword(12)
			}
			cfg.Password = passVal
			step = 4

		case 4:
			// Step 4: Review Summary Card
			tui.ClearScreen()
			summary := []string{
				fmt.Sprintf("%sDeployment Mode:%s     %s", core.CYAN, core.NC, strings.ToUpper(cfg.Mode)),
				fmt.Sprintf("%sShare Name:%s          \\\\<SERVER>\\%s", core.CYAN, core.NC, cfg.ShareName),
				fmt.Sprintf("%sShare Path:%s          %s", core.CYAN, core.NC, cfg.SharePath),
			}
			if cfg.AccessType == "guest" {
				summary = append(summary, fmt.Sprintf("%sAccess Security:%s    %sGuest / Anonymous (No Password)%s", core.GREEN, core.NC, core.BOLD, core.NC))
			} else {
				summary = append(summary, fmt.Sprintf("%sAccess Security:%s    %sProtected / Authenticated%s", core.YELLOW, core.NC, core.BOLD, core.NC))
				summary = append(summary, fmt.Sprintf("%sUsername:%s           %s", core.CYAN, core.NC, cfg.Username))
				summary = append(summary, fmt.Sprintf("%sPassword:%s           %s", core.CYAN, core.NC, cfg.Password))
			}
			summary = append(summary, fmt.Sprintf("%sSamba SMB Port:%s     %s/TCP", core.CYAN, core.NC, cfg.Port))
			summary = append(summary, "")
			summary = append(summary, "Ready to deploy Samba Windows file sharing service.")

			tui.PrintInBox("--no-wait", "Step 4/4: Configuration Summary", summary)

			cOpts := []string{
				"Confirm & Proceed with Installation",
				"<- Change Configuration (Back)",
				"Cancel Installation",
			}
			cIdx, _, cSt := tui.SelectMenu("Confirm Samba Deployment", cOpts)
			if cSt != 0 || cIdx == 2 {
				return
			}
			if cIdx == 1 {
				if cfg.AccessType == "auth" {
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

func ExecuteDockerInstall(cfg SambaConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing Samba Docker Container Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker Samba installation...")

	if !system.EnsureDockerEngine() {
		return
	}

	if err := system.ValidateSafeDirectoryPath(cfg.SharePath); err != nil {
		logger.LogError("Invalid share path specified: " + err.Error())
		return
	}

	_ = os.MkdirAll(cfg.SharePath, 0755)

	composeDir := "/etc/samba-docker"
	_ = os.MkdirAll(composeDir, 0755)

	var cmdArgs string
	if cfg.AccessType == "guest" {
		cmdArgs = fmt.Sprintf("-s \"%s;/mount;yes;no;no;all;none\"", escapeYamlString(cfg.ShareName))
	} else {
		cmdArgs = fmt.Sprintf("-u \"%s;%s\" -s \"%s;/mount;yes;no;no;%s\"", escapeYamlString(cfg.Username), escapeYamlString(cfg.Password), escapeYamlString(cfg.ShareName), escapeYamlString(cfg.Username))
	}

	composeYaml := fmt.Sprintf(`version: '3.8'
services:
  samba:
    image: dperson/samba:latest
    container_name: krotek-samba
    restart: unless-stopped
    command: "%s"
    volumes:
      - %s:/mount
    ports:
      - "%s:445"
      - "139:139"
`, cmdArgs, cfg.SharePath, cfg.Port)

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}

	if !system.RunDockerCompose(composeDir, "Deploying Samba Docker Stack", true, "up", "-d") {
		return
	}

	system.SaveState(map[string]string{
		"ENV_TYPE":    "docker",
		"COMPOSE_DIR": composeDir,
		"SHARE_NAME":  cfg.ShareName,
		"SHARE_PATH":  cfg.SharePath,
		"ACCESS_TYPE": cfg.AccessType,
		"USERNAME":    cfg.Username,
		"PASSWORD":    cfg.Password,
		"PORT":        cfg.Port,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("Samba Docker deployment complete.")
	printSambaInstructions(cfg)
}

func ExecuteNativeInstall(cfg SambaConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing Samba Native Service ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native Samba installation...")

	var pkgs []string
	switch core.SysInfo.Family {
	case "alpine":
		pkgs = []string{"samba", "samba-common", "samba-client", "samba-server-openrc"}
	case "debian":
		pkgs = []string{"samba", "samba-common-bin"}
	case "rhel":
		pkgs = []string{"samba", "samba-common", "samba-client"}
	default:
		pkgs = []string{"samba", "samba-common-bin"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogError("Failed to install Samba packages.")
		return
	}

	cleanShareName := strings.TrimSpace(cfg.ShareName)
	for _, ch := range cleanShareName {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			cleanShareName = "share"
			break
		}
	}
	if cleanShareName == "" {
		cleanShareName = "share"
	}
	cfg.ShareName = cleanShareName
	cfg.SharePath = filepath.Clean(strings.TrimSpace(cfg.SharePath))
	if err := system.ValidateSafeDirectoryPath(cfg.SharePath); err != nil || strings.ContainsAny(cfg.SharePath, "\r\n") || !strings.HasPrefix(cfg.SharePath, "/") {
		logger.LogError("Invalid share path specified.")
		return
	}

	_ = os.MkdirAll(cfg.SharePath, 0755)

	// Backup existing /etc/samba/smb.conf
	if _, err := os.Stat("/etc/samba/smb.conf"); err == nil {
		_ = exec.Command("cp", "-a", "/etc/samba/smb.conf", "/etc/samba/smb.conf.bak").Run()
	}

	var shareBlock string
	if cfg.AccessType == "guest" {
		shareBlock = fmt.Sprintf("\n[%s]\n   path = %s\n   browseable = yes\n   read only = no\n   guest ok = yes\n   create mask = 0775\n   directory mask = 0775\n", cfg.ShareName, cfg.SharePath)
	} else {
		// Sanitize username to prevent argument or account injection
		cleanUsername := strings.TrimSpace(cfg.Username)
		for _, ch := range cleanUsername {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
				cleanUsername = "smbuser"
				break
			}
		}
		if cleanUsername == "" {
			cleanUsername = "smbuser"
		}
		cfg.Username = cleanUsername

		// Create Linux user if not exists
		if system.CommandExists("useradd") {
			_ = exec.Command("useradd", "-M", "-s", "/sbin/nologin", cfg.Username).Run()
		} else {
			_ = exec.Command("adduser", "-H", "-s", "/sbin/nologin", "-D", cfg.Username).Run()
		}
		// Set SMB password safely via stdin without shell execution or leaking into process table
		smbCmd := exec.Command("smbpasswd", "-s", "-a", cfg.Username)
		smbCmd.Stdin = strings.NewReader(fmt.Sprintf("%s\n%s\n", cfg.Password, cfg.Password))
		_ = smbCmd.Run()

		system.SecureWebRootPermissions(cfg.SharePath, cfg.Username)

		shareBlock = fmt.Sprintf("\n[%s]\n   path = %s\n   browseable = yes\n   read only = no\n   valid users = %s\n   create mask = 0775\n   directory mask = 0775\n", cfg.ShareName, cfg.SharePath, cfg.Username)
	}

	if err := system.AppendFileNoFollow("/etc/samba/smb.conf", []byte(shareBlock), 0644); err != nil {
		logger.LogWarn(fmt.Sprintf("Failed to update /etc/samba/smb.conf: %v", err))
	}

	if core.SysInfo.Family == "alpine" {
		system.StartService("samba")
	} else {
		if !system.StartService("smbd") {
			_ = system.StartService("smb")
		}
		if !system.StartService("nmbd") {
			_ = system.StartService("nmb")
		}
		if !system.RestartService("smbd") {
			_ = system.RestartService("smb")
		}
	}

	// Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	if system.CommandExists("ufw") {
		_ = exec.Command("ufw", "allow", "samba").Run()
	}
	if system.CommandExists("firewall-cmd") {
		_ = exec.Command("firewall-cmd", "--add-service=samba", "--permanent").Run()
	}
	system.ConfigureFirewallRules("139/tcp", "445/tcp", "137/udp", "138/udp")

	system.SaveState(map[string]string{
		"ENV_TYPE":    "native",
		"SHARE_NAME":  cfg.ShareName,
		"SHARE_PATH":  cfg.SharePath,
		"ACCESS_TYPE": cfg.AccessType,
		"USERNAME":    cfg.Username,
		"PASSWORD":    cfg.Password,
		"PORT":        cfg.Port,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("Samba Native setup finished successfully.")
	printSambaInstructions(cfg)
}

func printSambaInstructions(cfg SambaConfig) {
	fmt.Printf("\r\n%s------------------------------------------------------------%s\r\n", core.GREEN, core.NC)
	fmt.Printf("%s[OK] Samba Windows Share Ready!%s Connect from Windows / macOS / Linux:\r\n", core.BOLD+core.GREEN, core.NC)
	fmt.Printf("  %sWindows Explorer:%s \\\\<SERVER_IP>\\%s\r\n", core.CYAN, core.NC, cfg.ShareName)
	fmt.Printf("  %smacOS Finder:%s     smb://<SERVER_IP>/%s\r\n", core.CYAN, core.NC, cfg.ShareName)
	if cfg.AccessType == "auth" {
		fmt.Printf("  %sCredentials:%s      User: %s | Pass: %s\r\n", core.YELLOW, core.NC, cfg.Username, cfg.Password)
	} else {
		fmt.Printf("  %sCredentials:%s      Guest / No Password Required\r\n", core.GREEN, core.NC)
	}
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
