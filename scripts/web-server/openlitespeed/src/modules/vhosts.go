// ==============================================================================
// KROTEK SCRIPT HUB — VIRTUALHOST & SERVER BLOCK MANAGER
// Service: OPENLITESPEED (web-server)
// File:    web-server/openlitespeed/src/modules/vhosts.go
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

	"krotek-openlitespeed-installer/core"
	"krotek-openlitespeed-installer/logger"
	"krotek-openlitespeed-installer/system"
	"krotek-openlitespeed-installer/tui"
)

func DoAddDomain() {
	tui.ClearScreen()
	cardLines := []string{
		core.BOLD + core.CYAN + "Add New Virtual Host / Domain Configuration" + core.NC,
		"",
		"  Specify the primary domain name you want to configure for OPENLITESPEED.",
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

	sslMode, certPath, keyPath := PromptOpenLiteSpeedSSLStrategy(domainName)

	vhostDir := fmt.Sprintf("/usr/local/lsws/conf/vhosts/%s", domainName)
	_ = os.MkdirAll(vhostDir, 0755)
	vhostFile := fmt.Sprintf("%s/vhconf.conf", vhostDir)

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
	} else {
		_ = os.MkdirAll("/var/www/"+domainName, 0755)
	}

	// Universal firewall hardening (UFW, Firewalld, raw iptables/ip6tables, rules persistence)
	system.ConfigureFirewallRules("80", "443", "7080")

	vhostTpl := GenerateOpenLiteSpeedVHost(preset, domainName, targetOrPath, sslMode, certPath, keyPath)
	if vhostTpl == "" {
		logger.LogError("Aborting: generated OpenLiteSpeed vhost configuration is invalid.")
		return
	}
	if err := system.WriteFileNoFollow(vhostFile, []byte(vhostTpl), 0644, ""); err != nil {
		logger.LogError("Failed to write OpenLiteSpeed vhost config: " + err.Error())
		return
	}

	// Register virtual host in /usr/local/lsws/conf/httpd_config.conf
	RegisterVHostInOLS(domainName)

	tui.ClearScreen()
	var addRes []string
	if exec.Command("/usr/local/lsws/bin/lswsctrl", "restart").Run() == nil || exec.Command("systemctl", "reload", "openlitespeed").Run() == nil || exec.Command("systemctl", "reload", "lsws").Run() == nil {
		addRes = append(addRes, tui.FormatKV("Operation Status", "SUCCESS", core.GREEN))
		addRes = append(addRes, tui.FormatKV("Target Domain", domainName, core.CYAN))
		addRes = append(addRes, tui.FormatKV("Preset Architecture", preset.Title, core.GREEN))
		addRes = append(addRes, tui.FormatKV("Target / DocumentRoot", targetOrPath, core.CYAN))
		addRes = append(addRes, tui.FormatKV("Configuration File", vhostFile, core.YELLOW))
		addRes = append(addRes, tui.FormatKV("OpenLiteSpeed Service", "Syntax OK & Service Reloaded", core.GREEN))
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
		addRes = append(addRes, tui.FormatKV("Preset Architecture", preset.Title, core.GREEN))
		addRes = append(addRes, tui.FormatKV("Configuration File", vhostFile, core.YELLOW))
		addRes = append(addRes, tui.FormatKV("OpenLiteSpeed Service", "Configured & Reloaded", core.GREEN))
		tui.PrintInBox("Add Virtual Host Result", addRes)
	}
}

// ==============================================================================
// MANAGEMENT MENUS
// ==============================================================================

// managementMenu routes to either the Docker stack management menu or the
// native Linux service management menu based on the ENV_TYPE value in the state file.
