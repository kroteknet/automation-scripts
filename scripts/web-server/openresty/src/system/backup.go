// ==============================================================================
// KROTEK SCRIPT HUB — PRODUCTION SAFE BACKUP & DISASTER RECOVERY
// Service: OpenResty Web Platform (web-server/openresty)
// File:    web-server/openresty/src/system/backup.go
//
// Creates verified, timestamped, root-restricted (0600) configuration and data backups
// before updates or maintenance, and provides automated one-click rollback capabilities.
// ==============================================================================

package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"krotek-openresty-installer/core"
	"krotek-openresty-installer/logger"
)

// BackupInfo encapsulates telemetry and metadata for a created backup archive.
type BackupInfo struct {
	ArchivePath string
	Timestamp   time.Time
	SizeBytes   int64
	FilesCount  int
}

// GetDefaultBackupPaths returns standard config and data paths to back up for OpenResty.
func GetDefaultBackupPaths() []string {
	candidates := []string{
		"/usr/local/openresty/nginx/conf",
		"/etc/openresty",
		"/usr/local/openresty/site",
		"/usr/local/openresty/nginx/html",
		"/var/www/html",
	}
	if core.SysInfo.Family == "alpine" {
		candidates = append(candidates, "/etc/nginx")
	}
	state := LoadState()
	if wd := strings.TrimSpace(state["WEB_DIR"]); wd != "" && ValidateSafeDirectoryPath(wd) == nil {
		candidates = append(candidates, wd)
	}

	seen := make(map[string]bool)
	var existing []string
	for _, p := range candidates {
		clean := filepath.Clean(p)
		if clean == "" || seen[clean] {
			continue
		}
		if fi, err := os.Lstat(clean); err == nil && fi.Mode()&os.ModeSymlink == 0 {
			seen[clean] = true
			existing = append(existing, clean)
		}
	}
	return existing
}

// CreateSafeBackup creates a secure, root-restricted (0600) tar.gz backup before any mutation.
func CreateSafeBackup(extraPaths ...string) (*BackupInfo, error) {
	serviceID := "openresty"
	backupDir := fmt.Sprintf("/var/backups/krotek/%s", serviceID)
	if IsSymlinkPath("/var/backups/krotek") || IsSymlinkPath(backupDir) {
		return nil, fmt.Errorf("backup directory must not be a symlink")
	}
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create backup dir: %w", err)
	}
	_ = os.Chmod("/var/backups/krotek", 0700)
	_ = os.Chmod(backupDir, 0700)

	timestamp := time.Now().Format("20060102_150405")
	archiveName := fmt.Sprintf("%s_backup_%s.tar.gz", serviceID, timestamp)
	archivePath := filepath.Join(backupDir, archiveName)

	targetPaths := GetDefaultBackupPaths()
	seen := make(map[string]bool)
	for _, p := range targetPaths {
		seen[p] = true
	}
	for _, ep := range extraPaths {
		if ep != "" {
			clean := filepath.Clean(ep)
			if !seen[clean] {
				if fi, err := os.Lstat(clean); err == nil && fi.Mode()&os.ModeSymlink == 0 {
					seen[clean] = true
					targetPaths = append(targetPaths, clean)
				}
			}
		}
	}

	if len(targetPaths) == 0 {
		return nil, fmt.Errorf("no existing configuration or data directories found to back up")
	}

	tarArgs := []string{"-czf", archivePath}
	tarArgs = append(tarArgs, targetPaths...)
	if out, err := exec.Command("tar", tarArgs...).CombinedOutput(); err != nil {
		// GNU tar exits with code 1 when files change while being archived (e.g. live logs
		// or databases). The archive is still complete and usable, so tolerate that case
		// and rely on the integrity verification below.
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 1 {
			_ = os.Remove(archivePath)
			return nil, fmt.Errorf("tar failed: %s (%w)", strings.TrimSpace(string(out)), err)
		}
	}

	if err := exec.Command("tar", "-tzf", archivePath).Run(); err != nil {
		_ = os.Remove(archivePath)
		return nil, fmt.Errorf("archive integrity verification failed")
	}

	_ = os.Chmod(archivePath, 0600)

	stat, _ := os.Stat(archivePath)
	sizeBytes := int64(0)
	if stat != nil {
		sizeBytes = stat.Size()
	}

	state := LoadState()
	state["LAST_BACKUP"] = archivePath
	state["LAST_BACKUP_TIME"] = timestamp
	SaveState(state)

	logger.LogSuccess(fmt.Sprintf("Safe backup archive created: %s (%.2f MB)", archivePath, float64(sizeBytes)/(1024*1024)))

	return &BackupInfo{
		ArchivePath: archivePath,
		Timestamp:   time.Now(),
		SizeBytes:   sizeBytes,
		FilesCount:  len(targetPaths),
	}, nil
}

// RestoreSafeBackup restores a previously created tar.gz archive.
func RestoreSafeBackup(archivePath string) error {
	cleanArchive := filepath.Clean(strings.TrimSpace(archivePath))
	if !filepath.IsAbs(cleanArchive) {
		return fmt.Errorf("backup archive path must be absolute: %s", archivePath)
	}
	fi, err := os.Lstat(cleanArchive)
	if os.IsNotExist(err) {
		return fmt.Errorf("backup archive does not exist: %s", cleanArchive)
	}
	if err != nil {
		return fmt.Errorf("cannot inspect backup archive: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular or symlinked backup archive: %s", cleanArchive)
	}

	if err := ValidateTarGzArchive(cleanArchive, 2*1024*1024*1024); err != nil {
		return fmt.Errorf("backup archive failed security check: %w", err)
	}
	if err := exec.Command("tar", "-tzf", cleanArchive).Run(); err != nil {
		return fmt.Errorf("backup archive is corrupted: %w", err)
	}

	if out, err := exec.Command("tar", "-xzf", cleanArchive, "-C", "/").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to extract backup: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	if IsSystemdRunning() {
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}
	logger.LogSuccess(fmt.Sprintf("Restored configuration from backup: %s", archivePath))
	return nil
}
