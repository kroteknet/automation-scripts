// ==============================================================================
// KROTEK SCRIPT HUB — SYSTEM EXECUTION & PROCESS HELPERS
// Service: CYBERPANEL (webpanel)
// File:    webpanel/cyberpanel/src/system/helpers.go
//
// Provides low-level execution wrappers, asynchronous command spinners, privilege verification, network IP resolution, and file writing with restricted permissions.
// ==============================================================================

package system

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/tls"
	"net/url"
	"os/user"
	"regexp"
	"strconv"

	"krotek-cyberpanel-installer/core"
	"krotek-cyberpanel-installer/logger"
	"krotek-cyberpanel-installer/tui"
)

// RunStep executes an external command with an interactive animated TUI spinner.
// All output is redirected to the active session logfile in /var/log/ for diagnostic audits.
func RunStep(msg string, command string, args ...string) bool {
	tui.StartSpinner(msg)
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a", "UCF_FORCE_CONFFOLD=1")
	if core.LogFile != nil {
		cmd.Stdout = core.LogFile
		cmd.Stderr = core.LogFile
	} else {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	}

	err := cmd.Run()
	if err == nil {
		tui.StopSpinner(0)
		return true
	}

	tui.StopSpinner(1)
	logger.LogError(fmt.Sprintf("Failed! Check log file: cat %s", core.LogFilePath))
	return false
}

func GetShell() string {
	if CommandExists("bash") {
		return "bash"
	}
	return "sh"
}

// StartService enables and starts a system daemon unit using the active init system (systemd, sysvinit, or OpenRC).
func StartService(svcName string) bool {
	if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
		targetSvc := svcName
		if core.SysInfo.Family == "alpine" && (svcName == "mysql" || svcName == "mysqld") {
			targetSvc = "mariadb"
		}
		_ = os.MkdirAll("/run/"+targetSvc, 0755)
		_ = exec.Command("rc-update", "add", targetSvc, "default").Run()
		return exec.Command("rc-service", targetSvc, "restart").Run() == nil || exec.Command("rc-service", targetSvc, "start").Run() == nil
	}

	if IsSystemdRunning() {
		_ = exec.Command("systemctl", "enable", svcName).Run()
		if exec.Command("systemctl", "restart", svcName).Run() == nil || exec.Command("systemctl", "start", svcName).Run() == nil {
			return true
		}
		_ = exec.Command("systemctl", "reset-failed", svcName).Run()
		_ = exec.Command("systemctl", "daemon-reload").Run()
		if exec.Command("systemctl", "restart", svcName).Run() == nil || exec.Command("systemctl", "start", svcName).Run() == nil {
			return true
		}
	}

	// SysVinit / WSL / Docker / fallback
	_ = exec.Command("systemctl", "enable", svcName).Run()
	_ = exec.Command("update-rc.d", svcName, "defaults").Run()
	_ = exec.Command("chkconfig", svcName, "on").Run()
	return exec.Command("service", svcName, "restart").Run() == nil ||
		exec.Command("service", svcName, "start").Run() == nil ||
		exec.Command("/etc/init.d/"+svcName, "restart").Run() == nil ||
		exec.Command("/etc/init.d/"+svcName, "start").Run() == nil
}

// RestartService restarts an active system daemon unit using systemctl, service, or rc-service.
func RestartService(svcName string) bool {
	if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
		targetSvc := svcName
		if core.SysInfo.Family == "alpine" && (svcName == "mysql" || svcName == "mysqld") {
			targetSvc = "mariadb"
		}
		_ = os.MkdirAll("/run/"+targetSvc, 0755)
		return exec.Command("rc-service", targetSvc, "restart").Run() == nil || exec.Command("rc-service", targetSvc, "start").Run() == nil
	}

	if IsSystemdRunning() {
		if exec.Command("systemctl", "restart", svcName).Run() == nil || exec.Command("systemctl", "start", svcName).Run() == nil {
			return true
		}
		_ = exec.Command("systemctl", "reset-failed", svcName).Run()
		_ = exec.Command("systemctl", "daemon-reload").Run()
		if exec.Command("systemctl", "restart", svcName).Run() == nil || exec.Command("systemctl", "start", svcName).Run() == nil {
			return true
		}
	}

	return exec.Command("service", svcName, "restart").Run() == nil ||
		exec.Command("service", svcName, "start").Run() == nil ||
		exec.Command("/etc/init.d/"+svcName, "restart").Run() == nil ||
		exec.Command("/etc/init.d/"+svcName, "start").Run() == nil
}

func ReloadService(svcName string) bool {
	if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
		targetSvc := svcName
		if core.SysInfo.Family == "alpine" && (svcName == "mysql" || svcName == "mysqld") {
			targetSvc = "mariadb"
		}
		return exec.Command("rc-service", targetSvc, "reload").Run() == nil || exec.Command("rc-service", targetSvc, "restart").Run() == nil
	}

	if IsSystemdRunning() {
		if exec.Command("systemctl", "reload", svcName).Run() == nil || exec.Command("systemctl", "restart", svcName).Run() == nil {
			return true
		}
	}

	return exec.Command("service", svcName, "reload").Run() == nil ||
		exec.Command("service", svcName, "restart").Run() == nil ||
		exec.Command("/etc/init.d/"+svcName, "reload").Run() == nil ||
		exec.Command("/etc/init.d/"+svcName, "restart").Run() == nil
}

func StopService(svcName string) bool {
	if core.SysInfo.InitSystem == "openrc" || core.SysInfo.Family == "alpine" {
		targetSvc := svcName
		if core.SysInfo.Family == "alpine" && (svcName == "mysql" || svcName == "mysqld") {
			targetSvc = "mariadb"
		}
		return exec.Command("rc-service", targetSvc, "stop").Run() == nil
	}

	if IsSystemdRunning() {
		if exec.Command("systemctl", "stop", svcName).Run() == nil {
			return true
		}
	}

	return exec.Command("service", svcName, "stop").Run() == nil ||
		exec.Command("/etc/init.d/"+svcName, "stop").Run() == nil
}

func InitMariaDBDatadir() {
	if core.SysInfo.Family == "alpine" {
		if _, err := os.Stat("/var/lib/mysql/mysql"); os.IsNotExist(err) {
			_ = RunMultiStep("Initializing MariaDB data directory", "mariadb-install-db --user=mysql --datadir=/var/lib/mysql 2>/dev/null || mysql_install_db --user=mysql --datadir=/var/lib/mysql 2>/dev/null || true")
		}
	}
}

// RunMultiStep executes a complex shell pipeline with an interactive animated spinner.
func RunMultiStep(msg string, shellScript string) bool {
	tui.StartSpinner(msg)
	cmd := exec.Command(GetShell(), "-c", shellScript)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a", "UCF_FORCE_CONFFOLD=1")
	if core.LogFile != nil {
		cmd.Stdout = core.LogFile
		cmd.Stderr = core.LogFile
	} else {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	}

	err := cmd.Run()
	if err == nil {
		tui.StopSpinner(0)
		return true
	}

	tui.StopSpinner(1)
	logger.LogError(fmt.Sprintf("Failed! Check log file: cat %s", core.LogFilePath))
	return false
}

func RunStepOrAbort(msg string, command string, args ...string) bool {
	if !RunStep(msg, command, args...) {
		HandleInstallFailure(msg)
		return false
	}
	return true
}

func RunMultiStepOrAbort(msg string, shellScript string) bool {
	if !RunMultiStep(msg, shellScript) {
		HandleInstallFailure(msg)
		return false
	}
	return true
}

func HandleInstallFailure(failedStep string) {
	var errLines []string
	if core.LogFilePath != "" {
		if data, err := os.ReadFile(core.LogFilePath); err == nil {
			lines := strings.Split(string(data), "\n")
			for i := len(lines) - 1; i >= 0; i-- {
				l := strings.TrimSpace(lines[i])
				if l == "" {
					continue
				}
				cleanL := core.SanitizeTerminalText(l)
				if strings.Contains(cleanL, "Check log file: cat") {
					continue
				}
				if strings.Contains(cleanL, "[ERROR]") || strings.Contains(cleanL, "E:") || strings.Contains(cleanL, "Failed") || strings.Contains(cleanL, "Error") {
					if len(cleanL) > 110 {
						cleanL = cleanL[:110] + "..."
					}
					errLines = append([]string{cleanL}, errLines...)
					if len(errLines) >= 5 {
						break
					}
				}
			}
		}
	}

	var boxLines []string
	boxLines = append(boxLines, tui.FormatKV("Failed Step", core.SanitizeTerminalText(failedStep), core.RED+core.BOLD))
	if core.LogFilePath != "" {
		boxLines = append(boxLines, "")
		boxLines = append(boxLines, core.BOLD+core.CYAN+"LOG FILE & QUICK COMMANDS:"+core.NC)
		boxLines = append(boxLines, fmt.Sprintf("  %s%s Path :%s %s%s%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, core.LogFilePath, core.NC))
		boxLines = append(boxLines, fmt.Sprintf("  %s%s View :%s %scat %s%s", core.GREEN, core.Sym.Pointer, core.NC, core.BOLD+core.GREEN, core.LogFilePath, core.NC))
		boxLines = append(boxLines, fmt.Sprintf("  %s%s Tail :%s %stail -n 25 %s%s", core.BLUE, core.Sym.Pointer, core.NC, core.BOLD+core.BLUE, core.LogFilePath, core.NC))
	}
	if len(errLines) > 0 {
		boxLines = append(boxLines, "")
		boxLines = append(boxLines, core.BOLD+core.RED+"LOGGED ERRORS / DIAGNOSTICS:"+core.NC)
		for _, el := range errLines {
			boxLines = append(boxLines, fmt.Sprintf("  %s%s%s %s%s%s", core.RED, core.Sym.Pointer, core.NC, core.RED, el, core.NC))
		}
	}
	boxLines = append(boxLines, "")
	boxLines = append(boxLines, core.YELLOW+"Installation aborted to prevent saving an invalid state."+core.NC)

	tui.EnterAltScreen()
	tui.PrintInBox("INSTALLATION FAILED", boxLines)
}

// GetLocalIP discovers the primary non-loopback IPv4 address of the local machine.
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

// GetPublicIP queries an external resolver to determine the publicly routable WAN IP address.
func GetPublicIP() string {
	client := http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	resp, err := client.Get("https://api.ipify.org")
	if err == nil {
		defer resp.Body.Close()
		if body, err := io.ReadAll(io.LimitReader(resp.Body, 128)); err == nil {
			ip := strings.TrimSpace(string(body))
			if ip != "" && net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	return GetLocalIP()
}

func copyBinaryFile(src, dst string) error {
	cleanDst := filepath.Clean(strings.TrimSpace(dst))
	if err := validateTargetNoSymlink(cleanDst, true); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := openFileNoFollow(cleanDst, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	if st, sErr := out.Stat(); sErr != nil || !st.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular binary destination: %s", cleanDst)
	}
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Chmod(0755)
}

// CreateGlobalShortcut symlinks or copies the compiled management binary into /usr/local/bin for permanent system access.
func CreateGlobalShortcut() {
	execPath, err := os.Executable()
	if err != nil {
		return
	}
	_ = os.MkdirAll("/usr/local/bin", 0755)

	managerTarget := "/usr/local/bin/cyberpanel-manager"
	installerTarget := "/usr/local/bin/cyberpanel-installer"

	if execPath != managerTarget {
		_ = os.Remove(managerTarget)
		if err := copyBinaryFile(execPath, managerTarget); err == nil {
			_ = os.Chmod(managerTarget, 0755)
		}
	}

	if execPath != installerTarget {
		_ = os.Remove(installerTarget)
		_ = os.Symlink(managerTarget, installerTarget)
	}
}

// CommandExists checks whether a binary executable is present in the host system PATH.
func CommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func GetToolBadge(cmd string) string {
	w := tui.GetTermWidth()
	if CommandExists(cmd) {
		if w < 75 {
			return core.GREEN + "[OK]" + core.NC
		}
		return core.GREEN + "[INSTALLED]" + core.NC
	}
	if w < 75 {
		return core.RED + "[NO]" + core.NC
	}
	return core.RED + "[NOT INSTALLED]" + core.NC
}

func RunSilent(cmd string, args ...string) (string, error) {
	out, err := exec.Command(cmd, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// startServiceDirectBinary launches a service daemon directly as a background process.
// Used as a last-resort fallback in WSL2 environments where neither systemd nor OpenRC
// is booted as PID 1 (e.g. RHEL/AlmaLinux without [boot] systemd=true, Alpine chroots).
func startServiceDirectBinary(svcName string) bool {
	// Ensure required runtime dirs exist
	_ = os.MkdirAll("/run/"+svcName, 0755)
	_ = os.MkdirAll("/var/log/"+svcName, 0755)

	// Kill any stale process
	_ = exec.Command("pkill", "-f", svcName+": master").Run()
	_ = exec.Command("rm", "-f", "/run/"+svcName+"/"+svcName+".pid", "/run/"+svcName+".pid").Run()

	candidates := []string{
		svcName,
		"/usr/sbin/" + svcName,
		"/usr/local/sbin/" + svcName,
		"/usr/bin/" + svcName,
		"/usr/local/bin/" + svcName,
	}
	// PHP-FPM variants (php-fpm80, php-fpm82, php-fpm84 ...)
	if strings.HasPrefix(svcName, "php-fpm") || strings.HasPrefix(svcName, "php8") {
		for _, v := range []string{"84", "83", "82", "81", "80"} {
			candidates = append(candidates,
				"php-fpm"+v,
				"/usr/sbin/php-fpm"+v,
				"/usr/local/sbin/php-fpm"+v,
			)
		}
	}
	for _, bin := range candidates {
		if _, err := exec.LookPath(bin); err == nil {
			if exec.Command(bin).Run() == nil {
				return true
			}
		}
	}
	return false
}

// ValidateSafeDirectoryPath ensures that path is a valid absolute path and does not target
// root or critical system directories (/etc, /root, /bin, /sbin, /usr, /lib, /sys, /proc, /dev, /boot, /run).
func ValidateSafeDirectoryPath(pathStr string) error {
	trimmed := strings.TrimSpace(pathStr)
	if trimmed == "" {
		return fmt.Errorf("directory path cannot be empty")
	}
	if strings.ContainsAny(trimmed, "\r\n\x00") {
		return fmt.Errorf("directory path contains invalid control characters")
	}
	cleaned := filepath.Clean(trimmed)
	if !filepath.IsAbs(cleaned) || !strings.HasPrefix(cleaned, "/") {
		return fmt.Errorf("directory path must be an absolute path: %s", pathStr)
	}
	if cleaned == "/" {
		return fmt.Errorf("cannot use root filesystem '/' as directory path")
	}
	criticalRoots := []string{
		"/bin", "/boot", "/dev", "/etc", "/lib", "/lib32", "/lib64", "/libx32",
		"/proc", "/root", "/run", "/sbin", "/sys", "/usr",
	}
	for _, cr := range criticalRoots {
		if cleaned == cr || strings.HasPrefix(cleaned, cr+"/") {
			return fmt.Errorf("directory path cannot be within system directory %s", cr)
		}
	}
	if cleaned == "/var" || cleaned == "/home" || cleaned == "/srv" || cleaned == "/opt" || cleaned == "/tmp" {
		return fmt.Errorf("cannot use system root container '%s' directly; specify a subdirectory", cleaned)
	}
	return nil
}

// RunStepInDir executes an external command directly in a specific working directory without a shell.
func RunStepInDir(dir string, msg string, command string, args ...string) bool {
	tui.StartSpinner(msg)
	cmd := exec.Command(command, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a", "UCF_FORCE_CONFFOLD=1")
	if core.LogFile != nil {
		cmd.Stdout = core.LogFile
		cmd.Stderr = core.LogFile
	} else {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	}

	err := cmd.Run()
	if err == nil {
		tui.StopSpinner(0)
		return true
	}

	tui.StopSpinner(1)
	logger.LogError(fmt.Sprintf("Failed! Check log file: cat %s", core.LogFilePath))
	return false
}

// RunStepInDirOrAbort executes RunStepInDir and invokes HandleInstallFailure on error.
func RunStepInDirOrAbort(dir string, msg string, command string, args ...string) bool {
	if !RunStepInDir(dir, msg, command, args...) {
		HandleInstallFailure(msg)
		return false
	}
	return true
}

// RunDockerCompose validates composeDir and runs docker compose / docker-compose directly without sh -c.
func RunDockerCompose(composeDir, msg string, abortOnFail bool, subArgs ...string) bool {
	cleanDir := filepath.Clean(strings.TrimSpace(composeDir))
	if err := ValidateSafeComposeDir(cleanDir); err != nil {
		logger.LogError("Unsafe Docker Compose directory: " + err.Error())
		if abortOnFail {
			HandleInstallFailure(msg)
		}
		return false
	}
	bin := "docker"
	args := append([]string{"compose"}, subArgs...)
	if exec.Command("docker", "compose", "version").Run() != nil && CommandExists("docker-compose") {
		bin = "docker-compose"
		args = subArgs
	}
	if abortOnFail {
		return RunStepInDirOrAbort(cleanDir, msg, bin, args...)
	}
	return RunStepInDir(cleanDir, msg, bin, args...)
}



// IsSymlinkPath returns true when path exists as a symbolic link.
func IsSymlinkPath(pathStr string) bool {
	if pathStr == "" {
		return false
	}
	fi, err := os.Lstat(filepath.Clean(pathStr))
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// ValidateSafeComposeDir ensures a Docker Compose directory is absolute, clean, not a symlink,
// free of control/shell metacharacters, and scoped safely outside arbitrary system roots.
func ValidateSafeComposeDir(pathStr string) error {
	trimmed := strings.TrimSpace(pathStr)
	if trimmed == "" {
		return fmt.Errorf("compose directory path cannot be empty")
	}
	if strings.ContainsAny(trimmed, "\r\n\x00;|&$`\\\"'<>()") {
		return fmt.Errorf("compose directory path contains invalid characters")
	}
	cleaned := filepath.Clean(trimmed)
	if !filepath.IsAbs(cleaned) || !strings.HasPrefix(cleaned, "/") {
		return fmt.Errorf("compose directory must be an absolute path: %s", pathStr)
	}
	if IsSymlinkPath(cleaned) {
		return fmt.Errorf("compose directory must not be a symbolic link: %s", cleaned)
	}
	// Permit dedicated /etc/openresty-* or /etc/openresty/* Docker compose locations
	// while never confusing/allowing native /etc/openresty root or arbitrary /etc files.
	if regexp.MustCompile(`^/etc/[a-z0-9-]+-docker$`).MatchString(cleaned) {
		return nil
	}
	return ValidateSafeDirectoryPath(cleaned)
}

// ValidateProxyTarget validates reverse-proxy backend URLs to block whitespace,
// embedded credentials, fragments, cloud metadata SSRF targets, and directive injection characters.
func ValidateProxyTarget(target string) bool {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" || len(trimmed) > 2048 {
		return false
	}
	if strings.ContainsAny(trimmed, ";{}\"$'`\\") {
		return false
	}
	for _, r := range trimmed {
		if r <= 0x20 || r == 0x7f {
			return false
		}
	}
	u, err := url.ParseRequestURI(trimmed)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	if portStr := u.Port(); portStr != "" {
		pNum, pErr := strconv.Atoi(portStr)
		if pErr != nil || pNum < 1 || pNum > 65535 {
			return false
		}
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	switch host {
	case "metadata", "metadata.google.internal", "metadata.goog", "metadata.azure.com", "instance-data", "169.254.169.254", "100.100.100.200":
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			ip = v4
		}
		if ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
			return false
		}
		if len(ip) == 4 && ip[0] == 169 && ip[1] == 254 {
			return false
		}
		if ip.Equal(net.ParseIP("100.100.100.200")) || ip.Equal(net.ParseIP("fd00:ec2::254")) {
			return false
		}
	} else {
		// Block hex/octal/integer IP obfuscation forms (e.g., 2852039166, 0xa9fea9fe, 0251.0376.0251.0376)
		allNumericParts := true
		for _, part := range strings.Split(host, ".") {
			if part == "" || !numericHostPartRegex.MatchString(part) {
				allNumericParts = false
				break
			}
		}
		if allNumericParts {
			return false
		}
	}
	return true
}

var numericHostPartRegex = regexp.MustCompile(`^(?:0x[0-9a-f]+|[0-9]+)$`)

func lookupUserIDs(username string) (int, int, bool) {
	u, err := user.Lookup(strings.TrimSpace(username))
	if err != nil {
		return 0, 0, false
	}
	uid, uErr := strconv.Atoi(u.Uid)
	gid, gErr := strconv.Atoi(u.Gid)
	if uErr != nil || gErr != nil {
		return 0, 0, false
	}
	return uid, gid, true
}

func validateTargetNoSymlink(clean string, mustBeRegular bool) error {
	if clean == "" || clean == "." || strings.ContainsRune(clean, '\x00') {
		return fmt.Errorf("file path cannot be empty or contain NUL")
	}
	fi, err := os.Lstat(clean)
	if err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to write through symbolic link: %s", clean)
		}
		if mustBeRegular && !fi.Mode().IsRegular() {
			return fmt.Errorf("refusing to write non-regular file: %s", clean)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

// WriteFileNoFollow atomically opens a file with O_NOFOLLOW (preventing symlink overwrite
// privilege escalation when root writes inside web-user-owned directories), verifies regular
// file status, writes content, and sets explicit file mode and ownership on the open descriptor.
func WriteFileNoFollow(filePath string, data []byte, perm os.FileMode, ownerUser string) error {
	clean := filepath.Clean(strings.TrimSpace(filePath))
	if err := validateTargetNoSymlink(clean, true); err != nil {
		return err
	}
	f, err := openFileNoFollow(clean, perm)
	if err != nil {
		return err
	}
	st, statErr := f.Stat()
	if statErr != nil || !st.Mode().IsRegular() {
		_ = f.Close()
		if statErr != nil {
			return statErr
		}
		return fmt.Errorf("refusing non-regular file target: %s", clean)
	}
	_, writeErr := f.Write(data)
	chmodErr := f.Chmod(perm)
	if ownerUser != "" {
		if uid, gid, ok := lookupUserIDs(ownerUser); ok {
			_ = f.Chown(uid, gid)
		}
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if chmodErr != nil {
		return chmodErr
	}
	return closeErr
}

// AppendFileNoFollow safely opens and appends data to a regular file without following symlinks.
func AppendFileNoFollow(filePath string, data []byte, perm os.FileMode) error {
	clean := filepath.Clean(strings.TrimSpace(filePath))
	if err := validateTargetNoSymlink(clean, true); err != nil {
		return err
	}
	root, err := os.OpenRoot(filepath.Dir(clean))
	if err != nil {
		return err
	}
	defer root.Close()
	f, err := root.OpenFile(filepath.Base(clean), os.O_WRONLY|os.O_CREATE|os.O_APPEND, perm)
	if err != nil {
		return err
	}
	st, statErr := f.Stat()
	if statErr != nil || !st.Mode().IsRegular() {
		_ = f.Close()
		if statErr != nil {
			return statErr
		}
		return fmt.Errorf("refusing non-regular append target: %s", clean)
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// HttpFetchBytes performs a bounded HTTPS fetch enforcing TLS 1.2+, strict redirect downgrade
// prevention, timeouts, and maximum body size limits.
func HttpFetchBytes(rawURL string, maxBytes int64) ([]byte, int, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return nil, 0, fmt.Errorf("refusing non-HTTPS or invalid URL: %s", rawURL)
	}
	client := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("refusing HTTPS-to-HTTP redirect downgrade")
			}
			return nil
		},
	}
	return httpFetchBytesWithClient(client, rawURL, maxBytes)
}

func httpFetchBytesWithClient(client *http.Client, rawURL string, maxBytes int64) ([]byte, int, error) {
	if maxBytes <= 0 {
		maxBytes = 10 * 1024 * 1024
	}
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if int64(len(body)) > maxBytes {
		return nil, resp.StatusCode, fmt.Errorf("response body exceeded %d byte limit", maxBytes)
	}
	return body, resp.StatusCode, nil
}

// ValidateTarGzArchive scans a .tar.gz archive in Go to block path traversal (..), device/FIFO nodes,
// suspicious symlink targets, and archive bombs prior to invoking tar extraction.
func ValidateTarGzArchive(archivePath string, maxTotalBytes int64) error {
	if maxTotalBytes <= 0 {
		maxTotalBytes = 2 * 1024 * 1024 * 1024
	}
	clean := filepath.Clean(strings.TrimSpace(archivePath))
	fi, err := os.Lstat(clean)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular or symlinked archive: %s", clean)
	}
	f, err := os.Open(clean)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var totalBytes int64
	entryCount := 0
	const maxEntries = 100000
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		entryCount++
		if entryCount > maxEntries {
			return fmt.Errorf("archive exceeds maximum entry limit (%d)", maxEntries)
		}
		if strings.ContainsRune(hdr.Name, '\x00') || hasPathTraversalSegment(hdr.Name) {
			return fmt.Errorf("unsafe tar entry path: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			return fmt.Errorf("unsafe special device/fifo entry in archive: %s", hdr.Name)
		case tar.TypeSymlink, tar.TypeLink:
			if strings.ContainsRune(hdr.Linkname, '\x00') || hasPathTraversalSegment(hdr.Linkname) {
				return fmt.Errorf("unsafe link target in archive: %s -> %s", hdr.Name, hdr.Linkname)
			}
		}
		if hdr.Size < 0 {
			return fmt.Errorf("invalid negative size in tar entry: %s", hdr.Name)
		}
		totalBytes += hdr.Size
		if totalBytes > maxTotalBytes {
			return fmt.Errorf("archive uncompressed size exceeds %d byte limit", maxTotalBytes)
		}
	}
	return nil
}

// ExtractZipSafe safely extracts a ZIP archive into destDir while enforcing zip-slip,
// symlink rejection, entry-count caps, and per-file/total decompression limits.
func ExtractZipSafe(zipPath, destDir string, maxFiles int, maxFileBytes, maxTotalBytes int64) error {
	if maxFiles <= 0 {
		maxFiles = 10000
	}
	if maxFileBytes <= 0 {
		maxFileBytes = 128 * 1024 * 1024
	}
	if maxTotalBytes <= 0 {
		maxTotalBytes = 512 * 1024 * 1024
	}
	cleanDest := filepath.Clean(strings.TrimSpace(destDir))
	if cleanDest == "" || cleanDest == "." {
		return fmt.Errorf("destination directory cannot be empty")
	}
	if IsSymlinkPath(cleanDest) {
		return fmt.Errorf("destination directory must not be a symlink: %s", cleanDest)
	}
	if err := os.MkdirAll(cleanDest, 0755); err != nil {
		return err
	}
	zr, err := zip.OpenReader(filepath.Clean(strings.TrimSpace(zipPath)))
	if err != nil {
		return err
	}
	defer zr.Close()
	if len(zr.File) > maxFiles {
		return fmt.Errorf("zip archive has too many files (%d > %d)", len(zr.File), maxFiles)
	}
	var totalWritten int64
	for _, entry := range zr.File {
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink entry inside zip archive: %s", entry.Name)
		}
		if strings.ContainsRune(entry.Name, '\x00') || strings.HasPrefix(entry.Name, "/") || strings.HasPrefix(entry.Name, "\\") || filepath.IsAbs(entry.Name) || hasPathTraversalSegment(entry.Name) {
			return fmt.Errorf("refusing unsafe zip entry path: %s", entry.Name)
		}
		target := filepath.Join(cleanDest, filepath.FromSlash(entry.Name))
		rel, relErr := filepath.Rel(cleanDest, target)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("zip-slip path escape blocked: %s", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		rc, err := entry.Open()
		if err != nil {
			return err
		}
		limited := io.LimitReader(rc, maxFileBytes+1)
		data, readErr := io.ReadAll(limited)
		_ = rc.Close()
		if readErr != nil {
			return readErr
		}
		if int64(len(data)) > maxFileBytes {
			return fmt.Errorf("zip entry %s exceeds per-file limit (%d bytes)", entry.Name, maxFileBytes)
		}
		totalWritten += int64(len(data))
		if totalWritten > maxTotalBytes {
			return fmt.Errorf("zip archive total uncompressed size exceeds limit (%d bytes)", maxTotalBytes)
		}
		perm := entry.Mode().Perm()
		if perm == 0 {
			perm = 0644
		}
		if err := WriteFileNoFollow(target, data, perm, ""); err != nil {
			return err
		}
	}
	return nil
}

func hasPathTraversalSegment(p string) bool {
	norm := strings.ReplaceAll(p, "\\", "/")
	for _, seg := range strings.Split(norm, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// SecureWebRootPermissions safely applies directory (0755) and file (0644) permissions
// plus user/group ownership under a validated web root without ever traversing symlinks.
func SecureWebRootPermissions(rootDir, webUser string) {
	cleanRoot := filepath.Clean(strings.TrimSpace(rootDir))
	if err := ValidateSafeDirectoryPath(cleanRoot); err != nil {
		return
	}
	for _, parent := range []string{"/var", "/var/www"} {
		if fi, err := os.Lstat(parent); err == nil && fi.IsDir() && fi.Mode()&os.ModeSymlink == 0 {
			_ = os.Chmod(parent, 0755)
		}
	}
	rootInfo, err := os.Lstat(cleanRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return
	}
	uid, gid, hasIDs := lookupUserIDs(webUser)
	_ = filepath.WalkDir(cleanRoot, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			_ = os.Chmod(p, 0755)
		} else if d.Type().IsRegular() {
			_ = os.Chmod(p, 0644)
		} else {
			return nil
		}
		if hasIDs {
			_ = os.Lchown(p, uid, gid)
		}
		return nil
	})
}
