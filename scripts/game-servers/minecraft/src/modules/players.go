// ==============================================================================
// KROTEK SCRIPT HUB — GAME SERVER PLAYER & SESSION MANAGER
// Service: MINECRAFT (game-servers)
// File:    game-servers/minecraft/src/modules/players.go
//
// Inspects active player connections, manages whitelist/banlist records, and broadcasts server notifications.
// ==============================================================================

package modules

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"krotek-minecraft-installer/core"
	"krotek-minecraft-installer/system"
	"krotek-minecraft-installer/tui"
)

type OpEntry struct {
	UUID                string `json:"uuid"`
	Name                string `json:"name"`
	Level               int    `json:"level"`
	BypassesPlayerLimit bool   `json:"bypassesPlayerLimit"`
}

type WhitelistEntry struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type BanEntry struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Created string `json:"created"`
	Source  string `json:"source"`
	Expires string `json:"expires"`
	Reason  string `json:"reason"`
}

type BanIPEntry struct {
	IP      string `json:"ip"`
	Created string `json:"created"`
	Source  string `json:"source"`
	Expires string `json:"expires"`
	Reason  string `json:"reason"`
}

func getOfflineUUID(username string) string {
	// #nosec G401,G501 - Minecraft protocol standard specifies RFC 4122 v3 MD5 UUID for offline player accounts
	h := md5.New()
	h.Write([]byte("OfflinePlayer:" + username))
	b := h.Sum(nil)
	b[6] = (b[6] & 0x0f) | 0x30 // version 3
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122
	s := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", s[0:8], s[8:12], s[12:16], s[16:20], s[20:32])
}

func getServerDataDir() string {
	return system.GetMinecraftDataDir()
}

// ManagePlayers provides operator promotion/demotion, whitelist access control, and player ban management.
func ManagePlayers() {
	serverDir := getServerDataDir()
	opsFile := filepath.Join(serverDir, "ops.json")
	whitelistFile := filepath.Join(serverDir, "whitelist.json")
	bannedPlayersFile := filepath.Join(serverDir, "banned-players.json")
	bannedIPsFile := filepath.Join(serverDir, "banned-ips.json")
	propFile := filepath.Join(serverDir, "server.properties")

	for {
		whitelistActive := false
		if data, err := os.ReadFile(propFile); err == nil {
			for _, l := range strings.Split(string(data), "\n") {
				l = strings.TrimSpace(l)
				if strings.HasPrefix(l, "white-list=") {
					val := strings.TrimPrefix(l, "white-list=")
					whitelistActive = (val == "true")
					break
				}
			}
		}

		wlBadge := core.RED + "DISABLED" + core.NC
		if whitelistActive {
			wlBadge = core.GREEN + "ACTIVE" + core.NC
		}

		options := []string{
			"─── SERVER OPERATORS (ADMINS) ───",
			"List Current Operators (ops.json)",
			"Add Player as Operator / Admin",
			"Remove Player from Operators",
			"─── WHITELIST ACCESS CONTROL ───",
			fmt.Sprintf("Toggle Whitelist Status (Current: %s)", wlBadge),
			"List Whitelisted Players",
			"Add Player to Whitelist",
			"Remove Player from Whitelist",
			"─── BANS & ACCESS RESTRICTIONS ───",
			"List Banned Players",
			"Ban Player Nickname",
			"Pardon / Unban Player",
			"List Banned IP Addresses",
			"Ban IP Address",
			"Pardon / Unban IP Address",
			"─── NAVIGATION ───",
			"Back to Main Menu",
		}

		idx, opt, st := tui.SelectMenu("KROTEK MINECRAFT" + " " + core.Sym.EmDash + " " + "Player Access, Ops & Security", options)
		if st == 2 || strings.Contains(opt, "Back to Main Menu") {
			return
		}

		cleanOpt := core.StripANSI(opt)
		switch {
		case strings.HasPrefix(cleanOpt, "List Current Operators"):
			listOperators(opsFile)
		case strings.HasPrefix(cleanOpt, "Add Player as Operator"):
			addOperator(opsFile)
		case strings.HasPrefix(cleanOpt, "Remove Player from Operators"):
			removeOperator(opsFile)
		case strings.HasPrefix(cleanOpt, "Toggle Whitelist Status"):
			newVal := "true"
			if whitelistActive {
				newVal = "false"
			}
			updateServerProp(propFile, "white-list", newVal)
			tui.PrintInBox("Whitelist Updated", []string{
				fmt.Sprintf("%s[OK] Whitelist status set to: %s%s", core.GREEN, strings.ToUpper(newVal), core.NC),
				"If server is currently running, restart or type 'whitelist reload' in console.",
			})
		case strings.HasPrefix(cleanOpt, "List Whitelisted"):
			listWhitelist(whitelistFile)
		case strings.HasPrefix(cleanOpt, "Add Player to Whitelist"):
			addToWhitelist(whitelistFile)
		case strings.HasPrefix(cleanOpt, "Remove Player from Whitelist"):
			removeFromWhitelist(whitelistFile)
		case strings.HasPrefix(cleanOpt, "List Banned Players"):
			listBannedPlayers(bannedPlayersFile)
		case strings.HasPrefix(cleanOpt, "Ban Player Nickname"):
			banPlayer(bannedPlayersFile)
		case strings.HasPrefix(cleanOpt, "Pardon / Unban Player"):
			unbanPlayer(bannedPlayersFile)
		case strings.HasPrefix(cleanOpt, "List Banned IP"):
			listBannedIPs(bannedIPsFile)
		case strings.HasPrefix(cleanOpt, "Ban IP Address"):
			banIP(bannedIPsFile)
		case strings.HasPrefix(cleanOpt, "Pardon / Unban IP"):
			unbanIP(bannedIPsFile)
		}
		_ = idx
	}
}

func loadJSONSafe(filePath string, target interface{}) bool {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return true
		}
		return false
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return true
	}
	if err := json.Unmarshal(data, target); err != nil {
		tui.PrintInBox("JSON Parse Error", []string{
			fmt.Sprintf("%s[ERROR] Failed to parse %s: malformed JSON!%s", core.RED, filepath.Base(filePath), core.NC),
			"Aborting operation to prevent accidental data loss.",
		})
		return false
	}
	return true
}

func listOperators(opsFile string) {
	tui.ClearScreen()
	var ops []OpEntry
	if !loadJSONSafe(opsFile, &ops) {
		return
	}

	var lines []string
	if len(ops) == 0 {
		lines = append(lines, core.YELLOW+"No operators configured yet in ops.json."+core.NC)
		lines = append(lines, "Use 'Add Player as Operator' to grant full admin permissions.")
	} else {
		lines = append(lines, core.BOLD+core.YELLOW+fmt.Sprintf("REGISTERED OPERATORS (%d):", len(ops))+core.NC, "")
		for i, op := range ops {
			lines = append(lines, fmt.Sprintf("  %s[%02d]%s %s%-20s%s (Level: %d" + " " + core.Sym.Bullet + " " + "UUID: %s)",
				core.CYAN, i+1, core.NC, core.BOLD+core.GREEN, op.Name, core.NC, op.Level, op.UUID))
		}
	}

	tui.PrintInBox("Minecraft Server Operators", lines)
}

func addOperator(opsFile string) {
	name, st := tui.ReadTextInput("Enter player nickname to grant Operator permissions", "")
	name = strings.TrimSpace(name)
	if st != 0 || name == "" {
		return
	}

	var ops []OpEntry
	if !loadJSONSafe(opsFile, &ops) {
		return
	}

	for _, op := range ops {
		if strings.EqualFold(op.Name, name) {
			tui.PrintInBox("Already Operator", []string{core.YELLOW + name + " is already an operator." + core.NC})
			return
		}
	}

	newOp := OpEntry{
		UUID:                getOfflineUUID(name),
		Name:                name,
		Level:               4,
		BypassesPlayerLimit: false,
	}
	ops = append(ops, newOp)

	data, _ := json.MarshalIndent(ops, "", "  ")
	_ = system.WriteFileNoFollow(opsFile, data, 0644, "")

	tui.PrintInBox("Operator Added", []string{
		fmt.Sprintf("%s[SUCCESS] Player '%s' has been granted Operator (Level 4) permissions!%s", core.GREEN, name, core.NC),
		fmt.Sprintf("UUID: %s", newOp.UUID),
		"Permissions take effect immediately upon next server join or console reload.",
	})
}

func removeOperator(opsFile string) {
	var ops []OpEntry
	if !loadJSONSafe(opsFile, &ops) {
		return
	}

	if len(ops) == 0 {
		tui.PrintInBox("No Operators", []string{"No operators found to remove."})
		return
	}

	var menuOpts []string
	for _, op := range ops {
		menuOpts = append(menuOpts, fmt.Sprintf("%s (Level %d)", op.Name, op.Level))
	}
	menuOpts = append(menuOpts, "Cancel")

	idx, _, st := tui.SelectMenu("Select Operator to Remove", menuOpts)
	if st != 0 || idx >= len(ops) {
		return
	}

	removed := ops[idx].Name
	ops = append(ops[:idx], ops[idx+1:]...)

	data, _ := json.MarshalIndent(ops, "", "  ")
	_ = system.WriteFileNoFollow(opsFile, data, 0644, "")

	tui.PrintInBox("Operator Removed", []string{
		fmt.Sprintf("%s[OK] Removed '%s' from operators list.%s", core.GREEN, removed, core.NC),
	})
}

func listWhitelist(wlFile string) {
	tui.ClearScreen()
	var list []WhitelistEntry
	if !loadJSONSafe(wlFile, &list) {
		return
	}

	var lines []string
	if len(list) == 0 {
		lines = append(lines, core.YELLOW+"No players on the whitelist."+core.NC)
		lines = append(lines, "Add players using 'Add Player to Whitelist' before enabling whitelist mode.")
	} else {
		lines = append(lines, core.BOLD+core.YELLOW+fmt.Sprintf("WHITELISTED PLAYERS (%d):", len(list))+core.NC, "")
		for i, p := range list {
			lines = append(lines, fmt.Sprintf("  %s[%02d]%s %s%-20s%s (UUID: %s)",
				core.CYAN, i+1, core.NC, core.BOLD+core.CYAN, p.Name, core.NC, p.UUID))
		}
	}

	tui.PrintInBox("Minecraft Whitelist", lines)
}

func addToWhitelist(wlFile string) {
	name, st := tui.ReadTextInput("Enter player nickname to add to Whitelist", "")
	name = strings.TrimSpace(name)
	if st != 0 || name == "" {
		return
	}

	var list []WhitelistEntry
	if !loadJSONSafe(wlFile, &list) {
		return
	}

	for _, p := range list {
		if strings.EqualFold(p.Name, name) {
			tui.PrintInBox("Already Whitelisted", []string{core.YELLOW + name + " is already whitelisted." + core.NC})
			return
		}
	}

	newEntry := WhitelistEntry{
		UUID: getOfflineUUID(name),
		Name: name,
	}
	list = append(list, newEntry)

	data, _ := json.MarshalIndent(list, "", "  ")
	_ = system.WriteFileNoFollow(wlFile, data, 0644, "")

	tui.PrintInBox("Whitelist Updated", []string{
		fmt.Sprintf("%s[SUCCESS] Player '%s' added to whitelist!%s", core.GREEN, name, core.NC),
	})
}

func removeFromWhitelist(wlFile string) {
	var list []WhitelistEntry
	if !loadJSONSafe(wlFile, &list) {
		return
	}

	if len(list) == 0 {
		tui.PrintInBox("Empty Whitelist", []string{"Whitelist is currently empty."})
		return
	}

	var menuOpts []string
	for _, p := range list {
		menuOpts = append(menuOpts, p.Name)
	}
	menuOpts = append(menuOpts, "Cancel")

	idx, _, st := tui.SelectMenu("Select Player to Remove from Whitelist", menuOpts)
	if st != 0 || idx >= len(list) {
		return
	}

	removed := list[idx].Name
	list = append(list[:idx], list[idx+1:]...)

	data, _ := json.MarshalIndent(list, "", "  ")
	_ = system.WriteFileNoFollow(wlFile, data, 0644, "")

	tui.PrintInBox("Player Removed", []string{
		fmt.Sprintf("%s[OK] Removed '%s' from whitelist.%s", core.GREEN, removed, core.NC),
	})
}

func listBannedPlayers(banFile string) {
	tui.ClearScreen()
	var bans []BanEntry
	if !loadJSONSafe(banFile, &bans) {
		return
	}

	var lines []string
	if len(bans) == 0 {
		lines = append(lines, core.GREEN+"No players currently banned."+core.NC)
	} else {
		lines = append(lines, core.BOLD+core.RED+fmt.Sprintf("BANNED PLAYERS (%d):", len(bans))+core.NC, "")
		for i, b := range bans {
			reason := b.Reason
			if reason == "" {
				reason = "Banned by an operator."
			}
			lines = append(lines, fmt.Sprintf("  %s[%02d]%s %s%-18s%s" + " " + core.Sym.Bullet + " " + "Reason: %s (by %s)",
				core.RED, i+1, core.NC, core.BOLD+core.YELLOW, b.Name, core.NC, reason, b.Source))
		}
	}

	tui.PrintInBox("Banned Players (banned-players.json)", lines)
}

func banPlayer(banFile string) {
	name, st := tui.ReadTextInput("Enter player nickname to ban", "")
	name = strings.TrimSpace(name)
	if st != 0 || name == "" {
		return
	}

	reason, rSt := tui.ReadTextInput("Enter ban reason", "Banned by administrator")
	if rSt != 0 || strings.TrimSpace(reason) == "" {
		reason = "Banned by administrator"
	}

	var bans []BanEntry
	if !loadJSONSafe(banFile, &bans) {
		return
	}

	for _, b := range bans {
		if strings.EqualFold(b.Name, name) {
			tui.PrintInBox("Already Banned", []string{core.YELLOW + name + " is already banned." + core.NC})
			return
		}
	}

	nowStr := time.Now().Format("2006-01-02 15:04:05 -0700")
	newBan := BanEntry{
		UUID:    getOfflineUUID(name),
		Name:    name,
		Created: nowStr,
		Source:  "Server Administrator",
		Expires: "forever",
		Reason:  reason,
	}
	bans = append(bans, newBan)

	data, _ := json.MarshalIndent(bans, "", "  ")
	_ = system.WriteFileNoFollow(banFile, data, 0644, "")

	tui.PrintInBox("Player Banned", []string{
		fmt.Sprintf("%s[BANNED] Player '%s' has been banned!%s", core.RED, name, core.NC),
		fmt.Sprintf("Reason: %s", reason),
	})
}

func unbanPlayer(banFile string) {
	var bans []BanEntry
	if !loadJSONSafe(banFile, &bans) {
		return
	}

	if len(bans) == 0 {
		tui.PrintInBox("No Bans", []string{"No players are currently banned."})
		return
	}

	var menuOpts []string
	for _, b := range bans {
		menuOpts = append(menuOpts, fmt.Sprintf("%s (Reason: %s)", b.Name, b.Reason))
	}
	menuOpts = append(menuOpts, "Cancel")

	idx, _, st := tui.SelectMenu("Select Player to Pardon / Unban", menuOpts)
	if st != 0 || idx >= len(bans) {
		return
	}

	pardoned := bans[idx].Name
	bans = append(bans[:idx], bans[idx+1:]...)

	data, _ := json.MarshalIndent(bans, "", "  ")
	_ = system.WriteFileNoFollow(banFile, data, 0644, "")

	tui.PrintInBox("Player Pardoned", []string{
		fmt.Sprintf("%s[PARDONED] '%s' has been unbanned.%s", core.GREEN, pardoned, core.NC),
	})
}

func listBannedIPs(ipFile string) {
	tui.ClearScreen()
	var bans []BanIPEntry
	if !loadJSONSafe(ipFile, &bans) {
		return
	}

	var lines []string
	if len(bans) == 0 {
		lines = append(lines, core.GREEN+"No IP addresses currently banned."+core.NC)
	} else {
		lines = append(lines, core.BOLD+core.RED+fmt.Sprintf("BANNED IP ADDRESSES (%d):", len(bans))+core.NC, "")
		for i, b := range bans {
			lines = append(lines, fmt.Sprintf("  %s[%02d]%s %s%-18s%s" + " " + core.Sym.Bullet + " " + "Reason: %s",
				core.RED, i+1, core.NC, core.BOLD+core.YELLOW, b.IP, core.NC, b.Reason))
		}
	}

	tui.PrintInBox("Banned IP Addresses (banned-ips.json)", lines)
}

func banIP(ipFile string) {
	ipStr, st := tui.ReadTextInput("Enter IP address to ban", "")
	ipStr = strings.TrimSpace(ipStr)
	if st != 0 || ipStr == "" {
		return
	}

	reason, rSt := tui.ReadTextInput("Enter ban reason", "Banned IP by administrator")
	if rSt != 0 || strings.TrimSpace(reason) == "" {
		reason = "Banned IP by administrator"
	}

	var bans []BanIPEntry
	if !loadJSONSafe(ipFile, &bans) {
		return
	}

	nowStr := time.Now().Format("2006-01-02 15:04:05 -0700")
	newBan := BanIPEntry{
		IP:      ipStr,
		Created: nowStr,
		Source:  "Server Administrator",
		Expires: "forever",
		Reason:  reason,
	}
	bans = append(bans, newBan)

	data, _ := json.MarshalIndent(bans, "", "  ")
	_ = system.WriteFileNoFollow(ipFile, data, 0644, "")

	tui.PrintInBox("IP Banned", []string{
		fmt.Sprintf("%s[BANNED] IP Address '%s' has been banned!%s", core.RED, ipStr, core.NC),
		fmt.Sprintf("Reason: %s", reason),
	})
}

func unbanIP(ipFile string) {
	var bans []BanIPEntry
	if !loadJSONSafe(ipFile, &bans) {
		return
	}

	if len(bans) == 0 {
		tui.PrintInBox("No Banned IPs", []string{"No IP addresses are currently banned."})
		return
	}

	var menuOpts []string
	for _, b := range bans {
		menuOpts = append(menuOpts, fmt.Sprintf("%s (%s)", b.IP, b.Reason))
	}
	menuOpts = append(menuOpts, "Cancel")

	idx, _, st := tui.SelectMenu("Select IP to Pardon / Unban", menuOpts)
	if st != 0 || idx >= len(bans) {
		return
	}

	pardoned := bans[idx].IP
	bans = append(bans[:idx], bans[idx+1:]...)

	data, _ := json.MarshalIndent(bans, "", "  ")
	_ = system.WriteFileNoFollow(ipFile, data, 0644, "")

	tui.PrintInBox("IP Pardoned", []string{
		fmt.Sprintf("%s[PARDONED] IP '%s' has been unbanned.%s", core.GREEN, pardoned, core.NC),
	})
}

func updateServerProp(filePath, key, val string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+"=") {
			lines[i] = key + "=" + val
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, key+"="+val)
	}
	_ = system.WriteFileNoFollow(filePath, []byte(strings.Join(lines, "\n")), 0644, "")
}
