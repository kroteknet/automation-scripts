// ==============================================================================
// KROTEK SCRIPT HUB — AUTHENTICATION & CREDENTIAL SECURITY MANAGER
// Service: MYSQL (database)
// File:    database/mysql/src/modules/auth.go
//
// Manages administrative user accounts, masked password input dialogs, database user privileges, and secure token generation.
// ==============================================================================

package modules

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"io"
	"math/big"
	"time"

	"krotek-mysql-installer/core"
	"krotek-mysql-installer/system"
	"krotek-mysql-installer/tui"
)

// ReadPasswordInput prompts for masked password input without echoing characters to terminal history.
func ReadPasswordInput(promptLabel string) (string, int) {
	if core.AutoYes {
		pass, err := GenerateRandomPass(16)
		if err != nil {
			return "", 1
		}
		return pass, 0
	}
	return tui.ReadPasswordInput(promptLabel)
}

// VerifyAdminPassword compares candidate against configured in constant time, failing closed on empty secrets.
func VerifyAdminPassword(configured, candidate string) bool {
	if configured == "" || candidate == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(configured)) == 1
}

// AuthenticateAdminAccess verifies administrative privileges before performing destructive or elevated operations.
func AuthenticateAdminAccess() bool {
	if core.AutoYes {
		return true
	}
	state := system.LoadState()
	pass := state["ADMIN_PASS"]
	if pass == "" {
		tui.ClearScreen()
		tui.PrintInBox("Access Denied", []string{
			core.BOLD + core.RED + "[ERROR] Administrative password is not configured; authentication refused." + core.NC,
		})
		time.Sleep(1500 * time.Millisecond)
		return false
	}

	for attempts := 0; attempts < 3; attempts++ {
		tui.ClearScreen()
		lines := []string{
			core.YELLOW + "This operation requires root / administrative authentication." + core.NC,
			"Please enter the administrative password configured during setup.",
		}
		if attempts > 0 {
			lines = append(lines, "", fmt.Sprintf("%s[ERROR] Incorrect password! Attempt %d of 3.%s", core.BOLD+core.RED, attempts+1, core.NC))
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel", "Admin Authentication Required", lines)
		input, status := ReadPasswordInput("Enter Admin Password")
		if status != 0 {
			return false
		}
		if VerifyAdminPassword(pass, input) {
			return true
		}
	}

	tui.ClearScreen()
	tui.PrintInBox("Access Denied", []string{
		core.BOLD + core.RED + "[DENIED] Authentication failed 3 times." + core.NC,
	})
	time.Sleep(1500 * time.Millisecond)
	return false
}

// GenerateRandomPass generates a cryptographically random password of the given length without deterministic fallbacks.
func GenerateRandomPass(length int) (string, error) {
	return GenerateRandomPassFromReader(rand.Reader, length)
}

// GenerateRandomPassFromReader reads cryptographic randomness from r and returns an error if CSPRNG fails.
func GenerateRandomPassFromReader(r io.Reader, length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("invalid password length: %d", length)
	}
	if r == nil {
		return "", fmt.Errorf("entropy reader is nil")
	}
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	res := make([]byte, length)
	max := big.NewInt(int64(len(charset)))
	for i := 0; i < length; i++ {
		num, err := rand.Int(r, max)
		if err != nil || num == nil {
			return "", fmt.Errorf("crypto/rand failed: %w", err)
		}
		res[i] = charset[num.Int64()]
	}
	return string(res), nil
}
