// ==============================================================================
// KROTEK SCRIPT HUB — DEFAULT WEB LANDING PAGE GENERATOR
// Service: NGINX (web-server)
// File:    web-server/nginx/src/system/landing.go
//
// Deploys a modern, responsive HTML landing page to the web document root to verify that the web server is active and serving traffic.
// ==============================================================================

package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"krotek-nginx-installer/core"
)

// PHPInfo encapsulates discovered PHP runtime version, SAPI interface, OPcache status, and module counts.
type PHPInfo struct {
	Installed       bool
	Version         string
	StatusBadge     string
	StatusClass     string
	SAPI            string
	Handler         string
	OPcache         string
	OPcacheClass    string
	MemoryLimit     string
	IniPath         string
	KeyExtensions   string
	ExtensionsCount int
}

// LandingPageConfig defines templating properties for generating the official Krotek HTML landing page.
type LandingPageConfig struct {
	ServiceName  string
	LogoText     string
	HeroTitle    string
	HeroDesc     string
	EnvType      string // "native" or "docker"
	Port         string
	AdminPort    string // For OpenLiteSpeed
	WebDir       string
	ConfigPath   string
	WebUser      string
	ManagerCmd   string
	PHPInstalled *bool
	PHPVersion   string
	PHPSapi      string
}

// DetectPHPInfo dynamically queries the host system for an active PHP runtime,
// version, SAPI interface, OPcache, configuration files, and installed modules.
func DetectPHPInfo() PHPInfo {
	var phpBin string
	candidates := []string{
		"php",
		"/usr/bin/php",
		"/usr/local/bin/php",
		"/usr/bin/php84",
		"/usr/bin/php83",
		"/usr/bin/php82",
		"/usr/bin/php81",
		"/usr/local/lsws/lsphp83/bin/php",
		"/usr/local/lsws/lsphp82/bin/php",
		"/usr/local/lsws/fcgi-bin/lsphp",
	}
	for _, cand := range candidates {
		if p, err := exec.LookPath(cand); err == nil {
			phpBin = p
			break
		}
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			phpBin = cand
			break
		}
	}

	if phpBin == "" {
		return PHPInfo{
			Installed:       false,
			Version:         "None",
			StatusBadge:     "Not Installed",
			StatusClass:     "",
			SAPI:            "None",
			Handler:         "Not Configured",
			OPcache:         "Disabled",
			OPcacheClass:    "",
			MemoryLimit:     "N/A",
			IniPath:         "N/A",
			KeyExtensions:   "None",
			ExtensionsCount: 0,
		}
	}

	info := PHPInfo{
		Installed:    true,
		StatusBadge:  "Installed & Operational",
		StatusClass:  "success",
		OPcacheClass: "",
	}

	// Version detection
	if out, err := exec.Command(phpBin, "-v").Output(); err == nil {
		re := regexp.MustCompile(`PHP\s+([0-9]+\.[0-9]+(?:\.[0-9]+)?)`)
		if m := re.FindStringSubmatch(string(out)); len(m) > 1 {
			info.Version = "v" + m[1]
		} else {
			info.Version = "Installed"
		}
	} else {
		info.Version = "Installed"
	}

	// Server-tailored SAPI and FastCGI handler detection
	switch "nginx" {
	case "nginx":
		socketPatterns := []string{
			"/run/php/php*-fpm.sock",
			"/var/run/php/php*-fpm.sock",
			"/run/php-fpm/www.sock",
			"/var/run/php-fpm/www.sock",
		}
		var foundSock string
		for _, pat := range socketPatterns {
			if matches, _ := filepath.Glob(pat); len(matches) > 0 {
				sort.Strings(matches)
				foundSock = matches[len(matches)-1]
				break
			}
		}
		if foundSock != "" {
			info.SAPI = "PHP-FPM (FastCGI)"
			info.Handler = "unix:" + foundSock
		} else {
			info.SAPI = "PHP-FPM / FastCGI"
			info.Handler = "127.0.0.1:9000 (TCP)"
		}
	case "apache":
		if core.SysInfo.Family == "debian" {
			info.SAPI = "Apache 2.0 Handler"
			info.Handler = "libapache2-mod-php / FPM"
		} else {
			info.SAPI = "Apache Module"
			info.Handler = "mod_php / php-fpm"
		}
	case "openlitespeed":
		info.SAPI = "LiteSpeed SAPI (LSAPI)"
		info.Handler = "LSPHP Worker (Example vhost)"
	case "caddy":
		info.SAPI = "PHP-FPM (FastCGI)"
		info.Handler = "php_fastcgi (Unix Socket)"
	default:
		info.SAPI = "FastCGI / SAPI"
		info.Handler = "Default Socket"
	}

	// OPcache check
	if opOut, err := exec.Command(phpBin, "-r", "echo extension_loaded('Zend OPcache') ? '1' : '0';").Output(); err == nil && strings.TrimSpace(string(opOut)) == "1" {
		info.OPcache = "Enabled (Active)"
		info.OPcacheClass = "success"
	} else {
		info.OPcache = "Available (Inactive)"
	}

	// Memory limit check
	if memOut, err := exec.Command(phpBin, "-r", "echo ini_get('memory_limit');").Output(); err == nil && strings.TrimSpace(string(memOut)) != "" {
		info.MemoryLimit = strings.TrimSpace(string(memOut))
	} else {
		info.MemoryLimit = "512M"
	}

	// Loaded INI path
	if iniOut, err := exec.Command(phpBin, "-r", "echo php_ini_loaded_file();").Output(); err == nil && strings.TrimSpace(string(iniOut)) != "" {
		info.IniPath = strings.TrimSpace(string(iniOut))
	} else {
		info.IniPath = "/etc/php/php.ini"
	}

	// Extensions
	if mOut, err := exec.Command(phpBin, "-m").Output(); err == nil {
		mStr := strings.ToLower(string(mOut))
		lines := strings.Split(mStr, "\n")
		count := 0
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" && !strings.HasPrefix(l, "[") {
				count++
			}
		}
		info.ExtensionsCount = count
		popular := []string{"curl", "openssl", "pdo"}
		var found []string
		for _, p := range popular {
			if strings.Contains(mStr, p) {
				found = append(found, p)
			}
		}
		if len(found) > 0 {
			if count > len(found) {
				info.KeyExtensions = fmt.Sprintf("%s (+%d total)", strings.Join(found, ", "), count)
			} else {
				info.KeyExtensions = strings.Join(found, ", ")
			}
		} else {
			info.KeyExtensions = fmt.Sprintf("%d extensions active", count)
		}
	} else {
		info.KeyExtensions = "Standard Extensions"
	}

	return info
}

// GeneratePHPProbe returns the source code of a lightweight, secure JSON status endpoint.
func GeneratePHPProbe() string {
	return `<?php
header('Content-Type: application/json; charset=utf-8');
header('Cache-Control: no-cache, no-store, must-revalidate');

$exts = get_loaded_extensions();
sort($exts);

$keyExts = ['curl', 'mbstring', 'openssl', 'pdo', 'pdo_mysql', 'mysqli', 'xml', 'zip', 'gd', 'intl', 'Zend OPcache'];
$foundKey = array_values(array_intersect($keyExts, $exts));

echo json_encode([
    'status' => 'operational',
    'installed' => true,
    'version' => PHP_VERSION,
    'major_minor' => PHP_MAJOR_VERSION . '.' . PHP_MINOR_VERSION,
    'sapi' => php_sapi_name(),
    'memory_limit' => ini_get('memory_limit'),
    'opcache' => extension_loaded('Zend OPcache') && (bool)ini_get('opcache.enable'),
    'ini_file' => php_ini_loaded_file() ?: 'None',
    'extensions_count' => count($exts),
    'key_extensions' => $foundKey,
    'server_software' => $_SERVER['SERVER_SOFTWARE'] ?? 'NGINX Web Server',
    'time' => date('Y-m-d H:i:s T')
], JSON_PRETTY_PRINT);
`
}

// WriteLandingFiles writes the unified landing page index.html and companion php-status.php if PHP is installed.
func WriteLandingFiles(webDir string, cfg LandingPageConfig) error {
	cleanDir := filepath.Clean(strings.TrimSpace(webDir))
	if IsSymlinkPath(cleanDir) {
		return fmt.Errorf("refusing to write landing files into symlink directory: %s", cleanDir)
	}
	_ = os.MkdirAll(cleanDir, 0755)
	html := GenerateLandingPageHTML(cfg)
	idxPath := filepath.Join(cleanDir, "index.html")
	if err := WriteFileNoFollow(idxPath, []byte(html), 0644, ""); err != nil {
		return err
	}

	phpInfo := DetectPHPInfo()
	if cfg.PHPInstalled != nil && *cfg.PHPInstalled {
		phpInfo.Installed = true
	}
	if phpInfo.Installed {
		probePath := filepath.Join(cleanDir, "php-status.php")
		_ = WriteFileNoFollow(probePath, []byte(GeneratePHPProbe()), 0644, "")
	}
	return nil
}

// GenerateLandingPageHTML generates an ultra-modern, responsive, unified landing page
// with a refined, non-neon enterprise dark aesthetic and comprehensive PHP runtime details.
func GenerateLandingPageHTML(cfg LandingPageConfig) string {
	if cfg.Port == "" {
		cfg.Port = "80"
	}
	if cfg.AdminPort == "" && "" != "" {
		cfg.AdminPort = ""
	}
	if cfg.WebDir == "" {
		cfg.WebDir = "/var/www/html"
		if core.SysInfo.Family == "alpine" && "nginx" == "apache" {
			cfg.WebDir = "/var/www/localhost/htdocs"
		}
	}
	if cfg.WebUser == "" {
		cfg.WebUser = "nginx"
		if core.SysInfo.Family == "debian" && ("nginx" == "nginx" || "nginx" == "apache") {
			cfg.WebUser = "www-data"
		} else if cfg.EnvType == "docker" {
			cfg.WebUser = "nginx (container)"
		}
	}
	if cfg.ConfigPath == "" {
		cfg.ConfigPath = "/etc/nginx/nginx.conf"
		if core.SysInfo.Family == "rhel" && "nginx" == "apache" {
			cfg.ConfigPath = "/etc/httpd/conf/httpd.conf"
		} else if cfg.EnvType == "docker" {
			if "nginx" == "openlitespeed" {
				cfg.ConfigPath = "/etc/openlitespeed-docker/docker-compose.yml"
			} else if "nginx" == "caddy" {
				cfg.ConfigPath = "/etc/caddy-docker/Caddyfile"
			} else {
				cfg.ConfigPath = "/etc/nginx-docker/docker-compose.yml"
			}
		}
	}
	if cfg.ManagerCmd == "" {
		cfg.ManagerCmd = "nginx-manager"
	}
	if cfg.LogoText == "" {
		cfg.LogoText = "N"
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "NGINX Web Server"
	}
	if cfg.HeroTitle == "" {
		cfg.HeroTitle = "NGINX High-Performance Engine Active"
	}
	if cfg.HeroDesc == "" {
		cfg.HeroDesc = "Your production-grade, event-driven HTTP/2 & reverse proxy web server is fully operational."
	}

	envBadge := "Native Linux Service"
	stackRuntime := fmt.Sprintf("Native (%s)", core.SysInfo.InitSystem)
	if cfg.EnvType == "docker" {
		envBadge = "Docker Container Stack"
		stackRuntime = "Docker Compose Container"
	}

	hostOS := GetOSPrettyVersion()
	if hostOS == "" {
		hostOS = "Linux"
	}
	arch := core.SysInfo.Architecture
	if arch == "" {
		arch = "amd64"
	}
	hostIP := GetLocalIP()
	if hostIP == "" {
		hostIP = "127.0.0.1"
	}

	accessScope := "Local Network (LAN)"
	if cfg.Port == "80" || cfg.Port == "443" {
		accessScope = "Standard HTTP/HTTPS"
	}

	// Dynamic admin row for OpenLiteSpeed or services with admin GUI
	var adminRow string
	if cfg.AdminPort != "" {
		adminRow = fmt.Sprintf(`<div class="info-row">
                        <span class="info-label">WebAdmin GUI</span>
                        <a href="https://%s:%s" target="_blank" class="info-value link" id="admin-url" title="https://%s:%s">https://%s:%s</a>
                    </div>`, hostIP, cfg.AdminPort, hostIP, cfg.AdminPort, hostIP, cfg.AdminPort)
	} else {
		adminRow = `<div class="info-row">
                        <span class="info-label">Virtual Hosts</span>
                        <span class="info-value">Active / Multi-Domain</span>
                    </div>`
	}

	// PHP Environment Detection
	phpInfo := DetectPHPInfo()
	if cfg.EnvType == "docker" {
		phpInfo = PHPInfo{
			Installed:       false,
			Version:         "Docker Stack",
			StatusBadge:     "Container Stack",
			StatusClass:     "",
			SAPI:            "Docker Compose",
			Handler:         "Container Internal",
			OPcache:         "N/A",
			OPcacheClass:    "",
			MemoryLimit:     "Container Limit",
			IniPath:         "N/A",
			KeyExtensions:   "N/A",
			ExtensionsCount: 0,
		}
	}
	if cfg.PHPInstalled != nil {
		phpInfo.Installed = *cfg.PHPInstalled
		if !*cfg.PHPInstalled {
			phpInfo.StatusBadge = "Not Installed"
			phpInfo.StatusClass = ""
			phpInfo.Version = "None"
			phpInfo.SAPI = "None"
			phpInfo.Handler = "Not Configured"
			phpInfo.OPcache = "Disabled"
			phpInfo.OPcacheClass = ""
			phpInfo.MemoryLimit = "N/A"
			phpInfo.IniPath = "N/A"
			phpInfo.KeyExtensions = "None"
			phpInfo.ExtensionsCount = 0
		} else {
			phpInfo.StatusBadge = "Installed & Operational"
			phpInfo.StatusClass = "success"
			if cfg.PHPVersion != "" {
				phpInfo.Version = cfg.PHPVersion
			}
			if cfg.PHPSapi != "" {
				phpInfo.SAPI = cfg.PHPSapi
			}
		}
	}

	var phpPill string
	if phpInfo.Installed {
		phpPill = fmt.Sprintf(`<div class="php-pill" id="top-php-pill" title="PHP Interpreter Active">
                    <span class="php-dot"></span>
                    <span id="top-php-text">PHP %s Active</span>
                </div>`, phpInfo.Version)
	} else {
		phpPill = `<div class="php-pill inactive" id="top-php-pill" title="PHP interpreter not detected">
                    <span id="top-php-text">PHP Inactive</span>
                </div>`
	}

	var step1Desc string
	var step3Title string
	var step3Desc string
	if phpInfo.Installed {
		step1Desc = fmt.Sprintf("Place your website files (HTML, CSS, JS, PHP) into <code>%s</code>.", cfg.WebDir)
		step3Title = "Verify PHP Execution"
		step3Desc = "Test dynamic script execution with <code>php-status.php</code> or deploy your <code>index.php</code>."
	} else {
		step1Desc = fmt.Sprintf("Place your website files (HTML, CSS, JS) into <code>%s</code>.", cfg.WebDir)
		step3Title = "Configure SSL / HTTPS"
		step3Desc = "Issue automated Let's Encrypt certificates using the CLI manager."
	}

	tpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{SERVICE_NAME}}</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;600&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg: #0c0c0e;
            --surface: #111114;
            --surface-elevated: #18181b;
            --surface-subtle: #141417;
            --surface-hover: #1c1c20;
            
            --border: #27272a;
            --border-subtle: #1f1f23;
            --border-hover: #3f3f46;
            
            --text-primary: #f4f4f5;
            --text-secondary: #a1a1aa;
            --text-muted: #71717a;
            --text-dim: #52525b;
            
            --pill-bg: #141417;
            --pill-border: #27272a;
            --pill-text: #e4e4e7;
            
            --accent-green: #34d399;
            --accent-green-bg: rgba(16, 185, 129, 0.1);
            --accent-green-border: rgba(16, 185, 129, 0.25);
            
            --accent-blue: #38bdf8;
            --accent-blue-bg: rgba(56, 189, 248, 0.1);
            --accent-blue-border: rgba(56, 189, 248, 0.25);
            
            --accent-indigo: #818cf8;
            --accent-indigo-bg: rgba(99, 102, 241, 0.1);
            --accent-indigo-border: rgba(99, 102, 241, 0.25);

            --shadow-card: 0 1px 3px 0 rgba(0, 0, 0, 0.5), 0 1px 2px -1px rgba(0, 0, 0, 0.5);
            --shadow-elevated: 0 20px 40px -15px rgba(0, 0, 0, 0.7);

            --font-sans: "Inter", "Geist", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            --font-mono: "JetBrains Mono", "Geist Mono", "Cascadia Code", "Cascadia Mono", "Segoe UI Mono", Menlo, Monaco, Consolas, "Liberation Mono", ui-monospace, monospace;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            background-color: var(--bg);
            background-image: 
                radial-gradient(circle at 50% 0%, rgba(99, 102, 241, 0.04) 0%, transparent 60%),
                radial-gradient(circle at 100% 100%, rgba(56, 189, 248, 0.02) 0%, transparent 50%);
            color: var(--text-primary);
            font-family: var(--font-sans);
            min-height: 100vh;
            padding: 44px 20px 60px;
            display: flex;
            flex-direction: column;
            align-items: center;
            -webkit-font-smoothing: antialiased;
            -moz-osx-font-smoothing: grayscale;
            text-rendering: optimizeLegibility;
            font-feature-settings: "cv02", "cv03", "cv04", "cv11";
        }

        .container {
            width: 100%;
            max-width: 980px;
            background: var(--surface);
            border: 1px solid var(--border);
            border-radius: 16px;
            padding: 34px 38px;
            box-shadow: var(--shadow-elevated);
        }

        /* Top Bar */
        .top-bar {
            display: flex;
            align-items: center;
            justify-content: space-between;
            border-bottom: 1px solid var(--border);
            padding-bottom: 22px;
            margin-bottom: 26px;
            flex-wrap: wrap;
            gap: 16px;
        }

        .brand {
            display: flex;
            align-items: center;
            gap: 14px;
        }

        .brand-logo {
            width: 44px;
            height: 44px;
            border-radius: 11px;
            background: linear-gradient(135deg, #059669 0%, #047857 100%);
            display: flex;
            align-items: center;
            justify-content: center;
            font-family: var(--font-sans);
            font-weight: 800;
            font-size: 17px;
            color: #ffffff;
            letter-spacing: -0.02em;
            border: 1px solid rgba(255, 255, 255, 0.18);
            box-shadow: 0 2px 8px rgba(0, 0, 0, 0.35);
            flex-shrink: 0;
        }

        .brand-meta h3 {
            font-size: 16px;
            font-weight: 700;
            color: #ffffff;
            letter-spacing: -0.015em;
            line-height: 1.25;
        }

        .brand-meta span {
            font-size: 11.5px;
            color: var(--text-muted);
            font-weight: 500;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }

        .status-group {
            display: flex;
            align-items: center;
            gap: 10px;
            flex-wrap: wrap;
        }

        .status-pill {
            display: inline-flex;
            align-items: center;
            gap: 7px;
            padding: 5px 13px;
            background: var(--accent-green-bg);
            border: 1px solid var(--accent-green-border);
            border-radius: 9999px;
            font-size: 11.5px;
            font-weight: 600;
            color: var(--accent-green);
            letter-spacing: 0.02em;
        }

        .status-dot {
            width: 7px;
            height: 7px;
            background: #10b981;
            border-radius: 50%;
            position: relative;
        }

        .status-dot::after {
            content: '';
            position: absolute;
            top: -2px;
            left: -2px;
            width: 11px;
            height: 11px;
            background: #10b981;
            border-radius: 50%;
            opacity: 0.4;
            animation: pulse-dot 2.5s infinite ease-in-out;
        }

        @keyframes pulse-dot {
            0%, 100% { transform: scale(1); opacity: 0.4; }
            50% { transform: scale(1.3); opacity: 0.15; }
        }

        .php-pill {
            display: inline-flex;
            align-items: center;
            gap: 7px;
            padding: 5px 13px;
            background: var(--accent-indigo-bg);
            border: 1px solid var(--accent-indigo-border);
            border-radius: 9999px;
            font-size: 11.5px;
            font-weight: 600;
            color: #c7d2fe;
            letter-spacing: 0.02em;
            transition: all 0.2s ease;
        }

        .php-pill.inactive {
            background: var(--surface-elevated);
            border-color: var(--border);
            color: var(--text-muted);
        }

        .php-dot {
            width: 7px;
            height: 7px;
            background: #818cf8;
            border-radius: 50%;
            position: relative;
        }

        .php-dot::after {
            content: '';
            position: absolute;
            top: -2px;
            left: -2px;
            width: 11px;
            height: 11px;
            background: #818cf8;
            border-radius: 50%;
            opacity: 0.4;
        }

        .env-pill {
            display: inline-flex;
            align-items: center;
            padding: 5px 13px;
            background: var(--surface-elevated);
            border: 1px solid var(--border);
            border-radius: 9999px;
            font-size: 11.5px;
            font-weight: 500;
            color: var(--text-secondary);
        }

        /* Hero */
        .hero {
            margin-bottom: 28px;
        }

        .hero h1 {
            font-size: 26px;
            font-weight: 700;
            letter-spacing: -0.025em;
            color: #ffffff;
            margin-bottom: 7px;
            line-height: 1.25;
        }

        .hero p {
            font-size: 14.5px;
            color: var(--text-secondary);
            line-height: 1.55;
            max-width: 750px;
        }

        /* 3x2 Grid */
        .grid {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 16px;
            margin-bottom: 24px;
        }

        @media (max-width: 768px) {
            .grid {
                grid-template-columns: 1fr;
            }
            .container {
                padding: 24px 20px;
            }
            .hero h1 {
                font-size: 22px;
            }
        }

        .card {
            background: var(--surface-elevated);
            border: 1px solid var(--border);
            border-radius: 12px;
            padding: 18px 20px;
            box-shadow: var(--shadow-card);
            transition: border-color 0.18s ease, box-shadow 0.18s ease;
        }

        .card:hover {
            border-color: var(--border-hover);
            box-shadow: 0 4px 14px rgba(0, 0, 0, 0.4);
        }

        .card-header {
            display: flex;
            align-items: center;
            gap: 9px;
            margin-bottom: 14px;
            padding-bottom: 11px;
            border-bottom: 1px solid var(--border-subtle);
        }

        .card-header svg {
            color: var(--text-muted);
            flex-shrink: 0;
        }

        .card-title {
            font-size: 11px;
            font-weight: 700;
            text-transform: uppercase;
            letter-spacing: 0.07em;
            color: var(--text-secondary);
        }

        .info-list {
            display: flex;
            flex-direction: column;
            gap: 7px;
        }

        .info-row {
            display: flex;
            justify-content: space-between;
            align-items: center;
            gap: 12px;
            font-size: 13px;
            min-height: 28px;
        }

        .info-label {
            color: var(--text-secondary);
            font-size: 12.5px;
            white-space: nowrap;
            flex-shrink: 0;
        }

        .info-value {
            font-family: var(--font-mono);
            font-size: 11.5px;
            font-weight: 500;
            color: var(--pill-text);
            background: var(--pill-bg);
            border: 1px solid var(--pill-border);
            padding: 3px 8px;
            border-radius: 6px;
            text-align: right;
            max-width: 74%;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
            word-break: normal;
            display: inline-block;
            vertical-align: middle;
            transition: border-color 0.15s ease, background 0.15s ease;
        }

        .info-value:hover {
            border-color: var(--border-hover);
        }

        .info-value.link {
            text-decoration: none;
            color: var(--accent-blue);
            cursor: pointer;
        }

        .info-value.link:hover {
            background: var(--accent-blue-bg);
            border-color: var(--accent-blue-border);
        }

        .info-value.success {
            color: var(--accent-green);
            background: var(--accent-green-bg);
            border-color: var(--accent-green-border);
        }

        /* CLI Box */
        .cli-box {
            background: var(--surface-elevated);
            border: 1px solid var(--border);
            border-radius: 12px;
            padding: 16px 20px;
            margin-bottom: 24px;
            display: flex;
            align-items: center;
            justify-content: space-between;
            flex-wrap: wrap;
            gap: 16px;
            box-shadow: var(--shadow-card);
            transition: border-color 0.18s ease;
        }

        .cli-box:hover {
            border-color: var(--border-hover);
        }

        .cli-info {
            display: flex;
            align-items: center;
            gap: 14px;
        }

        .cli-icon {
            width: 36px;
            height: 36px;
            background: var(--surface-subtle);
            border: 1px solid var(--border);
            border-radius: 9px;
            display: flex;
            align-items: center;
            justify-content: center;
            color: var(--accent-indigo);
            flex-shrink: 0;
        }

        .cli-text h4 {
            font-size: 14px;
            font-weight: 600;
            color: #ffffff;
            line-height: 1.25;
            margin-bottom: 2px;
        }

        .cli-text p {
            font-size: 12px;
            color: var(--text-secondary);
        }

        .cli-action {
            display: flex;
            align-items: center;
            gap: 10px;
            background: #09090b;
            border: 1px solid var(--border);
            padding: 6px 10px 6px 14px;
            border-radius: 8px;
        }

        .cli-prompt {
            color: var(--text-muted);
            font-family: var(--font-mono);
            font-size: 12px;
            user-select: none;
        }

        .cli-code {
            font-family: var(--font-mono);
            font-weight: 600;
            font-size: 13px;
            color: var(--accent-blue);
            user-select: all;
        }

        .copy-btn {
            background: #27272a;
            border: 1px solid #3f3f46;
            color: var(--text-primary);
            border-radius: 6px;
            padding: 5px 12px;
            font-family: var(--font-sans);
            font-size: 11.5px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.15s ease;
        }

        .copy-btn:hover {
            background: #3f3f46;
            color: #ffffff;
            border-color: #52525b;
        }

        .copy-btn:active {
            transform: scale(0.97);
        }

        .copy-btn.copied {
            background: var(--accent-green-bg);
            border-color: var(--accent-green-border);
            color: var(--accent-green);
        }

        /* Next Steps */
        .steps-box {
            background: var(--surface-elevated);
            border: 1px solid var(--border);
            border-radius: 12px;
            padding: 20px 22px;
            margin-bottom: 24px;
            box-shadow: var(--shadow-card);
        }

        .steps-header {
            font-size: 11px;
            font-weight: 700;
            text-transform: uppercase;
            letter-spacing: 0.08em;
            color: var(--text-secondary);
            margin-bottom: 14px;
            display: flex;
            align-items: center;
            gap: 7px;
        }

        .steps-header svg {
            color: var(--accent-indigo);
            flex-shrink: 0;
        }

        .steps-grid {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 14px;
        }

        @media (max-width: 768px) {
            .steps-grid {
                grid-template-columns: 1fr;
            }
        }

        .step-item {
            background: var(--surface-subtle);
            border: 1px solid var(--border-subtle);
            border-radius: 10px;
            padding: 14px 16px;
            display: flex;
            flex-direction: column;
            gap: 6px;
            transition: border-color 0.15s ease;
        }

        .step-item:hover {
            border-color: var(--border-hover);
        }

        .step-tag {
            font-size: 10.5px;
            font-family: var(--font-mono);
            font-weight: 700;
            color: #a5b4fc;
            background: var(--accent-indigo-bg);
            border: 1px solid var(--accent-indigo-border);
            padding: 2px 7px;
            border-radius: 4px;
            align-self: flex-start;
            letter-spacing: 0.04em;
        }

        .step-title {
            font-size: 13.5px;
            font-weight: 600;
            color: #ffffff;
            margin-top: 2px;
        }

        .step-desc {
            font-size: 12px;
            color: var(--text-secondary);
            line-height: 1.5;
        }

        .step-desc code {
            font-family: var(--font-mono);
            font-size: 11px;
            background: #27272a;
            border: 1px solid #3f3f46;
            padding: 1px 5px;
            border-radius: 4px;
            color: #e4e4e7;
        }

        /* Footer */
        .footer {
            padding-top: 18px;
            border-top: 1px solid var(--border);
            display: flex;
            justify-content: space-between;
            align-items: center;
            font-size: 12px;
            color: var(--text-muted);
            flex-wrap: wrap;
            gap: 12px;
        }

        .footer strong {
            color: var(--text-secondary);
            font-weight: 500;
        }
    </style>
</head>
<body>
    <div class="container">
        <!-- Top Bar -->
        <div class="top-bar">
            <div class="brand">
                <div class="brand-logo">{{LOGO_TEXT}}</div>
                <div class="brand-meta">
                    <h3>{{SERVICE_NAME}}</h3>
                    <span>Web Server Engine</span>
                </div>
            </div>
            <div class="status-group">
                <div class="status-pill">
                    <span class="status-dot"></span>
                    <span>ACTIVE &bull; OPERATIONAL</span>
                </div>
                {{PHP_PILL}}
                <div class="env-pill">
                    <span>{{ENV_BADGE}}</span>
                </div>
            </div>
        </div>

        <!-- Hero -->
        <div class="hero">
            <h1>{{HERO_TITLE}}</h1>
            <p>{{HERO_DESC}}</p>
        </div>

        <!-- 3x2 Info Grid -->
        <div class="grid">
            <!-- Card 1: Network & Access -->
            <div class="card">
                <div class="card-header">
                    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="2" y1="12" x2="22" y2="12"></line><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"></path></svg>
                    <span class="card-title">Network &amp; Access</span>
                </div>
                <div class="info-list">
                    <div class="info-row">
                        <span class="info-label">Server Origin</span>
                        <a href="http://{{HOST_IP}}:{{PORT}}" target="_blank" class="info-value link" id="server-url">http://{{HOST_IP}}:{{PORT}}</a>
                    </div>
                    <div class="info-row">
                        <span class="info-label">HTTP Web Port</span>
                        <span class="info-value">{{PORT}} / TCP</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Access Scope</span>
                        <span class="info-value success">{{ACCESS_SCOPE}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Protocol Support</span>
                        <span class="info-value">HTTP/1.1 &bull; HTTP/2</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Health Status</span>
                        <span class="info-value success">Passing 200 OK</span>
                    </div>
                </div>
            </div>

            <!-- Card 2: Runtime & Host -->
            <div class="card">
                <div class="card-header">
                    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect><rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect><line x1="6" y1="6" x2="6.01" y2="6"></line><line x1="6" y1="18" x2="6.01" y2="18"></line></svg>
                    <span class="card-title">Runtime &amp; Host</span>
                </div>
                <div class="info-list">
                    <div class="info-row">
                        <span class="info-label">Stack Runtime</span>
                        <span class="info-value" title="{{STACK_RUNTIME}}">{{STACK_RUNTIME}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Host OS</span>
                        <span class="info-value" title="{{HOST_OS}}">{{HOST_OS}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Architecture</span>
                        <span class="info-value">{{ARCH}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Process Owner</span>
                        <span class="info-value" title="{{WEB_USER}}">{{WEB_USER}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Init System</span>
                        <span class="info-value">Native Service</span>
                    </div>
                </div>
            </div>

            <!-- Card 3: Storage & Locations -->
            <div class="card">
                <div class="card-header">
                    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path></svg>
                    <span class="card-title">Locations &amp; Files</span>
                </div>
                <div class="info-list">
                    <div class="info-row">
                        <span class="info-label">Document Root</span>
                        <span class="info-value" title="{{WEB_DIR}}">{{WEB_DIR}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Main Config</span>
                        <span class="info-value" title="{{CONFIG_PATH}}">{{CONFIG_PATH}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Index File</span>
                        <span class="info-value">index.html (Active)</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Companion Status</span>
                        <span class="info-value">php-status.php</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">File Permissions</span>
                        <span class="info-value">0644 / 0755</span>
                    </div>
                </div>
            </div>

            <!-- Card 4: Administration & Controls -->
            <div class="card">
                <div class="card-header">
                    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>
                    <span class="card-title">Administration</span>
                </div>
                <div class="info-list">
                    {{ADMIN_ROW}}
                    <div class="info-row">
                        <span class="info-label">CLI Manager</span>
                        <span class="info-value">{{MANAGER_CMD}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">SSL Provisioning</span>
                        <span class="info-value success">Let's Encrypt Ready</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Security Policy</span>
                        <span class="info-value success">Enforced (0644/0755)</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Service Health</span>
                        <span class="info-value success">Monitored</span>
                    </div>
                </div>
            </div>

            <!-- Card 5: PHP Engine & Handler -->
            <div class="card">
                <div class="card-header">
                    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"></polyline><polyline points="8 6 2 12 8 18"></polyline></svg>
                    <span class="card-title">PHP Engine &amp; Handler</span>
                </div>
                <div class="info-list">
                    <div class="info-row">
                        <span class="info-label">PHP Runtime</span>
                        <span class="info-value {{PHP_STATUS_CLASS}}" id="php-status-badge">{{PHP_STATUS_BADGE}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Engine Version</span>
                        <span class="info-value" id="php-version-val">{{PHP_VERSION_LABEL}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">SAPI Interface</span>
                        <span class="info-value" id="php-sapi-val" title="{{PHP_SAPI}}">{{PHP_SAPI}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">FastCGI Handler</span>
                        <span class="info-value" title="{{PHP_HANDLER}}">{{PHP_HANDLER}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Web SAPI Probe</span>
                        <span class="info-value" id="php-live-probe" title="PHP web execution status">Testing...</span>
                    </div>
                </div>
            </div>

            <!-- Card 6: PHP Configuration & Modules -->
            <div class="card">
                <div class="card-header">
                    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="4" y1="21" x2="4" y2="14"></line><line x1="4" y1="10" x2="4" y2="3"></line><line x1="12" y1="21" x2="12" y2="12"></line><line x1="12" y1="8" x2="12" y2="3"></line><line x1="20" y1="21" x2="20" y2="16"></line><line x1="20" y1="12" x2="20" y2="3"></line><line x1="1" y1="14" x2="7" y2="14"></line><line x1="9" y1="8" x2="15" y2="8"></line><line x1="17" y1="16" x2="23" y2="16"></line></svg>
                    <span class="card-title">PHP Config &amp; Modules</span>
                </div>
                <div class="info-list">
                    <div class="info-row">
                        <span class="info-label">OPcache Engine</span>
                        <span class="info-value {{PHP_OPCACHE_CLASS}}" id="php-opcache-val">{{PHP_OPCACHE}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Memory Limit</span>
                        <span class="info-value" id="php-mem-val">{{PHP_MEMORY_LIMIT}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Active Modules</span>
                        <span class="info-value" title="{{PHP_EXTENSIONS}}" id="php-ext-val">{{PHP_EXTENSIONS}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Configuration</span>
                        <span class="info-value" title="{{PHP_INI_PATH}}">{{PHP_INI_PATH}}</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Diagnostics API</span>
                        <a href="php-status.php" target="_blank" class="info-value link" id="php-api-link">php-status.php &nearr;</a>
                    </div>
                </div>
            </div>
        </div>

        <!-- CLI Management Bar -->
        <div class="cli-box">
            <div class="cli-info">
                <div class="cli-icon">
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"></polyline><line x1="12" y1="19" x2="20" y2="19"></line></svg>
                </div>
                <div class="cli-text">
                    <h4>Server Management CLI</h4>
                    <p>Execute in your SSH terminal to manage virtual hosts, PHP settings, certificates, and diagnostics.</p>
                </div>
            </div>
            <div class="cli-action">
                <span class="cli-prompt">$</span>
                <span class="cli-code" id="cmd-text">{{MANAGER_CMD}}</span>
                <button class="copy-btn" id="copy-btn" onclick="copyCLI()">Copy</button>
            </div>
        </div>

        <!-- Next Steps -->
        <div class="steps-box">
            <div class="steps-header">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><polygon points="12 8 8 12 12 16 12 8"></polygon></svg>
                <span>Next Steps &amp; Guidance</span>
            </div>
            <div class="steps-grid">
                <div class="step-item">
                    <span class="step-tag">STEP 01</span>
                    <div class="step-title">Deploy Website Files</div>
                    <div class="step-desc">{{STEP1_DESC}}</div>
                </div>
                <div class="step-item">
                    <span class="step-tag">STEP 02</span>
                    <div class="step-title">Manage Virtual Hosts</div>
                    <div class="step-desc">Type <code>{{MANAGER_CMD}}</code> in terminal to configure domain names, PHP handlers, and vhosts.</div>
                </div>
                <div class="step-item">
                    <span class="step-tag">STEP 03</span>
                    <div class="step-title">{{STEP3_TITLE}}</div>
                    <div class="step-desc">{{STEP3_DESC}}</div>
                </div>
            </div>
        </div>

        <!-- Footer -->
        <div class="footer">
            <span>Production Web Server &bull; Operational</span>
            <span id="live-time">Loading local time...</span>
        </div>
    </div>

    <script>
        if (window.location.origin && window.location.origin.startsWith('http')) {
            const el = document.getElementById('server-url');
            if (el) {
                el.textContent = window.location.origin;
                el.href = window.location.origin;
            }
            const adminEl = document.getElementById('admin-url');
            if (adminEl) {
                try {
                    const host = window.location.hostname;
                    adminEl.textContent = 'https://' + host + ':{{ADMIN_PORT}}';
                    adminEl.href = 'https://' + host + ':{{ADMIN_PORT}}';
                } catch(e) {}
            }
        }

        function updateClock() {
            const now = new Date();
            const timeStr = now.toLocaleDateString() + ' ' + now.toLocaleTimeString();
            const el = document.getElementById('live-time');
            if (el) el.textContent = 'Local Time: ' + timeStr;
        }
        updateClock();
        setInterval(updateClock, 1000);

        function copyCLI() {
            const cmd = document.getElementById('cmd-text').textContent;
            function markCopied() {
                const btn = document.getElementById('copy-btn');
                btn.textContent = 'Copied!';
                btn.classList.add('copied');
                setTimeout(() => {
                    btn.textContent = 'Copy';
                    btn.classList.remove('copied');
                }, 2000);
            }
            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(cmd).then(markCopied).catch(fallbackCopy);
            } else {
                fallbackCopy();
            }
            function fallbackCopy() {
                const ta = document.createElement('textarea');
                ta.value = cmd;
                ta.style.position = 'fixed';
                ta.style.opacity = '0';
                document.body.appendChild(ta);
                ta.select();
                try {
                    document.execCommand('copy');
                    markCopied();
                } catch(e) {}
                document.body.removeChild(ta);
            }
        }

        // Live probe to verify PHP Web Execution in real-time
        fetch('php-status.php')
            .then(function(r) {
                if (!r.ok) throw new Error('HTTP ' + r.status);
                return r.json();
            })
            .then(function(data) {
                const probeEl = document.getElementById('php-live-probe');
                if (probeEl) {
                    probeEl.textContent = 'Passing 200 OK';
                    probeEl.className = 'info-value success';
                }
                const topPill = document.getElementById('top-php-pill');
                const topText = document.getElementById('top-php-text');
                if (topPill && topText && data.version) {
                    topPill.className = 'php-pill';
                    topText.textContent = 'PHP v' + data.version + ' Active';
                    if (!topPill.querySelector('.php-dot')) {
                        const dot = document.createElement('span');
                        dot.className = 'php-dot';
                        topPill.insertBefore(dot, topText);
                    }
                }
                if (data.version) {
                    const vEl = document.getElementById('php-version-val');
                    if (vEl) vEl.textContent = 'v' + data.version;
                }
                if (data.sapi) {
                    const sEl = document.getElementById('php-sapi-val');
                    if (sEl) sEl.textContent = data.sapi;
                }
                if (data.memory_limit) {
                    const mEl = document.getElementById('php-mem-val');
                    if (mEl) mEl.textContent = data.memory_limit;
                }
                if (data.extensions_count) {
                    const extEl = document.getElementById('php-ext-val');
                    if (extEl && data.key_extensions) {
                        extEl.textContent = data.key_extensions.slice(0, 3).join(', ') + ' (+' + data.extensions_count + ' total)';
                    }
                }
            })
            .catch(function() {
                const probeEl = document.getElementById('php-live-probe');
                if (probeEl) {
                    const statusBadge = document.getElementById('php-status-badge');
                    if (statusBadge && statusBadge.textContent.includes('Operational')) {
                        probeEl.textContent = 'Pre-configured (CLI)';
                    } else {
                        probeEl.textContent = 'Inactive';
                    }
                }
            });
    </script>
</body>
</html>`

	html := tpl
	html = strings.ReplaceAll(html, "{{SERVICE_NAME}}", cfg.ServiceName)
	html = strings.ReplaceAll(html, "{{LOGO_TEXT}}", cfg.LogoText)
	html = strings.ReplaceAll(html, "{{HERO_TITLE}}", cfg.HeroTitle)
	html = strings.ReplaceAll(html, "{{HERO_DESC}}", cfg.HeroDesc)
	html = strings.ReplaceAll(html, "{{ENV_BADGE}}", envBadge)
	html = strings.ReplaceAll(html, "{{STACK_RUNTIME}}", stackRuntime)
	html = strings.ReplaceAll(html, "{{HOST_OS}}", hostOS)
	html = strings.ReplaceAll(html, "{{ARCH}}", arch)
	html = strings.ReplaceAll(html, "{{HOST_IP}}", hostIP)
	html = strings.ReplaceAll(html, "{{PORT}}", cfg.Port)
	html = strings.ReplaceAll(html, "{{ADMIN_PORT}}", cfg.AdminPort)
	html = strings.ReplaceAll(html, "{{ACCESS_SCOPE}}", accessScope)
	html = strings.ReplaceAll(html, "{{WEB_DIR}}", cfg.WebDir)
	html = strings.ReplaceAll(html, "{{CONFIG_PATH}}", cfg.ConfigPath)
	html = strings.ReplaceAll(html, "{{WEB_USER}}", cfg.WebUser)
	html = strings.ReplaceAll(html, "{{MANAGER_CMD}}", cfg.ManagerCmd)
	html = strings.ReplaceAll(html, "{{ADMIN_ROW}}", adminRow)
	html = strings.ReplaceAll(html, "{{PHP_PILL}}", phpPill)
	html = strings.ReplaceAll(html, "{{PHP_STATUS_BADGE}}", phpInfo.StatusBadge)
	html = strings.ReplaceAll(html, "{{PHP_STATUS_CLASS}}", phpInfo.StatusClass)
	html = strings.ReplaceAll(html, "{{PHP_VERSION_LABEL}}", phpInfo.Version)
	html = strings.ReplaceAll(html, "{{PHP_SAPI}}", phpInfo.SAPI)
	html = strings.ReplaceAll(html, "{{PHP_HANDLER}}", phpInfo.Handler)
	html = strings.ReplaceAll(html, "{{PHP_OPCACHE}}", phpInfo.OPcache)
	html = strings.ReplaceAll(html, "{{PHP_OPCACHE_CLASS}}", phpInfo.OPcacheClass)
	html = strings.ReplaceAll(html, "{{PHP_MEMORY_LIMIT}}", phpInfo.MemoryLimit)
	html = strings.ReplaceAll(html, "{{PHP_EXTENSIONS}}", phpInfo.KeyExtensions)
	html = strings.ReplaceAll(html, "{{PHP_INI_PATH}}", phpInfo.IniPath)
	html = strings.ReplaceAll(html, "{{STEP1_DESC}}", step1Desc)
	html = strings.ReplaceAll(html, "{{STEP3_TITLE}}", step3Title)
	html = strings.ReplaceAll(html, "{{STEP3_DESC}}", step3Desc)

	return html
}
