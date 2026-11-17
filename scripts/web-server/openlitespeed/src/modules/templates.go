// ==============================================================================
// KROTEK SCRIPT HUB — VIRTUAL HOST PRESETS & TEMPLATE ENGINE
// Service: OPENLITESPEED (web-server)
// File:    web-server/openlitespeed/src/modules/templates.go
//
// Provides battle-tested, pre-configured OpenLiteSpeed VirtualHost templates for:
//   1. Node.js / Express / Next.js / NestJS (Reverse Proxy with WebSockets)
//   2. Single Page Applications (React, Vue, Vite SPA with fallback routing)
//   3. PHP / WordPress / LiteSpeed Cache / Dynamic Web Applications
//   4. Python / FastAPI / Flask / Django / Gunicorn
//   5. Static HTML / High-Performance Assets
// Also includes automated systemd service generator for backend daemons.
// ==============================================================================

package modules

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"krotek-openlitespeed-installer/core"
	"krotek-openlitespeed-installer/system"
	"krotek-openlitespeed-installer/tui"
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
			Description: "Reverse proxy to localhost:3000 with WebSockets and event streaming.",
			Badge:       "[NODE/PROXY]",
		},
		{
			Type:        PresetSPA,
			Title:       "React / Vue / Vite SPA (Single Page App)",
			Description: "Static document root with OpenLiteSpeed rewrite fallback for index.html.",
			Badge:       "[REACT/SPA]",
		},
		{
			Type:        PresetPHP,
			Title:       "PHP / WordPress / Dynamic Web App",
			Description: "Optimized with LiteSpeed SAPI (lsphp) and index.php rewrite rules.",
			Badge:       "[PHP/OLS]",
		},
		{
			Type:        PresetPython,
			Title:       "Python / FastAPI / Flask / Django",
			Description: "Reverse proxy to Gunicorn/Uvicorn on localhost:8000 with proxy handler.",
			Badge:       "[PYTHON]",
		},
		{
			Type:        PresetStatic,
			Title:       "Static HTML / High Performance Website",
			Description: "Pure static file serving with OpenLiteSpeed event engine and Brotli/Gzip.",
			Badge:       "[STATIC]",
		},
	}
}

// PromptVHostPreset displays an interactive selection menu for application templates.
func PromptVHostPreset(domainName string) (VHostPresetDefinition, string, bool) {
	tui.ClearScreen()

	presets := GetAvailablePresets()
	cardLines := []string{
		core.BOLD + core.CYAN + "Application Architecture & VHost Presets" + core.NC,
		"",
		"  Select the application template tailored for " + core.BOLD + domainName + core.NC + ":",
		"",
		"  " + core.BOLD + core.GREEN + "[1] Node.js / Express / Next.js (Reverse Proxy)" + core.NC,
		"      Tailored for Express / Next.js / NestJS running on port 3000.",
		"      Configures external proxy processor and WebSocket upgrade tunneling.",
		"",
		"  " + core.BOLD + core.CYAN + "[2] React / Vue / Vite SPA (Single Page Application)" + core.NC,
		"      Tailored for frontend builds (dist/). Enables rewrite rule fallback to index.html.",
		"",
		"  " + core.BOLD + core.YELLOW + "[3] PHP / WordPress / LiteSpeed Cache" + core.NC,
		"      Native lsphp handler integration with clean URL rewriting.",
		"",
		"  " + core.BOLD + core.PURPLE + "[4] Python / FastAPI / Django / Gunicorn" + core.NC,
		"      Reverse proxy to backend server on port 8000.",
		"",
		"  " + core.BOLD + core.WHITE + "[5] Static HTML / Plain Website" + core.NC,
		"      High-performance static file serving with gzip & brotli compression.",
	}

	choices := []string{
		"1. Node.js / Express / Next.js (Reverse Proxy on port 3000)",
		"2. React / Vue / Vite SPA (Single Page Application)",
		"3. PHP / WordPress / Dynamic App",
		"4. Python / FastAPI / Django / Gunicorn (Port 8000)",
		"5. Static HTML / Plain Website",
	}

	idx, _, status := tui.SelectMenuWithContent("Choose VHost Template", cardLines, choices)
	if status != 0 {
		return VHostPresetDefinition{}, "", false
	}

	chosen := presets[idx]
	targetOrPath := ""

	tui.ClearScreen()
	switch chosen.Type {
	case PresetNodeJS:
		input, st := tui.ReadTextInput("Backend Proxy URL & Port", "http://127.0.0.1:3000")
		if st != 0 || input == "" {
			return chosen, "", false
		}
		if !system.ValidateProxyTarget(input) {
			tui.PrintInBox("Invalid Proxy Target", []string{
				core.RED + "Proxy URL must be http:// or https:// without whitespace, embedded credentials, metadata IPs, or unsafe characters." + core.NC,
			})
			return chosen, "", false
		}
		targetOrPath = strings.TrimSpace(input)

	case PresetPython:
		input, st := tui.ReadTextInput("Backend Proxy URL & Port", "http://127.0.0.1:8000")
		if st != 0 || input == "" {
			return chosen, "", false
		}
		if !system.ValidateProxyTarget(input) {
			tui.PrintInBox("Invalid Proxy Target", []string{
				core.RED + "Proxy URL must be http:// or https:// without whitespace, embedded credentials, metadata IPs, or unsafe characters." + core.NC,
			})
			return chosen, "", false
		}
		targetOrPath = strings.TrimSpace(input)

	case PresetSPA:
		defaultDir := fmt.Sprintf("/var/www/%s/dist", domainName)
		if _, err := os.Stat(fmt.Sprintf("/var/www/%s", domainName)); err == nil && func() bool { _, e := os.Stat(defaultDir); return os.IsNotExist(e) }() {
			defaultDir = fmt.Sprintf("/var/www/%s", domainName)
		}
		input, st := tui.ReadTextInput("Frontend Build Directory (Document Root)", defaultDir)
		if st != 0 || input == "" {
			return chosen, "", false
		}
		input = strings.TrimSpace(input)
		if err := system.ValidateSafeDirectoryPath(input); err != nil {
			tui.PrintInBox("Invalid Document Root", []string{
				core.RED + err.Error() + core.NC,
			})
			return chosen, "", false
		}
		targetOrPath = input

	case PresetPHP, PresetStatic:
		defaultDir := fmt.Sprintf("/var/www/%s", domainName)
		input, st := tui.ReadTextInput("Web Root Directory (Document Root)", defaultDir)
		if st != 0 || input == "" {
			return chosen, "", false
		}
		input = strings.TrimSpace(input)
		if err := system.ValidateSafeDirectoryPath(input); err != nil {
			tui.PrintInBox("Invalid Document Root", []string{
				core.RED + err.Error() + core.NC,
			})
			return chosen, "", false
		}
		targetOrPath = input
	}

	return chosen, targetOrPath, true
}

// PromptOpenLiteSpeedSSLStrategy lets operator choose SSL certificate approach.
func PromptOpenLiteSpeedSSLStrategy(domainName string) (string, string, string) {
	tui.ClearScreen()
	cardLines := []string{
		core.BOLD + core.CYAN + "SSL / TLS Encryption Strategy" + core.NC,
		"",
		"  Choose how SSL/TLS encryption should be handled for " + core.BOLD + domainName + core.NC + ":",
		"",
		"  " + core.BOLD + core.GREEN + "[1] None / HTTP Only (or Behind Cloudflare Proxy)" + core.NC,
		"      Bypasses local vhost SSL. Ideal if running behind Cloudflare Flexible or reverse proxy.",
		"",
		"  " + core.BOLD + core.CYAN + "[2] Custom SSL Certificate & Private Key" + core.NC,
		"      Specify pre-existing certificate files (e.g. from Let's Encrypt / Certbot / OVH / Cloudflare Origin).",
	}

	choices := []string{
		"1. None / HTTP Only (Behind Cloudflare or Plain HTTP)",
		"2. Custom SSL Certificate & Private Key",
	}

	idx, _, status := tui.SelectMenuWithContent("OpenLiteSpeed SSL Strategy", cardLines, choices)
	if status != 0 || idx == 0 {
		return "none", "", ""
	}

	// Custom Certificate paths
	tui.ClearScreen()
	defaultCert := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", domainName)
	if _, err := os.Stat(defaultCert); os.IsNotExist(err) {
		defaultCert = fmt.Sprintf("/etc/ssl/certs/%s.crt", domainName)
	}
	certPath, st1 := tui.ReadTextInput("Path to Fullchain Certificate (.crt / .pem)", defaultCert)
	if st1 != 0 || certPath == "" {
		return "none", "", ""
	}

	defaultKey := fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", domainName)
	if _, err := os.Stat(defaultKey); os.IsNotExist(err) {
		defaultKey = fmt.Sprintf("/etc/ssl/private/%s.key", domainName)
	}
	keyPath, st2 := tui.ReadTextInput("Path to Private Key (.key / .pem)", defaultKey)
	if st2 != 0 || keyPath == "" {
		return "none", "", ""
	}

	return "custom", certPath, keyPath
}

func extractHostPort(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			return u.Host
		}
	}
	return strings.TrimPrefix(strings.TrimPrefix(raw, "http://"), "https://")
}

// GenerateOpenLiteSpeedVHost generates the vhconf.conf configuration for OpenLiteSpeed.
func GenerateOpenLiteSpeedVHost(preset VHostPresetDefinition, domainName, targetOrPath, sslMode, certPath, keyPath string) string {
	sslBlock := ""
	if sslMode == "custom" && certPath != "" && keyPath != "" {
		sslBlock = fmt.Sprintf(`
vhssl {
  keyFile %s
  certFile %s
  certChain 1
  sslProtocol 30
}
`, keyPath, certPath)
	}

	switch preset.Type {
	case PresetNodeJS, PresetPython:
		if !system.ValidateProxyTarget(targetOrPath) {
			return ""
		}
		backendAddr := extractHostPort(targetOrPath)
		if backendAddr == "" || strings.ContainsAny(backendAddr, "\r\n\t ;\"'{}()$") {
			return ""
		}
		safeID := strings.ReplaceAll(domainName, ".", "_") + "_backend"
		return fmt.Sprintf(`docRoot /var/www/%s
vhDomain %s
enableGzip 1
enableBr 1

extprocessor %s {
  type proxy
  address %s
  maxConns 100
  pcKeepAliveTimeout 60
  initTimeout 60
  retryTimeout 0
  respBuffer 0
}

context / {
  type proxy
  handler %s
  addDefaultCharset off
}

websocket {
  uri /
  address %s
}
%s`, domainName, domainName, safeID, backendAddr, safeID, backendAddr, sslBlock)

	case PresetSPA:
		return fmt.Sprintf(`docRoot %s
vhDomain %s
enableGzip 1
enableBr 1

index {
  useServer 0
  indexFiles index.html
  autoIndex 0
}

context / {
  location %s
  allowBrowse 1
  rewrite {
    enable 1
    autoLoadHtaccess 1
    rules <<<END_rules
RewriteEngine On
RewriteCond %%{REQUEST_FILENAME} !-f
RewriteCond %%{REQUEST_FILENAME} !-d
RewriteRule ^(.*)$ /index.html [L]
END_rules
  }
}
%s`, targetOrPath, domainName, targetOrPath, sslBlock)

	case PresetPHP:
		return fmt.Sprintf(`docRoot %s
vhDomain %s
enableGzip 1
enableBr 1

index {
  useServer 0
  indexFiles index.php, index.html
  autoIndex 0
}

context / {
  location %s
  allowBrowse 1
  rewrite {
    enable 1
    autoLoadHtaccess 1
    rules <<<END_rules
RewriteEngine On
RewriteBase /
RewriteRule ^index\.php$ - [L]
RewriteCond %%{REQUEST_FILENAME} !-f
RewriteCond %%{REQUEST_FILENAME} !-d
RewriteRule . /index.php [L]
END_rules
  }
}

scripthandler {
  add ls:lsphp81 php
}
%s`, targetOrPath, domainName, targetOrPath, sslBlock)

	case PresetStatic:
		return fmt.Sprintf(`docRoot %s
vhDomain %s
enableGzip 1
enableBr 1

index {
  useServer 0
  indexFiles index.html, index.htm
  autoIndex 0
}

context / {
  location %s
  allowBrowse 1
}
%s`, targetOrPath, domainName, targetOrPath, sslBlock)
	}

	return fmt.Sprintf(`docRoot %s
vhDomain %s
enableGzip 1
%s`, targetOrPath, domainName, sslBlock)
}

// RegisterVHostInOLS updates /usr/local/lsws/conf/httpd_config.conf to add the virtual host and listener mapping.
func RegisterVHostInOLS(domainName string) {
	mainConf := "/usr/local/lsws/conf/httpd_config.conf"
	data, err := os.ReadFile(mainConf)
	if err != nil {
		return
	}
	content := string(data)

	// Check if already registered
	vhostToken := fmt.Sprintf("virtualHost %s {", domainName)
	if !strings.Contains(content, vhostToken) {
		vhostEntry := fmt.Sprintf(`
virtualHost %s {
  vhRoot /usr/local/lsws/conf/vhosts/%s
  configFile /usr/local/lsws/conf/vhosts/%s/vhconf.conf
  allowSymbolLink 1
  enableScript 1
  restrained 0
}
`, domainName, domainName, domainName)

		content += vhostEntry

		// Add listener mapping if listener Default or HTTP exists
		mapDirective := fmt.Sprintf("  map %s %s, www.%s\n", domainName, domainName, domainName)
		if idx := strings.Index(content, "listener Default {"); idx != -1 {
			if endIdx := strings.Index(content[idx:], "}"); endIdx != -1 {
				insertPos := idx + endIdx
				content = content[:insertPos] + mapDirective + content[insertPos:]
			}
		}

		_ = system.WriteFileNoFollow(mainConf, []byte(content), 0644, "")
	}
}

// GenerateSystemdServiceWizard provides an interactive guided setup to daemonize backend applications.
func GenerateSystemdServiceWizard() {
	tui.ClearScreen()

	cardLines := []string{
		core.BOLD + core.CYAN + "Background Service Generator (systemd)" + core.NC,
		"",
		"  Automatically generate, enable, and start a permanent background service",
		"  for your backend application (Node.js, Next.js, Express, Python, or Go).",
		"",
		"  " + core.BOLD + "Benefits:" + core.NC,
		"  " + core.Sym.Bullet + " Keeps app running 24/7 in the background",
		"  " + core.Sym.Bullet + " Automatically restarts on failure or server reboot",
		"  " + core.Sym.Bullet + " Fixes " + core.RED + "502 Bad Gateway" + core.NC + " by ensuring backend process is active",
	}
	tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel & Return to Menu", "systemd Generator", cardLines)

	svcName, st1 := tui.ReadTextInput("Service Name (systemd identifier)", "krotek-app")
	if st1 != 0 || svcName == "" {
		return
	}
	svcName = strings.TrimSuffix(strings.TrimSpace(svcName), ".service")
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(svcName) {
		tui.PrintInBox("Validation Error", []string{core.RED + "Invalid service name. Letters, digits, hyphens, and underscores only." + core.NC})
		return
	}

	defaultDir := "/var/www"
	workDir, st2 := tui.ReadTextInput("Application Working Directory", defaultDir)
	if st2 != 0 || workDir == "" {
		return
	}

	execCmd, st3 := tui.ReadTextInput("Start Command (ExecStart)", "/usr/bin/npm run start")
	if st3 != 0 || execCmd == "" {
		return
	}

	portIn, _ := tui.ReadTextInput("Application PORT Environment Variable", "3000")
	if portIn == "" {
		portIn = "3000"
	}
	portNum, pErr := strconv.Atoi(strings.TrimSpace(portIn))
	if pErr != nil || portNum < 1 || portNum > 65535 {
		tui.PrintInBox("Validation Error", []string{core.RED + "Invalid application port number. Must be between 1 and 65535." + core.NC})
		return
	}
	portIn = strconv.Itoa(portNum)

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
description="%s Backend Daemon"

directory="%s"
command="%s"
command_args="%s"
command_user="www-data"
supervisor="supervise-daemon"
respawn_delay=5
respawn_max=0

export NODE_ENV="production"
export PORT="%s"
export PATH="/usr/local/bin:/usr/bin:/bin"

depend() {
	need net
}
`, svcName, svcName, workDir, binCmd, argsCmd, portIn)

		initPath := fmt.Sprintf("/etc/init.d/%s", svcName)
		if err := system.WriteFileNoFollow(initPath, []byte(rcContent), 0755, ""); err != nil {
			tui.PrintInBox("Error Writing Service", []string{core.RED + "Failed to write OpenRC script: " + err.Error() + core.NC})
			return
		}

		system.RunStep(fmt.Sprintf("Enabling %s in OpenRC default runlevel", svcName), "rc-update", "add", svcName, "default")
		system.RunStep(fmt.Sprintf("Starting %s OpenRC service", svcName), "rc-service", svcName, "restart")

		tui.ClearScreen()
		successLines := []string{
			tui.FormatKV("Init System", "OpenRC (Alpine Linux)", core.PURPLE+core.BOLD),
			tui.FormatKV("Service Name", svcName, core.CYAN),
			tui.FormatKV("Init Script", initPath, core.YELLOW),
			tui.FormatKV("Status Check", "rc-service "+svcName+" status", core.GREEN),
			tui.FormatKV("Restart", "rc-service "+svcName+" restart", core.GREEN),
			tui.FormatKV("Port", portIn, core.GREEN),
		}
		tui.PrintInBox("OpenRC Service Deployed", successLines)
		time.Sleep(1500 * time.Millisecond)
		return
	}

	unitContent := fmt.Sprintf(`[Unit]
Description=%s Backend Daemon
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=%s
ExecStart=%s
Restart=always
RestartSec=3
Environment=PORT=%s
Environment=NODE_ENV=production
Environment=PATH=/usr/local/bin:/usr/bin:/bin

[Install]
WantedBy=multi-user.target
`, svcName, workDir, execCmd, portIn)

	unitPath := fmt.Sprintf("/etc/systemd/system/%s.service", svcName)
	if err := system.WriteFileNoFollow(unitPath, []byte(unitContent), 0644, ""); err != nil {
		tui.PrintInBox("Error Writing Service", []string{core.RED + "Failed to write systemd unit file: " + err.Error() + core.NC})
		return
	}

	system.RunStep("Reloading systemd daemon", "systemctl", "daemon-reload")
	system.RunStep(fmt.Sprintf("Enabling %s service", svcName), "systemctl", "enable", svcName)
	system.RunStep(fmt.Sprintf("Starting %s service", svcName), "systemctl", "restart", svcName)

	tui.ClearScreen()
	successLines := []string{
		tui.FormatKV("Service Name", svcName+".service", core.CYAN),
		tui.FormatKV("Unit File", unitPath, core.YELLOW),
		tui.FormatKV("Working Directory", workDir, core.PURPLE),
		tui.FormatKV("Exec Command", execCmd, core.PURPLE),
		tui.FormatKV("Port", portIn, core.GREEN),
		"",
		tui.FormatKV("Status Check", "systemctl status "+svcName, core.GREEN),
		tui.FormatKV("View Logs", "journalctl -u "+svcName+" -f", core.GREEN),
		tui.FormatKV("Restart", "systemctl restart "+svcName, core.GREEN),
	}
	tui.PrintInBox("Background Service Deployed", successLines)
	time.Sleep(1500 * time.Millisecond)
}
