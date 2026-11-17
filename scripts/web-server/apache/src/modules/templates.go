// ==============================================================================
// KROTEK SCRIPT HUB — VIRTUAL HOST PRESETS & TEMPLATE ENGINE
// Service: APACHE (web-server)
// File:    web-server/apache/src/modules/templates.go
//
// Provides battle-tested, pre-configured Apache VirtualHost templates for:
//   1. Node.js / Express / Next.js / NestJS (Reverse Proxy with WebSockets)
//   2. Single Page Applications (React, Vue, Vite SPA with fallback routing)
//   3. PHP-FPM / WordPress / Dynamic Web Applications
//   4. Python / FastAPI / Flask / Django / Gunicorn
//   5. Static HTML / High-Performance Assets
// Also includes automated systemd service generator for backend daemons.
// ==============================================================================

package modules

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"krotek-apache-installer/core"
	"krotek-apache-installer/system"
	"krotek-apache-installer/tui"
)

// PresetType identifies the application architecture.
type PresetType string

const (
	PresetNodeJS PresetType = "nodejs"
	PresetSPA    PresetType = "spa"
	PresetPHP    PresetType = "php"
	PresetPython PresetType = "python"
	PresetStatic PresetType = "static"
)

// VHostPresetDefinition contains metadata and generator logic for an application template.
type VHostPresetDefinition struct {
	Type        PresetType
	Title       string
	Description string
	Badge       string
}

// GetAvailablePresets returns the list of ready-to-deploy application templates.
func GetAvailablePresets() []VHostPresetDefinition {
	return []VHostPresetDefinition{
		{
			Type:        PresetNodeJS,
			Title:       "Node.js / Express / Next.js (Reverse Proxy)",
			Description: "Reverse proxy to localhost:3000 with WebSockets and header forwarding.",
			Badge:       "[NODE/PROXY]",
		},
		{
			Type:        PresetSPA,
			Title:       "React / Vue / Vite SPA (Single Page App)",
			Description: "Static document root with RewriteRule index.html fallback for client-side routing.",
			Badge:       "[REACT/SPA]",
		},
		{
			Type:        PresetPHP,
			Title:       "PHP / WordPress / Dynamic Web App",
			Description: "Document root with DirectoryIndex and FastCGI/mod_php execution.",
			Badge:       "[PHP/LAMP]",
		},
		{
			Type:        PresetPython,
			Title:       "Python / FastAPI / Flask / Django",
			Description: "Reverse proxy to Gunicorn/Uvicorn on localhost:8000 with proxy headers.",
			Badge:       "[PYTHON]",
		},
		{
			Type:        PresetStatic,
			Title:       "Static HTML / High Performance Website",
			Description: "Pure static file serving with DirectoryIndex and access controls.",
			Badge:       "[STATIC]",
		},
	}
}

// PromptVHostPreset lets the operator select which application preset to deploy.
func PromptVHostPreset() (PresetType, bool) {
	tui.ClearScreen()
	presets := GetAvailablePresets()

	var cardLines []string
	cardLines = append(cardLines, core.BOLD+core.CYAN+"Select VirtualHost Architecture Preset"+core.NC)
	cardLines = append(cardLines, "")
	cardLines = append(cardLines, "  Choose the workload template optimized for your application stack:")
	cardLines = append(cardLines, "")

	var choices []string
	for i, p := range presets {
		cardLines = append(cardLines, fmt.Sprintf("  %s%s %-42s%s", core.BOLD, p.Badge, p.Title, core.NC))
		cardLines = append(cardLines, fmt.Sprintf("      %s%s%s", core.DIM, p.Description, core.NC))
		cardLines = append(cardLines, "")
		choices = append(choices, fmt.Sprintf("%d. %s", i+1, p.Title))
	}

	idx, _, status := tui.SelectMenuWithContent("VirtualHost Application Preset", cardLines, choices)
	if status != 0 {
		return "", false
	}
	return presets[idx].Type, true
}

// GenerateTemplateVHost builds a production-ready Apache VirtualHost configuration string.
func GenerateTemplateVHost(domain string, preset PresetType, targetOrPath string, sslStrat system.SSLStrategy) string {
	logDir := "${APACHE_LOG_DIR}"
	if core.SysInfo.Family != "debian" {
		logDir = "/var/log/httpd"
	}

	hasSSL := sslStrat.Mode == "custom" || (sslStrat.Mode == "certbot" && sslStrat.CertPath != "")

	// 1. Build HTTP (Port 80) configuration
	var httpBlock strings.Builder
	httpBlock.WriteString(fmt.Sprintf("<VirtualHost *:80>\n"))
	httpBlock.WriteString(fmt.Sprintf("    ServerName %s\n", domain))
	httpBlock.WriteString(fmt.Sprintf("    ServerAlias www.%s\n\n", domain))

	if hasSSL {
		// If SSL is active, HTTP 80 redirects to HTTPS 443
		httpBlock.WriteString(fmt.Sprintf("    RewriteEngine On\n"))
		httpBlock.WriteString(fmt.Sprintf("    RewriteCond %%{HTTPS} off\n"))
		httpBlock.WriteString(fmt.Sprintf("    RewriteRule ^ https://%%{HTTP_HOST}%%{REQUEST_URI} [L,R=301]\n\n"))
		httpBlock.WriteString(fmt.Sprintf("    ErrorLog %s/%s_error.log\n", logDir, domain))
		httpBlock.WriteString(fmt.Sprintf("    CustomLog %s/%s_access.log combined\n", logDir, domain))
		httpBlock.WriteString("</VirtualHost>\n\n")
	}

	// 2. Build Primary VirtualHost block (Port 80 if no SSL, or Port 443 if SSL)
	vhostPort := "80"
	if hasSSL {
		vhostPort = "443"
	}

	var primary strings.Builder
	primary.WriteString(fmt.Sprintf("<VirtualHost *:%s>\n", vhostPort))
	primary.WriteString(fmt.Sprintf("    ServerName %s\n", domain))
	primary.WriteString(fmt.Sprintf("    ServerAlias www.%s\n\n", domain))

	if hasSSL {
		primary.WriteString(fmt.Sprintf("    SSLEngine on\n"))
		primary.WriteString(fmt.Sprintf("    SSLCertificateFile %s\n", sslStrat.CertPath))
		primary.WriteString(fmt.Sprintf("    SSLCertificateKeyFile %s\n\n", sslStrat.KeyPath))
	}

	switch preset {
	case PresetNodeJS, PresetPython:
		backendURL := strings.TrimSpace(targetOrPath)
		if backendURL == "" {
			if preset == PresetNodeJS {
				backendURL = "http://127.0.0.1:3000"
			} else {
				backendURL = "http://127.0.0.1:8000"
			}
		}
		if !strings.HasPrefix(backendURL, "http://") && !strings.HasPrefix(backendURL, "https://") {
			backendURL = "http://" + backendURL
		}
		backendURL = strings.TrimRight(backendURL, "/")
		if !system.ValidateProxyTarget(backendURL) {
			return ""
		}

		primary.WriteString(fmt.Sprintf("    ProxyPreserveHost On\n"))
		primary.WriteString(fmt.Sprintf("    ProxyRequests Off\n"))
		primary.WriteString(fmt.Sprintf("    RequestHeader set X-Forwarded-Proto expr=%%{REQUEST_SCHEME}\n"))
		primary.WriteString(fmt.Sprintf("    RequestHeader set X-Real-IP expr=%%{REMOTE_ADDR}\n\n"))
		primary.WriteString(fmt.Sprintf("    ProxyPass / %s/\n", backendURL))
		primary.WriteString(fmt.Sprintf("    ProxyPassReverse / %s/\n\n", backendURL))

	case PresetSPA:
		docRoot := targetOrPath
		if docRoot == "" {
			docRoot = "/var/www/" + domain
		}
		primary.WriteString(fmt.Sprintf("    DocumentRoot %s\n\n", docRoot))
		primary.WriteString(fmt.Sprintf("    <Directory %s>\n", docRoot))
		primary.WriteString(fmt.Sprintf("        Options -Indexes +FollowSymLinks\n"))
		primary.WriteString(fmt.Sprintf("        AllowOverride All\n"))
		primary.WriteString(fmt.Sprintf("        Require all granted\n\n"))
		primary.WriteString(fmt.Sprintf("        RewriteEngine On\n"))
		primary.WriteString(fmt.Sprintf("        RewriteBase /\n"))
		primary.WriteString(fmt.Sprintf("        RewriteRule ^index\\.html$ - [L]\n"))
		primary.WriteString(fmt.Sprintf("        RewriteCond %%{REQUEST_FILENAME} !-f\n"))
		primary.WriteString(fmt.Sprintf("        RewriteCond %%{REQUEST_FILENAME} !-d\n"))
		primary.WriteString(fmt.Sprintf("        RewriteRule . /index.html [L]\n"))
		primary.WriteString(fmt.Sprintf("    </Directory>\n\n"))

	case PresetPHP:
		docRoot := targetOrPath
		if docRoot == "" {
			docRoot = "/var/www/" + domain
		}
		primary.WriteString(fmt.Sprintf("    DocumentRoot %s\n", docRoot))
		primary.WriteString(fmt.Sprintf("    DirectoryIndex index.php index.html\n\n"))
		primary.WriteString(fmt.Sprintf("    <Directory %s>\n", docRoot))
		primary.WriteString(fmt.Sprintf("        Options -Indexes +FollowSymLinks\n"))
		primary.WriteString(fmt.Sprintf("        AllowOverride All\n"))
		primary.WriteString(fmt.Sprintf("        Require all granted\n"))
		primary.WriteString(fmt.Sprintf("    </Directory>\n\n"))

	case PresetStatic:
		fallthrough
	default:
		docRoot := targetOrPath
		if docRoot == "" {
			docRoot = "/var/www/" + domain
		}
		primary.WriteString(fmt.Sprintf("    DocumentRoot %s\n", docRoot))
		primary.WriteString(fmt.Sprintf("    DirectoryIndex index.html\n\n"))
		primary.WriteString(fmt.Sprintf("    <Directory %s>\n", docRoot))
		primary.WriteString(fmt.Sprintf("        Options -Indexes +FollowSymLinks\n"))
		primary.WriteString(fmt.Sprintf("        AllowOverride All\n"))
		primary.WriteString(fmt.Sprintf("        Require all granted\n"))
		primary.WriteString(fmt.Sprintf("    </Directory>\n\n"))
	}

	primary.WriteString(fmt.Sprintf("    ErrorLog %s/%s_error.log\n", logDir, domain))
	primary.WriteString(fmt.Sprintf("    CustomLog %s/%s_access.log combined\n", logDir, domain))
	primary.WriteString("</VirtualHost>\n")

	if hasSSL {
		return httpBlock.String() + primary.String()
	}
	return primary.String()
}

// GenerateSystemdServiceWizard creates a systemd service unit for background backend applications.
func GenerateSystemdServiceWizard() {
	tui.ClearScreen()
	headerLines := []string{
		core.BOLD + core.CYAN + "Systemd Service Generator (24/7 Daemon)" + core.NC,
		"",
		"  Automatically generate and register a systemd service to keep your",
		"  Node.js, Python, or binary backend running continuously in the background.",
	}
	tui.PrintInBox("--no-wait", "Systemd Service Creator", headerLines)

	appName, st := tui.ReadTextInput("Service Name (e.g. my-app)", "my-app")
	if st != 0 || strings.TrimSpace(appName) == "" {
		return
	}
	appName = strings.ToLower(strings.TrimSpace(appName))
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(appName) {
		tui.PrintInBox("Validation Error", []string{core.RED + "Invalid service name. Letters, digits, hyphens, and underscores only." + core.NC})
		return
	}

	workDir, st := tui.ReadTextInput("Working Directory (Project Root)", "/var/www/"+appName)
	if st != 0 || strings.TrimSpace(workDir) == "" {
		return
	}

	execCmd, st := tui.ReadTextInput("ExecStart Command (e.g. /usr/bin/node server.js)", "/usr/bin/node index.js")
	if st != 0 || strings.TrimSpace(execCmd) == "" {
		return
	}

	appUser, st := tui.ReadTextInput("Run As User", "www-data")
	if st != 0 || strings.TrimSpace(appUser) == "" {
		appUser = "www-data"
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(appUser) {
		tui.PrintInBox("Validation Error", []string{core.RED + "Invalid run-as user name. Letters, digits, hyphens, and underscores only." + core.NC})
		return
	}
	if strings.ContainsAny(workDir, "\r\n") || strings.ContainsAny(execCmd, "\r\n") {
		tui.PrintInBox("Validation Error", []string{core.RED + "Directory and command cannot contain newline characters." + core.NC})
		return
	}

	isOpenRC := core.SysInfo.Family == "alpine" || (!system.CommandExists("systemctl") && system.CommandExists("rc-service"))

	if isOpenRC {
		parts := strings.Fields(execCmd)
		binCmd := execCmd
		argsCmd := ""
		if len(parts) > 1 {
			binCmd = parts[0]
			argsCmd = strings.Join(parts[1:], " ")
		}

		rcContent := fmt.Sprintf(`#!/sbin/openrc-run

name="%s"
description="%s Application Daemon"

directory="%s"
command="%s"
command_args="%s"
command_user="%s"
supervisor="supervise-daemon"
respawn_delay=5
respawn_max=0

export NODE_ENV="production"
export PORT="3000"
export PATH="/usr/local/bin:/usr/bin:/bin"

depend() {
	need net
}
`, appName, appName, workDir, binCmd, argsCmd, appUser)

		initPath := fmt.Sprintf("/etc/init.d/%s", appName)
		if err := system.WriteFileNoFollow(initPath, []byte(rcContent), 0755, ""); err != nil {
			tui.PrintInBox("Error", []string{core.RED + "Failed to write OpenRC script: " + err.Error() + core.NC})
			return
		}

		system.RunStep(fmt.Sprintf("Enabling %s in OpenRC default runlevel", appName), "rc-update", "add", appName, "default")
		system.RunStep(fmt.Sprintf("Starting %s OpenRC service", appName), "rc-service", appName, "restart")

		tui.ClearScreen()
		successLines := []string{
			tui.FormatKV("Init System", "OpenRC (Alpine Linux)", core.PURPLE+core.BOLD),
			tui.FormatKV("Service Name", appName, core.CYAN),
			tui.FormatKV("Init Script", initPath, core.YELLOW),
			tui.FormatKV("Status Check", "rc-service "+appName+" status", core.GREEN),
			tui.FormatKV("Restart", "rc-service "+appName+" restart", core.GREEN),
		}
		tui.PrintInBox("OpenRC Service Deployed", successLines)
		time.Sleep(1500 * time.Millisecond)
		return
	}

	unitContent := fmt.Sprintf(`[Unit]
Description=%s Application Daemon
After=network.target

[Service]
Type=simple
User=%s
WorkingDirectory=%s
ExecStart=%s
Restart=always
RestartSec=3
Environment=NODE_ENV=production PORT=3000

[Install]
WantedBy=multi-user.target
`, appName, appUser, workDir, execCmd)

	unitPath := fmt.Sprintf("/etc/systemd/system/%s.service", appName)
	if err := system.WriteFileNoFollow(unitPath, []byte(unitContent), 0644, ""); err != nil {
		tui.PrintInBox("Error", []string{core.RED + "Failed to write service unit: " + err.Error() + core.NC})
		return
	}

	_ = exec.Command("systemctl", "daemon-reload").Run()
	_ = exec.Command("systemctl", "enable", appName).Run()
	_ = exec.Command("systemctl", "start", appName).Run()

	tui.ClearScreen()
	successLines := []string{
		core.BOLD + core.GREEN + "Systemd Service Configured & Started" + core.NC,
		"",
		tui.FormatKV("Service Name", appName+".service", core.CYAN),
		tui.FormatKV("Unit File", unitPath, core.YELLOW),
		tui.FormatKV("Status Check", "systemctl status "+appName, core.GREEN),
		tui.FormatKV("Live Logs", "journalctl -u "+appName+" -f", core.GREEN),
	}
	tui.PrintInBox("Service Deployed", successLines)
	time.Sleep(1500 * time.Millisecond)
}
