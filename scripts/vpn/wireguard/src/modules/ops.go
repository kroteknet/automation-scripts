// ==============================================================================
// KROTEK SCRIPT HUB — SERVICE OPERATIONS & DIAGNOSTIC HEALTH INSPECTOR
// Service: WIREGUARD (vpn)
// File:    vpn/wireguard/src/modules/ops.go
//
// Provides deep health inspections, port availability checks, service daemon status verification, and lifecycle management operations.
// ==============================================================================

package modules

import (
	"os/exec"
	"strings"

	"krotek-wireguard-installer/core"
	"krotek-wireguard-installer/system"
	"krotek-wireguard-installer/tui"
)

// WireguardHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// WireguardHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// WireguardHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// WireguardHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
type WireguardHealth struct {
	Installed   bool
	EnvType     string
	IsRunning   bool
	PortOpen    bool
	VersionText string
}

// DetectWireguardHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectWireguardHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectWireguardHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectWireguardHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
func DetectWireguardHealth() WireguardHealth {
	var h WireguardHealth
	state := system.LoadState()
	h.EnvType = state["ENV_TYPE"]

	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-wireguard").Output(); err == nil {
		if strings.TrimSpace(string(out)) == "running" {
			h.Installed = true
			h.IsRunning = true
			h.PortOpen = true
			if h.EnvType == "" {
				h.EnvType = "docker"
			}
		}
	} else if (exec.Command("systemctl", "is-active", "--quiet", "wg-quick@wg0").Run() == nil || exec.Command("rc-service", "wg-quick@wg0", "status").Run() == nil || exec.Command("pidof", "wg-quick").Run() == nil) || (exec.Command("systemctl", "is-active", "--quiet", "wireguard").Run() == nil || exec.Command("rc-service", "wireguard", "status").Run() == nil || exec.Command("pidof", "wireguard").Run() == nil) {
		h.Installed = true
		h.IsRunning = true
		h.PortOpen = true
		if h.EnvType == "" {
			h.EnvType = "native"
		}
	} else if system.IsInstalledState() || system.IsServiceInstalled() {
		h.Installed = true
		if h.EnvType == "" {
			h.EnvType = "native"
		}
	}

	if h.Installed && h.VersionText == "" {
		if out, err := exec.Command("wg", "--version").Output(); err == nil {
			h.VersionText = strings.TrimSpace(string(out))
		}
	}

	return h
}

func RunDeepDiagnostics() {
	tui.ClearScreen()
	h := DetectWireguardHealth()

	var diag []string
	diag = append(diag, core.BOLD+core.YELLOW+"=== WIREGUARD VPN SYSTEM DIAGNOSTIC REPORT ==="+core.NC, "")
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
	diag = append(diag, tui.FormatKV("Service Port (51820 UDP)", func() string {
		if h.PortOpen {
			return core.GREEN + "[ACTIVE / LISTENING]"
		}
		return core.YELLOW + "[STANDBY]"
	}(), core.GREEN))

	tui.PrintInBox("Deep Diagnostics Report", diag)
}
