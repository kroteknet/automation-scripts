// ==============================================================================
// KROTEK SCRIPT HUB — MULTI-DISTRO PACKAGE MANAGEMENT ABSTRACTION
// Service: OpenResty (web-server)
// File:    web-server/openresty/src/system/packages.go
//
// Abstracts native package managers (apt-get, dnf/yum, apk) across Linux distributions,
// providing consistent APIs for installing, removing, updating, querying software packages,
// and provisioning verified upstream OpenResty repositories.
// ==============================================================================

package system

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"krotek-openresty-installer/core"
	"krotek-openresty-installer/logger"
	"krotek-openresty-installer/tui"
)

// GetDpkgConfigFlags returns Debian dpkg configuration flags based on whether user chose to preserve or overwrite existing configs.
func GetDpkgConfigFlags() []string {
	if core.OverwriteExistingConfig {
		return []string{"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confnew"}
	}
	return []string{"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}
}

// InstallPackages provisions one or more packages using the native package manager
// appropriate for the host distribution (apt-get on Debian/Ubuntu, dnf/yum on RHEL, apk on Alpine).
func InstallPackages(packages ...string) bool {
	if len(packages) == 0 {
		return true
	}
	switch core.SysInfo.Family {
	case "debian":
		// Dpkg options preserve existing configuration or apply clean defaults without blocking TUI or automation
		args := append([]string{"install", "-y", "-q"}, GetDpkgConfigFlags()...)
		args = append(args, packages...)
		return RunStep("Installing system packages", "apt-get", args...)
	case "rhel":
		args := append([]string{"install", "-y", "-q"}, packages...)
		return RunRHELStep("Installing system packages", true, args...)
	case "alpine":
		args := append([]string{"add", "--quiet"}, packages...)
		return RunStep("Installing system packages", "apk", args...)
	default:
		args := append([]string{"install", "-y"}, packages...)
		return RunStep("Installing system packages", "apt-get", args...)
	}
}

// RemovePackages uninstalls one or more packages using the host distribution's package manager,
// purging configuration files where appropriate (e.g. apt-get purge).
func RemovePackages(packages ...string) bool {
	if len(packages) == 0 {
		return true
	}
	switch core.SysInfo.Family {
	case "debian":
		args := append([]string{"purge", "-y", "-q"}, packages...)
		return RunStep("Removing system packages", "apt-get", args...)
	case "rhel":
		args := append([]string{"remove", "-y", "-q"}, packages...)
		return RunRHELStep("Removing system packages", true, args...)
	case "alpine":
		args := append([]string{"del", "--quiet"}, packages...)
		return RunStep("Removing system packages", "apk", args...)
	default:
		args := append([]string{"remove", "-y"}, packages...)
		return RunStep("Removing system packages", "apt-get", args...)
	}
}

var rhelRepoMetaFailRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)Failed to download metadata for repo '([^']+)'`),
	regexp.MustCompile(`(?i)Failed to synchronize cache for repo '([^']+)'`),
	regexp.MustCompile(`(?i)Cannot find a valid baseurl for repo:\s*([^\s/]+)`),
}

var safeRHELRepoIDRegex = regexp.MustCompile(`^[a-zA-Z0-9._:@-]+$`)

func extractFailingRHELRepo(outStr string) string {
	for _, re := range rhelRepoMetaFailRegexes {
		if m := re.FindStringSubmatch(outStr); len(m) == 2 {
			repoID := strings.TrimSpace(m[1])
			if repoID != "" && !strings.HasPrefix(repoID, "-") && safeRHELRepoIDRegex.MatchString(repoID) {
				return repoID
			}
		}
	}
	return ""
}

func rhelPkgManager() string {
	if CommandExists("dnf") {
		return "dnf"
	}
	return "yum"
}

func isCriticalRHELRepo(repoID string) bool {
	lower := strings.ToLower(strings.TrimSpace(repoID))
	return lower == "" || lower == "openresty" || lower == "baseos" || lower == "appstream"
}

// ExecRHELCmd executes dnf/yum cleanly, automatically skipping unreachable third-party
// repositories on retry (while never disabling openresty or base OS repos).
func ExecRHELCmd(msg string, showSpinner bool, printError bool, args ...string) (string, int, bool) {
	mgr := rhelPkgManager()
	isCheckUpdate := len(args) > 0 && args[0] == "check-update"

	if showSpinner && msg != "" {
		tui.StartSpinner(msg)
	}

	var disabled []string
	seenDisabled := make(map[string]bool)

	for attempt := 0; attempt < 6; attempt++ {
		var runArgs []string
		if len(disabled) > 0 {
			runArgs = append(runArgs, "--disablerepo="+strings.Join(disabled, ","))
		}
		runArgs = append(runArgs, args...)

		cmd := exec.Command(mgr, runArgs...)
		var buf bytes.Buffer
		if core.LogFile != nil && !isCheckUpdate {
			mw := io.MultiWriter(&buf, core.LogFile)
			cmd.Stdout = mw
			cmd.Stderr = mw
		} else {
			cmd.Stdout = &buf
			cmd.Stderr = &buf
		}

		err := cmd.Run()
		if err == nil {
			if showSpinner && msg != "" {
				tui.StopSpinner(0)
			}
			return buf.String(), 0, true
		}

		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}

		// dnf/yum check-update exits with 100 when updates are available (not an error)
		if isCheckUpdate && exitCode == 100 {
			if showSpinner && msg != "" {
				tui.StopSpinner(0)
			}
			return buf.String(), 100, true
		}

		outStr := buf.String()
		if badRepo := extractFailingRHELRepo(outStr); badRepo != "" {
			if !isCriticalRHELRepo(badRepo) && !seenDisabled[badRepo] {
				seenDisabled[badRepo] = true
				disabled = append(disabled, badRepo)
				if showSpinner && msg != "" {
					tui.StopSpinnerQuiet()
				}
				warnMsg := fmt.Sprintf("Skipping unreachable third-party RPM repo '%s' and retrying...", badRepo)
				if !tui.IsBoxSpinnerActive() {
					logger.LogWarn(warnMsg)
				} else if core.LogFile != nil {
					_, _ = core.LogFile.WriteString(fmt.Sprintf("[%s] [WARN] %s\n", time.Now().Format(time.RFC3339), warnMsg))
				}
				if showSpinner && msg != "" {
					tui.StartSpinner(msg)
				}
				continue
			}
		}

		if showSpinner && msg != "" {
			if printError {
				tui.StopSpinner(1)
			} else {
				tui.StopSpinnerQuiet()
			}
		}
		if printError && !isCheckUpdate {
			logger.LogError(fmt.Sprintf("Failed! Check log file: cat %s", core.LogFilePath))
		}
		return outStr, exitCode, false
	}

	if showSpinner && msg != "" {
		if printError {
			tui.StopSpinner(1)
		} else {
			tui.StopSpinnerQuiet()
		}
	}
	if printError && !isCheckUpdate {
		logger.LogError(fmt.Sprintf("Failed! Check log file: cat %s", core.LogFilePath))
	}
	return "", 1, false
}

// RunRHELStep executes a dnf/yum operation with optional spinner and automatic broken-repo recovery.
func RunRHELStep(msg string, showSpinner bool, args ...string) bool {
	_, _, ok := ExecRHELCmd(msg, showSpinner, true, args...)
	return ok
}

// RunRHELStepOrAbort executes a dnf/yum step and aborts the installation cleanly on failure.
func RunRHELStepOrAbort(msg string, args ...string) bool {
	if !RunRHELStep(msg, true, args...) {
		HandleInstallFailure(msg)
		return false
	}
	return true
}

// runLoggedAptCmd executes apt-get with optional spinner, mirrors output to the active logfile,
// and returns the combined stdout/stderr text alongside success status.
func runLoggedAptCmd(msg string, showSpinner bool, deferErrorUI bool, extraEnv []string, args ...string) (string, bool) {
	if showSpinner {
		tui.StartSpinner(msg)
	}
	cmd := exec.Command("apt-get", args...)
	cmd.Env = append(append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a", "UCF_FORCE_CONFFOLD=1"), extraEnv...)

	var buf bytes.Buffer
	if core.LogFile != nil {
		mw := io.MultiWriter(&buf, core.LogFile)
		cmd.Stdout = mw
		cmd.Stderr = mw
	} else {
		cmd.Stdout = &buf
		cmd.Stderr = &buf
	}

	err := cmd.Run()
	if err == nil {
		if showSpinner {
			tui.StopSpinner(0)
		}
		return buf.String(), true
	}

	if showSpinner {
		if deferErrorUI {
			tui.StopSpinnerQuiet()
		} else {
			tui.StopSpinner(1)
		}
	}
	return buf.String(), false
}

const openrestyLegacyFingerprint = "E52218E7087897DC6DEA6D6D97DB7443D5EDEB74"

func isOpenRestySequoiaSHA1Error(aptOut string) bool {
	lower := strings.ToLower(aptOut)
	hasSequoiaSHA1 := strings.Contains(aptOut, "SHA1 is not considered secure") ||
		strings.Contains(aptOut, "/usr/bin/sqv") ||
		strings.Contains(lower, "policy rejected non-revocation signature")
	if !hasSequoiaSHA1 {
		return false
	}

	hasOpenResty := strings.Contains(lower, "openresty") || strings.Contains(strings.ToUpper(aptOut), openrestyLegacyFingerprint)
	if !hasOpenResty {
		return false
	}

	// Ensure unrelated repositories did not fail independently of OpenResty.
	for _, line := range strings.Split(aptOut, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Err:") && !strings.Contains(strings.ToLower(trimmed), "openresty.org") {
			return false
		}
	}
	return true
}

func writeScopedOpenRestySequoiaPolicy() (string, error) {
	const fallbackConfig = `[asymmetric_algorithms]
dsa2048 = 2024-02-01
dsa3072 = 2024-02-01
dsa4096 = 2024-02-01
brainpoolp256 = 2028-02-01
brainpoolp384 = 2028-02-01
brainpoolp512 = 2028-02-01
rsa2048  = 2030-02-01

[hash_algorithms]
sha1.second_preimage_resistance = 2028-02-01
sha224 = 2026-02-01

[packets]
signature.v3 = 2026-02-01
`
	policyContent := fallbackConfig
	if raw, err := os.ReadFile("/usr/share/apt/default-sequoia.config"); err == nil && len(raw) > 0 {
		s := string(raw)
		re := regexp.MustCompile(`(?m)^sha1\.second_preimage_resistance\s*=.*$`)
		if re.MatchString(s) {
			policyContent = re.ReplaceAllString(s, "sha1.second_preimage_resistance = 2028-02-01")
		} else if strings.Contains(s, "[hash_algorithms]") {
			policyContent = strings.Replace(s, "[hash_algorithms]", "[hash_algorithms]\nsha1.second_preimage_resistance = 2028-02-01", 1)
		} else {
			policyContent = s + "\n[hash_algorithms]\nsha1.second_preimage_resistance = 2028-02-01\n"
		}
	}

	tmpFile, err := os.CreateTemp("", "krotek-openresty-sequoia-*.config")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.WriteString(policyContent); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	_ = tmpFile.Close()
	// _apt executes sqv under an unprivileged UID, so this scoped temp policy must be readable by _apt.
	if err := os.Chmod(tmpPath, 0644); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func updateDebianAptWithOpenRestyFallback(showSpinner bool) bool {
	outStr, ok := runLoggedAptCmd("Updating package repository index", showSpinner, true, nil, "update", "-q")
	if ok {
		return true
	}

	const openrestyList = "/etc/apt/sources.list.d/openresty.list"
	if fileExistsLocal(openrestyList) && isOpenRestySequoiaSHA1Error(outStr) {
		warnMsg := "APT Sequoia policy rejected legacy SHA-1 self-signature on OpenResty signing key; verifying OpenResty repo exclusively with scoped temporary policy."
		if showSpinner {
			logger.LogWarn(warnMsg)
		} else if core.LogFile != nil {
			_, _ = core.LogFile.WriteString(fmt.Sprintf("[%s] [WARN] %s\n", time.Now().Format(time.RFC3339), warnMsg))
		}
		policyPath, err := writeScopedOpenRestySequoiaPolicy()
		if err != nil {
			errMsg := fmt.Sprintf("Failed to create scoped APT Sequoia policy: %v (Check log: cat %s)", err, core.LogFilePath)
			if showSpinner {
				logger.LogError(errMsg)
			} else if core.LogFile != nil {
				_, _ = core.LogFile.WriteString(fmt.Sprintf("[%s] [ERROR] %s\n", time.Now().Format(time.RFC3339), errMsg))
			}
			return false
		}
		defer os.Remove(policyPath)

		_, retryOK := runLoggedAptCmd(
			"Verifying OpenResty repository metadata",
			showSpinner,
			false,
			[]string{"APT_SEQUOIA_CRYPTO_POLICY=" + policyPath},
			"update", "-q",
			"-o", "Dir::Etc::sourcelist="+openrestyList,
			"-o", "Dir::Etc::sourceparts=-",
			"-o", "APT::Get::List-Cleanup=0",
		)
		if retryOK {
			return true
		}
	} else if showSpinner {
		fmt.Printf("\r\033[K%s[ERROR]%s Updating package repository index\r\n", core.RED, core.NC)
	}

	if showSpinner {
		logger.LogError(fmt.Sprintf("Failed! Check log file: cat %s", core.LogFilePath))
	} else if core.LogFile != nil {
		_, _ = core.LogFile.WriteString(fmt.Sprintf("[%s] [ERROR] Updating package repository index failed.\n", time.Now().Format(time.RFC3339)))
	}
	return false
}

// UpdatePackageIndex refreshes local package repository metadata cache (apt-get update, dnf makecache, apk update).
func UpdatePackageIndex() bool {
	showSpinner := !tui.IsBoxSpinnerActive()
	switch core.SysInfo.Family {
	case "debian":
		return updateDebianAptWithOpenRestyFallback(showSpinner)
	case "rhel":
		return RunRHELStep("Updating package repository index", showSpinner, "makecache", "-q")
	case "alpine":
		if showSpinner {
			return RunStep("Updating package repository index", "apk", "update", "--quiet")
		}
		cmd := exec.Command("apk", "update", "--quiet")
		if core.LogFile != nil {
			cmd.Stdout = core.LogFile
			cmd.Stderr = core.LogFile
		} else {
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
		}
		return cmd.Run() == nil
	default:
		return updateDebianAptWithOpenRestyFallback(showSpinner)
	}
}

func httpFetchBytes(url string) ([]byte, int, error) {
	return HttpFetchBytes(url, 4*1024*1024)
}

func parseReleaseComponent(releaseBody []byte, defaultComp string) string {
	for _, line := range strings.Split(string(releaseBody), "\n") {
		if strings.HasPrefix(line, "Components:") {
			fields := strings.Fields(strings.TrimPrefix(line, "Components:"))
			for _, f := range fields {
				if f == "openresty" || f == "main" {
					return f
				}
			}
			if len(fields) > 0 {
				return fields[0]
			}
		}
	}
	return defaultComp
}

// SetupOpenRestyRepository configures official OpenResty package repositories and cryptographic keys
// on Debian/Ubuntu and RHEL/AlmaLinux/Fedora, or enables Alpine's official community repository.
func SetupOpenRestyRepository() bool {
	switch core.SysInfo.Family {
	case "debian":
		_ = os.Remove("/etc/apt/sources.list.d/openresty.list")
		_ = UpdatePackageIndex()
		if !InstallPackages("ca-certificates", "gnupg", "curl") {
			logger.LogWarn("Failed installing prerequisite packages (ca-certificates, gnupg, curl); attempting repository setup anyway.")
		}

		distro := strings.ToLower(core.SysInfo.Distro)
		switch distro {
		case "debian", "raspbian", "kali", "devuan":
			distro = "debian"
		case "ubuntu", "linuxmint", "pop", "elementary", "zorin", "neon":
			distro = "ubuntu"
		default:
			if strings.Contains(strings.ToLower(core.SysInfo.Version), "ubuntu") {
				distro = "ubuntu"
			} else {
				distro = "debian"
			}
		}

		repoBase := "https://openresty.org/package/" + distro
		if core.SysInfo.Architecture == "arm64" {
			repoBase = "https://openresty.org/package/arm64/" + distro
		}

		var candidates []string
		if c := strings.ToLower(strings.TrimSpace(core.SysInfo.Codename)); c != "" {
			candidates = append(candidates, c)
		}
		if distro == "ubuntu" {
			candidates = append(candidates, "noble", "jammy", "focal", "bionic")
		} else {
			candidates = append(candidates, "trixie", "bookworm", "bullseye", "buster")
		}

		seen := make(map[string]bool)
		chosenCodename := ""
		chosenComponent := "openresty"
		if distro == "ubuntu" {
			chosenComponent = "main"
		}

		for _, cand := range candidates {
			if cand == "" || seen[cand] {
				continue
			}
			seen[cand] = true
			releaseURL := fmt.Sprintf("%s/dists/%s/Release", repoBase, cand)
			body, status, err := httpFetchBytes(releaseURL)
			if err == nil && status == 200 {
				chosenCodename = cand
				chosenComponent = parseReleaseComponent(body, chosenComponent)
				break
			}
		}

		if chosenCodename == "" {
			logger.LogError(fmt.Sprintf("Could not find a supported OpenResty APT suite under %s", repoBase))
			return false
		}
		if core.SysInfo.Codename != "" && chosenCodename != core.SysInfo.Codename {
			logger.LogInfo(fmt.Sprintf("Using compatible OpenResty APT suite '%s' for %s (%s)", chosenCodename, distro, core.SysInfo.Codename))
		}

		keyURL := "https://openresty.org/package/pubkey.gpg"
		keyBytes, keyStatus, keyErr := httpFetchBytes(keyURL)
		if keyErr != nil || keyStatus != 200 || len(keyBytes) == 0 {
			if out, err := exec.Command("curl", "--proto", "=https", "--tlsv1.2", "-fsSL", keyURL).Output(); err == nil && len(out) > 0 {
				keyBytes = out
			} else {
				logger.LogError("Failed to download OpenResty GPG public key from " + keyURL)
				return false
			}
		}

		// Optional secondary OpenResty key (supports future upstream key rotation transparently).
		key2URL := "https://openresty.org/package/pubkey2.gpg"
		key2Bytes, key2Status, key2Err := httpFetchBytes(key2URL)
		if key2Err != nil || key2Status != 200 || len(key2Bytes) == 0 {
			if out, err := exec.Command("curl", "--proto", "=https", "--tlsv1.2", "-fsSL", key2URL).Output(); err == nil && len(out) > 0 {
				key2Bytes = out
			}
		}

		tmpDir, err := os.MkdirTemp("", "krotek-openresty-key-*")
		if err != nil {
			logger.LogError("Failed to create temp directory for GPG key: " + err.Error())
			return false
		}
		defer os.RemoveAll(tmpDir)
		_ = os.Chmod(tmpDir, 0700)

		rawKeyPath := filepath.Join(tmpDir, "pubkey.gpg")
		if err := WriteFileNoFollow(rawKeyPath, keyBytes, 0600, ""); err != nil {
			logger.LogError("Failed to write temp GPG key file: " + err.Error())
			return false
		}

		dearmored1 := filepath.Join(tmpDir, "pubkey1.gpg")
		if out, err := exec.Command("gpg", "--batch", "--yes", "--dearmor", "-o", dearmored1, rawKeyPath).CombinedOutput(); err != nil {
			logger.LogError(fmt.Sprintf("Failed to import OpenResty GPG key: %s (%v)", strings.TrimSpace(string(out)), err))
			return false
		}
		combinedKeyring, err := os.ReadFile(dearmored1)
		if err != nil || len(combinedKeyring) == 0 {
			logger.LogError("Failed to read dearmored OpenResty GPG key.")
			return false
		}

		if len(key2Bytes) > 0 {
			rawKey2Path := filepath.Join(tmpDir, "pubkey2.gpg")
			dearmored2 := filepath.Join(tmpDir, "pubkey2.bin.gpg")
			if WriteFileNoFollow(rawKey2Path, key2Bytes, 0600, "") == nil {
				if exec.Command("gpg", "--batch", "--yes", "--dearmor", "-o", dearmored2, rawKey2Path).Run() == nil {
					if b2, err := os.ReadFile(dearmored2); err == nil && len(b2) > 0 {
						combinedKeyring = append(combinedKeyring, b2...)
					}
				}
			}
		}

		_ = os.MkdirAll("/usr/share/keyrings", 0755)
		gpgPath := "/usr/share/keyrings/openresty.gpg"
		_ = os.Remove("/etc/apt/trusted.gpg.d/openresty.gpg")
		if err := WriteFileNoFollow(gpgPath, combinedKeyring, 0644, ""); err != nil {
			logger.LogError("Failed to write OpenResty keyring " + gpgPath + ": " + err.Error())
			return false
		}

		sourcesLine := fmt.Sprintf("deb [signed-by=%s] %s %s %s\n", gpgPath, repoBase, chosenCodename, chosenComponent)
		if err := WriteFileNoFollow("/etc/apt/sources.list.d/openresty.list", []byte(sourcesLine), 0644, ""); err != nil {
			logger.LogError("Failed to write /etc/apt/sources.list.d/openresty.list: " + err.Error())
			return false
		}
		return UpdatePackageIndex()

	case "rhel":
		if !CommandExists("curl") || !fileExistsLocal("/etc/pki/tls/certs/ca-bundle.crt") {
			_ = InstallPackages("ca-certificates", "curl")
		}
		repoURL := "https://openresty.org/package/centos/openresty.repo"
		if strings.EqualFold(core.SysInfo.Distro, "fedora") {
			repoURL = "https://openresty.org/package/fedora/openresty.repo"
		}
		repoBytes, status, err := httpFetchBytes(repoURL)
		if err != nil || status != 200 || len(repoBytes) == 0 {
			if out, cErr := exec.Command("curl", "--proto", "=https", "--tlsv1.2", "-fsSL", repoURL).Output(); cErr == nil && len(out) > 0 {
				repoBytes = out
			} else {
				logger.LogError("Failed to download OpenResty RPM repository definition from " + repoURL)
				return false
			}
		}

		// RHEL 9+ / Fedora enforce SHA-256 RPM signatures and reject OpenResty's legacy SHA-1 key (pubkey.gpg).
		// Probe and import OpenResty's rotated SHA-256 signing key (pubkey2.gpg) while keeping gpgcheck=1 enforced.
		fetchGPGKey := func(u string) []byte {
			if b, st, fErr := httpFetchBytes(u); fErr == nil && st == 200 && len(b) > 0 {
				return b
			}
			if out, cErr := exec.Command("curl", "--proto", "=https", "--tlsv1.2", "-fsSL", u).Output(); cErr == nil && len(out) > 0 {
				return out
			}
			return nil
		}

		const key1URL = "https://openresty.org/package/pubkey.gpg"
		const key2URL = "https://openresty.org/package/pubkey2.gpg"
		hasKey1, hasKey2 := false, false
		if tmpKeyDir, kErr := os.MkdirTemp("", "krotek-openresty-rpmkey-*"); kErr == nil {
			_ = os.Chmod(tmpKeyDir, 0700)
			if k2 := fetchGPGKey(key2URL); len(k2) > 0 {
				k2Path := filepath.Join(tmpKeyDir, "pubkey2.gpg")
				if WriteFileNoFollow(k2Path, k2, 0600, "") == nil && exec.Command("rpm", "--import", k2Path).Run() == nil {
					hasKey2 = true
				}
			}
			if k1 := fetchGPGKey(key1URL); len(k1) > 0 {
				k1Path := filepath.Join(tmpKeyDir, "pubkey.gpg")
				if WriteFileNoFollow(k1Path, k1, 0600, "") == nil && exec.Command("rpm", "--import", k1Path).Run() == nil {
					hasKey1 = true
				}
			}
			_ = os.RemoveAll(tmpKeyDir)
		}

		repoContent := string(repoBytes)
		gpgKeyRe := regexp.MustCompile(`(?m)^gpgkey=.*$`)
		if hasKey2 && !hasKey1 {
			logger.LogInfo("Configuring OpenResty SHA-256 RPM signing key (pubkey2.gpg) for strict RPM crypto policy compatibility.")
			repoContent = gpgKeyRe.ReplaceAllString(repoContent, "gpgkey="+key2URL)
		} else if hasKey2 && hasKey1 {
			repoContent = gpgKeyRe.ReplaceAllString(repoContent, "gpgkey="+key1URL+"\n       "+key2URL)
		}

		_ = os.MkdirAll("/etc/yum.repos.d", 0755)
		if err := WriteFileNoFollow("/etc/yum.repos.d/openresty.repo", []byte(repoContent), 0644, ""); err != nil {
			logger.LogError("Failed to write /etc/yum.repos.d/openresty.repo: " + err.Error())
			return false
		}
		return UpdatePackageIndex()

	case "alpine":
		repoFile := "/etc/apk/repositories"
		data, err := os.ReadFile(repoFile)
		if err != nil {
			logger.LogError("Failed to read /etc/apk/repositories: " + err.Error())
			return false
		}
		var cleaned []string
		hasActiveCommunity := false
		var activeMainMirror string

		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			// Remove obsolete dead third-party openresty.org/package/alpine entries
			if strings.Contains(trimmed, "openresty.org/package/alpine") {
				continue
			}
			// Uncomment commented community repo lines matching active Alpine repos
			if strings.HasPrefix(trimmed, "#") {
				uncommented := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
				if (strings.HasPrefix(uncommented, "http://") || strings.HasPrefix(uncommented, "https://")) && strings.HasSuffix(uncommented, "/community") {
					cleaned = append(cleaned, uncommented)
					hasActiveCommunity = true
					continue
				}
			}
			cleaned = append(cleaned, trimmed)
			if !strings.HasPrefix(trimmed, "#") && strings.HasSuffix(trimmed, "/main") && activeMainMirror == "" {
				activeMainMirror = trimmed
			}
			if !strings.HasPrefix(trimmed, "#") && strings.HasSuffix(trimmed, "/community") {
				hasActiveCommunity = true
			}
		}

		if !hasActiveCommunity {
			if activeMainMirror != "" {
				cleaned = append(cleaned, strings.TrimSuffix(activeMainMirror, "/main")+"/community")
			} else {
				cleaned = append(cleaned, "https://dl-cdn.alpinelinux.org/alpine/latest-stable/community")
			}
		}

		if err := WriteFileNoFollow(repoFile, []byte(strings.Join(cleaned, "\n")+"\n"), 0644, ""); err != nil {
			logger.LogError("Failed to update /etc/apk/repositories: " + err.Error())
			return false
		}
		return UpdatePackageIndex()
	}
	return true
}

// EnsureDockerEngine guarantees that Docker Engine and Docker Compose are installed and running.
func EnsureDockerEngine() bool {
	// 1. If docker binary is already installed, ensure service is running and compose works
	if CommandExists("docker") {
		if exec.Command("docker", "info").Run() != nil {
			_ = RunMultiStep("Starting Docker Service", "systemctl enable --now docker 2>/dev/null || (rc-service cgroups start 2>/dev/null; rc-update add docker default 2>/dev/null && rc-service docker start 2>/dev/null) || service docker start 2>/dev/null || true")
			time.Sleep(2 * time.Second)
		}
		if exec.Command("docker", "compose", "version").Run() == nil || CommandExists("docker-compose") {
			return true
		}
		if core.SysInfo.Family == "alpine" {
			_ = RunMultiStep("Installing Docker Compose Plugin", "apk add --no-cache docker-cli-compose 2>/dev/null || apk add --no-cache docker-compose 2>/dev/null || true")
			return true
		}
	}

	// 2. Install Docker Engine based on OS family
	switch core.SysInfo.Family {
	case "alpine":
		cmd := "apk update -q && (apk add --no-cache docker docker-cli-compose curl iptables ip6tables iproute2 2>/dev/null || apk add --no-cache docker docker-compose curl iptables ip6tables iproute2); modprobe ip_tables 2>/dev/null || true; modprobe iptable_nat 2>/dev/null || true; modprobe br_netfilter 2>/dev/null || true; sysctl -w net.ipv4.ip_forward=1 2>/dev/null || true; rc-service cgroups start 2>/dev/null || true; rc-update add docker default 2>/dev/null || true; rc-service docker restart 2>/dev/null || rc-service docker start 2>/dev/null || true"
		if !RunMultiStepOrAbort("Installing Docker Engine", cmd) {
			return false
		}
	case "rhel":
		if !RunRHELStep("Installing Docker Engine", true, "install", "-y", "docker-ce", "docker-ce-cli", "containerd.io", "docker-compose-plugin") {
			if !RunRHELStep("Installing Docker & Compose", true, "install", "-y", "docker", "docker-compose") {
				cmd := "TMP_D=$(mktemp -d /tmp/krotek-docker-XXXXXX) && chmod 700 \"$TMP_D\" && curl --proto '=https' --tlsv1.2 -fsSL https://get.docker.com -o \"$TMP_D/get-docker.sh\" && [ -s \"$TMP_D/get-docker.sh\" ] && sh \"$TMP_D/get-docker.sh\" && rc=0 || rc=$?; rm -rf \"$TMP_D\"; exit $rc"
				if !RunMultiStepOrAbort("Installing Docker Engine", cmd) {
					return false
				}
			}
		}
		_ = exec.Command("systemctl", "enable", "--now", "docker").Run()
	case "debian":
		cmd := "(apt-get update -qq && (apt-get install -y docker.io docker-compose-v2 2>/dev/null || apt-get install -y docker.io docker-compose-plugin 2>/dev/null || apt-get install -y docker.io docker-compose)) || (TMP_D=$(mktemp -d /tmp/krotek-docker-XXXXXX) && chmod 700 \"$TMP_D\" && curl --proto '=https' --tlsv1.2 -fsSL https://get.docker.com -o \"$TMP_D/get-docker.sh\" && [ -s \"$TMP_D/get-docker.sh\" ] && sh \"$TMP_D/get-docker.sh\"; rm -rf \"$TMP_D\"); systemctl enable --now docker 2>/dev/null || true"
		if !RunMultiStepOrAbort("Installing Docker Engine", cmd) {
			return false
		}
	default:
		cmd := "(TMP_D=$(mktemp -d /tmp/krotek-docker-XXXXXX) && chmod 700 \"$TMP_D\" && curl --proto '=https' --tlsv1.2 -fsSL https://get.docker.com -o \"$TMP_D/get-docker.sh\" && [ -s \"$TMP_D/get-docker.sh\" ] && sh \"$TMP_D/get-docker.sh\"; rm -rf \"$TMP_D\") || true; systemctl enable --now docker 2>/dev/null || rc-service docker start 2>/dev/null || true"
		if !RunMultiStepOrAbort("Installing Docker Engine", cmd) {
			return false
		}
	}

	// 3. Post-install daemon startup & enablement
	_ = exec.Command("systemctl", "enable", "docker").Run()
	_ = exec.Command("systemctl", "start", "docker").Run()
	_ = exec.Command("rc-service", "cgroups", "start").Run()
	_ = exec.Command("rc-update", "add", "docker", "default").Run()
	_ = exec.Command("rc-service", "docker", "start").Run()
	_ = exec.Command("service", "docker", "start").Run()

	for i := 0; i < 5; i++ {
		if exec.Command("docker", "info").Run() == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	return CommandExists("docker")
}
