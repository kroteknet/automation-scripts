// ==============================================================================
// KROTEK SCRIPT HUB — DEEP SYSTEM DIAGNOSTICS & NETWORK AUDIT
// Service: FIREWALL-MANAGER (security)
// File:    security/firewall-manager/src/modules/diagnostics.go
//
// Executes comprehensive network connectivity, DNS resolution, port listening, and socket state diagnostics.
// ==============================================================================

package modules

import (
	"fmt"
	"strings"

	"krotek-firewall-manager-installer/core"
	"krotek-firewall-manager-installer/system"
	"krotek-firewall-manager-installer/tui"
)

func RunDeepDiagnostics() {
	tui.ClearScreen()
	h := DetectFirewallHealth()

	fwType := h.Backend
	if fwType == "" {
		fwType = "NONE"
	}

	f2bActive, jailCount, bannedTotal, _ := getFail2banStats()
	hardened := isKernelHardened()

	diagReport := []string{
		core.BOLD + core.YELLOW + "DEEP FIREWALL & SECURITY DIAGNOSTICS SCAN" + core.NC,
		"",
		tui.FormatKV("Host Operating System", system.GetOSPrettyVersion(), core.CYAN),
		tui.FormatKV("System Architecture", core.SysInfo.Architecture, core.CYAN),
		tui.FormatKV("Active Firewall Backend", func() string {
			if h.IsActive {
				return core.GREEN + "[ACTIVE] (" + fwType + ")"
			}
			if fwType != "" && fwType != "NONE" {
				return core.YELLOW + "[INSTALLED BUT INACTIVE] (" + fwType + ")"
			}
			return core.RED + "[NO ACTIVE FIREWALL DETECTED]"
		}(), core.GREEN),
		tui.FormatKV("UFW Status", system.GetToolBadge("ufw"), core.YELLOW),
		tui.FormatKV("Firewalld Status", system.GetToolBadge("firewall-cmd"), core.YELLOW),
		tui.FormatKV("Iptables Status", system.GetToolBadge("iptables"), core.YELLOW),
		tui.FormatKV("Fail2ban Status", func() string {
			if f2bActive {
				return fmt.Sprintf("%s[ACTIVE] (%d Jails, %d Banned IPs)%s", core.GREEN, jailCount, bannedTotal, core.NC)
			}
			return core.RED + "[INACTIVE / NOT INSTALLED]" + core.NC
		}(), core.GREEN),
		tui.FormatKV("Kernel Hardening", func() string {
			if hardened {
				return core.GREEN + "[HARDENED (SYN Flood & Anti-Spoof Active)]" + core.NC
			}
			return core.YELLOW + "[STANDARD (Default sysctl)]" + core.NC
		}(), core.GREEN),
		tui.FormatKV("Public Listening Ports", getOpenPortsSummary(), core.CYAN),
		tui.FormatKV("Management CLI Command", core.GREEN+"firewall-manager"+core.NC, core.GREEN),
	}

	if h.UFWStatus != "" {
		diagReport = append(diagReport, "")
		diagReport = append(diagReport, core.BOLD+core.CYAN+"UFW Active Rules Output:"+core.NC)
		for _, line := range strings.Split(h.UFWStatus, "\n") {
			trimmed := strings.TrimSpace(core.SanitizeTerminalText(line))
			if trimmed == "" {
				continue
			}
			upper := strings.ToUpper(trimmed)
			switch {
			case strings.HasPrefix(upper, "STATUS: ACTIVE"):
				diagReport = append(diagReport, tui.FormatKV("UFW Status", "ACTIVE", core.GREEN+core.BOLD))
			case strings.HasPrefix(upper, "STATUS: INACTIVE"):
				diagReport = append(diagReport, tui.FormatKV("UFW Status", "INACTIVE", core.RED+core.BOLD))
			case strings.HasPrefix(trimmed, "To ") || strings.HasPrefix(trimmed, "--"):
				diagReport = append(diagReport, "  "+core.BOLD+core.CYAN+trimmed+core.NC)
			case strings.Contains(upper, "ALLOW"):
				diagReport = append(diagReport, "  "+core.GREEN+trimmed+core.NC)
			case strings.Contains(upper, "DENY") || strings.Contains(upper, "REJECT"):
				diagReport = append(diagReport, "  "+core.RED+trimmed+core.NC)
			case strings.Contains(upper, "LIMIT"):
				diagReport = append(diagReport, "  "+core.YELLOW+trimmed+core.NC)
			default:
				diagReport = append(diagReport, "  "+trimmed)
			}
		}
	}

	tui.PrintInBox("Deep Firewall & Security Diagnostics Report", diagReport)
}
