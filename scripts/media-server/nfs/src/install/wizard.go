// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: NFS Kernel Network File Server (nfs)
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
	"strings"

	"krotek-nfs-installer/core"
	"krotek-nfs-installer/logger"
	"krotek-nfs-installer/system"
	"krotek-nfs-installer/tui"
)

type NFSConfig struct {
	Mode        string
	ExportDir   string
	Clients     string
	Permissions string
	PermDesc    string
	Port        string
}

// FreshInstallWizard handles the end-to-end installation pipeline for NFS Kernel Network File Server.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(mode string) {
	cfg := NFSConfig{
		Mode:        mode,
		ExportDir:   "/srv/nfs/share",
		Clients:     "*",
		Permissions: "rw,sync,no_subtree_check,root_squash",
		PermDesc:    "Read/Write (Squash Root to nobody - Recommended)",
		Port:        "2049",
	}

	PromptExistingConfigAction("NFS Network File System", "/etc/exports")

	step := 1
	for {
		switch step {
		case 1:
			// Step 1: Export Directory
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "NFS Share Directory Selection" + core.NC,
				"",
				"  Specify the directory on this host to export over NFS.",
				"  This path will be created automatically if it does not exist.",
				"  " + core.BOLD + "Default:" + core.NC + " /srv/nfs/share",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Exit Setup", "Step 1/4: Export Share Directory", cardLines)

			val, st := tui.ReadTextInput("NFS Export Directory Path", cfg.ExportDir)
			if st != 0 {
				return
			}
			val = strings.TrimSpace(val)
			if val == "" {
				val = "/srv/nfs/share"
			}
			if err := system.ValidateSafeDirectoryPath(val); err != nil {
				tui.PrintInBox("Invalid Export Directory", []string{
					core.RED + err.Error() + core.NC,
				})
				continue
			}
			cfg.ExportDir = val
			step = 2

		case 2:
			// Step 2: Allowed Clients / Subnet
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Client Access Control" + core.NC,
				"",
				"  Specify which clients or networks are authorized to mount this NFS share.",
				"  Examples:",
				"    *               -> All clients (LAN & local access)",
				"    192.168.1.0/24  -> Entire local subnet",
				"    192.168.1.50    -> Single specific IP",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: Client IP / Subnet Access", cardLines)

			val, st := tui.ReadTextInput("Allowed Client(s) [e.g. * or CIDR]", cfg.Clients)
			if st != 0 {
				step = 1
				continue
			}
			val = strings.TrimSpace(val)
			if val == "" {
				val = "*"
			}
			// Sanitize client input to strictly allow CIDR, IP, wildcard, or safe hostname characters
			val = regexp.MustCompile(`[^a-zA-Z0-9.*\/_-]`).ReplaceAllString(val, "")
			if val == "" {
				val = "*"
			}
			cfg.Clients = val
			step = 3

		case 3:
			// Step 3: Access Permissions
			tui.ClearScreen()
			permOptions := []string{
				"Read/Write (Squash Root to nobody - Recommended)",
				"Read/Write (No Root Squash - Full Root Access)",
				"Read-Only (ro,sync,no_subtree_check)",
				"<- Back to Previous Step",
			}
			pIdx, _, pSt := tui.SelectMenu("Step 3/4: Access Permissions & Security", permOptions)
			if pSt != 0 || pIdx == 3 {
				step = 2
				continue
			}
			switch pIdx {
			case 0:
				cfg.Permissions = "rw,sync,no_subtree_check,root_squash"
				cfg.PermDesc = "Read/Write (Squash Root to nobody)"
			case 1:
				if cfg.Clients == "*" {
					tui.ClearScreen()
					tui.PrintInBox("Security Policy Violation", []string{
						core.RED + core.BOLD + "[CRITICAL SECURITY RESTRICTION]" + core.NC,
						"",
						"  NFS exports allowing 'no_root_squash' combined with wildcard '*'",
						"  allow ANY network client to gain immediate root privilege escalation",
						"  by creating SUID binaries on this host.",
						"",
						core.YELLOW + "  To use 'no_root_squash', specify a dedicated trusted IP or CIDR in Step 2." + core.NC,
						"",
						core.CYAN + "  Reverting to 'root_squash' for wildcard clients." + core.NC,
					})
					tui.ReadKey()
					cfg.Permissions = "rw,sync,no_subtree_check,root_squash"
					cfg.PermDesc = "Read/Write (Squash Root to nobody - Enforced for wildcard *)"
				} else {
					cfg.Permissions = "rw,sync,no_subtree_check,no_root_squash"
					cfg.PermDesc = "Read/Write (No Root Squash - Root Access for " + cfg.Clients + ")"
				}
			case 2:
				cfg.Permissions = "ro,sync,no_subtree_check"
				cfg.PermDesc = "Read-Only Archive"
			}
			step = 4

		case 4:
			// Step 4: Review Summary Card
			tui.ClearScreen()
			summary := []string{
				fmt.Sprintf("%sDeployment Mode:%s     %s", core.CYAN, core.NC, strings.ToUpper(cfg.Mode)),
				fmt.Sprintf("%sExport Directory:%s    %s", core.CYAN, core.NC, cfg.ExportDir),
				fmt.Sprintf("%sAllowed Clients:%s     %s", core.CYAN, core.NC, cfg.Clients),
				fmt.Sprintf("%sAccess Mode:%s         %s", core.CYAN, core.NC, cfg.PermDesc),
				fmt.Sprintf("%sNFS Server Port:%s     %s/TCP, %s/UDP", core.CYAN, core.NC, cfg.Port, cfg.Port),
				"",
				"Ready to install and configure NFS server.",
			}
			tui.PrintInBox("--no-wait", "Step 4/4: Configuration Summary", summary)

			cOpts := []string{
				"Confirm & Proceed with Installation",
				"<- Change Configuration (Back)",
				"Cancel Installation",
			}
			cIdx, _, cSt := tui.SelectMenu("Confirm NFS Deployment", cOpts)
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

func ExecuteDockerInstall(cfg NFSConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing NFS Docker Container Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker NFS installation...")

	if !system.EnsureDockerEngine() {
		return
	}

	cfg.ExportDir = filepath.Clean(strings.TrimSpace(cfg.ExportDir))
	if err := system.ValidateSafeDirectoryPath(cfg.ExportDir); err != nil || strings.ContainsAny(cfg.ExportDir, "\r\n#;\"'") || !strings.HasPrefix(cfg.ExportDir, "/") {
		logger.LogError("Invalid NFS export directory specified.")
		return
	}
	if strings.ContainsAny(cfg.Permissions, "\r\n#;\"'") {
		logger.LogError("Invalid NFS permissions parameter specified.")
		return
	}

	_ = os.MkdirAll(cfg.ExportDir, 0755)

	composeDir := "/etc/nfs-docker"
	_ = os.MkdirAll(composeDir, 0755)

	exportEnv := fmt.Sprintf("%s %s(%s)", cfg.ExportDir, cfg.Clients, cfg.Permissions)

	composeYaml := fmt.Sprintf(`version: '3.8'
services:
  nfs:
    image: erichough/nfs-server:latest
    container_name: krotek-nfs
    restart: unless-stopped
    privileged: true
    environment:
      - "NFS_EXPORT_0=%s"
    volumes:
      - %s:%s
    ports:
      - "%s:2049"
      - "2049:2049/udp"
      - "111:111"
      - "111:111/udp"
`, exportEnv, cfg.ExportDir, cfg.ExportDir, cfg.Port)

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}

	if !system.RunDockerCompose(composeDir, "Deploying NFS Docker Stack", true, "up", "-d") {
		return
	}

	system.SaveState(map[string]string{
		"ENV_TYPE":    "docker",
		"COMPOSE_DIR": composeDir,
		"EXPORT_DIR":  cfg.ExportDir,
		"CLIENTS":     cfg.Clients,
		"PERMS":       cfg.Permissions,
		"PORT":        cfg.Port,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("NFS Docker deployment complete.")
	printClientMountInstructions(cfg)
}

func ExecuteNativeInstall(cfg NFSConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing NFS Native Service ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native NFS installation...")

	var pkgs []string
	switch core.SysInfo.Family {
	case "alpine":
		pkgs = []string{"nfs-utils", "nfs-utils-openrc", "rpcbind"}
	case "debian":
		pkgs = []string{"nfs-kernel-server", "nfs-common", "rpcbind"}
	case "rhel":
		pkgs = []string{"nfs-utils", "rpcbind"}
	default:
		pkgs = []string{"nfs-kernel-server", "nfs-common", "rpcbind"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogError("Failed to install NFS packages.")
		return
	}

	cfg.ExportDir = filepath.Clean(strings.TrimSpace(cfg.ExportDir))
	if err := system.ValidateSafeDirectoryPath(cfg.ExportDir); err != nil || strings.ContainsAny(cfg.ExportDir, "\r\n#;\"'") || !strings.HasPrefix(cfg.ExportDir, "/") {
		logger.LogError("Invalid NFS export directory specified.")
		return
	}
	if strings.ContainsAny(cfg.Permissions, "\r\n#;\"'") {
		logger.LogError("Invalid NFS permissions parameter specified.")
		return
	}

	_ = os.MkdirAll(cfg.ExportDir, 0755)

	// Backup existing /etc/exports if present
	if _, err := os.Stat("/etc/exports"); err == nil {
		_ = exec.Command("cp", "-a", "/etc/exports", "/etc/exports.bak").Run()
	}

	// Append export rule safely without following symlinks
	exportLine := fmt.Sprintf("%s %s(%s)\n", cfg.ExportDir, cfg.Clients, cfg.Permissions)
	if err := system.AppendFileNoFollow("/etc/exports", []byte(exportLine), 0644); err != nil {
		logger.LogWarn(fmt.Sprintf("Failed to update /etc/exports: %v", err))
	}

	_ = exec.Command("exportfs", "-rav").Run()
	if core.SysInfo.Family == "alpine" {
		system.StartService("nfs")
	} else {
		if !system.StartService("nfs-server") {
			_ = system.StartService("nfs-kernel-server")
		}
		if !system.RestartService("nfs-server") {
			_ = system.RestartService("nfs-kernel-server")
		}
	}

	// Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	if system.CommandExists("ufw") {
		_ = exec.Command("ufw", "allow", "nfs").Run()
	}
	if system.CommandExists("firewall-cmd") {
		_ = exec.Command("firewall-cmd", "--add-service=nfs", "--permanent").Run()
	}
	system.ConfigureFirewallRules("2049/tcp", "2049/udp", "111/tcp", "111/udp")

	system.SaveState(map[string]string{
		"ENV_TYPE":   "native",
		"EXPORT_DIR": cfg.ExportDir,
		"CLIENTS":    cfg.Clients,
		"PERMS":      cfg.Permissions,
		"PORT":       cfg.Port,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("NFS Native setup finished successfully.")
	printClientMountInstructions(cfg)
}

func printClientMountInstructions(cfg NFSConfig) {
	fmt.Printf("\r\n%s------------------------------------------------------------%s\r\n", core.GREEN, core.NC)
	fmt.Printf("%s[OK] NFS Export Ready!%s Mount this share on client machines using:\r\n", core.BOLD+core.GREEN, core.NC)
	fmt.Printf("  %smkdir -p /mnt/nfs_share%s\r\n", core.CYAN, core.NC)
	fmt.Printf("  %smount -t nfs <SERVER_IP>:%s /mnt/nfs_share%s\r\n", core.CYAN, cfg.ExportDir, core.NC)
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
