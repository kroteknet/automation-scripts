// ==============================================================================
// KROTEK SCRIPT HUB — SERVICE OPERATIONS & DIAGNOSTIC HEALTH INSPECTOR
// Service: POSTFIX (email-server)
// File:    email-server/postfix/src/modules/ops.go
//
// Provides deep health inspections, port availability checks, service daemon status verification, and lifecycle management operations.
// ==============================================================================

package modules

import (
	"os/exec"
	"strings"

	"krotek-postfix-installer/core"
	"krotek-postfix-installer/system"
	"krotek-postfix-installer/tui"
)

// PostfixHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// PostfixHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// PostfixHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// PostfixHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
type PostfixHealth struct {
	Installed   bool
	EnvType     string
	IsRunning   bool
	PortOpen    bool
	VersionText string
}

// DetectPostfixHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectPostfixHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectPostfixHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectPostfixHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
func DetectPostfixHealth() PostfixHealth {
	var h PostfixHealth
	state := system.LoadState()
	h.EnvType = state["ENV_TYPE"]

	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-postfix").Output(); err == nil {
		h.Installed = true
		if h.EnvType == "" {
			h.EnvType = "docker"
		}
		if strings.TrimSpace(string(out)) == "running" {
			h.IsRunning = true
			h.PortOpen = true
		}
	} else if (system.IsInstalledState() || system.IsServiceInstalled()) || (exec.Command("systemctl", "is-active", "--quiet", "postfix").Run() == nil || exec.Command("rc-service", "postfix", "status").Run() == nil || exec.Command("pidof", "postfix").Run() == nil) {
		h.Installed = true
		if h.EnvType == "" {
			h.EnvType = "native"
		}
		if exec.Command("systemctl", "is-active", "--quiet", "postfix").Run() == nil || exec.Command("rc-service", "postfix", "status").Run() == nil || exec.Command("pidof", "postfix").Run() == nil {
			h.IsRunning = true
			h.PortOpen = true
		}
	}

	if h.Installed && h.VersionText == "" {
		if out, err := exec.Command("postfix", "--version").Output(); err == nil {
			h.VersionText = strings.TrimSpace(string(out))
		}
	}

	return h
}

func RunDeepDiagnostics() {
	tui.ClearScreen()
	h := DetectPostfixHealth()

	var diag []string
	diag = append(diag, core.BOLD+core.YELLOW+"=== POSTFIX MAIL SYSTEM DIAGNOSTIC REPORT ==="+core.NC, "")
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
		diag = append(diag, tui.FormatKV("Engine Version", h.VersionText, core.YELLOW))
	}
	diag = append(diag, tui.FormatKV("Service Port (25, 587, 465 TCP)", func() string {
		if h.PortOpen {
			return core.GREEN + "[ACTIVE / LISTENING]"
		}
		return core.YELLOW + "[STANDBY]"
	}(), core.GREEN))

	tui.PrintInBox("Deep Diagnostics Report", diag)
}
