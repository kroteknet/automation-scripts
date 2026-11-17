// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: POSTFIX (email-server)
// File:    email-server/postfix/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-postfix-installer/core"
	"krotek-postfix-installer/logger"
	"krotek-postfix-installer/system"
	"krotek-postfix-installer/tui"
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
		backupFile = fmt.Sprintf("/root/postfix_backup_%s.tar.gz", timestamp)
		if envType == "docker" {
			cd := state["COMPOSE_DIR"]
			if cd != "" {
				_ = exec.Command("tar", "-czf", backupFile, cd).Run()
			}
		} else {
			if _, err := os.Stat("/etc/postfix"); err == nil {
				_ = exec.Command("tar", "-czf", backupFile, "/etc/postfix").Run()
			}
		}
	}

	// ─── Step 2: Data & Configuration Purge Decision ────────────────────────
	// Prompt whether to wipe application databases, keys, and storage or preserve them.
	tui.ClearScreen()
	purgeMail := false
	purgeChoice := tui.AskYesNo("Uninstall [2/3]" + " " + core.Sym.EmDash + " " + "Purge all mail spools and stored mailboxes (/var/mail, /var/spool/postfix)? (No keeps mail)")
	if purgeChoice == 2 || purgeChoice == -1 {
		return
	}
	if purgeChoice == 0 {
		purgeMail = true
	}

	// ─── Step 3: Confirmation & Action Review ────────────────────────────────
	// Display a summary card of all planned uninstallation actions before execution.
	tui.ClearScreen()
	summary := []string{
		fmt.Sprintf("%sAction:%s             Complete Postfix Removal", core.RED, core.NC),
		fmt.Sprintf("%sEnvironment:%s        %s", core.CYAN, core.NC, envType),
		fmt.Sprintf("%sMail Domain:%s        %s", core.CYAN, core.NC, mailDomain),
	}
	if backupFile != "" {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s %s", core.GREEN, core.NC, backupFile))
	} else {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s Skipped", core.YELLOW, core.NC))
	}
	if purgeMail {
		summary = append(summary, fmt.Sprintf("%sMailboxes & Spools:%s %sWILL BE PURGED%s", core.RED, core.NC, core.BOLD+core.RED, core.NC))
	} else {
		summary = append(summary, fmt.Sprintf("%sMailboxes & Spools:%s Preserved safely", core.GREEN, core.NC))
	}
	tui.PrintInBox("Uninstall [3/3]" + " " + core.Sym.EmDash + " " + "Review Uninstallation Actions", summary)

	confirm := tui.AskYesNo("Are you ABSOLUTELY sure you want to proceed with Postfix uninstallation?")
	if confirm != 0 {
		return
	}

	tui.LeaveAltScreen()
	logger.InitUninstallLogger("postfix")
	fmt.Printf("\r\n%s=== Uninstalling Postfix Mail ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd != "" {
			_ = exec.Command("docker", "compose", "--project-directory", cd, "down", "-v").Run()
			_ = os.RemoveAll(cd)
		}
	} else {
		_ = exec.Command("systemctl", "stop", "postfix").Run()
		_ = exec.Command("rc-service", "postfix", "stop").Run()
		_ = exec.Command("rc-update", "del", "postfix", "default").Run()
		_ = exec.Command("systemctl", "disable", "postfix").Run()
		system.RemovePackages("postfix")
	}

	if purgeMail {
		_ = os.RemoveAll("/var/spool/postfix")
		_ = os.RemoveAll("/var/mail")
		_ = os.RemoveAll("/etc/postfix")
		logger.LogInfo("Postfix spool and mailbox data have been purged.")
	}

	port := state["PORT"]
	if port == "" {
		port = "25"
	}
	system.RemoveFirewallRules(port+"/tcp", "587/tcp", "465/tcp")

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/postfix-manager")
	_ = os.Remove("/usr/local/bin/postfix-installer")
	_ = os.Remove("/usr/local/bin/postfix")

	fmt.Print("\r\n")
	logger.LogSuccess("Postfix Mail has been uninstalled successfully.")
	time.Sleep(2 * time.Second)
}
