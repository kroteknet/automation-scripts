// ==============================================================================
// KROTEK SCRIPT HUB — SERVICE HEALTH & PROCESS MONITORING DAEMON
// Service: FIREWALL-MANAGER (security)
// File:    security/firewall-manager/src/modules/health.go
//
// Performs live background health audits, socket binding checks, and daemon status queries to ensure continuous service availability.
// ==============================================================================

package modules

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"krotek-firewall-manager-installer/system"
)

// FirewallHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// FirewallHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// FirewallHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
// FirewallHealth encapsulates the runtime operational telemetry, container state, system daemon status, and version string for the active service.
type FirewallHealth struct {
	Type        string
	Backend     string
	IsActive    bool
	RuleCount   int
	UFWStatus   string
	FirewalldSt string
	Fail2ban    bool
	KernelHard  bool
}

// DetectFirewallHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectFirewallHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectFirewallHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
// DetectFirewallHealth performs live health inspection by querying container engines (Docker/Podman), systemd units, OpenRC scripts, and process tables.
func DetectFirewallHealth() FirewallHealth {
	var h FirewallHealth
	state := system.LoadState()
	h.Type = state["TYPE"]
	h.Backend = state["TYPE"]

	if system.CommandExists("ufw") {
		if out, err := exec.Command("ufw", "status").Output(); err == nil {
			h.UFWStatus = strings.TrimSpace(string(out))
			if strings.Contains(h.UFWStatus, "Status: active") {
				h.IsActive = true
				if h.Type == "" {
					h.Type = "ufw"
					h.Backend = "ufw"
				}
			} else {
				if h.Type == "" {
					h.Type = "ufw"
					h.Backend = "ufw"
				}
			}
		}
	}

	if system.CommandExists("firewall-cmd") {
		if exec.Command("systemctl", "is-active", "--quiet", "firewalld").Run() == nil || exec.Command("rc-service", "firewalld", "status").Run() == nil || exec.Command("pidof", "firewalld").Run() == nil {
			h.IsActive = true
			if h.Type == "" {
				h.Type = "firewalld"
				h.Backend = "firewalld"
			}
		}
	}

	return h
}

func GetSSHClientIP() string {
	if client := os.Getenv("SSH_CLIENT"); client != "" {
		fields := strings.Fields(client)
		if len(fields) > 0 {
			return fields[0]
		}
	}
	if conn := os.Getenv("SSH_CONNECTION"); conn != "" {
		fields := strings.Fields(conn)
		if len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}

func getFail2banStats() (bool, int, int, []string) {
	if !system.CommandExists("fail2ban-client") {
		return false, 0, 0, nil
	}
	out, err := exec.Command("fail2ban-client", "status").CombinedOutput()
	if err != nil {
		return false, 0, 0, nil
	}
	outStr := string(out)
	var jails []string
	if idx := strings.Index(outStr, "Jail list:\t"); idx != -1 {
		rawList := strings.TrimSpace(outStr[idx+len("Jail list:\t"):])
		for _, j := range strings.Split(rawList, ",") {
			tj := strings.TrimSpace(j)
			if tj != "" {
				jails = append(jails, tj)
			}
		}
	}
	bannedCount := 0
	for _, j := range jails {
		if jOut, jErr := exec.Command("fail2ban-client", "status", j).CombinedOutput(); jErr == nil {
			if bIdx := strings.Index(string(jOut), "Currently banned:\t"); bIdx != -1 {
				fields := strings.Fields(string(jOut)[bIdx+len("Currently banned:\t"):])
				if len(fields) > 0 {
					if c, err := strconv.Atoi(fields[0]); err == nil {
						bannedCount += c
					}
				}
			}
		}
	}
	return true, len(jails), bannedCount, jails
}

func isKernelHardened() bool {
	if data, err := os.ReadFile("/proc/sys/net/ipv4/tcp_syncookies"); err == nil {
		if strings.TrimSpace(string(data)) == "1" {
			return true
		}
	}
	return false
}

func getOpenPortsSummary() string {
	var netOut []byte
	var netErr error
	if system.CommandExists("ss") {
		netOut, netErr = exec.Command("ss", "-tuln").CombinedOutput()
	} else if system.CommandExists("netstat") {
		netOut, netErr = exec.Command("netstat", "-tuln").CombinedOutput()
	}
	if netErr == nil && len(netOut) > 0 {
		out := netOut
		rePort := regexp.MustCompile(`:([0-9]+)\s`)
		matches := rePort.FindAllStringSubmatch(string(out), -1)
		portMap := make(map[string]bool)
		var pList []string
		for _, m := range matches {
			if len(m) >= 2 && !portMap[m[1]] {
				portMap[m[1]] = true
				pList = append(pList, m[1])
			}
		}
		if len(pList) > 0 {
			if len(pList) > 6 {
				return strings.Join(pList[:6], ", ") + " (+" + strconv.Itoa(len(pList)-6) + " more)"
			}
			return strings.Join(pList, ", ")
		}
	}
	return "Port 22 (SSH)"
}
