// ==============================================================================
// KROTEK SCRIPT HUB — DYNAMIC PHP RUNTIME & FPM POOL CONFIGURATOR
// Service: OpenResty (web-server)
// File:    web-server/openresty/src/system/php.go
//
// Detects and installs the latest stable PHP version and required extensions
// across distributions, configures PHP-FPM pool socket permissions for the
// active OpenResty runtime user, and tunes OPcache/memory settings.
// ==============================================================================

package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"krotek-openresty-installer/core"
	"krotek-openresty-installer/logger"
)

// DetectLatestAlpinePHP queries apk repositories for available PHP versions
// and dynamically selects the highest stable version (e.g. php84, php83, php82).
func DetectLatestAlpinePHP() string {
	out, err := exec.Command("apk", "search", "-q", "php*-fpm").Output()
	if err == nil && len(out) > 0 {
		re := regexp.MustCompile(`^(php(\d+))-fpm$`)
		lines := strings.Split(string(out), "\n")
		var bestPrefix string
		var maxVer int
		for _, line := range lines {
			line = strings.TrimSpace(line)
			m := re.FindStringSubmatch(line)
			if len(m) == 3 {
				prefix := m[1]
				verNum, err := strconv.Atoi(m[2])
				if err == nil && verNum > maxVer {
					maxVer = verNum
					bestPrefix = prefix
				}
			}
		}
		if bestPrefix != "" {
			logger.LogInfo(fmt.Sprintf("Auto-detected latest Alpine PHP package: %s", bestPrefix))
			return bestPrefix
		}
	}

	// Fallback check: query installed PHP packages via apk info
	if infoOut, err := exec.Command("apk", "info").Output(); err == nil && len(infoOut) > 0 {
		re := regexp.MustCompile(`^(php(\d+))-fpm$`)
		for _, line := range strings.Split(string(infoOut), "\n") {
			if m := re.FindStringSubmatch(strings.TrimSpace(line)); len(m) >= 2 {
				return m[1]
			}
		}
	}

	// Secondary fallback check in descending order
	candidates := []string{"php85", "php84", "php83", "php82", "php81", "php"}
	for _, cand := range candidates {
		if err := exec.Command("apk", "info", "-e", cand+"-fpm").Run(); err == nil {
			return cand
		}
	}

	return "php83"
}

func alpinePackageAvailable(pkg string) bool {
	out, err := exec.Command("apk", "search", "-q", "-e", pkg).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == pkg
}

// InstallPHPStack dynamically discovers and installs the latest stable PHP version
// and essential extensions for the current operating system without hardcoding.
func InstallPHPStack() (string, bool) {
	switch core.SysInfo.Family {
	case "alpine":
		phpPrefix := DetectLatestAlpinePHP()
		desired := []string{
			phpPrefix,
			phpPrefix + "-fpm",
			phpPrefix + "-opcache",
			phpPrefix + "-curl",
			phpPrefix + "-mbstring",
			phpPrefix + "-openssl",
			phpPrefix + "-json",
			phpPrefix + "-mysqli",
			phpPrefix + "-session",
			phpPrefix + "-xml",
			phpPrefix + "-zip",
		}
		var pkgs []string
		for _, p := range desired {
			if p == phpPrefix || p == phpPrefix+"-fpm" || alpinePackageAvailable(p) {
				pkgs = append(pkgs, p)
			}
		}
		args := append([]string{"add", "--quiet"}, pkgs...)
		if !RunStep(fmt.Sprintf("Installing Latest PHP Stack (%s) & Extensions", phpPrefix), "apk", args...) {
			if !RunStep(fmt.Sprintf("Installing Core PHP (%s) & FPM", phpPrefix), "apk", "add", "--quiet", phpPrefix, phpPrefix+"-fpm") {
				return phpPrefix, false
			}
		}
		// Ensure `php` CLI symlink is present if Alpine installs `php83`/`php84`
		if !CommandExists("php") {
			if p, err := exec.LookPath(phpPrefix); err == nil {
				_ = os.Symlink(p, "/usr/bin/php")
			}
		}
		EnsurePHPFPMCompatibility()
		return phpPrefix, true

	case "debian":
		pkgs := []string{"php-fpm", "php-cli", "php-common", "php-mbstring", "php-xml", "php-curl", "php-mysql", "php-zip"}
		args := append([]string{"install", "-y", "-q"}, pkgs...)
		ok := RunStep("Installing Latest Stable PHP & Extensions", "apt-get", args...)
		if ok {
			EnsurePHPFPMCompatibility()
		}
		return "php", ok

	case "rhel":
		pkgs := []string{"php", "php-fpm", "php-cli", "php-common", "php-mbstring", "php-xml", "php-mysqlnd", "php-opcache", "php-pecl-zip"}
		args := append([]string{"install", "-y", "-q"}, pkgs...)
		ok := RunRHELStep("Installing Latest Stable PHP & Extensions", true, args...)
		if !ok {
			ok = RunRHELStep("Installing Core PHP & PHP-FPM", true, "install", "-y", "-q", "php", "php-fpm", "php-cli", "php-common")
		}
		if ok {
			EnsurePHPFPMCompatibility()
		}
		return "php", ok

	default:
		return "php", false
	}
}

// EnsurePHPFPMCompatibility ensures PHP-FPM runtime directories exist and Unix socket
// ownership/ACLs grant access to the active OpenResty worker user (e.g. www-data, nginx, nobody).
func EnsurePHPFPMCompatibility() {
	webUser := GetRuntimeWebUser()
	_ = os.MkdirAll("/run/php-fpm", 0755)
	_ = os.MkdirAll("/run/php", 0755)
	_ = os.MkdirAll("/var/log/php-fpm", 0755)

	// On RHEL/AlmaLinux, php-fpm defaults to user=apache and listen.acl_users=apache,nginx.
	// If OpenResty runs as nobody (when no nginx user exists), ensure the pool grants socket access to webUser.
	if core.SysInfo.Family == "rhel" && fileExistsLocal("/etc/php-fpm.d/www.conf") {
		dropIn := fmt.Sprintf("; Krotek OpenResty PHP-FPM compatibility\n[www]\nlisten.acl_users =\nlisten.owner = %s\nlisten.group = %s\nlisten.mode = 0660\n", webUser, webUser)
		_ = WriteFileNoFollow("/etc/php-fpm.d/zz-openresty.conf", []byte(dropIn), 0644, "")
	}

	// Ensure pool sockets/TCP configuration match active webUser on Debian/Alpine if present
	poolGlobs := []string{
		"/etc/php/*/fpm/pool.d/www.conf",
		"/etc/php*/php-fpm.d/www.conf",
	}
	for _, pat := range poolGlobs {
		matches, _ := filepath.Glob(pat)
		for _, poolFile := range matches {
			data, err := os.ReadFile(poolFile)
			if err != nil {
				continue
			}
			content := string(data)
			changed := false
			if strings.Contains(content, ";listen.owner =") {
				content = strings.ReplaceAll(content, ";listen.owner = nobody", "listen.owner = "+webUser)
				content = strings.ReplaceAll(content, ";listen.group = nobody", "listen.group = "+webUser)
				changed = true
			}
			if changed {
				_ = WriteFileNoFollow(poolFile, []byte(content), 0644, "")
			}
		}
	}
}

func isPHPFPMRunning() bool {
	procDirs, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
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
		if strings.Contains(cmdline, "php-fpm") {
			return true
		}
	}
	return false
}

func startPHPFPMDirectFallback(svcName string) bool {
	_ = os.MkdirAll("/run/php-fpm", 0755)
	_ = os.MkdirAll("/run/php", 0755)
	_ = os.MkdirAll("/var/log/php-fpm", 0755)

	candidates := []string{}
	if svcName != "" {
		candidates = append(candidates, svcName, "/usr/sbin/"+svcName, "/usr/local/sbin/"+svcName)
	}
	for _, v := range []string{"85", "84", "83", "82", "81", "80"} {
		candidates = append(candidates,
			"php-fpm"+v,
			"/usr/sbin/php-fpm"+v,
			"/usr/local/sbin/php-fpm"+v,
			"/usr/sbin/php-fpm"+v[:1]+"."+v[1:],
		)
	}
	candidates = append(candidates, "php-fpm", "/usr/sbin/php-fpm", "/usr/local/sbin/php-fpm")

	for _, cand := range candidates {
		binPath, err := exec.LookPath(cand)
		if err != nil {
			if !fileExistsLocal(cand) {
				continue
			}
			binPath = cand
		}
		// Force daemonize (-D) so foreground pools never block execution
		cmd := exec.Command(binPath, "-D")
		if err := cmd.Run(); err == nil {
			time.Sleep(250 * time.Millisecond)
			if isPHPFPMRunning() {
				return true
			}
		}
		// Some Debian/Ubuntu versions ship /usr/sbin/php-fpmX.Y where -D is default or requires --daemonize
		cmd2 := exec.Command(binPath, "--daemonize")
		if err := cmd2.Run(); err == nil {
			time.Sleep(250 * time.Millisecond)
			if isPHPFPMRunning() {
				return true
			}
		}
	}
	return isPHPFPMRunning()
}

// StartPHPFPMService dynamically detects the installed PHP-FPM service name
// and enables/starts it under OpenRC (Alpine), systemd, or direct daemonization in WSL/containers.
func StartPHPFPMService() bool {
	EnsurePHPFPMCompatibility()

	if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
		ensureOpenRCSoftlevel()
		var svcName string
		entries, err := os.ReadDir("/etc/init.d")
		if err == nil {
			var candidates []string
			for _, e := range entries {
				name := e.Name()
				if strings.HasPrefix(name, "php-fpm") {
					candidates = append(candidates, name)
				}
			}
			sort.Strings(candidates)
			if len(candidates) > 0 {
				svcName = candidates[len(candidates)-1]
			}
		}
		if svcName == "" {
			svcName = "php-fpm"
		}
		_ = exec.Command("rc-update", "add", svcName, "default").Run()
		if exec.Command("rc-service", svcName, "restart").Run() == nil || exec.Command("rc-service", svcName, "start").Run() == nil {
			if isPHPFPMRunning() {
				return true
			}
		}
		return startPHPFPMDirectFallback(svcName)
	}

	var verStr string
	if out, err := exec.Command("php", "-r", "echo PHP_MAJOR_VERSION.'.'.PHP_MINOR_VERSION;").Output(); err == nil {
		verStr = strings.TrimSpace(string(out))
	}

	var svcs []string
	if verStr != "" {
		svcs = append(svcs, "php"+verStr+"-fpm")
	}
	svcs = append(svcs, "php-fpm")

	for _, svc := range svcs {
		if IsSystemdRunning() {
			_ = exec.Command("systemctl", "enable", svc).Run()
			if exec.Command("systemctl", "restart", svc).Run() == nil || exec.Command("systemctl", "start", svc).Run() == nil {
				if isPHPFPMRunning() {
					ensureLiveSocketAccess()
					return true
				}
			}
		}
		if exec.Command("service", svc, "restart").Run() == nil || exec.Command("service", svc, "start").Run() == nil {
			if isPHPFPMRunning() {
				ensureLiveSocketAccess()
				return true
			}
		}
	}

	for _, svc := range svcs {
		if startPHPFPMDirectFallback(svc) {
			ensureLiveSocketAccess()
			return true
		}
	}
	ensureLiveSocketAccess()
	return isPHPFPMRunning()
}

func ensureLiveSocketAccess() {
	webUser := GetRuntimeWebUser()
	socketPatterns := []string{
		"/run/php/php*-fpm.sock",
		"/var/run/php/php*-fpm.sock",
		"/run/php-fpm/www.sock",
		"/var/run/php-fpm/www.sock",
	}
	for _, pat := range socketPatterns {
		matches, _ := filepath.Glob(pat)
		for _, sock := range matches {
			_ = exec.Command("chown", webUser+":"+webUser, sock).Run()
			_ = exec.Command("chmod", "0660", sock).Run()
		}
	}
}

// GetFastCGIPass returns the appropriate fastcgi_pass target for OpenResty
// (Unix domain socket or 127.0.0.1:9000).
func GetFastCGIPass() string {
	socketPatterns := []string{
		"/run/php/php*-fpm.sock",
		"/var/run/php/php*-fpm.sock",
		"/run/php-fpm/www.sock",
		"/var/run/php-fpm/www.sock",
	}
	for _, pat := range socketPatterns {
		matches, _ := filepath.Glob(pat)
		if len(matches) > 0 {
			sort.Strings(matches)
			return "unix:" + matches[len(matches)-1]
		}
	}
	return "127.0.0.1:9000"
}

// GetFastCGIParamsPath resolves the active fastcgi_params file path matching OpenResty's conf dir.
func GetFastCGIParamsPath() string {
	confDir := GetOpenRestyConfDir()
	candidates := []string{
		filepath.Join(confDir, "fastcgi_params"),
		"/usr/local/openresty/nginx/conf/fastcgi_params",
		"/etc/nginx/fastcgi_params",
	}
	for _, c := range candidates {
		if fileExistsLocal(c) {
			return c
		}
	}
	return "fastcgi_params"
}

// GetPHPFastCGIBlock returns a tuned location ~ \.php$ FastCGI block for OpenResty.
func GetPHPFastCGIBlock() string {
	passTarget := GetFastCGIPass()
	paramsPath := GetFastCGIParamsPath()
	return fmt.Sprintf(`    location ~ \.php$ {
        try_files $uri =404;
        include %s;
        fastcgi_pass %s;
        fastcgi_index index.php;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
    }
`, paramsPath, passTarget)
}

// OptimizePHPSettings tunes memory_limit and OPcache across all detected php.ini and conf.d locations.
func OptimizePHPSettings() {
	var iniFiles []string
	searchDirs := []string{"/etc/php", "/etc/php8", "/etc/php7", "/etc/php81", "/etc/php82", "/etc/php83", "/etc/php84", "/etc/php85"}
	for _, sdir := range searchDirs {
		matches, _ := filepath.Glob(sdir + "*")
		for _, m := range matches {
			_ = filepath.WalkDir(m, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if !d.IsDir() && d.Name() == "php.ini" {
					iniFiles = append(iniFiles, p)
				}
				return nil
			})
		}
	}
	if _, err := os.Stat("/etc/php.ini"); err == nil {
		iniFiles = append(iniFiles, "/etc/php.ini")
	}

	settings := []struct {
		key string
		val string
	}{
		{"memory_limit", "512M"},
		{"opcache.enable", "1"},
		{"opcache.enable_cli", "1"},
		{"opcache.interned_strings_buffer", "32"},
	}

	for _, pfile := range iniFiles {
		data, err := os.ReadFile(pfile)
		if err != nil {
			continue
		}
		content := string(data)
		for _, s := range settings {
			re := regexp.MustCompile("(?m)^[;[:space:]]*" + regexp.QuoteMeta(s.key) + "[[:space:]]*=.*$")
			if re.MatchString(content) {
				content = re.ReplaceAllString(content, s.key+" = "+s.val)
			} else {
				content += fmt.Sprintf("\n%s = %s\n", s.key, s.val)
			}
		}
		_ = WriteFileNoFollow(pfile, []byte(content), 0644, "")
	}

	var confDirs []string
	for _, sdir := range searchDirs {
		matches, _ := filepath.Glob(sdir + "*/conf.d")
		confDirs = append(confDirs, matches...)
	}
	tuningSnippet := []byte("; Krotek Script Hub PHP Optimization\nmemory_limit = 512M\nopcache.enable = 1\nopcache.enable_cli = 1\nopcache.interned_strings_buffer = 32\n")
	for _, cdir := range confDirs {
		_ = WriteFileNoFollow(filepath.Join(cdir, "99_krotek_tuning.ini"), tuningSnippet, 0644, "")
	}
}
