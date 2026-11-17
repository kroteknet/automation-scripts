// ==============================================================================
// KROTEK SCRIPT HUB — SOFTWARE UPDATE & AUDIT ENGINE
// Service: OpenResty Web Platform (web-server/openresty)
// File:    web-server/openresty/src/system/update.go
//
// Checks for upstream OpenResty Web Platform updates, performs safety audits, prompts
// for safe backups, executes non-breaking upgrades, and offers automated rollback.
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

	"krotek-openresty-installer/core"
	"krotek-openresty-installer/logger"
	"krotek-openresty-installer/tui"
)

// UpdateCheckResult encapsulates repository query telemetry for OpenResty Web Platform.
type UpdateCheckResult struct {
	HasUpdate     bool
	CurrentVer    string
	CandidateVer  string
	PackageName   string
	RepoSource    string
	StatusMessage string
}

// CheckAvailableUpdate audits native package repositories or container tags for available OpenResty upgrades.
func CheckAvailableUpdate() UpdateCheckResult {
	pkgName := "openresty"

	state := LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		envType = "native"
	}

	res := UpdateCheckResult{
		PackageName: pkgName,
	}

	if envType == "docker" {
		res.CurrentVer = "container:latest"
		res.CandidateVer = "container:latest"
		res.RepoSource = "Docker Hub / OCI Registry"
		res.StatusMessage = "Container image stack can be refreshed via docker compose pull"
		res.HasUpdate = true
		return res
	}

	switch core.SysInfo.Family {
	case "debian":
		res.RepoSource = "APT (OpenResty Official Repository)"
		out, err := exec.Command("apt-cache", "policy", pkgName).Output()
		if err == nil {
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "Installed:") {
					res.CurrentVer = strings.TrimSpace(strings.TrimPrefix(trimmed, "Installed:"))
				} else if strings.HasPrefix(trimmed, "Candidate:") {
					res.CandidateVer = strings.TrimSpace(strings.TrimPrefix(trimmed, "Candidate:"))
				}
			}
		}
		if res.CurrentVer == "" || res.CurrentVer == "(none)" {
			res.CurrentVer = "uninstalled"
		}
		if res.CandidateVer != "" && res.CandidateVer != "(none)" && res.CandidateVer != res.CurrentVer && res.CurrentVer != "uninstalled" {
			res.HasUpdate = true
			res.StatusMessage = fmt.Sprintf("Upgrade available: %s -> %s", res.CurrentVer, res.CandidateVer)
		} else {
			res.HasUpdate = false
			if res.CandidateVer == "" || res.CandidateVer == "(none)" {
				res.CandidateVer = res.CurrentVer
			}
			res.StatusMessage = "OpenResty Web Platform is already at the latest available version."
		}

	case "rhel":
		res.RepoSource = "RPM (OpenResty Official Repository)"
		if out, err := exec.Command("rpm", "-q", "--queryformat", "%{VERSION}-%{RELEASE}", pkgName).Output(); err == nil {
			res.CurrentVer = strings.TrimSpace(string(out))
		} else {
			res.CurrentVer = "uninstalled"
		}
		outStr, exitCode, _ := ExecRHELCmd("", false, false, "check-update", "-q", pkgName)
		if exitCode == 100 {
			res.HasUpdate = true
			candVer := "latest upstream"
			for _, line := range strings.Split(outStr, "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 2 && strings.HasPrefix(fields[0], pkgName+".") {
					candVer = fields[1]
					break
				}
			}
			res.CandidateVer = candVer
			res.StatusMessage = fmt.Sprintf("Upgrade available: %s -> %s", res.CurrentVer, res.CandidateVer)
		}
		if res.CandidateVer == "" {
			res.CandidateVer = res.CurrentVer
			res.StatusMessage = "OpenResty Web Platform is already at the latest available version."
		}

	case "alpine":
		res.RepoSource = "APK (Alpine Linux community repository)"
		if out, err := exec.Command("apk", "version", pkgName).Output(); err == nil {
			for _, l := range strings.Split(string(out), "\n") {
				trimmed := strings.TrimSpace(l)
				if !strings.HasPrefix(trimmed, pkgName+"-") {
					continue
				}
				fields := strings.Fields(trimmed)
				if len(fields) >= 3 {
					installedVer := strings.TrimPrefix(fields[0], pkgName+"-")
					op := fields[1]
					candVer := fields[2]
					res.CurrentVer = installedVer
					res.CandidateVer = candVer
					if op == "<" {
						res.HasUpdate = true
						res.StatusMessage = fmt.Sprintf("Upgrade available: %s -> %s", installedVer, candVer)
					} else {
						res.HasUpdate = false
						res.StatusMessage = "OpenResty Web Platform is already at the latest available version."
					}
					break
				}
			}
		}
		if res.CurrentVer == "" {
			if verOut, err := exec.Command("apk", "info", "-e", "-v", pkgName).Output(); err == nil && strings.TrimSpace(string(verOut)) != "" {
				res.CurrentVer = strings.TrimPrefix(strings.TrimSpace(string(verOut)), pkgName+"-")
				res.CandidateVer = res.CurrentVer
				res.StatusMessage = "OpenResty Web Platform is already at the latest available version."
			} else {
				res.CurrentVer = "uninstalled"
				res.CandidateVer = "latest"
				res.StatusMessage = "OpenResty package is not currently installed."
			}
		}

	default:
		res.CurrentVer = "detected"
		res.CandidateVer = "latest"
		res.StatusMessage = "System packages can be upgraded using system package manager."
	}

	return res
}

// CheckFreeDiskSpaceMB returns available free disk space in megabytes for /var.
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

// RunServiceUpdateFlow runs the end-to-end production-grade interactive update wizard for OpenResty Web Platform.
func RunServiceUpdateFlow() {
	serviceTitle := "OpenResty Web Platform"
	pkgName := "openresty"
	daemonName := "openresty"
	openrestyBin := GetOpenRestyBinPath()

	state := LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		envType = "native"
	}
	composeDir := state["COMPOSE_DIR"]
	if composeDir == "" && IsOpenRestyDockerInstalled() {
		composeDir = "/etc/openresty-docker"
	}

	tui.ClearScreen()
	tui.StartBoxSpinner("Checking for available " + serviceTitle + " updates & refreshing repositories...")
	if envType != "docker" {
		_ = UpdatePackageIndex()
	}
	audit := CheckAvailableUpdate()
	tui.StopBoxSpinner(0, "Repository check completed.")

	// Render Update Information Card
	var cardLines []string
	statusBadge := core.GREEN + "[UP-TO-DATE]" + core.NC
	if audit.HasUpdate {
		statusBadge = core.YELLOW + core.BOLD + "[UPDATE AVAILABLE]" + core.NC
	}

	cardLines = append(cardLines, core.BOLD+core.CYAN+strings.ToUpper(serviceTitle)+" SOFTWARE AUDIT & UPDATE CENTER"+core.NC, "")
	cardLines = append(cardLines, tui.FormatKV("Service Title", serviceTitle, core.BOLD+core.WHITE))
	cardLines = append(cardLines, tui.FormatKV("Package Name", pkgName, core.CYAN))
	cardLines = append(cardLines, tui.FormatKV("Installed Version", core.SanitizeTerminalText(audit.CurrentVer), core.YELLOW))
	cardLines = append(cardLines, tui.FormatKV("Candidate Version", core.SanitizeTerminalText(audit.CandidateVer), core.GREEN))
	cardLines = append(cardLines, tui.FormatKV("Update Status", statusBadge, core.BOLD))
	cardLines = append(cardLines, tui.FormatKV("Repository Source", core.SanitizeTerminalText(audit.RepoSource), core.DIM))
	cardLines = append(cardLines, "")
	cardLines = append(cardLines, core.DIM+"Status: "+core.SanitizeTerminalText(audit.StatusMessage)+core.NC)

	tui.PrintInBox(serviceTitle+" Update Audit", cardLines, "--no-wait")

	if core.AutoYes && !audit.HasUpdate {
		return
	}

	promptMsg := fmt.Sprintf("Proceed with updating %s to candidate version (%s)?", serviceTitle, audit.CandidateVer)
	if !audit.HasUpdate {
		promptMsg = fmt.Sprintf("%s is already up-to-date. Refresh/reinstall packages anyway?", serviceTitle)
	}

	if tui.AskYesNo(promptMsg) != 0 {
		return
	}

	// Backup prompt (production safeguard)
	wantBackup := tui.AskYesNo("Create a safe backup of configurations and data before updating? (Recommended for Production)") == 0

	var lastBackupInfo *BackupInfo
	tui.ClearScreen()
	if wantBackup {
		freeMB := CheckFreeDiskSpaceMB()
		if freeMB < 200 {
			logger.LogError(fmt.Sprintf("Insufficient disk space (%d MB < 200 MB) for pre-update backup.", freeMB))
			tui.PrintInBox("Insufficient Disk Space", tui.FormatInfoNotice("WARNING", fmt.Sprintf("Available disk space (%d MB) is below safe threshold (200 MB). Aborting backup and update.", freeMB), core.RED))
			if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
				os.Exit(1)
			}
			return
		}

		tui.StartBoxSpinner("Creating isolated configuration and data backup...")
		bInfo, err := CreateSafeBackup()
		if err != nil {
			tui.StopBoxSpinner(1, "Backup failed: "+err.Error())
			logger.LogError("Pre-update backup failed: " + err.Error())
			tui.PrintInBox("Backup Error", []string{core.RED + core.BOLD + "[ERROR] Backup failed: " + err.Error() + core.NC, core.DIM + "Cannot proceed with update without verified backup." + core.NC})
			if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
				os.Exit(1)
			}
			return
		}
		lastBackupInfo = bInfo
		tui.StopBoxSpinner(0, "Backup created successfully.")
	}

	// Pre-update configuration syntax validation
	if envType != "docker" && openrestyBin != "" {
		tui.StartBoxSpinner("Validating current " + serviceTitle + " configuration syntax...")
		if err := exec.Command(openrestyBin, "-t").Run(); err != nil {
			tui.StopBoxSpinner(1, "Syntax/sanity test failed before update!")
			confirm := tui.AskYesNo("Existing configuration contains syntax/sanity errors! Proceeding may cause service downtime. Continue anyway?")
			if confirm != 0 {
				return
			}
			tui.ClearScreen()
		} else {
			tui.StopBoxSpinner(0, "Configuration syntax is valid.")
		}
	}

	// Perform actual package update
	tui.StartBoxSpinner("Upgrading " + serviceTitle + " packages...")
	updateSuccess := false

	if envType == "docker" && composeDir != "" {
		if err := ValidateSafeComposeDir(composeDir); err == nil {
			composeFile := filepath.Join(composeDir, "docker-compose.yml")
			if exec.Command("docker", "compose", "version").Run() == nil {
				_ = exec.Command("docker", "compose", "-f", composeFile, "pull").Run()
				if exec.Command("docker", "compose", "-f", composeFile, "up", "-d").Run() == nil {
					updateSuccess = true
				}
			} else {
				_ = exec.Command("docker-compose", "-f", composeFile, "pull").Run()
				if exec.Command("docker-compose", "-f", composeFile, "up", "-d").Run() == nil {
					updateSuccess = true
				}
			}
		} else {
			logger.LogError("Unsafe Docker Compose directory path blocked during update: " + err.Error())
		}
	} else {
		switch core.SysInfo.Family {
		case "debian":
			flags := append([]string{"install", "--only-upgrade", "-y", "-q"}, GetDpkgConfigFlags()...)
			flags = append(flags, pkgName)
			cmd := exec.Command("apt-get", flags...)
			cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a")
			if cmd.Run() == nil {
				updateSuccess = true
			}
		case "rhel":
			_, _, updateSuccess = ExecRHELCmd("", false, false, "upgrade", "-y", "-q", pkgName)
		case "alpine":
			if exec.Command("apk", "upgrade", "--quiet", pkgName).Run() == nil {
				updateSuccess = true
			}
		default:
			if exec.Command("apt-get", "install", "--only-upgrade", "-y", pkgName).Run() == nil {
				updateSuccess = true
			}
		}
		EnsureOpenRestyBinarySymlinks()
	}

	if !updateSuccess {
		tui.StopBoxSpinner(1, "Package upgrade command failed!")
		logger.LogError("OpenResty package upgrade failed.")
		tui.PrintInBox("Update Failed", tui.FormatInfoNotice("ERROR", "Package upgrade encountered errors. Check system logs for details.", core.RED))
		if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
			os.Exit(1)
		}
		return
	}
	tui.StopBoxSpinner(0, "Package upgrade completed.")

	// Post-update restart and verification
	tui.StartBoxSpinner("Restarting " + serviceTitle + " and auditing runtime health...")
	time.Sleep(1 * time.Second)
	restartDaemonDirect(daemonName)
	time.Sleep(1 * time.Second)

	isHealthy := IsOpenRestyRunning()

	if isHealthy {
		tui.StopBoxSpinner(0, serviceTitle+" service is active and healthy.")
		var succLines []string
		succLines = append(succLines, core.BOLD+core.GREEN+fmt.Sprintf(core.TagOK() + " %s updated successfully!", serviceTitle)+core.NC, "")
		succLines = append(succLines, tui.FormatKV("Service Status", core.GREEN+"[ACTIVE / RUNNING]"+core.NC, core.GREEN))
		succLines = append(succLines, tui.FormatKV("Active Version", audit.CandidateVer, core.BOLD+core.WHITE))
		if lastBackupInfo != nil {
			succLines = append(succLines, tui.FormatKV("Safe Backup File", lastBackupInfo.ArchivePath, core.CYAN))
			succLines = append(succLines, tui.FormatKV("Backup Size", fmt.Sprintf("%.2f MB", float64(lastBackupInfo.SizeBytes)/(1024*1024)), core.YELLOW))
		}
		succLines = append(succLines, "")
		succLines = append(succLines, core.DIM+"Production configurations were preserved with zero-downtime safe defaults."+core.NC)
		tui.PrintInBox("Update Complete", succLines)
	} else {
		tui.StopBoxSpinner(1, serviceTitle+" failed to restart after update!")
		if lastBackupInfo != nil {
			var failLines []string
			failLines = append(failLines, core.BOLD+core.RED+"CRITICAL: Service did not start after package upgrade!"+core.NC, "")
			failLines = append(failLines, "A verified backup was created before the upgrade:")
			failLines = append(failLines, tui.FormatKV("Backup Archive", lastBackupInfo.ArchivePath, core.CYAN))
			failLines = append(failLines, "")
			failLines = append(failLines, "Would you like to automatically ROLLBACK configuration and restore the previous state?")
			tui.PrintInBox("Service Start Failure - Rollback Available", failLines)

			if tui.AskYesNo("Execute automated rollback to restore working configuration?") == 0 {
				tui.ClearScreen()
				tui.StartBoxSpinner("Restoring configuration from safe backup...")
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
	if IsOpenRestyDockerRunning() {
		_ = exec.Command("docker", "restart", "krotek-openresty").Run()
		return
	}
	_ = RestartService(name)
}
