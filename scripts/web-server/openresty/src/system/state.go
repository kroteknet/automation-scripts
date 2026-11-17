// ==============================================================================
// KROTEK SCRIPT HUB — PERSISTENT INSTALLATION STATE MANAGER
// Service: OpenResty (web-server)
// File:    web-server/openresty/src/system/state.go
//
// Maintains persistent deployment state on disk across reboot and manager invocations.
// Records environment type (native vs docker), listening ports, and configuration paths.
// ==============================================================================

package system

import (
	"os"
	"strings"

	"krotek-openresty-installer/core"
)

// sanitizeStateToken strips control characters and newlines from state keys/values.
func sanitizeStateToken(s string, stripEq bool) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == 0 {
			return -1
		}
		if stripEq && r == '=' {
			return -1
		}
		return r
	}, s)
}

// SaveState writes installation key-value pairs to the persistent state file on disk with 0600 permissions.
func SaveState(data map[string]string) {
	var lines []string
	for k, v := range data {
		cleanK := sanitizeStateToken(strings.TrimSpace(k), true)
		if cleanK == "" {
			continue
		}
		cleanV := sanitizeStateToken(v, false)
		lines = append(lines, cleanK+"="+cleanV)
	}
	_ = WriteFileNoFollow(core.StateFile, []byte(strings.Join(lines, "\n")), 0600, "")
}

// LoadState parses the persistent state file from disk into a key-value map.
func LoadState() map[string]string {
	data := make(map[string]string)
	content, err := os.ReadFile(core.StateFile)
	if err == nil {
		for _, line := range strings.Split(string(content), "\n") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				data[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}
	return data
}

func IsInstalledState() bool {
	if _, err := os.Stat(core.StateFile); err == nil {
		return true
	}
	return false
}

// IsServiceInstalled determines if OpenResty is currently installed and registered on this machine.
func IsServiceInstalled() bool {
	return IsOpenRestyInstalled()
}
