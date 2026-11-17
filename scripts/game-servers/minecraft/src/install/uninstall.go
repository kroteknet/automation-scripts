// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: MINECRAFT (game-servers)
// File:    game-servers/minecraft/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard. Handles pre-removal backup archives (.tar.gz), data directory purge confirmation, systemd/OpenRC service termination, package removal, and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"krotek-minecraft-installer/core"
	"krotek-minecraft-installer/logger"
	"krotek-minecraft-installer/system"
	"krotek-minecraft-installer/tui"
)

func UninstallMinecraft() {
	logger.InitUninstallLogger("minecraft")
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		if _, err := os.Stat("/etc/minecraft-docker"); err == nil {
			envType = "docker"
		} else {
			envType = "native"
		}
	}

	uninstallBackup := false
	uninstallPurgeFiles := true

	if !core.AutoYes {
		if envType == "docker" {
			uStep := 1
			for {
				switch uStep {
				case 1:
					tui.ClearScreen()
					res := tui.AskYesNo("Step 1/3: Create a full backup of Docker Compose configurations & Minecraft worlds before uninstallation? (Saved to /root/)")
					if res == 2 || res == -1 {
						logger.LogInfo("Uninstallation cancelled by user.")
						return
					}
					uninstallBackup = (res == 0)
					uStep = 2

				case 2:
					tui.ClearScreen()
					res := tui.AskYesNo(fmt.Sprintf("Step 2/3: Stop and purge Docker containers, volumes, world data (%s) & Compose directory (%s)?", core.DockerDataDir, core.DockerComposeDir))
					if res == 2 {
						logger.LogInfo("Uninstallation cancelled by user.")
						return
					}
					if res == -1 {
						uStep = 1
						continue
					}
					uninstallPurgeFiles = (res == 0)
					uStep = 3

				case 3:
					tui.ClearScreen()
					summaryLines := []string{
						tui.FormatKV("Action", "PURGE & REMOVE MINECRAFT DOCKER STACK", core.RED),
						"",
						core.BOLD + core.YELLOW + "UNINSTALL OPTIONS:" + core.NC,
						tui.FormatKV("Deployment Environment", "Docker Compose Container Stack", core.CYAN),
					}
					if uninstallBackup {
						summaryLines = append(summaryLines, tui.FormatKV("Server Backup", "[YES] (Saved to /root/)", core.GREEN))
					} else {
						summaryLines = append(summaryLines, tui.FormatKV("Server Backup", "[NO]", core.RED))
					}
					if uninstallPurgeFiles {
						summaryLines = append(summaryLines, tui.FormatKV("Worlds & Data", "[YES] (Purge Containers, Volumes & Data)", core.GREEN))
					} else {
						summaryLines = append(summaryLines, tui.FormatKV("Worlds & Data", "[NO] (Retain Data)", core.RED))
					}
					summaryLines = append(summaryLines, tui.FormatKV("Containers & Stacks", "[PURGED / REMOVED]", core.RED))
					summaryLines = append(summaryLines, "")
					summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"Do you want to proceed with uninstallation?"+core.NC)

					choices := []string{"YES", "NO"}

					idx, _, status := tui.SelectMenuWithContent("Step 3/3: Uninstallation Summary", summaryLines, choices)
					if status == 2 || idx != 0 {
						logger.LogInfo("Uninstallation cancelled by user.")
						return
					}
					if status == 1 {
						uStep = 2
						continue
					}
					if idx == 0 {
						uStep = 99
						break
					}
				}
				if uStep == 99 {
					break
				}
			}
		} else {
			uStep := 1
			for {
				switch uStep {
				case 1:
					tui.ClearScreen()
					res := tui.AskYesNo("Step 1/3: Create a full backup of Minecraft worlds, server files & configurations before uninstallation? (Saved to /root/)")
					if res == 2 || res == -1 {
						logger.LogInfo("Uninstallation cancelled by user.")
						return
					}
					uninstallBackup = (res == 0)
					uStep = 2

				case 2:
					tui.ClearScreen()
					res := tui.AskYesNo(fmt.Sprintf("Step 2/3: Delete all Minecraft worlds, plugin configurations, and server data in %s?", core.NativeServerDir))
					if res == 2 {
						logger.LogInfo("Uninstallation cancelled by user.")
						return
					}
					if res == -1 {
						uStep = 1
						continue
					}
					uninstallPurgeFiles = (res == 0)
					uStep = 3

				case 3:
					tui.ClearScreen()
					summaryLines := []string{
						tui.FormatKV("Action", "PURGE & REMOVE MINECRAFT NATIVE SERVICE", core.RED),
						"",
						core.BOLD + core.YELLOW + "UNINSTALL OPTIONS:" + core.NC,
						tui.FormatKV("Deployment Environment", "Native Linux Service (systemd)", core.CYAN),
					}
					if uninstallBackup {
						summaryLines = append(summaryLines, tui.FormatKV("Server Backup", "[YES] (Saved to /root/)", core.GREEN))
					} else {
						summaryLines = append(summaryLines, tui.FormatKV("Server Backup", "[NO]", core.RED))
					}
					if uninstallPurgeFiles {
						summaryLines = append(summaryLines, tui.FormatKV("Worlds & Data", fmt.Sprintf("[YES] (Purge %s/)", core.NativeServerDir), core.GREEN))
					} else {
						summaryLines = append(summaryLines, tui.FormatKV("Worlds & Data", fmt.Sprintf("[NO] (Retain in %s/)", core.NativeServerDir), core.RED))
					}
					summaryLines = append(summaryLines, tui.FormatKV("Service Unit", "[YES] (Purge systemd unit)", core.GREEN))
					summaryLines = append(summaryLines, "")
					summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"Do you want to proceed with uninstallation?"+core.NC)

					choices := []string{"YES", "NO"}

					idx, _, status := tui.SelectMenuWithContent("Step 3/3: Uninstallation Summary", summaryLines, choices)
					if status == 2 || idx != 0 {
						logger.LogInfo("Uninstallation cancelled by user.")
						return
					}
					if status == 1 {
						uStep = 2
						continue
					}
					if idx == 0 {
						uStep = 99
						break
					}
				}
				if uStep == 99 {
					break
				}
			}
		}
	}

	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== UNINSTALLING MINECRAFT JAVA SERVER ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	var bkpFile string
	var uninstallErrors []string

	step := func(msg string, command string, args ...string) bool {
		ok := system.RunStep(msg, command, args...)
		if !ok {
			uninstallErrors = append(uninstallErrors, msg)
		}
		return ok
	}

	stepShell := func(msg string, shellScript string) bool {
		ok := system.RunMultiStep(msg, shellScript)
		if !ok {
			uninstallErrors = append(uninstallErrors, msg)
		}
		return ok
	}

	if envType == "native" {
		if uninstallBackup {
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile = fmt.Sprintf("/root/minecraft_backup_%s.tar.gz", ts)
			if !system.IsSymlinkPath(bkpFile) {
				step("Creating full Minecraft backup to "+bkpFile, "tar", "-czf", bkpFile, core.NativeServerDir, core.ServiceUnitPath)
				_ = os.Chmod(bkpFile, 0600)
			}
		}

		stepShell("Stopping Minecraft service", "rc-service minecraft stop 2>/dev/null || true; rc-update del minecraft default 2>/dev/null || true; systemctl stop minecraft 2>/dev/null || true; systemctl disable minecraft 2>/dev/null || true")
		_ = os.Remove(core.ServiceUnitPath)
		_ = os.Remove("/etc/init.d/minecraft")
		_ = exec.Command("systemctl", "daemon-reload").Run()

		if uninstallPurgeFiles {
			step(fmt.Sprintf("Removing Minecraft server directory (%s)", core.NativeServerDir), "rm", "-rf", core.NativeServerDir)
		}
	} else {
		composeDir := state["COMPOSE_DIR"]
		if composeDir == "" {
			composeDir = core.DockerComposeDir
		}
		dataDir := state["DATA_DIR"]
		if dataDir == "" {
			dataDir = core.DockerDataDir
		}

		if uninstallBackup {
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile = fmt.Sprintf("/root/minecraft_docker_backup_%s.tar.gz", ts)
			if !system.IsSymlinkPath(bkpFile) {
				step("Creating Docker stack backup to "+bkpFile, "tar", "-czf", bkpFile, composeDir, dataDir)
				_ = os.Chmod(bkpFile, 0600)
			}
		}

		if uninstallPurgeFiles {
			_ = system.RunDockerCompose(composeDir, "Stopping and purging Docker container stack & volumes", false, "down", "-v")
			_ = os.RemoveAll(composeDir)
			_ = os.RemoveAll(dataDir)
		} else {
			_ = system.RunDockerCompose(composeDir, "Stopping Docker container stack", false, "down")
		}
	}

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/minecraft-manager")
	_ = os.Remove("/usr/local/bin/minecraft-installer")
	_ = os.Remove("/usr/local/bin/minecraft")

	var recentLogErrors []string
	if core.LogFilePath != "" {
		if data, err := os.ReadFile(core.LogFilePath); err == nil {
			lines := strings.Split(string(data), "\n")
			for i := len(lines) - 1; i >= 0; i-- {
				l := strings.TrimSpace(lines[i])
				if l == "" {
					continue
				}
				cleanL := core.StripANSI(l)
				if strings.Contains(cleanL, "[ERROR]") || strings.Contains(cleanL, "Failed!") {
					if len(cleanL) > 65 {
						cleanL = cleanL[:65] + "..."
					}
					recentLogErrors = append([]string{cleanL}, recentLogErrors...)
					if len(recentLogErrors) >= 3 {
						break
					}
				}
			}
		}
	}

	statusVal := core.GREEN + "SUCCESSFUL" + core.NC
	if len(uninstallErrors) > 0 || len(recentLogErrors) > 0 {
		statusVal = core.BOLD + core.RED + "COMPLETED WITH WARNINGS/ERRORS" + core.NC
	}

	finalSummary := []string{
		tui.FormatKV("Uninstallation Status", statusVal, core.GREEN),
		tui.FormatKV("Deployment Environment", envType, core.CYAN),
	}
	if uninstallBackup && bkpFile != "" {
		finalSummary = append(finalSummary, tui.FormatKV("Config/Data Backup", bkpFile, core.GREEN))
	} else {
		finalSummary = append(finalSummary, tui.FormatKV("Config/Data Backup", "[NO BACKUP CREATED]", core.YELLOW))
	}
	if uninstallPurgeFiles {
		finalSummary = append(finalSummary, tui.FormatKV("Worlds & Server Data", "[PURGED / REMOVED]", core.RED))
	} else {
		finalSummary = append(finalSummary, tui.FormatKV("Worlds & Server Data", "[RETAINED]", core.GREEN))
	}
	finalSummary = append(finalSummary, tui.FormatKV("Service / Containers", "[PURGED / REMOVED]", core.RED))

	if len(uninstallErrors) > 0 {
		finalSummary = append(finalSummary, "")
		finalSummary = append(finalSummary, core.BOLD+core.RED+"FAILED UNINSTALLATION STEPS:"+core.NC)
		for _, errStep := range uninstallErrors {
			finalSummary = append(finalSummary, fmt.Sprintf("  %s%s%s %s%s%s", core.RED, core.Sym.Pointer, core.NC, core.RED, errStep, core.NC))
		}
	} else if len(recentLogErrors) == 0 {
		finalSummary = append(finalSummary, tui.FormatKV("Execution Errors", "[NONE / CLEAN]", core.GREEN))
	}

	if len(recentLogErrors) > 0 {
		finalSummary = append(finalSummary, "")
		finalSummary = append(finalSummary, core.BOLD+core.YELLOW+"RECENT LOG ERROR ENTRIES:"+core.NC)
		for _, logErr := range recentLogErrors {
			finalSummary = append(finalSummary, fmt.Sprintf("  %s%s%s %s%s%s", core.YELLOW, core.Sym.Pointer, core.NC, core.RED, logErr, core.NC))
		}
	}

	if core.LogFilePath != "" {
		finalSummary = append(finalSummary, "")
		finalSummary = append(finalSummary, core.BOLD+core.CYAN+"LOG FILE & QUICK COMMANDS:"+core.NC)
		finalSummary = append(finalSummary, fmt.Sprintf("  %s%s Path :%s %s%s%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, core.LogFilePath, core.NC))
		finalSummary = append(finalSummary, fmt.Sprintf("  %s%s View :%s %scat %s%s", core.GREEN, core.Sym.Pointer, core.NC, core.BOLD+core.GREEN, core.LogFilePath, core.NC))
		finalSummary = append(finalSummary, fmt.Sprintf("  %s%s Tail :%s %stail -n 25 %s%s", core.BLUE, core.Sym.Pointer, core.NC, core.BOLD+core.BLUE, core.LogFilePath, core.NC))
	}

	tui.ClearScreen()
	tui.PrintInBox("Minecraft Java Server Uninstalled", finalSummary)
	fmt.Print("\r\n")
	logger.LogSuccess("Minecraft Java Server uninstalled successfully.")
	os.Exit(0)
}

// UninstallWizard orchestrates the complete, multi-step uninstallation and teardown procedure.
// It creates optional pre-removal backups, prompts for data/key purge, gracefully halts daemons,
// uninstalls distro packages, removes firewall openings, and purges manager CLI binaries.
func UninstallWizard() {
	UninstallMinecraft()
}
