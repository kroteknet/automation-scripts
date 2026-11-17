// ==============================================================================
// KROTEK SCRIPT HUB — SECURITY VULNERABILITY & PORT SCANNER
// Service: FIREWALL-MANAGER (security)
// File:    security/firewall-manager/src/modules/scanner.go
//
// Scans local and network interfaces for exposed ports, outdated daemon versions, and common configuration weaknesses.
// ==============================================================================

package modules

import (
	"fmt"
	"os/exec"
	"strings"

	"krotek-firewall-manager-installer/core"
	"krotek-firewall-manager-installer/tui"
)

// OpenPortsScanner probes local network interfaces to identify all actively listening TCP and UDP service ports.
func OpenPortsScanner() {
	tui.ClearScreen()
	var portLines []string
	portLines = append(portLines, core.BOLD+core.YELLOW+"LOCAL LISTENING NETWORK PORTS & PROCESS SCANNER"+core.NC)
	portLines = append(portLines, "")

	if out, err := exec.Command("ss", "-tulpn").CombinedOutput(); err == nil && len(out) > 0 {
		lines := strings.Split(string(out), "\n")
		header := fmt.Sprintf("%-6s %-24s %-20s %-16s", "PROTO", "LOCAL ADDRESS", "PROCESS (PID)", "EXPOSURE")
		portLines = append(portLines, core.BOLD+core.CYAN+header+core.NC)
		portLines = append(portLines, strings.Repeat(core.Sym.BoxH, 70))

		for _, l := range lines {
			fields := strings.Fields(l)
			if len(fields) >= 5 && (fields[0] == "tcp" || fields[0] == "udp") {
				proto := strings.ToUpper(fields[0])
				addr := fields[4]
				proc := "N/A"
				if len(fields) >= 7 {
					proc = fields[6]
					if idx := strings.Index(proc, "\""); idx != -1 {
						endQ := strings.Index(proc[idx+1:], "\"")
						if endQ != -1 {
							proc = proc[idx+1 : idx+1+endQ]
						}
					}
				}

				exposure := core.GREEN + "[LOCAL ONLY]" + core.NC
				if strings.HasPrefix(addr, "0.0.0.0:") || strings.HasPrefix(addr, "*:") || strings.HasPrefix(addr, "[::]:") || strings.HasPrefix(addr, ":::") {
					exposure = core.YELLOW + core.BOLD + "[PUBLIC WAN]" + core.NC
					if strings.Contains(addr, ":3306") || strings.Contains(addr, ":5432") || strings.Contains(addr, ":6379") || strings.Contains(addr, ":27017") {
						exposure = core.RED + core.BOLD + "[CRITICAL: DB EXPOSED]" + core.NC
					}
				}

				row := fmt.Sprintf("%-6s %-24s %-20s %-16s", proto, addr, proc, exposure)
				portLines = append(portLines, row)
			}
		}
	} else {
		portLines = append(portLines, "ss command unavailable. Ensure iproute2 package is installed.")
	}

	tui.PrintInBox("Open Listening Ports Diagnostic Report", portLines)
}

// ActiveConnectionsInspector inspects established remote socket connections, foreign IP addresses, and throughput telemetry.
func ActiveConnectionsInspector() {
	tui.ClearScreen()
	var connLines []string
	connLines = append(connLines, core.BOLD+core.YELLOW+"ACTIVE ESTABLISHED NETWORK CONNECTIONS"+core.NC)
	connLines = append(connLines, "")

	if out, err := exec.Command("ss", "-tun", "state", "established").CombinedOutput(); err == nil && len(out) > 0 {
		lines := strings.Split(string(out), "\n")
		header := fmt.Sprintf("%-6s %-24s %-24s", "PROTO", "LOCAL ADDRESS", "PEER / REMOTE ADDRESS")
		connLines = append(connLines, core.BOLD+core.CYAN+header+core.NC)
		connLines = append(connLines, strings.Repeat(core.Sym.BoxH, 65))

		count := 0
		for _, l := range lines {
			fields := strings.Fields(l)
			if len(fields) >= 5 && (fields[0] == "tcp" || fields[0] == "udp") {
				proto := strings.ToUpper(fields[0])
				local := fields[3]
				peer := fields[4]
				connLines = append(connLines, fmt.Sprintf("%-6s %-24s %-24s", proto, local, peer))
				count++
			}
		}
		if count == 0 {
			connLines = append(connLines, core.DIM+"  No external established connections at this moment."+core.NC)
		}
	} else {
		connLines = append(connLines, core.YELLOW+"[INFO] Unable to query active connections."+core.NC)
	}

	tui.PrintInBox("Active Connections Inspector", connLines)
}

// ==============================================================================
// KERNEL HARDENING, PANIC MODE & BACKUPS
// ==============================================================================
