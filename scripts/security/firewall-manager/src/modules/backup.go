// ==============================================================================
// KROTEK SCRIPT HUB — CONFIGURATION SNAPSHOT & BACKUP UTILITY
// Service: FIREWALL-MANAGER (security)
// File:    security/firewall-manager/src/modules/backup.go
//
// Generates timestamped configuration backups and handles safe restoration of prior application states.
// ==============================================================================

package modules

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"krotek-firewall-manager-installer/core"
	"krotek-firewall-manager-installer/system"
	"krotek-firewall-manager-installer/tui"
)

func BackupRestoreMenu() {
	backupDir := "/etc/firewall-manager/backups"
	_ = os.MkdirAll(backupDir, 0700)

	for {
		tui.ClearScreen()
		options := []string{
			"─── BACKUP & RESTORE ───",
			"[1] Create Timestamped Firewall Configuration Backup",
			"[2] Restore Firewall Rules from Existing Backup",
			"─── NAVIGATION ───",
			"Back to Main Menu",
		}

		titleStr := "FIREWALL & SECURITY MANAGER - Backup Engine"
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)
		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Back to Main Menu") {
			return
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "[1] Create Timestamped"):
			tui.ClearScreen()
			ts := time.Now().Format("2006-01-02_15-04-05")
			bkpFile := filepath.Join(backupDir, fmt.Sprintf("rules_%s.tar.gz", ts))
			var res []string
			var tarSources []string
			if _, err := os.Stat("/etc/ufw"); err == nil {
				tarSources = append(tarSources, "/etc/ufw")
			}
			if _, err := os.Stat("/etc/firewall-manager"); err == nil {
				tarSources = append(tarSources, "/etc/firewall-manager")
			}

			tarArgs := append([]string{"-czf", bkpFile}, tarSources...)
			if len(tarSources) > 0 && exec.Command("tar", tarArgs...).Run() == nil {
				res = append(res, tui.FormatKV("Backup File", bkpFile, core.GREEN+core.BOLD))
				res = append(res, tui.FormatKV("Status", "SUCCESSFULLY CREATED", core.GREEN))
			} else {
				// Fallback to iptables-save
				iptOut, err := exec.Command("iptables-save").CombinedOutput()
				if err == nil && len(iptOut) > 0 {
					_ = system.WriteFileNoFollow(bkpFile+".iptables", iptOut, 0600, "")
					res = append(res, tui.FormatKV("Backup File", bkpFile+".iptables", core.GREEN+core.BOLD))
					res = append(res, tui.FormatKV("Status", "SUCCESSFULLY CREATED (iptables-save)", core.GREEN))
				} else {
					res = append(res, tui.FormatKV("Status", "FAILED TO CREATE BACKUP", core.RED))
				}
			}
			tui.PrintInBox("Backup Creation Result", res)

		case strings.HasPrefix(cleanOpt, "[2] Restore Firewall"):
			tui.ClearScreen()
			files, _ := os.ReadDir(backupDir)
			var bkpFiles []string
			for _, f := range files {
				if !f.IsDir() {
					bkpFiles = append(bkpFiles, f.Name())
				}
			}
			if len(bkpFiles) == 0 {
				tui.PrintInBox("Restore Result", tui.FormatInfoNotice("INFO", "No backup archives found in "+backupDir+".", core.YELLOW))
				continue
			}
			bkpFiles = append(bkpFiles, "Cancel")
			bIdx, selectedBkp, bStat := tui.SelectMenu("Select Backup to Restore", bkpFiles)
			if bStat != 0 || selectedBkp == "Cancel" {
				continue
			}
			targetPath := filepath.Join(backupDir, bkpFiles[bIdx])
			cleanTarget := filepath.Clean(targetPath)
			if !strings.HasPrefix(cleanTarget, filepath.Clean(backupDir)+string(filepath.Separator)) {
				tui.PrintInBox("Security Error", []string{core.RED + "Invalid backup file path outside backup directory." + core.NC})
				continue
			}

			tui.ClearScreen()
			conf := tui.AskYesNo("Are you sure you want to restore " + bkpFiles[bIdx] + "?")
			if conf == 0 {
				tui.ClearScreen()
				// Validate archive contents if tar.gz
				if strings.HasSuffix(cleanTarget, ".tar.gz") {
					listOut, err := exec.Command("tar", "-tzf", cleanTarget).CombinedOutput()
					if err != nil {
						tui.PrintInBox("Restore Error", []string{core.RED + "Corrupted or invalid tar archive: " + err.Error() + core.NC})
						continue
					}
					isSafe := true
					for _, line := range strings.Split(string(listOut), "\n") {
						entry := strings.TrimSpace(line)
						if entry == "" {
							continue
						}
						cleanEntry := filepath.Clean(entry)
						if strings.HasPrefix(cleanEntry, "..") || strings.Contains(cleanEntry, "/../") || strings.HasPrefix(cleanEntry, "/") {
							isSafe = false
							break
						}
						if !strings.HasPrefix(cleanEntry, "etc/ufw") && !strings.HasPrefix(cleanEntry, "etc/firewall-manager") && !strings.HasPrefix(cleanEntry, "etc/iptables") {
							isSafe = false
							break
						}
					}
					if !isSafe {
						tui.PrintInBox("Security Error", []string{core.RED + "Malicious archive detected: entries target paths outside allowed firewall directories." + core.NC})
						continue
					}
					if valErr := system.ValidateTarGzArchive(cleanTarget, 256*1024*1024); valErr != nil {
						tui.PrintInBox("Security Error", []string{core.RED + "Unsafe archive rejected: " + valErr.Error() + core.NC})
						continue
					}

					var rRes []string
					if exec.Command("tar", "-xzf", cleanTarget, "-C", "/").Run() == nil {
						if system.CommandExists("ufw") {
							_ = exec.Command("ufw", "reload").Run()
						}
						if system.CommandExists("firewall-cmd") {
							_ = exec.Command("firewall-cmd", "--reload").Run()
						}
						rRes = append(rRes, tui.FormatKV("Restored File", cleanTarget, core.GREEN+core.BOLD))
						rRes = append(rRes, tui.FormatKV("Status", "SUCCESS (Rules Restored & Reloaded)", core.GREEN))
					} else {
						rRes = append(rRes, tui.FormatKV("Status", "FAILED TO RESTORE", core.RED))
					}
					tui.PrintInBox("Restore Result", rRes)
				}
			}
		}
	}
}
