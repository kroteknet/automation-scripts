// ==============================================================================
// KROTEK SCRIPT HUB — DIAGNOSTIC LOGGING SUBSYSTEM
// Service: PLEX (media-server)
// File:    media-server/plex/src/logger/logger.go
//
// Manages timestamped disk logging to /var/log/ for all installation, configuration, and maintenance actions, as well as formatted terminal output.
// ==============================================================================

package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"krotek-plex-installer/core"
)

// InitLogger initializes the session log file in /var/log/<service>/ with restrictive 0600 permissions.
func InitLogger() {
	logDir := "/var/log/plex"
	_ = os.MkdirAll(logDir, 0755)

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	core.LogFilePath = filepath.Join(logDir, fmt.Sprintf("install_%s.log", timestamp))

	if fi, lErr := os.Lstat(core.LogFilePath); lErr != nil || (fi.Mode()&os.ModeSymlink == 0 && fi.Mode().IsRegular()) {
		if root, rErr := os.OpenRoot(logDir); rErr == nil {
			if f, err := root.OpenFile(filepath.Base(core.LogFilePath), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600); err == nil {
				core.LogFile = f
			}
			_ = root.Close()
		}
	}
}

// InitUninstallLogger initializes uninstallation log file in /var/log/ for diagnostic tracking.
func InitUninstallLogger(srv string) {
	logDir := fmt.Sprintf("/var/log/%s", srv)
	if !strings.HasSuffix(srv, "-manager") {
		logDir = fmt.Sprintf("/var/log/%s-manager", srv)
	}
	_ = os.MkdirAll(logDir, 0755)
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	newLogPath := filepath.Join(logDir, fmt.Sprintf("uninstall_%s.log", timestamp))
	if core.LogFile != nil {
		_ = core.LogFile.Close()
	}
	if fi, lErr := os.Lstat(newLogPath); lErr != nil || (fi.Mode()&os.ModeSymlink == 0 && fi.Mode().IsRegular()) {
		if root, rErr := os.OpenRoot(logDir); rErr == nil {
			if f, err := root.OpenFile(filepath.Base(newLogPath), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600); err == nil {
				core.LogFile = f
				core.LogFilePath = newLogPath
			}
			_ = root.Close()
		}
	}
}

// LogMessage writes a timestamped record to the active log file and prints to stdout with ANSI styling.
func LogMessage(prefix, msg string) {
	formatted := fmt.Sprintf("%s %s\r\n", prefix, msg)
	if !core.BoxSpinnerActive.Load() && !core.SpinnerActive.Load() {
		fmt.Print(formatted)
	}
	if core.LogFile != nil {
		cleanMsg := core.StripANSI(formatted)
		_, _ = core.LogFile.WriteString(fmt.Sprintf("[%s] %s", time.Now().Format(time.RFC3339), cleanMsg))
	}
}

func LogInfo(msg string)    { LogMessage(core.BLUE+"[INFO]"+core.NC, msg) }
func LogWarn(msg string)    { LogMessage(core.YELLOW+"[WARN]"+core.NC, msg) }
func LogError(msg string)   { LogMessage(core.RED+"[ERROR]"+core.NC, msg) }
func LogSuccess(msg string) { LogMessage(core.GREEN+"[OK]"+core.NC, msg) }
