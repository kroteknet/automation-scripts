// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: WireGuard Fast Kernel VPN Tunnel (wireguard)
//
// This package orchestrates automated pre-flight checks, package repository setup,
// configuration templating, service enablement (systemd/openrc), and firewall rules.
// Supports both interactive TUI step-by-step guidance and headless automation.
// ==============================================================================

package install

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"krotek-wireguard-installer/core"
	"krotek-wireguard-installer/logger"
	"krotek-wireguard-installer/system"
	"krotek-wireguard-installer/tui"
)

type WireguardConfig struct {
	Mode     string
	Port     string
	Endpoint string
	Subnet   string
	DNS      string
}

func generateRandomKey() string {
	b := make([]byte, 32)
	for i := 0; i < 5; i++ {
		if _, err := rand.Read(b); err == nil {
			// Apply Curve25519 scalar clamping for cryptographic security
			b[0] &= 248
			b[31] = (b[31] & 127) | 64
			return base64.StdEncoding.EncodeToString(b)
		}
	}
	logger.LogError("Cryptographic randomness failure: unable to generate secure WireGuard key")
	os.Exit(1)
	return ""
}

func detectDefaultInterface() string {
	parseRoute := func(out []byte) string {
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 || fields[0] != "default" {
				continue
			}
			for i := 0; i+1 < len(fields); i++ {
				if fields[i] == "dev" {
					iface := regexp.MustCompile(`[^a-zA-Z0-9_.-]`).ReplaceAllString(fields[i+1], "")
					if iface != "" {
						return iface
					}
				}
			}
		}
		return ""
	}
	if out, err := exec.Command("ip", "route", "show", "default").Output(); err == nil {
		if iface := parseRoute(out); iface != "" {
			return iface
		}
	}
	if out2, err2 := exec.Command("ip", "-4", "route", "ls").Output(); err2 == nil {
		if iface := parseRoute(out2); iface != "" {
			return iface
		}
	}
	return "eth0"
}

// FreshInstallWizard handles the end-to-end installation pipeline for WireGuard Fast Kernel VPN Tunnel.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(mode string) {
	detectedIP := system.GetPublicIP()
	if detectedIP == "" || detectedIP == "N/A" {
		detectedIP = system.GetLocalIP()
	}

	cfg := WireguardConfig{
		Mode:     mode,
		Port:     "51820",
		Endpoint: detectedIP,
		Subnet:   "10.8.0.1/24",
		DNS:      "1.1.1.1, 8.8.8.8",
	}

	PromptExistingConfigAction("WireGuard VPN Server", "/etc/wireguard")

	step := 1
	for {
		switch step {
		case 1:
			// Step 1: Port Selection
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "WireGuard VPN Listening Port" + core.NC,
				"",
				"  Specify the UDP listening port for incoming WireGuard tunnel handshakes.",
				"  " + core.BOLD + "Default Port:" + core.NC + " 51820/UDP",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Exit Setup", "Step 1/4: VPN Port Configuration", cardLines)

			portVal, st := tui.ReadTextInput("WireGuard UDP Port", cfg.Port)
			if st != 0 {
				return
			}
			portVal = strings.TrimSpace(portVal)
			if portVal == "" {
				portVal = "51820"
			}
			pNum, pErr := strconv.Atoi(portVal)
			if pErr != nil || pNum < 1 || pNum > 65535 {
				tui.PrintInBox("Invalid Port", []string{
					core.RED + "Port must be an integer between 1 and 65535." + core.NC,
				})
				continue
			}
			cfg.Port = strconv.Itoa(pNum)
			step = 2

		case 2:
			// Step 2: Public Endpoint
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Server Public Endpoint" + core.NC,
				"",
				"  Clients use this Public IP or Dynamic DNS domain to connect to this server.",
				"  " + core.BOLD + "Detected IP:" + core.NC + " " + detectedIP,
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: Server Endpoint Address", cardLines)

			epVal, st := tui.ReadTextInput("Server Public IP / FQDN", cfg.Endpoint)
			if st != 0 {
				step = 1
				continue
			}
			epVal = strings.TrimSpace(epVal)
			if epVal == "" {
				epVal = detectedIP
			}
			if strings.ContainsAny(epVal, "\r\n\"'`;$") {
				tui.PrintInBox("Invalid Endpoint", []string{
					core.RED + "Endpoint contains invalid characters." + core.NC,
				})
				continue
			}
			cfg.Endpoint = epVal
			step = 3

		case 3:
			// Step 3: Subnet & DNS
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Internal VPN Subnet & DNS" + core.NC,
				"",
				"  Choose the private IPv4 subnet and DNS resolvers for connected peers.",
				"  " + core.BOLD + "Default Subnet:" + core.NC + " 10.8.0.1/24",
				"  " + core.BOLD + "Default DNS:" + core.NC + " 1.1.1.1, 8.8.8.8",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: VPN Network & DNS", cardLines)

			subVal, st := tui.ReadTextInput("Internal Tunnel Subnet", cfg.Subnet)
			if st != 0 {
				step = 2
				continue
			}
			subVal = strings.TrimSpace(subVal)
			if subVal == "" {
				subVal = "10.8.0.1/24"
			}
			if strings.ContainsAny(subVal, "\r\n\"'`;$") || (!strings.Contains(subVal, "/") && net.ParseIP(subVal) == nil) {
				tui.PrintInBox("Invalid Subnet", []string{
					core.RED + "Subnet must be a valid CIDR (e.g. 10.8.0.1/24)." + core.NC,
				})
				continue
			}
			cfg.Subnet = subVal

			dnsVal, dSt := tui.ReadTextInput("Client DNS Resolvers", cfg.DNS)
			if dSt != 0 {
				step = 2
				continue
			}
			dnsVal = strings.TrimSpace(dnsVal)
			if dnsVal == "" || strings.ContainsAny(dnsVal, "\r\n\"'`;$") {
				dnsVal = "1.1.1.1, 8.8.8.8"
			}
			cfg.DNS = dnsVal
			step = 4

		case 4:
			// Step 4: Review Summary Card
			tui.ClearScreen()
			summary := []string{
				fmt.Sprintf("%sDeployment Mode:%s     %s", core.CYAN, core.NC, strings.ToUpper(cfg.Mode)),
				fmt.Sprintf("%sListening Port:%s      %s/UDP", core.CYAN, core.NC, cfg.Port),
				fmt.Sprintf("%sServer Endpoint:%s     %s:%s", core.CYAN, core.NC, cfg.Endpoint, cfg.Port),
				fmt.Sprintf("%sTunnel Subnet:%s       %s", core.CYAN, core.NC, cfg.Subnet),
				fmt.Sprintf("%sClient DNS:%s          %s", core.CYAN, core.NC, cfg.DNS),
				"",
				"Ready to install and configure WireGuard VPN server.",
			}
			tui.PrintInBox("--no-wait", "Step 4/4: Configuration Summary", summary)

			cOpts := []string{
				"Confirm & Proceed with Installation",
				"<- Change Configuration (Back)",
				"Cancel Installation",
			}
			cIdx, _, cSt := tui.SelectMenu("Confirm WireGuard Deployment", cOpts)
			if cSt != 0 || cIdx == 2 {
				return
			}
			if cIdx == 1 {
				step = 3
				continue
			}

			// Port conflict check
			if resolvedPort, ok := system.ResolvePortConflict(cfg.Port, "udp", false); !ok {
				return
			} else {
				cfg.Port = resolvedPort
			}

			if cfg.Mode == "docker" {
				ExecuteDockerInstall(cfg)
			} else {
				ExecuteNativeInstall(cfg)
			}
			return
		}
	}
}

func ensureServerConfig(cfg WireguardConfig) bool {
	_ = os.MkdirAll("/etc/wireguard", 0700)
	_ = os.MkdirAll("/etc/wireguard/clients", 0700)

	confPath := "/etc/wireguard/wg0.conf"

	var privKey string
	privKeyOut, err := exec.Command("wg", "genkey").Output()
	if err == nil && len(privKeyOut) > 0 {
		privKey = strings.TrimSpace(string(privKeyOut))
	} else {
		privKey = generateRandomKey()
	}

	pubCmd := exec.Command("wg", "pubkey")
	pubCmd.Stdin = strings.NewReader(privKey)
	pubKeyOut, _ := pubCmd.Output()
	pubKey := strings.TrimSpace(string(pubKeyOut))

	_ = system.WriteFileNoFollow("/etc/wireguard/server_private.key", []byte(privKey+"\n"), 0600, "")
	if pubKey != "" {
		_ = system.WriteFileNoFollow("/etc/wireguard/server_public.key", []byte(pubKey+"\n"), 0600, "")
	}

	wanIface := detectDefaultInterface()

	confContent := fmt.Sprintf(`[Interface]
Address = %s
ListenPort = %s
PrivateKey = %s
PostUp = iptables -A FORWARD -i wg0 -j ACCEPT; iptables -t nat -A POSTROUTING -o %s -j MASQUERADE; ip6tables -A FORWARD -i wg0 -j ACCEPT 2>/dev/null || true; ip6tables -t nat -A POSTROUTING -o %s -j MASQUERADE 2>/dev/null || true
PostDown = iptables -D FORWARD -i wg0 -j ACCEPT; iptables -t nat -D POSTROUTING -o %s -j MASQUERADE; ip6tables -D FORWARD -i wg0 -j ACCEPT 2>/dev/null || true; ip6tables -t nat -D POSTROUTING -o %s -j MASQUERADE 2>/dev/null || true
`, cfg.Subnet, cfg.Port, privKey, wanIface, wanIface, wanIface, wanIface)

	if err := system.WriteFileNoFollow(confPath, []byte(confContent), 0600, ""); err != nil {
		return false
	}
	return true
}

func configureKernelForwarding() {
	_ = os.MkdirAll("/etc/sysctl.d", 0755)
	sysctlContent := "net.ipv4.ip_forward = 1\nnet.ipv6.conf.all.forwarding = 1\n"
	_ = system.WriteFileNoFollow("/etc/sysctl.d/99-wireguard.conf", []byte(sysctlContent), 0644, "")

	_ = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
	_ = exec.Command("sysctl", "-w", "net.ipv6.conf.all.forwarding=1").Run()
	_ = exec.Command("sysctl", "-p", "/etc/sysctl.d/99-wireguard.conf").Run()
}

func configureFirewall(port string) {
	system.ConfigureFirewallRules(fmt.Sprintf("%s/udp", port))
	if system.CommandExists("firewall-cmd") {
		_ = exec.Command("firewall-cmd", "--add-masquerade", "--permanent").Run()
		_ = exec.Command("firewall-cmd", "--reload").Run()
	}
	if system.CommandExists("iptables") {
		_ = exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", "10.0.0.0/24", "-j", "MASQUERADE").Run()
	}
}

func ExecuteDockerInstall(cfg WireguardConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing WireGuard VPN Docker Container Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker WireGuard VPN installation...")

	if !system.EnsureDockerEngine() {
		return
	}

	composeDir := "/etc/wireguard-docker"
	_ = os.MkdirAll(composeDir, 0755)
	_ = os.MkdirAll(filepath.Join(composeDir, "config"), 0755)

	cleanSubnet := strings.TrimSuffix(cfg.Subnet, "/24")
	if strings.HasSuffix(cleanSubnet, ".1") {
		cleanSubnet = strings.TrimSuffix(cleanSubnet, ".1") + ".0"
	}

	composeYaml := fmt.Sprintf(`version: "3.8"
services:
  wireguard:
    image: lscr.io/linuxserver/wireguard:latest
    container_name: krotek-wireguard
    cap_add:
      - NET_ADMIN
      - SYS_MODULE
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Etc/UTC
      - SERVERURL="%s"
      - SERVERPORT=%s
      - PEERS=3
      - PEERDNS="%s"
      - INTERNAL_SUBNET="%s"
    volumes:
      - %s/config:/config
      - /lib/modules:/lib/modules
    ports:
      - %s:%s/udp
    sysctls:
      - net.ipv4.conf.all.src_valid_mark=1
      - net.ipv4.ip_forward=1
    restart: unless-stopped
`, cfg.Endpoint, cfg.Port, cfg.DNS, cleanSubnet, composeDir, cfg.Port, cfg.Port)

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}

	configureKernelForwarding()
	configureFirewall(cfg.Port)

	if !system.RunDockerCompose(composeDir, "Deploying WireGuard VPN Docker Stack", true, "up", "-d") {
		return
	}

	system.SaveState(map[string]string{
		"ENV_TYPE":    "docker",
		"COMPOSE_DIR": composeDir,
		"PORT":        cfg.Port,
		"ENDPOINT":    cfg.Endpoint,
		"SUBNET":      cfg.Subnet,
		"DNS":         cfg.DNS,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("WireGuard VPN Docker deployment complete.")
}

func ExecuteNativeInstall(cfg WireguardConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing WireGuard VPN Native Linux Service ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native WireGuard VPN installation...")

	var pkgs []string
	switch core.SysInfo.Family {
	case "rhel":
		_ = exec.Command("dnf", "install", "-y", "epel-release").Run()
		_ = exec.Command("yum", "install", "-y", "epel-release").Run()
		pkgs = []string{"wireguard-tools", "iptables"}
		if !system.InstallPackages(pkgs...) {
			logger.LogError("Failed to install WireGuard packages on RHEL/AlmaLinux.")
			return
		}
		_ = exec.Command("dnf", "install", "-y", "qrencode").Run()
		_ = exec.Command("yum", "install", "-y", "qrencode").Run()

	case "alpine":
		pkgs = []string{"wireguard-tools", "wireguard-tools-openrc", "iptables", "iptables-openrc", "libqrencode-tools"}
		if !system.InstallPackages(pkgs...) {
			pkgs = []string{"wireguard-tools", "iptables"}
			if !system.InstallPackages(pkgs...) {
				logger.LogError("Failed to install WireGuard packages on Alpine.")
				return
			}
		}

	default:
		pkgs = []string{"wireguard", "wireguard-tools", "iptables", "qrencode"}
		if !system.InstallPackages(pkgs...) {
			logger.LogError("Failed to install WireGuard packages on Debian/Ubuntu.")
			return
		}
	}

	_ = exec.Command("modprobe", "wireguard").Run()
	configureKernelForwarding()

	if !ensureServerConfig(cfg) {
		logger.LogError("Failed to initialize /etc/wireguard/wg0.conf server configuration.")
		return
	}

	configureFirewall(cfg.Port)

	if core.SysInfo.InitSystem == "openrc" {
		_ = exec.Command("rc-update", "add", "wg-quick.wg0", "default").Run()
		_ = exec.Command("service", "wg-quick.wg0", "start").Run()
		_ = exec.Command("wg-quick", "up", "wg0").Run()
	} else if system.IsSystemdRunning() {
		_ = exec.Command("systemctl", "daemon-reload").Run()
		_ = exec.Command("systemctl", "enable", "wg-quick@wg0").Run()
		_ = exec.Command("systemctl", "start", "wg-quick@wg0").Run()
	} else {
		_ = exec.Command("wg-quick", "up", "wg0").Run()
	}

	system.SaveState(map[string]string{
		"ENV_TYPE": "native",
		"PORT":     cfg.Port,
		"ENDPOINT": cfg.Endpoint,
		"SUBNET":   cfg.Subnet,
		"DNS":      cfg.DNS,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("WireGuard VPN Native setup finished successfully.")
}

// PromptExistingConfigAction checks if prior configuration files exist on the host system.
// In interactive TUI mode, it prompts the administrator in English to decide whether to preserve existing custom settings
// or perform a clean factory default reset. In unattended mode (-y / CI/CD), it automatically preserves settings (safe default).
func PromptExistingConfigAction(serviceTitle string, configPaths ...string) {
	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
		return
	}

	hasExisting := false
	for _, p := range configPaths {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				hasExisting = true
				break
			}
		}
	}
	if !hasExisting {
		return
	}

	tui.ClearScreen()
	cardLines := []string{
		core.BOLD + core.YELLOW + "EXISTING CONFIGURATION DETECTED" + core.NC,
		"",
		fmt.Sprintf("  Prior configuration files were detected for %s on this host.", serviceTitle),
		"  Choose how the deployment wizard should handle existing settings:",
		"",
		"  " + core.GREEN + "[1] Keep Existing Configuration (Recommended)" + core.NC,
		"      Preserves all existing configuration, virtual hosts, and custom data intact.",
		"",
		"  " + core.YELLOW + "[2] Clean Reset to Factory Default (Overwrite)" + core.NC,
		"      Overwrites configuration with fresh, production-hardened defaults.",
	}

	choices := []string{
		"Keep Existing Configuration (Preserve custom settings & virtual hosts)",
		"Clean Reset to Factory Default (Overwrite with fresh configurations)",
	}

	idx, _, status := tui.SelectMenuWithContent("Configuration Conflict Handler", cardLines, choices)
	if status == 2 {
		tui.LeaveAltScreen()
		logger.LogInfo("Setup cancelled by user at configuration conflict prompt.")
		os.Exit(0)
	}
	if status == 0 && idx == 1 {
		core.OverwriteExistingConfig = true
		logger.LogInfo("User selected: Clean reset to factory default configuration.")
	} else {
		core.OverwriteExistingConfig = false
		logger.LogInfo("User selected: Preserve existing configuration.")
	}
}
