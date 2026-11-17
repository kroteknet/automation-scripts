// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: NFS (media-server)
// File:    media-server/nfs/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"krotek-nfs-installer/core"
	"krotek-nfs-installer/logger"
	"krotek-nfs-installer/system"
	"krotek-nfs-installer/tui"
)

// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	exportDir := state["EXPORT_DIR"]
	if exportDir == "" {
		exportDir = "/srv/nfs/share"
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
		backupFile = fmt.Sprintf("/root/nfs_backup_%s.tar.gz", timestamp)
		if envType == "docker" {
			cd := state["COMPOSE_DIR"]
			if cd != "" {
				_ = exec.Command("tar", "-czf", backupFile, cd).Run()
			}
		} else {
			if _, err := os.Stat("/etc/exports"); err == nil {
				_ = exec.Command("tar", "-czf", backupFile, "/etc/exports").Run()
			}
		}
	}

	// ─── Step 2: Data & Configuration Purge Decision ────────────────────────
	// Prompt whether to wipe application databases, keys, and storage or preserve them.
	tui.ClearScreen()
	purgeDir := false
	purgeChoice := tui.AskYesNo(fmt.Sprintf("Uninstall [2/3]" + " " + core.Sym.EmDash + " " + "Purge all files in exported share (%s)? (No keeps your data)", exportDir))
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
		fmt.Sprintf("%sAction:%s             Complete NFS Removal", core.RED, core.NC),
		fmt.Sprintf("%sEnvironment:%s        %s", core.CYAN, core.NC, envType),
		fmt.Sprintf("%sExport Directory:%s   %s", core.CYAN, core.NC, exportDir),
	}
	if backupFile != "" {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s %s", core.GREEN, core.NC, backupFile))
	} else {
		summary = append(summary, fmt.Sprintf("%sPre-removal Backup:%s Skipped", core.YELLOW, core.NC))
	}
	if purgeDir {
		summary = append(summary, fmt.Sprintf("%sShare Directory:%s    %sWILL BE DELETED%s", core.RED, core.NC, core.BOLD+core.RED, core.NC))
	} else {
		summary = append(summary, fmt.Sprintf("%sShare Directory:%s    Preserved safely", core.GREEN, core.NC))
	}
	tui.PrintInBox("Uninstall [3/3]" + " " + core.Sym.EmDash + " " + "Review Uninstallation Actions", summary)

	confirm := tui.AskYesNo("Are you ABSOLUTELY sure you want to proceed with NFS uninstallation?")
	if confirm != 0 {
		return
	}

	tui.LeaveAltScreen()
	logger.InitUninstallLogger("nfs")
	fmt.Printf("\r\n%s=== Uninstalling NFS Network File System ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	if envType == "docker" {
		cd := state["COMPOSE_DIR"]
		if cd != "" {
			_ = exec.Command("docker", "compose", "--project-directory", cd, "down", "-v").Run()
			_ = os.RemoveAll(cd)
		}
	} else {
		_ = exec.Command("exportfs", "-uav").Run()
		_ = exec.Command("systemctl", "stop", "nfs-server", "nfs-kernel-server", "nfs").Run()
		_ = exec.Command("rc-service", "nfs-server", "stop").Run()
		_ = exec.Command("rc-update", "del", "nfs-server", "default").Run()
		_ = exec.Command("rc-service", "nfs-kernel-server", "stop").Run()
		_ = exec.Command("rc-update", "del", "nfs-kernel-server", "default").Run()
		_ = exec.Command("rc-service", "nfs", "stop").Run()
		_ = exec.Command("rc-update", "del", "nfs", "default").Run()
		_ = exec.Command("systemctl", "disable", "nfs-server", "nfs-kernel-server", "nfs").Run()

		var pkgs []string
		switch core.SysInfo.Family {
		case "debian":
			pkgs = []string{"nfs-kernel-server", "nfs-common", "rpcbind"}
		case "rhel":
			pkgs = []string{"nfs-utils", "rpcbind"}
		case "alpine":
			pkgs = []string{"nfs-utils", "nfs-utils-openrc", "rpcbind"}
		default:
			pkgs = []string{"nfs-kernel-server", "nfs-common", "rpcbind"}
		}
		system.RemovePackages(pkgs...)
	}

	if purgeDir && exportDir != "" && exportDir != "/" && exportDir != "/root" && exportDir != "/home" {
		_ = os.RemoveAll(exportDir)
		logger.LogInfo(fmt.Sprintf("Export directory %s has been purged.", exportDir))
	}

	system.RemoveFirewallRules("2049/tcp", "2049/udp", "111/tcp", "111/udp")

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/nfs-manager")
	_ = os.Remove("/usr/local/bin/nfs-installer")
	_ = os.Remove("/usr/local/bin/nfs")

	fmt.Print("\r\n")
	logger.LogSuccess("NFS uninstallation completed successfully.")
	time.Sleep(2 * time.Second)
}
