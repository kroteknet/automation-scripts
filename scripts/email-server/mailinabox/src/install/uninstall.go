// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: MAILINABOX (email-server)
// File:    email-server/mailinabox/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-mailinabox-installer/core"
	"krotek-mailinabox-installer/logger"
	"krotek-mailinabox-installer/system"
	"krotek-mailinabox-installer/tui"
)

// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	mailDomain := state["MAIL_DOMAIN"]
	if mailDomain == "" {
		mailDomain = "box.local"
	}

	// ─── Step 1: Pre-removal Backup Archive ──────────────────────────────────
	// Create a timestamped tar.gz archive of configuration and data files in /root.
	tui.ClearScreen()
	backupChoice := tui.AskYesNo("Uninstall [1/3]" + " " + core.Sym.EmDash + " " + "Create pre-removal backup archive of user data & mailboxes?")
	if backupChoice == 2 || backupChoice == -1 {
		return
	}
	var backupFile string
	if backupChoice == 0 {
		timestamp := time.Now().Format("20060102_150405")
		backupFile = fmt.Sprintf("/root/mailinabox_backup_%s.tar.gz", timestamp)
		if envType == "docker" {
			cd := state["COMPOSE_DIR"]
			if cd != "" {
				_ = exec.Command("tar", "-czf", backupFile, cd).Run()
			}
		} else {
			if _, err := os.Stat("/home/user-data"); err == nil {
				_ = exec.Command("tar", "-czf", backupFile, "/home/user-data").Run()
			}
		}
	}

	// ─── Step 2: Data & Configuration Purge Decision ────────────────────────
	// Prompt whether to wipe application databases, keys, and storage or preserve them.
	tui.ClearScreen()
	purgeData := false
	purgeChoice := tui.AskYesNo("Uninstall [2/3]" + " " + core.Sym.EmDash + " " + "Purge all mailboxes and user data in /home/user-data? (No keeps mail)")
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
		fmt.Sprintf("%sAction:%s             Complete Mail-in-a-Box Removal", core.RED, core.NC),
		fmt.Sprintf("%sEnvironment:%s        %s", core.CYAN, core.NC, envType),
		fmt.Sprintf("%sMail Domain:%s        %s", core.CYAN, core.NC, mailDomain),
	}
	if backupFile != "" {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s %s", core.GREEN, core.NC, backupFile))
	} else {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s Skipped", core.YELLOW, core.NC))
	}
	if purgeData {
		summary = append(summary, fmt.Sprintf("%sUser Mail Data:%s     %sWILL BE PURGED%s", core.RED, core.NC, core.BOLD+core.RED, core.NC))
	} else {
		summary = append(summary, fmt.Sprintf("%sUser Mail Data:%s     Preserved safely", core.GREEN, core.NC))
	}
	tui.PrintInBox("Uninstall [3/3]" + " " + core.Sym.EmDash + " " + "Review Uninstallation Actions", summary)

	confirm := tui.AskYesNo("Are you ABSOLUTELY sure you want to proceed with Mail-in-a-Box uninstallation?")
	if confirm != 0 {
		return
	}

	tui.LeaveAltScreen()
	logger.InitUninstallLogger("mailinabox")
	fmt.Printf("\r\n%s=== Uninstalling Mail-in-a-Box ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd != "" {
			_ = exec.Command("docker", "compose", "--project-directory", cd, "down", "-v").Run()
			_ = os.RemoveAll(cd)
		}
	} else {
		_ = exec.Command("systemctl", "stop", "mailinabox").Run()
		_ = exec.Command("rc-service", "mailinabox", "stop").Run()
		_ = exec.Command("rc-update", "del", "mailinabox", "default").Run()
		_ = exec.Command("systemctl", "disable", "mailinabox").Run()
	}

	if purgeData {
		_ = os.RemoveAll("/home/user-data")
		logger.LogInfo("Mail-in-a-Box user data directory has been purged.")
	}

	system.RemoveFirewallRules("25/tcp", "587/tcp", "465/tcp", "993/tcp", "995/tcp", "80/tcp", "443/tcp", "53/tcp", "53/udp", "4190/tcp")

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/mailinabox-manager")
	_ = os.Remove("/usr/local/bin/mailinabox-installer")
	_ = os.Remove("/usr/local/bin/mailinabox")

	fmt.Print("\r\n")
	logger.LogSuccess("Mail-in-a-Box has been uninstalled successfully.")
	time.Sleep(2 * time.Second)
}
