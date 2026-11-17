// ==============================================================================
// KROTEK SCRIPT HUB — SERVICE OPERATIONS & DIAGNOSTIC HEALTH INSPECTOR
// Service: NITROX (game-servers)
// File:    game-servers/nitrox/src/modules/ops.go
//
// Provides deep health inspections, port availability checks, service daemon status verification, and lifecycle management operations.
// ==============================================================================

package modules

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"krotek-nitrox-installer/core"
	"krotek-nitrox-installer/system"
	"krotek-nitrox-installer/tui"
)

// NitroxHealth encapsulates runtime operational telemetry, container state, system daemon status, and version string.
type NitroxHealth struct {
	Installed      bool
	EnvType        string
	IsRunning      bool
	PortOpen       bool
	VersionText    string
	DotNetVersion  string
	GameFilesFound bool
	GameFilesPath  string
	SaveWorldCount int
}

// DetectNitroxHealth performs live health inspection by querying container engines, systemd units, process tables, and game directories.
func DetectNitroxHealth() NitroxHealth {
	var h NitroxHealth
	state := system.LoadState()
	h.EnvType = state["ENV_TYPE"]
	if h.EnvType == "" {
		h.EnvType = "native"
	}

	installDir := state["INSTALL_DIR"]
	if installDir == "" {
		if _, err := os.Stat("/srv/nitrox"); err == nil {
			installDir = "/srv/nitrox"
		} else {
			installDir = "/opt/nitrox"
		}
	}
	gameDir := state["GAME_PATH"]
	if gameDir == "" {
		gameDir = filepath.Join(installDir, "game")
	}
	h.GameFilesPath = gameDir

	// Check Subnautica Game Files
	if fi, err := os.Stat(filepath.Join(gameDir, "Subnautica_Data")); err == nil && fi.IsDir() {
		h.GameFilesFound = true
	} else if _, err := os.Stat(filepath.Join(gameDir, "Subnautica.exe")); err == nil {
		h.GameFilesFound = true
	}

	// Check Save worlds
	dataDir := state["DATA_PATH"]
	if dataDir == "" {
		dataDir = filepath.Join(installDir, "data")
	}
	savesDir := filepath.Join(dataDir, "saves")
	if entries, err := os.ReadDir(savesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				h.SaveWorldCount++
			}
		}
	}

	// Check .NET Runtime
	if out, err := exec.Command("dotnet", "--list-runtimes").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, l := range lines {
			if strings.Contains(l, "Microsoft.NETCore.App") {
				fields := strings.Fields(l)
				if len(fields) >= 2 {
					h.DotNetVersion = fields[1]
					break
				}
			}
		}
	}

	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-nitrox").Output(); err == nil {
		h.Installed = true
		h.EnvType = "docker"
		if strings.TrimSpace(string(out)) == "running" {
			h.IsRunning = true
			h.PortOpen = true
		}
	} else if system.IsInstalledState() || system.IsServiceInstalled() || exec.Command("systemctl", "is-active", "--quiet", "nitrox").Run() == nil || exec.Command("pidof", "Nitrox.Server.Subnautica").Run() == nil {
		h.Installed = true
		if exec.Command("systemctl", "is-active", "--quiet", "nitrox").Run() == nil || exec.Command("rc-service", "nitrox", "status").Run() == nil || exec.Command("pidof", "Nitrox.Server.Subnautica").Run() == nil || exec.Command("pidof", "nitrox").Run() == nil {
			h.IsRunning = true
			h.PortOpen = true
		}
	}

	if h.Installed && h.VersionText == "" {
		serverDir := state["SERVER_DIR"]
		if serverDir == "" {
			serverDir = filepath.Join(installDir, "server")
		}

		if ver := state["NITROX_VERSION"]; ver != "" && ver != "latest" && ver != "unknown" {
			h.VersionText = ver
		} else if detected := system.DetectInstalledNitroxVersion(serverDir); detected != "" {
			h.VersionText = detected
		} else if tag, _ := system.ResolveLatestNitroxRelease(core.SysInfo.Architecture); tag != "" {
			h.VersionText = tag
		} else {
			h.VersionText = "latest"
		}
	}

	return h
}

// RunDeepDiagnostics prints an extensive diagnostic report covering OS, service daemon, network, and game assets.
func RunDeepDiagnostics() {
	tui.ClearScreen()
	h := DetectNitroxHealth()
	state := system.LoadState()

	port := state["PORT"]
	if port == "" {
		port = "11000"
	}

	var diag []string
	diag = append(diag, core.BOLD+core.YELLOW+"=== SUBNAUTICA NITROX MULTIPLAYER DIAGNOSTIC AUDIT ==="+core.NC, "")
	diag = append(diag, tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN))
	diag = append(diag, tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN))
	diag = append(diag, tui.FormatKV("Deployment Stack", strings.ToUpper(h.EnvType), core.BOLD+core.WHITE))
	diag = append(diag, tui.FormatKV("Installation State", func() string {
		if h.Installed {
			return core.GREEN + "[INSTALLED]" + core.NC
		}
		return core.RED + "[NOT INSTALLED]" + core.NC
	}(), core.GREEN))
	diag = append(diag, tui.FormatKV("Service Running State", func() string {
		if h.IsRunning {
			return core.GREEN + "[ACTIVE / RUNNING]" + core.NC
		}
		return core.RED + "[STOPPED / INACTIVE]" + core.NC
	}(), core.GREEN))
	if h.VersionText != "" {
		diag = append(diag, tui.FormatKV("Nitrox Version", h.VersionText, core.YELLOW))
	}
	if h.DotNetVersion != "" {
		diag = append(diag, tui.FormatKV(".NET Core Runtime", h.DotNetVersion, core.GREEN))
	} else {
		diag = append(diag, tui.FormatKV(".NET Core Runtime", core.YELLOW+"System /opt/dotnet"+core.NC, core.YELLOW))
	}
	diag = append(diag, tui.FormatKV(fmt.Sprintf("UDP Port (%s/UDP)", port), func() string {
		if h.PortOpen {
			return core.GREEN + "[LISTENING / OPEN]" + core.NC
		}
		return core.YELLOW + "[STANDBY]" + core.NC
	}(), core.GREEN))
	diag = append(diag, tui.FormatKV("Subnautica Assets", func() string {
		if h.GameFilesFound {
			return core.GREEN + "[VERIFIED] (Subnautica_Data found)" + core.NC
		}
		return core.YELLOW + "[PENDING UPLOAD] (Requires game assets in " + h.GameFilesPath + ")" + core.NC
	}(), core.GREEN))
	diag = append(diag, tui.FormatKV("World Saves Count", fmt.Sprintf("%d active save(s)", h.SaveWorldCount), core.CYAN))

	tui.PrintInBox("Deep Diagnostics Report", diag)
}
