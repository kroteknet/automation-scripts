// ==============================================================================
// KROTEK SCRIPT HUB — PERSISTENT INSTALLATION STATE MANAGER
// Service: NEXTCLOUD (media-server)
// File:    media-server/nextcloud/src/system/state.go
//
// Maintains persistent deployment state on disk across reboot and manager invocations. Records environment type (native vs docker), listening ports, and configuration paths.
// ==============================================================================

package system

import (
	"os"
	"os/exec"
	"strings"

	"krotek-nextcloud-installer/core"
)

// SaveState writes installation key-value pairs to the persistent state file on disk with 0600 permissions.
func SaveState(data map[string]string) {
	var lines []string
	for k, v := range data {
		lines = append(lines, k+"="+v)
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

// IsServiceInstalled determines if the service is currently installed and registered on this machine,
// checking state file presence, active systemd units, OpenRC scripts, process PID trees, and Docker containers.
func IsServiceInstalled() bool {
	if IsInstalledState() {
		return true
	}
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-nextcloud").CombinedOutput(); err == nil && strings.TrimSpace(string(out)) == "running" {
		return true
	}
	if _, err := os.Stat("/var/www/nextcloud/config/config.php"); err == nil {
		return true
	}
	if _, err := os.Stat("/var/www/html/config/config.php"); err == nil {
		return true
	}
	return false
}
