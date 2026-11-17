// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: SAMBA (media-server)
// File:    media-server/samba/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-samba-installer/core"
	"krotek-samba-installer/logger"
	"krotek-samba-installer/system"
	"krotek-samba-installer/tui"
)

// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	sharePath := state["SHARE_PATH"]
	if sharePath == "" {
		sharePath = "/srv/samba/share"
	}
	shareName := state["SHARE_NAME"]
	if shareName == "" {
		shareName = "share"
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
		backupFile = fmt.Sprintf("/root/samba_backup_%s.tar.gz", timestamp)
		if envType == "docker" {
			cd := state["COMPOSE_DIR"]
			if cd != "" {
				_ = exec.Command("tar", "-czf", backupFile, cd).Run()
			}
		} else {
			if _, err := os.Stat("/etc/samba/smb.conf"); err == nil {
				_ = exec.Command("tar", "-czf", backupFile, "/etc/samba").Run()
			}
		}
	}

	// ─── Step 2: Data & Configuration Purge Decision ────────────────────────
	// Prompt whether to wipe application databases, keys, and storage or preserve them.
	tui.ClearScreen()
	purgeDir := false
	purgeChoice := tui.AskYesNo(fmt.Sprintf("Uninstall [2/3]" + " " + core.Sym.EmDash + " " + "Purge all files in Samba share directory (%s)? (No keeps your files)", sharePath))
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
		fmt.Sprintf("%sAction:%s             Complete Samba Removal", core.RED, core.NC),
		fmt.Sprintf("%sEnvironment:%s        %s", core.CYAN, core.NC, envType),
		fmt.Sprintf("%sShare Name:%s         %s", core.CYAN, core.NC, shareName),
		fmt.Sprintf("%sShare Path:%s         %s", core.CYAN, core.NC, sharePath),
	}
	if backupFile != "" {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s %s", core.GREEN, core.NC, backupFile))
	} else {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s Skipped", core.YELLOW, core.NC))
	}
	if purgeDir {
		summary = append(summary, fmt.Sprintf("%sShare Storage:%s      %sWILL BE DELETED%s", core.RED, core.NC, core.BOLD+core.RED, core.NC))
	} else {
		summary = append(summary, fmt.Sprintf("%sShare Storage:%s      Preserved safely", core.GREEN, core.NC))
	}
	tui.PrintInBox("Uninstall [3/3]" + " " + core.Sym.EmDash + " " + "Review Uninstallation Actions", summary)

	confirm := tui.AskYesNo("Are you ABSOLUTELY sure you want to proceed with Samba uninstallation?")
	if confirm != 0 {
		return
	}

	tui.LeaveAltScreen()
	logger.InitUninstallLogger("samba")
	fmt.Printf("\r\n%s=== Uninstalling Samba Windows Share ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd != "" {
			_ = exec.Command("docker", "compose", "--project-directory", cd, "down", "-v").Run()
			_ = os.RemoveAll(cd)
		}
	} else {
		_ = exec.Command("systemctl", "stop", "smbd", "smb", "nmbd", "nmb").Run()
		_ = exec.Command("rc-service", "smbd", "stop").Run()
		_ = exec.Command("rc-update", "del", "smbd", "default").Run()
		_ = exec.Command("rc-service", "smb", "stop").Run()
		_ = exec.Command("rc-update", "del", "smb", "default").Run()
		_ = exec.Command("rc-service", "nmbd", "stop").Run()
		_ = exec.Command("rc-update", "del", "nmbd", "default").Run()
		_ = exec.Command("rc-service", "nmb", "stop").Run()
		_ = exec.Command("rc-update", "del", "nmb", "default").Run()
		_ = exec.Command("systemctl", "disable", "smbd", "smb", "nmbd", "nmb").Run()

		var pkgs []string
		switch core.SysInfo.Family {
		case "debian":
			pkgs = []string{"samba", "samba-common-bin"}
		case "rhel":
			pkgs = []string{"samba", "samba-common", "samba-client"}
		case "alpine":
			pkgs = []string{"samba", "samba-common", "samba-client"}
		default:
			pkgs = []string{"samba", "samba-common-bin"}
		}
		system.RemovePackages(pkgs...)
	}

	if purgeDir && sharePath != "" && sharePath != "/" && sharePath != "/root" && sharePath != "/home" {
		_ = os.RemoveAll(sharePath)
		logger.LogInfo(fmt.Sprintf("Samba share directory %s has been purged.", sharePath))
	}

	system.RemoveFirewallRules("139/tcp", "445/tcp", "137/udp", "138/udp")

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/samba-manager")
	_ = os.Remove("/usr/local/bin/samba-installer")
	_ = os.Remove("/usr/local/bin/smbd-manager")
	_ = os.Remove("/usr/local/bin/smbd-installer")
	_ = os.Remove("/usr/local/bin/samba")

	fmt.Print("\r\n")
	logger.LogSuccess("Samba Windows Share has been uninstalled successfully.")
	time.Sleep(2 * time.Second)
}
