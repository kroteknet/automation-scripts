// ==============================================================================
// KROTEK SCRIPT HUB — DYNAMIC PHP RUNTIME & FPM POOL CONFIGURATOR
// Service: DRUPAL (cms)
// File:    cms/drupal/src/system/php.go
//
// Detects and installs the latest stable PHP version and required extensions across distributions, configuring optimized Unix sockets and OPcache tuning.
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

	"krotek-drupal-installer/core"
	"krotek-drupal-installer/logger"
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
		re := regexp.MustCompile(`(php(\d+))-fpm`)
		m := re.FindStringSubmatch(string(infoOut))
		if len(m) >= 2 {
			return m[1]
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

// InstallPHPStack dynamically discovers and installs the latest stable PHP version
// and essential extensions for the current operating system without hardcoding.
func InstallPHPStack() (string, bool) {
	switch core.SysInfo.Family {
	case "alpine":
		phpPrefix := DetectLatestAlpinePHP()
		pkgs := []string{
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
		args := append([]string{"add", "--quiet"}, pkgs...)
		if !RunStep(fmt.Sprintf("Installing Latest PHP Stack (%s) & Extensions", phpPrefix), "apk", args...) {
			if !RunStep(fmt.Sprintf("Installing Core PHP (%s) & FPM", phpPrefix), "apk", "add", "--quiet", phpPrefix, phpPrefix+"-fpm", phpPrefix+"-opcache") {
				return phpPrefix, false
			}
		}
		return phpPrefix, true

	case "debian":
		pkgs := []string{"php-fpm", "php-cli", "php-common", "php-mbstring", "php-xml", "php-curl", "php-mysql", "php-opcache", "php-zip"}
		args := append([]string{"install", "-y", "-q"}, pkgs...)
		ok := RunStep("Installing Latest Stable PHP & Extensions", "apt-get", args...)
		return "php", ok

	case "rhel":
		mgr := "dnf"
		if !CommandExists("dnf") {
			mgr = "yum"
		}
		pkgs := []string{"php", "php-fpm", "php-cli", "php-common", "php-mbstring", "php-xml", "php-mysqlnd", "php-opcache", "php-pecl-zip"}
		args := append([]string{"install", "-y", "-q"}, pkgs...)
		ok := RunStep("Installing Latest Stable PHP & Extensions", mgr, args...)
		return "php", ok

	default:
		return "php", false
	}
}

// StartPHPFPMService dynamically detects the installed PHP-FPM service name
// and enables/starts it under both OpenRC (Alpine) and systemd.
func StartPHPFPMService() bool {
	if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
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
		_ = exec.Command("rc-service", svcName, "restart").Run()
		_ = exec.Command("rc-service", svcName, "start").Run()
		return true
	}

	// systemd
	var verStr string
	if out, err := exec.Command("php", "-r", "echo PHP_MAJOR_VERSION.'.'.PHP_MINOR_VERSION;").Output(); err == nil {
		verStr = strings.TrimSpace(string(out))
	}

	if verStr != "" {
		StartService("php" + verStr + "-fpm")
	}
	StartService("php-fpm")
	return true
}

// GetFastCGIPass returns the appropriate fastcgi_pass target for Nginx
// (unix domain socket or 127.0.0.1:9000).
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

// GetPHPFastCGIBlock returns a tuned location ~ \.php$ Nginx block.
func GetPHPFastCGIBlock() string {
	passTarget := GetFastCGIPass()
	return fmt.Sprintf(`    location ~ \.php$ {
        include fastcgi_params;
        fastcgi_pass %s;
        fastcgi_index index.php;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
    }
`, passTarget)
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
