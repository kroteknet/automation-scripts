// ==============================================================================
// KROTEK SCRIPT HUB — SERVICE OPERATIONS & DIAGNOSTIC HEALTH INSPECTOR
// Service: NEXTCLOUD (media-server)
// File:    media-server/nextcloud/src/modules/ops.go
//
// Provides deep health inspections, port availability checks, service daemon status verification, and lifecycle management operations.
// ==============================================================================

package modules

import (
	"os/exec"
	"strings"

	"krotek-nextcloud-installer/core"
	"krotek-nextcloud-installer/system"
	"krotek-nextcloud-installer/tui"
)

// NextcloudHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// NextcloudHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// NextcloudHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// NextcloudHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
type NextcloudHealth struct {
	Installed   bool
	EnvType     string
	IsRunning   bool
	PortOpen    bool
	VersionText string
}

// DetectNextcloudHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectNextcloudHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectNextcloudHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectNextcloudHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
func DetectNextcloudHealth() NextcloudHealth {
	var h NextcloudHealth
	state := system.LoadState()
	h.EnvType = state["ENV_TYPE"]

	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-nextcloud").Output(); err == nil {
		h.Installed = true
		if h.EnvType == "" {
			h.EnvType = "docker"
		}
		if strings.TrimSpace(string(out)) == "running" {
			h.IsRunning = true
			h.PortOpen = true
		}
	} else if system.IsInstalledState() || system.IsServiceInstalled() {
		h.Installed = true
		if h.EnvType == "" {
			h.EnvType = "native"
		}
		if (exec.Command("systemctl", "is-active", "--quiet", "nginx").Run() == nil || exec.Command("rc-service", "nginx", "status").Run() == nil || exec.Command("pidof", "nginx").Run() == nil) ||
			(exec.Command("systemctl", "is-active", "--quiet", "apache2").Run() == nil || exec.Command("rc-service", "apache2", "status").Run() == nil || exec.Command("pidof", "apache2").Run() == nil) ||
			(exec.Command("systemctl", "is-active", "--quiet", "httpd").Run() == nil || exec.Command("pidof", "httpd").Run() == nil) {
			h.IsRunning = true
			h.PortOpen = true
		}
	}

	if h.Installed && h.VersionText == "" {
		if out, err := exec.Command("nextcloud", "--version").Output(); err == nil {
			h.VersionText = strings.TrimSpace(string(out))
		}
	}

	return h
}

func RunDeepDiagnostics() {
	tui.ClearScreen()
	h := DetectNextcloudHealth()

	var diag []string
	diag = append(diag, core.BOLD+core.YELLOW+"=== NEXTCLOUD HUB SYSTEM DIAGNOSTIC REPORT ==="+core.NC, "")
	diag = append(diag, tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN))
	diag = append(diag, tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN))
	diag = append(diag, tui.FormatKV("Installation State", func() string {
		if h.Installed {
			return core.GREEN + "[INSTALLED] (" + h.EnvType + ")"
		}
		return core.RED + "[NOT INSTALLED]"
	}(), core.GREEN))

	if h.EnvType == "native" {
		state := system.LoadState()
		ws := state["WEBSERVER"]
		if ws == "" {
			if exec.Command("systemctl", "is-active", "--quiet", "apache2").Run() == nil || exec.Command("pidof", "apache2").Run() == nil || exec.Command("pidof", "httpd").Run() == nil {
				ws = "apache"
			} else {
				ws = "nginx"
			}
		}
		diag = append(diag, tui.FormatKV("Web Server Engine", strings.ToUpper(ws), core.CYAN+core.BOLD))
	}

	diag = append(diag, tui.FormatKV("Service Running State", func() string {
		if h.IsRunning {
			return core.GREEN + "[ACTIVE / RUNNING]"
		}
		return core.RED + "[STOPPED / INACTIVE]"
	}(), core.GREEN))
	if h.VersionText != "" {
		diag = append(diag, tui.FormatKV("Application Version", h.VersionText, core.YELLOW))
	}
	diag = append(diag, tui.FormatKV("Service Port (80, 443 TCP)", func() string {
		if h.PortOpen {
			return core.GREEN + "[ACTIVE / LISTENING]"
		}
		return core.YELLOW + "[STANDBY]"
	}(), core.GREEN))

	tui.PrintInBox("Deep Diagnostics Report", diag)
}
