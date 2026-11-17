// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED UNINSTALLATION & TEARDOWN WIZARD
// Service: OpenResty (web-server)
// File:    web-server/openresty/src/install/uninstall.go
//
// Provides a safe, multi-step uninstallation wizard for OpenResty. Handles
// pre-removal backup archives (.tar.gz), optional web root purge, systemd/OpenRC
// daemon termination, OpenResty package/repository removal (preserving host
// vanilla NGINX on Debian/RHEL), and global binary cleanup.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"krotek-openresty-installer/core"
	"krotek-openresty-installer/logger"
	"krotek-openresty-installer/system"
	"krotek-openresty-installer/tui"
)

func isSafeComposeDirForPurge(dir string) bool {
	cleaned := filepath.Clean(strings.TrimSpace(dir))
	if cleaned == "" || cleaned == "/" || cleaned == "/etc" || cleaned == "/etc/nginx" || cleaned == "/etc/openresty" {
		return false
	}
	if system.IsSymlinkPath(cleaned) {
		return false
	}
	return system.ValidateSafeComposeDir(cleaned) == nil
}

func removeIfSymlinkToOpenResty(path string) {
	fi, err := os.Lstat(path)
	if err != nil {
		return
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err == nil && (strings.Contains(target, "openresty") || strings.Contains(target, "/usr/lib/nginx")) {
			_ = os.Remove(path)
		}
	}
}

func UninstallOpenResty() {
	logger.InitUninstallLogger("openresty")
	state := system.LoadState()
	envType := state["ENV_TYPE"]
	if envType == "" {
		if system.IsOpenRestyDockerInstalled() {
			envType = "docker"
		} else {
			envType = "native"
		}
	}

	uninstallBackup := false
	uninstallPurgeFiles := true
	uninstallPurgeConf := true

	if !core.AutoYes {
		if envType == "docker" {
			uStep := 1
			for {
				switch uStep {
				case 1:
					tui.ClearScreen()
					res := tui.AskYesNo("Step 1/3: Create a full backup of OpenResty Docker Compose configs & web files before uninstallation? (Saved to /root/)")
					if res == 2 || res == -1 {
						return
					}
					uninstallBackup = (res == 0)
					uStep = 2

				case 2:
					tui.ClearScreen()
					res := tui.AskYesNo("Step 2/3: Stop and purge Docker containers, volumes & Compose directory (/etc/openresty-docker)?")
					if res == 2 {
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
						tui.FormatKV("Action", "PURGE & REMOVE OPENRESTY DOCKER STACK", core.RED),
						"",
						core.BOLD + core.YELLOW + "UNINSTALL OPTIONS:" + core.NC,
					}
					if uninstallBackup {
						summaryLines = append(summaryLines, tui.FormatKV("Backup", "[YES] (Saved to /root/)", core.GREEN))
					} else {
						summaryLines = append(summaryLines, tui.FormatKV("Backup", "[NO]", core.RED))
					}
					if uninstallPurgeFiles {
						summaryLines = append(summaryLines, tui.FormatKV("Purge Stack", "[YES] (Containers, Volumes & Files)", core.GREEN))
					} else {
						summaryLines = append(summaryLines, tui.FormatKV("Purge Stack", "[NO]", core.RED))
					}
					summaryLines = append(summaryLines, "")
					summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"Do you want to proceed with uninstallation?"+core.NC)

					idx, _, status := tui.SelectMenuWithContent("Step 3/3: Uninstallation Summary", summaryLines, []string{"YES", "NO"})
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
					res := tui.AskYesNo("Step 1/4: Create a full backup of OpenResty configurations & web files before uninstallation? (Saved to /root/)")
					if res == 2 || res == -1 {
						return
					}
					uninstallBackup = (res == 0)
					uStep = 2

				case 2:
					tui.ClearScreen()
					res := tui.AskYesNo("Step 2/4: Delete managed web root files (/var/www/html & configured site root)?")
					if res == 2 {
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
					res := tui.AskYesNo("Step 3/4: Purge OpenResty packages, Lua configs, and repositories?")
					if res == 2 {
						return
					}
					if res == -1 {
						uStep = 2
						continue
					}
					uninstallPurgeConf = (res == 0)
					uStep = 4

				case 4:
					tui.ClearScreen()
					confSummaryLabel := "[YES] (Purge OpenResty & Repos)"
					summaryLines := []string{
						tui.FormatKV("Action", "PURGE & REMOVE OPENRESTY NATIVE STACK", core.RED),
						"",
						core.BOLD + core.YELLOW + "UNINSTALL OPTIONS:" + core.NC,
					}
					if uninstallBackup {
						summaryLines = append(summaryLines, tui.FormatKV("Backup", "[YES] (Saved to /root/)", core.GREEN))
					} else {
						summaryLines = append(summaryLines, tui.FormatKV("Backup", "[NO]", core.RED))
					}
					if uninstallPurgeFiles {
						summaryLines = append(summaryLines, tui.FormatKV("Files", "[YES] (Purge managed web roots)", core.GREEN))
					} else {
						summaryLines = append(summaryLines, tui.FormatKV("Files", "[NO]", core.RED))
					}
					if uninstallPurgeConf {
						summaryLines = append(summaryLines, tui.FormatKV("Configs", confSummaryLabel, core.GREEN))
					} else {
						summaryLines = append(summaryLines, tui.FormatKV("Configs", "[NO]", core.RED))
					}
					summaryLines = append(summaryLines, "")
					summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"Do you want to proceed with uninstallation?"+core.NC)

					idx, _, status := tui.SelectMenuWithContent("Step 4/4: Uninstallation Summary", summaryLines, []string{"YES", "NO"})
					if status == 2 || idx != 0 {
						logger.LogInfo("Uninstallation cancelled by user.")
						return
					}
					if status == 1 {
						uStep = 3
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
	fmt.Printf("\r\n%s=== UNINSTALLING OPENRESTY SERVER ===%s\r\n\r\n", core.BOLD+core.RED, core.NC)

	var bkpFile string
	var uninstallErrors []string

	step := func(msg string, command string, args ...string) bool {
		ok := system.RunStep(msg, command, args...)
		if !ok {
			uninstallErrors = append(uninstallErrors, msg)
		}
		return ok
	}

	if envType == "native" {
		if uninstallBackup {
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile = fmt.Sprintf("/root/openresty_backup_%s.tar.gz", ts)
			bkpPaths := system.GetDefaultBackupPaths()
			existingPaths := []string{}
			for _, p := range bkpPaths {
				if _, err := os.Stat(p); err == nil {
					existingPaths = append(existingPaths, p)
				}
			}
			if len(existingPaths) > 0 {
				tarArgs := append([]string{"-czf", bkpFile}, existingPaths...)
				_ = system.RunStep("Creating configuration & web backup to "+bkpFile, "tar", tarArgs...)
				if _, err := os.Stat(bkpFile); err == nil {
					_ = os.Chmod(bkpFile, 0600)
				}
			}
		}

		tui.StartSpinner("Stopping & disabling OpenResty service")
		_ = system.StopService("openresty")
		if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
			_ = exec.Command("rc-update", "del", "openresty", "default").Run()
		} else {
			_ = exec.Command("systemctl", "disable", "openresty").Run()
			_ = exec.Command("update-rc.d", "openresty", "disable").Run()
		}
		tui.StopSpinner(0)

		if uninstallPurgeConf {
			switch core.SysInfo.Family {
			case "debian":
				out, _ := exec.Command("dpkg-query", "-W", "-f=${Package}\n", "openresty*").Output()
				var orPkgs []string
				for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
					p := strings.TrimSpace(line)
					if p != "" && strings.HasPrefix(p, "openresty") {
						orPkgs = append(orPkgs, p)
					}
				}
				if len(orPkgs) > 0 {
					purgeArgs := append([]string{"purge", "-y"}, orPkgs...)
					step("Purging OpenResty packages", "apt-get", purgeArgs...)
				}
				step("Cleaning up orphaned dependency packages", "apt-get", "autoremove", "-y")
				_ = os.Remove("/etc/apt/sources.list.d/openresty.list")
				_ = os.Remove("/etc/apt/trusted.gpg.d/openresty.gpg")
				_ = os.Remove("/usr/share/keyrings/openresty.gpg")

			case "rhel":
				out, _ := exec.Command("rpm", "-qa", "--qf", "%{NAME}\n", "openresty*").Output()
				var orPkgs []string
				for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
					p := strings.TrimSpace(line)
					if p != "" && strings.HasPrefix(p, "openresty") {
						orPkgs = append(orPkgs, p)
					}
				}
				if len(orPkgs) > 0 {
					rmArgs := append([]string{"remove", "-y"}, orPkgs...)
					if !system.RunRHELStep("Removing OpenResty packages", true, rmArgs...) {
						uninstallErrors = append(uninstallErrors, "Removing OpenResty packages")
					}
				}
				_ = os.Remove("/etc/yum.repos.d/openresty.repo")
				_ = os.Remove("/etc/php-fpm.d/zz-openresty.conf")

			case "alpine":
				step("Removing OpenResty packages", "apk", "del", "openresty", "openresty-openrc")
				// Only remove /etc/nginx on Alpine where OpenResty uses /etc/nginx exclusively
				_ = os.RemoveAll("/etc/nginx")
			}

			_ = os.RemoveAll("/usr/local/openresty")
			_ = os.RemoveAll("/etc/openresty")
			_ = os.RemoveAll("/run/openresty")
			_ = os.RemoveAll("/var/log/openresty")
		}

		if uninstallPurgeFiles {
			if webDir := strings.TrimSpace(state["WEB_DIR"]); webDir != "" && filepath.Clean(webDir) != "/var/www/html" {
				cleanWebDir := filepath.Clean(webDir)
				if system.ValidateSafeDirectoryPath(cleanWebDir) == nil && !system.IsSymlinkPath(cleanWebDir) {
					if _, err := os.Lstat(cleanWebDir); err == nil {
						step("Removing configured site root ("+cleanWebDir+")", "rm", "-rf", cleanWebDir)
					}
				}
			}
			if !system.IsSymlinkPath("/var/www/html") {
				if _, err := os.Lstat("/var/www/html"); err == nil {
					step("Removing default web root (/var/www/html)", "rm", "-rf", "/var/www/html")
				}
			}
			_ = os.Remove("/var/www")
		}
	} else {
		composeDir := strings.TrimSpace(state["COMPOSE_DIR"])
		if composeDir == "" || !isSafeComposeDirForPurge(composeDir) {
			composeDir = "/etc/openresty-docker"
		} else {
			composeDir = filepath.Clean(composeDir)
		}

		if uninstallBackup {
			if _, err := os.Lstat(composeDir); err == nil && !system.IsSymlinkPath(composeDir) {
				ts := time.Now().Format("2006-01-02_15-04-05")
				bkpFile = fmt.Sprintf("/root/openresty_docker_backup_%s.tar.gz", ts)
				_ = system.RunStep("Creating Docker stack backup to "+bkpFile, "tar", "-czf", bkpFile, composeDir)
				if _, err := os.Stat(bkpFile); err == nil {
					_ = os.Chmod(bkpFile, 0600)
				}
			}
		}

		if uninstallPurgeFiles {
			if !system.RunDockerCompose(composeDir, "Stopping and purging Docker container stack", false, "down", "-v") {
				_ = system.RunStep("Removing Docker container", "docker", "rm", "-f", "krotek-openresty")
			}
			if isSafeComposeDirForPurge(composeDir) {
				_ = os.RemoveAll(composeDir)
			}
		} else {
			if !system.RunDockerCompose(composeDir, "Stopping Docker container stack", false, "down") {
				_ = system.RunStep("Stopping Docker container", "docker", "stop", "krotek-openresty")
			}
		}
	}

	_ = os.Remove(core.StateFile)
	_ = os.Remove("/usr/local/bin/openresty-manager")
	_ = os.Remove("/usr/local/bin/openresty-installer")
	removeIfSymlinkToOpenResty("/usr/local/bin/openresty")
	removeIfSymlinkToOpenResty("/usr/local/bin/resty")
	removeIfSymlinkToOpenResty("/usr/local/bin/opm")

	var recentLogErrors []string
	if core.LogFilePath != "" {
		if data, err := os.ReadFile(core.LogFilePath); err == nil {
			lines := strings.Split(string(data), "\n")
			for i := len(lines) - 1; i >= 0; i-- {
				l := strings.TrimSpace(lines[i])
				if l == "" {
					continue
				}
				cleanL := core.SanitizeTerminalText(l)
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
		finalSummary = append(finalSummary, tui.FormatKV("Files & Storage", "[PURGED / REMOVED]", core.RED))
	} else {
		finalSummary = append(finalSummary, tui.FormatKV("Files & Storage", "[RETAINED]", core.GREEN))
	}
	if envType == "native" {
		if uninstallPurgeConf {
			finalSummary = append(finalSummary, tui.FormatKV("Packages & Configs", "[PURGED / REMOVED]", core.RED))
		} else {
			finalSummary = append(finalSummary, tui.FormatKV("Packages & Configs", "[RETAINED]", core.GREEN))
		}
	}

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
	tui.PrintInBox("OpenResty Server Stack Uninstalled", finalSummary)

	fmt.Print("\r\n")
	logger.LogSuccess("OpenResty server stack uninstalled successfully.")
	os.Exit(0)
}

// UninstallWizard provides standardized invocation interface
func UninstallWizard() {
	UninstallOpenResty()
}
