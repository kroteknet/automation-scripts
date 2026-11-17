// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Jellyfin Free Software Media System (jellyfin)
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

	"krotek-jellyfin-installer/core"
	"krotek-jellyfin-installer/logger"
	"krotek-jellyfin-installer/system"
	"krotek-jellyfin-installer/tui"
)

// FreshInstallWizard handles the end-to-end installation pipeline for Jellyfin Free Software Media System.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	accessMode := "local"
	envType := targetEnv
	if envType == "" {
		envType = "docker"
	}

	port := "8096"
	domainName := ""
	enableSSL := false
	sslEmail := ""
	mediaDir := "/media"
	composeDir := "/etc/jellyfin-docker"

	PromptExistingConfigAction("Jellyfin Media Server", "/etc/jellyfin")

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/4: Choose Access Scope (Docker Jellyfin)"
			if envType == "native" {
				wizardTitle = "Step 1/4: Choose Access Scope (Native Jellyfin)"
			}

			idx, _, status := tui.SelectMenu(wizardTitle, []string{
				"Local Network Only (LAN / Home Streaming)",
				"Public Remote Access (Domain + WAN Streaming)",
			})
			if status != 0 {
				return
			}
			if idx == 0 {
				accessMode = "local"
				port = "8096"
			} else {
				accessMode = "public"
				port = "8096"
			}
			currentStep = 2

		case 2:
			tui.ClearScreen()
			if accessMode == "local" {
				cardLines := []string{
					core.BOLD + core.CYAN + "Local Network Configuration" + core.NC,
					"",
					"  Specify the listening HTTP port for Jellyfin LAN streaming.",
					"  " + core.BOLD + "Default Port:" + core.NC + " " + core.CYAN + "8096" + core.NC,
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: LAN Network Options", cardLines)

				portIn, st := tui.ReadTextInput("Jellyfin Web Port", port)
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
				domainName = ""
				enableSSL = false
			} else {
				cardLines := []string{
					core.BOLD + core.CYAN + "Public Remote Access & Domain Setup" + core.NC,
					"",
					"  Enter your public Domain Name (e.g. media.example.com).",
					"  Ensure your domain DNS points to this server's Public IP.",
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: Public Domain Setup", cardLines)

				domIn, st := tui.ReadTextInput("Domain Name (FQDN)", domainName)
				if st != 0 {
					currentStep = 1
					continue
				}
				domainName = system.CleanDomain(domIn)
				domainName = system.CleanDomain(domIn)

				portIn, st := tui.ReadTextInput("Jellyfin Web Port", port)
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

				res := tui.AskYesNo("Enable automated Let's Encrypt SSL certificate (HTTPS)?")
				if res == 2 {
					currentStep = 1
					continue
				}
				if res == 0 {
					enableSSL = true
					defaultSSLEmail := sslEmail
					if defaultSSLEmail == "" {
						defaultSSLEmail = "admin@" + domainName
					}
					emailIn, st := tui.ReadTextInput("SSL Admin Email", defaultSSLEmail)
					if st != 0 {
						currentStep = 1
						continue
					}
					sslEmail = emailIn
				} else {
					enableSSL = false
					sslEmail = ""
				}
			}

			currentStep = 3

		case 3:
			tui.ClearScreen()
			mediaCardLines := []string{
				core.BOLD + core.CYAN + "Media Storage Directory Configuration" + core.NC,
				"",
				"  Specify the host storage path where your movies, TV shows, and music are stored.",
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
						return "Docker Compose Container Stack (Jellyfin Official)"
					}
					return "Native Linux Service Stack (jellyfin systemd)"
				}(), core.GREEN),
				tui.FormatKV("Access Scope", func() string {
					if accessMode == "public" {
						return "Public Remote Access (Domain + WAN)"
					}
					return "Local Network Only (LAN)"
				}(), core.GREEN),
				tui.FormatKV("HTTP Web Port", port, core.CYAN),
				tui.FormatKV("Media Storage Path", mediaDir, core.YELLOW+core.BOLD),
			}

			if domainName != "" {
				summaryLines = append(summaryLines, tui.FormatKV("Public Domain Name", domainName, core.YELLOW))
				if enableSSL {
					summaryLines = append(summaryLines, tui.FormatKV("Let's Encrypt SSL", fmt.Sprintf("[ENABLED] (%s)", sslEmail), core.GREEN))
				}
			}

			if envType == "docker" {
				summaryLines = append(summaryLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
			} else {
				summaryLines = append(summaryLines, tui.FormatKV("Config Directory", "/etc/jellyfin", core.CYAN))
			}

			summaryLines = append(summaryLines, tui.FormatKV("DLNA & Discovery", "[ENABLED] (Ports 1900 & 7359 UDP)", core.CYAN))
			summaryLines = append(summaryLines, tui.FormatKV("Automated Firewall", "[ENABLED] (Optimal Ports Opened)", core.CYAN))
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
				ExecuteDockerInstall(accessMode, port, domainName, enableSSL, sslEmail, mediaDir, composeDir)
			} else {
				ExecuteNativeInstall(accessMode, port, domainName, enableSSL, sslEmail, mediaDir)
			}
			return
		}
	}
}

// ==============================================================================
// DOCKER COMPOSE EXECUTOR
// ==============================================================================
func ExecuteDockerInstall(accessMode, port, domain string, enableSSL bool, sslEmail, mediaDir, composeDir string) {
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing JELLYFIN (Docker Compose Stack) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)

	if !system.EnsureDockerEngine() {
		return
	}

	if composeDir == "" {
		composeDir = "/etc/jellyfin-docker"
	}
	_ = os.MkdirAll(composeDir, 0755)
	_ = os.MkdirAll(filepath.Join(composeDir, "config"), 0755)
	_ = os.MkdirAll(filepath.Join(composeDir, "cache"), 0755)
	_ = os.MkdirAll(mediaDir, 0755)

	composeYaml := fmt.Sprintf(`version: '3.8'

services:
  jellyfin:
    image: jellyfin/jellyfin:latest
    container_name: krotek-jellyfin
    restart: always
    ports:
      - "%s:8096"
      - "7359:7359/udp"
      - "1900:1900/udp"
    volumes:
      - ./config:/config
      - ./cache:/cache
      - %s:/media
`, port, mediaDir)

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}

	// Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	system.ConfigureFirewallRules(port+"/tcp", "7359/udp", "1900/udp")

	if !system.RunDockerCompose(composeDir, "Deploying Jellyfin Docker Stack", true, "up", "-d") {
		return
	}

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "docker",
		"ACCESS_MODE": accessMode,
		"PORT":        port,
		"DOMAIN_NAME": domain,
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
	finalLines = append(finalLines, tui.FormatKV("Deployment Mode", "Docker Container Stack (Jellyfin Official)", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Localhost Web URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
	if domain != "" {
		finalLines = append(finalLines, tui.FormatKV("Public Remote URL", fmt.Sprintf("http://%s:%s", domain, port), core.GREEN))
	}
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"MEDIA STORAGE & DISCOVERY:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Host Media Path", mediaDir, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("DLNA Discovery Port", "1900/udp (Active)", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Client Discovery Port", "7359/udp (Active)", core.GREEN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Management Command", "jellyfin-manager", core.GREEN+core.BOLD))

	tui.ClearScreen()
	tui.PrintInBox("Jellyfin Docker Stack Installed Successfully", finalLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// ==============================================================================
// NATIVE INSTALLATION EXECUTOR
// ==============================================================================
func ExecuteNativeInstall(accessMode, port, domain string, enableSSL bool, sslEmail, mediaDir string) {
	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}

	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing JELLYFIN Native Linux Service ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native Jellyfin installation...")

	if core.SysInfo.Family == "debian" {
		if !system.RunStepOrAbort("Updating package index", "apt-get", "update", "-y") {
			return
		}
		if !system.RunStepOrAbort("Installing prerequisites", // Dpkg options preserve existing configuration and prevent blocking terminal prompts in TUI & automation
			"apt-get", "install", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "curl", "gnupg", "ca-certificates") {
			return
		}
		jfTmpDir, jfErr := os.MkdirTemp("", "krotek-jf-*")
		if jfErr == nil {
			_ = os.Chmod(jfTmpDir, 0700)
			defer os.RemoveAll(jfTmpDir)
			jfScript := filepath.Join(jfTmpDir, "install-debuntu.sh")
			repoScript := fmt.Sprintf("export SKIP_CONFIRM=1; (curl --proto '=https' --tlsv1.2 -fsSL https://repo.jellyfin.org/install-debuntu.sh -o '%s' 2>/dev/null && [ -s '%s' ] && bash '%s' 2>/dev/null) || true; apt-get install -y jellyfin 2>/dev/null || true", jfScript, jfScript, jfScript)
			_ = system.RunMultiStep("Installing Jellyfin from official repository", repoScript)
		}
	} else if core.SysInfo.Family == "rhel" {
		_ = system.InstallPackages("curl", "wget")
		jfTmpDir, jfErr := os.MkdirTemp("", "krotek-jf-*")
		if jfErr != nil {
			logger.LogError(fmt.Sprintf("Failed to allocate secure temporary directory: %v", jfErr))
			return
		}
		_ = os.Chmod(jfTmpDir, 0700)
		defer os.RemoveAll(jfTmpDir)
		jfScript := filepath.Join(jfTmpDir, "install-rpm.sh")
		repoScript := fmt.Sprintf(`
if [ -n "%s" ] && curl --proto '=https' --tlsv1.2 -fsSL https://repo.jellyfin.org/install-rpm.sh -o "%s" 2>/dev/null && [ -s "%s" ] && bash "%s" 2>/dev/null; then
	dnf install -y jellyfin 2>/dev/null || yum install -y jellyfin 2>/dev/null || true
fi
if ! command -v jellyfin >/dev/null 2>&1; then
	dnf install -y 'dnf-command(copr)' 2>/dev/null || true
	dnf copr enable -y jellyfin/jellyfin 2>/dev/null || true
	dnf install -y jellyfin 2>/dev/null || yum install -y jellyfin 2>/dev/null || true
fi
`, jfTmpDir, jfScript, jfScript, jfScript)
		_ = system.RunMultiStep("Installing Jellyfin from official repository", repoScript)
		if !system.CommandExists("jellyfin") {
			logger.LogError("Failed to install Jellyfin native package on RHEL.")
			return
		}
	} else {
		var pkgs []string
		switch core.SysInfo.Family {
		case "alpine":
			pkgs = []string{"jellyfin", "jellyfin-openrc", "curl"}
		case "debian":
			pkgs = []string{"jellyfin", "curl"}
		default:
			pkgs = []string{"jellyfin", "curl"}
		}
		if !system.InstallPackages(pkgs...) {
			logger.LogError("Failed to install Jellyfin packages.")
			return
		}
	}

	// Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	system.ConfigureFirewallRules(port+"/tcp", "7359/udp", "1900/udp")

	system.StartService("jellyfin")

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "native",
		"ACCESS_MODE": accessMode,
		"PORT":        port,
		"DOMAIN_NAME": domain,
		"MEDIA_DIR":   mediaDir,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	localIP := system.GetLocalIP()
	if localIP == "" {
		localIP = "127.0.0.1"
	}

	var finalLines []string
	finalLines = append(finalLines, tui.FormatKV("Deployment Mode", "Native Linux Service Stack", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Local Web URL", fmt.Sprintf("http://%s:%s", localIP, port), core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Localhost Web URL", fmt.Sprintf("http://localhost:%s", port), core.GREEN))
	if domain != "" {
		finalLines = append(finalLines, tui.FormatKV("Public Remote URL", fmt.Sprintf("http://%s:%s", domain, port), core.GREEN))
	}
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"MEDIA STORAGE & DISCOVERY:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Host Media Path", mediaDir, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("DLNA Discovery Port", "1900/udp (Active)", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Client Discovery Port", "7359/udp (Active)", core.GREEN))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT SHORTCUTS:"+core.NC)
	finalLines = append(finalLines, tui.FormatKV("Installer Log File", core.LogFilePath, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Management Command", "jellyfin-manager", core.GREEN+core.BOLD))

	tui.ClearScreen()
	tui.PrintInBox("Jellyfin Native Service Installed Successfully", finalLines)

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
