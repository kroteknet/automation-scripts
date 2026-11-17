// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: WIREGUARD (vpn)
// File:    vpn/wireguard/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-wireguard-installer/core"
	"krotek-wireguard-installer/logger"
	"krotek-wireguard-installer/system"
	"krotek-wireguard-installer/tui"
)

// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	port := state["PORT"]
	if port == "" {
		port = "51820"
	}

	// ─── Step 1: Pre-removal Backup Archive ──────────────────────────────────
	// Create a timestamped tar.gz archive of configuration and data files in /root.
	tui.ClearScreen()
	backupChoice := tui.AskYesNo("Uninstall [1/3]" + " " + core.Sym.EmDash + " " + "Create pre-removal configuration & keys backup archive?")
	if backupChoice == 2 || backupChoice == -1 {
		return
	}
	var backupFile string
	if backupChoice == 0 {
		timestamp := time.Now().Format("20060102_150405")
		backupFile = fmt.Sprintf("/root/wireguard_backup_%s.tar.gz", timestamp)
		if envType == "docker" {
			cd := state["COMPOSE_DIR"]
			if cd != "" {
				_ = exec.Command("tar", "-czf", backupFile, cd).Run()
			}
		} else {
			if _, err := os.Stat("/etc/wireguard"); err == nil {
				_ = exec.Command("tar", "-czf", backupFile, "/etc/wireguard").Run()
			}
		}
	}

	// ─── Step 2: Data & Configuration Purge Decision ────────────────────────
	// Prompt whether to wipe application databases, keys, and storage or preserve them.
	tui.ClearScreen()
	purgeKeys := false
	purgeChoice := tui.AskYesNo("Uninstall [2/3]" + " " + core.Sym.EmDash + " " + "Purge all WireGuard server keys and client peer profiles? (No preserves keys)")
	if purgeChoice == 2 || purgeChoice == -1 {
		return
	}
	if purgeChoice == 0 {
		purgeKeys = true
	}

	// ─── Step 3: Confirmation & Action Review ────────────────────────────────
	// Display a summary card of all planned uninstallation actions before execution.
	tui.ClearScreen()
	summary := []string{
		fmt.Sprintf("%sAction:%s             Complete WireGuard VPN Removal", core.RED, core.NC),
		fmt.Sprintf("%sEnvironment:%s        %s", core.CYAN, core.NC, envType),
		fmt.Sprintf("%sListening Port:%s     %s/UDP", core.CYAN, core.NC, port),
	}
	if backupFile != "" {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s %s", core.GREEN, core.NC, backupFile))
	} else {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s Skipped", core.YELLOW, core.NC))
	}
	if purgeKeys {
		summary = append(summary, fmt.Sprintf("%sKeys & Peer Profiles:%s %sWILL BE PURGED%s", core.RED, core.NC, core.BOLD+core.RED, core.NC))
	} else {
		summary = append(summary, fmt.Sprintf("%sKeys & Peer Profiles:%s Preserved safely", core.GREEN, core.NC))
	}
	tui.PrintInBox("Uninstall [3/3]" + " " + core.Sym.EmDash + " " + "Review Uninstallation Actions", summary)

	confirm := tui.AskYesNo("Are you ABSOLUTELY sure you want to proceed with WireGuard uninstallation?")
	if confirm != 0 {
		return
	}

	tui.LeaveAltScreen()
	logger.InitUninstallLogger("wireguard")
	fmt.Printf("\r\n%s=== Uninstalling WireGuard VPN ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd != "" {
			_ = exec.Command("docker", "compose", "--project-directory", cd, "down", "-v").Run()
			_ = os.RemoveAll(cd)
		}
	} else {
		_ = exec.Command("systemctl", "stop", "wg-quick@wg0").Run()
		_ = exec.Command("rc-service", "wg-quick@wg0", "stop").Run()
		_ = exec.Command("rc-update", "del", "wg-quick@wg0", "default").Run()
		_ = exec.Command("systemctl", "disable", "wg-quick@wg0").Run()
		_ = exec.Command("wg-quick", "down", "wg0").Run()

		var pkgs []string
		switch core.SysInfo.Family {
		case "debian":
			pkgs = []string{"wireguard", "wireguard-tools"}
		case "rhel":
			pkgs = []string{"wireguard-tools"}
		case "alpine":
			pkgs = []string{"wireguard-tools"}
		default:
			pkgs = []string{"wireguard", "wireguard-tools"}
		}
		system.RemovePackages(pkgs...)
	}

	if purgeKeys {
		_ = os.RemoveAll("/etc/wireguard")
		logger.LogInfo("All WireGuard keys and profiles have been purged.")
	}

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/wireguard-manager")
	_ = os.Remove("/usr/local/bin/wireguard-installer")
	_ = os.Remove("/usr/local/bin/wg-manager")
	_ = os.Remove("/usr/local/bin/wg-installer")
	_ = os.Remove("/usr/local/bin/wireguard")

	fmt.Print("\r\n")
	logger.LogSuccess("WireGuard VPN has been uninstalled successfully.")
	time.Sleep(2 * time.Second)
}
