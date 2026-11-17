// ==============================================================================
// KROTEK SCRIPT HUB — SYSTEM & DAEMON SECURITY HARDENING
// Service: FIREWALL-MANAGER (security)
// File:    security/firewall-manager/src/modules/hardening.go
//
// Applies OS-level security hardening, sysctl network optimizations, file permission lockdowns, and SSH security policies.
// ==============================================================================

package modules

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"krotek-firewall-manager-installer/core"
	"krotek-firewall-manager-installer/system"
	"krotek-firewall-manager-installer/tui"
)

// KernelHardeningMenu configures OS-level sysctl network security parameters (SYN cookies, reverse path filtering, ICMP broadcast ignores).
func KernelHardeningMenu() {
	for {
		tui.ClearScreen()
		hardened := isKernelHardened()

		options := []string{
			"─── KERNEL SYSCTL SECURITY TWEAKS ───",
			"[1] Enable Anti-DDoS & Kernel Hardening (SYN Cookies, Anti-Spoofing)",
			"[2] Disable Kernel Hardening (Restore System Defaults)",
			"[3] View Current Sysctl Security Parameters",
			"─── NAVIGATION ───",
			"Back to Main Menu",
		}

		titleStr := "KERNEL HARDENING ENGINE - Status: "
		if hardened {
			titleStr += "[HARDENED]"
		} else {
			titleStr += "[STANDARD]"
		}

		_, selectedOpt, status := tui.SelectMenu(titleStr, options)
		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Back to Main Menu") {
			return
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "[1] Enable Anti-DDoS"):
			tui.ClearScreen()
			sysctlContent := `# Krotek Security Engine Hardening Configuration
net.ipv4.tcp_syncookies = 1
net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.default.rp_filter = 1
net.ipv4.icmp_echo_ignore_broadcasts = 1
net.ipv4.icmp_ignore_bogus_error_responses = 1
net.ipv4.conf.all.accept_source_route = 0
net.ipv4.conf.default.accept_source_route = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.default.send_redirects = 0
net.ipv4.tcp_max_syn_backlog = 2048
net.ipv4.tcp_synack_retries = 2
net.ipv4.tcp_rfc1337 = 1
`
			_ = system.WriteFileNoFollow("/etc/sysctl.d/99-krotek-security.conf", []byte(sysctlContent), 0644, "")
			_ = exec.Command("sysctl", "--system").Run()

			var res []string
			res = append(res, tui.FormatKV("SYN Cookies (Anti-SYN Flood)", "ENABLED (net.ipv4.tcp_syncookies=1)", core.GREEN+core.BOLD))
			res = append(res, tui.FormatKV("Reverse Path Filter (Anti-Spoof)", "ENABLED (rp_filter=1)", core.GREEN+core.BOLD))
			res = append(res, tui.FormatKV("ICMP Broadcast Smurf Protection", "ENABLED (ignore_broadcasts=1)", core.GREEN+core.BOLD))
			res = append(res, tui.FormatKV("Source Routing Protection", "ENABLED (accept_source_route=0)", core.GREEN+core.BOLD))
			res = append(res, tui.FormatKV("Configuration Path", "/etc/sysctl.d/99-krotek-security.conf", core.CYAN))
			tui.PrintInBox("Kernel Security Hardening Applied", res)

		case strings.HasPrefix(cleanOpt, "[2] Disable Kernel Hardening"):
			tui.ClearScreen()
			_ = os.Remove("/etc/sysctl.d/99-krotek-security.conf")
			_ = exec.Command("sysctl", "--system").Run()
			var res []string
			res = append(res, tui.FormatKV("Kernel Hardening", "REMOVED", core.YELLOW))
			res = append(res, tui.FormatKV("Status", "RESTORED TO SYSTEM DEFAULTS", core.GREEN))
			tui.PrintInBox("Kernel Hardening Reverted", res)

		case strings.HasPrefix(cleanOpt, "[3] View Current Sysctl"):
			tui.ClearScreen()
			var pLines []string
			pLines = append(pLines, core.BOLD+core.YELLOW+"ACTIVE KERNEL NETWORK SECURITY PARAMETERS:"+core.NC)
			pLines = append(pLines, "")
			params := []string{
				"net.ipv4.tcp_syncookies",
				"net.ipv4.conf.all.rp_filter",
				"net.ipv4.icmp_echo_ignore_broadcasts",
				"net.ipv4.conf.all.accept_source_route",
				"net.ipv4.conf.all.send_redirects",
				"net.ipv4.tcp_max_syn_backlog",
			}
			for _, p := range params {
				if out, err := exec.Command("sysctl", "-n", p).Output(); err == nil {
					pLines = append(pLines, tui.FormatKV(p, strings.TrimSpace(string(out)), core.CYAN))
				}
			}
			tui.PrintInBox("Sysctl Security Parameters", pLines)
		}
	}
}

// PanicEmergencyLockdown executes an emergency port shutdown, closing all non-SSH incoming traffic in under 1 second.
func PanicEmergencyLockdown() {
	tui.ClearScreen()
	adminIP := GetSSHClientIP()
	if adminIP == "" || adminIP == "127.0.0.1" {
		tui.ClearScreen()
		tui.PrintInBox("Admin IP Required", []string{
			core.YELLOW + "Could not automatically determine your remote SSH Client IP." + core.NC,
			core.YELLOW + "Please enter the public or VPN IP address you are connecting from" + core.NC,
			core.YELLOW + "to ensure your SSH connection is not severed during emergency lockdown." + core.NC,
		})
		enteredIP, stat := tui.ReadTextInput("Enter your current Admin IP (e.g. 203.0.113.4)", "")
		if stat != 0 || strings.TrimSpace(enteredIP) == "" {
			tui.PrintInBox("Aborted", []string{core.RED + "Emergency lockdown aborted: Admin IP was not specified." + core.NC})
			return
		}
		adminIP = strings.TrimSpace(enteredIP)
	}

	if net.ParseIP(adminIP) == nil {
		tui.PrintInBox("Aborted", []string{core.RED + "Invalid Admin IP address format: " + adminIP + core.NC})
		return
	}

	warningLines := []string{
		core.BOLD + core.RED + "EMERGENCY PANIC MODE: FULL INBOUND LOCKDOWN" + core.NC,
		"",
		tui.FormatKV("Detected Admin IP", adminIP, core.GREEN+core.BOLD),
		tui.FormatKV("Default Policy", "DROP / DENY ALL INCOMING TRAFFIC", core.RED+core.BOLD),
		tui.FormatKV("SSH Access", fmt.Sprintf("ALLOWED ONLY FROM %s", adminIP), core.GREEN+core.BOLD),
		"",
		core.BOLD + core.YELLOW + "Are you sure you want to trigger Emergency Lockdown?" + core.NC,
	}

	cIdx, _, cStat := tui.SelectMenuWithContent("Confirm Emergency Panic Mode", warningLines, []string{"LOCK DOWN NOW", "CANCEL"})
	if cStat != 0 || cIdx != 0 {
		return
	}

	tui.ClearScreen()
	var res []string
	if system.CommandExists("ufw") {
		system.RunStep("Setting Default Inbound Deny Policy", "ufw", "default", "deny", "incoming")
		system.RunStep("Setting Default Outbound Allow Policy", "ufw", "default", "allow", "outgoing")
		system.RunStep("Allowing SSH from Admin IP "+adminIP, "ufw", "allow", "from", adminIP, "to", "any", "port", "22", "proto", "tcp")
		_ = exec.Command("ufw", "reload").Run()
		res = append(res, tui.FormatKV("Emergency Policy", "ACTIVE", core.RED+core.BOLD))
		res = append(res, tui.FormatKV("Protected SSH IP", adminIP, core.GREEN+core.BOLD))
		res = append(res, tui.FormatKV("Status", "ALL INBOUND TRAFFIC LOCKED DOWN", core.GREEN))
	} else {
		res = append(res, tui.FormatKV("Error", "UFW not found", core.RED))
	}
	tui.PrintInBox("Emergency Lockdown Activated", res)
}
