// ==============================================================================
// KROTEK SCRIPT HUB — INTRUSION PREVENTION & FAIL2BAN INTEGRATION
// Service: FIREWALL-MANAGER (security)
// File:    security/firewall-manager/src/modules/fail2ban.go
//
// Configures Fail2ban jails, monitors authentication failure logs, and enforces temporary IP bans against brute-force attacks.
// ==============================================================================

package modules

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"krotek-firewall-manager-installer/core"
	"krotek-firewall-manager-installer/system"
	"krotek-firewall-manager-installer/tui"
)

// Fail2banMenu provides intrusion prevention controls, jail status viewers, and IP unbanning actions.
func Fail2banMenu() {
	if !system.CommandExists("fail2ban-client") {
		tui.ClearScreen()
		conf := tui.AskYesNo("Fail2ban is not currently installed. Install and configure Fail2ban now?")
		if conf == 0 {
			tui.ClearScreen()
			if core.SysInfo.Family == "debian" {
				system.RunStepOrAbort("Installing Fail2ban", // Dpkg options preserve existing configuration and prevent blocking terminal prompts in TUI & automation
					"apt-get", "install", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "fail2ban")
			} else {
				system.RunStepOrAbort("Installing Fail2ban", "dnf", "install", "-y", "fail2ban")
			}
			system.StartService("fail2ban")
		} else {
			return
		}
	}

	for {
		tui.ClearScreen()
		active, jailCount, bannedCount, jails := getFail2banStats()

		options := []string{
			"─── FAIL2BAN INTRUSION PREVENTION ───",
			"[1] View Active Jails & Security Metrics",
			"[2] List Banned IPs & Quick Unban Engine",
			"[3] Add IP to Whitelist (ignoreip)",
			"[4] Restart Fail2ban Service Daemon",
			"─── NAVIGATION ───",
			"Back to Main Menu",
		}

		titleStr := fmt.Sprintf("FAIL2BAN ENGINE - Active: %d Jails | %d Banned Threats", jailCount, bannedCount)
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)
		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Back to Main Menu") {
			return
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "[1] View Active Jails"):
			tui.ClearScreen()
			var jLines []string
			jLines = append(jLines, tui.FormatKV("Fail2ban Service Status", func() string {
				if active {
					return core.GREEN + core.BOLD + "ACTIVE & RUNNING" + core.NC
				}
				return core.RED + core.BOLD + "INACTIVE" + core.NC
			}(), core.GREEN))
			jLines = append(jLines, tui.FormatKV("Active Jails Count", strconv.Itoa(jailCount), core.CYAN))
			jLines = append(jLines, tui.FormatKV("Total Banned Threats", strconv.Itoa(bannedCount), core.RED+core.BOLD))
			jLines = append(jLines, "")

			for _, j := range jails {
				if out, err := exec.Command("fail2ban-client", "status", j).Output(); err == nil {
					jLines = append(jLines, core.BOLD+core.YELLOW+core.Sym.Pointer+" Jail: "+j+core.NC)
					lines := strings.Split(string(out), "\n")
					for _, l := range lines {
						trimmed := strings.TrimSpace(core.SanitizeTerminalText(l))
						if parts := strings.SplitN(trimmed, ":", 2); len(parts) == 2 {
							k := strings.TrimSpace(parts[0])
							v := strings.TrimSpace(parts[1])
							switch k {
							case "Currently failed", "Total failed":
								c := core.GREEN
								if v != "0" {
									c = core.YELLOW
								}
								jLines = append(jLines, tui.FormatKV(k, v, c))
							case "Currently banned":
								c := core.GREEN
								if v != "0" {
									c = core.RED + core.BOLD
								}
								jLines = append(jLines, tui.FormatKV(k, v, c))
							case "Banned IP list":
								c := core.DIM
								if v != "" {
									c = core.RED + core.BOLD
								} else {
									v = "(none)"
								}
								jLines = append(jLines, tui.FormatKV(k, v, c))
							}
						}
					}
				}
			}
			tui.PrintInBox("Fail2ban Active Jails Detailed Report", jLines)

		case strings.HasPrefix(cleanOpt, "[2] List Banned IPs"):
			Fail2banUnbanMenu()

		case strings.HasPrefix(cleanOpt, "[3] Add IP to Whitelist"):
			tui.ClearScreen()
			ip, stat := tui.ReadTextInput("Enter IP or Subnet to Whitelist in Fail2ban", GetSSHClientIP())
			if stat == 0 && strings.TrimSpace(ip) != "" {
				cleanIP := strings.TrimSpace(ip)
				if strings.ContainsAny(cleanIP, " \t\r\n;$\"'`><") {
					tui.PrintInBox("Validation Error", []string{core.RED + "IP or subnet contains illegal characters or whitespace." + core.NC})
					continue
				}
				if net.ParseIP(cleanIP) == nil {
					if _, _, err := net.ParseCIDR(cleanIP); err != nil {
						tui.PrintInBox("Validation Error", []string{core.RED + "Invalid IP address or CIDR notation: " + cleanIP + core.NC})
						continue
					}
				}
				confPath := "/etc/fail2ban/jail.local"
				if _, err := os.Stat(confPath); os.IsNotExist(err) {
					_ = system.WriteFileNoFollow(confPath, []byte("[DEFAULT]\nignoreip = 127.0.0.1/8 ::1 "+cleanIP+"\n"), 0600, "")
				} else {
					if data, err := os.ReadFile(confPath); err == nil {
						content := string(data)
						if strings.Contains(content, "ignoreip") {
							content = strings.Replace(content, "ignoreip =", "ignoreip = "+cleanIP+" ", 1)
						} else {
							content = "[DEFAULT]\nignoreip = 127.0.0.1/8 ::1 " + cleanIP + "\n" + content
						}
						_ = system.WriteFileNoFollow(confPath, []byte(content), 0600, "")
					}
				}
				_ = exec.Command("systemctl", "restart", "fail2ban").Run()
				tui.ClearScreen()
				var wLines []string
				wLines = append(wLines, tui.FormatKV("Whitelisted IP", cleanIP, core.GREEN+core.BOLD))
				wLines = append(wLines, tui.FormatKV("Config File", confPath, core.CYAN))
				wLines = append(wLines, tui.FormatKV("Status", "SUCCESS (Fail2ban Reloaded)", core.GREEN))
				tui.PrintInBox("Fail2ban Whitelist Result", wLines)
			}

		case strings.HasPrefix(cleanOpt, "[4] Restart Fail2ban"):
			tui.ClearScreen()
			var rLines []string
			if system.RestartService("fail2ban") {
				rLines = append(rLines, tui.FormatKV("Status", "SUCCESS", core.GREEN+core.BOLD))
			} else {
				rLines = append(rLines, tui.FormatKV("Status", "FAILED", core.RED+core.BOLD))
			}
			tui.PrintInBox("Fail2ban Restart Result", rLines)
		}
	}
}

// Fail2banUnbanMenu displays currently banned IP addresses across all active jails with one-click unban capability.
func Fail2banUnbanMenu() {
	for {
		tui.ClearScreen()
		type BannedEntry struct {
			Jail string
			IP   string
		}
		var bannedList []BannedEntry

		_, _, _, jails := getFail2banStats()
		for _, j := range jails {
			if out, err := exec.Command("fail2ban-client", "status", j).Output(); err == nil {
				lines := strings.Split(string(out), "\n")
				for _, l := range lines {
					if strings.Contains(l, "Banned IP list:") {
						idx := strings.Index(l, "Banned IP list:")
						ipStr := strings.TrimSpace(l[idx+15:])
						for _, ip := range strings.Fields(ipStr) {
							cleanIP := strings.TrimSpace(ip)
							if cleanIP != "" {
								bannedList = append(bannedList, BannedEntry{Jail: j, IP: cleanIP})
							}
						}
					}
				}
			}
		}

		if len(bannedList) == 0 {
			var noLines []string
			noLines = append(noLines, tui.FormatKV("Status", "CLEAN (No active bans)", core.GREEN+core.BOLD))
			noLines = append(noLines, "There are currently 0 blocked IPs across all active Fail2ban jails.")
			tui.PrintInBox("Banned IPs Inspector", noLines)
			return
		}

		var options []string
		options = append(options, "─── SELECT BANNED IP TO UNBAN ───")
		for _, b := range bannedList {
			options = append(options, fmt.Sprintf("Jail: %-15s  ->  IP: %-18s  [UNBAN]", b.Jail, b.IP))
		}
		options = append(options, "─── NAVIGATION ───")
		options = append(options, "Back to Previous Menu")

		titleStr := fmt.Sprintf("FAIL2BAN UNBAN ENGINE - %d Active Threats Blocked", len(bannedList))
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)
		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Back to Previous Menu") {
			return
		}

		cleanOpt := core.StripANSI(selectedOpt)
		for _, b := range bannedList {
			if strings.Contains(cleanOpt, b.IP) && strings.Contains(cleanOpt, b.Jail) {
				tui.ClearScreen()
				conf := tui.AskYesNo(fmt.Sprintf("Unban IP address %s from jail [%s]?", b.IP, b.Jail))
				if conf == 0 {
					tui.ClearScreen()
					var uRes []string
					if system.RunStep(fmt.Sprintf("Unbanning %s from %s", b.IP, b.Jail), "fail2ban-client", "set", b.Jail, "unbanip", b.IP) {
						uRes = append(uRes, tui.FormatKV("Unbanned IP", b.IP, core.GREEN+core.BOLD))
						uRes = append(uRes, tui.FormatKV("Jail", b.Jail, core.CYAN))
						uRes = append(uRes, tui.FormatKV("Status", "SUCCESSFULLY UNBANNED", core.GREEN))
					} else {
						uRes = append(uRes, tui.FormatKV("Status", "FAILED TO UNBAN", core.RED))
					}
					tui.PrintInBox("Unban Result", uRes)
				}
				break
			}
		}
	}
}
