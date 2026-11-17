// ==============================================================================
// KROTEK SCRIPT HUB — SELF-HEALING & AUTOMATED REPAIR ENGINE
// Service: OPENLITESPEED (web-server)
// File:    web-server/openlitespeed/src/modules/heal.go
//
// Inspects broken service configurations, checks for permission errors, creates missing document roots, and restarts failed service units.
// ==============================================================================

package modules

import (
	"os"
	"path/filepath"
	"strings"

	"krotek-openlitespeed-installer/system"
)

// EnsureOpenLiteSpeedWorking inspects the environment and auto-repairs broken or dummy configurations,
// such as missing index.html or OpenLiteSpeed's default welcome page in /usr/local/lsws/Example/html.
// EnsureOpenLiteSpeedWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
// EnsureOpenLiteSpeedWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
// EnsureOpenLiteSpeedWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
// EnsureOpenLiteSpeedWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
func EnsureOpenLiteSpeedWorking() {
	if !system.IsServiceInstalled() {
		return
	}
	state := system.LoadState()
	if state["ACTION"] == "adopted" || !system.IsInstalledState() {
		return
	}
	docRoots := []string{
		"/usr/local/lsws/Example/html",
		"/usr/local/lsws/DEFAULT/html",
		"/var/www/html",
		"/var/www/vhosts/localhost/html",
		"/etc/openlitespeed-docker/html",
	}

	defaultHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
		ServiceName: "OpenLiteSpeed Web Server",
		LogoText:    "OLS",
		HeroTitle:   "OpenLiteSpeed Event Engine Active",
		HeroDesc:    "Your high-velocity, event-driven HTTP/3 & QUIC web server with LiteSpeed Cache is fully operational.",
		EnvType:     "native",
		Port:        "80",
		AdminPort:   "7080",
		WebDir:      "/usr/local/lsws/Example/html",
		ConfigPath:  "/usr/local/lsws/conf/httpd_config.conf",
		WebUser:     "nobody / lsadm",
		ManagerCmd:  "openlitespeed-manager",
	})

	webUser := "nobody"

	for _, d := range docRoots {
		// Only touch directories whose parent exists or is an established OLS root
		parent := filepath.Dir(d)
		if _, err := os.Stat(parent); os.IsNotExist(err) && !strings.HasPrefix(d, "/usr/local/lsws") {
			continue
		}
		if system.IsSymlinkPath(d) {
			continue
		}

		_ = os.MkdirAll(d, 0755)
		system.SecureWebRootPermissions(d, webUser)

		idxPath := filepath.Join(d, "index.html")
		if system.IsSymlinkPath(idxPath) {
			continue
		}
		content, err := os.ReadFile(idxPath)
		// If file doesn't exist, is small, or contains OpenLiteSpeed default placeholder:
		if err != nil || len(content) < 200 || strings.Contains(string(content), "Congratulations") || strings.Contains(string(content), "LiteSpeed") && !strings.Contains(string(content), "Web Server Engine") {
			_ = system.WriteFileNoFollow(idxPath, []byte(defaultHTML), 0644, webUser)
		}
	}
}
