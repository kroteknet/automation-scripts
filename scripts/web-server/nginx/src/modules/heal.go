// ==============================================================================
// KROTEK SCRIPT HUB — SELF-HEALING & AUTOMATED REPAIR ENGINE
// Service: NGINX (web-server)
// File:    web-server/nginx/src/modules/heal.go
//
// Inspects broken service configurations, checks for permission errors, creates missing document roots, and restarts failed service units.
// ==============================================================================

package modules

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"krotek-nginx-installer/core"
	"krotek-nginx-installer/system"
)

// EnsureNginxWorking inspects the environment and auto-repairs broken or dummy configurations,
// such as Alpine Linux's default 404 catch-all block or missing /var/www/html/index.html.
// EnsureNginxWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
// EnsureNginxWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
// EnsureNginxWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
// EnsureNginxWorking inspects the environment and auto-repairs broken or dummy configurations (e.g. missing document root or invalid defaults).
func EnsureNginxWorking() {
	if !system.IsServiceInstalled() && !system.CommandExists("nginx") {
		return
	}

	state := system.LoadState()
	// Preserve existing files and configurations if adopted into TUI management
	// or if this is a pre-existing host installation not yet configured by Krotek installer.
	if state["ACTION"] == "adopted" || (!system.IsInstalledState() && (system.CommandExists("nginx") || fileExists("/etc/nginx/nginx.conf"))) {
		syncPidSymlinks()
		return
	}

	webUser := "nginx"
	if core.SysInfo.Family == "debian" {
		webUser = "www-data"
	}

	webDir := "/var/www/html"
	_ = os.MkdirAll(webDir, 0755)
	_ = exec.Command("chmod", "755", "/var").Run()
	_ = exec.Command("chmod", "755", "/var/www").Run()
	system.SecureWebRootPermissions(webDir, webUser)

	domainName := strings.TrimSpace(state["DOMAIN_NAME"])
	if domainName != "" {
		domainDir := filepath.Join("/var/www", domainName)
		if err := system.ValidateSafeDirectoryPath(domainDir); err == nil && !system.IsSymlinkPath(domainDir) {
			_ = os.MkdirAll(domainDir, 0755)
			system.SecureWebRootPermissions(domainDir, webUser)
		}
	}

	defaultHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
		ServiceName: "NGINX Web Server",
		LogoText:    "N",
		HeroTitle:   "Nginx Engine Server Active",
		HeroDesc:    "Your high-performance HTTP engine and reverse proxy is fully operational and tuned for production.",
		EnvType:     "native",
		Port:        "80",
		WebDir:      webDir,
		ConfigPath:  "/etc/nginx/nginx.conf",
		WebUser:     webUser,
		ManagerCmd:  "nginx-manager",
	})

	docRoots := []string{
		"/var/www/html",
		"/var/lib/nginx/html",
		"/usr/share/nginx/html",
	}
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
		if err != nil || len(content) < 200 || strings.Contains(string(content), "404") || strings.Contains(string(content), "Welcome to nginx") {
			_ = system.WriteFileNoFollow(idxPath, []byte(defaultHTML), 0644, webUser)
		}
	}

	_ = os.MkdirAll("/run/nginx", 0755)
	_ = os.MkdirAll("/var/log/nginx", 0755)
	if core.SysInfo.Family == "alpine" {
		_ = exec.Command("chown", "-R", "nginx:nginx", "/run/nginx").Run()
		_ = exec.Command("chmod", "775", "/run/nginx").Run()
	}

	// PID file cross-symlink synchronization so both /run/nginx.pid and /run/nginx/nginx.pid work
	syncPidSymlinks()

	vhostContent := `server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _;

    root /var/www/html;
    index index.html index.htm;

    location / {
        try_files $uri $uri/ /index.html =404;
    }
}
`

	healed := false

	if core.SysInfo.Family == "alpine" {
		alpineDefault := "/etc/nginx/http.d/default.conf"
		needsReplace := false
		if data, err := os.ReadFile(alpineDefault); err == nil {
			content := string(data)
			// Alpine's default package installs a dummy catch-all with "return 404;"
			if strings.Contains(content, "return 404") || !strings.Contains(content, "root ") {
				needsReplace = true
			}
		} else if os.IsNotExist(err) {
			files, _ := filepath.Glob("/etc/nginx/http.d/*.conf")
			if len(files) == 0 {
				needsReplace = true
			}
		}

		if needsReplace {
			_ = os.MkdirAll("/etc/nginx/http.d", 0755)
			_ = system.WriteFileNoFollow(alpineDefault, []byte(vhostContent), 0644, "")
			_ = os.Remove("/etc/nginx/conf.d/default.conf")
			healed = true
		}
	} else if core.SysInfo.Family == "debian" {
		debDefault := "/etc/nginx/sites-available/default"
		enabledDefault := "/etc/nginx/sites-enabled/default"
		needsReplace := false
		if data, err := os.ReadFile(debDefault); err == nil {
			if strings.Contains(string(data), "return 404") {
				needsReplace = true
			}
		} else if os.IsNotExist(err) {
			needsReplace = true
		}

		if needsReplace {
			_ = os.MkdirAll("/etc/nginx/sites-available", 0755)
			_ = os.MkdirAll("/etc/nginx/sites-enabled", 0755)
			_ = system.WriteFileNoFollow(debDefault, []byte(vhostContent), 0644, "")
			_ = os.Remove(enabledDefault)
			_ = os.Symlink(debDefault, enabledDefault)
			healed = true
		}
	} else {
		// RHEL / CentOS
		rhelDefault := "/etc/nginx/conf.d/default.conf"
		needsReplace := false
		if data, err := os.ReadFile(rhelDefault); err == nil {
			if strings.Contains(string(data), "return 404") {
				needsReplace = true
			}
		} else if os.IsNotExist(err) {
			files, _ := filepath.Glob("/etc/nginx/conf.d/*.conf")
			if len(files) == 0 {
				needsReplace = true
			}
		}

		if needsReplace {
			_ = os.MkdirAll("/etc/nginx/conf.d", 0755)
			_ = system.WriteFileNoFollow(rhelDefault, []byte(vhostContent), 0644, "")
			healed = true
		}
	}

	if healed {
		if exec.Command("nginx", "-t").Run() == nil {
			system.ReloadService("nginx")
		}
	}
}

func syncPidSymlinks() {
	if _, err := os.Stat("/run/nginx/nginx.pid"); err == nil {
		if _, err2 := os.Lstat("/run/nginx.pid"); os.IsNotExist(err2) {
			_ = os.Symlink("/run/nginx/nginx.pid", "/run/nginx.pid")
		}
	} else if _, err := os.Stat("/run/nginx.pid"); err == nil {
		if _, err2 := os.Lstat("/run/nginx/nginx.pid"); os.IsNotExist(err2) {
			_ = os.Symlink("/run/nginx.pid", "/run/nginx/nginx.pid")
		}
	}
}
