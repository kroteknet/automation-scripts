// ==============================================================================
//  _  ______   ___ _____ _____ _  __   ____   ____ ____  ___ ____ _____
// | |/ /  _ \ / _ \_   _| ____| |/ /  / ___| / ___|  _ \|_ _|  _ \_   _|
// | ' /| |_) | | | || | |  _| | ' /   \___ \| |   | |_) || || |_) || |
// | . \|  _ <| |_| || | | |___| . \    ___) | |___|  _ < | ||  __/ | |
// |_|\_\_| \_\\___/ |_| |_____|_|\_\  |____/ \____|_| \_\___|_|    |_|
//
// KROTEK SCRIPT HUB — PRODUCTION AUTOMATION & MANAGEMENT ENGINE
// ------------------------------------------------------------------------------
// Service:         Caddy 2 Web Server with Automatic HTTPS
// Category:        Web Servers & Reverse Proxies (caddy)
// Runtime Engine:  Go 1.21+ (CGO_ENABLED=0, Static ELF Standalone Binary)
// Global Command:  sudo caddy-manager
// Supported OS:    Debian, Ubuntu, RHEL, AlmaLinux, Rocky Linux, Alpine Linux
// Architectures:   x86_64 (amd64), aarch64 (arm64)
// Service ID:      caddy
//
// DESCRIPTION:
// Zero-configuration web server and reverse proxy with automated ACME TLS certificate management and Caddyfile validator.
//
// MODES OF OPERATION:
//   1. Interactive TUI: Zero-flicker terminal interface with real-time status,
//      live service health metrics, log monitoring, and guided setup wizards.
//   2. Headless CLI: Scriptable, unattended automation mode for CI/CD pipelines
//      and cloud provisioning scripts.
//
// CLI FLAGS & OPTIONS:
//   --non-interactive (-y, --yes, -s, --silent) [Env: KROTEK_NON_INTERACTIVE=1]
//       Enables fully unattended mode. Skips interactive TUI menus and proceeds immediately using supplied flags or defaults.
//   --mode (-m) <native|docker> [Env: KROTEK_MODE=native]
//       Selects the installation stack architecture: direct host system daemon (native) or container stack (docker).
//   --install (-i)
//       Forces clean installation wizard setup even if an existing service or container is detected on the system.
//   --uninstall
//       Executes clean uninstallation, stops background daemons, cleans up systemd units and optionally purges data.
//   --port (-p) <port> [Env: KROTEK_PORT]
//       Standard HTTP listening port. Caddy automatically provisions HTTPS on port 443.
//   --domain (-d) <fqdn> [Env: KROTEK_DOMAIN]
//       Domain name for Caddyfile. Caddy automatically negotiates Let's Encrypt TLS.
//
// PERMANENT SYSTEM ACCESS:
// After first execution, this tool installs itself into /usr/local/bin so you
// can launch the management panel anytime using:
//   sudo caddy-manager
// ==============================================================================

package main

import (
	"fmt"
	"krotek-caddy-installer/core"
	"krotek-caddy-installer/install"
	"krotek-caddy-installer/logger"
	"krotek-caddy-installer/modules"
	"krotek-caddy-installer/system"
	"krotek-caddy-installer/tui"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// main is the primary execution entry point for the Caddy 2 Web Server with Automatic HTTPS.
// It validates root access, parses CLI arguments, initializes logging, registers signal
// handlers for clean terminal teardown, and routes to the appropriate TUI or headless mode.
func printHelp(binName string) {
	fmt.Printf(`Usage: sudo %s [OPTIONS]

Krotek Script Hub" + " " + core.Sym.EmDash + " " + "Caddy 2 Web Server with Automatic HTTPS
Global Command: sudo caddy-manager

Options:
  -y, --yes, --non-interactive  Run unattended without TUI
  -m, --mode <native|docker>    Installation architecture (default: native)
  -p, --port <1-65535>          Service network port
  -d, --domain <fqdn>           Configure domain / VirtualHost
      --ssl                     Automate Let's Encrypt certificates
  -i, --install                 Force clean installation wizard
      --reinstall               Force reinstallation setup
      --force, -f               Overwrite existing configuration files
      --uninstall               Clean removal of services and configs
  -u, --update                  Run automated service updates
      --ascii                   Force plain ASCII symbols (no Unicode/box glyphs)
      --fancy                   Force Unicode box/technical symbols (no emojis)
  -h, --help                    Display this help message
`, binName)
}

func main() {
	var binName string
	binName = filepath.Base(os.Args[0])
	if binName == "" || binName == "." || binName == "main" || strings.HasPrefix(binName, "krotek-installer") {
		binName = "caddy-manager"
	}
	binName = filepath.Base(os.Args[0])
	if binName == "" || binName == "." || binName == "main" || strings.HasPrefix(binName, "krotek-installer") {
		binName = "caddy-manager"
	}

	// ─── Step 0: Global Panic Recovery (Terminal Restoration Guard) ───────────
	defer func() {
		if r := recover(); r != nil {
			tui.ShowCursor()
			tui.DisableRawMode()
			tui.LeaveAltScreen()
			fmt.Fprintf(os.Stderr, "\n%s[CRITICAL ERROR]%s Unexpected runtime failure: %v\n", core.RED, core.NC, r)
			os.Exit(2)
		}
	}()
	// ─── Early CLI Dispatch (--help) ──────────────────────────────────────────
	for _, a := range os.Args[1:] {
		if rich, ok := core.SymbolModeArg(a); ok {
			core.SetSymbolMode(rich)
			if rich {
				_ = os.Setenv("KROTEK_SYMBOLS", "fancy")
			} else {
				_ = os.Setenv("KROTEK_SYMBOLS", "ascii")
			}
		}
		if a == "-h" || a == "--help" || a == "help" {
			printHelp(binName)
			os.Exit(0)
		}
	}

	// ─── Step 1: Root Privilege Verification ──────────────────────────────────
	// Superuser privileges are mandatory for managing system services and packages.
	if os.Getuid() != 0 {
		fmt.Fprintf(os.Stderr, "%s[ERROR]%s This tool must be run as root. Try: sudo %s\n", core.RED, core.NC, binName)
		os.Exit(1)
	}

	// ─── Step 2: Parse CLI Arguments & Unattended Automation Flags ───────────
	// Evaluates command-line flags and sets environment variables for silent CI/CD runs.
	forceInstall := false
	forceReinstall := false
	modeFlag := ""
	doUninstall := false
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-y" || arg == "--yes" || arg == "--non-interactive" || arg == "--silent" || arg == "-s" {
			core.AutoYes = true
			_ = os.Setenv("KROTEK_NON_INTERACTIVE", "1")
		}
		if arg == "--install" || arg == "-i" || arg == "install" {
			forceInstall = true
		}
		if arg == "--reinstall" {
			forceReinstall = true
			forceInstall = true
		}
		if arg == "--force" || arg == "-f" || arg == "force" || os.Getenv("KROTEK_FORCE") == "1" {
			core.OverwriteExistingConfig = true
			forceReinstall = true
			forceInstall = true
		}
		if arg == "--uninstall" || arg == "uninstall" {
			doUninstall = true
		}
		if arg == "--update" || arg == "update" {
			system.RunServiceUpdateFlow()
			os.Exit(0)
		}
		if arg == "--mode" || arg == "-m" {
			if i+1 < len(args) {
				m := strings.ToLower(args[i+1])
				if m != "native" && m != "docker" {
					fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid installation mode '%s'. Allowed: native, docker\n", core.RED, core.NC, args[i+1])
					os.Exit(1)
				}
				modeFlag = m
				_ = os.Setenv("KROTEK_MODE", modeFlag)
				i++
			} else {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Flag %s requires an argument (native|docker)\n", core.RED, core.NC, arg)
				os.Exit(1)
			}
		}
		if arg == "--port" || arg == "-p" {
			if i+1 < len(args) {
				pNum, err := strconv.Atoi(args[i+1])
				if err != nil || pNum < 1 || pNum > 65535 {
					fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid port '%s'. Port must be between 1 and 65535\n", core.RED, core.NC, args[i+1])
					os.Exit(1)
				}
				_ = os.Setenv("KROTEK_PORT", args[i+1])
				i++
			} else {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Flag %s requires a port number\n", core.RED, core.NC, arg)
				os.Exit(1)
			}
		}
	}
	binName = filepath.Base(os.Args[0])
	if strings.Contains(binName, "installer") {
		forceInstall = true
	}

	if envPort := os.Getenv("KROTEK_PORT"); envPort != "" {
		if pNum, err := strconv.Atoi(envPort); err != nil || pNum < 1 || pNum > 65535 {
			fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid KROTEK_PORT environment variable '%s'. Must be between 1 and 65535\n", core.RED, core.NC, envPort)
			os.Exit(1)
		}
	}

	// ─── Step 2b: TTY Verification (Non-Interactive Fallback Check) ───────────
	inStat, inErr := os.Stdin.Stat()
	outStat, outErr := os.Stdout.Stat()
	isTTY := inErr == nil && outErr == nil && (inStat.Mode()&os.ModeCharDevice) != 0 && (outStat.Mode()&os.ModeCharDevice) != 0

	if !isTTY && !core.AutoYes && os.Getenv("KROTEK_NON_INTERACTIVE") != "1" {
		fmt.Fprintf(os.Stderr, "%s[ERROR]%s Interactive terminal (TTY) not detected.\nWhen running in pipelines, automated scripts, or background tasks, specify -y / --non-interactive.\n", core.RED, core.NC)
		os.Exit(1)
	}

	// ─── Step 3: Hardware, OS & Init-System Auto-Detection ────────────────────
	// Identifies distribution family (Debian/RHEL/Alpine), package manager, and init system.
	system.DetectSystem()

	// ─── Step 4: Persistent File Logger Initialization ───────────────────────
	// Logs operational events and diagnostic reports to /var/log/.
	logger.InitLogger()
	defer func() {
		if core.LogFile != nil {
			_ = core.LogFile.Close()
		}
	}()

	core.BannerProvider = modules.GetContextualBanner

	// ─── Unattended Execution & Clean CLI Dispatch ────────────────────────────
	if doUninstall {
		install.UninstallWizard()
		os.Exit(0)
	}

	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
		mode := modeFlag
		if mode == "" {
			mode = os.Getenv("KROTEK_MODE")
		}
		if mode == "" {
			mode = "native"
		}
		install.FreshInstallWizard(mode)
		os.Exit(0)
	}

	// ─── Step 5: Terminal Signal Trapping (Graceful TUI Restoration) ──────────
	// Traps SIGINT/SIGTERM/SIGHUP to restore terminal cursor and disable raw mode on exit.
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGWINCH)
	go func() {
		for sig := range sigChan {
			if sig == syscall.SIGWINCH {
				tui.InvalidateTermCache()
			} else {
				tui.ShowCursor()
				tui.DisableRawMode()
				tui.LeaveAltScreen()
				os.Exit(0)
			}
		}
	}()

	// ─── Step 6: Enter TUI Alternate Screen Buffer ───────────────────────────
	// Activates full-screen private buffer for flicker-free interactive navigation.
	tui.EnterAltScreen()
	defer func() {
		tui.ShowCursor()
		tui.DisableRawMode()
		tui.LeaveAltScreen()
	}()

	modules.EnsureCaddyWorking()

	// ─── Step 7: Mode Dispatcher (Existing Detection vs Installer vs Management Dashboard) ─────────
	if system.IsServiceInstalled() {
		if forceReinstall {
			installerMenu()
		} else if forceInstall {
			existingInstallationPrompt()
		} else {
			managementMenu()
		}
	} else {
		installerMenu()
	}
}

// installerMenu displays the interactive first-time setup wizard for Caddy 2 Web Server with Automatic HTTPS,
// guiding the administrator through native system package installation or container deployment.
func installerMenu() {
	for {
		options := []string{
			"─── INSTALLATION MODES ───",
			"Install Caddy Web Server (Guided Native Wizard)",
			"Install Caddy via Docker Container Stack",
			"─── ADVANCED ───",
			"Skip Installation -> Go to Management Panel",
			"Exit",
		}

		_, selectedOpt, status := tui.SelectMenu("KROTEK CADDY WEB SERVER" + " " + core.Sym.EmDash + " " + "First-Time Setup Installer", options)

		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Exit") {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		cleanOpt := core.StripANSI(selectedOpt)
		switch {
		case strings.Contains(cleanOpt, "Guided Native Wizard"):
			install.FreshInstallWizard("native")
			managementMenu()
			return

		case strings.Contains(cleanOpt, "Docker Container Stack"):
			install.FreshInstallWizard("docker")
			managementMenu()
			return

		case strings.Contains(cleanOpt, "Skip Installation"):
			managementMenu()
			return
		}
	}
}

// existingInstallationPrompt detects whether an active or previous installation exists on the host
// and prompts the user whether they want to preserve their existing files and attach the TUI management overlay,
// or execute a complete clean reinstallation.
func existingInstallationPrompt() {
	serviceTitle := "Caddy 2 Web Server with Automatic HTTPS"
	managerCmd := "caddy-manager"

	cardLines := []string{
		core.BOLD + core.YELLOW + "EXISTING " + strings.ToUpper("caddy") + " INSTALLATION DETECTED" + core.NC,
		"",
		fmt.Sprintf("  An existing %s installation was detected on this system.", serviceTitle),
		"  Please choose how you would like to proceed:",
		"",
		tui.FormatKV("Service Name", serviceTitle, core.CYAN),
		tui.FormatKV("Installation Status", core.GREEN+"[DETECTED ON HOST]"+core.NC, core.GREEN),
		tui.FormatKV("Management Command", "sudo "+managerCmd, core.CYAN),
		"",
		"  " + core.BOLD + core.GREEN + "[1] Keep Existing Files & Attach TUI Manager (Recommended)" + core.NC,
		"      Preserves all existing data, configurations, databases, and custom files.",
		fmt.Sprintf("      Installs the permanent '%s' TUI overlay to manage your service.", managerCmd),
		"",
		"  " + core.BOLD + core.YELLOW + "[2] Reinstall (Clean / Guided Setup Wizard)" + core.NC,
		"      Runs the full installer wizard to reinstall packages or recreate stack.",
		"      Allows reconfiguring server settings or resetting to factory defaults.",
	}

	choices := []string{
		"1. Keep Existing Files & Attach TUI Manager (Recommended)",
		"2. Reinstall (Clean / Guided Setup Wizard)",
		"0. Exit Installer",
	}

	for {
		tui.ClearScreen()
		idx, selectedOpt, status := tui.SelectMenuWithContent("KrotekNet" + " " + core.Sym.EmDash + " " + "Existing Installation Conflict Handler", cardLines, choices)
		cleanOpt := core.StripANSI(selectedOpt)

		if status == 2 || (status == 0 && (strings.HasPrefix(cleanOpt, "0.") || strings.Contains(cleanOpt, "Exit"))) {
			tui.LeaveAltScreen()
			os.Exit(0)
		}

		switch idx {
		case 0:
			adoptExistingService(managerCmd, serviceTitle)
			managementMenu()
			return
		case 1:
			installerMenu()
			return
		default:
			tui.LeaveAltScreen()
			os.Exit(0)
		}
	}
}

// adoptExistingService saves the environment configuration, registers global management shortcuts,
// and confirms that existing custom files and data remain completely untouched.
func adoptExistingService(managerCmd, serviceTitle string) {
	state := system.LoadState()
	if state["ACTION"] == "" {
		state["ACTION"] = "adopted"
	}
	if state["ENV_TYPE"] == "" {
		state["ENV_TYPE"] = "native"
	}
	system.SaveState(state)
	system.CreateGlobalShortcut()

	tui.ClearScreen()
	var summaryLines []string
	summaryLines = append(summaryLines, core.BOLD+core.GREEN+"TUI MANAGEMENT OVERLAY ATTACHED SUCCESSFULLY!"+core.NC)
	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, "  * All existing configuration files and data were preserved untouched.")
	summaryLines = append(summaryLines, "  * No packages or custom settings were overwritten.")
	summaryLines = append(summaryLines, fmt.Sprintf("  * Global Management Command registered: %s%ssudo %s%s", core.BOLD, core.GREEN, managerCmd, core.NC))
	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, tui.FormatKV("Service", serviceTitle, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "sudo "+managerCmd, core.GREEN))
	summaryLines = append(summaryLines, tui.FormatKV("Status", "Ready for management", core.GREEN))
	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.YELLOW+"Entering Management Dashboard... Press [Enter] to continue."+core.NC)

	tui.PrintInBox("KrotekNet" + " " + core.Sym.EmDash + " " + "TUI Overlay Active", summaryLines)
}
