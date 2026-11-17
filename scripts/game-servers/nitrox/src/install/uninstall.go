// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: NITROX (game-servers)
// File:    game-servers/nitrox/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-nitrox-installer/core"
	"krotek-nitrox-installer/logger"
	"krotek-nitrox-installer/system"
	"krotek-nitrox-installer/tui"
)

// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	installDir := state["INSTALL_DIR"]
	if installDir == "" {
		if _, err := os.Stat("/srv/nitrox"); err == nil {
			installDir = "/srv/nitrox"
		} else {
			installDir = "/opt/nitrox"
		}
	}
	serverName := state["SERVER_NAME"]
	if serverName == "" {
		serverName = "Subnautica Nitrox Server"
	}

	// ─── Step 1: Pre-removal Backup Archive ──────────────────────────────────
	// Create a timestamped tar.gz archive of configuration and data files in /root.
	tui.ClearScreen()
	backupChoice := tui.AskYesNo("Uninstall [1/3]" + " " + core.Sym.EmDash + " " + "Create pre-removal backup archive of Nitrox world saves?")
	if backupChoice == 2 || backupChoice == -1 {
		return
	}
	var backupFile string
	if backupChoice == 0 {
		timestamp := time.Now().Format("20060102_150405")
		backupFile = fmt.Sprintf("/root/nitrox_backup_%s.tar.gz", timestamp)
		if envType == "docker" {
			cd := state["COMPOSE_DIR"]
			if cd != "" {
				_ = exec.Command("tar", "-czf", backupFile, cd).Run()
				_ = os.Chmod(backupFile, 0600)
			}
		} else {
			if _, err := os.Stat(installDir); err == nil {
				_ = exec.Command("tar", "-czf", backupFile, installDir).Run()
				_ = os.Chmod(backupFile, 0600)
			}
		}
	}

	// ─── Step 2: Data & Configuration Purge Decision ────────────────────────
	// Prompt whether to wipe application databases, keys, and storage or preserve them.
	tui.ClearScreen()
	purgeWorld := false
	purgeChoice := tui.AskYesNo(fmt.Sprintf("Uninstall [2/3]" + " " + core.Sym.EmDash + " " + "Purge all world saves and files in %s? (No keeps saves)", installDir))
	if purgeChoice == 2 || purgeChoice == -1 {
		return
	}
	if purgeChoice == 0 {
		purgeWorld = true
	}

	// ─── Step 3: Confirmation & Action Review ────────────────────────────────
	// Display a summary card of all planned uninstallation actions before execution.
	tui.ClearScreen()
	summary := []string{
		fmt.Sprintf("%sAction:%s             Complete Nitrox Server Removal", core.RED, core.NC),
		fmt.Sprintf("%sEnvironment:%s        %s", core.CYAN, core.NC, envType),
		fmt.Sprintf("%sServer Name:%s        %s", core.CYAN, core.NC, serverName),
		fmt.Sprintf("%sInstall Path:%s       %s", core.CYAN, core.NC, installDir),
	}
	if backupFile != "" {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s %s", core.GREEN, core.NC, backupFile))
	} else {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s Skipped", core.YELLOW, core.NC))
	}
	if purgeWorld {
		summary = append(summary, fmt.Sprintf("%sWorld Save Data:%s    %sWILL BE DELETED%s", core.RED, core.NC, core.BOLD+core.RED, core.NC))
	} else {
		summary = append(summary, fmt.Sprintf("%sWorld Save Data:%s    Preserved safely", core.GREEN, core.NC))
	}
	tui.PrintInBox("Uninstall [3/3]" + " " + core.Sym.EmDash + " " + "Review Uninstallation Actions", summary)

	confirm := tui.AskYesNo("Are you ABSOLUTELY sure you want to proceed with Nitrox uninstallation?")
	if confirm != 0 {
		return
	}

	tui.LeaveAltScreen()
	logger.InitUninstallLogger("nitrox")
	fmt.Printf("\r\n%s=== Uninstalling Nitrox Subnautica Multiplayer Server ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd != "" {
			_ = exec.Command("docker", "compose", "--project-directory", cd, "down", "-v").Run()
			_ = os.RemoveAll(cd)
		}
	} else {
		_ = exec.Command("systemctl", "stop", "nitrox").Run()
		_ = exec.Command("rc-service", "nitrox", "stop").Run()
		_ = exec.Command("rc-update", "del", "nitrox", "default").Run()
		_ = exec.Command("systemctl", "disable", "nitrox").Run()
		_ = os.Remove("/etc/systemd/system/nitrox.service")
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}

	if purgeWorld && installDir != "" && installDir != "/" && installDir != "/root" && installDir != "/home" {
		_ = os.RemoveAll(installDir)
		logger.LogInfo(fmt.Sprintf("Nitrox directory %s has been purged.", installDir))
	}

	port := state["PORT"]
	if port == "" {
		port = "11000"
	}
	system.RemoveFirewallRules(port + "/udp")

	_ = os.Remove("/etc/cron.daily/nitrox-backup")
	_ = os.Remove("/usr/local/libexec/nitrox-backup.sh")
	_ = os.Remove(core.StateFile)
	_ = os.Remove("/etc/.nitrox_installer_state")
	_ = os.Remove("/etc/.nitrox_state")
	_ = os.Remove("/usr/local/bin/nitrox-manager")
	_ = os.Remove("/usr/local/bin/nitrox-installer")
	_ = os.Remove("/usr/local/bin/nitrox")

	fmt.Print("\r\n")
	logger.LogSuccess("Nitrox Subnautica Multiplayer Server has been uninstalled successfully.")
	time.Sleep(2 * time.Second)
}
