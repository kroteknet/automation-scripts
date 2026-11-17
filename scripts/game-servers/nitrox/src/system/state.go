// ==============================================================================
// KROTEK SCRIPT HUB — PERSISTENT INSTALLATION STATE MANAGER
// Service: NITROX (game-servers)
// File:    game-servers/nitrox/src/system/state.go
//
// Maintains persistent deployment state on disk across reboot and manager invocations. Records environment type (native vs docker), listening ports, and configuration paths.
// ==============================================================================

package system

import (
	"os"
	"os/exec"
	"strings"

	"krotek-nitrox-installer/core"
)

// SaveState writes installation key-value pairs to persistent state files on disk with 0600 permissions.
func SaveState(data map[string]string) {
	var lines []string
	for k, v := range data {
		lines = append(lines, k+"="+v)
	}
	payload := []byte(strings.Join(lines, "\n") + "\n")
	_ = WriteFileNoFollow(core.StateFile, payload, 0600, "")
	_ = WriteFileNoFollow("/etc/.nitrox_state", payload, 0600, "")
}

// LoadState parses the persistent state file from disk into a key-value map.
func LoadState() map[string]string {
	data := make(map[string]string)
	content, err := os.ReadFile(core.StateFile)
	if err != nil {
		content, err = os.ReadFile("/etc/.nitrox_state")
	}
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
	if _, err := os.Stat("/etc/.nitrox_state"); err == nil {
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
	if out, err := exec.Command("systemctl", "is-active", "nitrox").CombinedOutput(); err == nil && strings.TrimSpace(string(out)) == "active" {
		return true
	}
	if exec.Command("rc-service", "nitrox", "status").Run() == nil || exec.Command("pidof", "nitrox").Run() == nil || exec.Command("pidof", "Nitrox.Server.Subnautica").Run() == nil {
		return true
	}
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-nitrox").CombinedOutput(); err == nil && strings.TrimSpace(string(out)) == "running" {
		return true
	}
	return false
}
