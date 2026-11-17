// ==============================================================================
// KROTEK SCRIPT HUB — HARDWARE & OPERATING SYSTEM DETECTION ENGINE
// Service: NITROX (game-servers)
// File:    game-servers/nitrox/src/system/detect.go
//
// Discovers the host operating system family (Debian/Ubuntu, RHEL/CentOS/Rocky/Alma, Alpine Linux), architecture (amd64, arm64), version, and active init system (systemd, OpenRC).
// ==============================================================================

package system

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"krotek-nitrox-installer/core"
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

// DetectInstalledNitroxVersion inspects local server binaries to dynamically determine the exact installed version.
// Reads Nitrox.Server.Subnautica.deps.json, PATCH NOTES.txt, or README.txt without any hardcoded values.
func DetectInstalledNitroxVersion(serverDir string) string {
	depsPath := filepath.Join(serverDir, "Nitrox.Server.Subnautica.deps.json")
	if data, err := os.ReadFile(depsPath); err == nil {
		re := regexp.MustCompile(`"Nitrox\.Server\.Subnautica/([0-9.]+)"`)
		if matches := re.FindStringSubmatch(string(data)); len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	patchPath := filepath.Join(serverDir, "PATCH NOTES.txt")
	if data, err := os.ReadFile(patchPath); err == nil {
		re := regexp.MustCompile(`(?:version|v)?\s*([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)`)
		if matches := re.FindStringSubmatch(string(data)); len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	readmePath := filepath.Join(serverDir, "README.txt")
	if data, err := os.ReadFile(readmePath); err == nil {
		re := regexp.MustCompile(`([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)`)
		if matches := re.FindStringSubmatch(string(data)); len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	return ""
}

// DetectRequiredDotNetChannel reads Nitrox.Server.Subnautica.runtimeconfig.json on disk to determine the required .NET version channel.
func DetectRequiredDotNetChannel(serverDir string) string {
	cfgPath := filepath.Join(serverDir, "Nitrox.Server.Subnautica.runtimeconfig.json")
	if data, err := os.ReadFile(cfgPath); err == nil {
		re := regexp.MustCompile(`"tfm":\s*"net([0-9.]+)"`)
		if matches := re.FindStringSubmatch(string(data)); len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
		reVer := regexp.MustCompile(`"version":\s*"([0-9]+)\.`)
		if matches := reVer.FindStringSubmatch(string(data)); len(matches) > 1 {
			return strings.TrimSpace(matches[1]) + ".0"
		}
	}
	return "9.0"
}

// ResolveLatestNitroxRelease dynamically queries the latest official SubnauticaNitrox/Nitrox release from upstream.
// Uses redirect headers and API inspection without hardcoded release versions or API rate-limit bottlenecks.
func ResolveLatestNitroxRelease(arch string) (tag string, downloadURL string) {
	assetArch := "linux_x64"
	if arch == "arm64" || arch == "aarch64" {
		assetArch = "linux_arm64"
	}

	// 1. Upstream redirect header check (zero API rate limits)
	redirectClient := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if req, err := http.NewRequest("HEAD", "https://github.com/SubnauticaNitrox/Nitrox/releases/latest", nil); err == nil {
		req.Header.Set("User-Agent", "KrotekInstaller")
		if resp, err := redirectClient.Do(req); err == nil {
			loc := resp.Header.Get("Location")
			_ = resp.Body.Close()
			if idx := strings.LastIndex(loc, "/tag/"); idx != -1 {
				candidateTag := strings.TrimSpace(loc[idx+5:])
				if candidateTag != "" && !strings.Contains(candidateTag, "/") && regexp.MustCompile(`^[A-Za-z0-9._-]+$`).MatchString(candidateTag) {
					tag = candidateTag
					downloadURL = fmt.Sprintf("https://github.com/SubnauticaNitrox/Nitrox/releases/download/%s/Nitrox_%s_%s.zip", tag, tag, assetArch)
					return tag, downloadURL
				}
			}
		}
	}

	// 2. Official GitHub REST API check
	if body, code, err := HttpFetchBytes("https://api.github.com/repos/SubnauticaNitrox/Nitrox/releases/latest", 2*1024*1024); err == nil && code == http.StatusOK {
		var release struct {
			TagName string `json:"tag_name"`
			Assets  []struct {
				BrowserDownloadURL string `json:"browser_download_url"`
			} `json:"assets"`
		}
		if json.Unmarshal(body, &release) == nil && release.TagName != "" {
			for _, a := range release.Assets {
				if strings.HasPrefix(a.BrowserDownloadURL, "https://") &&
					strings.Contains(a.BrowserDownloadURL, assetArch) &&
					strings.HasSuffix(a.BrowserDownloadURL, ".zip") {
					return release.TagName, a.BrowserDownloadURL
				}
			}
		}
	}

	// 3. GitHub releases page HTML regex scrape fallback
	if body, code, err := HttpFetchBytes("https://github.com/SubnauticaNitrox/Nitrox/releases", 4*1024*1024); err == nil && code == http.StatusOK {
		re := regexp.MustCompile(fmt.Sprintf(`/SubnauticaNitrox/Nitrox/releases/download/([^/"\s]+)/Nitrox_[^"\s]+_%s\.zip`, regexp.QuoteMeta(assetArch)))
		if matches := re.FindSubmatch(body); len(matches) >= 2 {
			relPath := string(matches[0])
			tag = string(matches[1])
			return tag, "https://github.com" + relPath
		}
	}

	return "", ""
}
