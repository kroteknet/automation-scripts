// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: VSFTPD (media-server)
// File:    media-server/vsftpd/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-vsftpd-installer/core"
	"krotek-vsftpd-installer/logger"
	"krotek-vsftpd-installer/system"
	"krotek-vsftpd-installer/tui"
)

// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	storageDir := state["STORAGE_DIR"]
	if storageDir == "" {
		storageDir = "/srv/ftp/upload"
	}
	username := state["USERNAME"]
	if username == "" {
		username = "ftpuser"
	}

	// ─── Step 1: Pre-removal Backup Archive ──────────────────────────────────
	// Create a timestamped tar.gz archive of configuration and data files in /root.
	tui.ClearScreen()
	backupChoice := tui.AskYesNo("Uninstall [1/3]" + " " + core.Sym.EmDash + " " + "Create pre-removal configuration backup archive?")
	if backupChoice == 2 || backupChoice == -1 {
		return
	}
	var backupFile string
	if backupChoice == 0 {
		timestamp := time.Now().Format("20060102_150405")
		backupFile = fmt.Sprintf("/root/vsftpd_backup_%s.tar.gz", timestamp)
		if envType == "docker" {
			cd := state["COMPOSE_DIR"]
			if cd != "" {
				_ = exec.Command("tar", "-czf", backupFile, cd).Run()
			}
		} else {
			confPath := state["CONF_PATH"]
			if confPath == "" {
				confPath = "/etc/vsftpd.conf"
			}
			if _, err := os.Stat(confPath); err == nil {
				_ = exec.Command("tar", "-czf", backupFile, confPath).Run()
			}
		}
	}

	// ─── Step 2: Data & Configuration Purge Decision ────────────────────────
	// Prompt whether to wipe application databases, keys, and storage or preserve them.
	tui.ClearScreen()
	purgeDir := false
	purgeChoice := tui.AskYesNo(fmt.Sprintf("Uninstall [2/3]" + " " + core.Sym.EmDash + " " + "Purge all files in FTP storage directory (%s)? (No keeps files)", storageDir))
	if purgeChoice == 2 || purgeChoice == -1 {
		return
	}
	if purgeChoice == 0 {
		purgeDir = true
	}

	// ─── Step 3: Confirmation & Action Review ────────────────────────────────
	// Display a summary card of all planned uninstallation actions before execution.
	tui.ClearScreen()
	summary := []string{
		fmt.Sprintf("%sAction:%s             Complete VSFTPD Removal", core.RED, core.NC),
		fmt.Sprintf("%sEnvironment:%s        %s", core.CYAN, core.NC, envType),
		fmt.Sprintf("%sFTP User:%s           %s", core.CYAN, core.NC, username),
		fmt.Sprintf("%sStorage Path:%s       %s", core.CYAN, core.NC, storageDir),
	}
	if backupFile != "" {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s %s", core.GREEN, core.NC, backupFile))
	} else {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s Skipped", core.YELLOW, core.NC))
	}
	if purgeDir {
		summary = append(summary, fmt.Sprintf("%sStorage Directory:%s  %sWILL BE DELETED%s", core.RED, core.NC, core.BOLD+core.RED, core.NC))
	} else {
		summary = append(summary, fmt.Sprintf("%sStorage Directory:%s  Preserved safely", core.GREEN, core.NC))
	}
	tui.PrintInBox("Uninstall [3/3]" + " " + core.Sym.EmDash + " " + "Review Uninstallation Actions", summary)

	confirm := tui.AskYesNo("Are you ABSOLUTELY sure you want to proceed with VSFTPD uninstallation?")
	if confirm != 0 {
		return
	}

	tui.LeaveAltScreen()
	logger.InitUninstallLogger("vsftpd")
	fmt.Printf("\r\n%s=== Uninstalling VSFTPD Secure FTP Server ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd != "" {
			_ = exec.Command("docker", "compose", "--project-directory", cd, "down", "-v").Run()
			_ = os.RemoveAll(cd)
		}
	} else {
		_ = exec.Command("systemctl", "stop", "vsftpd").Run()
		_ = exec.Command("rc-service", "vsftpd", "stop").Run()
		_ = exec.Command("rc-update", "del", "vsftpd", "default").Run()
		_ = exec.Command("systemctl", "disable", "vsftpd").Run()

		var pkgs []string
		switch core.SysInfo.Family {
		case "debian":
			pkgs = []string{"vsftpd", "ssl-cert"}
		case "rhel":
			pkgs = []string{"vsftpd"}
		case "alpine":
			pkgs = []string{"vsftpd", "vsftpd-openrc"}
		default:
			pkgs = []string{"vsftpd", "ssl-cert"}
		}
		system.RemovePackages(pkgs...)
		_ = exec.Command("userdel", "-r", username).Run()
	}

	if purgeDir && storageDir != "" && storageDir != "/" && storageDir != "/root" && storageDir != "/home" {
		_ = os.RemoveAll(storageDir)
		logger.LogInfo(fmt.Sprintf("FTP storage directory %s has been purged.", storageDir))
	}

	port := state["PORT"]
	if port == "" {
		port = "21"
	}
	pasvMin := state["PASV_MIN"]
	if pasvMin == "" {
		pasvMin = "40000"
	}
	pasvMax := state["PASV_MAX"]
	if pasvMax == "" {
		pasvMax = "40100"
	}
	system.RemoveFirewallRules(port+"/tcp", fmt.Sprintf("%s:%s/tcp", pasvMin, pasvMax))

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/vsftpd-manager")
	_ = os.Remove("/usr/local/bin/vsftpd-installer")
	_ = os.Remove("/usr/local/bin/vsftpd")

	fmt.Print("\r\n")
	logger.LogSuccess("VSFTPD Secure FTP Server has been uninstalled successfully.")
	time.Sleep(2 * time.Second)
}
