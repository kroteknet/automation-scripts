// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Minecraft PaperMC Dedicated Game Server (minecraft)
//
// This package orchestrates automated pre-flight checks, package repository setup,
// configuration templating, service enablement (systemd/openrc), and firewall rules.
// Supports both interactive TUI step-by-step guidance and headless automation.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"krotek-minecraft-installer/core"
	"krotek-minecraft-installer/logger"
	"krotek-minecraft-installer/system"
	"krotek-minecraft-installer/tui"
)

// InstallConfig is the fully resolved description of one deployment.
type InstallConfig struct {
	Engine         string
	EngineType     string
	Version        string
	JarDownloadURL string
	BuildID        string // upstream build id (Paper build, Purpur build, Fabric loader, NeoForge version)
	LoaderVersion  string // Fabric loader / NeoForge version when applicable
	SHA256         string // optional upstream-published checksum of the download
	JavaVersion    int
	JavaBin        string
	RAM            string
	Port           string
	Mode           string
}

// IsSameMinorVersion wraps the compat module helper so install code can use it
// without creating a circular import. See modules/compat.go for full details.
func IsSameMinorVersion(v1, v2 string) bool {
	p1 := parseVersionParts(v1)
	p2 := parseVersionParts(v2)
	if len(p1) < 2 || len(p2) < 2 {
		return false
	}
	if p1[0] == 1 && p2[0] == 1 {
		return p1[1] == p2[1]
	}
	return p1[0] == p2[0]
}

func parseVersionParts(v string) []int {
	clean := regexp.MustCompile(`[^0-9.]`).ReplaceAllString(v, "")
	parts := strings.Split(clean, ".")
	var res []int
	for _, p := range parts {
		if val, err := strconv.Atoi(p); err == nil {
			res = append(res, val)
		} else {
			res = append(res, 0)
		}
	}
	return res
}

func compareVersions(v1, v2 string) int {
	p1 := parseVersionParts(v1)
	p2 := parseVersionParts(v2)
	maxLen := len(p1)
	if len(p2) > maxLen {
		maxLen = len(p2)
	}
	for i := 0; i < maxLen; i++ {
		val1 := 0
		if i < len(p1) {
			val1 = p1[i]
		}
		val2 := 0
		if i < len(p2) {
			val2 = p2[i]
		}
		if val1 > val2 {
			return 1
		}
		if val1 < val2 {
			return -1
		}
	}
	return 0
}

func isJavaVersionSufficient(javaPath string, requiredVersion int) bool {
	if _, err := os.Stat(javaPath); err != nil {
		return false
	}
	cmd := exec.Command(javaPath, "-version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	s := string(out)
	re := regexp.MustCompile(`"(1\.)?(\d+)`)
	matches := re.FindStringSubmatch(s)
	if len(matches) >= 3 {
		v, _ := strconv.Atoi(matches[2])
		return v >= requiredVersion
	}
	return false
}

func FindCompatibleJavaBinary(requiredVersion int) string {
	arch := "x64"
	if core.SysInfo.Architecture == "arm64" || core.SysInfo.Architecture == "aarch64" {
		arch = "aarch64"
	}

	optJava := fmt.Sprintf("/opt/java-%d/bin/java", requiredVersion)
	if isJavaVersionSufficient(optJava, requiredVersion) {
		return optJava
	}

	candidates := []string{
		optJava,
		"/opt/java/bin/java",
		fmt.Sprintf("/usr/lib/jvm/temurin-%d-jre-%s/bin/java", requiredVersion, arch),
		fmt.Sprintf("/usr/lib/jvm/temurin-%d-jdk-%s/bin/java", requiredVersion, arch),
		fmt.Sprintf("/usr/lib/jvm/java-%d-openjdk-%s/bin/java", requiredVersion, arch),
		fmt.Sprintf("/usr/lib/jvm/java-%d-openjdk/bin/java", requiredVersion),
		fmt.Sprintf("/usr/lib/jvm/openjdk-%d/bin/java", requiredVersion),
		fmt.Sprintf("/usr/lib/jvm/java-%d-oracle/bin/java", requiredVersion),
		"/usr/bin/java",
	}

	if out, err := exec.Command("which", "java").Output(); err == nil {
		candidates = append(candidates, strings.TrimSpace(string(out)))
	}

	for _, cand := range candidates {
		if cand != "" && isJavaVersionSufficient(cand, requiredVersion) {
			return cand
		}
	}

	return ""
}

func EnsureJavaInstalled(requiredJava int) (string, bool) {
	if bin := FindCompatibleJavaBinary(requiredJava); bin != "" {
		logger.LogSuccess(fmt.Sprintf("Found compatible Java runtime at %s (>= Java %d).", bin, requiredJava))
		return bin, true
	}

	logger.LogInfo(fmt.Sprintf("Minecraft requires Java %d runtime. Installing...", requiredJava))

	arch := "x64"
	if core.SysInfo.Architecture == "arm64" || core.SysInfo.Architecture == "aarch64" {
		arch = "aarch64"
	}

	// 1. Try system package manager
	var pkgs []string
	switch core.SysInfo.Family {
	case "debian":
		pkgs = []string{fmt.Sprintf("openjdk-%d-jre-headless", requiredJava), "curl", "tar", "ca-certificates"}
	case "rhel":
		if requiredJava == 8 {
			pkgs = []string{"java-1.8.0-openjdk-headless", "curl", "tar"}
		} else {
			pkgs = []string{fmt.Sprintf("java-%d-openjdk-headless", requiredJava), "curl", "tar"}
		}
	case "alpine":
		pkgs = []string{fmt.Sprintf("openjdk%d-jre-headless", requiredJava), "curl", "tar"}
	default:
		pkgs = []string{fmt.Sprintf("openjdk-%d-jre-headless", requiredJava), "curl", "tar"}
	}

	_ = system.InstallPackages(pkgs...)

	if bin := FindCompatibleJavaBinary(requiredJava); bin != "" {
		return bin, true
	}

	// 2. If Debian/Ubuntu, try configuring Adoptium APT repository
	if core.SysInfo.Family == "debian" {
		tui.StartSpinner(fmt.Sprintf("Configuring Eclipse Temurin APT repository for Java %d...", requiredJava))
		adoptiumRepoCmd := fmt.Sprintf(`
			apt-get install -y -q ca-certificates curl gnupg 2>/dev/null || true
			mkdir -p /etc/apt/keyrings
			curl --proto '=https' --tlsv1.2 -fsSL https://packages.adoptium.net/artifactory/api/gpg/key/public | gpg --dearmor -o /etc/apt/keyrings/adoptium.gpg --yes 2>/dev/null || true
			DISTRO_CODE=$(awk -F= '/^VERSION_CODENAME/{print$2}' /etc/os-release 2>/dev/null || echo 'bookworm')
			echo "deb [signed-by=/etc/apt/keyrings/adoptium.gpg] https://packages.adoptium.net/artifactory/deb ${DISTRO_CODE} main" > /etc/apt/sources.list.d/adoptium.list
			apt-get update -q 2>/dev/null || true
			apt-get install -y -q temurin-%d-jre 2>/dev/null || true
		`, requiredJava)
		_ = exec.Command(system.GetShell(), "-c", adoptiumRepoCmd).Run()
		tui.StopSpinner(0)

		if bin := FindCompatibleJavaBinary(requiredJava); bin != "" {
			return bin, true
		}
	}

	// 3. Fail-Safe: Direct Eclipse Temurin official JRE tarball deployment into /opt/java-{version}
	targetDir := fmt.Sprintf("/opt/java-%d", requiredJava)
	osName := "linux"
	if core.SysInfo.Family == "alpine" {
		osName = "alpine-linux"
	}
	downloadURL := fmt.Sprintf("https://api.adoptium.net/v3/binary/latest/%d/ga/%s/%s/jre/hotspot/normal/eclipse", requiredJava, osName, arch)

	tmpDir, tmpErr := os.MkdirTemp("", "krotek-temurin-*")
	if tmpErr != nil {
		logger.LogError(fmt.Sprintf("Failed to create secure temp dir for Java %d download: %v", requiredJava, tmpErr))
		return "", false
	}
	_ = os.Chmod(tmpDir, 0700)
	defer os.RemoveAll(tmpDir)
	tarPath := filepath.Join(tmpDir, "temurin.tar.gz")

	if !system.RunStep(fmt.Sprintf("Downloading Eclipse Temurin OpenJDK %d tarball", requiredJava),
		"curl", "--proto", "=https", "--tlsv1.2", "-fSL", downloadURL, "-o", tarPath) {
		logger.LogError(fmt.Sprintf("Failed to download Java %d.", requiredJava))
		return "", false
	}
	if valErr := system.ValidateTarGzArchive(tarPath, 1024*1024*1024); valErr != nil {
		logger.LogError(fmt.Sprintf("Unsafe Java %d archive rejected: %v", requiredJava, valErr))
		return "", false
	}
	if mkErr := os.MkdirAll(targetDir, 0755); mkErr != nil {
		logger.LogError(fmt.Sprintf("Failed to create Java directory %s: %v", targetDir, mkErr))
		return "", false
	}
	if !system.RunStep(fmt.Sprintf("Deploying Eclipse Temurin OpenJDK %d runtime to %s", requiredJava, targetDir),
		"tar", "-xzf", tarPath, "-C", targetDir, "--strip-components=1") {
		logger.LogError(fmt.Sprintf("Failed to extract Java %d.", requiredJava))
		return "", false
	}
	if entries, rdErr := os.ReadDir(filepath.Join(targetDir, "bin")); rdErr == nil {
		for _, e := range entries {
			if e.Type()&os.ModeSymlink == 0 && !e.IsDir() {
				_ = os.Chmod(filepath.Join(targetDir, "bin", e.Name()), 0755)
			}
		}
	}

	installedBin := filepath.Join(targetDir, "bin", "java")
	if isJavaVersionSufficient(installedBin, requiredJava) {
		_ = os.Remove(fmt.Sprintf("/usr/local/bin/java%d", requiredJava))
		_ = os.Symlink(installedBin, fmt.Sprintf("/usr/local/bin/java%d", requiredJava))
		logger.LogSuccess(fmt.Sprintf("Eclipse Temurin Java %d deployed successfully at %s.", requiredJava, installedBin))
		return installedBin, true
	}

	return "", false
}

func getHostTotalRAMGB() int {
	totalGB := 4
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
						gb := kb / (1024 * 1024)
						if gb > 1024 {
							gb = 1024
						}
						totalGB = int(gb)
					}
				}
				break
			}
		}
	}
	return totalGB
}

func SelectRAM() (string, int) {
	totalGB := getHostTotalRAMGB()

	var options []string
	var ramValues []string

	if totalGB >= 16 {
		options = append(options,
			fmt.Sprintf("4G (Lightweight" + " " + core.Sym.Bullet + " " + "5-10 players) [Host Total: %d GB]", totalGB),
			"8G (Recommended" + " " + core.Sym.Bullet + " " + "15-30 players & plugins)",
			fmt.Sprintf("%dG (Heavy Workload" + " " + core.Sym.Bullet + " " + "50+ players & heavy worldgen)", totalGB-4),
		)
		ramValues = append(ramValues, "4G", "8G", fmt.Sprintf("%dG", totalGB-4))
	} else if totalGB >= 8 {
		options = append(options,
			fmt.Sprintf("2G (Lightweight" + " " + core.Sym.Bullet + " " + "2-5 players) [Host Total: %d GB]", totalGB),
			"4G (Recommended" + " " + core.Sym.Bullet + " " + "5-15 players & plugins)",
			"6G (Heavy Plugins" + " " + core.Sym.Bullet + " " + "up to 25 players)",
		)
		ramValues = append(ramValues, "2G", "4G", "6G")
	} else if totalGB >= 4 {
		options = append(options,
			fmt.Sprintf("2G (Recommended" + " " + core.Sym.Bullet + " " + "Standard Survival) [Host Total: %d GB]", totalGB),
			"3G (Heavy Allocation" + " " + core.Sym.Bullet + " " + "10-15 players)",
		)
		ramValues = append(ramValues, "2G", "3G")
	} else {
		options = append(options,
			fmt.Sprintf("1G (Minimal" + " " + core.Sym.Bullet + " " + "1-2 players) [Host Total: %d GB]", totalGB),
			fmt.Sprintf("%dM (Low Memory Host)", totalGB*750),
		)
		ramValues = append(ramValues, "1G", fmt.Sprintf("%dM", totalGB*750))
	}

	options = append(options, "Custom RAM Allocation (Manual input, e.g. 6G, 12G)")
	options = append(options, "Back / Cancel")

	idx, opt, status := tui.SelectMenu("KROTEK MINECRAFT" + " " + core.Sym.EmDash + " " + "Server Memory (RAM) Allocation", options)
	if status == 2 || strings.Contains(opt, "Back / Cancel") {
		return "", 2
	}

	if idx < len(ramValues) {
		return ramValues[idx], 0
	}

	for {
		custom, st := tui.ReadTextInput("Enter RAM allocation (e.g. 4G, 8G, 3072M)", "4G")
		if st != 0 {
			return "", 2
		}
		custom = strings.ToUpper(strings.TrimSpace(custom))
		if strings.HasSuffix(custom, "G") || strings.HasSuffix(custom, "M") {
			return custom, 0
		}
		fmt.Printf("  %s[!] Please specify unit 'G' or 'M' (e.g. 4G or 4096M)%s\r\n", core.YELLOW, core.NC)
	}
}

func SelectPort() (string, int) {
	for {
		portStr, st := tui.ReadTextInput("Enter Minecraft Server Port", "25565")
		if st != 0 {
			return "", 2
		}
		portStr = strings.TrimSpace(portStr)
		if portStr == "" {
			portStr = "25565"
		}
		p, err := strconv.Atoi(portStr)
		if err != nil || p < 1 || p > 65535 {
			fmt.Printf("  %s[!] Invalid port number. Must be between 1 and 65535.%s\r\n", core.RED, core.NC)
			continue
		}

		resolvedPort, ok := system.ResolvePortConflict(portStr, "tcp", true)
		if !ok {
			return "", 2
		}
		return resolvedPort, 0
	}
}

// FreshInstallWizard handles the end-to-end installation pipeline for Minecraft PaperMC Dedicated Game Server.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(mode string) {
	StartGuidedInstall(mode)
}

// PromptExistingConfigAction checks if prior configuration files exist on the host system.
// In interactive TUI mode, it prompts the administrator in English to decide whether to preserve existing custom settings
// or perform a clean factory default reset. In unattended mode (-y / CI/CD), it automatically preserves settings (safe default).
func PromptExistingConfigAction(serviceTitle string, configPaths ...string) {
	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
		return
	}

	hasExisting := false
	for _, p := range configPaths {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				hasExisting = true
				break
			}
		}
	}
	if !hasExisting {
		return
	}

	tui.ClearScreen()
	cardLines := []string{
		core.BOLD + core.YELLOW + "EXISTING CONFIGURATION DETECTED" + core.NC,
		"",
		fmt.Sprintf("  Prior configuration files were detected for %s on this host.", serviceTitle),
		"  Choose how the deployment wizard should handle existing settings:",
		"",
		"  " + core.GREEN + "[1] Keep Existing Configuration (Recommended)" + core.NC,
		"      Preserves all existing configuration, virtual hosts, and custom data intact.",
		"",
		"  " + core.YELLOW + "[2] Clean Reset to Factory Default (Overwrite)" + core.NC,
		"      Overwrites configuration with fresh, production-hardened defaults.",
	}

	choices := []string{
		"Keep Existing Configuration (Preserve custom settings & virtual hosts)",
		"Clean Reset to Factory Default (Overwrite with fresh configurations)",
	}

	idx, _, status := tui.SelectMenuWithContent("Configuration Conflict Handler", cardLines, choices)
	if status == 2 {
		tui.LeaveAltScreen()
		logger.LogInfo("Setup cancelled by user at configuration conflict prompt.")
		os.Exit(0)
	}
	if status == 0 && idx == 1 {
		core.OverwriteExistingConfig = true
		logger.LogInfo("User selected: Clean reset to factory default configuration.")
	} else {
		core.OverwriteExistingConfig = false
		logger.LogInfo("User selected: Preserve existing configuration.")
	}
}
