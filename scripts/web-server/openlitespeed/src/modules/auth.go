// ==============================================================================
// KROTEK SCRIPT HUB — AUTHENTICATION & CREDENTIAL SECURITY MANAGER
// Service: OPENLITESPEED (web-server)
// File:    web-server/openlitespeed/src/modules/auth.go
//
// Manages administrative user accounts, masked password input dialogs, database user privileges, and secure token generation.
// ==============================================================================

package modules

import (
	"crypto/subtle"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"krotek-openlitespeed-installer/core"
	"krotek-openlitespeed-installer/system"
	"krotek-openlitespeed-installer/tui"
)

// ReadPasswordInput prompts for masked password input without echoing characters to terminal history.
func ReadPasswordInput(promptLabel string) (string, int) {
	if core.AutoYes {
		if envPass := os.Getenv("KROTEK_ADMIN_PASS"); envPass != "" {
			return envPass, 0
		}
		state := system.LoadState()
		if state["ADMIN_PASS"] != "" {
			return state["ADMIN_PASS"], 0
		}
		if state["ROOT_PASS"] != "" {
			return state["ROOT_PASS"], 0
		}
		return "", 1
	}
	fmt.Printf("  %s%s%s %s%s%s: ", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, promptLabel, core.NC)

	tui.EnableRawMode()
	tui.ShowCursor()
	defer func() {
		tui.DisableRawMode()
		tui.ShowCursor()
	}()

	var input []rune
	for {
		k := tui.ReadKeyRaw()
		if k == "esc" {
			fmt.Print("\r\n")
			return "", 1
		}
		if k == "backspace" {
			if len(input) == 0 {
				fmt.Print("\r\n")
				return "", 1
			}
			input = input[:len(input)-1]
			fmt.Print("\b \b")
			continue
		}
		if k == "enter" {
			fmt.Print("\r\n")
			return string(input), 0
		}
		if len(k) == 1 && k[0] >= 32 {
			input = append(input, []rune(k)...)
			fmt.Print(core.Sym.Bullet)
		}
	}
}

// ==============================================================================
// SECURITY AUTHENTICATION
// ==============================================================================

// authenticateAdminAccess prompts for the system root/sudo password and verifies
// it via sudo -S -v. Also accepts stored ADMIN_PASS or ROOT_PASS from the
// state file as alternatives. Returns true if authentication succeeds.
// AuthenticateAdminAccess verifies administrative privileges before performing destructive or elevated operations.
func AuthenticateAdminAccess() bool {
	tui.ClearScreen()
	cardLines := []string{
		core.BOLD + core.RED + "SECURITY AUTHENTICATION REQUIRED" + core.NC,
		"",
		"  Viewing saved system credentials & secrets requires root identity verification.",
		"  Please enter system root/sudo password to authorize access.",
	}
	tui.PrintInBox("--no-wait", "--help-str", "[Esc] -> Cancel Authorization", "SECURITY VERIFICATION", cardLines)

	passInput, status := ReadPasswordInput("System Root/Sudo Password")
	if status != 0 || strings.TrimSpace(passInput) == "" {
		return false
	}

	cleanPass := strings.TrimSpace(passInput)
	_ = exec.Command("sudo", "-k").Run()
	sudoCmd := exec.Command("sudo", "-S", "-v")
	sudoCmd.Stdin = strings.NewReader(cleanPass + "\n")
	if err := sudoCmd.Run(); err == nil {
		return true
	}

	state := system.LoadState()
	if state["ADMIN_PASS"] != "" && subtle.ConstantTimeCompare([]byte(cleanPass), []byte(state["ADMIN_PASS"])) == 1 {
		return true
	}
	if state["ROOT_PASS"] != "" && subtle.ConstantTimeCompare([]byte(cleanPass), []byte(state["ROOT_PASS"])) == 1 {
		return true
	}

	return false
}

// ==============================================================================
// ATOMIC DOUBLE-BUFFERED ZERO-FLICKER SCROLLABLE MENU ENGINE
// ==============================================================================

// selectMenuWithContent renders a full-screen interactive menu with an optional
// live system banner, optional content block, and a scrollable option list.
// Navigation: arrow keys / number keys to move, Enter to confirm, Esc to cancel,
// Backspace to go back. Returns (selectedIndex, selectedText, statusCode):
// status 0 = selected, 1 = backspace, 2 = escape.
