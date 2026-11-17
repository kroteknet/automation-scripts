// ==============================================================================
// KROTEK SCRIPT HUB — PERSISTENT INSTALLATION STATE MANAGER
// Service: NFS (media-server)
// File:    media-server/nfs/src/system/state.go
//
// Maintains persistent deployment state on disk across reboot and manager invocations. Records environment type (native vs docker), listening ports, and configuration paths.
// ==============================================================================

package system

import (
	"os"
	"os/exec"
	"strings"

	"krotek-nfs-installer/core"
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
	if out, err := exec.Command("systemctl", "is-active", "nfs-server").CombinedOutput(); err == nil && strings.TrimSpace(string(out)) == "active" {
		return true
	}
	if out, err := exec.Command("systemctl", "is-active", "nfs-kernel-server").CombinedOutput(); err == nil && strings.TrimSpace(string(out)) == "active" {
		return true
	}
	if out, err := exec.Command("systemctl", "is-active", "nfs").CombinedOutput(); err == nil && strings.TrimSpace(string(out)) == "active" {
		return true
	}
	if exec.Command("rc-service", "nfs-server", "status").Run() == nil || exec.Command("pidof", "nfs-server").Run() == nil {
		return true
	}
	if exec.Command("rc-service", "nfs-kernel-server", "status").Run() == nil || exec.Command("pidof", "nfs-kernel-server").Run() == nil {
		return true
	}
	if exec.Command("rc-service", "nfs", "status").Run() == nil || exec.Command("pidof", "nfs").Run() == nil {
		return true
	}
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-nfs").CombinedOutput(); err == nil && strings.TrimSpace(string(out)) == "running" {
		return true
	}
	return false
}
