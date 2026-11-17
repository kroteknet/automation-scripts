// ==============================================================================
// KROTEK SCRIPT HUB — OPENRESTY PATHS, BINARIES & RUNTIME DISCOVERY
// Service: OpenResty (web-server)
// File:    web-server/openresty/src/system/paths.go
//
// Centralizes single-source-of-truth OpenResty paths across Debian/Ubuntu,
// RHEL/AlmaLinux/Rocky/Fedora, and Alpine Linux, alongside binary resolution,
// runtime user lookup, Lua library directives, IPv6 capability probing, and
// OpenResty-specific installation/process checks isolated from host NGINX.
// ==============================================================================

package system

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"krotek-openresty-installer/core"
)

func fileExistsLocal(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// LookupSystemUser returns the first candidate username present in the host user database,
// falling back safely to "nobody".
func LookupSystemUser(candidates ...string) string {
	for _, u := range candidates {
		if u == "" {
			continue
		}
		if _, err := user.Lookup(u); err == nil {
			return u
		}
	}
	return "nobody"
}

// GetRuntimeWebUser returns the non-privileged worker user for OpenResty on this OS.
func GetRuntimeWebUser() string {
	if core.SysInfo.Family == "debian" {
		return LookupSystemUser("www-data", "nginx", "nobody")
	}
	return LookupSystemUser("nginx", "www-data", "nobody")
}

// GetLogDirOwner returns "user:group" suitable for OpenResty log directories.
func GetLogDirOwner(webUser string) string {
	if webUser == "" {
		webUser = GetRuntimeWebUser()
	}
	if core.SysInfo.Family == "debian" {
		if _, err := user.LookupGroup("adm"); err == nil {
			return webUser + ":adm"
		}
	}
	return webUser + ":" + webUser
}

// GetOpenRestyBinPath returns the active OpenResty server executable path, or "" if absent.
// It strictly identifies OpenResty binaries and never mistakes host vanilla NGINX on Debian/RHEL.
func GetOpenRestyBinPath() string {
	if core.SysInfo.Family == "alpine" {
		if fileExistsLocal("/usr/lib/nginx/bin/openresty") {
			return "/usr/lib/nginx/bin/openresty"
		}
		if fileExistsLocal("/etc/init.d/openresty") && fileExistsLocal("/usr/sbin/nginx") {
			return "/usr/sbin/nginx"
		}
		if p, err := exec.LookPath("openresty"); err == nil {
			return p
		}
		return ""
	}

	candidates := []string{
		"/usr/local/openresty/bin/openresty",
		"/usr/local/openresty/nginx/sbin/nginx",
		"/usr/bin/openresty",
		"/usr/local/bin/openresty",
	}
	for _, c := range candidates {
		if fileExistsLocal(c) {
			return c
		}
	}
	if p, err := exec.LookPath("openresty"); err == nil {
		return p
	}
	return ""
}

// EnsureOpenRestyBinarySymlinks exposes openresty, resty, and opm under /usr/local/bin when missing from PATH.
func EnsureOpenRestyBinarySymlinks() {
	_ = os.MkdirAll("/usr/local/bin", 0755)
	tools := []string{"openresty", "resty", "opm"}
	srcRoots := []string{"/usr/local/openresty/bin", "/usr/lib/nginx/bin"}
	for _, tool := range tools {
		target := filepath.Join("/usr/local/bin", tool)
		if _, err := os.Lstat(target); err == nil {
			continue
		}
		if CommandExists(tool) {
			continue
		}
		for _, root := range srcRoots {
			cand := filepath.Join(root, tool)
			if fileExistsLocal(cand) {
				_ = os.Symlink(cand, target)
				break
			}
		}
	}
}

// GetOpenRestyMainConfigPath returns the primary nginx.conf file read by OpenResty on this OS.
func GetOpenRestyMainConfigPath() string {
	if core.SysInfo.Family == "alpine" {
		return "/etc/nginx/nginx.conf"
	}
	if fileExistsLocal("/usr/local/openresty/nginx/conf/nginx.conf") {
		return "/usr/local/openresty/nginx/conf/nginx.conf"
	}
	if fileExistsLocal("/etc/openresty/nginx.conf") && !fileExistsLocal("/usr/local/openresty/nginx/conf") {
		return "/etc/openresty/nginx.conf"
	}
	return "/usr/local/openresty/nginx/conf/nginx.conf"
}

// GetOpenRestyConfDir returns the directory containing OpenResty's primary nginx.conf.
func GetOpenRestyConfDir() string {
	return filepath.Dir(GetOpenRestyMainConfigPath())
}

// GetOpenRestyMimeTypesPath returns an existing mime.types path matching the active OpenResty conf dir.
func GetOpenRestyMimeTypesPath() string {
	confDir := GetOpenRestyConfDir()
	candidates := []string{
		filepath.Join(confDir, "mime.types"),
		"/usr/local/openresty/nginx/conf/mime.types",
		"/etc/nginx/mime.types",
	}
	for _, c := range candidates {
		if fileExistsLocal(c) {
			return c
		}
	}
	return filepath.Join(confDir, "mime.types")
}

// GetOpenRestyVHostDirs returns (availableDir, enabledDir) for VirtualHost configs on this OS.
func GetOpenRestyVHostDirs() (string, string) {
	confDir := GetOpenRestyConfDir()
	switch core.SysInfo.Family {
	case "alpine":
		return "/etc/nginx/http.d", "/etc/nginx/http.d"
	case "debian":
		return filepath.Join(confDir, "sites-available"), filepath.Join(confDir, "sites-enabled")
	default:
		return filepath.Join(confDir, "conf.d"), filepath.Join(confDir, "conf.d")
	}
}

// GetOpenRestyDefaultVHostPath returns the default server block file path for this OS.
func GetOpenRestyDefaultVHostPath() string {
	availDir, _ := GetOpenRestyVHostDirs()
	switch core.SysInfo.Family {
	case "debian":
		return filepath.Join(availDir, "default")
	default:
		return filepath.Join(availDir, "default.conf")
	}
}

// GetOpenRestyDomainVHostPaths returns (availFile, enabledSymlinkPath) for a given domain.
// enabledSymlinkPath is non-empty only on Debian/Ubuntu where sites-enabled symlinks are used.
func GetOpenRestyDomainVHostPaths(domain string) (string, string) {
	availDir, enabledDir := GetOpenRestyVHostDirs()
	switch core.SysInfo.Family {
	case "debian":
		return filepath.Join(availDir, domain), filepath.Join(enabledDir, domain)
	default:
		return filepath.Join(availDir, domain+".conf"), ""
	}
}

// GetOpenRestyPidPath returns the PID file path expected by systemd/OpenRC for OpenResty.
func GetOpenRestyPidPath() string {
	if core.SysInfo.Family == "alpine" {
		if data, err := os.ReadFile("/etc/init.d/openresty"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				t := strings.TrimSpace(line)
				if strings.HasPrefix(t, "pidfile=") {
					val := strings.Trim(strings.TrimPrefix(t, "pidfile="), "\"'")
					if val != "" {
						return val
					}
				}
			}
		}
		return "/run/nginx/nginx.pid"
	}

	unitPaths := []string{
		"/lib/systemd/system/openresty.service",
		"/usr/lib/systemd/system/openresty.service",
		"/etc/systemd/system/openresty.service",
	}
	for _, up := range unitPaths {
		if data, err := os.ReadFile(up); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				t := strings.TrimSpace(line)
				if strings.HasPrefix(t, "PIDFile=") {
					val := strings.TrimSpace(strings.TrimPrefix(t, "PIDFile="))
					if val != "" {
						return val
					}
				}
			}
		}
	}
	return "/usr/local/openresty/nginx/logs/nginx.pid"
}

// GetOpenRestyLogDir returns the primary directory used for OpenResty logs.
func GetOpenRestyLogDir() string {
	return "/var/log/openresty"
}

// GetLuaPackageDirectives returns nginx http-block Lua path and shared dictionary directives
// tailored to the active distro's OpenResty lualib installation layout.
func GetLuaPackageDirectives() string {
	var pathEntries []string
	var cpathEntries []string

	if fileExistsLocal("/usr/local/openresty/lualib") {
		pathEntries = append(pathEntries,
			"/usr/local/openresty/site/lualib/?.ljbc",
			"/usr/local/openresty/site/lualib/?.lua",
			"/usr/local/openresty/site/lualib/?/init.lua",
			"/usr/local/openresty/lualib/?.ljbc",
			"/usr/local/openresty/lualib/?.lua",
			"/usr/local/openresty/lualib/?/init.lua",
		)
		cpathEntries = append(cpathEntries,
			"/usr/local/openresty/site/lualib/?.so",
			"/usr/local/openresty/lualib/?.so",
		)
	}
	if fileExistsLocal("/usr/lib/nginx/lualib") {
		pathEntries = append(pathEntries,
			"/usr/lib/nginx/site/lualib/?.lua",
			"/usr/lib/nginx/site/lualib/?/init.lua",
			"/usr/lib/nginx/lualib/?.lua",
			"/usr/lib/nginx/lualib/?/init.lua",
		)
		cpathEntries = append(cpathEntries,
			"/usr/lib/nginx/site/lualib/?.so",
			"/usr/lib/nginx/lualib/?.so",
		)
	}

	var b strings.Builder
	if len(pathEntries) > 0 {
		b.WriteString(fmt.Sprintf("    lua_package_path \"%s;;\";\n", strings.Join(pathEntries, ";")))
	}
	if len(cpathEntries) > 0 {
		b.WriteString(fmt.Sprintf("    lua_package_cpath \"%s;;\";\n", strings.Join(cpathEntries, ";")))
	}
	b.WriteString("    lua_shared_dict cache_store 10m;\n")
	return b.String()
}

// EnsureOpenRestyRuntimeDirs creates and secures runtime PID, log, and temp directories.
func EnsureOpenRestyRuntimeDirs(webUser string) {
	if webUser == "" {
		webUser = GetRuntimeWebUser()
	}
	owner := webUser + ":" + webUser
	logOwner := GetLogDirOwner(webUser)

	_ = os.MkdirAll("/var/log/openresty", 0755)
	_ = exec.Command("chown", "-R", logOwner, "/var/log/openresty").Run()
	_ = exec.Command("chmod", "755", "/var/log/openresty").Run()

	_ = os.MkdirAll("/run/openresty", 0755)
	_ = exec.Command("chown", "-R", owner, "/run/openresty").Run()

	pidDir := filepath.Dir(GetOpenRestyPidPath())
	if pidDir != "" && pidDir != "/" && pidDir != "/run" {
		_ = os.MkdirAll(pidDir, 0755)
		_ = exec.Command("chown", "-R", owner, pidDir).Run()
		_ = exec.Command("chmod", "755", pidDir).Run()
	}

	if fileExistsLocal("/usr/local/openresty/nginx") {
		_ = os.MkdirAll("/usr/local/openresty/nginx/logs", 0755)
		_ = exec.Command("chown", "-R", logOwner, "/usr/local/openresty/nginx/logs").Run()
	}

	if core.SysInfo.Family == "alpine" {
		_ = os.MkdirAll("/var/log/nginx", 0755)
		_ = exec.Command("chown", "-R", owner, "/var/log/nginx").Run()
		_ = os.MkdirAll("/run/nginx", 0755)
		_ = exec.Command("chown", "-R", owner, "/run/nginx").Run()
		_ = exec.Command("chmod", "775", "/run/nginx").Run()
		for _, sub := range []string{"client_body", "proxy", "fastcgi", "uwsgi", "scgi"} {
			_ = os.MkdirAll(filepath.Join("/var/tmp/nginx", sub), 0755)
		}
		_ = exec.Command("chown", "-R", owner, "/var/tmp/nginx").Run()
	}
}

// HasIPv6 reports whether the host kernel can bind TCP IPv6 sockets cleanly.
func HasIPv6() bool {
	ln, err := net.Listen("tcp6", "[::]:0")
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// ListenDirectives builds IPv4 and (if host supports IPv6) IPv6 listen lines.
func ListenDirectives(port string, defaultServer bool, sslSuffix string) string {
	if port == "" {
		port = "80"
	}
	extra := ""
	if defaultServer {
		extra += " default_server"
	}
	if sslSuffix != "" {
		extra += " " + strings.TrimSpace(sslSuffix)
	}
	lines := fmt.Sprintf("    listen %s%s;\n", port, extra)
	if HasIPv6() {
		lines += fmt.Sprintf("    listen [::]:%s%s;\n", port, extra)
	}
	return lines
}

// GetConfiguredPort returns the persisted OpenResty HTTP port from state, defaulting to "80".
func GetConfiguredPort() string {
	state := LoadState()
	if p := strings.TrimSpace(state["PORT"]); p != "" {
		if num, err := strconv.Atoi(p); err == nil && num >= 1 && num <= 65535 {
			return p
		}
	}
	return "80"
}

// CountConfiguredVHosts returns the number of enabled VirtualHost files for OpenResty.
func CountConfiguredVHosts() int {
	_, enabledDir := GetOpenRestyVHostDirs()
	count := 0
	if files, err := filepath.Glob(filepath.Join(enabledDir, "*")); err == nil {
		for _, f := range files {
			if fi, err := os.Stat(f); err == nil && !fi.IsDir() {
				count++
			}
		}
	}
	return count
}

// IsOpenRestyDockerInstalled checks for a Krotek OpenResty container or Compose directory.
func IsOpenRestyDockerInstalled() bool {
	if fileExistsLocal("/etc/openresty-docker/docker-compose.yml") {
		return true
	}
	if out, err := exec.Command("docker", "ps", "-a", "--filter", "name=krotek-openresty", "--format", "{{.Names}}").Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		return true
	}
	return false
}

// IsOpenRestyDockerRunning reports whether the Krotek OpenResty container is actively running.
func IsOpenRestyDockerRunning() bool {
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-openresty").Output(); err == nil && strings.TrimSpace(string(out)) == "running" {
		return true
	}
	if out, err := exec.Command("docker", "ps", "--filter", "name=openresty", "--format", "{{.Names}}").Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		return true
	}
	return false
}

// GetOpenRestyMasterPID reads /proc or PID file to locate the running OpenResty master process PID
// without ever matching vanilla host nginx on Debian/RHEL or our own installer process.
func GetOpenRestyMasterPID() string {
	pidPath := GetOpenRestyPidPath()
	if data, err := os.ReadFile(pidPath); err == nil {
		p := strings.TrimSpace(string(data))
		if _, err := strconv.Atoi(p); err == nil && fileExistsLocal(filepath.Join("/proc", p)) {
			return p
		}
	}

	procDirs, err := os.ReadDir("/proc")
	if err != nil {
		return ""
	}
	alpineHasOpenResty := core.SysInfo.Family == "alpine" && (fileExistsLocal("/usr/lib/nginx/bin/openresty") || fileExistsLocal("/etc/init.d/openresty"))
	for _, entry := range procDirs {
		if !entry.IsDir() {
			continue
		}
		pid := entry.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}
		cmdlineBytes, err := os.ReadFile(filepath.Join("/proc", pid, "cmdline"))
		if err != nil {
			continue
		}
		cmdline := strings.ReplaceAll(string(cmdlineBytes), "\x00", " ")
		if !strings.Contains(cmdline, "master process") {
			continue
		}
		exe, _ := os.Readlink(filepath.Join("/proc", pid, "exe"))
		if strings.Contains(exe, "openresty") || strings.Contains(cmdline, "openresty") {
			return pid
		}
		if alpineHasOpenResty && (exe == "/usr/sbin/nginx" || strings.Contains(cmdline, "/usr/sbin/nginx")) {
			return pid
		}
	}
	return ""
}

// IsOpenRestyProcessRunning returns true if a native OpenResty master process is active.
func IsOpenRestyProcessRunning() bool {
	return GetOpenRestyMasterPID() != ""
}

// IsOpenRestyRunning reports whether OpenResty (Docker or native) is currently active.
func IsOpenRestyRunning() bool {
	state := LoadState()
	if state["ENV_TYPE"] == "docker" {
		return IsOpenRestyDockerRunning()
	}
	if IsOpenRestyDockerRunning() && !fileExistsLocal(GetOpenRestyBinPath()) {
		return true
	}
	if IsSystemdRunning() && exec.Command("systemctl", "is-active", "--quiet", "openresty").Run() == nil {
		return true
	}
	if core.SysInfo.InitSystem == "openrc" && exec.Command("rc-service", "openresty", "status").Run() == nil {
		return true
	}
	return IsOpenRestyProcessRunning()
}

// IsOpenRestyInstalled checks strictly whether OpenResty is installed on this host.
func IsOpenRestyInstalled() bool {
	if IsInstalledState() {
		return true
	}
	if GetOpenRestyBinPath() != "" {
		return true
	}
	for _, p := range []string{
		"/usr/local/openresty/nginx/conf/nginx.conf",
		"/etc/openresty/nginx.conf",
		"/etc/init.d/openresty",
		"/lib/systemd/system/openresty.service",
		"/usr/lib/systemd/system/openresty.service",
	} {
		if fileExistsLocal(p) {
			return true
		}
	}
	return IsOpenRestyDockerInstalled()
}

