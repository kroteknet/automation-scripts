// ==============================================================================
// KROTEK SCRIPT HUB — SELF-HEALING & AUTOMATED REPAIR ENGINE
// Service: OpenResty (web-server)
// File:    web-server/openresty/src/modules/heal.go
//
// Inspects OpenResty runtime directories, permission errors, and PID symlinks,
// creating missing document roots or restoring stock placeholders safely without
// ever interfering with vanilla NGINX on Debian/RHEL or user-customized sites.
// ==============================================================================

package modules

import (
	"os"
	"path/filepath"
	"strings"

	"krotek-openresty-installer/system"
)

func isStockPlaceholderHTML(content string) bool {
	return strings.Contains(content, "Welcome to OpenResty!") ||
		strings.Contains(content, "Welcome to nginx!") ||
		strings.Contains(content, "If you see this page, the nginx web server")
}

// EnsureOpenRestyWorking inspects the OpenResty environment and auto-repairs
// runtime directories, PID symlinks, and missing default landing pages.
func EnsureOpenRestyWorking() {
	if !system.IsOpenRestyInstalled() {
		return
	}

	state := system.LoadState()
	webUser := system.GetRuntimeWebUser()
	system.EnsureOpenRestyRuntimeDirs(webUser)
	syncPidSymlinks()

	// Do not overwrite landing pages when adopting an external OpenResty setup or Docker stack
	if !system.IsInstalledState() || state["ACTION"] == "adopted" || state["ENV_TYPE"] == "docker" {
		return
	}

	webDir := "/var/www/html"
	_ = os.MkdirAll(webDir, 0755)
	system.SecureWebRootPermissions(webDir, webUser)

	domainName := strings.TrimSpace(state["DOMAIN_NAME"])
	if domainName != "" && system.ValidateDomainName(domainName) {
		domainDir := "/var/www/" + system.CleanDomain(domainName)
		if customWebDir := strings.TrimSpace(state["WEB_DIR"]); customWebDir != "" && system.ValidateSafeDirectoryPath(customWebDir) == nil {
			domainDir = customWebDir
		}
		_ = os.MkdirAll(domainDir, 0755)
		system.SecureWebRootPermissions(domainDir, webUser)
	}

	port := system.GetConfiguredPort()
	confPath := system.GetOpenRestyMainConfigPath()

	docRoots := []string{webDir}
	if fi, err := os.Stat("/usr/local/openresty/nginx/html"); err == nil && fi.IsDir() {
		docRoots = append(docRoots, "/usr/local/openresty/nginx/html")
	}

	for _, d := range docRoots {
		_ = os.MkdirAll(d, 0755)
		system.SecureWebRootPermissions(d, webUser)
		idxPath := filepath.Join(d, "index.html")
		content, err := os.ReadFile(idxPath)
		if err != nil || isStockPlaceholderHTML(string(content)) {
			defaultHTML := system.GenerateLandingPageHTML(system.LandingPageConfig{
				ServiceName: "OpenResty Web Platform",
				LogoText:    "OR",
				HeroTitle:   "OpenResty Lua Platform Active",
				HeroDesc:    "Your high-performance dynamic web platform integrating enhanced Nginx core, LuaJIT, and OPM ecosystem is operational.",
				EnvType:     "native",
				Port:        port,
				WebDir:      d,
				ConfigPath:  confPath,
				WebUser:     webUser,
				ManagerCmd:  "openresty-manager",
			})
			_ = system.WriteFileNoFollow(idxPath, []byte(defaultHTML), 0644, webUser)
		}
	}
}

func syncPidSymlinks() {
	pidPath := system.GetOpenRestyPidPath()
	if _, err := os.Stat(pidPath); err == nil {
		_ = os.MkdirAll("/run/openresty", 0755)
		runPid := "/run/openresty/openresty.pid"
		if pidPath != runPid {
			if _, err2 := os.Lstat(runPid); os.IsNotExist(err2) {
				_ = os.Symlink(pidPath, runPid)
			}
		}
	}
	if _, err := os.Stat("/run/openresty/openresty.pid"); err == nil {
		if _, err2 := os.Lstat("/run/openresty.pid"); os.IsNotExist(err2) {
			_ = os.Symlink("/run/openresty/openresty.pid", "/run/openresty.pid")
		}
	} else if _, err := os.Stat("/run/openresty.pid"); err == nil {
		if _, err2 := os.Lstat("/run/openresty/openresty.pid"); os.IsNotExist(err2) {
			_ = os.Symlink("/run/openresty.pid", "/run/openresty/openresty.pid")
		}
	}
}
