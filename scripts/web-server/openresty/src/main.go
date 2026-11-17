// ==============================================================================
//  _  ______   ___ _____ _____ _  __   ____   ____ ____  ___ ____ _____
// | |/ /  _ \ / _ \_   _| ____| |/ /  / ___| / ___|  _ \|_ _|  _ \_   _|
// | ' /| |_) | | | || | |  _| | ' /   \___ \| |   | |_) || || |_) || |
// | . \|  _ <| |_| || | | |___| . \    ___) | |___|  _ < | ||  __/ | |
// |_|\_\_| \_\\___/ |_| |_____|_|\_\  |____/ \____|_| \_\___|_|    |_|
//
// KROTEK SCRIPT HUB — PRODUCTION AUTOMATION & MANAGEMENT ENGINE
// ------------------------------------------------------------------------------
// Service:         OpenResty High-Performance Web Server & Reverse Proxy
// Category:        Web Servers & Reverse Proxies (OpenResty)
// Runtime Engine:  Go 1.21+ (CGO_ENABLED=0, Static ELF Standalone Binary)
// Global Command:  sudo openresty-manager
// Supported OS:    Debian, Ubuntu, RHEL, AlmaLinux, Rocky Linux, Alpine Linux
// Architectures:   x86_64 (amd64), aarch64 (arm64)
// Service ID:      OpenResty
//
// DESCRIPTION:
// Production OpenResty server with VirtualHost manager, Let's Encrypt SSL auto-renewal, PHP-FPM wiring, and gzip compression.
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
//   --reinstall
//       Forces clean reinstallation setup wizard directly, skipping the existing installation detection prompt.
//   --uninstall
//       Executes clean uninstallation, stops background daemons, cleans up systemd units and optionally purges data.
//   --port (-p) <port> [Env: KROTEK_PORT]
//       TCP network port for the primary HTTP server and VirtualHost listeners.
//   --domain (-d) <fqdn> [Env: KROTEK_DOMAIN]
//       Configures public VirtualHost with domain name (e.g. example.com). Automatically sets public internet mode.
//   --ssl [Env: KROTEK_SSL=true]
//       Requests and automates Let's Encrypt SSL/TLS certificates with auto-renewal timers.
//   --php [Env: KROTEK_PHP=true]
//       Installs and wires PHP-FPM FastCGI backend pool with optimized socket routing.
//
// PERMANENT SYSTEM ACCESS:
// After first execution, this tool installs itself into /usr/local/bin so you
// can launch the management panel anytime using:
//   sudo openresty-manager
// ==============================================================================

package main

import (
	"fmt"
	"krotek-openresty-installer/core"
	"krotek-openresty-installer/install"
	"krotek-openresty-installer/logger"
	"krotek-openresty-installer/modules"
	"krotek-openresty-installer/system"
	"krotek-openresty-installer/tui"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func printHelp(binName string) {
	fmt.Printf(`Usage: sudo %s [OPTIONS]

Options:
  -y, --yes, --non-interactive  Run unattended without TUI
  -m, --mode <native|docker>    Installation architecture (default: native)
  -p, --port <1-65535>          HTTP listening port (default: 80)
  -d, --domain <fqdn>           Configure VirtualHost with domain
      --ssl [mode]              SSL mode (certbot|cloudflare|custom|none, default: certbot)
      --ssl-cert <path>         Path to certificate file (.crt/.pem) for custom SSL
      --ssl-key <path>          Path to private key file (.key) for custom SSL
      --ssl-email <email>       Admin email for Let's Encrypt Certbot registration
      --php                     Enable PHP-FPM FastCGI stack
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

// main is the primary execution entry point for the OpenResty High-Performance Web Server & Reverse Proxy.
// It validates root access, parses CLI arguments, initializes logging, registers signal
// handlers for clean terminal teardown, and routes to the appropriate TUI or headless mode.
func main() {
	var binName string
	binName = filepath.Base(os.Args[0])
	if binName == "" || binName == "." || binName == "main" || strings.HasPrefix(binName, "krotek-installer") {
		binName = "openresty-manager"
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
	doUpdate := false
	args := os.Args[1:]

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			printHelp(binName)
			os.Exit(0)
		case arg == "-y" || arg == "--yes" || arg == "--non-interactive" || arg == "--silent" || arg == "-s":
			core.AutoYes = true
			_ = os.Setenv("KROTEK_NON_INTERACTIVE", "1")
		case arg == "--update" || arg == "-u" || arg == "update" || arg == "--upgrade" || arg == "upgrade":
			doUpdate = true
		case arg == "--install" || arg == "-i" || arg == "install":
			forceInstall = true
		case arg == "--reinstall":
			forceReinstall = true
			forceInstall = true
		case arg == "--force" || arg == "-f" || arg == "force" || os.Getenv("KROTEK_FORCE") == "1":
			core.OverwriteExistingConfig = true
			forceReinstall = true
			forceInstall = true
		case arg == "--uninstall" || arg == "uninstall":
			doUninstall = true
		case arg == "--mode" || arg == "-m":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Missing value for %s flag\n", core.RED, core.NC, arg)
				os.Exit(1)
			}
			modeFlag = strings.ToLower(args[i+1])
			if modeFlag != "native" && modeFlag != "docker" {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid mode '%s'. Allowed values: 'native' or 'docker'\n", core.RED, core.NC, modeFlag)
				os.Exit(1)
			}
			_ = os.Setenv("KROTEK_MODE", modeFlag)
			i++
		case strings.HasPrefix(arg, "--mode="):
			modeFlag = strings.ToLower(strings.TrimPrefix(arg, "--mode="))
			if modeFlag != "native" && modeFlag != "docker" {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid mode '%s'. Allowed values: 'native' or 'docker'\n", core.RED, core.NC, modeFlag)
				os.Exit(1)
			}
			_ = os.Setenv("KROTEK_MODE", modeFlag)
		case arg == "--port" || arg == "-p":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Missing value for %s flag\n", core.RED, core.NC, arg)
				os.Exit(1)
			}
			portVal := args[i+1]
			pNum, err := strconv.Atoi(portVal)
			if err != nil || pNum < 1 || pNum > 65535 {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid port '%s'. Must be an integer between 1 and 65535\n", core.RED, core.NC, portVal)
				os.Exit(1)
			}
			_ = os.Setenv("KROTEK_PORT", portVal)
			i++
		case strings.HasPrefix(arg, "--port="):
			portVal := strings.TrimPrefix(arg, "--port=")
			pNum, err := strconv.Atoi(portVal)
			if err != nil || pNum < 1 || pNum > 65535 {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid port '%s'. Must be an integer between 1 and 65535\n", core.RED, core.NC, portVal)
				os.Exit(1)
			}
			_ = os.Setenv("KROTEK_PORT", portVal)
		case arg == "--domain" || arg == "-d":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Missing value for %s flag\n", core.RED, core.NC, arg)
				os.Exit(1)
			}
			cleanDom := system.CleanDomain(args[i+1])
			if !system.ValidateDomainName(cleanDom) {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid domain '%s'. Must be a valid FQDN (e.g. example.com)\n", core.RED, core.NC, args[i+1])
				os.Exit(1)
			}
			_ = os.Setenv("KROTEK_DOMAIN", cleanDom)
			i++
		case strings.HasPrefix(arg, "--domain="):
			rawDom := strings.TrimPrefix(arg, "--domain=")
			cleanDom := system.CleanDomain(rawDom)
			if !system.ValidateDomainName(cleanDom) {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid domain '%s'. Must be a valid FQDN (e.g. example.com)\n", core.RED, core.NC, rawDom)
				os.Exit(1)
			}
			_ = os.Setenv("KROTEK_DOMAIN", cleanDom)
		case arg == "--ssl":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				sslVal := strings.ToLower(strings.TrimSpace(args[i+1]))
				switch sslVal {
				case "1", "true", "certbot", "letsencrypt", "cloudflare", "cf", "custom", "manual", "ovh", "origin", "none", "skip", "0", "false":
					_ = os.Setenv("KROTEK_SSL", sslVal)
				default:
					fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid SSL mode '%s'. Allowed: certbot, cloudflare, custom, none\n", core.RED, core.NC, args[i+1])
					os.Exit(1)
				}
				i++
			} else {
				_ = os.Setenv("KROTEK_SSL", "certbot")
			}
		case strings.HasPrefix(arg, "--ssl="):
			rawSSL := strings.TrimPrefix(arg, "--ssl=")
			sslVal := strings.ToLower(strings.TrimSpace(rawSSL))
			switch sslVal {
			case "1", "true", "certbot", "letsencrypt", "cloudflare", "cf", "custom", "manual", "ovh", "origin", "none", "skip", "0", "false":
				_ = os.Setenv("KROTEK_SSL", sslVal)
			default:
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid SSL mode '%s'. Allowed: certbot, cloudflare, custom, none\n", core.RED, core.NC, rawSSL)
				os.Exit(1)
			}
		case arg == "--ssl-cert":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Missing value for %s flag\n", core.RED, core.NC, arg)
				os.Exit(1)
			}
			_ = os.Setenv("KROTEK_SSL_CERT", args[i+1])
			i++
		case strings.HasPrefix(arg, "--ssl-cert="):
			_ = os.Setenv("KROTEK_SSL_CERT", strings.TrimPrefix(arg, "--ssl-cert="))
		case arg == "--ssl-key":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Missing value for %s flag\n", core.RED, core.NC, arg)
				os.Exit(1)
			}
			_ = os.Setenv("KROTEK_SSL_KEY", args[i+1])
			i++
		case strings.HasPrefix(arg, "--ssl-key="):
			_ = os.Setenv("KROTEK_SSL_KEY", strings.TrimPrefix(arg, "--ssl-key="))
		case arg == "--ssl-email":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Missing value for %s flag\n", core.RED, core.NC, arg)
				os.Exit(1)
			}
			_ = os.Setenv("KROTEK_SSL_EMAIL", args[i+1])
			i++
		case strings.HasPrefix(arg, "--ssl-email="):
			_ = os.Setenv("KROTEK_SSL_EMAIL", strings.TrimPrefix(arg, "--ssl-email="))
		case arg == "--php":
			_ = os.Setenv("KROTEK_PHP", "true")
		case arg == "--ascii":
			core.SetSymbolMode(false)
			_ = os.Setenv("KROTEK_SYMBOLS", "ascii")
		case arg == "--fancy" || arg == "--unicode":
			core.SetSymbolMode(true)
			_ = os.Setenv("KROTEK_SYMBOLS", "fancy")
		default:
			fmt.Fprintf(os.Stderr, "%s[ERROR]%s Unknown argument: %s. Use --help for usage.\n", core.RED, core.NC, arg)
			os.Exit(1)
		}
	}

	if envPort := os.Getenv("KROTEK_PORT"); envPort != "" {
		if pNum, err := strconv.Atoi(envPort); err != nil || pNum < 1 || pNum > 65535 {
			fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid KROTEK_PORT environment variable '%s'. Must be an integer between 1 and 65535\n", core.RED, core.NC, envPort)
			os.Exit(1)
		}
	}
	if envMode := strings.ToLower(strings.TrimSpace(os.Getenv("KROTEK_MODE"))); envMode != "" {
		if envMode != "native" && envMode != "docker" {
			fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid KROTEK_MODE environment variable '%s'. Allowed values: 'native' or 'docker'\n", core.RED, core.NC, envMode)
			os.Exit(1)
		}
		_ = os.Setenv("KROTEK_MODE", envMode)
	}
	if envDom := strings.TrimSpace(os.Getenv("KROTEK_DOMAIN")); envDom != "" {
		cleanDom := system.CleanDomain(envDom)
		if !system.ValidateDomainName(cleanDom) {
			fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid KROTEK_DOMAIN environment variable '%s'. Must be a valid FQDN\n", core.RED, core.NC, envDom)
			os.Exit(1)
		}
		_ = os.Setenv("KROTEK_DOMAIN", cleanDom)
	}
	if envSSL := strings.ToLower(strings.TrimSpace(os.Getenv("KROTEK_SSL"))); envSSL != "" {
		switch envSSL {
		case "1", "true", "certbot", "letsencrypt", "cloudflare", "cf", "custom", "manual", "ovh", "origin", "none", "skip", "0", "false":
		default:
			fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid KROTEK_SSL environment variable '%s'. Allowed: certbot, cloudflare, custom, none\n", core.RED, core.NC, envSSL)
			os.Exit(1)
		}
	}
	if envWebDir := strings.TrimSpace(os.Getenv("KROTEK_WEB_DIR")); envWebDir != "" {
		if err := system.ValidateSafeDirectoryPath(envWebDir); err != nil {
			fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid KROTEK_WEB_DIR environment variable '%s': %v\n", core.RED, core.NC, envWebDir, err)
			os.Exit(1)
		}
	}
	for _, sslEnv := range []string{"KROTEK_SSL_CERT", "KROTEK_SSL_KEY"} {
		if p := strings.TrimSpace(os.Getenv(sslEnv)); p != "" {
			if !strings.HasPrefix(p, "/") || strings.Contains(p, "..") || strings.ContainsAny(p, ";\r\n\x00\"' \t$`\\|&<>{}") {
				fmt.Fprintf(os.Stderr, "%s[ERROR]%s Invalid %s path '%s': must be an absolute path without unsafe characters\n", core.RED, core.NC, sslEnv, p)
				os.Exit(1)
			}
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

	if strings.Contains(binName, "installer") {
		forceInstall = true
	}

	// ─── Step 3: Hardware, OS & Init-System Auto-Detection ────────────────────
	// Identifies distribution family (Debian/RHEL/Alpine), package manager, and init system.
	system.DetectSystem()
	modules.EnsureOpenRestyWorking()

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

	if doUpdate {
		system.RunServiceUpdateFlow()
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

	// ─── Step 5: Terminal Signal Trapping (Graceful TUI Restoration & SIGWINCH) ──────────
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

// installerMenu displays the interactive first-time setup wizard for OpenResty High-Performance Web Server & Reverse Proxy,
// guiding the administrator through native system package installation or container deployment.
func installerMenu() {
	for {
		options := []string{
			"─── INSTALLATION MODES ───",
			"Install OpenResty Web Server (Guided Native Wizard)",
			"Install OpenResty via Docker Container Stack",
			"─── ADVANCED ───",
			"Skip Installation -> Go to Management Panel",
			"Exit",
		}

		_, selectedOpt, status := tui.SelectMenu("KROTEK OpenResty WEB SERVER" + " " + core.Sym.EmDash + " " + "Setup & Installation Wizard", options)

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
	serviceTitle := "OpenResty High-Performance Web Server & Reverse Proxy"
	managerCmd := "openresty-manager"

	cardLines := []string{
		core.BOLD + core.YELLOW + "EXISTING " + strings.ToUpper("OpenResty") + " INSTALLATION DETECTED" + core.NC,
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
		"      Tip: You can run with --force (-f) flag to automatically overwrite all existing files.",
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
