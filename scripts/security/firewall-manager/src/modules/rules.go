// ==============================================================================
// KROTEK SCRIPT HUB — FIREWALL POLICY & RULE COMPILER
// Service: FIREWALL-MANAGER (security)
// File:    security/firewall-manager/src/modules/rules.go
//
// Compiles high-level network rules into low-level firewall configuration syntax for iptables, nftables, and zone filters.
// ==============================================================================

package modules

import (
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"krotek-firewall-manager-installer/core"
	"krotek-firewall-manager-installer/system"
	"krotek-firewall-manager-installer/tui"
)

// ViewAndManageRulesMenu lists all active firewall policies and packet filter rules across UFW, Firewalld, or iptables.
func ViewAndManageRulesMenu() {
	for {
		tui.ClearScreen()
		h := DetectFirewallHealth()
		var options []string
		options = append(options, "─── ACTIVE FIREWALL RULES ───")

		var rawRules []string
		if system.CommandExists("ufw") {
			if out, err := exec.Command("ufw", "status", "numbered").CombinedOutput(); err == nil {
				lines := strings.Split(string(out), "\n")
				inRules := false
				for _, l := range lines {
					trimmed := strings.TrimSpace(core.SanitizeTerminalText(l))
					if strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "]") {
						inRules = true
						rawRules = append(rawRules, trimmed)
						upper := strings.ToUpper(trimmed)
						ruleColor := core.WHITE
						if strings.Contains(upper, "ALLOW") {
							ruleColor = core.GREEN
						} else if strings.Contains(upper, "DENY") || strings.Contains(upper, "REJECT") {
							ruleColor = core.RED
						} else if strings.Contains(upper, "LIMIT") {
							ruleColor = core.YELLOW
						}
						options = append(options, fmt.Sprintf("%s%-55s%s -> %s[DELETE]%s", ruleColor, trimmed, core.NC, core.RED+core.BOLD, core.NC))
					}
				}
				if !inRules {
					options = append(options, core.DIM+"[No active rules found in UFW]"+core.NC)
				}
			}
		} else if system.CommandExists("firewall-cmd") {
			if out, err := exec.Command("firewall-cmd", "--list-all").CombinedOutput(); err == nil {
				lines := strings.Split(string(out), "\n")
				for _, l := range lines {
					trimmed := strings.TrimSpace(core.SanitizeTerminalText(l))
					if strings.HasPrefix(trimmed, "ports:") || strings.HasPrefix(trimmed, "services:") || strings.HasPrefix(trimmed, "rich rules:") {
						options = append(options, core.CYAN+trimmed+core.NC)
					}
				}
			}
		} else {
			if out, err := exec.Command("iptables", "-L", "-n", "--line-numbers").CombinedOutput(); err == nil {
				lines := strings.Split(string(out), "\n")
				for _, l := range lines {
					trimmed := strings.TrimSpace(core.SanitizeTerminalText(l))
					if trimmed != "" && !strings.HasPrefix(trimmed, "Chain") && !strings.HasPrefix(trimmed, "num") {
						upper := strings.ToUpper(trimmed)
						ruleColor := core.WHITE
						if strings.Contains(upper, "ACCEPT") {
							ruleColor = core.GREEN
						} else if strings.Contains(upper, "DROP") || strings.Contains(upper, "REJECT") {
							ruleColor = core.RED
						}
						options = append(options, ruleColor+trimmed+core.NC)
					}
				}
			}
		}

		options = append(options, "─── RULE ACTIONS ───")
		options = append(options, "[+] Add New Port / Service Rule")
		options = append(options, "[*] Quick Service Security Presets")
		options = append(options, "[R] Reload Firewall Ruleset")
		options = append(options, "─── NAVIGATION ───")
		options = append(options, "Back to Main Menu")

		titleStr := "FIREWALL & SECURITY MANAGER - Active Rules"
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)
		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Back to Main Menu") {
			return
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.HasPrefix(cleanOpt, "[+] Add New Port"):
			AddNewRuleWizard()
		case strings.HasPrefix(cleanOpt, "[*] Quick Service"):
			QuickPresetsMenu()
		case strings.HasPrefix(cleanOpt, "[R] Reload"):
			tui.ClearScreen()
			var res []string
			if h.Type == "firewalld" {
				if system.RunStep("Reloading Firewalld", "firewall-cmd", "--reload") {
					res = append(res, tui.FormatKV("Status", "SUCCESS", core.GREEN+core.BOLD))
				} else {
					res = append(res, tui.FormatKV("Status", "FAILED", core.RED+core.BOLD))
				}
			} else {
				if system.RunStep("Reloading UFW", "ufw", "reload") {
					res = append(res, tui.FormatKV("Status", "SUCCESS", core.GREEN+core.BOLD))
				} else {
					res = append(res, tui.FormatKV("Status", "FAILED", core.RED+core.BOLD))
				}
			}
			tui.PrintInBox("Firewall Reload Result", res)
		default:
			// User clicked on a numbered rule to delete
			if strings.HasPrefix(cleanOpt, "[") && strings.Contains(cleanOpt, "]") && strings.Contains(cleanOpt, "[DELETE]") {
				rIdx := strings.Index(cleanOpt, "]")
				if rIdx != -1 {
					ruleNumStr := strings.TrimSpace(cleanOpt[1:rIdx])
					if _, err := strconv.Atoi(ruleNumStr); err != nil {
						continue
					}
					tui.ClearScreen()
					confirm := tui.AskYesNo(fmt.Sprintf("Are you sure you want to DELETE rule #%s?", ruleNumStr))
					if confirm == 0 {
						tui.ClearScreen()
						var delRes []string
						if system.RunStep(fmt.Sprintf("Deleting rule #%s", ruleNumStr), "ufw", "--force", "delete", ruleNumStr) {
							delRes = append(delRes, tui.FormatKV("Deleted Rule", ruleNumStr, core.GREEN+core.BOLD))
							delRes = append(delRes, tui.FormatKV("Status", "SUCCESSFULLY REMOVED", core.GREEN))
						} else {
							delRes = append(delRes, tui.FormatKV("Status", "FAILED TO DELETE", core.RED))
						}
						tui.PrintInBox("Rule Deletion Result", delRes)
					}
				}
			}
		}
	}
}

// AddNewRuleWizard guides administrators through adding custom port, protocol, and subnet allow/deny firewall rules.
func AddNewRuleWizard() {
	action := "allow"
	proto := "tcp"
	port := ""
	source := "Anywhere"

	// Step 1: Select Action
	tui.ClearScreen()
	actOpts := []string{
		"ALLOW -> Permit Incoming Traffic (Standard)",
		"DENY  -> Block / Drop Incoming Traffic (Drop silently)",
		"LIMIT -> Rate-Limit Inbound Connections (Anti-Bruteforce / SSH protection)",
	}
	aIdx, _, aStat := tui.SelectMenu("Add Rule - Step 1/4: Choose Rule Action", actOpts)
	if aStat != 0 {
		return
	}
	switch aIdx {
	case 0:
		action = "allow"
	case 1:
		action = "deny"
	case 2:
		action = "limit"
	}

	// Step 2: Select Protocol
	tui.ClearScreen()
	protoOpts := []string{
		core.CYAN + "TCP" + core.NC + "  -> Transmission Control Protocol (Web, SSH, DB, Mail)",
		core.YELLOW + "UDP" + core.NC + "  -> User Datagram Protocol (DNS, WireGuard, VoIP, Streaming)",
		core.GREEN + "BOTH" + core.NC + " -> Both TCP and UDP Protocols",
	}
	pIdx, _, pStat := tui.SelectMenu("Add Rule - Step 2/4: Choose Network Protocol", protoOpts)
	if pStat != 0 {
		return
	}
	switch pIdx {
	case 0:
		proto = "tcp"
	case 1:
		proto = "udp"
	case 2:
		proto = "both"
	}

	// Step 3: Input Port or Range
	tui.ClearScreen()
	portInput, pInStat := tui.ReadTextInput("Enter Port, Range, or Service Name (e.g. 8080, 3000:3050, https)", "8080")
	if pInStat != 0 || strings.TrimSpace(portInput) == "" {
		return
	}
	port = strings.TrimSpace(portInput)
	if !regexp.MustCompile(`^[a-zA-Z0-9_:-]+$`).MatchString(port) {
		tui.PrintInBox("Invalid Port", []string{core.RED + "[ERROR] Port or service format invalid. Only numbers, ranges (3000:3050) or standard service names are allowed." + core.NC})
		return
	}

	// Step 4: Source Restriction
	tui.ClearScreen()
	clientIP := GetSSHClientIP()
	if clientIP == "" {
		clientIP = system.GetLocalIP()
	}
	srcOpts := []string{
		"Anywhere (0.0.0.0/0 -> Open to Entire Public Internet)",
		fmt.Sprintf("Current Admin IP Only (%s -> Restricted Safe Access)", clientIP),
		"Custom IP / Subnet CIDR (e.g. 192.168.1.0/24 or 89.12.34.56)",
	}
	sIdx, _, sStat := tui.SelectMenu("Add Rule - Step 4/4: Choose Allowed Source Address", srcOpts)
	if sStat != 0 {
		return
	}

	switch sIdx {
	case 0:
		source = "Anywhere"
	case 1:
		source = clientIP
	case 2:
		tui.ClearScreen()
		custIP, cStat := tui.ReadTextInput("Enter Custom Source IP or CIDR Subnet", "192.168.1.0/24")
		if cStat != 0 || strings.TrimSpace(custIP) == "" {
			return
		}
		custTrim := strings.TrimSpace(custIP)
		if net.ParseIP(custTrim) == nil {
			if _, _, err := net.ParseCIDR(custTrim); err != nil {
				tui.PrintInBox("Invalid Address", []string{core.RED + "[ERROR] Invalid IP or CIDR notation format." + core.NC})
				return
			}
		}
		source = custTrim
	}

	// Confirmation Summary
	tui.ClearScreen()
	summary := []string{
		tui.FormatKV("Rule Action", strings.ToUpper(action), func() string {
			if action == "allow" {
				return core.GREEN + core.BOLD
			} else if action == "limit" {
				return core.YELLOW + core.BOLD
			}
			return core.RED + core.BOLD
		}()),
		tui.FormatKV("Protocol", strings.ToUpper(proto), core.CYAN),
		tui.FormatKV("Target Port / Range", port, core.GREEN+core.BOLD),
		tui.FormatKV("Source Address", source, core.PURPLE),
		"",
		core.BOLD + core.YELLOW + "Do you want to apply this firewall rule now?" + core.NC,
	}

	cIdx, _, cStat := tui.SelectMenuWithContent("Confirm New Firewall Rule", summary, []string{"YES, APPLY RULE", "NO, CANCEL"})
	if cStat != 0 || cIdx != 0 {
		return
	}

	tui.ClearScreen()
	var execRes []string
	var success bool

	if system.CommandExists("ufw") {
		var ufwArgs []string
		if action == "limit" {
			ufwArgs = append(ufwArgs, "limit")
		} else if action == "deny" {
			ufwArgs = append(ufwArgs, "deny")
		} else {
			ufwArgs = append(ufwArgs, "allow")
		}

		if source != "Anywhere" {
			ufwArgs = append(ufwArgs, "from", source, "to", "any", "port", port)
			if proto != "both" {
				ufwArgs = append(ufwArgs, "proto", proto)
			}
		} else {
			if proto != "both" {
				ufwArgs = append(ufwArgs, port+"/"+proto)
			} else {
				ufwArgs = append(ufwArgs, port)
			}
		}
		success = system.RunStep("Applying UFW rule", "ufw", ufwArgs...)
	} else if system.CommandExists("firewall-cmd") {
		actStr := "accept"
		if action == "deny" {
			actStr = "drop"
		}
		protocols := []string{proto}
		if proto == "both" {
			protocols = []string{"tcp", "udp"}
		}

		if source != "Anywhere" || action == "deny" {
			family := "ipv4"
			if strings.Contains(source, ":") {
				family = "ipv6"
			}
			for _, p := range protocols {
				var rule string
				if source != "Anywhere" {
					rule = fmt.Sprintf("rule family=\"%s\" source address=\"%s\" port port=\"%s\" protocol=\"%s\" %s", family, source, port, p, actStr)
				} else {
					rule = fmt.Sprintf("rule family=\"%s\" port port=\"%s\" protocol=\"%s\" %s", family, port, p, actStr)
				}
				_ = exec.Command("firewall-cmd", "--permanent", "--add-rich-rule="+rule).Run()
			}
		} else {
			for _, p := range protocols {
				_ = exec.Command("firewall-cmd", "--permanent", "--add-port="+port+"/"+p).Run()
			}
		}
		success = exec.Command("firewall-cmd", "--reload").Run() == nil
	}

	if success {
		execRes = append(execRes, tui.FormatKV("Rule Applied", fmt.Sprintf("%s %s/%s from %s", strings.ToUpper(action), port, proto, source), core.GREEN+core.BOLD))
		execRes = append(execRes, tui.FormatKV("Status", "ACTIVE & OPERATIONAL", core.GREEN))
	} else {
		execRes = append(execRes, tui.FormatKV("Status", "FAILED TO APPLY RULE", core.RED+core.BOLD))
	}
	tui.PrintInBox("Rule Creation Result", execRes)
}

func QuickPresetsMenu() {
	for {
		tui.ClearScreen()
		presets := []string{
			"─── WEB & APPLICATION PRESETS ───",
			"Web Server Stack         ->  Open Ports 80 (HTTP) & 443 (HTTPS)",
			"SSH Safe Access          ->  Open Port 22 (TCP) with Rate-Limiting",
			"─── VPN & TUNNEL PRESETS ───",
			"WireGuard VPN Server     ->  Open Port 51820 (UDP)",
			"OpenVPN Server           ->  Open Port 1194 (UDP)",
			"─── CONTAINER & DEVOPS ───",
			"Docker Swarm & Portainer ->  Open Ports 9000, 9443, 2377 (TCP)",
			"Database IP Whitelist    ->  Open MySQL (3306) / Postgres (5432) to specific IP",
			"─── MEDIA & GAMING ───",
			"Media Server (Plex/Jelly)->  Open Ports 32400, 8096 (TCP)",
			"Game Server (Minecraft)  ->  Open Port 25565 (TCP/UDP)",
			"─── MAIL SERVER ───",
			"Mail Server Suite        ->  Open Ports 25, 465, 587, 993 (TCP)",
			"─── NAVIGATION ───",
			"Back to Previous Menu",
		}

		_, selectedOpt, status := tui.SelectMenu("FIREWALL & SECURITY MANAGER - Quick Presets", presets)
		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Back to Previous Menu") {
			return
		}

		cleanOpt := core.StripANSI(selectedOpt)
		tui.ClearScreen()
		var res []string

		switch {
		case strings.HasPrefix(cleanOpt, "Web Server Stack"):
			system.RunStep("Opening Port 80/tcp (HTTP)", "ufw", "allow", "80/tcp")
			system.RunStep("Opening Port 443/tcp (HTTPS)", "ufw", "allow", "443/tcp")
			res = append(res, tui.FormatKV("Preset", "Web Server Stack (80, 443)", core.GREEN+core.BOLD))
			res = append(res, tui.FormatKV("Status", "APPLIED", core.GREEN))
			tui.PrintInBox("Preset Applied", res)

		case strings.HasPrefix(cleanOpt, "SSH Safe Access"):
			system.RunStep("Rate-limiting SSH Port 22/tcp", "ufw", "limit", "22/tcp")
			res = append(res, tui.FormatKV("Preset", "SSH Safe Rate-Limited (22/tcp)", core.GREEN+core.BOLD))
			res = append(res, tui.FormatKV("Status", "APPLIED", core.GREEN))
			tui.PrintInBox("Preset Applied", res)

		case strings.HasPrefix(cleanOpt, "WireGuard VPN"):
			system.RunStep("Opening WireGuard Port 51820/udp", "ufw", "allow", "51820/udp")
			res = append(res, tui.FormatKV("Preset", "WireGuard VPN (51820/udp)", core.GREEN+core.BOLD))
			res = append(res, tui.FormatKV("Status", "APPLIED", core.GREEN))
			tui.PrintInBox("Preset Applied", res)

		case strings.HasPrefix(cleanOpt, "OpenVPN Server"):
			system.RunStep("Opening OpenVPN Port 1194/udp", "ufw", "allow", "1194/udp")
			res = append(res, tui.FormatKV("Preset", "OpenVPN (1194/udp)", core.GREEN+core.BOLD))
			res = append(res, tui.FormatKV("Status", "APPLIED", core.GREEN))
			tui.PrintInBox("Preset Applied", res)

		case strings.HasPrefix(cleanOpt, "Docker Swarm"):
			system.RunStep("Opening Port 9000/tcp", "ufw", "allow", "9000/tcp")
			system.RunStep("Opening Port 9443/tcp", "ufw", "allow", "9443/tcp")
			system.RunStep("Opening Port 2377/tcp", "ufw", "allow", "2377/tcp")
			res = append(res, tui.FormatKV("Preset", "Docker Swarm & Portainer", core.GREEN+core.BOLD))
			res = append(res, tui.FormatKV("Status", "APPLIED", core.GREEN))
			tui.PrintInBox("Preset Applied", res)

		case strings.HasPrefix(cleanOpt, "Database IP Whitelist"):
			tui.ClearScreen()
			dbIP, dStat := tui.ReadTextInput("Enter Trusted IP Address allowed to access Databases", GetSSHClientIP())
			if dStat == 0 && strings.TrimSpace(dbIP) != "" {
				system.RunStep("Allowing MySQL from "+dbIP, "ufw", "allow", "from", strings.TrimSpace(dbIP), "to", "any", "port", "3306", "proto", "tcp")
				system.RunStep("Allowing PostgreSQL from "+dbIP, "ufw", "allow", "from", strings.TrimSpace(dbIP), "to", "any", "port", "5432", "proto", "tcp")
				system.RunStep("Allowing Redis from "+dbIP, "ufw", "allow", "from", strings.TrimSpace(dbIP), "to", "any", "port", "6379", "proto", "tcp")
				res = append(res, tui.FormatKV("Database Whitelist", strings.TrimSpace(dbIP), core.GREEN+core.BOLD))
				res = append(res, tui.FormatKV("Allowed Ports", "3306, 5432, 6379", core.CYAN))
				res = append(res, tui.FormatKV("Status", "APPLIED", core.GREEN))
				tui.PrintInBox("Database Whitelist Result", res)
			}

		case strings.HasPrefix(cleanOpt, "Media Server"):
			system.RunStep("Opening Plex Port 32400/tcp", "ufw", "allow", "32400/tcp")
			system.RunStep("Opening Jellyfin Port 8096/tcp", "ufw", "allow", "8096/tcp")
			res = append(res, tui.FormatKV("Preset", "Media Server (Plex 32400, Jellyfin 8096)", core.GREEN+core.BOLD))
			res = append(res, tui.FormatKV("Status", "APPLIED", core.GREEN))
			tui.PrintInBox("Preset Applied", res)

		case strings.HasPrefix(cleanOpt, "Game Server"):
			system.RunStep("Opening Minecraft Port 25565", "ufw", "allow", "25565")
			res = append(res, tui.FormatKV("Preset", "Minecraft Game Server (25565)", core.GREEN+core.BOLD))
			res = append(res, tui.FormatKV("Status", "APPLIED", core.GREEN))
			tui.PrintInBox("Preset Applied", res)

		case strings.HasPrefix(cleanOpt, "Mail Server Suite"):
			system.RunStep("Opening SMTP Port 25/tcp", "ufw", "allow", "25/tcp")
			system.RunStep("Opening SMTPS Port 465/tcp", "ufw", "allow", "465/tcp")
			system.RunStep("Opening Submission Port 587/tcp", "ufw", "allow", "587/tcp")
			system.RunStep("Opening IMAPS Port 993/tcp", "ufw", "allow", "993/tcp")
			res = append(res, tui.FormatKV("Preset", "Mail Server Suite (25, 465, 587, 993)", core.GREEN+core.BOLD))
			res = append(res, tui.FormatKV("Status", "APPLIED", core.GREEN))
			tui.PrintInBox("Preset Applied", res)
		}
	}
}

// ==============================================================================
// FAIL2BAN & INTRUSION PREVENTION ENGINE
// ==============================================================================
