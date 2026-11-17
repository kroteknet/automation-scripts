// ==============================================================================
// KROTEK SCRIPT HUB — HARDWARE & OPERATING SYSTEM DETECTION ENGINE
// Service: OpenResty (web-server)
// File:    web-server/openresty/src/system/detect.go
//
// Discovers the host operating system family (Debian/Ubuntu, RHEL/CentOS/Rocky/Alma, Alpine Linux), architecture (amd64, arm64), version, codename, and active init system (systemd, OpenRC).
// ==============================================================================

package system

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	"krotek-openresty-installer/core"
)

// IsSystemdRunning checks if systemd is booted and running as PID 1 on the host machine.
func IsSystemdRunning() bool {
	if fi, err := os.Stat("/run/systemd/system"); err == nil && fi.IsDir() {
		return true
	}
	return false
}

// DetectSystem inspects the host machine to discover hardware architecture (amd64, arm64),
// Linux distribution family (Debian/Ubuntu, RHEL/CentOS, Alpine), version, and active init system (systemd, OpenRC, sysvinit).
// The discovered metadata is stored in core.SysInfo for package management and service unit decisions.
func DetectSystem() {
	core.SysInfo.Architecture = runtime.GOARCH

	if _, err := os.Stat("/etc/alpine-release"); err == nil {
		core.SysInfo.Distro = "alpine"
		core.SysInfo.Family = "alpine"
		core.SysInfo.InitSystem = "openrc"
		if data, err := os.ReadFile("/etc/alpine-release"); err == nil {
			rel := strings.TrimSpace(string(data))
			core.SysInfo.Version = rel
			parts := strings.Split(rel, ".")
			if len(parts) >= 2 {
				core.SysInfo.Codename = "v" + parts[0] + "." + parts[1]
			} else if rel != "" {
				core.SysInfo.Codename = rel
			}
		}
		return
	}

	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(data), "\n")
		var id, idLike, versionId, versionCodename, prettyName string
		for _, line := range lines {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
				switch key {
				case "ID":
					id = strings.ToLower(val)
				case "ID_LIKE":
					idLike = strings.ToLower(val)
				case "VERSION_ID":
					versionId = val
				case "VERSION_CODENAME":
					versionCodename = strings.ToLower(val)
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
			parts := strings.Split(versionId, ".")
			if len(parts) >= 2 {
				core.SysInfo.Codename = "v" + parts[0] + "." + parts[1]
			} else {
				core.SysInfo.Codename = versionCodename
			}
		} else {
			if id == "centos" || id == "rhel" || id == "almalinux" || id == "rocky" || id == "fedora" || id == "ol" || strings.Contains(idLike, "rhel") || strings.Contains(idLike, "centos") || strings.Contains(idLike, "fedora") {
				core.SysInfo.Family = "rhel"
				core.SysInfo.Codename = versionId
			} else {
				core.SysInfo.Family = "debian"
				if versionCodename == "" && CommandExists("lsb_release") {
					if out, err := exec.Command("lsb_release", "-cs").Output(); err == nil {
						c := strings.ToLower(strings.TrimSpace(string(out)))
						if c != "" && c != "n/a" {
							versionCodename = c
						}
					}
				}
				if versionCodename == "" {
					switch {
					case id == "debian" && strings.HasPrefix(versionId, "13"):
						versionCodename = "trixie"
					case id == "debian" && strings.HasPrefix(versionId, "12"):
						versionCodename = "bookworm"
					case id == "debian" && strings.HasPrefix(versionId, "11"):
						versionCodename = "bullseye"
					case id == "debian" && strings.HasPrefix(versionId, "10"):
						versionCodename = "buster"
					case id == "ubuntu" && strings.HasPrefix(versionId, "24.04"):
						versionCodename = "noble"
					case id == "ubuntu" && strings.HasPrefix(versionId, "22.04"):
						versionCodename = "jammy"
					case id == "ubuntu" && strings.HasPrefix(versionId, "20.04"):
						versionCodename = "focal"
					}
				}
				core.SysInfo.Codename = versionCodename
			}

			if IsSystemdRunning() {
				core.SysInfo.InitSystem = "systemd"
			} else if CommandExists("rc-service") || func() bool { _, err := os.Stat("/run/openrc"); return err == nil }() {
				core.SysInfo.InitSystem = "openrc"
			} else {
				core.SysInfo.InitSystem = "sysvinit"
			}
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
	return strings.Join(strings.Fields(osStr), " ")
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
