// ==============================================================================
// KROTEK SCRIPT HUB — SELF-HEALING & AUTOMATED REPAIR ENGINE
// Service: CADDY (web-server)
// File:    web-server/caddy/src/modules/heal.go
//
// Inspects broken service configurations, checks for permission errors, creates missing document roots, and restarts failed service units.
// ==============================================================================

package modules

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"krotek-caddy-installer/system"
)

// EnsureCaddyWorking inspects the environment and auto-repairs broken or dummy configurations,
// such as missing index.html or Caddy's default welcome page in /usr/share/caddy or /var/www/html.
// EnsureCaddyWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
// EnsureCaddyWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
// EnsureCaddyWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
// EnsureCaddyWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
func EnsureCaddyWorking() {
	if !system.IsServiceInstalled() && !system.CommandExists("caddy") {
		return
	}

	state := system.LoadState()
	if state["ACTION"] == "adopted" || !system.IsInstalledState() {
		return
	}

	webUser := "caddy"
	docRoots := []string{
		"/var/www/html",
		"/usr/share/caddy", // Default package root on Debian and Alpine
	}

	defaultHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
		ServiceName: "Caddy Web Server",
		LogoText:    "C",
		HeroTitle:   "Caddy Web Server Active",
		HeroDesc:    "Your enterprise-grade, memory-safe web server with automated HTTPS is fully operational and tuned.",
		EnvType:     "native",
		Port:        "80",
		WebDir:      "/var/www/html",
		ConfigPath:  "/etc/caddy/Caddyfile",
		WebUser:     webUser,
		ManagerCmd:  "caddy-manager",
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
		// If file doesn't exist, is small, or contains Caddy default placeholder:
		if err != nil || len(content) < 200 || strings.Contains(string(content), "Congratulations") || strings.Contains(string(content), "Welcome to Caddy") {
			_ = system.WriteFileNoFollow(idxPath, []byte(defaultHTML), 0644, webUser)
		}
	}

	EnsureValidCaddyfile()
}
