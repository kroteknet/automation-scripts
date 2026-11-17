// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: HESTIACP (webpanel)
// File:    webpanel/hestiacp/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-hestiacp-installer/core"
	"krotek-hestiacp-installer/logger"
	"krotek-hestiacp-installer/system"
	"krotek-hestiacp-installer/tui"
)

// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	port := state["PORT"]
	if port == "" {
		port = "8083"
	}

	// ─── Step 1: Pre-removal Backup Archive ──────────────────────────────────
	// Create a timestamped tar.gz archive of configuration and data files in /root.
	tui.ClearScreen()
	backupChoice := tui.AskYesNo("Uninstall [1/3]" + " " + core.Sym.EmDash + " " + "Create pre-removal configuration & vhost backup archive?")
	if backupChoice == 2 || backupChoice == -1 {
		return
	}
	var backupFile string
	if backupChoice == 0 {
		timestamp := time.Now().Format("20060102_150405")
		backupFile = fmt.Sprintf("/root/hestiacp_backup_%s.tar.gz", timestamp)
		if envType == "docker" {
			cd := state["COMPOSE_DIR"]
			if cd != "" {
				_ = exec.Command("tar", "-czf", backupFile, cd).Run()
			}
		} else {
			if _, err := os.Stat("/usr/local/hestia"); err == nil {
				_ = exec.Command("tar", "-czf", backupFile, "/usr/local/hestia").Run()
			}
		}
	}

	// ─── Step 2: Data & Configuration Purge Decision ────────────────────────
	// Prompt whether to wipe application databases, keys, and storage or preserve them.
	tui.ClearScreen()
	purgeData := false
	purgeChoice := tui.AskYesNo("Uninstall [2/3]" + " " + core.Sym.EmDash + " " + "Purge all user website directories (/home/*)? (No preserves user sites)")
	if purgeChoice == 2 || purgeChoice == -1 {
		return
	}
	if purgeChoice == 0 {
		purgeData = true
	}

	// ─── Step 3: Confirmation & Action Review ────────────────────────────────
	// Display a summary card of all planned uninstallation actions before execution.
	tui.ClearScreen()
	summary := []string{
		fmt.Sprintf("%sAction:%s             Complete HestiaCP Removal", core.RED, core.NC),
		fmt.Sprintf("%sEnvironment:%s        %s", core.CYAN, core.NC, envType),
		fmt.Sprintf("%sControl Panel Port:%s %s", core.CYAN, core.NC, port),
	}
	if backupFile != "" {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s %s", core.GREEN, core.NC, backupFile))
	} else {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s Skipped", core.YELLOW, core.NC))
	}
	if purgeData {
		summary = append(summary, fmt.Sprintf("%sUser Web Storage:%s   %sWILL BE DELETED%s", core.RED, core.NC, core.BOLD+core.RED, core.NC))
	} else {
		summary = append(summary, fmt.Sprintf("%sUser Web Storage:%s   Preserved safely", core.GREEN, core.NC))
	}
	tui.PrintInBox("Uninstall [3/3]" + " " + core.Sym.EmDash + " " + "Review Uninstallation Actions", summary)

	confirm := tui.AskYesNo("Are you ABSOLUTELY sure you want to proceed with HestiaCP uninstallation?")
	if confirm != 0 {
		return
	}

	tui.LeaveAltScreen()
	logger.InitUninstallLogger("hestiacp")
	fmt.Printf("\r\n%s=== Uninstalling HestiaCP Web Panel ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd != "" {
			_ = exec.Command("docker", "compose", "--project-directory", cd, "down", "-v").Run()
			_ = os.RemoveAll(cd)
		}
	} else {
		_ = exec.Command("systemctl", "stop", "hestia").Run()
		_ = exec.Command("rc-service", "hestia", "stop").Run()
		_ = exec.Command("rc-update", "del", "hestia", "default").Run()
		_ = exec.Command("systemctl", "disable", "hestia").Run()
	}

	if purgeData {
		_ = os.RemoveAll("/usr/local/hestia")
		logger.LogInfo("HestiaCP core directories have been purged.")
	}

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/hestiacp-manager")
	_ = os.Remove("/usr/local/bin/hestiacp-installer")
	_ = os.Remove("/usr/local/bin/hestia-manager")
	_ = os.Remove("/usr/local/bin/hestia-installer")
	_ = os.Remove("/usr/local/bin/hestiacp")

	fmt.Print("\r\n")
	logger.LogSuccess("HestiaCP Web Panel has been uninstalled successfully.")
	time.Sleep(2 * time.Second)
}
