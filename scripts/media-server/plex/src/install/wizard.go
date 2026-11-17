// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Plex Media Server Suite (plex)
//
// This package orchestrates automated pre-flight checks, package repository setup,
// configuration templating, service enablement (systemd/openrc), and firewall rules.
// Supports both interactive TUI step-by-step guidance and headless automation.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"krotek-plex-installer/core"
	"krotek-plex-installer/logger"
	"krotek-plex-installer/system"
	"krotek-plex-installer/tui"
)

// FreshInstallWizard handles the end-to-end installation pipeline for Plex Media Server Suite.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	accessMode := "local"
	envType := targetEnv
	if envType == "" {
		envType = "docker"
	}

	port := "32400"
	claimToken := ""
	mediaDir := "/media"
	composeDir := "/etc/plex-docker"

	PromptExistingConfigAction("Plex Media Server", "/var/lib/plexmediaserver")

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/4: Choose Access Scope (Docker Plex)"
			if envType == "native" {
				wizardTitle = "Step 1/4: Choose Access Scope (Native Plex)"
			}

			idx, _, status := tui.SelectMenu(wizardTitle, []string{
				"Local Network Only (LAN / Home Streaming)",
				"Remote Access (Plex Cloud Relay / Remote Streaming)",
			})
			if status != 0 {
				return
			}
			if idx == 0 {
				accessMode = "local"
			} else {
				accessMode = "public"
			}
			currentStep = 2

		case 2:
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Plex Media Server Port & Account Link" + core.NC,
				"",
				"  Specify listening web port and optional Plex Claim Token.",
				"  " + core.BOLD + "Default Port:" + core.NC + " " + core.CYAN + "32400" + core.NC + " (Plex Standard Port)",
				"  " + core.BOLD + "Claim Token :" + core.NC + " Obtain at " + core.YELLOW + "https://plex.tv/claim" + core.NC + " (optional)",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: Port & Claim Token", cardLines)

			portIn, st := tui.ReadTextInput("Plex Web Port", port)
			if st != 0 {
				currentStep = 1
				continue
			}
			if resolvedPort, ok := system.ResolvePortConflict(portIn, "tcp", true); !ok {
				currentStep = 1
				continue
			} else {
				port = resolvedPort
			}

			cIn, st2 := tui.ReadTextInput("Plex Claim Token (leave blank to skip)", claimToken)
			if st2 != 0 {
				currentStep = 1
				continue
			}
			claimToken = cIn

			currentStep = 3

		case 3:
			tui.ClearScreen()
			mediaCardLines := []string{
				core.BOLD + core.CYAN + "Media Storage Directory Configuration" + core.NC,
				"",
				"  Specify the host storage path where your movies, TV shows, and media are kept.",
				"  " + core.BOLD + "Default Location :" + core.NC + " " + core.YELLOW + "/media" + core.NC + " (or /var/media)",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: Media Storage Path", mediaCardLines)

			mIn, st := tui.ReadTextInput("Host Media Path", mediaDir)
			if st != 0 {
				currentStep = 2
				continue
			}
			if mIn != "" {
				mIn = strings.TrimSpace(mIn)
				if err := system.ValidateSafeDirectoryPath(mIn); err != nil {
					tui.PrintInBox("Invalid Media Storage Path", []string{
						core.RED + err.Error() + core.NC,
					})
					continue
				}
				mediaDir = mIn
			}

			currentStep = 4

		case 4:
			tui.ClearScreen()
			summaryLines := []string{
				tui.FormatKV("Deployment Target", func() string {
					if envType == "docker" {
						return "Docker Compose Container Stack (Plex Official)"
					}
					return "Native Linux Service Stack (plexmediaserver systemd)"
				}(), core.GREEN),
				tui.FormatKV("Access Scope", func() string {
					if accessMode == "public" {
						return "Remote Access (WAN + Plex.tv Cloud Link)"
					}
					return "Local Network Only (LAN)"
				}(), core.GREEN),
				tui.FormatKV("Plex Web Port", port, core.CYAN),
				tui.FormatKV("Media Storage Path", mediaDir, core.YELLOW+core.BOLD),
			}

			if claimToken != "" {
				summaryLines = append(summaryLines, tui.FormatKV("Plex Claim Token", "[PROVIDED]", core.GREEN))
			} else {
				summaryLines = append(summaryLines, tui.FormatKV("Plex Claim Token", "[NONE - Setup via Web GUI]", core.YELLOW))
			}

			if envType == "docker" {
				summaryLines = append(summaryLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
			} else {
				summaryLines = append(summaryLines, tui.FormatKV("Config Directory", "/var/lib/plexmediaserver", core.CYAN))
			}

			summaryLines = append(summaryLines, tui.FormatKV("Automated Firewall", "[ENABLED] (Port 32400 & Discovery)", core.CYAN))
			summaryLines = append(summaryLines, "")
			summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"Do you want to proceed with installation?"+core.NC)

			choices := []string{"YES", "NO"}

			idx, _, status := tui.SelectMenuWithContent("Step 4/4: Installation Summary", summaryLines, choices)
			if status == 2 || idx != 0 {
				logger.LogInfo("Installation aborted by user.")
				return
			}
			if status == 1 {
				currentStep = 3
				continue
			}

			if envType == "docker" {
				ExecuteDockerInstall(accessMode, port, claimToken, mediaDir, composeDir)
			} else {
				ExecuteNativeInstall(accessMode, port, claimToken, mediaDir)
			}
			return
		}
	}
}

// ==============================================================================
// DOCKER COMPOSE EXECUTOR
// ==============================================================================
func ExecuteDockerInstall(accessMode, port, claimToken, mediaDir, composeDir string) {
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing PLEX MEDIA SERVER (Docker Compose Stack) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)

	if !system.EnsureDockerEngine() {
		return
	}

	if composeDir == "" {
		composeDir = "/etc/plex-docker"
	}
	_ = os.MkdirAll(composeDir, 0755)
	_ = os.MkdirAll(filepath.Join(composeDir, "config"), 0755)
	_ = os.MkdirAll(filepath.Join(composeDir, "transcode"), 0755)
	_ = os.MkdirAll(mediaDir, 0755)

	claimEnv := ""
	if claimToken != "" {
		claimEnv = fmt.Sprintf("      - PLEX_CLAIM=%s\n", claimToken)
	}

	composeYaml := fmt.Sprintf(`version: '3.8'

services:
  plex:
    image: plexinc/pms-docker:latest
    container_name: krotek-plex
    restart: always
    environment:
      - TZ=UTC
%s    ports:
      - "%s:32400"
      - "1900:1900/udp"
      - "32410:32410/udp"
      - "32412:32412/udp"
      - "32413:32413/udp"
      - "32414:32414/udp"
    volumes:
      - ./config:/config
      - ./transcode:/transcode
      - %s:/data
`, claimEnv, port, mediaDir)

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}

	// Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	system.ConfigureFirewallRules(port+"/tcp", "1900/udp", "32410/udp", "32412/udp", "32413/udp", "32414/udp", "32469/tcp", "8324/tcp")

	if !system.RunDockerCompose(composeDir, "Deploying Plex Docker Stack", true, "up", "-d") {
		return
	}

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "docker",
		"ACCESS_MODE": accessMode,
		"PORT":        port,
		"MEDIA_DIR":   mediaDir,
		"COMPOSE_DIR": composeDir,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	localIP := system.GetLocalIP()
	if localIP == "" {
		localIP = "127.0.0.1"
	}
	pubIP := system.GetPublicIP()
	if pubIP == "" {
		pubIP = localIP
	}

	var finalLines []string
	finalLines = append(finalLines, tui.FormatKV("Deployment Mode", "Docker Container Stack (Plex Official)", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Web App Local URL", fmt.Sprintf("http://%s:%s/web", localIP, port), core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Localhost URL", fmt.Sprintf("http://localhost:%s/web", port), core.GREEN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"MEDIA STORAGE PATHS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Host Media Path", mediaDir, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Public Server IP", pubIP, core.CYAN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Management Command", "plex-manager", core.GREEN+core.BOLD))

	tui.ClearScreen()
	tui.PrintInBox("Plex Media Server Docker Stack Installed Successfully", finalLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// ==============================================================================
// NATIVE INSTALLATION EXECUTOR
// ==============================================================================
func ExecuteNativeInstall(accessMode, port, claimToken, mediaDir string) {
	if core.SysInfo.Family == "alpine" {
		tui.PrintInBox("PLEX Native" + " " + core.Sym.EmDash + " " + "Alpine Linux Not Supported", []string{
			core.BOLD + core.RED + "Native PLEX service is not supported on Alpine Linux." + core.NC,
			"Alpine Linux lacks required systemd/glibc dependencies.",
			"",
			core.BOLD + core.GREEN + "Recommendation:" + core.NC + " Use the Docker container deployment mode instead.",
		})
		logger.LogError("Native PLEX is unsupported on Alpine Linux. Aborting.")
		return
	}
	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}

	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing PLEX MEDIA SERVER Native Linux Service ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native Plex installation...")

	if core.SysInfo.Family == "debian" {
		if !system.RunStepOrAbort("Updating package index", "apt-get", "update", "-y") {
			return
		}
		if !system.RunStepOrAbort("Installing prerequisites", // Dpkg options preserve existing configuration and prevent blocking terminal prompts in TUI & automation
			"apt-get", "install", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "curl", "wget", "gnupg", "ca-certificates") {
			return
		}
		repoScript := "curl --proto '=https' --tlsv1.2 -fsSL https://downloads.plex.tv/plex-keys/PlexSign.key | gpg --dearmor -o /usr/share/keyrings/plexmediaserver.gpg --yes 2>/dev/null || true; echo 'deb [signed-by=/usr/share/keyrings/plexmediaserver.gpg] https://downloads.plex.tv/repo/deb public main' > /etc/apt/sources.list.d/plexmediaserver.list; apt-get update -y -qq; apt-get install -y plexmediaserver"
		_ = system.RunMultiStep("Installing Plex Media Server from official repository", repoScript)
	} else if core.SysInfo.Family == "rhel" {
		if !system.InstallPackages("curl", "wget") {
			logger.LogError("Failed to install prerequisites.")
			return
		}
		repoScript := `tee /etc/yum.repos.d/plex.repo <<'EOF'
[PlexRepo]
name=Plex
baseurl=https://downloads.plex.tv/repo/rpm/$basearch/
enabled=1
gpgkey=https://downloads.plex.tv/plex-keys/PlexSign.key
gpgcheck=1
EOF
dnf install -y plexmediaserver 2>/dev/null || yum install -y plexmediaserver`
		if !system.RunMultiStep("Installing Plex Media Server from official repository", repoScript) {
			logger.LogError("Failed to install Plex Media Server on RHEL.")
			return
		}
	} else {
		var pkgs []string
		pkgs = []string{"curl", "wget"}
		if !system.InstallPackages(pkgs...) {
			logger.LogError("Failed to install prerequisites.")
			return
		}
	}

	// Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	system.ConfigureFirewallRules(port+"/tcp", "1900/udp", "32410/udp", "32412/udp", "32413/udp", "32414/udp", "32469/tcp", "8324/tcp")

	system.StartService("plexmediaserver")

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "native",
		"ACCESS_MODE": accessMode,
		"PORT":        port,
		"MEDIA_DIR":   mediaDir,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	localIP := system.GetLocalIP()
	if localIP == "" {
		localIP = "127.0.0.1"
	}

	var finalLines []string
	finalLines = append(finalLines, tui.FormatKV("Deployment Mode", "Native Linux Service (plexmediaserver)", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Web App Local URL", fmt.Sprintf("http://%s:%s/web", localIP, port), core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Localhost Web URL", fmt.Sprintf("http://localhost:%s/web", port), core.GREEN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"MEDIA STORAGE PATHS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Host Media Path", mediaDir, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Config Directory", "/var/lib/plexmediaserver", core.CYAN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Management Command", "plex-manager", core.GREEN+core.BOLD))

	tui.ClearScreen()
	tui.PrintInBox("Plex Media Server Native Service Installed Successfully", finalLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// PromptExistingConfigAction checks if prior configuration files exist on the host system.
// In interactive TUI mode, it prompts the administrator in English to decide whether to preserve existing custom settings
// or perform a clean factory default reset. In unattended mode (-y / CI/CD), it automatically preserves settings (safe default).
func PromptExistingConfigAction(serviceTitle string, configPaths ...string) {
	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
		return
	}

	hasExisting := false
	for _, p := range configPaths {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				hasExisting = true
				break
			}
		}
	}
	if !hasExisting {
		return
	}

	tui.ClearScreen()
	cardLines := []string{
		core.BOLD + core.YELLOW + "EXISTING CONFIGURATION DETECTED" + core.NC,
		"",
		fmt.Sprintf("  Prior configuration files were detected for %s on this host.", serviceTitle),
		"  Choose how the deployment wizard should handle existing settings:",
		"",
		"  " + core.GREEN + "[1] Keep Existing Configuration (Recommended)" + core.NC,
		"      Preserves all existing configuration, virtual hosts, and custom data intact.",
		"",
		"  " + core.YELLOW + "[2] Clean Reset to Factory Default (Overwrite)" + core.NC,
		"      Overwrites configuration with fresh, production-hardened defaults.",
	}

	choices := []string{
		"Keep Existing Configuration (Preserve custom settings & virtual hosts)",
		"Clean Reset to Factory Default (Overwrite with fresh configurations)",
	}

	idx, _, status := tui.SelectMenuWithContent("Configuration Conflict Handler", cardLines, choices)
	if status == 2 {
		tui.LeaveAltScreen()
		logger.LogInfo("Setup cancelled by user at configuration conflict prompt.")
		os.Exit(0)
	}
	if status == 0 && idx == 1 {
		core.OverwriteExistingConfig = true
		logger.LogInfo("User selected: Clean reset to factory default configuration.")
	} else {
		core.OverwriteExistingConfig = false
		logger.LogInfo("User selected: Preserve existing configuration.")
	}
}
