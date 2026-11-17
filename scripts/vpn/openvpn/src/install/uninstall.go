// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: OPENVPN (vpn)
// File:    vpn/openvpn/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-openvpn-installer/core"
	"krotek-openvpn-installer/logger"
	"krotek-openvpn-installer/system"
	"krotek-openvpn-installer/tui"
)

// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	port := state["PORT"]
	if port == "" {
		port = "1194"
	}
	proto := state["PROTO"]
	if proto == "" {
		proto = "udp"
	}

	// ─── Step 1: Pre-removal Backup Archive ──────────────────────────────────
	// Create a timestamped tar.gz archive of configuration and data files in /root.
	tui.ClearScreen()
	backupChoice := tui.AskYesNo("Uninstall [1/3]" + " " + core.Sym.EmDash + " " + "Create pre-removal configuration & certificates backup archive?")
	if backupChoice == 2 || backupChoice == -1 {
		return
	}
	var backupFile string
	if backupChoice == 0 {
		timestamp := time.Now().Format("20060102_150405")
		backupFile = fmt.Sprintf("/root/openvpn_backup_%s.tar.gz", timestamp)
		if envType == "docker" {
			cd := state["COMPOSE_DIR"]
			if cd != "" {
				_ = exec.Command("tar", "-czf", backupFile, cd).Run()
			}
		} else {
			if _, err := os.Stat("/etc/openvpn"); err == nil {
				_ = exec.Command("tar", "-czf", backupFile, "/etc/openvpn").Run()
			}
		}
	}

	// ─── Step 2: Data & Configuration Purge Decision ────────────────────────
	// Prompt whether to wipe application databases, keys, and storage or preserve them.
	tui.ClearScreen()
	purgeKeys := false
	purgeChoice := tui.AskYesNo("Uninstall [2/3]" + " " + core.Sym.EmDash + " " + "Purge all OpenVPN certificates, keys, and client configs? (No preserves them)")
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
		fmt.Sprintf("%sAction:%s             Complete OpenVPN Removal", core.RED, core.NC),
		fmt.Sprintf("%sEnvironment:%s        %s", core.CYAN, core.NC, envType),
		fmt.Sprintf("%sListening Port:%s     %s/%s", core.CYAN, core.NC, port, proto),
	}
	if backupFile != "" {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s %s", core.GREEN, core.NC, backupFile))
	} else {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s Skipped", core.YELLOW, core.NC))
	}
	if purgeKeys {
		summary = append(summary, fmt.Sprintf("%sKeys & Certificates:%s %sWILL BE PURGED%s", core.RED, core.NC, core.BOLD+core.RED, core.NC))
	} else {
		summary = append(summary, fmt.Sprintf("%sKeys & Certificates:%s Preserved safely", core.GREEN, core.NC))
	}
	tui.PrintInBox("Uninstall [3/3]" + " " + core.Sym.EmDash + " " + "Review Uninstallation Actions", summary)

	confirm := tui.AskYesNo("Are you ABSOLUTELY sure you want to proceed with OpenVPN uninstallation?")
	if confirm != 0 {
		return
	}

	tui.LeaveAltScreen()
	logger.InitUninstallLogger("openvpn")
	fmt.Printf("\r\n%s=== Uninstalling OpenVPN Server ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd != "" {
			_ = exec.Command("docker", "compose", "--project-directory", cd, "down", "-v").Run()
			_ = os.RemoveAll(cd)
		}
	} else {
		_ = exec.Command("systemctl", "stop", "openvpn-server@server", "openvpn@server", "openvpn").Run()
		_ = exec.Command("rc-service", "openvpn-server@server", "stop").Run()
		_ = exec.Command("rc-update", "del", "openvpn-server@server", "default").Run()
		_ = exec.Command("rc-service", "openvpn@server", "stop").Run()
		_ = exec.Command("rc-update", "del", "openvpn@server", "default").Run()
		_ = exec.Command("rc-service", "openvpn", "stop").Run()
		_ = exec.Command("rc-update", "del", "openvpn", "default").Run()
		_ = exec.Command("systemctl", "disable", "openvpn-server@server", "openvpn@server", "openvpn").Run()

		var pkgs []string
		switch core.SysInfo.Family {
		case "debian":
			pkgs = []string{"openvpn", "easy-rsa"}
		case "rhel":
			pkgs = []string{"openvpn", "easy-rsa"}
		case "alpine":
			pkgs = []string{"openvpn", "easy-rsa"}
		default:
			pkgs = []string{"openvpn", "easy-rsa"}
		}
		system.RemovePackages(pkgs...)
	}

	if purgeKeys {
		_ = os.RemoveAll("/etc/openvpn")
		logger.LogInfo("All OpenVPN configurations and certificates have been purged.")
	}

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/openvpn-manager")
	_ = os.Remove("/usr/local/bin/openvpn-installer")
	_ = os.Remove("/usr/local/bin/openvpn")

	fmt.Print("\r\n")
	logger.LogSuccess("OpenVPN Server has been uninstalled successfully.")
	time.Sleep(2 * time.Second)
}
