// ==============================================================================
// KROTEK SCRIPT HUB — SERVICE OPERATIONS & DIAGNOSTIC HEALTH INSPECTOR
// Service: MINECRAFT (game-servers)
// File:    game-servers/minecraft/src/modules/ops.go
//
// Provides deep health inspections, port availability checks, service daemon status verification, and lifecycle management operations.
// ==============================================================================

package modules

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"krotek-minecraft-installer/core"
	"krotek-minecraft-installer/system"
	"krotek-minecraft-installer/tui"
)

// MinecraftHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// MinecraftHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// MinecraftHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// MinecraftHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
type MinecraftHealth struct {
	Installed   bool
	EnvType     string
	IsRunning   bool
	PortOpen    bool
	VersionText string
}

// DetectMinecraftHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectMinecraftHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectMinecraftHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectMinecraftHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
func DetectMinecraftHealth() MinecraftHealth {
	var h MinecraftHealth
	state := system.LoadState()
	h.EnvType = state["ENV_TYPE"]

	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-minecraft").Output(); err == nil {
		h.Installed = true
		if h.EnvType == "" {
			h.EnvType = "docker"
		}
		if strings.TrimSpace(string(out)) == "running" {
			h.IsRunning = true
		}
	} else if system.IsServiceInstalled() || (exec.Command("systemctl", "is-active", "--quiet", "minecraft").Run() == nil || exec.Command("rc-service", "minecraft", "status").Run() == nil || exec.Command("pidof", "minecraft").Run() == nil) {
		h.Installed = true
		if h.EnvType == "" {
			h.EnvType = "native"
		}
		if exec.Command("systemctl", "is-active", "--quiet", "minecraft").Run() == nil || exec.Command("rc-service", "minecraft", "status").Run() == nil || exec.Command("pidof", "minecraft").Run() == nil {
			h.IsRunning = true
		}
	}

	port := state["SERVER_PORT"]
	if port == "" {
		port = core.DefaultServerPort
	}
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 600*time.Millisecond); err == nil {
		_ = conn.Close()
		h.PortOpen = true
	}

	if h.Installed && h.VersionText == "" {
		propPath := filepath.Join(system.GetMinecraftDataDir(), "server.properties")
		if data, err := os.ReadFile(propPath); err == nil {
			for _, l := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(l, "server-version=") {
					h.VersionText = strings.TrimSpace(strings.TrimPrefix(l, "server-version="))
					break
				}
			}
		}
		if h.VersionText == "" {
			if v, ok := state["MC_VERSION"]; ok && v != "" {
				h.VersionText = v
			} else {
				h.VersionText = "Minecraft Java Server"
			}
		}
	}

	return h
}

func RunDeepDiagnostics() {
	tui.ClearScreen()
	h := DetectMinecraftHealth()
	state := system.LoadState()

	serverDir := system.GetMinecraftDataDir()

	eulaStatus := "[NOT FOUND]"
	if data, err := os.ReadFile(filepath.Join(serverDir, "eula.txt")); err == nil && strings.Contains(string(data), "eula=true") {
		eulaStatus = "[ACCEPTED]"
	}

	onlineMode := "false"
	serverPort := core.DefaultServerPort
	if data, err := os.ReadFile(filepath.Join(serverDir, "server.properties")); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "online-mode=") {
				onlineMode = strings.TrimPrefix(l, "online-mode=")
			} else if strings.HasPrefix(l, "server-port=") {
				serverPort = strings.TrimPrefix(l, "server-port=")
			}
		}
	}

	javaInfo := state["JAVA_BIN"]
	if javaInfo == "" {
		if path, err := exec.LookPath("java"); err == nil {
			javaInfo = path
		} else {
			javaInfo = "Not detected in PATH"
		}
	}

	ramAlloc := state["ALLOCATED_RAM"]
	if ramAlloc == "" {
		ramAlloc = "2G"
	}

	var diag []string
	diag = append(diag, core.BOLD+core.YELLOW+"=== MINECRAFT JAVA SERVER SYSTEM DIAGNOSTIC REPORT ==="+core.NC, "")
	diag = append(diag, tui.FormatKV("Host System", system.GetOSPrettyVersion(), core.CYAN))
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
	diag = append(diag, tui.FormatKV("Application Engine", func() string {
		eng := state["ENGINE"]
		if eng == "" {
			eng = "PaperMC"
		}
		ver := h.VersionText
		if ver == "" {
			ver = state["MC_VERSION"]
		}
		return fmt.Sprintf("%s %s", eng, ver)
	}(), core.YELLOW))
	diag = append(diag, tui.FormatKV("Java Runtime Path", javaInfo, core.CYAN))
	diag = append(diag, tui.FormatKV("Allocated Memory (Heap)", fmt.Sprintf("-Xms1G -Xmx%s", ramAlloc), core.CYAN))
	diag = append(diag, tui.FormatKV("Mojang EULA Status", func() string {
		if eulaStatus == "[ACCEPTED]" {
			return core.GREEN + eulaStatus
		}
		return core.RED + eulaStatus
	}(), core.GREEN))
	diag = append(diag, tui.FormatKV("Online Mode Authentication", func() string {
		if onlineMode == "true" {
			return core.GREEN + "TRUE (Premium Mojang Accounts Only)"
		}
		return core.YELLOW + "FALSE (LAN / Offline Clients Allowed)"
	}(), core.CYAN))
	diag = append(diag, tui.FormatKV("Service Port ("+serverPort+" TCP)", func() string {
		if h.PortOpen {
			return core.GREEN + "[ACTIVE / LISTENING]"
		}
		return core.YELLOW + "[STANDBY / CLOSED]"
	}(), core.GREEN))
	diag = append(diag, tui.FormatKV("Local Network Access", fmt.Sprintf("%s:%s", system.GetLocalIP(), serverPort), core.BOLD+core.YELLOW))
	diag = append(diag, tui.FormatKV("Public WAN Access", fmt.Sprintf("%s:%s", system.GetPublicIP(), serverPort), core.BOLD+core.GREEN))

	tui.PrintInBox("Deep Diagnostics Report", diag)
}
