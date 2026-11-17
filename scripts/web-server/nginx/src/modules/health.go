// ==============================================================================
// KROTEK SCRIPT HUB — SERVICE HEALTH & PROCESS MONITORING DAEMON
// Service: NGINX (web-server)
// File:    web-server/nginx/src/modules/health.go
//
// Performs live background health audits, socket binding checks, and daemon status queries to ensure continuous service availability.
// ==============================================================================

package modules

import (
	"os"
	"os/exec"
	"strings"

	"krotek-nginx-installer/core"
	"krotek-nginx-installer/system"
	"krotek-nginx-installer/tui"
)

// NginxHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// NginxHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// NginxHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// NginxHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
type NginxHealth struct {
	Installed   bool
	IsRunning   bool
	EnvType     string
	SyntaxOK    bool
	SyntaxError string
	Port80      bool
	Port8080    bool
	Port443     bool
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DetectNginxHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectNginxHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectNginxHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectNginxHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
func DetectNginxHealth() NginxHealth {
	var h NginxHealth

	state := system.LoadState()
	h.EnvType = state["ENV_TYPE"]

	// 1. Docker check
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-nginx").Output(); err == nil {
		if strings.TrimSpace(string(out)) == "running" {
			h.Installed = true
			h.IsRunning = true
			if h.EnvType == "" {
				h.EnvType = "docker"
			}
			if exec.Command("docker", "exec", "krotek-nginx", "nginx", "-t").Run() == nil {
				h.SyntaxOK = true
			} else {
				h.SyntaxOK = false
			}
		}
	} else if out, err := exec.Command("docker", "ps", "--filter", "name=nginx", "--format", "{{.Names}}").Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		h.Installed = true
		h.IsRunning = true
		if h.EnvType == "" {
			h.EnvType = "docker"
		}
	}

	// 2. Native check if running (systemd, OpenRC, or process)
	if !h.IsRunning {
		if (exec.Command("systemctl", "is-active", "--quiet", "nginx").Run() == nil || exec.Command("rc-service", "nginx", "status").Run() == nil || exec.Command("pidof", "nginx").Run() == nil) ||
			exec.Command("rc-service", "nginx", "status").Run() == nil ||
			exec.Command("pidof", "nginx").Run() == nil ||
			exec.Command("pgrep", "-x", "nginx").Run() == nil {
			h.Installed = true
			h.IsRunning = true
			if h.EnvType == "" {
				h.EnvType = "native"
			}
		}
	}

	// 3. Native check if installed on system even if stopped
	if !h.Installed {
		if (system.IsInstalledState() || system.IsServiceInstalled()) ||
			system.CommandExists("nginx") ||
			fileExists("/usr/sbin/nginx") ||
			fileExists("/usr/bin/nginx") ||
			fileExists("/usr/local/sbin/nginx") ||
			fileExists("/usr/local/bin/nginx") ||
			fileExists("/etc/nginx/nginx.conf") {
			h.Installed = true
			if h.EnvType == "" {
				h.EnvType = "native"
			}
		}
	}

	// 4. Test syntax for native nginx
	if h.EnvType != "docker" {
		if system.CommandExists("nginx") || fileExists("/usr/sbin/nginx") || fileExists("/usr/bin/nginx") {
			out, err := exec.Command("nginx", "-t").CombinedOutput()
			if err == nil {
				h.SyntaxOK = true
			} else {
				h.SyntaxOK = false
				h.SyntaxError = core.SanitizeTerminalText(strings.TrimSpace(string(out)))
			}
		}
	}

	// 5. Port check via ss or netstat
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

	return h
}

func RunDeepDiagnostics() {
	tui.ClearScreen()
	health := DetectNginxHealth()

	diagReport := []string{
		core.BOLD + core.YELLOW + "DEEP SYSTEM DIAGNOSTICS REPORT" + core.NC,
		"",
		tui.FormatKV("Host System", system.GetOSPrettyVersion(), core.CYAN),
		tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
		tui.FormatKV("Nginx Installation", func() string {
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
		tui.FormatKV("Config Syntax Check", func() string {
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
	}

	if !health.IsRunning && core.SysInfo.InitSystem == "systemd" {
		if out, err := exec.Command("journalctl", "-u", "nginx", "-n", "8", "--no-pager").Output(); err == nil && len(out) > 0 {
			diagReport = append(diagReport, "")
			diagReport = append(diagReport, core.BOLD+core.RED+"RECENT JOURNALCTL SYSTEM LOGS:"+core.NC)
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				trimmed := strings.TrimSpace(line)
				if trimmed != "" {
					if len(trimmed) > 85 {
						trimmed = trimmed[:85] + "..."
					}
					diagReport = append(diagReport, "  "+core.StripANSI(trimmed))
				}
			}
		}
	}

	tui.PrintInBox("Deep System Diagnostics Scan", diagReport)
}
