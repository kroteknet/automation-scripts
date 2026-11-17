// ==============================================================================
// KROTEK SCRIPT HUB — SERVICE HEALTH & PROCESS MONITORING DAEMON
// Service: CADDY (web-server)
// File:    web-server/caddy/src/modules/health.go
//
// Performs live background health audits, socket binding checks, and daemon status queries to ensure continuous service availability.
// ==============================================================================

package modules

import (
	"os/exec"
	"strings"

	"krotek-caddy-installer/core"
	"krotek-caddy-installer/system"
	"krotek-caddy-installer/tui"
)

// CaddyHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// CaddyHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// CaddyHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// CaddyHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
type CaddyHealth struct {
	Installed   bool
	IsRunning   bool
	EnvType     string
	SyntaxOK    bool
	SyntaxError string
	Port80      bool
	Port8080    bool
	Port443     bool
	Port2019    bool
}

// DetectCaddyHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectCaddyHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectCaddyHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectCaddyHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
func DetectCaddyHealth() CaddyHealth {
	h := CaddyHealth{}

	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-caddy").Output(); err == nil {
		if strings.TrimSpace(string(out)) == "running" {
			h.Installed = true
			h.IsRunning = true
			h.EnvType = "docker"
			h.SyntaxOK = true
		}
	}

	if !h.IsRunning {
		if (exec.Command("systemctl", "is-active", "--quiet", "caddy").Run() == nil || exec.Command("rc-service", "caddy", "status").Run() == nil || exec.Command("pidof", "caddy").Run() == nil) ||
			exec.Command("rc-service", "caddy", "status").Run() == nil ||
			exec.Command("pidof", "caddy").Run() == nil {
			h.Installed = true
			h.IsRunning = true
			h.EnvType = "native"
		}
	}

	if !h.Installed {
		if (system.IsInstalledState() || system.IsServiceInstalled()) ||
			system.CommandExists("caddy") {
			h.Installed = true
			if h.EnvType == "" {
				h.EnvType = "native"
			}
		}
	}

	if h.EnvType != "docker" && system.CommandExists("caddy") {
		if exec.Command("caddy", "validate", "--config", "/etc/caddy/Caddyfile").Run() == nil {
			h.SyntaxOK = true
		}
	}

	var netOut []byte
	if system.CommandExists("ss") {
		netOut, _ = exec.Command("ss", "-tuln").Output()
	} else if system.CommandExists("netstat") {
		netOut, _ = exec.Command("netstat", "-tuln").Output()
	}
	s := string(netOut)
	if strings.Contains(s, ":80 ") || strings.Contains(s, ":80\t") || strings.Contains(s, ":::80 ") {
		h.Port80 = true
	}
	if strings.Contains(s, ":8080 ") || strings.Contains(s, ":8080\t") || strings.Contains(s, ":::8080 ") {
		h.Port8080 = true
	}
	if strings.Contains(s, ":443 ") || strings.Contains(s, ":443\t") || strings.Contains(s, ":::443 ") {
		h.Port443 = true
	}
	if strings.Contains(s, ":2019 ") || strings.Contains(s, ":2019\t") || strings.Contains(s, ":::2019 ") {
		h.Port2019 = true
	}

	return h
}

func RunDeepDiagnostics() {
	tui.ClearScreen()
	health := DetectCaddyHealth()

	diagReport := []string{
		core.BOLD + core.YELLOW + "DEEP SYSTEM DIAGNOSTICS REPORT" + core.NC,
		"",
		tui.FormatKV("Host System", system.GetOSPrettyVersion(), core.CYAN),
		tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
		tui.FormatKV("Caddy Installation", func() string {
			if health.Installed {
				return core.GREEN + "[INSTALLED] (" + health.EnvType + ")"
			}
			return core.RED + "[NOT INSTALLED]"
		}(), core.GREEN),
		tui.FormatKV("Service Running State", func() string {
			if health.IsRunning {
				return core.GREEN + "[ACTIVE / RUNNING]"
			}
			return core.RED + "[STOPPED / INACTIVE]"
		}(), core.GREEN),
		tui.FormatKV("Caddyfile Syntax Check", func() string {
			if !health.Installed {
				return core.DIM + "[N/A - NOT INSTALLED]" + core.NC
			}
			if health.SyntaxOK {
				return core.GREEN + "[SYNTAX OK]"
			}
			return core.RED + "[SYNTAX ERROR]"
		}(), core.GREEN),
		"",
		core.BOLD + core.YELLOW + "NETWORK PORTS LISTENING:" + core.NC,
		tui.FormatKV("Port 80 (HTTP)", func() string {
			if health.Port80 {
				return core.GREEN + "[ACTIVE / OPEN]"
			}
			return core.YELLOW + "[CLOSED / NOT IN USE]"
		}(), core.GREEN),
		tui.FormatKV("Port 8080 (LAN)", func() string {
			if health.Port8080 {
				return core.GREEN + "[ACTIVE / OPEN]"
			}
			return core.YELLOW + "[CLOSED / NOT IN USE]"
		}(), core.GREEN),
		tui.FormatKV("Port 443 (HTTPS)", func() string {
			if health.Port443 {
				return core.GREEN + "[ACTIVE / OPEN]"
			}
			return core.YELLOW + "[CLOSED / NOT IN USE]"
		}(), core.GREEN),
		tui.FormatKV("Port 2019 (Admin API)", func() string {
			if health.Port2019 {
				return core.GREEN + "[ACTIVE / OPEN]"
			}
			return core.YELLOW + "[CLOSED / NOT IN USE]"
		}(), core.GREEN),
	}

	tui.PrintInBox("Deep System Diagnostics Scan", diagReport)
}
