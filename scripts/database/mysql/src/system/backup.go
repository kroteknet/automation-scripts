// ==============================================================================
// KROTEK SCRIPT HUB — PRODUCTION SAFE BACKUP & DISASTER RECOVERY
// Service: MySQL Server (database/mysql)
// File:    database/mysql/src/system/backup.go
//
// Creates verified, timestamped, root-restricted (0600) configuration and data backups
// before updates or maintenance, and provides automated one-click rollback capabilities.
// ==============================================================================

package system

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"krotek-mysql-installer/core"
	"krotek-mysql-installer/logger"
)

// BackupInfo encapsulates telemetry and metadata for a created backup archive.
type BackupInfo struct {
	ArchivePath string
	Timestamp   time.Time
	SizeBytes   int64
	FilesCount  int
}

// GetDefaultBackupPaths returns standard config and data paths to back up for MySQL Server.
func GetDefaultBackupPaths() []string {
	paths := []string{
		"/etc/mysql",
		"/etc/my.cnf",
		"/var/lib/mysql",
	}

	var existing []string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			existing = append(existing, p)
		}
	}
	return existing
}

// CreateSafeBackup creates a secure, root-restricted (0600) tar.gz backup before any mutation.
func CreateSafeBackup(extraPaths ...string) (*BackupInfo, error) {
	serviceID := "mysql"
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

	dumpFile := filepath.Join(backupDir, fmt.Sprintf("mysql_dump_%s.sql", timestamp))
	adminPass := LoadState()["ADMIN_PASS"]
	outFile, err := func() (*os.File, error) {
		if vErr := validateTargetNoSymlink(dumpFile, true); vErr != nil {
			return nil, vErr
		}
		return openFileNoFollow(dumpFile, 0600)
	}()
	if err == nil {
		dumpBin := "mysqldump"
		if !CommandExists(dumpBin) && CommandExists("mariadb-dump") {
			dumpBin = "mariadb-dump"
		}
		func() {
			var dumpProc *exec.Cmd
			if adminPass != "" {
				if tmpCnfDir, cnfFile, cErr := BuildMySQLClientCnf("root", adminPass); cErr == nil {
					defer os.RemoveAll(tmpCnfDir)
					dumpProc = exec.Command(dumpBin, "--defaults-extra-file="+cnfFile, "--all-databases")
				}
			}
			if dumpProc == nil && adminPass == "" {
				dumpProc = exec.Command(dumpBin, "--all-databases", "-u", "root")
			}
			if dumpProc != nil {
				dumpProc.Stdout = outFile
				if dumpProc.Run() == nil {
					extraPaths = append(extraPaths, dumpFile)
				}
			}
		}()
		_ = outFile.Close()
	}

	targetPaths := GetDefaultBackupPaths()
	for _, ep := range extraPaths {
		if ep != "" {
			if fi, err := os.Lstat(filepath.Clean(ep)); err == nil && fi.Mode()&os.ModeSymlink == 0 {
				targetPaths = append(targetPaths, filepath.Clean(ep))
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
		return fmt.Errorf("failed to extract backup: %s (%w)", core.SanitizeTerminalText(strings.TrimSpace(string(out))), err)
	}

	if IsSystemdRunning() {
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}
	logger.LogSuccess(fmt.Sprintf("Restored configuration from backup: %s", archivePath))
	return nil
}

// BuildMySQLClientCnf creates a private 0700 temp directory and writes a 0600 [client] defaults-extra-file
// so passwords are never placed in CLI arguments or shell strings. Caller must remove tmpDir.
func BuildMySQLClientCnf(user, pass string) (string, string, error) {
	tmpDir, err := os.MkdirTemp("", "krotek-mycnf-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create private credential directory: %w", err)
	}
	_ = os.Chmod(tmpDir, 0700)
	cnfFile := filepath.Join(tmpDir, "client.cnf")
	sanitize := func(s string) string {
		return strings.NewReplacer("\r", "", "\n", "", "\\", "\\\\", "\"", "\\\"").Replace(s)
	}
	var content string
	if user != "" {
		content = fmt.Sprintf("[client]\nuser=\"%s\"\npassword=\"%s\"\n", sanitize(user), sanitize(pass))
	} else {
		content = fmt.Sprintf("[client]\npassword=\"%s\"\n", sanitize(pass))
	}
	if err := WriteFileNoFollow(cnfFile, []byte(content), 0600, ""); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", "", err
	}
	return tmpDir, cnfFile, nil
}

// StreamCommandGzipNoFollow runs cmd without shell redirection and gzip-compresses stdout into destPath (0600, O_NOFOLLOW).
func StreamCommandGzipNoFollow(destPath string, cmd *exec.Cmd) error {
	if cmd == nil {
		return fmt.Errorf("nil command")
	}
	if err := validateTargetNoSymlink(destPath, true); err != nil {
		return err
	}
	outFile, err := openFileNoFollow(destPath, 0600)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(destPath)
		}
	}()
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = outFile.Close()
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = outFile.Close()
		return err
	}
	gw := gzip.NewWriter(outFile)
	_, copyErr := io.Copy(gw, stdoutPipe)
	gwErr := gw.Close()
	closeErr := outFile.Close()
	waitErr := cmd.Wait()
	if copyErr != nil {
		return copyErr
	}
	if gwErr != nil {
		return gwErr
	}
	if closeErr != nil {
		return closeErr
	}
	if waitErr != nil {
		return waitErr
	}
	_ = os.Chmod(destPath, 0600)
	cleanup = false
	return nil
}
