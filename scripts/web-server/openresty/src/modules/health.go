// ==============================================================================
// KROTEK SCRIPT HUB — SERVICE HEALTH & PROCESS MONITORING DAEMON
// Service: OpenResty (web-server)
// File:    web-server/openresty/src/modules/health.go
//
// Performs live background health audits, socket binding checks, and daemon
// status queries isolated strictly to OpenResty (never mistaking host nginx).
// ==============================================================================

package modules

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"krotek-openresty-installer/core"
	"krotek-openresty-installer/system"
	"krotek-openresty-installer/tui"
)

type OpenRestyHealth struct {
	Installed   bool
	IsRunning   bool
	EnvType     string
	SyntaxOK    bool
	SyntaxError string
	ConfigPort  string
	PortActive  bool
	Port80      bool
	Port8080    bool
	Port443     bool
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isPortListening(netOutStr string, port string) bool {
	if port == "" {
		return false
	}
	return strings.Contains(netOutStr, ":"+port+" ") ||
		strings.Contains(netOutStr, ":"+port+"\t") ||
		strings.Contains(netOutStr, ":::"+port+" ") ||
		strings.Contains(netOutStr, ":::"+port+"\t")
}

// DetectOpenRestyHealth performs live health inspection by querying container engines, systemd units, OpenRC scripts, and OpenResty process tables.
func DetectOpenRestyHealth() OpenRestyHealth {
	var h OpenRestyHealth

	state := system.LoadState()
	h.EnvType = state["ENV_TYPE"]
	h.ConfigPort = system.GetConfiguredPort()

	// 1. Docker check
	if system.IsOpenRestyDockerInstalled() {
		h.Installed = true
		if h.EnvType == "" {
			h.EnvType = "docker"
		}
		if system.IsOpenRestyDockerRunning() {
			h.IsRunning = true
			if exec.Command("docker", "exec", "krotek-openresty", "openresty", "-t").Run() == nil ||
				exec.Command("docker", "exec", "krotek-openresty", "nginx", "-t").Run() == nil {
				h.SyntaxOK = true
			} else {
				h.SyntaxOK = false
			}
		}
	}

	// 2. Native check (isolated from host vanilla NGINX)
	if h.EnvType != "docker" && system.IsOpenRestyInstalled() {
		h.Installed = true
		if h.EnvType == "" {
			h.EnvType = "native"
		}
		if system.IsOpenRestyRunning() {
			h.IsRunning = true
		}
	}

	// 3. Test syntax for native OpenResty
	if h.Installed && h.EnvType != "docker" {
		if bin := system.GetOpenRestyBinPath(); bin != "" {
			out, err := exec.Command(bin, "-t").CombinedOutput()
			if err == nil {
				h.SyntaxOK = true
			} else {
				h.SyntaxOK = false
				h.SyntaxError = strings.TrimSpace(string(out))
			}
		}
	}

	// 4. Port check via ss or netstat
	var netOut []byte
	if system.CommandExists("ss") {
		netOut, _ = exec.Command("ss", "-tuln").Output()
	} else if system.CommandExists("netstat") {
		netOut, _ = exec.Command("netstat", "-tuln").Output()
	}
	s := string(netOut)
	h.Port80 = isPortListening(s, "80")
	h.Port8080 = isPortListening(s, "8080")
	h.Port443 = isPortListening(s, "443")
	h.PortActive = isPortListening(s, h.ConfigPort)

	return h
}

func RunDeepDiagnostics() {
	tui.ClearScreen()
	health := DetectOpenRestyHealth()
	binPath := system.GetOpenRestyBinPath()
	if binPath == "" {
		binPath = "Not found"
	}
	confPath := system.GetOpenRestyMainConfigPath()
	masterPID := system.GetOpenRestyMasterPID()
	if masterPID == "" {
		masterPID = "Not running"
	}

	diagReport := []string{
		core.BOLD + core.YELLOW + "DEEP SYSTEM DIAGNOSTICS REPORT" + core.NC,
		"",
		tui.FormatKV("Host System", system.GetOSPrettyVersion(), core.CYAN),
		tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
		tui.FormatKV("OpenResty Installation", func() string {
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
		tui.FormatKV("Master Process PID", masterPID, core.CYAN),
		tui.FormatKV("OpenResty Binary", binPath, core.CYAN),
		tui.FormatKV("Primary Config", confPath, core.CYAN),
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
		tui.FormatKV(fmt.Sprintf("Configured Port (%s)", health.ConfigPort), func() string {
			if health.PortActive {
				return core.GREEN + "[ACTIVE / OPEN]"
			}
			return core.YELLOW + "[CLOSED / NOT LISTENING]"
		}(), core.GREEN),
		tui.FormatKV("Port 80 (HTTP)", func() string {
			if health.Port80 {
				return core.GREEN + "[ACTIVE / OPEN]"
			}
			return core.YELLOW + "[CLOSED / NOT IN USE]"
		}(), core.GREEN),
		tui.FormatKV("Port 8080 (ALT)", func() string {
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

	if !health.IsRunning && system.IsSystemdRunning() {
		if out, err := exec.Command("journalctl", "-u", "openresty", "-n", "8", "--no-pager").Output(); err == nil && len(out) > 0 {
			diagReport = append(diagReport, "")
			diagReport = append(diagReport, core.BOLD+core.RED+"RECENT JOURNALCTL SYSTEM LOGS:"+core.NC)
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				trimmed := core.SanitizeTerminalText(strings.TrimSpace(line))
				if trimmed != "" {
					if len(trimmed) > 85 {
						trimmed = trimmed[:85] + "..."
					}
					diagReport = append(diagReport, "  "+trimmed)
				}
			}
		}
	} else if !health.SyntaxOK && health.SyntaxError != "" {
		diagReport = append(diagReport, "")
		diagReport = append(diagReport, core.BOLD+core.RED+"CONFIG SYNTAX OUTPUT:"+core.NC)
		for _, line := range strings.Split(strings.TrimSpace(health.SyntaxError), "\n") {
			if t := core.SanitizeTerminalText(strings.TrimSpace(line)); t != "" {
				diagReport = append(diagReport, "  "+core.RED+t+core.NC)
			}
		}
	}

	tui.PrintInBox("Deep System Diagnostics Scan", diagReport)
}
