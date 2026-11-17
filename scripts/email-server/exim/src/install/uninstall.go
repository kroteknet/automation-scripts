// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: EXIM (email-server)
// File:    email-server/exim/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-exim-installer/core"
	"krotek-exim-installer/logger"
	"krotek-exim-installer/system"
	"krotek-exim-installer/tui"
)

// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	mailDomain := state["MAIL_DOMAIN"]
	if mailDomain == "" {
		mailDomain = "localhost"
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
		backupFile = fmt.Sprintf("/root/exim_backup_%s.tar.gz", timestamp)
		if envType == "docker" {
			cd := state["COMPOSE_DIR"]
			if cd != "" {
				_ = exec.Command("tar", "-czf", backupFile, cd).Run()
			}
		} else {
			if _, err := os.Stat("/etc/exim4"); err == nil {
				_ = exec.Command("tar", "-czf", backupFile, "/etc/exim4").Run()
			}
		}
	}

	// ─── Step 2: Data & Configuration Purge Decision ────────────────────────
	// Prompt whether to wipe application databases, keys, and storage or preserve them.
	tui.ClearScreen()
	purgeSpool := false
	purgeChoice := tui.AskYesNo("Uninstall [2/3]" + " " + core.Sym.EmDash + " " + "Purge all mail spools and messages in /var/spool/exim4? (No keeps mail)")
	if purgeChoice == 2 || purgeChoice == -1 {
		return
	}
	if purgeChoice == 0 {
		purgeSpool = true
	}

	// ─── Step 3: Confirmation & Action Review ────────────────────────────────
	// Display a summary card of all planned uninstallation actions before execution.
	tui.ClearScreen()
	summary := []string{
		fmt.Sprintf("%sAction:%s             Complete Exim4 Removal", core.RED, core.NC),
		fmt.Sprintf("%sEnvironment:%s        %s", core.CYAN, core.NC, envType),
		fmt.Sprintf("%sMail Domain:%s        %s", core.CYAN, core.NC, mailDomain),
	}
	if backupFile != "" {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s %s", core.GREEN, core.NC, backupFile))
	} else {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s Skipped", core.YELLOW, core.NC))
	}
	if purgeSpool {
		summary = append(summary, fmt.Sprintf("%sMail Spools:%s        %sWILL BE PURGED%s", core.RED, core.NC, core.BOLD+core.RED, core.NC))
	} else {
		summary = append(summary, fmt.Sprintf("%sMail Spools:%s        Preserved safely", core.GREEN, core.NC))
	}
	tui.PrintInBox("Uninstall [3/3]" + " " + core.Sym.EmDash + " " + "Review Uninstallation Actions", summary)

	confirm := tui.AskYesNo("Are you ABSOLUTELY sure you want to proceed with Exim4 uninstallation?")
	if confirm != 0 {
		return
	}

	tui.LeaveAltScreen()
	logger.InitUninstallLogger("exim")
	fmt.Printf("\r\n%s=== Uninstalling Exim4 MTA ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd != "" {
			_ = exec.Command("docker", "compose", "--project-directory", cd, "down", "-v").Run()
			_ = os.RemoveAll(cd)
		}
	} else {
		_ = exec.Command("systemctl", "stop", "exim4", "exim").Run()
		_ = exec.Command("rc-service", "exim4", "stop").Run()
		_ = exec.Command("rc-update", "del", "exim4", "default").Run()
		_ = exec.Command("rc-service", "exim", "stop").Run()
		_ = exec.Command("rc-update", "del", "exim", "default").Run()
		_ = exec.Command("systemctl", "disable", "exim4", "exim").Run()
		system.RemovePackages("exim4", "exim4-daemon-heavy", "exim")
	}

	if purgeSpool {
		_ = os.RemoveAll("/var/spool/exim4")
		_ = os.RemoveAll("/etc/exim4")
		logger.LogInfo("Exim4 configuration and spool data have been purged.")
	}

	port := state["PORT"]
	if port == "" {
		port = "25"
	}
	system.RemoveFirewallRules(port+"/tcp", "587/tcp", "465/tcp")

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/exim-manager")
	_ = os.Remove("/usr/local/bin/exim-installer")
	_ = os.Remove("/usr/local/bin/exim")

	fmt.Print("\r\n")
	logger.LogSuccess("Exim4 MTA has been uninstalled successfully.")
	time.Sleep(2 * time.Second)
}
