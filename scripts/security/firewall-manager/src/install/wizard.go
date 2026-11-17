// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Universal Linux Firewall Management Engine (firewall-manager)
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

	"krotek-firewall-manager-installer/core"
	"krotek-firewall-manager-installer/logger"
	"krotek-firewall-manager-installer/system"
	"krotek-firewall-manager-installer/tui"
)

func ExecuteDockerInstall() {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing firewall-manager Docker Container Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker firewall-manager installation...")

	if !system.EnsureDockerEngine() {
		return
	}

	composeDir := "/etc/firewall-manager-docker"
	_ = os.MkdirAll(composeDir, 0755)

	composeYaml := fmt.Sprintf("version: '3.8'\nservices:\n  firewall-manager:\n    image: alpine:latest\n    container_name: krotek-firewall-manager\n    restart: unless-stopped\n    command: tail -f /dev/null\n    cap_add:\n      - NET_ADMIN\n")
	if err := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write docker-compose.yml: %v", err))
		return
	}

	if !system.RunDockerCompose(composeDir, "Deploying firewall-manager Docker Stack", true, "up", "-d") {
		return
	}

	system.SaveState(map[string]string{
		"ENV_TYPE":    "docker",
		"COMPOSE_DIR": composeDir,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("firewall-manager Docker deployment complete.")
}

func ExecuteNativeInstall() {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing firewall-manager Native Service ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native firewall-manager installation...")

	var pkgs []string
	switch core.SysInfo.Family {
	case "debian":
		pkgs = []string{"ufw", "iptables"}
	case "rhel":
		pkgs = []string{"firewalld", "iptables"}
	case "alpine":
		pkgs = []string{"iptables", "iptables-openrc"}
	default:
		pkgs = []string{"ufw", "iptables"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogError("Failed to install firewall-manager packages.")
		return
	}

	if core.SysInfo.Family == "alpine" || core.SysInfo.InitSystem == "openrc" {
		_ = exec.Command("rc-update", "add", "iptables", "default").Run()
		_ = exec.Command("rc-service", "iptables", "start").Run()
	} else {
		system.StartService("firewalld")
		system.StartService("ufw")
		system.StartService("iptables")
	}

	system.SaveState(map[string]string{
		"ENV_TYPE": "native",
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("firewall-manager Native setup finished successfully.")
}

// FreshInstallWizard handles the end-to-end installation pipeline for Universal Linux Firewall Management Engine.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(mode string) {
	PromptExistingConfigAction("Firewall Manager & Security Suite", "/etc/ufw/ufw.conf", "/etc/firewalld")

	keepSSH := true
	allowWeb := true
	enableFail2ban := true
	enableSysctl := true

	if !core.AutoYes && os.Getenv("KROTEK_NON_INTERACTIVE") != "1" {
		step := 1
		for {
			switch step {
			case 1:
				tui.ClearScreen()
				guardLines := []string{
					core.BOLD + core.RED + "CRITICAL REMOTE ACCESS PROTECTION (SSH GUARD)" + core.NC,
					"",
					"  To prevent accidentally locking yourself out of this remote machine,",
					"  the installer will automatically ensure Port 22 (SSH) remains OPEN.",
					"",
					core.BOLD + core.YELLOW + "Keep SSH Port (22) Open & Allow Inbound Admin Sessions?" + core.NC,
				}
				idx, _, st := tui.SelectMenuWithContent("Step 1/3: Remote Access Safety Guard", guardLines, []string{
					"YES" + " " + core.Sym.EmDash + " " + "Keep SSH Port 22 Open (STRONGLY RECOMMENDED)",
					"NO " + " " + core.Sym.EmDash + " " + "Do Not Automatically Open SSH (High Risk of Lockout)",
				})
				if st != 0 {
					return
				}
				keepSSH = (idx == 0)
				step = 2

			case 2:
				tui.ClearScreen()
				trafficLines := []string{
					core.BOLD + core.CYAN + "Inbound Web Services & Hardening Policy" + core.NC,
					"",
					"  Select which standard inbound services to permit through the firewall:",
					"  HTTP (Port 80) and HTTPS (Port 443) are standard for web servers and reverse proxies.",
					"",
					core.BOLD + core.YELLOW + "Allow Standard Web Traffic (Ports 80 & 443)?" + core.NC,
				}
				wIdx, _, wSt := tui.SelectMenuWithContent("Step 2/3: Standard Inbound Traffic", trafficLines, []string{
					"YES" + " " + core.Sym.EmDash + " " + "Open HTTP (80) & HTTPS (443)",
					"NO " + " " + core.Sym.EmDash + " " + "Block Web Traffic by Default",
				})
				if wSt != 0 {
					step = 1
					continue
				}
				allowWeb = (wIdx == 0)

				f2bRes := tui.AskYesNo("Enable Fail2ban Automated Intrusion Prevention (SSH Brute-Force Jails)?")
				enableFail2ban = (f2bRes == 0)

				sysRes := tui.AskYesNo("Apply Kernel Sysctl Hardening (Anti-DDoS, SYN Flood, ICMP Spoofing)?")
				enableSysctl = (sysRes == 0)

				step = 3

			case 3:
				tui.ClearScreen()
				summaryLines := []string{
					tui.FormatKV("Deployment Target", func() string {
						if mode == "docker" {
							return "Docker Containerized Firewall Engine"
						}
						return "Native Linux Firewall (UFW / firewalld / iptables)"
					}(), core.GREEN),
					tui.FormatKV("SSH Port 22 Guard", func() string {
						if keepSSH {
							return core.GREEN + "[ENABLED] (Lockout Prevention Active)" + core.NC
						}
						return core.RED + "[DISABLED] (Manual SSH Config Required)" + core.NC
					}(), core.GREEN),
					tui.FormatKV("Web Traffic (80/443)", func() string {
						if allowWeb {
							return core.GREEN + "[ALLOWED]" + core.NC
						}
						return core.YELLOW + "[BLOCKED]" + core.NC
					}(), core.GREEN),
					tui.FormatKV("Fail2ban Intrusion Defense", func() string {
						if enableFail2ban {
							return core.GREEN + "[ENABLED] (Active Jails)" + core.NC
						}
						return core.YELLOW + "[SKIPPED]" + core.NC
					}(), core.CYAN),
					tui.FormatKV("Kernel Sysctl Hardening", func() string {
						if enableSysctl {
							return core.GREEN + "[ENABLED] (SYN Flood Guard)" + core.NC
						}
						return core.YELLOW + "[SKIPPED]" + core.NC
					}(), core.CYAN),
				}
				idx, _, st := tui.SelectMenuWithContent("Step 3/3: Security Policy Summary", summaryLines, []string{
					"Confirm & Apply Firewall Rules",
					"<- Change Configuration (Back)",
					"Cancel Setup & Exit",
				})
				if st != 0 || idx == 2 {
					return
				}
				if idx == 1 {
					step = 2
					continue
				}

				if mode == "docker" {
					ExecuteDockerInstall()
				} else {
					ExecuteNativeInstall()
				}

				// Apply selected rules
				if keepSSH {
					_ = exec.Command("ufw", "allow", "22/tcp").Run()
					_ = exec.Command("firewall-cmd", "--permanent", "--add-service=ssh").Run()
				}
				if allowWeb {
					_ = exec.Command("ufw", "allow", "80/tcp").Run()
					_ = exec.Command("ufw", "allow", "443/tcp").Run()
					_ = exec.Command("firewall-cmd", "--permanent", "--add-service=http").Run()
					_ = exec.Command("firewall-cmd", "--permanent", "--add-service=https").Run()
				}
				if enableFail2ban {
					if core.SysInfo.Family == "debian" {
						_ = exec.Command("apt-get", "install", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "fail2ban").Run()
					} else {
						_ = exec.Command("dnf", "install", "-y", "fail2ban").Run()
					}
					system.StartService("fail2ban")
				}
				if enableSysctl {
					sysctlCfg := "# Krotek Security Hardening\nnet.ipv4.tcp_syncookies = 1\nnet.ipv4.conf.all.rp_filter = 1\nnet.ipv4.conf.default.rp_filter = 1\nnet.ipv4.icmp_echo_ignore_broadcasts = 1\nnet.ipv4.icmp_ignore_bogus_error_responses = 1\nnet.ipv4.conf.all.accept_source_route = 0\nnet.ipv4.tcp_max_syn_backlog = 2048\n"
					_ = system.WriteFileNoFollow("/etc/sysctl.d/99-krotek-security.conf", []byte(sysctlCfg), 0644, "")
					_ = exec.Command("sysctl", "--system").Run()
				}
				return
			}
		}
	}

	if mode == "docker" {
		ExecuteDockerInstall()
	} else {
		ExecuteNativeInstall()
	}
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
