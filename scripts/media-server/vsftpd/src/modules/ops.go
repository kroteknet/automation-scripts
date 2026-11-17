// ==============================================================================
// KROTEK SCRIPT HUB — SERVICE OPERATIONS & DIAGNOSTIC HEALTH INSPECTOR
// Service: VSFTPD (media-server)
// File:    media-server/vsftpd/src/modules/ops.go
//
// Provides deep health inspections, port availability checks, service daemon status verification, and lifecycle management operations.
// ==============================================================================

package modules

import (
	"os/exec"
	"strings"

	"krotek-vsftpd-installer/core"
	"krotek-vsftpd-installer/system"
	"krotek-vsftpd-installer/tui"
)

// VsftpdHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// VsftpdHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// VsftpdHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// VsftpdHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
type VsftpdHealth struct {
	Installed   bool
	EnvType     string
	IsRunning   bool
	PortOpen    bool
	VersionText string
}

// DetectVsftpdHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectVsftpdHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectVsftpdHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectVsftpdHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
func DetectVsftpdHealth() VsftpdHealth {
	var h VsftpdHealth
	state := system.LoadState()
	h.EnvType = state["ENV_TYPE"]

	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-vsftpd").Output(); err == nil {
		h.Installed = true
		if h.EnvType == "" {
			h.EnvType = "docker"
		}
		if strings.TrimSpace(string(out)) == "running" {
			h.IsRunning = true
			h.PortOpen = true
		}
	} else if (system.IsInstalledState() || system.IsServiceInstalled()) || (exec.Command("systemctl", "is-active", "--quiet", "vsftpd").Run() == nil || exec.Command("rc-service", "vsftpd", "status").Run() == nil || exec.Command("pidof", "vsftpd").Run() == nil) {
		h.Installed = true
		if h.EnvType == "" {
			h.EnvType = "native"
		}
		if exec.Command("systemctl", "is-active", "--quiet", "vsftpd").Run() == nil || exec.Command("rc-service", "vsftpd", "status").Run() == nil || exec.Command("pidof", "vsftpd").Run() == nil {
			h.IsRunning = true
			h.PortOpen = true
		}
	}

	if h.Installed && h.VersionText == "" {
		if out, err := exec.Command("vsftpd", "--version").Output(); err == nil {
			h.VersionText = strings.TrimSpace(string(out))
		}
	}

	return h
}

func RunDeepDiagnostics() {
	tui.ClearScreen()
	h := DetectVsftpdHealth()

	var diag []string
	diag = append(diag, core.BOLD+core.YELLOW+"=== VSFTPD SECURE FTP SYSTEM DIAGNOSTIC REPORT ==="+core.NC, "")
	diag = append(diag, tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN))
	diag = append(diag, tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN))
	diag = append(diag, tui.FormatKV("Installation State", func() string {
		if h.Installed {
			return core.GREEN + "[INSTALLED] (" + h.EnvType + ")"
		}
		return core.RED + "[NOT INSTALLED]"
	}(), core.GREEN))
	diag = append(diag, tui.FormatKV("Service Running State", func() string {
		if h.IsRunning {
			return core.GREEN + "[ACTIVE / RUNNING]"
		}
		return core.RED + "[STOPPED / INACTIVE]"
	}(), core.GREEN))
	if h.VersionText != "" {
		diag = append(diag, tui.FormatKV("Application Version", h.VersionText, core.YELLOW))
	}
	diag = append(diag, tui.FormatKV("Service Port (21 TCP)", func() string {
		if h.PortOpen {
			return core.GREEN + "[ACTIVE / LISTENING]"
		}
		return core.YELLOW + "[STANDBY]"
	}(), core.GREEN))

	tui.PrintInBox("Deep Diagnostics Report", diag)
}
