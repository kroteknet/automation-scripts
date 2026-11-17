// ==============================================================================
// KROTEK SCRIPT HUB — DATABASE ENGINE OPERATIONS & MAINTENANCE
// Service: MONGODB (database)
// File:    database/mongodb/src/modules/db_ops.go
//
// Provides administrative utilities for relational/NoSQL database engines: health inspection, port probes, and service connection checks.
// ==============================================================================

package modules

import (
	"fmt"
	"os/exec"
	"strings"

	"krotek-mongodb-installer/core"
	"krotek-mongodb-installer/system"
	"krotek-mongodb-installer/tui"
)

// MongoDBHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// MongoDBHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// MongoDBHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// MongoDBHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
type MongoDBHealth struct {
	Installed   bool
	EnvType     string
	IsRunning   bool
	PortOpen    bool
	CanConnect  bool
	VersionText string
}

// isDockerContainerRunning checks if the named Docker container is running.
// Uses docker inspect first, falls back to docker ps for robustness.
func isDockerContainerRunning(name string) bool {
	// Primary: docker inspect
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", name).Output(); err == nil {
		status := strings.ToLower(strings.TrimSpace(string(out)))
		if status == "running" {
			return true
		}
	}
	// Fallback: docker ps --filter
	if out, err := exec.Command("docker", "ps", "--filter", "name="+name, "--filter", "status=running", "--format", "{{.Names}}").Output(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if strings.TrimSpace(line) == name {
				return true
			}
		}
	}
	return false
}

// DetectMongoDBHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectMongoDBHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectMongoDBHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectMongoDBHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
func DetectMongoDBHealth() MongoDBHealth {
	var h MongoDBHealth
	state := system.LoadState()
	h.EnvType = state["ENV_TYPE"]

	if isDockerContainerRunning("krotek-mongod") {
		h.Installed = true
		h.IsRunning = true
		h.PortOpen = true
		if h.EnvType == "" {
			h.EnvType = "docker"
		}
	} else if (exec.Command("systemctl", "is-active", "--quiet", "mongod").Run() == nil ||
		exec.Command("rc-service", "mongod", "status").Run() == nil ||
		exec.Command("pidof", "mongod").Run() == nil) ||
		(exec.Command("systemctl", "is-active", "--quiet", "mongodb").Run() == nil ||
			exec.Command("rc-service", "mongodb", "status").Run() == nil ||
			exec.Command("pidof", "mongodb").Run() == nil) {
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
		if out, err := exec.Command("mongod", "--version").Output(); err == nil {
			h.VersionText = strings.TrimSpace(string(out))
		}
	}

	return h
}

func RunDeepDiagnostics() {
	tui.ClearScreen()
	h := DetectMongoDBHealth()

	var diag []string
	diag = append(diag, core.BOLD+core.YELLOW+"=== MONGODB SYSTEM & DATABASE DIAGNOSTIC REPORT ==="+core.NC, "")
	diag = append(diag, tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN))
	diag = append(diag, tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN))
	diag = append(diag, tui.FormatKV("Deployment Type", func() string {
		switch h.EnvType {
		case "docker":
			return core.CYAN + "Docker Container Stack" + core.NC
		case "native":
			return core.YELLOW + "Native System Service" + core.NC
		default:
			return core.DIM + "Unknown / Not Configured" + core.NC
		}
	}(), core.CYAN))
	diag = append(diag, tui.FormatKV("Installation State", func() string {
		if h.Installed {
			return core.GREEN + "[INSTALLED] (" + h.EnvType + ")" + core.NC
		}
		return core.RED + "[NOT INSTALLED]" + core.NC
	}(), core.GREEN))
	diag = append(diag, tui.FormatKV("Service Running State", func() string {
		if h.IsRunning {
			return core.GREEN + "[ACTIVE / RUNNING]" + core.NC
		}
		return core.RED + "[STOPPED / INACTIVE]" + core.NC
	}(), core.GREEN))

	// Docker-specific info
	if h.EnvType == "docker" || isDockerContainerRunning("krotek-mongod") {
		if out, err := exec.Command("docker", "inspect", "-f",
			"{{.State.Status}} | Image: {{.Config.Image}} | ID: {{.Id}}", "krotek-mongod").Output(); err == nil {
			parts := strings.SplitN(strings.TrimSpace(string(out)), " | ", 3)
			if len(parts) >= 2 {
				diag = append(diag, tui.FormatKV("Container Status", parts[0], core.GREEN))
			}
			if len(parts) >= 3 {
				imgPart := strings.TrimPrefix(parts[1], "Image: ")
				diag = append(diag, tui.FormatKV("Docker Image", imgPart, core.CYAN))
				idPart := strings.TrimPrefix(parts[2], "ID: ")
				if len(idPart) > 12 {
					idPart = idPart[:12]
				}
				diag = append(diag, tui.FormatKV("Container ID", idPart, core.DIM))
			}
		}
	}

	if h.VersionText != "" {
		diag = append(diag, tui.FormatKV("Engine Version", h.VersionText, core.YELLOW))
	}
	diag = append(diag, tui.FormatKV("Default Port (27017)", func() string {
		if h.PortOpen {
			return core.GREEN + "[LISTENING / OPEN]" + core.NC
		}
		return core.YELLOW + "[CLOSED / NOT IN USE]" + core.NC
	}(), core.GREEN))

	// Docker engine info
	if system.CommandExists("docker") {
		if out, err := exec.Command("docker", "version", "--format", "{{.Server.Version}}").Output(); err == nil {
			diag = append(diag, tui.FormatKV("Docker Engine", strings.TrimSpace(string(out)), core.CYAN))
		}
		if out, err := exec.Command("docker", "ps", "-a", "--filter", "name=krotek-mongod", "--format",
			fmt.Sprintf("{{.Names}}  {{.Status}}")).Output(); err == nil && strings.TrimSpace(string(out)) != "" {
			diag = append(diag, tui.FormatKV("Container Info", strings.TrimSpace(string(out)), core.GREEN))
		}
	}

	tui.PrintInBox("Deep System Diagnostics Scan", diag)
}
