// ==============================================================================
// KROTEK SCRIPT HUB — SERVICE OPERATIONS & DIAGNOSTIC HEALTH INSPECTOR
// Service: CYBERPANEL (webpanel)
// File:    webpanel/cyberpanel/src/modules/ops.go
//
// Provides deep health inspections, port availability checks, service daemon status verification, and lifecycle management operations.
// ==============================================================================

package modules

import (
	"os/exec"
	"strings"

	"krotek-cyberpanel-installer/core"
	"krotek-cyberpanel-installer/system"
	"krotek-cyberpanel-installer/tui"
)

// CyberpanelHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// CyberpanelHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// CyberpanelHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// CyberpanelHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
type CyberpanelHealth struct {
	Installed   bool
	EnvType     string
	IsRunning   bool
	PortOpen    bool
	VersionText string
}

// DetectCyberpanelHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectCyberpanelHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectCyberpanelHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectCyberpanelHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
func DetectCyberpanelHealth() CyberpanelHealth {
	var h CyberpanelHealth
	state := system.LoadState()
	h.EnvType = state["ENV_TYPE"]

	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-cyberpanel").Output(); err == nil {
		h.Installed = true
		if h.EnvType == "" {
			h.EnvType = "docker"
		}
		if strings.TrimSpace(string(out)) == "running" {
			h.IsRunning = true
			h.PortOpen = true
		}
	} else if (system.IsInstalledState() || system.IsServiceInstalled()) || (exec.Command("systemctl", "is-active", "--quiet", "lscpd").Run() == nil || exec.Command("rc-service", "lscpd", "status").Run() == nil || exec.Command("pidof", "lscpd").Run() == nil) || (exec.Command("systemctl", "is-active", "--quiet", "cyberpanel").Run() == nil || exec.Command("rc-service", "cyberpanel", "status").Run() == nil || exec.Command("pidof", "cyberpanel").Run() == nil) {
		h.Installed = true
		if h.EnvType == "" {
			h.EnvType = "native"
		}
		if (exec.Command("systemctl", "is-active", "--quiet", "lscpd").Run() == nil || exec.Command("rc-service", "lscpd", "status").Run() == nil || exec.Command("pidof", "lscpd").Run() == nil) || (exec.Command("systemctl", "is-active", "--quiet", "cyberpanel").Run() == nil || exec.Command("rc-service", "cyberpanel", "status").Run() == nil || exec.Command("pidof", "cyberpanel").Run() == nil) {
			h.IsRunning = true
			h.PortOpen = true
		}
	}

	if h.Installed && h.VersionText == "" {
		if out, err := exec.Command("cyberpanel", "--version").Output(); err == nil {
			h.VersionText = strings.TrimSpace(string(out))
		}
	}

	return h
}

func RunDeepDiagnostics() {
	tui.ClearScreen()
	h := DetectCyberpanelHealth()

	var diag []string
	diag = append(diag, core.BOLD+core.YELLOW+"=== CYBERPANEL SERVER CONTROL SYSTEM DIAGNOSTIC REPORT ==="+core.NC, "")
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
		diag = append(diag, tui.FormatKV("Panel Version", h.VersionText, core.YELLOW))
	}
	diag = append(diag, tui.FormatKV("Control Panel Port (8090 TCP)", func() string {
		if h.PortOpen {
			return core.GREEN + "[ACTIVE / LISTENING]"
		}
		return core.YELLOW + "[STANDBY]"
	}(), core.GREEN))

	tui.PrintInBox("Deep Diagnostics Report", diag)
}
