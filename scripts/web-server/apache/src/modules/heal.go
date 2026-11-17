// ==============================================================================
// KROTEK SCRIPT HUB — SELF-HEALING & AUTOMATED REPAIR ENGINE
// Service: APACHE (web-server)
// File:    web-server/apache/src/modules/heal.go
//
// Inspects broken service configurations, checks for permission errors, creates missing document roots, and restarts failed service units.
// ==============================================================================

package modules

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"krotek-apache-installer/core"
	"krotek-apache-installer/system"
)

// EnsureApacheWorking inspects the environment and auto-repairs broken or dummy configurations,
// such as Alpine Linux's default "It works!" placeholder or missing index.html.
// EnsureApacheWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
// EnsureApacheWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
// EnsureApacheWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
// EnsureApacheWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
func EnsureApacheWorking() {
	if !system.IsServiceInstalled() && !system.CommandExists("httpd") && !system.CommandExists("apache2") {
		return
	}

	state := system.LoadState()
	if state["ACTION"] == "adopted" || !system.IsInstalledState() {
		return
	}

	webUser := "apache"
	if core.SysInfo.Family == "debian" {
		webUser = "www-data"
	}

	docRoots := []string{
		"/var/www/localhost/htdocs", // Alpine Linux default DocumentRoot
		"/var/www/html",             // Debian/Ubuntu/RHEL default DocumentRoot
		"/var/www/htdocs",
	}

	activeDocRoot := "/var/www/html"
	if core.SysInfo.Family == "alpine" {
		activeDocRoot = "/var/www/localhost/htdocs"
	}

	defaultHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
		ServiceName: "Apache HTTP Server",
		LogoText:    "A",
		HeroTitle:   "Apache HTTP Server Active",
		HeroDesc:    "Your robust, production-ready Apache web server is fully operational and tuned for production.",
		EnvType:     "native",
		Port:        "80",
		WebDir:      activeDocRoot,
		ConfigPath:  "/etc/apache2/httpd.conf",
		WebUser:     webUser,
		ManagerCmd:  "apache-manager",
	})

	_ = exec.Command("chmod", "755", "/var").Run()
	_ = exec.Command("chmod", "755", "/var/www").Run()

	for _, d := range docRoots {
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
		// If file doesn't exist, is small, or contains the default distro placeholder:
		isDefaultPlaceholder := strings.Contains(string(content), "It works!") ||
			strings.Contains(string(content), "Apache2 Debian Default Page") ||
			strings.Contains(string(content), "Apache2 Ubuntu Default Page") ||
			strings.Contains(string(content), "Fedora Webserver")
		if err != nil || len(content) < 200 || isDefaultPlaceholder {
			_ = system.WriteFileNoFollow(idxPath, []byte(defaultHTML), 0644, webUser)
		}
	}
}
