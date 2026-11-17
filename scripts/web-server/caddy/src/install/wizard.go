// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Caddy 2 Web Server with Automatic HTTPS (caddy)
//
// This package orchestrates automated pre-flight checks, package repository setup,
// configuration templating, service enablement (systemd/openrc), and firewall rules.
// Supports both interactive TUI step-by-step guidance and headless automation.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"krotek-caddy-installer/core"
	"krotek-caddy-installer/logger"
	"krotek-caddy-installer/modules"
	"krotek-caddy-installer/system"
	"krotek-caddy-installer/tui"
)

// FreshInstallWizard handles the end-to-end installation pipeline for Caddy 2 Web Server with Automatic HTTPS.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	envType := targetEnv
	if envType == "" {
		envType = "native"
	}

	presetProfile := "production"
	PromptExistingConfigAction("Caddy Modern Web Server", "/etc/caddy/Caddyfile")

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			presetOptions := []string{
				"Reverse Proxy & HTTPS Gateway (Recommended for Containers & Web Apps)",
				"Static Web Server & HTML File Hosting (/var/www/html)",
				"Local Network Access Only (LAN / Wi-Fi Private Server - Port 80)",
			}
			idx, _, status := tui.SelectMenu("Step 1/2: Choose Caddy Deployment Architecture Profile", presetOptions)
			if status != 0 {
				return
			}
			switch idx {
			case 0:
				presetProfile = "local"
			case 1:
				presetProfile = "production"
			case 2:
				presetProfile = "public"
			}
			currentStep = 2

		case 2:
			tui.ClearScreen()
			summaryLines := []string{
				tui.FormatKV("Deployment Target", func() string {
					if envType == "docker" {
						return "Docker Compose Container Stack"
					}
					return "Native Linux Service"
				}(), core.GREEN),
				tui.FormatKV("Configuration Preset", func() string {
					switch presetProfile {
					case "local":
						return "Local / Dev Stack (LAN Fast Access)"
					case "public":
						return "Public Internet / WAN Stack (SSL & Security)"
					default:
						return "Production Hardened Stack (Tuned & Secured)"
					}
				}(), core.GREEN+core.BOLD),
				tui.FormatKV("Automated Firewall", "[ENABLED] (Optimal Ports Opened)", core.CYAN),
				"",
				core.BOLD + core.YELLOW + "Do you want to proceed with installation?" + core.NC,
			}

			choices := []string{"YES", "NO"}

			idx, _, status := tui.SelectMenuWithContent("Step 2/2: Installation Summary", summaryLines, choices)
			if status == 2 || idx != 0 {
				logger.LogInfo("Installation aborted by user.")
				return
			}
			if status == 1 {
				currentStep = 1
				continue
			}

			if envType == "docker" {
				ExecuteDockerInstall()
			} else {
				ExecuteNativeInstall()
			}
			return
		}
	}
}

// executeDockerInstall deploys the service as a Docker container stack.
// It installs Docker if absent, writes a docker-compose.yml, starts the stack,
// persists state, and displays a post-install summary.
func ExecuteDockerInstall() {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing CADDY Docker Container Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker CADDY installation...")

	if !system.EnsureDockerEngine() {
		return
	}

	composeDir := "/etc/caddy-docker"
	_ = os.MkdirAll(composeDir, 0755)
	htmlDir := filepath.Join(composeDir, "html")
	_ = os.MkdirAll(htmlDir, 0755)

	caddyfileContent := `:80 {
    root * /usr/share/caddy
    file_server
    encode gzip
}
`
	_ = system.WriteFileNoFollow(filepath.Join(composeDir, "Caddyfile"), []byte(caddyfileContent), 0644, "")

	landingHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
		ServiceName: "Caddy Web Server",
		LogoText:    "C",
		HeroTitle:   "Caddy Web Server Active",
		HeroDesc:    "Your containerized Caddy web server is fully operational and healthy.",
		EnvType:     "docker",
		Port:        "80",
		WebDir:      filepath.Join(composeDir, "html"),
		ConfigPath:  filepath.Join(composeDir, "Caddyfile"),
		WebUser:     "caddy (container)",
		ManagerCmd:  "caddy-manager",
	})
	_ = system.WriteFileNoFollow(filepath.Join(htmlDir, "index.html"), []byte(landingHTML), 0644, "")

	composeYaml := `version: '3.8'

services:
  caddy:
    image: caddy:alpine
    container_name: krotek-caddy
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - ./html:/usr/share/caddy
      - caddy_data:/data
      - caddy_config:/config

volumes:
  caddy_data:
  caddy_config:
`

	_ = system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, "")

	if !system.RunDockerCompose(composeDir, "Deploying CADDY Docker Stack", true, "up", "-d") {
		return
	}

	// Universal firewall hardening (UFW, Firewalld, raw iptables/ip6tables, rules persistence)
	system.ConfigureFirewallRules("80", "443")

	system.SaveState(map[string]string{
		"ENV_TYPE":    "docker",
		"COMPOSE_DIR": composeDir,
	})
	system.CreateGlobalShortcut()

	tui.EnterAltScreen()
	var finalLines []string
	finalLines = append(finalLines, tui.FormatKV("Deployment", "Docker Container", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
	finalLines = append(finalLines, tui.FormatKV("Status", "Running & Active", core.GREEN+core.BOLD))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, tui.FormatKV("To manage, type", "caddy-manager", core.GREEN+core.BOLD))

	tui.PrintInBox("CADDY Docker Stack Installed", finalLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// executeNativeInstall installs the service directly on the host OS.
// Pre-cleans conflicting services and runs the official installer script.
func ExecuteNativeInstall() {
	if _, ok := system.ResolvePortConflict("80", "tcp", true); !ok {
		return
	}
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing CADDY Native Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native CADDY installation...")

	if core.SysInfo.Family == "debian" {
		if !system.RunStepOrAbort("Updating package lists", "apt-get", "update", "-y") {
			return
		}
		_ = exec.Command( // Dpkg options preserve existing configuration and prevent blocking terminal prompts in TUI & automation
			"apt-get", "install", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "debian-keyring", "debian-archive-keyring", "apt-transport-https", "curl").Run()
		_ = exec.Command(system.GetShell(), "-c", "curl --proto '=https' --tlsv1.2 -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor --batch --yes -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg 2>/dev/null || true; curl --proto '=https' --tlsv1.2 -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list 2>/dev/null || true; apt-get update -y -qq").Run()
		if !system.RunStepOrAbort("Installing CADDY packages", // Dpkg options preserve existing configuration and prevent blocking terminal prompts in TUI & automation
			"apt-get", "install", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "caddy") {
			return
		}
	} else if core.SysInfo.Family == "alpine" {
		if !system.RunStepOrAbort("Installing CADDY packages", "apk", "add", "caddy", "caddy-openrc", "curl") {
			return
		}
	} else if core.SysInfo.Family == "rhel" {
		_ = system.RunStep("Refreshing DNF package cache", "dnf", "makecache")
		_ = system.RunMultiStep("Configuring Caddy COPR repository", "dnf install -y 'dnf-command(copr)' 2>/dev/null || dnf install -y epel-release 2>/dev/null || true; dnf copr enable -y @caddy/caddy 2>/dev/null || true")
		if !system.RunStep("Installing CADDY packages", "dnf", "install", "-y", "caddy") {
			logger.LogWarn("COPR repository install failed. Installing official standalone Caddy binary...")
			installScript := `
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) CADDY_ARCH="amd64" ;;
    aarch64|arm64) CADDY_ARCH="arm64" ;;
    *) CADDY_ARCH="amd64" ;;
esac
curl --proto '=https' --tlsv1.2 -fsSL "https://caddyserver.com/api/download?os=linux&arch=${CADDY_ARCH}" -o /usr/bin/caddy || \
curl --proto '=https' --tlsv1.2 -fsSL "https://github.com/caddyserver/caddy/releases/latest/download/caddy_linux_${CADDY_ARCH}.tar.gz" | tar -xz -C /usr/bin caddy
chmod +x /usr/bin/caddy
useradd --system --home /var/lib/caddy --shell /sbin/nologin -c "Caddy web server" caddy 2>/dev/null || true
mkdir -p /etc/caddy /var/lib/caddy /var/log/caddy
cat <<'EOF' > /etc/systemd/system/caddy.service
[Unit]
Description=Caddy Web Server
After=network.target network-online.target
Requires=network-online.target

[Service]
Type=notify
User=caddy
Group=caddy
ExecStart=/usr/bin/caddy run --environ --config /etc/caddy/Caddyfile
ExecReload=/usr/bin/caddy reload --config /etc/caddy/Caddyfile
TimeoutStopSec=5s
LimitNOFILE=1048576
PrivateTmp=true
ProtectSystem=full
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
`
			if !system.RunMultiStepOrAbort("Installing standalone Caddy binary and service", installScript) {
				return
			}
		}
	}

	// Universal firewall hardening (UFW, Firewalld, raw iptables/ip6tables, rules persistence)
	system.ConfigureFirewallRules("80", "443")

	modules.EnsureValidCaddyfile()
	_ = os.MkdirAll("/var/www/html", 0755)
	landingHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
		ServiceName: "Caddy Web Server",
		LogoText:    "C",
		HeroTitle:   "Caddy Web Server Active",
		HeroDesc:    "Your enterprise-grade, memory-safe web server with automated HTTPS is fully operational and tuned.",
		EnvType:     "native",
		Port:        "80",
		WebDir:      "/var/www/html",
		ConfigPath:  "/etc/caddy/Caddyfile",
		WebUser:     "caddy",
		ManagerCmd:  "caddy-manager",
	})
	docRoots := []string{"/var/www/html", "/usr/share/caddy"}
	for _, d := range docRoots {
		if system.IsSymlinkPath(d) {
			continue
		}
		_ = os.MkdirAll(d, 0755)
		system.SecureWebRootPermissions(d, "caddy")
		idxPath := filepath.Join(d, "index.html")
		if _, err := os.Lstat(idxPath); os.IsNotExist(err) || core.OverwriteExistingConfig {
			_ = system.WriteFileNoFollow(idxPath, []byte(landingHTML), 0644, "caddy")
		}
	}
	modules.EnsureCaddyWorking()

	if !system.StartService("caddy") {
		_ = system.RestartService("caddy")
	}

	system.SaveState(map[string]string{
		"ENV_TYPE": "native",
	})
	system.CreateGlobalShortcut()

	tui.EnterAltScreen()
	var finalLines []string
	finalLines = append(finalLines, tui.FormatKV("Deployment", "Native Linux Service", core.GREEN))
	finalLines = append(finalLines, tui.FormatKV("Status", "Installed & Configured", core.GREEN+core.BOLD))
	finalLines = append(finalLines, "")
	finalLines = append(finalLines, tui.FormatKV("To manage, type", "caddy-manager", core.GREEN+core.BOLD))

	tui.PrintInBox("CADDY Native Stack Installed", finalLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Installation complete!")
}

// ==============================================================================

// ==============================================================================
// LOG INSPECTOR & GRAPHICAL TERMINAL VIEWER ENGINE
// ==============================================================================

// LogFileTarget describes a single log source for the log viewer: either a file
// path (IsCmd=false) or a shell command whose output is read as log content (IsCmd=true).
type LogFileTarget struct {
	Name  string
	Path  string
	IsCmd bool
}

// getServiceLogTargets returns the list of predefined log sources for this service.

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
