// ==============================================================================
// KROTEK SCRIPT HUB — VPN PEER & CLIENT PROFILE MANAGER
// Service: WIREGUARD (vpn)
// File:    vpn/wireguard/src/modules/peers.go
//
// Generates WireGuard/OpenVPN client configurations, cryptographic keypairs, preshared keys, and ASCII QR codes for mobile setup.
// ==============================================================================

package modules

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"krotek-wireguard-installer/core"
	"krotek-wireguard-installer/system"
	"krotek-wireguard-installer/tui"
)

func generateRandomBase64Key() string {
	b := make([]byte, 32)
	for i := 0; i < 5; i++ {
		if _, err := rand.Read(b); err == nil {
			return base64.StdEncoding.EncodeToString(b)
		}
	}
	panic("crypto/rand: failed to generate secure random key")
}

func getServerPublicKey() string {
	if pubData, err := os.ReadFile("/etc/wireguard/server_public.key"); err == nil {
		k := strings.TrimSpace(string(pubData))
		if k != "" {
			return k
		}
	}
	if privData, err := os.ReadFile("/etc/wireguard/server_private.key"); err == nil {
		priv := strings.TrimSpace(string(privData))
		pubCmd := exec.Command("wg", "pubkey")
		pubCmd.Stdin = strings.NewReader(priv)
		if pubOut, err := pubCmd.Output(); err == nil {
			k := strings.TrimSpace(string(pubOut))
			_ = system.WriteFileNoFollow("/etc/wireguard/server_public.key", []byte(k+"\n"), 0600, "")
			return k
		}
	}
	return ""
}

// SyncWireGuardConfig synchronizes the running WireGuard interface without using bash process substitution.
func SyncWireGuardConfig(iface string) error {
	cleanIface := strings.TrimSpace(iface)
	if cleanIface == "" {
		return fmt.Errorf("wireguard interface name cannot be empty")
	}
	for _, ch := range cleanIface {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			return fmt.Errorf("invalid wireguard interface name: %s", iface)
		}
	}
	if stripOut, err := exec.Command("wg-quick", "strip", cleanIface).Output(); err == nil {
		syncCmd := exec.Command("wg", "syncconf", cleanIface, "/dev/stdin")
		syncCmd.Stdin = bytes.NewReader(stripOut)
		if syncCmd.Run() == nil {
			return nil
		}
	}
	if err := exec.Command("systemctl", "reload", "wg-quick@"+cleanIface).Run(); err == nil {
		return nil
	}
	return fmt.Errorf("failed to reload wireguard interface %s", cleanIface)
}

func getNextAvailableIP() (int, error) {
	usedIPs := make(map[int]bool)
	usedIPs[1] = true // Server IP

	if data, err := os.ReadFile("/etc/wireguard/wg0.conf"); err == nil {
		re := regexp.MustCompile(`10\.8\.0\.(\d+)`)
		matches := re.FindAllStringSubmatch(string(data), -1)
		for _, m := range matches {
			if len(m) >= 2 {
				if n, err := strconv.Atoi(m[1]); err == nil {
					usedIPs[n] = true
				}
			}
		}
	}

	for i := 2; i <= 254; i++ {
		if !usedIPs[i] {
			return i, nil
		}
	}
	return 0, fmt.Errorf("no available IPs left in 10.8.0.0/24 subnet")
}

// AddNewClientPeerWizard guides through generating a cryptographic keypair, assigning an IP, and rendering a client QR code.
func AddNewClientPeerWizard() {
	tui.ClearScreen()

	serverPubKey := getServerPublicKey()
	if serverPubKey == "" {
		tui.PrintInBox("Error", []string{core.RED + "[ERROR] Server public key could not be determined." + core.NC, "Please make sure WireGuard native service is initialized."})
		return
	}

	tui.PrintInBox("Add New Client Peer", []string{
		"Create a new WireGuard client configuration profile.",
		"This will generate secure private/public keys, allocate a static tunnel IP,",
		"and generate an ASCII QR code for fast mobile connection.",
		"",
	})

	clientName, status := tui.ReadTextInput("Client Profile Name (e.g. phone, laptop, user1)", "phone")
	if status != 0 {
		return
	}

	clientName = strings.TrimSpace(clientName)
	clientName = regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(clientName, "")
	if clientName == "" {
		clientName = "client"
	}

	nextIPNum, err := getNextAvailableIP()
	if err != nil {
		tui.PrintInBox("Subnet Full", []string{core.RED + "[ERROR] " + err.Error() + core.NC})
		return
	}

	clientIP4 := fmt.Sprintf("10.8.0.%d", nextIPNum)
	clientIP6 := fmt.Sprintf("fd42:42:42::%d", nextIPNum)

	// Generate Client Keys
	var clientPrivKey string
	if privOut, err := exec.Command("wg", "genkey").Output(); err == nil && len(privOut) > 0 {
		clientPrivKey = strings.TrimSpace(string(privOut))
	} else {
		clientPrivKey = generateRandomBase64Key()
	}

	pubCmd := exec.Command("wg", "pubkey")
	pubCmd.Stdin = strings.NewReader(clientPrivKey)
	pubOut, _ := pubCmd.Output()
	clientPubKey := strings.TrimSpace(string(pubOut))

	var clientPsk string
	if pskOut, err := exec.Command("wg", "genpsk").Output(); err == nil && len(pskOut) > 0 {
		clientPsk = strings.TrimSpace(string(pskOut))
	} else {
		clientPsk = generateRandomBase64Key()
	}

	serverEndpoint := system.GetPublicIP()
	if serverEndpoint == "" || serverEndpoint == "N/A" {
		serverEndpoint = system.GetLocalIP()
	}

	// 1. Append to /etc/wireguard/wg0.conf
	peerBlock := fmt.Sprintf("\n### Client: %s ###\n[Peer]\nPublicKey = %s\nPresharedKey = %s\nAllowedIPs = %s/32, %s/128\n", clientName, clientPubKey, clientPsk, clientIP4, clientIP6)

	if err := system.AppendFileNoFollow("/etc/wireguard/wg0.conf", []byte(peerBlock), 0600); err != nil {
		tui.PrintInBox("Error", []string{core.RED + "[ERROR] Failed to write to /etc/wireguard/wg0.conf: " + err.Error() + core.NC})
		return
	}

	// 2. Create client configuration file
	_ = os.MkdirAll("/etc/wireguard/clients", 0700)
	clientConfPath := filepath.Join("/etc/wireguard/clients", clientName+".conf")

	clientConfig := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s/24, %s/64
DNS = 1.1.1.1, 8.8.8.8

[Peer]
PublicKey = %s
PresharedKey = %s
Endpoint = %s:51820
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
`, clientPrivKey, clientIP4, clientIP6, serverPubKey, clientPsk, serverEndpoint)

	_ = system.WriteFileNoFollow(clientConfPath, []byte(clientConfig), 0600, "")

	// 3. Sync running wg interface without shell process substitution
	_ = SyncWireGuardConfig("wg0")

	tui.ClearScreen()
	successReport := []string{
		core.GREEN + core.BOLD + core.TagOK() + " WireGuard Client Peer created successfully!" + core.NC,
		"",
		tui.FormatKV("Client Name", clientName, core.CYAN),
		tui.FormatKV("Client IPv4", clientIP4+"/24", core.GREEN),
		tui.FormatKV("Client IPv6", clientIP6+"/64", core.GREEN),
		tui.FormatKV("Server Endpoint", serverEndpoint+":51820", core.YELLOW),
		tui.FormatKV("Config File", clientConfPath, core.BOLD+core.CYAN),
		"",
		core.BOLD + core.YELLOW + "You can transfer this config or scan the QR Code below on mobile." + core.NC,
	}

	tui.PrintInBox("Peer Creation Success", successReport)

	// Display QR Code
	ShowClientQRCode(clientConfPath, clientName)
}

// ShowClientQRCode displays an ASCII terminal QR code or prints raw WireGuard configuration details.
func ShowClientQRCode(confPath string, clientName string) {
	if _, err := os.Stat(confPath); err != nil {
		tui.PrintInBox("Error", []string{core.RED + "[ERROR] Client configuration file not found: " + confPath + core.NC})
		return
	}

	tui.ClearScreen()
	fmt.Printf("\r\n%s=== WIREQUARD MOBILE QR CODE: %s ===%s\r\n\r\n", core.BOLD+core.CYAN, clientName, core.NC)

	if system.CommandExists("qrencode") {
		cmd := exec.Command("qrencode", "-t", "ansiutf8", "-r", confPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	} else {
		fmt.Printf("%s[NOTICE] 'qrencode' tool is not available to generate ASCII QR code.%s\r\n", core.YELLOW, core.NC)
		fmt.Printf("%sDisplaying styled client configuration instead:%s\r\n\r\n", core.DIM, core.NC)
		data, _ := os.ReadFile(confPath)
		for _, line := range tui.FormatKeyValueOutput(string(data), "Empty client configuration.") {
			fmt.Println(line)
		}
	}

	fmt.Printf("\r\n%sConfig File saved at:%s %s%s%s\r\n", core.BOLD, core.NC, core.CYAN, confPath, core.NC)
	fmt.Printf("%sPress Enter to return to menu...%s", core.BOLD+core.GREEN, core.NC)
	_, _ = fmt.Scanln()
}

// ListAndManagePeersMenu displays all configured WireGuard clients, their configuration paths, and QR code access.
func ListAndManagePeersMenu() {
	for {
		tui.ClearScreen()
		clientFiles, _ := filepath.Glob("/etc/wireguard/clients/*.conf")

		if len(clientFiles) == 0 {
			opts := []string{"Add New Client Peer", "Back"}
			idx, _, _ := tui.SelectMenu("No WireGuard clients configured yet.", opts)
			if idx == 0 {
				AddNewClientPeerWizard()
			} else {
				return
			}
			continue
		}

		var options []string
		options = append(options, fmt.Sprintf("─── CONFIGURED CLIENT PEERS (%d) ───", len(clientFiles)))
		for _, cf := range clientFiles {
			cName := strings.TrimSuffix(filepath.Base(cf), ".conf")
			options = append(options, fmt.Sprintf("View QR & Config: %s[%s]%s", core.CYAN, cName, core.NC))
		}
		options = append(options, "─── ACTIONS ───")
		options = append(options, "Add New Client Peer")
		options = append(options, "Revoke / Delete a Client Peer")
		options = append(options, "Back / Return")

		_, sel, status := tui.SelectMenu("WireGuard Peer Management Panel", options)
		cleanSel := core.StripANSI(sel)
		if status == 2 || (status == 0 && strings.Contains(cleanSel, "Back / Return")) {
			return
		}

		if strings.Contains(cleanSel, "Add New Client Peer") {
			AddNewClientPeerWizard()
			continue
		}

		if strings.Contains(cleanSel, "Revoke / Delete a Client Peer") {
			RevokePeerWizard()
			continue
		}

		for _, cf := range clientFiles {
			cName := strings.TrimSuffix(filepath.Base(cf), ".conf")
			if strings.Contains(cleanSel, "["+cName+"]") {
				ShowClientQRCode(cf, cName)
				break
			}
		}
	}
}

// RevokePeerWizard removes a client peer public key from the active wg0 interface and deletes its client config file.
func RevokePeerWizard() {
	clientFiles, _ := filepath.Glob("/etc/wireguard/clients/*.conf")
	if len(clientFiles) == 0 {
		tui.PrintInBox("No Clients", tui.FormatInfoNotice("INFO", "There are no configured clients to revoke.", core.YELLOW))
		return
	}

	var options []string
	options = append(options, "─── SELECT PEER TO REVOKE ───")
	for _, cf := range clientFiles {
		cName := strings.TrimSuffix(filepath.Base(cf), ".conf")
		options = append(options, fmt.Sprintf("%sRevoke:%s %s", core.RED, core.NC, cName))
	}
	options = append(options, "─── NAVIGATION ───")
	options = append(options, "Cancel")

	_, sel, status := tui.SelectMenu("Select Client Peer to Revoke & Dismantle", options)
	cleanSel := core.StripANSI(sel)
	if status == 2 || strings.Contains(cleanSel, "Cancel") {
		return
	}

	var chosenConf, chosenName string
	for _, cf := range clientFiles {
		cName := strings.TrimSuffix(filepath.Base(cf), ".conf")
		if strings.TrimPrefix(cleanSel, "Revoke: ") == cName {
			chosenConf = cf
			chosenName = cName
			break
		}
	}
	if chosenConf == "" {
		return
	}

	confirm := tui.AskYesNo(fmt.Sprintf("Are you sure you want to revoke peer '%s'?", chosenName))
	if confirm != 0 {
		return
	}

	// Remove from wg0.conf
	if confData, err := os.ReadFile("/etc/wireguard/wg0.conf"); err == nil {
		lines := strings.Split(string(confData), "\n")
		var newLines []string
		inTargetPeer := false

		for _, l := range lines {
			if strings.Contains(l, fmt.Sprintf("### Client: %s ###", chosenName)) {
				inTargetPeer = true
				continue
			}
			if inTargetPeer && strings.HasPrefix(strings.TrimSpace(l), "### Client:") {
				inTargetPeer = false
			}
			if !inTargetPeer {
				newLines = append(newLines, l)
			}
		}

		_ = system.WriteFileNoFollow("/etc/wireguard/wg0.conf", []byte(strings.Join(newLines, "\n")), 0600, "")
	}

	_ = os.Remove(chosenConf)
	_ = SyncWireGuardConfig("wg0")

	tui.PrintInBox("Peer Revoked", []string{core.GREEN + fmt.Sprintf("Peer '%s' has been successfully removed and revoked.", chosenName) + core.NC})
}

// FormatWireguardShowOutput formats raw `wg show` output into styled interface and peer cards.
func FormatWireguardShowOutput(raw string) []string {
	trimmedRaw := strings.TrimSpace(raw)
	if trimmedRaw == "" {
		return tui.FormatInfoNotice("INFO", "WireGuard interface wg0 is currently inactive or no peers connected.", core.YELLOW)
	}

	var formatted []string
	peerIdx := 0
	for _, line := range strings.Split(trimmedRaw, "\n") {
		clean := core.SanitizeTerminalText(strings.TrimRight(line, "\r"))
		t := strings.TrimSpace(clean)
		if t == "" {
			continue
		}

		if strings.HasPrefix(t, "interface:") {
			iface := strings.TrimSpace(strings.TrimPrefix(t, "interface:"))
			if len(formatted) > 0 {
				formatted = append(formatted, "")
			}
			formatted = append(formatted, fmt.Sprintf("%s%s%s INTERFACE:%s %s%s%s", core.BOLD, core.CYAN, core.Sym.Pointer, core.NC, core.BOLD+core.GREEN, iface, core.NC))
			formatted = append(formatted, "  "+core.DIM+strings.Repeat(core.Sym.BoxH, 68)+core.NC)
			continue
		}

		if strings.HasPrefix(t, "peer:") {
			peerIdx++
			peerKey := strings.TrimSpace(strings.TrimPrefix(t, "peer:"))
			if len(formatted) > 0 {
				formatted = append(formatted, "")
			}
			formatted = append(formatted, fmt.Sprintf("%s%s%s PEER #%d:%s %s%s%s", core.BOLD, core.YELLOW, core.Sym.Pointer, peerIdx, core.NC, core.BOLD+core.WHITE, peerKey, core.NC))
			formatted = append(formatted, "  "+core.DIM+strings.Repeat(core.Sym.BoxH, 68)+core.NC)
			continue
		}

		if parts := strings.SplitN(t, ":", 2); len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			valColor := core.WHITE
			switch strings.ToLower(k) {
			case "public key":
				valColor = core.CYAN
			case "private key", "preshared key":
				if v == "(hidden)" {
					valColor = core.DIM
				} else {
					valColor = core.YELLOW
				}
			case "listening port", "fwmark":
				valColor = core.YELLOW
			case "allowed ips", "endpoint":
				valColor = core.GREEN
			case "latest handshake":
				valColor = core.YELLOW
			case "transfer", "persistent keepalive":
				valColor = core.CYAN
			}
			formatted = append(formatted, tui.FormatKV(k, v, valColor))
			continue
		}

		formatted = append(formatted, "  "+t)
	}

	if len(formatted) == 0 {
		return tui.FormatInfoNotice("INFO", "WireGuard interface wg0 is currently inactive or no peers connected.", core.YELLOW)
	}
	return formatted
}

func ShowLiveWireguardTelemetry() {
	tui.ClearScreen()
	out, err := exec.Command("wg", "show").CombinedOutput()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		tui.PrintInBox("Live WireGuard Status & Peers (wg show)", FormatWireguardShowOutput(string(out)))
		return
	}

	sysOut, _ := exec.Command("systemctl", "status", "wg-quick@wg0").CombinedOutput()
	if len(strings.TrimSpace(string(sysOut))) > 0 {
		tui.PrintInBox("Live WireGuard Status & Peers (wg show)", tui.FormatSystemctlStatusOutput("wg-quick@wg0", string(sysOut)))
		return
	}

	tui.PrintInBox("Live WireGuard Status & Peers (wg show)", tui.FormatInfoNotice("INFO", "WireGuard interface wg0 is currently inactive or no peers connected.", core.YELLOW))
}
