// ==============================================================================
// KROTEK SCRIPT HUB — UNIFIED MINECRAFT SERVICE LIFECYCLE CONTROLLER
// Service: MINECRAFT (game-servers)
// File:    game-servers/minecraft/src/system/service.go
//
// Provides unified lifecycle management (start, stop, restart, health, logs)
// regardless of whether Minecraft is running as a native systemd/OpenRC service
// or as a containerized Docker Compose stack.
// ==============================================================================

package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"krotek-minecraft-installer/core"
)

// GetMinecraftEnvType returns the deployment environment ("docker" or "native").
func GetMinecraftEnvType() string {
	state := LoadState()
	envType := state["ENV_TYPE"]
	if envType != "" {
		return envType
	}

	// Heuristic detection if state file is uninitialized
	if _, err := os.Stat("/etc/minecraft-docker/docker-compose.yml"); err == nil {
		return "docker"
	}
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-minecraft").Output(); err == nil {
		if strings.TrimSpace(string(out)) != "" {
			return "docker"
		}
	}
	return "native"
}

// GetMinecraftDataDir returns the absolute path to the Minecraft server data folder.
func GetMinecraftDataDir() string {
	if GetMinecraftEnvType() == "docker" {
		state := LoadState()
		if d, ok := state["DATA_DIR"]; ok && d != "" {
			return d
		}
		return core.DockerDataDir
	}
	return core.NativeServerDir
}

// GetMinecraftComposeDir returns the Docker Compose project directory.
func GetMinecraftComposeDir() string {
	state := LoadState()
	if cd, ok := state["COMPOSE_DIR"]; ok && cd != "" {
		return cd
	}
	return core.DockerComposeDir
}

// IsMinecraftActive checks if the Minecraft service/container is currently running.
func IsMinecraftActive() bool {
	if GetMinecraftEnvType() == "docker" {
		out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", core.DockerContainerName).Output()
		if err == nil && strings.TrimSpace(string(out)) == "running" {
			return true
		}
		return false
	}

	if IsSystemdRunning() {
		return exec.Command("systemctl", "is-active", "--quiet", "minecraft").Run() == nil
	}
	if exec.Command("rc-service", "minecraft", "status").Run() == nil {
		return true
	}
	return exec.Command("pidof", "minecraft").Run() == nil || exec.Command("pgrep", "-f", "server.jar").Run() == nil
}

// StartMinecraftService starts the Minecraft daemon or Docker container.
func StartMinecraftService() (bool, error) {
	if GetMinecraftEnvType() == "docker" {
		composeDir := GetMinecraftComposeDir()
		if err := ValidateSafeComposeDir(composeDir); err != nil {
			return false, err
		}
		var out []byte
		var err error
		for _, spec := range []struct {
			bin  string
			args []string
		}{
			{"docker", []string{"compose", "up", "-d"}},
			{"docker-compose", []string{"up", "-d"}},
			{"docker", []string{"start", "krotek-minecraft"}},
		} {
			c := exec.Command(spec.bin, spec.args...)
			c.Dir = composeDir
			out, err = c.CombinedOutput()
			if err == nil {
				return true, nil
			}
		}
		return false, fmt.Errorf("docker start failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	if IsSystemdRunning() {
		_ = exec.Command("systemctl", "daemon-reload").Run()
		out, err := exec.Command("systemctl", "start", "minecraft").CombinedOutput()
		if err != nil {
			return false, fmt.Errorf("systemctl start failed: %s (%w)", strings.TrimSpace(string(out)), err)
		}
		return true, nil
	}

	return StartService("minecraft"), nil
}

// StopMinecraftService stops the Minecraft daemon or Docker container.
func StopMinecraftService() (bool, error) {
	if GetMinecraftEnvType() == "docker" {
		composeDir := GetMinecraftComposeDir()
		if err := ValidateSafeComposeDir(composeDir); err != nil {
			return false, err
		}
		var out []byte
		var err error
		for _, spec := range []struct {
			bin  string
			args []string
		}{
			{"docker", []string{"compose", "stop"}},
			{"docker-compose", []string{"stop"}},
			{"docker", []string{"stop", "krotek-minecraft"}},
		} {
			c := exec.Command(spec.bin, spec.args...)
			c.Dir = composeDir
			out, err = c.CombinedOutput()
			if err == nil {
				return true, nil
			}
		}
		return false, fmt.Errorf("docker stop failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	if IsSystemdRunning() {
		out, err := exec.Command("systemctl", "stop", "minecraft").CombinedOutput()
		if err != nil {
			return false, fmt.Errorf("systemctl stop failed: %s (%w)", strings.TrimSpace(string(out)), err)
		}
		return true, nil
	}

	if exec.Command("rc-service", "minecraft", "stop").Run() == nil {
		return true, nil
	}
	return exec.Command("service", "minecraft", "stop").Run() == nil, nil
}

// RestartMinecraftService restarts the Minecraft daemon or Docker container.
func RestartMinecraftService() (bool, error) {
	if GetMinecraftEnvType() == "docker" {
		composeDir := GetMinecraftComposeDir()
		if err := ValidateSafeComposeDir(composeDir); err != nil {
			return false, err
		}
		var out []byte
		var err error
		for _, spec := range []struct {
			bin  string
			args []string
		}{
			{"docker", []string{"compose", "restart"}},
			{"docker-compose", []string{"restart"}},
			{"docker", []string{"restart", "krotek-minecraft"}},
		} {
			c := exec.Command(spec.bin, spec.args...)
			c.Dir = composeDir
			out, err = c.CombinedOutput()
			if err == nil {
				return true, nil
			}
		}
		return false, fmt.Errorf("docker restart failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	if IsSystemdRunning() {
		_ = exec.Command("systemctl", "daemon-reload").Run()
		out, err := exec.Command("systemctl", "restart", "minecraft").CombinedOutput()
		if err != nil {
			return false, fmt.Errorf("systemctl restart failed: %s (%w)", strings.TrimSpace(string(out)), err)
		}
		return true, nil
	}

	return RestartService("minecraft"), nil
}

// GetMinecraftServiceLogs retrieves the latest N lines of Minecraft console/service logs.
func GetMinecraftServiceLogs(n int) ([]string, error) {
	if n <= 0 {
		n = 50
	}

	if GetMinecraftEnvType() == "docker" {
		composeDir := GetMinecraftComposeDir()
		if err := ValidateSafeComposeDir(composeDir); err != nil {
			return nil, err
		}
		nArg := fmt.Sprintf("%d", n)
		for _, spec := range []struct {
			bin  string
			args []string
		}{
			{"docker", []string{"compose", "logs", "--tail", nArg}},
			{"docker", []string{"logs", "--tail", nArg, "krotek-minecraft"}},
		} {
			c := exec.Command(spec.bin, spec.args...)
			c.Dir = composeDir
			if out, err := c.CombinedOutput(); err == nil {
				return strings.Split(string(out), "\n"), nil
			}
		}
		c := exec.Command("docker", "logs", "--tail", nArg, "krotek-minecraft")
		out, err := c.CombinedOutput()
		if err != nil {
			return nil, err
		}
		return strings.Split(string(out), "\n"), nil
	}

	// Try reading latest.log directly first for clean game output
	serverDir := GetMinecraftDataDir()
	latestLog := filepath.Join(serverDir, "logs", "latest.log")
	if data, err := os.ReadFile(latestLog); err == nil {
		lines := strings.Split(string(data), "\n")
		if len(lines) > n {
			return lines[len(lines)-n:], nil
		}
		return lines, nil
	}

	// Fallback to journalctl
	out, err := exec.Command("journalctl", "-u", "minecraft", "-n", fmt.Sprintf("%d", n), "--no-pager").CombinedOutput()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(out), "\n"), nil
}

// WaitForMinecraftHealthy polls for up to timeoutSeconds until Minecraft is active.
func WaitForMinecraftHealthy(timeoutSeconds int) bool {
	for i := 0; i < timeoutSeconds; i++ {
		time.Sleep(1 * time.Second)
		if IsMinecraftActive() {
			return true
		}
	}
	return false
}

// OpenUniversalFirewallPort opens a TCP or UDP port across all detected firewall engines.
func OpenUniversalFirewallPort(port string, proto string) []string {
	var results []string
	proto = strings.ToLower(proto)
	if proto == "" {
		proto = "tcp"
	}

	if CommandExists("iptables") {
		_ = exec.Command("iptables", "-I", "INPUT", "-p", proto, "--dport", port, "-j", "ACCEPT").Run()
		results = append(results, fmt.Sprintf("%s[OK] iptables: Port %s %s allowed%s", core.GREEN, port, strings.ToUpper(proto), core.NC))
	}
	if CommandExists("ip6tables") {
		_ = exec.Command("ip6tables", "-I", "INPUT", "-p", proto, "--dport", port, "-j", "ACCEPT").Run()
		results = append(results, fmt.Sprintf("%s[OK] ip6tables: Port %s %s allowed%s", core.GREEN, port, strings.ToUpper(proto), core.NC))
	}
	if CommandExists("nft") {
		_ = exec.Command("nft", "add", "rule", "inet", "filter", "input", proto, "dport", port, "accept").Run()
		results = append(results, fmt.Sprintf("%s[OK] nftables: Port %s %s allowed%s", core.GREEN, port, strings.ToUpper(proto), core.NC))
	}
	if CommandExists("ufw") {
		_ = exec.Command("ufw", "allow", fmt.Sprintf("%s/%s", port, proto)).Run()
		results = append(results, fmt.Sprintf("%s[OK] ufw: Port %s/%s allowed%s", core.GREEN, port, proto, core.NC))
	}
	if CommandExists("firewall-cmd") {
		_ = exec.Command("firewall-cmd", fmt.Sprintf("--add-port=%s/%s", port, proto), "--permanent").Run()
		_ = exec.Command("firewall-cmd", "--reload").Run()
		results = append(results, fmt.Sprintf("%s[OK] firewalld: Port %s/%s allowed%s", core.GREEN, port, proto, core.NC))
	}
	return results
}
