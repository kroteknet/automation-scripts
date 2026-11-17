// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: OpenVPN Community Access Server (openvpn)
//
// This package orchestrates automated pre-flight checks, package repository setup,
// configuration templating, service enablement (systemd/openrc), and firewall rules.
// Supports both interactive TUI step-by-step guidance and headless automation.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"krotek-openvpn-installer/core"
	"krotek-openvpn-installer/logger"
	"krotek-openvpn-installer/system"
	"krotek-openvpn-installer/tui"
)

type OpenVPNConfig struct {
	Mode     string
	Proto    string
	Port     string
	Endpoint string
	Subnet   string
	DNS      string
}

// FreshInstallWizard handles the end-to-end installation pipeline for OpenVPN Community Access Server.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(mode string) {
	detectedIP := system.GetPublicIP()
	if detectedIP == "" || detectedIP == "N/A" {
		detectedIP = system.GetLocalIP()
	}

	cfg := OpenVPNConfig{
		Mode:     mode,
		Proto:    "udp",
		Port:     "1194",
		Endpoint: detectedIP,
		Subnet:   "10.8.0.0",
		DNS:      "1.1.1.1, 8.8.8.8",
	}

	PromptExistingConfigAction("OpenVPN Server", "/etc/openvpn")

	step := 1
	for {
		switch step {
		case 1:
			// Step 1: Protocol & Port
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "OpenVPN Protocol & Port Configuration" + core.NC,
				"",
				"  Select transmission protocol and listening port for the VPN.",
				"  UDP is strongly recommended for lower latency and better throughput.",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Exit Setup", "Step 1/4: Protocol & Port", cardLines)

			protoOpts := []string{
				"UDP (Fast, High Performance - Recommended)",
				"TCP (Reliable, Can bypass restrictive firewalls)",
				core.Sym.ArrowLeft + " Cancel Setup",
			}
			pIdx, _, pSt := tui.SelectMenu("Select OpenVPN Protocol", protoOpts)
			if pSt != 0 || pIdx == 2 {
				return
			}
			if pIdx == 0 {
				cfg.Proto = "udp"
			} else {
				cfg.Proto = "tcp"
			}

			portVal, st := tui.ReadTextInput("OpenVPN Listening Port", cfg.Port)
			if st != 0 {
				continue
			}
			portVal = strings.TrimSpace(portVal)
			if portVal == "" {
				portVal = "1194"
			}
			pNum, err := strconv.Atoi(portVal)
			if err != nil || pNum < 1 || pNum > 65535 {
				tui.PrintInBox("Invalid Port", []string{core.RED + "[ERROR] Port must be a numeric integer between 1 and 65535." + core.NC})
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
			epVal = regexp.MustCompile(`[^a-zA-Z0-9.-]`).ReplaceAllString(epVal, "")
			if epVal == "" {
				epVal = detectedIP
			}
			cfg.Endpoint = epVal
			step = 3

		case 3:
			// Step 3: Subnet & DNS
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Internal VPN Subnet & DNS Resolvers" + core.NC,
				"",
				"  Specify the private subnet network and upstream DNS for clients.",
				"  " + core.BOLD + "Default Subnet:" + core.NC + " 10.8.0.0 (Netmask: 255.255.255.0)",
				"  " + core.BOLD + "Default DNS:" + core.NC + " 1.1.1.1, 8.8.8.8",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: VPN Network & DNS", cardLines)

			subVal, st := tui.ReadTextInput("Internal Tunnel Subnet Network", cfg.Subnet)
			if st != 0 {
				step = 2
				continue
			}
			subVal = strings.TrimSpace(subVal)
			if subVal == "" {
				subVal = "10.8.0.0"
			}
			cfg.Subnet = subVal

			dnsVal, dSt := tui.ReadTextInput("Client DNS Resolvers", cfg.DNS)
			if dSt != 0 {
				step = 2
				continue
			}
			dnsVal = strings.TrimSpace(dnsVal)
			if dnsVal == "" {
				dnsVal = "1.1.1.1, 8.8.8.8"
			}
			cfg.DNS = dnsVal
			step = 4

		case 4:
			// Step 4: Review Summary Card
			tui.ClearScreen()
			summary := []string{
				fmt.Sprintf("%sDeployment Mode:%s     %s", core.CYAN, core.NC, strings.ToUpper(cfg.Mode)),
				fmt.Sprintf("%sProtocol & Port:%s     %s/%s", core.CYAN, core.NC, cfg.Port, strings.ToUpper(cfg.Proto)),
				fmt.Sprintf("%sServer Endpoint:%s     %s:%s", core.CYAN, core.NC, cfg.Endpoint, cfg.Port),
				fmt.Sprintf("%sTunnel Subnet:%s       %s 255.255.255.0", core.CYAN, core.NC, cfg.Subnet),
				fmt.Sprintf("%sClient DNS:%s          %s", core.CYAN, core.NC, cfg.DNS),
				"",
				"Ready to install and configure OpenVPN server.",
			}
			tui.PrintInBox("--no-wait", "Step 4/4: Configuration Summary", summary)

			cOpts := []string{
				"Confirm & Proceed with Installation",
				"<- Change Configuration (Back)",
				"Cancel Installation",
			}
			cIdx, _, cSt := tui.SelectMenu("Confirm OpenVPN Deployment", cOpts)
			if cSt != 0 || cIdx == 2 {
				return
			}
			if cIdx == 1 {
				step = 3
				continue
			}

			// Port conflict check
			if resolvedPort, ok := system.ResolvePortConflict(cfg.Port, cfg.Proto, false); !ok {
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

func configureFirewall(port string, proto string) {
	system.ConfigureFirewallRules(fmt.Sprintf("%s/%s", port, proto))
	if system.CommandExists("firewall-cmd") {
		_ = exec.Command("firewall-cmd", "--add-masquerade", "--permanent").Run()
		_ = exec.Command("firewall-cmd", "--reload").Run()
	}
	if system.CommandExists("iptables") {
		_ = exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", "10.8.0.0/24", "-j", "MASQUERADE").Run()
	}
}

func configureSysctl() {
	_ = os.MkdirAll("/etc/sysctl.d", 0755)
	sysctlContent := "net.ipv4.ip_forward = 1\nnet.ipv6.conf.all.forwarding = 1\n"
	_ = system.WriteFileNoFollow("/etc/sysctl.d/99-openvpn.conf", []byte(sysctlContent), 0644, "")
	_ = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
	_ = exec.Command("sysctl", "-p", "/etc/sysctl.d/99-openvpn.conf").Run()
}

func ExecuteDockerInstall(cfg OpenVPNConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing OpenVPN Docker Container Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker OpenVPN installation...")

	if !system.EnsureDockerEngine() {
		return
	}

	composeDir := "/etc/openvpn-docker"
	_ = os.MkdirAll(composeDir, 0755)
	_ = os.MkdirAll(filepath.Join(composeDir, "openvpn-data"), 0755)

	composeYaml := fmt.Sprintf(`version: '3.8'
services:
  openvpn:
    image: kylemanna/openvpn:latest
    container_name: krotek-openvpn
    restart: unless-stopped
    cap_add:
      - NET_ADMIN
    ports:
      - "%s:%s/%s"
    volumes:
      - %s/openvpn-data:/etc/openvpn
`, cfg.Port, cfg.Port, cfg.Proto, composeDir)

	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}

	configureSysctl()
	configureFirewall(cfg.Port, cfg.Proto)

	// Initialize OpenVPN configuration if not present
	dataDir := filepath.Join(composeDir, "openvpn-data")
	dockerArgs := []string{
		"run", "-v", fmt.Sprintf("%s:/etc/openvpn", dataDir),
		"--rm", "kylemanna/openvpn",
		"ovpn_genconfig", "-u", fmt.Sprintf("%s://%s:%s", cfg.Proto, cfg.Endpoint, cfg.Port),
	}
	_ = exec.Command("docker", dockerArgs...).Run()

	if !system.RunDockerCompose(composeDir, "Deploying OpenVPN Docker Stack", true, "up", "-d") {
		return
	}

	system.SaveState(map[string]string{
		"ENV_TYPE":    "docker",
		"COMPOSE_DIR": composeDir,
		"PORT":        cfg.Port,
		"PROTO":       cfg.Proto,
		"ENDPOINT":    cfg.Endpoint,
		"SUBNET":      cfg.Subnet,
		"DNS":         cfg.DNS,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("OpenVPN Docker deployment complete.")
	printOpenVPNInstructions(cfg)
}

func ExecuteNativeInstall(cfg OpenVPNConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing OpenVPN Native Service ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native OpenVPN installation...")

	var pkgs []string
	switch core.SysInfo.Family {
	case "alpine":
		pkgs = []string{"openvpn", "openvpn-openrc", "easy-rsa", "iptables", "iptables-openrc"}
	case "debian":
		pkgs = []string{"openvpn", "easy-rsa", "iptables"}
	case "rhel":
		pkgs = []string{"epel-release", "openvpn", "easy-rsa", "iptables"}
	default:
		pkgs = []string{"openvpn", "easy-rsa", "iptables"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogError("Failed to install OpenVPN packages.")
		return
	}

	_ = os.MkdirAll("/etc/openvpn/server", 0750)
	configureSysctl()
	configureFirewall(cfg.Port, cfg.Proto)

	// Automated PKI and Certificate Generation using OpenSSL & OpenVPN
	pkiCmd := `
		cd /etc/openvpn/server
		if [ ! -f ca.crt ] || [ ! -f server.crt ]; then
			openssl req -new -x509 -days 3650 -nodes -newkey rsa:2048 -out ca.crt -keyout ca.key -subj "/CN=Krotek-OpenVPN-CA" 2>/dev/null
			openssl req -new -nodes -newkey rsa:2048 -out server.csr -keyout server.key -subj "/CN=server" 2>/dev/null
			openssl x509 -req -days 3650 -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt 2>/dev/null
			openssl dhparam -out dh.pem 2048 2>/dev/null || true
			openvpn --genkey secret tc.key 2>/dev/null || openvpn --genkey --secret tc.key 2>/dev/null || true
			chmod 600 ca.key server.key tc.key 2>/dev/null || true
		fi
		# Generate client certificate
		if [ ! -f client1.crt ]; then
			openssl req -new -nodes -newkey rsa:2048 -out client1.csr -keyout client1.key -subj "/CN=client1" 2>/dev/null
			openssl x509 -req -days 3650 -in client1.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out client1.crt 2>/dev/null
			chmod 600 client1.key client1.crt 2>/dev/null || true
		fi
	`
	_ = system.RunMultiStep("Generating OpenVPN PKI, server certificates & client profile", pkiCmd)

	// Generate initial client configuration (client1.ovpn) safely in Go
	if _, err := os.Stat("/root/client1.ovpn"); os.IsNotExist(err) {
		caData, _ := os.ReadFile("/etc/openvpn/server/ca.crt")
		certData, _ := os.ReadFile("/etc/openvpn/server/client1.crt")
		keyData, _ := os.ReadFile("/etc/openvpn/server/client1.key")
		tcData, _ := os.ReadFile("/etc/openvpn/server/tc.key")

		clientOvpn := fmt.Sprintf("client\ndev tun\nproto %s\nremote %s %s\nresolv-retry infinite\nnobind\npersist-key\npersist-tun\nremote-cert-tls server\ncipher AES-256-GCM\nauth SHA256\nverb 3\n<ca>\n%s\n</ca>\n<cert>\n%s\n</cert>\n<key>\n%s\n</key>\n",
			cfg.Proto, cfg.Endpoint, cfg.Port, strings.TrimSpace(string(caData)), strings.TrimSpace(string(certData)), strings.TrimSpace(string(keyData)))
		if len(tcData) > 0 {
			clientOvpn += fmt.Sprintf("<tls-crypt>\n%s\n</tls-crypt>\n", strings.TrimSpace(string(tcData)))
		}
		_ = system.WriteFileNoFollow("/root/client1.ovpn", []byte(clientOvpn), 0600, "")
	}

	vpnGroup := "nogroup"
	if core.SysInfo.Family == "rhel" {
		vpnGroup = "nobody"
	}

	serverConf := fmt.Sprintf(`port %s
proto %s
dev tun
topology subnet
server %s 255.255.255.0
push "redirect-gateway def1 bypass-dhcp"
push "dhcp-option DNS 1.1.1.1"
push "dhcp-option DNS 8.8.8.8"
ca /etc/openvpn/server/ca.crt
cert /etc/openvpn/server/server.crt
key /etc/openvpn/server/server.key
dh /etc/openvpn/server/dh.pem
tls-crypt /etc/openvpn/server/tc.key
keepalive 10 120
cipher AES-256-GCM
auth SHA256
user nobody
group %s
persist-key
persist-tun
status /var/log/openvpn-status.log
verb 3
explicit-exit-notify 1
`, cfg.Port, cfg.Proto, cfg.Subnet, vpnGroup)

	_ = system.WriteFileNoFollow("/etc/openvpn/server/server.conf", []byte(serverConf), 0600, "")
	_ = system.WriteFileNoFollow("/etc/openvpn/server.conf", []byte(serverConf), 0600, "")

	if core.SysInfo.Family == "alpine" {
		system.StartService("openvpn")
	} else {
		_ = exec.Command("systemctl", "daemon-reload").Run()
		if !system.StartService("openvpn-server@server") {
			_ = system.StartService("openvpn@server")
		}
		_ = exec.Command("systemctl", "restart", "openvpn-server@server").Run()
		_ = exec.Command("systemctl", "restart", "openvpn@server").Run()
	}

	system.SaveState(map[string]string{
		"ENV_TYPE": "native",
		"PORT":     cfg.Port,
		"PROTO":    cfg.Proto,
		"ENDPOINT": cfg.Endpoint,
		"SUBNET":   cfg.Subnet,
		"DNS":      cfg.DNS,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("OpenVPN Native setup finished successfully.")
	printOpenVPNInstructions(cfg)
}

func printOpenVPNInstructions(cfg OpenVPNConfig) {
	fmt.Printf("\r\n%s------------------------------------------------------------%s\r\n", core.GREEN, core.NC)
	fmt.Printf("%s[OK] OpenVPN Server Deployed!%s\r\n", core.BOLD+core.GREEN, core.NC)
	fmt.Printf("  %sEndpoint:%s %s:%s/%s\r\n", core.CYAN, core.NC, cfg.Endpoint, cfg.Port, strings.ToUpper(cfg.Proto))
	fmt.Printf("  %sSubnet:%s   %s/24\r\n", core.CYAN, core.NC, cfg.Subnet)
	fmt.Printf("%s------------------------------------------------------------%s\r\n\r\n", core.GREEN, core.NC)
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
