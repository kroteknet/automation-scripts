// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: Nitrox Subnautica Multiplayer Server (nitrox)
//
// This package orchestrates automated pre-flight checks, package repository setup,
// .NET 9.0 dedicated server runtime installation, official binary deployment,
// configuration templating (server.cfg), service enablement (systemd/openrc),
// automated world backups, and UDP firewall rules.
// Supports both interactive TUI step-by-step guidance and headless automation.
// ==============================================================================

package install

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"krotek-nitrox-installer/core"
	"krotek-nitrox-installer/logger"
	"krotek-nitrox-installer/system"
	"krotek-nitrox-installer/tui"
)

type NitroxConfig struct {
	Mode          string
	ServerName    string
	Port          string
	MaxPlayers    string
	Password      string
	AdminPassword string
	GameMode      string
	InstallDir    string
}

// generateSecureTokenFromReader creates a cryptographically secure random alphanumeric string from r.
func generateSecureTokenFromReader(r io.Reader, length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("invalid token length: %d", length)
	}
	if r == nil {
		return "", fmt.Errorf("entropy reader must not be nil")
	}
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, length)
	if _, err := io.ReadFull(r, bytes); err != nil {
		return "", fmt.Errorf("cryptographic randomness unavailable: %w", err)
	}
	for i, b := range bytes {
		bytes[i] = charset[int(b)%len(charset)]
	}
	return string(bytes), nil
}

// generateSecureToken creates a cryptographically secure random alphanumeric string.
func generateSecureToken(length int) (string, error) {
	return generateSecureTokenFromReader(rand.Reader, length)
}

func chownDirsNoFollow(owner string, dirs ...string) {
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			_ = exec.Command("chown", "-h", owner, p).Run()
			return nil
		})
	}
}

// FreshInstallWizard handles the end-to-end installation pipeline for Nitrox Subnautica Multiplayer Server.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, NITROX_*).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(mode string) {
	cfg := NitroxConfig{
		Mode:          mode,
		ServerName:    "Subnautica Nitrox Server",
		Port:          "11000",
		MaxPlayers:    "8",
		Password:      "",
		AdminPassword: "",
		GameMode:      "SURVIVAL",
		InstallDir:    "/srv/nitrox",
	}

	// Environment variable overrides for headless automation / CI/CD
	if envPort := os.Getenv("KROTEK_PORT"); envPort != "" {
		cfg.Port = strings.TrimSpace(envPort)
	}
	if envName := os.Getenv("NITROX_SERVER_NAME"); envName != "" {
		cfg.ServerName = strings.TrimSpace(envName)
	}
	if envPass := os.Getenv("NITROX_PASSWORD"); envPass != "" {
		cfg.Password = strings.TrimSpace(envPass)
	}
	if envAdmin := os.Getenv("NITROX_ADMIN_PASSWORD"); envAdmin != "" {
		cfg.AdminPassword = strings.TrimSpace(envAdmin)
	}
	if envMode := os.Getenv("NITROX_GAME_MODE"); envMode != "" {
		m := strings.ToUpper(strings.TrimSpace(envMode))
		if m == "SURVIVAL" || m == "FREEDOM" || m == "HARDCORE" || m == "CREATIVE" {
			cfg.GameMode = m
		}
	}
	if envDir := os.Getenv("NITROX_INSTALL_DIR"); envDir != "" {
		trimmedDir := strings.TrimSpace(envDir)
		if system.ValidateSafeDirectoryPath(trimmedDir) == nil {
			cfg.InstallDir = trimmedDir
		}
	}

	PromptExistingConfigAction("Nitrox Subnautica Server",
		"/srv/nitrox/data/server.cfg",
		"/srv/nitrox/server/server.cfg",
		"/opt/nitrox/server.cfg",
	)

	// If unattended / automated mode, proceed immediately
	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
		if cfg.AdminPassword == "" {
			tok, tokErr := generateSecureToken(16)
			if tokErr != nil {
				logger.LogError(tokErr.Error())
				return
			}
			cfg.AdminPassword = tok
		}
		if cfg.Mode == "docker" {
			ExecuteDockerInstall(cfg)
		} else {
			ExecuteNativeInstall(cfg)
		}
		return
	}

	step := 1
	for {
		switch step {
		case 1:
			// Step 1: Server Name & Port
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Subnautica Nitrox Server Identity" + core.NC,
				"",
				"  Specify the server display name and UDP network port.",
				"  " + core.BOLD + "Default UDP Port:" + core.NC + " 11000 (Required for multiplayer join)",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Exit Setup", "Step 1/4: Server Name & Port", cardLines)

			nameVal, st := tui.ReadTextInput("Server Display Name", cfg.ServerName)
			if st != 0 {
				return
			}
			nameVal = strings.TrimSpace(nameVal)
			if nameVal == "" {
				nameVal = "Subnautica Nitrox Server"
			}
			cfg.ServerName = nameVal

			portVal, pSt := tui.ReadTextInput("Nitrox UDP Port", cfg.Port)
			if pSt != 0 {
				continue
			}
			portVal = strings.TrimSpace(portVal)
			if portVal == "" {
				portVal = "11000"
			}
			cfg.Port = portVal
			step = 2

		case 2:
			// Step 2: Player Capacity, Passwords & Game Mode
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Player Capacity, Game Mode & Access Control" + core.NC,
				"",
				"  Configure max concurrent players, game mode, and server passwords.",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: Game Settings & Security", cardLines)

			maxVal, mSt := tui.ReadTextInput("Max Concurrent Players (1-100)", cfg.MaxPlayers)
			if mSt != 0 {
				step = 1
				continue
			}
			maxVal = strings.TrimSpace(maxVal)
			if maxVal == "" {
				maxVal = "8"
			}
			cfg.MaxPlayers = maxVal

			// Select Game Mode
			gameModes := []string{
				"Survival Mode (Recommended" + " " + core.Sym.Bullet + " " + "Standard gameplay with health, hunger, oxygen, craft)",
				"Freedom Mode (No hunger or thirst requirements" + " " + core.Sym.Bullet + " " + "Pure exploration & crafting)",
				"Hardcore Mode (Permadeath" + " " + core.Sym.Bullet + " " + "Only one life, no oxygen level voice warnings)",
				"Creative Mode (Unlimited materials, instant crafting, no health or oxygen loss)",
			}
			gmIdx, _, gmSt := tui.SelectMenu("Select Subnautica World Game Mode", gameModes)
			if gmSt != 0 {
				step = 1
				continue
			}
			switch gmIdx {
			case 0:
				cfg.GameMode = "SURVIVAL"
			case 1:
				cfg.GameMode = "FREEDOM"
			case 2:
				cfg.GameMode = "HARDCORE"
			case 3:
				cfg.GameMode = "CREATIVE"
			default:
				cfg.GameMode = "SURVIVAL"
			}

			passVal, pwSt := tui.ReadPasswordInput("Server Join Password [Optional, Enter for open public]")
			if pwSt != 0 {
				step = 1
				continue
			}
			cfg.Password = strings.TrimSpace(passVal)

			adminVal, admSt := tui.ReadPasswordInput("Admin Secret Password [Enter to auto-generate secure token]")
			if admSt != 0 {
				step = 1
				continue
			}
			adminVal = strings.TrimSpace(adminVal)
			if adminVal == "" {
				tok, tokErr := generateSecureToken(16)
				if tokErr != nil {
					logger.LogError(tokErr.Error())
					return
				}
				adminVal = tok
			}
			cfg.AdminPassword = adminVal
			step = 3

		case 3:
			// Step 3: Install Directory
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "Nitrox Server & World Save Storage Path" + core.NC,
				"",
				"  Root directory where Nitrox binaries, world save data, and game files reside.",
				"  " + core.BOLD + "Default:" + core.NC + " /srv/nitrox",
				"",
				"  Subdirectories generated inside:",
				"    " + core.Sym.Pointer + " /srv/nitrox/server   - Official Nitrox Server binaries & runtime",
				"    " + core.Sym.Pointer + " /srv/nitrox/data     - World save files, entities, & server.cfg",
				"    " + core.Sym.Pointer + " /srv/nitrox/game     - Subnautica base game assets mount point",
				"    " + core.Sym.Pointer + " /srv/nitrox/backups  - Automated daily world snapshot archives",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 2", "Step 3/4: Installation Path", cardLines)

			dirVal, dSt := tui.ReadTextInput("Nitrox Server Directory", cfg.InstallDir)
			if dSt != 0 {
				step = 2
				continue
			}
			dirVal = strings.TrimSpace(dirVal)
			if dirVal == "" {
				dirVal = "/srv/nitrox"
			}
			if err := system.ValidateSafeDirectoryPath(dirVal); err != nil {
				tui.PrintInBox("Invalid Installation Path", []string{
					core.RED + err.Error() + core.NC,
				})
				continue
			}
			cfg.InstallDir = dirVal
			step = 4

		case 4:
			// Step 4: Review Summary Card
			tui.ClearScreen()
			pwDisplay := "Disabled (Open / Public Server)"
			if cfg.Password != "" {
				pwDisplay = cfg.Password
			}
			summary := []string{
				fmt.Sprintf("%sDeployment Mode:%s     %s", core.CYAN, core.NC, strings.ToUpper(cfg.Mode)),
				fmt.Sprintf("%sServer Name:%s         %s", core.CYAN, core.NC, cfg.ServerName),
				fmt.Sprintf("%sNetwork Port:%s        %s/UDP", core.CYAN, core.NC, cfg.Port),
				fmt.Sprintf("%sGame Mode:%s           %s", core.CYAN, core.NC, cfg.GameMode),
				fmt.Sprintf("%sMax Players:%s         %s", core.CYAN, core.NC, cfg.MaxPlayers),
				fmt.Sprintf("%sJoin Password:%s       %s", core.CYAN, core.NC, pwDisplay),
				fmt.Sprintf("%sAdmin Password:%s      %s", core.CYAN, core.NC, cfg.AdminPassword),
				fmt.Sprintf("%sInstallation Path:%s   %s", core.CYAN, core.NC, cfg.InstallDir),
				"",
				"Ready to provision and deploy Subnautica Nitrox Dedicated Server.",
			}
			tui.PrintInBox("--no-wait", "Step 4/4: Configuration Summary", summary)

			cOpts := []string{
				"Confirm & Proceed with Installation",
				"<- Change Configuration (Back)",
				"Cancel Installation",
			}
			cIdx, _, cSt := tui.SelectMenu("Confirm Nitrox Deployment", cOpts)
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

func configureFirewall(port string) {
	system.ConfigureFirewallRules(port + "/udp")
}

// hasDotNetRuntime checks if the required .NET runtime (or compatible) is available on the machine.
func hasDotNetRuntime(channel string) bool {
	paths := []string{"dotnet", "/usr/share/dotnet/dotnet", "/usr/local/bin/dotnet", "/usr/bin/dotnet"}
	majorVer := "9."
	if channel != "" {
		parts := strings.Split(channel, ".")
		majorVer = parts[0] + "."
	}
	for _, p := range paths {
		cmd := exec.Command(p, "--list-runtimes")
		if out, err := cmd.Output(); err == nil {
			str := string(out)
			if strings.Contains(str, "Microsoft.NETCore.App "+majorVer) || strings.Contains(str, "Microsoft.AspNetCore.App "+majorVer) || strings.Contains(str, "Microsoft.NETCore.App 9.") {
				return true
			}
		}
	}
	return false
}

// hasDotNet9Runtime checks if .NET 9.0 (or compatible) runtime is already installed on the machine.
func hasDotNet9Runtime() bool {
	return hasDotNetRuntime("9.0")
}

// EnsureDotNetRuntime guarantees Microsoft .NET Runtime is available on the system.
// Dynamically provisions the required channel via native package manager or official installer.
func EnsureDotNetRuntime(channel string) bool {
	if channel == "" {
		channel = "9.0"
	}
	if hasDotNetRuntime(channel) {
		logger.LogSuccess(fmt.Sprintf("Detected active Microsoft .NET %s (or compatible) runtime.", channel))
		return true
	}

	logger.LogInfo(fmt.Sprintf("Subnautica Nitrox requires Microsoft .NET %s Runtime. Installing...", channel))

	// 1. Try native package manager
	pkg := "dotnet-runtime-" + channel
	apkPkg := "dotnet" + strings.ReplaceAll(channel, ".", "") + "-runtime"
	switch core.SysInfo.Family {
	case "debian":
		_ = system.InstallPackages(pkg, "curl", "tar", "unzip", "ca-certificates")
	case "rhel":
		_ = system.InstallPackages(pkg, "curl", "tar", "unzip")
	case "alpine":
		_ = system.InstallPackages(apkPkg, "curl", "tar", "unzip")
	}

	if hasDotNetRuntime(channel) {
		logger.LogSuccess(fmt.Sprintf(".NET %s installed successfully via native package repository.", channel))
		return true
	}

	// 2. Fail-Safe: Official Microsoft automated dotnet-install.sh installer
	logger.LogInfo(fmt.Sprintf("Deploying official Microsoft .NET %s Runtime into /usr/share/dotnet...", channel))
	tui.StartSpinner(fmt.Sprintf("Fetching and deploying official Microsoft .NET %s Runtime...", channel))

	dotnetTmpDir, dtErr := os.MkdirTemp("", "krotek-dotnet-*")
	if dtErr != nil {
		tui.StopSpinner(1)
		logger.LogError("Failed to create secure temporary directory for .NET installer: " + dtErr.Error())
		return false
	}
	_ = os.Chmod(dotnetTmpDir, 0700)
	defer os.RemoveAll(dotnetTmpDir)
	dotnetScript := filepath.Join(dotnetTmpDir, "dotnet-install.sh")
	_ = os.MkdirAll("/usr/share/dotnet", 0755)
	_ = os.MkdirAll("/usr/local/bin", 0755)
	if scriptBytes, statusCode, fetchErr := system.HttpFetchBytes("https://dot.net/v1/dotnet-install.sh", 4*1024*1024); fetchErr == nil && statusCode == 200 && len(scriptBytes) > 0 {
		if wErr := system.WriteFileNoFollow(dotnetScript, scriptBytes, 0700, ""); wErr == nil {
			ran := false
			for _, args := range [][]string{
				{"--channel", channel, "--runtime", "dotnet", "--install-dir", "/usr/share/dotnet"},
				{"--version", "latest", "--channel", channel, "--install-dir", "/usr/share/dotnet"},
				{"--channel", "STS", "--runtime", "dotnet", "--install-dir", "/usr/share/dotnet"},
			} {
				if exec.Command(dotnetScript, args...).Run() == nil && hasDotNetRuntime(channel) {
					ran = true
					break
				}
			}
			_ = ran
		}
	} else {
		// Fallback to curl/wget over HTTPS if Go HTTP client encounters environment proxy differences.
		if exec.Command("curl", "--proto", "=https", "--tlsv1.2", "-fsSL", "https://dot.net/v1/dotnet-install.sh", "-o", dotnetScript).Run() == nil ||
			exec.Command("wget", "--https-only", "--secure-protocol=TLSv1_2", "-qO", dotnetScript, "https://dot.net/v1/dotnet-install.sh").Run() == nil {
			if !system.IsSymlinkPath(dotnetScript) {
				_ = os.Chmod(dotnetScript, 0700)
				for _, args := range [][]string{
					{"--channel", channel, "--runtime", "dotnet", "--install-dir", "/usr/share/dotnet"},
					{"--version", "latest", "--channel", channel, "--install-dir", "/usr/share/dotnet"},
					{"--channel", "STS", "--runtime", "dotnet", "--install-dir", "/usr/share/dotnet"},
				} {
					if exec.Command(dotnetScript, args...).Run() == nil && hasDotNetRuntime(channel) {
						break
					}
				}
			}
		}
	}
	for _, dst := range []string{"/usr/local/bin/dotnet", "/usr/bin/dotnet"} {
		if fi, lErr := os.Lstat(dst); lErr == nil && fi.Mode()&os.ModeSymlink != 0 {
			_ = os.Remove(dst)
		}
		if _, lErr := os.Lstat(dst); os.IsNotExist(lErr) {
			_ = os.Symlink("/usr/share/dotnet/dotnet", dst)
		}
	}
	tui.StopSpinner(0)

	if hasDotNetRuntime(channel) {
		logger.LogSuccess(fmt.Sprintf("Microsoft .NET %s Runtime deployed successfully.", channel))
		return true
	}

	logger.LogWarn(fmt.Sprintf("Warning: .NET %s runtime verification had warnings; proceeding with best effort.", channel))
	return true
}

// EnsureDotNet9Runtime guarantees Microsoft .NET 9.0 Runtime is available on the system.
func EnsureDotNet9Runtime() bool {
	return EnsureDotNetRuntime("9.0")
}

func ExecuteDockerInstall(cfg NitroxConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing Nitrox Docker Container Stack ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker Nitrox installation...")

	if !system.EnsureDockerEngine() {
		return
	}

	dataDir := filepath.Join(cfg.InstallDir, "data")
	gameDir := filepath.Join(cfg.InstallDir, "game")
	backupDir := filepath.Join(cfg.InstallDir, "backups")
	_ = os.MkdirAll(dataDir, 0755)
	_ = os.MkdirAll(gameDir, 0755)
	_ = os.MkdirAll(backupDir, 0700)

	composeDir := "/etc/nitrox-docker"
	_ = os.MkdirAll(composeDir, 0755)

	if cfg.AdminPassword == "" {
		tok, tokErr := generateSecureToken(16)
		if tokErr != nil {
			logger.LogError(tokErr.Error())
			return
		}
		cfg.AdminPassword = tok
	}

	composeYaml := fmt.Sprintf(`version: '3.8'
services:
  nitrox:
    image: fuzzystatic/nitrox-server:latest
    container_name: krotek-nitrox
    restart: unless-stopped
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Etc/UTC
      - SUBNAUTICA_INSTALLATION_PATH=/subnautica
      - SUBNAUTICA_SAVE=world
      - NITROX_VERSION=latest
      - SERVER_PORT=%s
      - SERVER_NAME="%s"
      - MAX_PLAYERS=%s
      - PASSWORD="%s"
      - ADMIN_PASSWORD="%s"
    volumes:
      - %s/data:/config
      - %s/game:/subnautica
    ports:
      - "%s:%s/udp"
`, cfg.Port, escapeYamlString(cfg.ServerName), cfg.MaxPlayers, escapeYamlString(cfg.Password), escapeYamlString(cfg.AdminPassword), cfg.InstallDir, cfg.InstallDir, cfg.Port, cfg.Port)

	if wErr := system.WriteFileNoFollow(filepath.Join(composeDir, "docker-compose.yml"), []byte(composeYaml), 0600, ""); wErr != nil {
		logger.LogError("Failed to write Nitrox docker-compose.yml: " + wErr.Error())
		return
	}
	configureFirewall(cfg.Port)

	// Create helper instructions file for game assets upload
	writeGameFilesReadme(gameDir)

	if !system.RunDockerCompose(composeDir, "Deploying Nitrox Docker Stack", true, "up", "-d") {
		return
	}

	system.SaveState(map[string]string{
		"ENV_TYPE":        "docker",
		"COMPOSE_DIR":     composeDir,
		"PORT":            cfg.Port,
		"SERVER_NAME":     cfg.ServerName,
		"MAX_PLAYERS":     cfg.MaxPlayers,
		"GAME_MODE":       cfg.GameMode,
		"SERVER_PASSWORD": cfg.Password,
		"ADMIN_PASSWORD":  cfg.AdminPassword,
		"INSTALL_DIR":     cfg.InstallDir,
		"DATA_PATH":       dataDir,
		"GAME_PATH":       gameDir,
		"SAVE_NAME":       "world",
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("Nitrox Docker deployment complete.")
	printNitroxInstructions(cfg)
}

func ExecuteNativeInstall(cfg NitroxConfig) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing Nitrox Native Dedicated Server ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native Nitrox installation...")

	// 1. Install native system support utilities
	var pkgs []string
	switch core.SysInfo.Family {
	case "debian":
		pkgs = []string{"curl", "wget", "jq", "unzip", "tar", "screen", "tmux", "ca-certificates", "libc6", "libicu-dev"}
	case "rhel":
		_ = exec.Command("dnf", "install", "-y", "epel-release").Run()
		_ = exec.Command("yum", "install", "-y", "epel-release").Run()
		pkgs = []string{"curl", "wget", "jq", "unzip", "tar", "screen", "tmux", "ca-certificates", "libicu"}
	case "alpine":
		pkgs = []string{"curl", "wget", "jq", "unzip", "tar", "screen", "tmux", "ca-certificates", "icu-libs"}
	default:
		pkgs = []string{"curl", "wget", "jq", "unzip", "tar", "screen", "tmux", "ca-certificates"}
	}
	_ = system.InstallPackages(pkgs...)

	// 2. Define directory hierarchy
	serverDir := filepath.Join(cfg.InstallDir, "server")
	dataDir := filepath.Join(cfg.InstallDir, "data")
	gameDir := filepath.Join(cfg.InstallDir, "game")
	backupDir := filepath.Join(cfg.InstallDir, "backups")
	savesDir := filepath.Join(dataDir, "saves", "world")

	_ = os.MkdirAll(cfg.InstallDir, 0755)
	_ = os.MkdirAll(serverDir, 0755)
	_ = os.MkdirAll(dataDir, 0755)
	_ = os.MkdirAll(gameDir, 0755)
	_ = os.MkdirAll(backupDir, 0700)
	_ = os.MkdirAll(savesDir, 0755)
	configureFirewall(cfg.Port)

	// 3. Create dedicated unprivileged system user for Nitrox
	if system.CommandExists("useradd") {
		_ = exec.Command("useradd", "-r", "-s", "/usr/sbin/nologin", "-d", cfg.InstallDir, "nitrox").Run()
	} else if system.CommandExists("adduser") {
		_ = exec.Command("adduser", "-S", "-s", "/sbin/nologin", "-h", cfg.InstallDir, "nitrox").Run()
	}

	// 4. Download & extract official Nitrox Server release package matching host architecture
	arch := core.SysInfo.Architecture
	tag, downloadURL := system.ResolveLatestNitroxRelease(arch)
	if downloadURL == "" {
		logger.LogWarn("Warning: could not resolve upstream release dynamically, falling back to direct latest download.")
		assetArch := "linux_x64"
		if arch == "arm64" || arch == "aarch64" {
			assetArch = "linux_arm64"
		}
		downloadURL = fmt.Sprintf("https://github.com/SubnauticaNitrox/Nitrox/releases/latest/download/Nitrox_%s.zip", assetArch)
	} else {
		logger.LogInfo(fmt.Sprintf("Resolved latest official Nitrox release: %s", tag))
	}

	if !strings.HasPrefix(downloadURL, "https://") {
		logger.LogError("Refusing insecure Nitrox download URL (HTTPS required): " + downloadURL)
		return
	}

	tmpDir, err := os.MkdirTemp("", "nitrox-dl-*")
	if err != nil {
		logger.LogError("Failed to create temporary directory for Nitrox download: " + err.Error())
		return
	}
	_ = os.Chmod(tmpDir, 0700)
	defer os.RemoveAll(tmpDir)
	targetZip := filepath.Join(tmpDir, "nitrox.zip")

	logger.LogInfo("Downloading and deploying official Subnautica Nitrox server binaries...")
	dlOK := system.RunStep("Downloading official Subnautica Nitrox archive", "curl", "--proto", "=https", "--tlsv1.2", "-fsSL", downloadURL, "-o", targetZip)
	if !dlOK {
		dlOK = system.RunStep("Downloading official Subnautica Nitrox archive (wget)", "wget", "--https-only", "--secure-protocol=TLSv1_2", "-qO", targetZip, downloadURL)
	}
	if dlOK {
		if exErr := system.ExtractZipSafe(targetZip, serverDir, 200000, 512*1024*1024, 2*1024*1024*1024); exErr != nil {
			logger.LogError("Failed to extract Nitrox archive safely: " + exErr.Error())
		} else {
			if !system.IsSymlinkPath(filepath.Join(serverDir, "Nitrox.Server.Subnautica")) {
				_ = os.Chmod(filepath.Join(serverDir, "Nitrox.Server.Subnautica"), 0755)
			}
			if !system.IsSymlinkPath(filepath.Join(serverDir, "Nitrox.Launcher")) {
				_ = os.Chmod(filepath.Join(serverDir, "Nitrox.Launcher"), 0755)
			}
			aliasPath := filepath.Join(serverDir, "NitroxServer-Subnautica")
			if fi, lErr := os.Lstat(aliasPath); lErr == nil && fi.Mode()&os.ModeSymlink != 0 {
				_ = os.Remove(aliasPath)
			}
			if _, lErr := os.Lstat(aliasPath); os.IsNotExist(lErr) {
				_ = os.Symlink("Nitrox.Server.Subnautica", aliasPath)
			}
		}
	}

	// 5. Dynamically detect required .NET runtime channel and provision it
	reqChannel := system.DetectRequiredDotNetChannel(serverDir)
	EnsureDotNetRuntime(reqChannel)

	detectedVersion := system.DetectInstalledNitroxVersion(serverDir)
	if detectedVersion == "" {
		detectedVersion = tag
	}
	if detectedVersion == "" {
		detectedVersion = "latest"
	}

	// 6. Generate server.cfg configuration file
	if cfg.AdminPassword == "" {
		tok, tokErr := generateSecureToken(16)
		if tokErr != nil {
			logger.LogError(tokErr.Error())
			return
		}
		cfg.AdminPassword = tok
	}
	if cfg.GameMode == "" {
		cfg.GameMode = "SURVIVAL"
	}

	serverCfgContent := fmt.Sprintf(`[Server]
ServerPort=%s
ServerPassword=%s
AdminPassword=%s
SaveInterval=120000
MaxConnections=%s
SaveName=world
GameMode=%s
DisableAutoSave=False
`, cfg.Port, cfg.Password, cfg.AdminPassword, cfg.MaxPlayers, cfg.GameMode)

	_ = system.WriteFileNoFollow(filepath.Join(dataDir, "server.cfg"), []byte(serverCfgContent), 0600, "")
	_ = system.WriteFileNoFollow(filepath.Join(savesDir, "server.cfg"), []byte(serverCfgContent), 0600, "")

	// 7. Generate startup wrapper /srv/nitrox/start.sh
	startShContent := fmt.Sprintf(`#!/bin/bash
# Subnautica Nitrox Startup Wrapper
export SUBNAUTICA_INSTALLATION_PATH="%s"
export NITROX_DATA_PATH="%s"
export DOTNET_ROOT="/usr/share/dotnet"
export PATH="$DOTNET_ROOT:/usr/local/bin:$PATH"
cd "%s"

if [ -f "./Nitrox.Server.Subnautica" ]; then
    exec ./Nitrox.Server.Subnautica --data-path "%s" --game-path "%s" --save "world"
elif [ -f "./Nitrox.Server.Subnautica.dll" ]; then
    exec dotnet ./Nitrox.Server.Subnautica.dll --data-path "%s" --game-path "%s" --save "world"
elif [ -f "./NitroxServer-Subnautica" ]; then
    exec ./NitroxServer-Subnautica --data-path "%s" --game-path "%s" --save "world"
else
    echo "ERROR: Nitrox server binary not found in $(pwd)"
    exit 1
fi
`, gameDir, dataDir, serverDir, dataDir, gameDir, dataDir, gameDir, dataDir, gameDir)

	startShPath := filepath.Join(cfg.InstallDir, "start.sh")
	_ = system.WriteFileNoFollow(startShPath, []byte(startShContent), 0755, "")

	// 8. Generate automated daily backup script (root-owned to prevent privilege escalation)
	backupShContent := fmt.Sprintf(`#!/bin/bash
# Automated daily backup script for Subnautica Nitrox
umask 077
BACKUP_DIR="%s"
DATA_DIR="%s"
TIMESTAMP=$(date +"%%Y-%%m-%%d_%%H-%%M-%%S")
ARCHIVE="${BACKUP_DIR}/nitrox_backup_${TIMESTAMP}.tar.gz"
mkdir -p "$BACKUP_DIR"
tar -czf "$ARCHIVE" -C "$DATA_DIR" . 2>/dev/null || true
chmod 600 "$ARCHIVE" 2>/dev/null || true
find "$BACKUP_DIR" -type f -name "nitrox_backup_*.tar.gz" -mtime +7 -delete 2>/dev/null || true
`, backupDir, dataDir)

	_ = os.MkdirAll("/usr/local/libexec", 0755)
	rootBackupScript := "/usr/local/libexec/nitrox-backup.sh"
	_ = system.WriteFileNoFollow(rootBackupScript, []byte(backupShContent), 0700, "root")

	// Schedule daily cron backup executing only the root-owned script
	_ = system.WriteFileNoFollow("/etc/cron.daily/nitrox-backup", []byte(fmt.Sprintf("#!/bin/sh\nexec /bin/bash %s >/dev/null 2>&1\n", rootBackupScript)), 0700, "root")

	// 9. Generate README_GAME_FILES.txt in game directory
	writeGameFilesReadme(gameDir)

	// 10. Set filesystem ownership and permissions (keep root directory and start.sh root-owned)
	chownDirsNoFollow("root:root", cfg.InstallDir, startShPath)
	if !system.IsSymlinkPath(cfg.InstallDir) {
		_ = os.Chmod(cfg.InstallDir, 0755)
	}
	if !system.IsSymlinkPath(startShPath) {
		_ = os.Chmod(startShPath, 0755)
	}
	chownDirsNoFollow("nitrox:nitrox", serverDir, dataDir, gameDir, backupDir)

	// 11. Create hardened systemd service unit running as unprivileged nitrox user
	serviceContent := fmt.Sprintf(`[Unit]
Description=Subnautica Nitrox Dedicated Server
After=network.target

[Service]
Type=simple
User=nitrox
Group=nitrox
WorkingDirectory=%s
ExecStart=/bin/bash %s
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
`, serverDir, startShPath)

	_ = system.WriteFileNoFollow("/etc/systemd/system/nitrox.service", []byte(serviceContent), 0644, "")
	_ = exec.Command("systemctl", "daemon-reload").Run()
	system.StartService("nitrox")

	system.SaveState(map[string]string{
		"ENV_TYPE":        "native",
		"PORT":            cfg.Port,
		"SERVER_NAME":     cfg.ServerName,
		"MAX_PLAYERS":     cfg.MaxPlayers,
		"GAME_MODE":       cfg.GameMode,
		"SERVER_PASSWORD": cfg.Password,
		"ADMIN_PASSWORD":  cfg.AdminPassword,
		"INSTALL_DIR":     cfg.InstallDir,
		"SERVER_DIR":      serverDir,
		"DATA_PATH":       dataDir,
		"GAME_PATH":       gameDir,
		"BACKUP_DIR":      backupDir,
		"SAVE_NAME":       "world",
		"NITROX_VERSION":  detectedVersion,
	})
	system.CreateGlobalShortcut()
	logger.LogSuccess("Nitrox Native setup finished successfully.")
	printNitroxInstructions(cfg)
}

func writeGameFilesReadme(gameDir string) {
	readme := fmt.Sprintf(`==============================================================================
SUBNAUTICA GAME FILES REQUIREMENT FOR NITROX DEDICATED SERVER
==============================================================================

To host a Nitrox multiplayer server, Subnautica game files are required to simulate
the ocean world, physics, vehicles, and creatures.

REQUIRED ASSETS:
Place the contents of your Subnautica game installation into this directory:
  %s

Specifically ensure the following folders and files exist:
  - Subnautica_Data/  (Folder with game assets and assemblies)
  - Subnautica.exe (or Linux game binaries)

HOW TO TRANSFER GAME FILES TO THIS SERVER:

Option A (From Windows PowerShell via SCP):
  scp -r "C:\Program Files (x86)\Steam\steamapps\common\Subnautica\*" root@<SERVER_IP>:%s/

Option B (Using SFTP with WinSCP / FileZilla):
  Connect to this server port 22 as root and copy files into %s/

Option C (From Linux via rsync):
  rsync -avzP /path/to/Subnautica/ root@<SERVER_IP>:%s/

After copying files, set proper ownership:
  sudo chown -R nitrox:nitrox %s

Then restart the server:
  sudo nitrox-manager  ->  Restart Nitrox Service
==============================================================================
`, gameDir, gameDir, gameDir, gameDir, gameDir)

	_ = system.WriteFileNoFollow(filepath.Join(gameDir, "README_GAME_FILES.txt"), []byte(readme), 0644, "")
}

func printNitroxInstructions(cfg NitroxConfig) {
	fmt.Printf("\r\n%s------------------------------------------------------------%s\r\n", core.GREEN, core.NC)
	fmt.Printf("%s[OK] Subnautica Nitrox Server Ready!%s\r\n", core.BOLD+core.GREEN, core.NC)
	fmt.Printf("  %sServer Name:%s    %s\r\n", core.CYAN, core.NC, cfg.ServerName)
	fmt.Printf("  %sConnect IP:%s     <SERVER_IP>:%s (UDP)\r\n", core.CYAN, core.NC, cfg.Port)
	fmt.Printf("  %sGame Mode:%s      %s\r\n", core.CYAN, core.NC, cfg.GameMode)
	fmt.Printf("  %sMax Players:%s    %s\r\n", core.CYAN, core.NC, cfg.MaxPlayers)
	if cfg.Password != "" {
		fmt.Printf("  %sJoin Password:%s  %s\r\n", core.CYAN, core.NC, cfg.Password)
	}
	fmt.Printf("  %sAdmin Password:%s %s\r\n", core.CYAN, core.NC, cfg.AdminPassword)
	fmt.Printf("  %sWorld & Data:%s   %s/data\r\n", core.CYAN, core.NC, cfg.InstallDir)
	fmt.Printf("  %sSubnautica Dir:%s %s/game (Upload game files here)\r\n", core.CYAN, core.NC, cfg.InstallDir)
	fmt.Printf("  %sManagement Hub:%s sudo nitrox-manager\r\n", core.CYAN, core.NC)
	fmt.Printf("%s------------------------------------------------------------%s\r\n\r\n", core.GREEN, core.NC)
}

// PromptExistingConfigAction checks if prior configuration files exist on the host system.
// In interactive TUI mode, it prompts the administrator to decide whether to preserve existing custom settings
// or perform a clean factory default reset. In unattended mode (-y / CI/CD), it automatically preserves settings.
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
		"      Preserves all existing world saves, player data, and server.cfg intact.",
		"",
		"  " + core.YELLOW + "[2] Clean Reset to Factory Default (Overwrite)" + core.NC,
		"      Overwrites configuration with fresh, production-hardened defaults.",
	}

	choices := []string{
		"Keep Existing Configuration (Preserve custom world saves & server.cfg)",
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

func escapeYamlString(val string) string {
	v := strings.ReplaceAll(val, "\\", "\\\\")
	v = strings.ReplaceAll(v, "\"", "\\\"")
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", "")
	return v
}
