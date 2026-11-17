// ==============================================================================
// KROTEK SCRIPT HUB — VIRTUALHOST & SERVER BLOCK MANAGER
// Service: CADDY (web-server)
// File:    web-server/caddy/src/modules/vhosts.go
//
// Manages website domain configurations, reverse proxy upstream blocks, directory permissions, and Let's Encrypt SSL/TLS certificate directives.
// ==============================================================================

package modules

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"krotek-caddy-installer/core"
	"krotek-caddy-installer/logger"
	"krotek-caddy-installer/system"
	"krotek-caddy-installer/tui"
)

func GetDefaultCaddyfile() string {
	return `:80 {
    root * /var/www/html
    file_server
    encode gzip
}

import /etc/caddy/conf.d/*.caddy
`
}

// ensureValidCaddyfile ensures /etc/caddy/Caddyfile exists and has valid directives.
func EnsureValidCaddyfile() {
	_ = os.MkdirAll("/etc/caddy", 0755)
	_ = os.MkdirAll("/etc/caddy/conf.d", 0755)
	_ = os.MkdirAll("/var/www/html", 0755)
	_ = os.MkdirAll("/var/log/caddy", 0755)

	mainConf := "/etc/caddy/Caddyfile"
	stat, err := os.Lstat(mainConf)
	if os.IsNotExist(err) || (err == nil && stat.Mode().IsRegular() && stat.Size() < 30) {
		_ = system.WriteFileNoFollow(mainConf, []byte(GetDefaultCaddyfile()), 0644, "")
	}
}

// runEditor opens the specified file in the user's preferred editor ($EDITOR),
// falling back to nano, vim, or vi. The editor is attached directly to /dev/tty.

func DoAddDomain() {
	tui.ClearScreen()
	cardLines := []string{
		core.BOLD + core.CYAN + "Add New Virtual Host / Domain Configuration" + core.NC,
		"",
		"  Specify the primary domain name you want to configure for CADDY.",
		"  " + core.BOLD + "Examples:" + core.NC + " " + core.CYAN + "next-app.com" + core.NC + ", " + core.CYAN + "api.service.org" + core.NC + ", " + core.CYAN + "sub.domain.io" + core.NC,
	}
	tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel & Return to Menu", "Add Virtual Host / Domain", cardLines)

	domainName, st := tui.ReadTextInput("Enter domain name", "")
	if st != 0 || strings.TrimSpace(domainName) == "" {
		logger.LogInfo("Cancelled by user.")
		return
	}
	domainName = system.CleanDomain(domainName)
	if !system.ValidateDomainName(domainName) {
		tui.PrintInBox("Validation Error", []string{
			core.RED + "Invalid domain name syntax. Must be a valid FQDN (e.g. example.com)." + core.NC,
			"",
			"Entered: " + domainName,
		})
		return
	}

	preset, targetOrPath, ok := PromptVHostPreset(domainName)
	if !ok {
		logger.LogInfo("Cancelled by user.")
		return
	}

	sslMode, certPath, keyPath := PromptCaddySSLStrategy(domainName)

	confDir := "/etc/caddy/conf.d"
	_ = os.MkdirAll(confDir, 0755)
	vhostFile := fmt.Sprintf("%s/%s.caddy", confDir, domainName)

	if preset.Type == PresetSPA || preset.Type == PresetPHP || preset.Type == PresetStatic {
		if err := system.ValidateSafeDirectoryPath(targetOrPath); err != nil {
			logger.LogError("Aborting: " + err.Error())
			return
		}
		if system.IsSymlinkPath(targetOrPath) {
			logger.LogError("Aborting: document root must not be a symbolic link: " + targetOrPath)
			return
		}
		_ = os.MkdirAll(targetOrPath, 0755)
	}

	// Universal firewall hardening (UFW, Firewalld, raw iptables/ip6tables, rules persistence)
	system.ConfigureFirewallRules("80", "443")

	vhostTpl := GenerateCaddyVHost(preset, domainName, targetOrPath, sslMode, certPath, keyPath)
	if vhostTpl == "" {
		logger.LogError("Aborting: generated Caddy vhost configuration is invalid.")
		return
	}
	if err := system.WriteFileNoFollow(vhostFile, []byte(vhostTpl), 0644, ""); err != nil {
		logger.LogError("Failed to write Caddy virtual host: " + err.Error())
		return
	}

	tui.ClearScreen()
	var addRes []string
	out, testErr := exec.Command("caddy", "validate", "--config", "/etc/caddy/Caddyfile").CombinedOutput()
	if testErr == nil {
		_ = exec.Command("systemctl", "reload", "caddy").Run()
		addRes = append(addRes, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
		addRes = append(addRes, tui.FormatKV("Target Domain", domainName, core.CYAN))
		addRes = append(addRes, tui.FormatKV("Preset Architecture", preset.Title, core.GREEN))
		addRes = append(addRes, tui.FormatKV("Target / DocumentRoot", targetOrPath, core.CYAN))
		addRes = append(addRes, tui.FormatKV("Configuration File", vhostFile, core.YELLOW))
		addRes = append(addRes, tui.FormatKV("Caddy Service", "Syntax OK & Service Reloaded", core.GREEN))
		tui.PrintInBox("Add Virtual Host Result", addRes)

		// If backend proxy preset was selected, offer automated systemd background service generator
		if preset.Type == PresetNodeJS || preset.Type == PresetPython {
			time.Sleep(1 * time.Second)
			tui.ClearScreen()
			askService := tui.AskYesNo("Deploy background systemd service for this backend app now?")
			if askService == 0 {
				GenerateSystemdServiceWizard()
			}
		}
	} else {
		addRes = append(addRes, tui.FormatKV("Operation Status", "WARNING", core.YELLOW))
		addRes = append(addRes, tui.FormatKV("Target Domain", domainName, core.CYAN))
		addRes = append(addRes, tui.FormatKV("Configuration File", vhostFile, core.YELLOW))
		addRes = append(addRes, tui.FormatKV("Caddy Service", "Syntax test failed!", core.RED))
		addRes = append(addRes, "")
		addRes = append(addRes, core.BOLD+"Syntax Diagnostics Output:"+core.NC)
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			addRes = append(addRes, "  "+core.RED+core.SanitizeTerminalText(line)+core.NC)
		}
		tui.PrintInBox("Add Virtual Host Result", addRes)
	}
}

// ==============================================================================
// MANAGEMENT MENUS
// ==============================================================================

// managementMenu routes to either the Docker stack management menu or the
// native Linux service management menu based on the ENV_TYPE value in the state file.
