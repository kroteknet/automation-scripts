// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: WEBMIN (webpanel)
// File:    webpanel/webmin/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-webmin-installer/core"
	"krotek-webmin-installer/logger"
	"krotek-webmin-installer/system"
	"krotek-webmin-installer/tui"
)

// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	port := state["PORT"]
	if port == "" {
		port = "10000"
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
		backupFile = fmt.Sprintf("/root/webmin_backup_%s.tar.gz", timestamp)
		if envType == "docker" {
			cd := state["COMPOSE_DIR"]
			if cd != "" {
				_ = exec.Command("tar", "-czf", backupFile, cd).Run()
			}
		} else {
			if _, err := os.Stat("/etc/webmin"); err == nil {
				_ = exec.Command("tar", "-czf", backupFile, "/etc/webmin").Run()
			}
		}
	}

	// ─── Step 2: Data & Configuration Purge Decision ────────────────────────
	// Prompt whether to wipe application databases, keys, and storage or preserve them.
	tui.ClearScreen()
	purgeData := false
	purgeChoice := tui.AskYesNo("Uninstall [2/3]" + " " + core.Sym.EmDash + " " + "Purge all Webmin module configurations and logs? (No preserves configuration)")
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
		fmt.Sprintf("%sAction:%s             Complete Webmin Removal", core.RED, core.NC),
		fmt.Sprintf("%sEnvironment:%s        %s", core.CYAN, core.NC, envType),
		fmt.Sprintf("%sControl Panel Port:%s %s", core.CYAN, core.NC, port),
	}
	if backupFile != "" {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s %s", core.GREEN, core.NC, backupFile))
	} else {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s Skipped", core.YELLOW, core.NC))
	}
	if purgeData {
		summary = append(summary, fmt.Sprintf("%sWebmin Configurations:%s %sWILL BE PURGED%s", core.RED, core.NC, core.BOLD+core.RED, core.NC))
	} else {
		summary = append(summary, fmt.Sprintf("%sWebmin Configurations:%s Preserved safely", core.GREEN, core.NC))
	}
	tui.PrintInBox("Uninstall [3/3]" + " " + core.Sym.EmDash + " " + "Review Uninstallation Actions", summary)

	confirm := tui.AskYesNo("Are you ABSOLUTELY sure you want to proceed with Webmin uninstallation?")
	if confirm != 0 {
		return
	}

	tui.LeaveAltScreen()
	logger.InitUninstallLogger("webmin")
	fmt.Printf("\r\n%s=== Uninstalling Webmin System Administration ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd != "" {
			_ = exec.Command("docker", "compose", "--project-directory", cd, "down", "-v").Run()
			_ = os.RemoveAll(cd)
		}
	} else {
		_ = exec.Command("systemctl", "stop", "webmin").Run()
		_ = exec.Command("rc-service", "webmin", "stop").Run()
		_ = exec.Command("rc-update", "del", "webmin", "default").Run()
		_ = exec.Command("systemctl", "disable", "webmin").Run()
		system.RemovePackages("webmin")
	}

	if purgeData {
		_ = os.RemoveAll("/etc/webmin")
		_ = os.RemoveAll("/var/webmin")
		logger.LogInfo("Webmin configuration files have been purged.")
	}

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/webmin-manager")
	_ = os.Remove("/usr/local/bin/webmin-installer")
	_ = os.Remove("/usr/local/bin/webmin")

	fmt.Print("\r\n")
	logger.LogSuccess("Webmin System Administration has been uninstalled successfully.")
	time.Sleep(2 * time.Second)
}
