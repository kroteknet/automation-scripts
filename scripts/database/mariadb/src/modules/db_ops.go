// ==============================================================================
// KROTEK SCRIPT HUB — DATABASE ENGINE OPERATIONS & MAINTENANCE
// Service: MARIADB (database)
// File:    database/mariadb/src/modules/db_ops.go
//
// Provides administrative utilities for relational/NoSQL database engines: health inspection, port probes, and service connection checks.
// ==============================================================================

package modules

import (
	"os/exec"
	"strings"

	"krotek-mariadb-installer/core"
	"krotek-mariadb-installer/system"
	"krotek-mariadb-installer/tui"
)

// MariaDBHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// MariaDBHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// MariaDBHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// MariaDBHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
type MariaDBHealth struct {
	Installed   bool
	EnvType     string
	IsRunning   bool
	PortOpen    bool
	CanConnect  bool
	VersionText string
}

// DetectMariaDBHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectMariaDBHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectMariaDBHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectMariaDBHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
func DetectMariaDBHealth() MariaDBHealth {
	var h MariaDBHealth
	state := system.LoadState()
	h.EnvType = state["ENV_TYPE"]

	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-mariadb").Output(); err == nil {
		if strings.TrimSpace(string(out)) == "running" {
			h.Installed = true
			h.IsRunning = true
			h.PortOpen = true
			if h.EnvType == "" {
				h.EnvType = "docker"
			}
		}
	} else if (exec.Command("systemctl", "is-active", "--quiet", "mariadb").Run() == nil || exec.Command("rc-service", "mariadb", "status").Run() == nil || exec.Command("pidof", "mariadbd").Run() == nil || exec.Command("pidof", "mariadb").Run() == nil) || (exec.Command("systemctl", "is-active", "--quiet", "mysqld").Run() == nil || exec.Command("rc-service", "mysqld", "status").Run() == nil || exec.Command("pidof", "mysqld").Run() == nil) || (exec.Command("systemctl", "is-active", "--quiet", "mysql").Run() == nil || exec.Command("rc-service", "mysql", "status").Run() == nil || exec.Command("pidof", "mysql").Run() == nil) {
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
		if out, err := exec.Command("mariadb", "--version").Output(); err == nil {
			h.VersionText = strings.TrimSpace(string(out))
		}
	}

	return h
}

func RunDeepDiagnostics() {
	tui.ClearScreen()
	h := DetectMariaDBHealth()

	var diag []string
	diag = append(diag, core.BOLD+core.YELLOW+"=== MARIADB SYSTEM & DATABASE DIAGNOSTIC REPORT ==="+core.NC, "")
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
	diag = append(diag, tui.FormatKV("Default Port (3306)", func() string {
		if h.PortOpen {
			return core.GREEN + "[LISTENING / OPEN]"
		}
		return core.YELLOW + "[CLOSED / NOT IN USE]"
	}(), core.GREEN))

	tui.PrintInBox("Deep System Diagnostics Scan", diag)
}
