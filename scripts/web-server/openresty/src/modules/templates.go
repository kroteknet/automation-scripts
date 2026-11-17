// ==============================================================================
// KROTEK SCRIPT HUB — VIRTUAL HOST PRESETS & TEMPLATE ENGINE
// Service: OpenResty (web-server)
// File:    web-server/openresty/src/modules/templates.go
//
// Provides battle-tested, pre-configured OpenResty VirtualHost templates for:
//   1. Node.js / Express / Next.js / NestJS (Reverse Proxy with WebSockets)
//   2. Single Page Applications (React, Vue, Vite SPA with fallback routing)
//   3. PHP-FPM / WordPress / Dynamic Web Applications
//   4. Python / FastAPI / Flask / Django / Gunicorn
//   5. Static HTML / High-Performance Assets
// Also includes automated systemd/OpenRC service generator for backend daemons.
// ==============================================================================

package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"krotek-openresty-installer/core"
	"krotek-openresty-installer/system"
	"krotek-openresty-installer/tui"
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
			Description: "Reverse proxy to localhost:3000 with WebSockets, streaming & 100MB upload limits.",
			Badge:       "[NODE/PROXY]",
		},
		{
			Type:        PresetSPA,
			Title:       "React / Vue / Vite SPA (Single Page App)",
			Description: "Static document root with index.html fallback, 1y asset caching & no-cache HTML.",
			Badge:       "[REACT/SPA]",
		},
		{
			Type:        PresetPHP,
			Title:       "PHP-FPM / WordPress / Dynamic App",
			Description: "FastCGI unix socket integration with security rules for uploads and hidden files.",
			Badge:       "[PHP-FPM]",
		},
		{
			Type:        PresetPython,
			Title:       "Python / FastAPI / Flask / Django",
			Description: "Reverse proxy to Gunicorn/Uvicorn on localhost:8000 with streaming buffers.",
			Badge:       "[PYTHON]",
		},
		{
			Type:        PresetStatic,
			Title:       "Static HTML / High Performance Website",
			Description: "Pure static file serving with gzip compression, security headers and 404 handler.",
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
		"      Tailored for server.ts / Express / Next.js / NestJS running on port 3000.",
		"      Includes WebSocket upgrades, 100MB body size, and unbuffered streaming.",
		"",
		"  " + core.BOLD + core.CYAN + "[2] React / Vue / Vite SPA (Single Page Application)" + core.NC,
		"      Tailored for frontend builds (dist/). Enables try_files $uri /index.html,",
		"      fingerprinted 1-year asset cache, and instant cache-busting for index.html.",
		"",
		"  " + core.BOLD + core.YELLOW + "[3] PHP-FPM / WordPress / Laravel" + core.NC,
		"      Configures FastCGI socket, index.php routing, and security deny rules.",
		"",
		"  " + core.BOLD + core.PURPLE + "[4] Python / FastAPI / Django / Gunicorn" + core.NC,
		"      Reverse proxy to backend server on port 8000 with proxy headers.",
		"",
		"  " + core.BOLD + core.WHITE + "[5] Static HTML / Plain Website" + core.NC,
		"      High-performance static file serving with Gzip compression.",
	}

	choices := []string{
		"1. Node.js / Express / Next.js (Reverse Proxy on port 3000)",
		"2. React / Vue / Vite SPA (Single Page Application)",
		"3. PHP-FPM / WordPress / Dynamic App",
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
		input = strings.TrimSpace(input)
		if !system.ValidateProxyTarget(input) {
			tui.PrintInBox("Validation Error", []string{
				core.RED + "Invalid backend proxy URL. Must use http:// or https:// without whitespace or injection characters." + core.NC,
			})
			return chosen, "", false
		}
		targetOrPath = input

	case PresetPython:
		input, st := tui.ReadTextInput("Backend Proxy URL & Port", "http://127.0.0.1:8000")
		if st != 0 || input == "" {
			return chosen, "", false
		}
		input = strings.TrimSpace(input)
		if !system.ValidateProxyTarget(input) {
			tui.PrintInBox("Validation Error", []string{
				core.RED + "Invalid backend proxy URL. Must use http:// or https:// without whitespace or injection characters." + core.NC,
			})
			return chosen, "", false
		}
		targetOrPath = input

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

// GenerateTemplateVHost builds the OpenResty configuration based on preset and SSL strategy.
func GenerateTemplateVHost(preset VHostPresetDefinition, domainName, targetOrPath string, ssl system.SSLStrategy) string {
	cleanDom := system.CleanDomain(domainName)
	if system.ValidateDomainName(cleanDom) {
		domainName = cleanDom
	} else {
		domainName = "localhost"
	}
	if preset.Type == PresetNodeJS || preset.Type == PresetPython {
		if !system.ValidateProxyTarget(targetOrPath) {
			if preset.Type == PresetPython {
				targetOrPath = "http://127.0.0.1:8000"
			} else {
				targetOrPath = "http://127.0.0.1:3000"
			}
		}
	} else if err := system.ValidateSafeDirectoryPath(targetOrPath); err != nil {
		targetOrPath = "/var/www/" + domainName
	}

	extraTopDirectives := ""
	if ssl.Mode == "cloudflare" {
		extraTopDirectives = system.GetCloudflareRealIPBlock() + "\n"
	}

	httpPort := system.GetConfiguredPort()
	httpListen := system.ListenDirectives(httpPort, false, "")
	httpsListen := system.ListenDirectives("443", false, "ssl http2")

	var innerLocationBlocks string
	switch preset.Type {
	case PresetNodeJS, PresetPython:
		innerLocationBlocks = fmt.Sprintf(`    client_max_body_size 100M;

    location / {
        proxy_pass %s;
        proxy_http_version 1.1;

        # WebSocket & Upgrade Headers
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        # Client Identity Headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Streaming & Performance
        proxy_buffering off;
        proxy_read_timeout 300s;
        proxy_connect_timeout 60s;
    }
`, targetOrPath)

	case PresetSPA:
		innerLocationBlocks = fmt.Sprintf(`    root %s;
    index index.html;

    # Single Page App Routing Fallback
    location / {
        try_files $uri $uri/ /index.html;
    }

    # Hashed Assets Long-Term Cache
    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    # SPA Shell No-Cache Rule
    location = /index.html {
        expires -1;
        add_header Cache-Control "no-store, no-cache, must-revalidate";
    }
`, targetOrPath)

	case PresetPHP:
		phpLoc := system.GetPHPFastCGIBlock()
		innerLocationBlocks = fmt.Sprintf(`    root %s;
    index index.php index.html index.htm;

    location / {
        try_files $uri $uri/ /index.php?$args;
    }

%s

    # Security Rules
    location ~ /\.ht {
        deny all;
    }
    location = /favicon.ico {
        log_not_found off;
        access_log off;
    }
    location = /robots.txt {
        log_not_found off;
        access_log off;
    }
`, targetOrPath, phpLoc)

	case PresetStatic:
		innerLocationBlocks = fmt.Sprintf(`    root %s;
    index index.html index.htm;

    location / {
        try_files $uri $uri/ =404;
    }
`, targetOrPath)
	}

	// Build HTTP / HTTPS wrapper
	if ssl.Mode == "custom" {
		return fmt.Sprintf(`server {
%s    server_name %s www.%s;
    return 301 https://$host$request_uri;
}

server {
%s    server_name %s www.%s;

%s
%s
    # Gzip Compression
    gzip on;
    gzip_vary on;
    gzip_proxied any;
    gzip_comp_level 6;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript;

%s}
`, httpListen, domainName, domainName, httpsListen, domainName, domainName, system.GetCustomSSLBlock(ssl.CertPath, ssl.KeyPath), extraTopDirectives, innerLocationBlocks)
	}

	// Plain HTTP (or Certbot / Cloudflare edge mode)
	return fmt.Sprintf(`server {
%s    server_name %s www.%s;

%s
    # Gzip Compression
    gzip on;
    gzip_vary on;
    gzip_proxied any;
    gzip_comp_level 6;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript;

%s}
`, httpListen, domainName, domainName, extraTopDirectives, innerLocationBlocks)
}

// GenerateSystemdServiceWizard provides an interactive guided setup to daemonize backend applications.
func GenerateSystemdServiceWizard() {
	tui.ClearScreen()

	cardLines := []string{
		core.BOLD + core.CYAN + "Background Service Generator (systemd / OpenRC)" + core.NC,
		"",
		"  Automatically generate, enable, and start a permanent background service",
		"  for your backend application (Node.js, Next.js, Express, Python, or Go).",
		"",
		"  " + core.BOLD + "Benefits:" + core.NC,
		"  " + core.Sym.Bullet + " Keeps app running 24/7 in the background",
		"  " + core.Sym.Bullet + " Automatically restarts on failure or server reboot",
		"  " + core.Sym.Bullet + " Fixes " + core.RED + "502 Bad Gateway" + core.NC + " by ensuring backend process is active",
	}
	tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel & Return to Menu", "Service Generator", cardLines)

	svcName, st1 := tui.ReadTextInput("Service Name (systemd/OpenRC identifier)", "krotek-app")
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
	workDir = strings.TrimSpace(workDir)
	if err := system.ValidateSafeDirectoryPath(workDir); err != nil {
		tui.PrintInBox("Validation Error", []string{core.RED + err.Error() + core.NC})
		return
	}
	workDir = filepath.Clean(workDir)

	execCmd, st3 := tui.ReadTextInput("Start Command (ExecStart)", "/usr/bin/npm run start")
	if st3 != 0 || execCmd == "" {
		return
	}
	execCmd = strings.TrimSpace(execCmd)

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

	if strings.ContainsAny(workDir, "\r\n\x00[]") || strings.ContainsAny(execCmd, "\r\n\x00") {
		tui.PrintInBox("Validation Error", []string{core.RED + "Directory and command cannot contain newline or control characters." + core.NC})
		return
	}

	serviceUser := system.GetRuntimeWebUser()
	isOpenRC := core.SysInfo.Family == "alpine" || (!system.CommandExists("systemctl") && system.CommandExists("rc-service"))

	if isOpenRC {
		if strings.ContainsAny(workDir, "\"$`\\;|&<>()'") || strings.ContainsAny(execCmd, "\"$`\\;|&<>()'") {
			tui.PrintInBox("Validation Error", []string{core.RED + "OpenRC directory and command cannot contain quotes or shell metacharacters." + core.NC})
			return
		}
		parts := strings.Fields(execCmd)
		if len(parts) == 0 || !regexp.MustCompile(`^[a-zA-Z0-9_./-]+$`).MatchString(parts[0]) {
			tui.PrintInBox("Validation Error", []string{core.RED + "Invalid OpenRC executable command path." + core.NC})
			return
		}
		binCmd := parts[0]
		argsCmd := ""
		if len(parts) > 1 {
			argsCmd = strings.Join(parts[1:], " ")
		}

		rcContent := fmt.Sprintf(`#!/sbin/openrc-run

name="%s"
description="%s Backend Daemon"

directory="%s"
command="%s"
command_args="%s"
command_user="%s"
supervisor="supervise-daemon"
respawn_delay=5
respawn_max=0

export NODE_ENV="production"
export PORT="%s"
export PATH="/usr/local/bin:/usr/bin:/bin"

depend() {
	need net
}
`, svcName, svcName, workDir, binCmd, argsCmd, serviceUser, portIn)

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
User=%s
WorkingDirectory=%s
ExecStart=%s
Restart=always
RestartSec=3
Environment=PORT=%s
Environment=NODE_ENV=production
Environment=PATH=/usr/local/bin:/usr/bin:/bin

[Install]
WantedBy=multi-user.target
`, svcName, serviceUser, workDir, execCmd, portIn)

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
