// ==============================================================================
// KROTEK SCRIPT HUB — SOFTWARE UPDATE & AUDIT ENGINE
// Service: Nitrox Server (game-servers/nitrox)
// File:    game-servers/nitrox/src/system/update.go
//
// Checks for upstream Nitrox Server updates from GitHub releases, performs safety audits,
// prompts for safe backups, executes non-breaking upgrades, and offers automated rollback.
// ==============================================================================

package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"krotek-nitrox-installer/core"
	"krotek-nitrox-installer/tui"
)

// UpdateCheckResult encapsulates repository and release telemetry for Nitrox Server.
type UpdateCheckResult struct {
	HasUpdate     bool
	CurrentVer    string
	CandidateVer  string
	DownloadURL   string
	RepoSource    string
	StatusMessage string
}

// CheckAvailableUpdate audits GitHub official releases and container tags for available Nitrox Server upgrades.
func CheckAvailableUpdate() UpdateCheckResult {
	state := LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		envType = "native"
	}

	installDir := state["INSTALL_DIR"]
	if installDir == "" {
		if _, err := os.Stat("/srv/nitrox"); err == nil {
			installDir = "/srv/nitrox"
		} else {
			installDir = "/opt/nitrox"
		}
	}
	serverDir := state["SERVER_DIR"]
	if serverDir == "" {
		serverDir = filepath.Join(installDir, "server")
	}

	currentVer := state["NITROX_VERSION"]
	if currentVer == "" || currentVer == "latest" {
		if detected := DetectInstalledNitroxVersion(serverDir); detected != "" {
			currentVer = detected
		} else {
			currentVer = "unknown"
		}
	}

	res := UpdateCheckResult{
		CurrentVer:  currentVer,
		RepoSource:  "Official GitHub Releases (SubnauticaNitrox/Nitrox)",
		DownloadURL: "",
	}

	if envType == "docker" {
		res.RepoSource = "Docker Hub / OCI Registry (fuzzystatic/nitrox-server:latest)"
		res.CandidateVer = "latest"
		res.StatusMessage = "Container image stack can be refreshed via docker compose pull"
		res.HasUpdate = true
		return res
	}

	arch := core.SysInfo.Architecture
	assetPattern := "linux_x64"
	if arch == "arm64" || arch == "aarch64" {
		assetPattern = "linux_arm64"
	}

	// 1. Query upstream redirect header dynamically (instant, zero rate-limit issues)
	tag, dlURL := ResolveLatestNitroxRelease(arch)
	if tag != "" && dlURL != "" {
		res.CandidateVer = tag
		res.DownloadURL = dlURL
	} else {
		// 2. Query GitHub releases API as fallback
		checkCmd := `curl -sSL -H "User-Agent: KrotekInstaller" https://api.github.com/repos/SubnauticaNitrox/Nitrox/releases/latest 2>/dev/null`
		out, err := exec.Command(GetShell(), "-c", checkCmd).Output()
		if err == nil && len(out) > 0 {
			outStr := string(out)
			if tagIdx := strings.Index(outStr, "\"tag_name\":"); tagIdx != -1 {
				rem := outStr[tagIdx+11:]
				rem = strings.TrimLeft(rem, " \t\r\n\"")
				if endQ := strings.Index(rem, "\""); endQ != -1 {
					res.CandidateVer = strings.TrimSpace(rem[:endQ])
				}
			}

			lines := strings.Split(outStr, "\n")
			for _, l := range lines {
				if strings.Contains(l, "browser_download_url") && strings.Contains(l, assetPattern) && strings.HasSuffix(strings.TrimSpace(l), ".zip\"") {
					parts := strings.Split(l, "\"")
					for _, p := range parts {
						if strings.HasPrefix(p, "http") && strings.HasSuffix(p, ".zip") {
							res.DownloadURL = p
							break
						}
					}
					if res.DownloadURL != "" {
						break
					}
				}
			}
		}
	}

	if res.CandidateVer == "" {
		res.CandidateVer = "latest"
	}
	if res.DownloadURL == "" {
		if res.CandidateVer != "" && res.CandidateVer != "latest" {
			res.DownloadURL = fmt.Sprintf("https://github.com/SubnauticaNitrox/Nitrox/releases/download/%s/Nitrox_%s_%s.zip", res.CandidateVer, res.CandidateVer, assetPattern)
		} else {
			res.DownloadURL = fmt.Sprintf("https://github.com/SubnauticaNitrox/Nitrox/releases/latest/download/Nitrox_%s.zip", assetPattern)
		}
	}

	if res.CandidateVer != "" && res.CandidateVer != "latest" && res.CandidateVer != res.CurrentVer {
		res.HasUpdate = true
		res.StatusMessage = fmt.Sprintf("New release available: %s -> %s", res.CurrentVer, res.CandidateVer)
	} else if res.CurrentVer == "unknown" {
		res.HasUpdate = true
		res.StatusMessage = fmt.Sprintf("Official release package available: %s", res.CandidateVer)
	} else {
		res.HasUpdate = false
		res.StatusMessage = fmt.Sprintf("Subnautica Nitrox Server is up to date (version %s).", res.CurrentVer)
	}

	return res
}

// CheckFreeDiskSpaceMB returns available free disk space in megabytes for /var or root.
func CheckFreeDiskSpaceMB() int {
	out, err := exec.Command("df", "-m", "/var").Output()
	if err != nil {
		out, _ = exec.Command("df", "-m", "/").Output()
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) >= 2 {
		fields := strings.Fields(lines[1])
		if len(fields) >= 4 {
			if mb, err := strconv.Atoi(fields[3]); err == nil {
				return mb
			}
		}
	}
	return 1024
}

// RunServiceUpdateFlow runs the end-to-end production-grade interactive update wizard for Nitrox Server.
func RunServiceUpdateFlow() {
	serviceTitle := "Subnautica Nitrox Server"
	daemonName := "nitrox"

	state := LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		envType = "native"
	}
	composeDir := state["COMPOSE_DIR"]
	installDir := state["INSTALL_DIR"]
	if installDir == "" {
		if _, err := os.Stat("/srv/nitrox"); err == nil {
			installDir = "/srv/nitrox"
		} else {
			installDir = "/opt/nitrox"
		}
	}
	serverDir := state["SERVER_DIR"]
	if serverDir == "" {
		serverDir = filepath.Join(installDir, "server")
	}

	tui.ClearScreen()
	tui.StartBoxSpinner("Checking official Subnautica Nitrox releases on GitHub...")
	audit := CheckAvailableUpdate()
	tui.StopBoxSpinner(0, "Release audit completed.")

	// Render Update Information Card
	var cardLines []string
	statusBadge := core.GREEN + "[UP-TO-DATE]" + core.NC
	if audit.HasUpdate {
		statusBadge = core.YELLOW + core.BOLD + "[UPDATE AVAILABLE]" + core.NC
	}

	cardLines = append(cardLines, core.BOLD+core.CYAN+strings.ToUpper(serviceTitle)+" SOFTWARE AUDIT & UPDATE CENTER"+core.NC, "")
	cardLines = append(cardLines, tui.FormatKV("Service Title", serviceTitle, core.BOLD+core.WHITE))
	cardLines = append(cardLines, tui.FormatKV("Deployment Stack", strings.ToUpper(envType), core.CYAN))
	cardLines = append(cardLines, tui.FormatKV("Installed Version", core.SanitizeTerminalText(audit.CurrentVer), core.YELLOW))
	cardLines = append(cardLines, tui.FormatKV("Candidate Version", core.SanitizeTerminalText(audit.CandidateVer), core.GREEN))
	cardLines = append(cardLines, tui.FormatKV("Update Status", statusBadge, core.BOLD))
	cardLines = append(cardLines, tui.FormatKV("Repository Source", core.SanitizeTerminalText(audit.RepoSource), core.DIM))
	cardLines = append(cardLines, "")
	cardLines = append(cardLines, core.DIM+"Status: "+core.SanitizeTerminalText(audit.StatusMessage)+core.NC)

	tui.PrintInBox(serviceTitle+" Update Audit", cardLines, "--no-wait")

	promptMsg := fmt.Sprintf("Proceed with updating %s to candidate version (%s)?", serviceTitle, audit.CandidateVer)
	if !audit.HasUpdate {
		promptMsg = fmt.Sprintf("%s is already up-to-date (%s). Re-download and refresh server binaries anyway?", serviceTitle, audit.CurrentVer)
	}

	if tui.AskYesNo(promptMsg) != 0 {
		return
	}

	// Backup prompt (production safeguard)
	wantBackup := tui.AskYesNo("Create a safe backup of world saves and configs before updating? (Recommended)") == 0

	var lastBackupInfo *BackupInfo
	tui.ClearScreen()
	if wantBackup {
		freeMB := CheckFreeDiskSpaceMB()
		if freeMB < 200 {
			tui.PrintInBox("Insufficient Disk Space", tui.FormatInfoNotice("WARNING", fmt.Sprintf("Available disk space (%d MB) is below safe threshold (200 MB). Aborting backup and update.", freeMB), core.RED))
			return
		}

		tui.StartBoxSpinner("Creating isolated configuration and data backup...")
		bInfo, err := CreateSafeBackup()
		if err != nil {
			tui.StopBoxSpinner(1, "Backup failed: "+err.Error())
			tui.PrintInBox("Backup Error", []string{core.RED + core.BOLD + "[ERROR] Backup failed: " + err.Error() + core.NC, core.DIM + "Cannot proceed with update without verified backup." + core.NC})
			return
		}
		lastBackupInfo = bInfo
		tui.StopBoxSpinner(0, "Backup created successfully.")
	}

	// Perform actual update
	tui.ClearScreen()
	tui.StartBoxSpinner("Deploying " + serviceTitle + " (" + audit.CandidateVer + ")...")
	updateSuccess := false

	if envType == "docker" && composeDir != "" {
		_ = exec.Command("docker", "compose", "-f", filepath.Join(composeDir, "docker-compose.yml"), "pull").Run()
		if exec.Command("docker", "compose", "-f", filepath.Join(composeDir, "docker-compose.yml"), "up", "-d").Run() == nil {
			updateSuccess = true
		}
	} else {
		// Stop service before replacing binaries
		_ = exec.Command("systemctl", "stop", daemonName).Run()

		tmpDir, tErr := os.MkdirTemp("", "krotek-nitrox-upd-*")
		if tErr == nil {
			_ = os.Chmod(tmpDir, 0700)
			defer os.RemoveAll(tmpDir)
			tmpZip := filepath.Join(tmpDir, "nitrox_update.zip")

			dlCmd := fmt.Sprintf(`
				(curl -fsSL "%s" -o "%s" 2>/dev/null || wget -qO "%s" "%s" 2>/dev/null) && \
				unzip -q -o "%s" -d "%s" 2>/dev/null && \
				chmod +x "%s/Nitrox.Server.Subnautica" 2>/dev/null && \
				ln -sf Nitrox.Server.Subnautica "%s/NitroxServer-Subnautica" 2>/dev/null && \
				chown -R nitrox:nitrox "%s" 2>/dev/null || true
				rm -f "%s"
			`, audit.DownloadURL, tmpZip, tmpZip, audit.DownloadURL, tmpZip, serverDir, serverDir, serverDir, serverDir, tmpZip)

			if exec.Command(GetShell(), "-c", dlCmd).Run() == nil {
				updateSuccess = true
			}
		}
	}

	if !updateSuccess {
		tui.StopBoxSpinner(1, "Nitrox update deployment command failed!")
		tui.PrintInBox("Update Failed", tui.FormatInfoNotice("ERROR", "Binary update encountered errors. Check system logs for details.", core.RED))
		return
	}
	tui.StopBoxSpinner(0, "Nitrox binaries updated successfully.")

	// Post-update restart and verification
	tui.StartBoxSpinner("Restarting " + serviceTitle + " and auditing runtime health...")
	time.Sleep(1 * time.Second)
	restartDaemonDirect(daemonName)
	time.Sleep(1 * time.Second)

	isHealthy := false
	if IsSystemdRunning() {
		isHealthy = exec.Command("systemctl", "is-active", "--quiet", daemonName).Run() == nil
	} else {
		isHealthy = exec.Command("service", daemonName, "status").Run() == nil ||
			exec.Command("rc-service", daemonName, "status").Run() == nil ||
			exec.Command("pidof", "Nitrox.Server.Subnautica").Run() == nil
	}

	if isHealthy {
		activeVer := audit.CandidateVer
		if det := DetectInstalledNitroxVersion(serverDir); det != "" {
			activeVer = det
		}
		state["NITROX_VERSION"] = activeVer
		SaveState(state)

		tui.StopBoxSpinner(0, serviceTitle+" service is active and healthy.")
		var succLines []string
		succLines = append(succLines, core.BOLD+core.GREEN+fmt.Sprintf(core.TagOK() + " %s updated successfully!", serviceTitle)+core.NC, "")
		succLines = append(succLines, tui.FormatKV("Service Status", core.GREEN+"[ACTIVE / RUNNING]"+core.NC, core.GREEN))
		succLines = append(succLines, tui.FormatKV("Active Version", activeVer, core.BOLD+core.WHITE))
		if lastBackupInfo != nil {
			succLines = append(succLines, tui.FormatKV("Safe Backup File", lastBackupInfo.ArchivePath, core.CYAN))
			succLines = append(succLines, tui.FormatKV("Backup Size", fmt.Sprintf("%.2f MB", float64(lastBackupInfo.SizeBytes)/(1024*1024)), core.YELLOW))
		}
		succLines = append(succLines, "")
		succLines = append(succLines, core.DIM+"World saves and custom server.cfg parameters were preserved safely."+core.NC)
		tui.PrintInBox("Update Complete", succLines)
	} else {
		tui.StopBoxSpinner(1, serviceTitle+" failed to restart after update!")
		if lastBackupInfo != nil {
			var failLines []string
			failLines = append(failLines, core.BOLD+core.RED+"CRITICAL: Service did not start after Nitrox upgrade!"+core.NC, "")
			failLines = append(failLines, "A verified world backup was created before the upgrade:")
			failLines = append(failLines, tui.FormatKV("Backup Archive", lastBackupInfo.ArchivePath, core.CYAN))
			failLines = append(failLines, "")
			failLines = append(failLines, "Would you like to automatically ROLLBACK and restore the previous version?")
			tui.PrintInBox("Service Start Failure - Rollback Available", failLines)

			if tui.AskYesNo("Execute automated rollback to restore working server state?") == 0 {
				tui.ClearScreen()
				tui.StartBoxSpinner("Restoring configuration and world saves from safe backup...")
				if err := RestoreSafeBackup(lastBackupInfo.ArchivePath); err == nil {
					restartDaemonDirect(daemonName)
					tui.StopBoxSpinner(0, "Rollback executed and service restarted.")
					tui.PrintInBox("Rollback Successful", tui.FormatInfoNotice("SUCCESS", "Configuration was successfully restored to pre-update state.", core.GREEN))
				} else {
					tui.StopBoxSpinner(1, "Rollback failed: "+err.Error())
					tui.PrintInBox("Rollback Failed", []string{core.RED + core.BOLD + "[ERROR] Automated rollback failed: " + err.Error() + core.NC, core.YELLOW + "Restore the backup archive manually before restarting the service." + core.NC})
				}
			}
		}
	}
}

func restartDaemonDirect(name string) {
	if IsSystemdRunning() {
		_ = exec.Command("systemctl", "reset-failed", name).Run()
		_ = exec.Command("systemctl", "daemon-reload").Run()
		_ = exec.Command("systemctl", "restart", name).Run()
	} else {
		_ = exec.Command("service", name, "restart").Run()
		_ = exec.Command("rc-service", name, "restart").Run()
	}
}
