// ==============================================================================
// KROTEK SCRIPT HUB — HARDWARE & OPERATING SYSTEM DETECTION ENGINE
// Service: JOOMLA (cms)
// File:    cms/joomla/src/system/detect.go
//
// Discovers the host operating system family (Debian/Ubuntu, RHEL/CentOS/Rocky/Alma, Alpine Linux), architecture (amd64, arm64), version, and active init system (systemd, OpenRC).
// ==============================================================================

package system

import (
	"os"
	"runtime"
	"strings"

	"krotek-joomla-installer/core"
)

// DetectSystem inspects the host machine to discover hardware architecture (amd64, arm64),
// Linux distribution family (Debian/Ubuntu, RHEL/CentOS, Alpine), version, and active init system (systemd, OpenRC).
// The discovered metadata is stored in core.SysInfo for package management and service unit decisions.
// IsSystemdRunning checks if systemd is booted and running as PID 1 on the host machine.
func IsSystemdRunning() bool {
	if fi, err := os.Stat("/run/systemd/system"); err == nil && fi.IsDir() {
		return true
	}
	return false
}

func DetectSystem() {
	core.SysInfo.Architecture = runtime.GOARCH

	if _, err := os.Stat("/etc/alpine-release"); err == nil {
		core.SysInfo.Distro = "alpine"
		core.SysInfo.Family = "alpine"
		core.SysInfo.InitSystem = "openrc"
		if data, err := os.ReadFile("/etc/alpine-release"); err == nil {
			core.SysInfo.Version = strings.TrimSpace(string(data))
		}
		return
	}

	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(data), "\n")
		var id, idLike, versionId, prettyName string
		for _, line := range lines {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.Trim(strings.TrimSpace(parts[1]), "\"")
				switch key {
				case "ID":
					id = strings.ToLower(val)
				case "ID_LIKE":
					idLike = strings.ToLower(val)
				case "VERSION_ID":
					versionId = val
				case "PRETTY_NAME":
					prettyName = val
				}
			}
		}
		core.SysInfo.Distro = id
		if prettyName != "" {
			core.SysInfo.Version = prettyName
		} else {
			core.SysInfo.Version = versionId
		}

		if id == "alpine" || strings.Contains(idLike, "alpine") {
			core.SysInfo.Family = "alpine"
			core.SysInfo.InitSystem = "openrc"
		} else if id == "ubuntu" || id == "debian" || strings.Contains(idLike, "ubuntu") || strings.Contains(idLike, "debian") {
			core.SysInfo.Family = "debian"
			core.SysInfo.InitSystem = "systemd"
		} else if id == "centos" || id == "rhel" || id == "almalinux" || id == "rocky" || id == "fedora" || strings.Contains(idLike, "rhel") || strings.Contains(idLike, "centos") {
			core.SysInfo.Family = "rhel"
			core.SysInfo.InitSystem = "systemd"
		} else {
			core.SysInfo.Family = "debian"
			core.SysInfo.InitSystem = "systemd"
		}
		return
	}

	core.SysInfo.Distro = "Linux"
	core.SysInfo.Family = "debian"
	if IsSystemdRunning() {
		core.SysInfo.InitSystem = "systemd"
	} else {
		core.SysInfo.InitSystem = "sysvinit"
	}
}

// GetOSPrettyVersion returns a clean, human-readable operating system string (e.g. "Ubuntu 24.04 LTS").
func GetOSPrettyVersion() string {
	osStr := ""
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, l := range lines {
			if strings.HasPrefix(l, "PRETTY_NAME=") {
				osStr = strings.Trim(strings.TrimPrefix(l, "PRETTY_NAME="), "\"")
				break
			}
		}
	}
	if osStr == "" {
		osStr = core.SysInfo.Distro + " " + core.SysInfo.Version
	}
	osStr = strings.ReplaceAll(osStr, "GNU/Linux", "")
	return strings.TrimSpace(osStr)
}

// FormatCompactUptime converts raw system uptime into a space-efficient format (e.g. "4d 12h 30m").
func FormatCompactUptime(raw string) string {
	u := strings.TrimPrefix(strings.TrimSpace(raw), "up ")
	u = strings.ReplaceAll(u, " hours", "h")
	u = strings.ReplaceAll(u, " hour", "h")
	u = strings.ReplaceAll(u, " minutes", "m")
	u = strings.ReplaceAll(u, " minute", "m")
	u = strings.ReplaceAll(u, " days", "d")
	u = strings.ReplaceAll(u, " day", "d")
	u = strings.ReplaceAll(u, ", ", " ")
	return u
}
