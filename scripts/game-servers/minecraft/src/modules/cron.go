// ==============================================================================
// KROTEK SCRIPT HUB — SCHEDULED TASKS & CRON JOB MANAGER
// Service: MINECRAFT (game-servers)
// File:    game-servers/minecraft/src/modules/cron.go
//
// Manages system crontab entries for automated database maintenance, world saves, SSL renewals, and backup routines.
// ==============================================================================

package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"krotek-minecraft-installer/core"
	"krotek-minecraft-installer/system"
	"krotek-minecraft-installer/tui"
)

const cronFilePath = "/etc/cron.d/krotek-minecraft"

type CronSettings struct {
	BackupEnabled  bool
	BackupHour     int
	RestartEnabled bool
	RestartHour    int
}

func LoadCronSettings() CronSettings {
	var s CronSettings
	data, err := os.ReadFile(cronFilePath)
	if err != nil {
		return s
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "#") || l == "" {
			continue
		}
		if strings.Contains(l, "backup --cron") {
			s.BackupEnabled = true
			s.BackupHour = 4
		}
		if strings.Contains(l, "restart --cron") {
			s.RestartEnabled = true
			s.RestartHour = 5
		}
	}
	return s
}

func SaveCronSettings(s CronSettings) error {
	if !s.BackupEnabled && !s.RestartEnabled {
		_ = os.Remove(cronFilePath)
		return nil
	}

	var lines []string
	lines = append(lines, "# Krotek Minecraft Automated Tasks")
	lines = append(lines, "# Generated automatically by minecraft-manager")
	lines = append(lines, "SHELL=/bin/bash")
	lines = append(lines, "PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin")
	lines = append(lines, "")

	if s.BackupEnabled {
		h := s.BackupHour
		if h < 0 || h > 23 {
			h = 4
		}
		lines = append(lines, fmt.Sprintf("0 %d * * * root /usr/local/bin/minecraft-manager backup --cron >/dev/null 2>&1", h))
	}

	if s.RestartEnabled {
		h := s.RestartHour
		if h < 0 || h > 23 {
			h = 5
		}
		lines = append(lines, fmt.Sprintf("0 %d * * * root /usr/local/bin/minecraft-manager restart --cron >/dev/null 2>&1", h))
	}
	lines = append(lines, "")

	return system.WriteFileNoFollow(cronFilePath, []byte(strings.Join(lines, "\n")), 0644, "")
}

func RunCronBackupWithRetention() {
	CreateWorldBackup(false)

	// Clean up backups older than configured retention days
	backupDir := core.BackupDir
	matches, err := filepath.Glob(filepath.Join(backupDir, "minecraft-backup-*.tar.gz"))
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-time.Duration(core.BackupRetentionDays) * 24 * time.Hour)
	for _, m := range matches {
		if fi, err := os.Stat(m); err == nil {
			if fi.ModTime().Before(cutoff) {
				_ = os.Remove(m)
			}
		}
	}
}

func ManageScheduledTasks() {
	for {
		s := LoadCronSettings()

		bStatus := core.RED + "[DISABLED]" + core.NC
		if s.BackupEnabled {
			bStatus = core.GREEN + "[ACTIVE" + " " + core.Sym.Bullet + " " + "04:00 AM Daily]" + core.NC
		}

		rStatus := core.RED + "[DISABLED]" + core.NC
		if s.RestartEnabled {
			rStatus = core.GREEN + "[ACTIVE" + " " + core.Sym.Bullet + " " + "05:00 AM Daily]" + core.NC
		}

		options := []string{
			"─── AUTOMATED CRON TASKS ───",
			fmt.Sprintf("Nightly World Backup (04:00 AM)      : %s", bStatus),
			fmt.Sprintf("Nightly Server Restart (05:00 AM)    : %s", rStatus),
			"─── MAINTENANCE & ACTIONS ───",
			fmt.Sprintf("Run Backup & %d-Day Retention Cleanup Now", core.BackupRetentionDays),
			"Back to Main Menu",
		}

		idx, opt, st := tui.SelectMenu("KROTEK MINECRAFT" + " " + core.Sym.EmDash + " " + "Automated Scheduled Tasks (Cron)", options)
		if st == 2 || strings.Contains(opt, "Back to Main Menu") {
			return
		}

		cleanOpt := core.StripANSI(opt)
		switch {
		case strings.Contains(cleanOpt, "Nightly World Backup"):
			s.BackupEnabled = !s.BackupEnabled
			s.BackupHour = 4
			_ = SaveCronSettings(s)
			stateMsg := fmt.Sprintf("ENABLED (Runs every night at 04:00 AM with %d-day retention)", core.BackupRetentionDays)
			if !s.BackupEnabled {
				stateMsg = "DISABLED"
			}
			tui.PrintInBox("Automated Backup", []string{
				fmt.Sprintf("%s[OK] Nightly World Backup is now: %s%s", core.GREEN, stateMsg, core.NC),
				fmt.Sprintf("Automated archives are saved to %s/.", core.BackupDir),
			})

		case strings.Contains(cleanOpt, "Nightly Server Restart"):
			s.RestartEnabled = !s.RestartEnabled
			s.RestartHour = 5
			_ = SaveCronSettings(s)
			stateMsg := "ENABLED (Runs every night at 05:00 AM to flush JVM heap)"
			if !s.RestartEnabled {
				stateMsg = "DISABLED"
			}
			tui.PrintInBox("Automated Restart", []string{
				fmt.Sprintf("%s[OK] Nightly Server Restart is now: %s%s", core.GREEN, stateMsg, core.NC),
			})

		case strings.Contains(cleanOpt, "Run Backup & 7-Day Retention Cleanup"):
			tui.StartSpinner("Executing automated backup and pruning archives older than 7 days...")
			RunCronBackupWithRetention()
			tui.StopSpinner(0)
			tui.PrintInBox("Backup & Prune Executed", []string{
				core.GREEN + "[SUCCESS] Backup created and old archives pruned!" + core.NC,
			})
		}
		_ = idx
	}
}
