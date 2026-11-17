// ==============================================================================
// KROTEK SCRIPT HUB — DEPLOYMENT PIPELINES (NATIVE SYSTEMD / DOCKER / HEADLESS)
// Service: Minecraft (game-servers/minecraft)
// File:    game-servers/minecraft/src/install/deploy.go
//
// Turns an InstallConfig into a running server. All engine specifics (jar vs. NeoForge
// installer vs. Fabric launcher), Java selection and JVM tuning are derived from the
// configuration rather than hardcoded.
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
	"time"

	"krotek-minecraft-installer/core"
	"krotek-minecraft-installer/logger"
	"krotek-minecraft-installer/system"
	"krotek-minecraft-installer/tui"
)

var safeURLRe = regexp.MustCompile(`^https://[A-Za-z0-9._~:/?#@!$&()*+,;=%\[\]-]+$`)

// aikarFlags returns Aikar's recommended G1GC flag set, switching to the large-heap
// profile for heaps of 12 GB and above, as documented at https://flags.sh.
func aikarFlags(ramMB int, preTouch bool) string {
	newSize, maxNew, region, reserve, ihop := "30", "40", "8M", "20", "15"
	if ramMB >= 12*1024 {
		newSize, maxNew, region, reserve, ihop = "40", "50", "16M", "15", "20"
	}
	pre := ""
	if preTouch {
		pre = " -XX:+AlwaysPreTouch"
	}
	return fmt.Sprintf("-XX:+UseG1GC -XX:+ParallelRefProcEnabled -XX:MaxGCPauseMillis=200 "+
		"-XX:+UnlockExperimentalVMOptions -XX:+DisableExplicitGC%s "+
		"-XX:G1NewSizePercent=%s -XX:G1MaxNewSizePercent=%s -XX:G1HeapRegionSize=%s -XX:G1ReservePercent=%s "+
		"-XX:G1HeapWastePercent=5 -XX:G1MixedGCCountTarget=4 -XX:InitiatingHeapOccupancyPercent=%s "+
		"-XX:G1MixedGCLiveThresholdPercent=90 -XX:G1RSetUpdatingPauseTimePercent=5 -XX:SurvivorRatio=32 "+
		"-XX:+PerfDisableSharedMem -XX:MaxTenuringThreshold=1 "+
		"-Dusing.aikars.flags=https://mcflags.emc.gs -Daikars.new.flags=true",
		pre, newSize, maxNew, region, reserve, ihop)
}

// serverLaunchArgs returns what follows the JVM flags on the java command line.
func serverLaunchArgs(cfg InstallConfig) string {
	if cfg.EngineType == "neoforge" && cfg.LoaderVersion != "" {
		return fmt.Sprintf("@libraries/net/neoforged/neoforge/%s/unix_args.txt nogui", cfg.LoaderVersion)
	}
	return "-jar server.jar nogui"
}

// setProperty updates (or appends) a key in a java .properties file.
func setProperty(path, key, val string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), key+"=") {
			lines[i] = key + "=" + val
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, key+"="+val)
	}
	_ = system.WriteFileNoFollow(path, []byte(strings.Join(lines, "\n")), 0600, "")
}

// ensureCurl installs curl when it is missing (Java may already be present, which skips the package step).
func ensureCurl() {
	if system.CommandExists("curl") {
		return
	}
	_ = system.InstallPackages("curl", "ca-certificates")
}

func quoteSh(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ExecuteNativeInstall deploys the server as a systemd/OpenRC managed service. Returns true on success.
func ExecuteNativeInstall(cfg InstallConfig) bool {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Deploying Minecraft %s Native System Service ===%s\r\n\r\n", core.BOLD+core.GREEN, cfg.Engine, core.NC)
	logger.LogInfo(fmt.Sprintf("Starting Native %s %s installation (RAM: %s, Port: %s)...", cfg.Engine, cfg.Version, cfg.RAM, cfg.Port))

	if !safeURLRe.MatchString(cfg.JarDownloadURL) {
		logger.LogError("Refusing to download from an unexpected or malformed URL: " + cfg.JarDownloadURL)
		tui.EnterAltScreen()
		return false
	}

	// Step 1: Ensure exact compatible Java runtime is installed
	javaBin, ok := EnsureJavaInstalled(cfg.JavaVersion)
	if !ok || javaBin == "" {
		logger.LogError("Could not set up compatible Java runtime. Installation aborted.")
		tui.EnterAltScreen()
		return false
	}
	cfg.JavaBin = javaBin
	ensureCurl()

	// Stop a previously deployed instance so the jar can be replaced safely.
	if _, err := os.Stat(core.ServiceUnitPath); err == nil {
		_, _ = system.StopMinecraftService()
	}

	// Step 2: Create dedicated system user and directories
	if !system.RunMultiStepOrAbort("Creating dedicated '"+core.ServiceUser+"' system user & directories", fmt.Sprintf(`
		id -u %[1]s >/dev/null 2>&1 || useradd -r -m -U -d %[2]s -s /bin/bash %[1]s
		mkdir -p %[2]s/logs %[3]s
		chmod 755 %[2]s
	`, core.ServiceUser, core.NativeServerDir, core.BackupDir)) {
		tui.EnterAltScreen()
		return false
	}
	for _, d := range []string{core.NativeServerDir, core.BackupDir} {
		_ = filepath.WalkDir(d, func(p string, ent os.DirEntry, wErr error) error {
			if wErr != nil || ent.Type()&os.ModeSymlink != 0 {
				return nil
			}
			_ = exec.Command("chown", "-h", core.ServiceUser+":"+core.ServiceUser, p).Run()
			return nil
		})
	}

	// Step 3: Download server jar (or NeoForge installer) with optional SHA-256 verification
	dest := filepath.Join(core.NativeServerDir, "server.jar")
	label := "server jar"
	if cfg.EngineType == "neoforge" {
		dest = filepath.Join(core.NativeServerDir, "neoforge-installer.jar")
		label = "installer"
	}
	dlCmd := fmt.Sprintf("curl --proto '=https' --tlsv1.2 -fSL --retry 3 --connect-timeout 15 -H %s -o %s %s",
		quoteSh("User-Agent: "+core.UserAgent), quoteSh(dest), quoteSh(cfg.JarDownloadURL))
	if cfg.SHA256 != "" && regexp.MustCompile(`^[0-9a-fA-F]{64}$`).MatchString(cfg.SHA256) {
		dlCmd += fmt.Sprintf(" && echo %s | sha256sum -c -", quoteSh(cfg.SHA256+"  "+dest))
	}
	if !system.RunMultiStepOrAbort(fmt.Sprintf("Downloading %s %s %s", cfg.Engine, cfg.Version, label), dlCmd) {
		tui.EnterAltScreen()
		return false
	}

	// Step 3b: NeoForge ships as an installer that must generate the server libraries first
	if cfg.EngineType == "neoforge" {
		installCmd := fmt.Sprintf(`cd %[1]s && %[2]s -jar neoforge-installer.jar --installServer && rm -f neoforge-installer.jar neoforge-installer.jar.log installer.log run.sh run.bat`,
			core.NativeServerDir, quoteSh(cfg.JavaBin))
		if !system.RunMultiStepOrAbort(fmt.Sprintf("Installing NeoForge %s server libraries", cfg.LoaderVersion), installCmd) {
			tui.EnterAltScreen()
			return false
		}
		argsFile := filepath.Join(core.NativeServerDir, "libraries", "net", "neoforged", "neoforge", cfg.LoaderVersion, "unix_args.txt")
		if _, err := os.Stat(argsFile); err != nil {
			logger.LogError("NeoForge installer finished but " + argsFile + " is missing.")
			tui.EnterAltScreen()
			return false
		}
	}

	// Step 4: Write EULA
	_ = system.WriteFileNoFollow(filepath.Join(core.NativeServerDir, "eula.txt"),
		[]byte("# By changing the setting below to TRUE you are indicating your agreement to our EULA (https://aka.ms/MinecraftEULA).\neula=true\n"), 0644, "")

	// Step 5: Default server.properties (never clobber an existing one unless a clean reset was requested)
	propFile := filepath.Join(core.NativeServerDir, "server.properties")
	if _, err := os.Stat(propFile); os.IsNotExist(err) || core.OverwriteExistingConfig {
		motdStr := fmt.Sprintf("\\u00a7b\\u00a7lKrotek \\u00a7fMinecraft Server \\u00a77[\\u00a7a%s\\u00a77]", cfg.Version)
		defaultProps := fmt.Sprintf(`# Minecraft Server Configuration
# Generated by Krotek Script Hub
server-port=%s
server-ip=0.0.0.0
motd=%s
difficulty=easy
gamemode=survival
max-players=20
online-mode=true
enable-command-block=false
spawn-protection=16
view-distance=10
simulation-distance=8
sync-chunk-writes=true
white-list=false
pvp=true
enable-rcon=false
`, cfg.Port, motdStr)
		_ = system.WriteFileNoFollow(propFile, []byte(defaultProps), 0600, "")
	} else {
		setProperty(propFile, "server-port", cfg.Port)
	}

	// Step 6: Ensure non-symlink ownership & sane permissions across the server directory
	_ = filepath.WalkDir(core.NativeServerDir, func(p string, d os.DirEntry, wErr error) error {
		if wErr != nil || d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		_ = exec.Command("chown", "-h", core.ServiceUser+":"+core.ServiceUser, p).Run()
		if d.IsDir() {
			_ = os.Chmod(p, 0755)
		} else if fi, sErr := d.Info(); sErr == nil {
			if fi.Mode()&0111 != 0 {
				_ = os.Chmod(p, 0755)
			} else {
				_ = os.Chmod(p, 0644)
			}
		}
		return nil
	})
	if !system.IsSymlinkPath(propFile) {
		_ = os.Chmod(propFile, 0600)
	}

	// Step 7: JVM memory & Aikar flags (small initial heap avoids early OOM commits on constrained hosts)
	minRAM := "1G"
	if strings.HasPrefix(cfg.RAM, "1G") || strings.HasSuffix(cfg.RAM, "M") {
		minRAM = "512M"
	}
	ramMB := ramToMB(cfg.RAM)
	preTouch := getHostTotalRAMGB() >= 8

	serviceContent := fmt.Sprintf(`[Unit]
Description=Minecraft Java Server (%s %s)
After=network.target

[Service]
User=%s
Group=%s
WorkingDirectory=%s
Environment="HOME=%s"
Type=simple
Restart=on-failure
RestartSec=15s
LimitNOFILE=65536
TimeoutStopSec=60s
NoNewPrivileges=true
PrivateTmp=true
StandardOutput=journal
StandardError=journal
SyslogIdentifier=%s
ExecStart=%s -Xms%s -Xmx%s -Djava.net.preferIPv4Stack=true %s %s
SuccessExitStatus=0 143

[Install]
WantedBy=multi-user.target
`, cfg.Engine, cfg.Version, core.ServiceUser, core.ServiceUser, core.NativeServerDir, core.NativeServerDir,
		core.ServiceName, cfg.JavaBin, minRAM, cfg.RAM, aikarFlags(ramMB, preTouch), serverLaunchArgs(cfg))

	_ = system.WriteFileNoFollow(core.ServiceUnitPath, []byte(serviceContent), 0644, "")

	// Step 8: Open only the game port (Bedrock/BlueMap ports are opened when those addons are installed)
	system.ConfigureFirewallRules(cfg.Port + "/tcp")

	// Step 9: Reload and start service
	if system.IsSystemdRunning() {
		_ = system.RunStep("Reloading systemd manager configuration", "systemctl", "daemon-reload")
		_ = exec.Command("systemctl", "enable", core.ServiceName).Run()
	}
	if !system.StartService(core.ServiceName) {
		_ = system.RestartService(core.ServiceName)
	}

	// Step 10: Verify JVM startup health
	tui.StartSpinner("Verifying Minecraft service startup and JVM health...")
	healthy := false
	for i := 0; i < 15; i++ {
		time.Sleep(1 * time.Second)
		if system.IsSystemdRunning() {
			if out, err := exec.Command("systemctl", "is-active", core.ServiceName).Output(); err == nil && strings.TrimSpace(string(out)) == "active" {
				healthy = true
				break
			}
			if out, err := exec.Command("systemctl", "is-failed", core.ServiceName).Output(); err == nil && strings.TrimSpace(string(out)) == "failed" {
				break
			}
		} else if exec.Command("pgrep", "-f", "server.jar|unix_args.txt").Run() == nil || exec.Command("service", core.ServiceName, "status").Run() == nil {
			healthy = true
			break
		}
	}
	tui.StopSpinner(0)

	if !healthy {
		out, _ := exec.Command("journalctl", "-u", core.ServiceName, "-n", "30", "--no-pager").CombinedOutput()
		logger.LogError("Minecraft service failed to start or keep running!")
		tui.EnterAltScreen()
		tui.PrintInBox("Service Startup Failure (journalctl -u "+core.ServiceName+")", tui.FormatStyledLogLines(string(out), "No startup journal logs available."))
		return false
	}

	system.SaveState(map[string]string{
		"ENV_TYPE":       "native",
		"ENGINE":         cfg.Engine,
		"ENGINE_TYPE":    cfg.EngineType,
		"MC_VERSION":     cfg.Version,
		"BUILD_ID":       cfg.BuildID,
		"LOADER_VERSION": cfg.LoaderVersion,
		"ALLOCATED_RAM":  cfg.RAM,
		"SERVER_PORT":    cfg.Port,
		"SERVER_DIR":     core.NativeServerDir,
		"JAVA_VERSION":   strconv.Itoa(cfg.JavaVersion),
		"JAVA_BIN":       cfg.JavaBin,
	})
	system.CreateGlobalShortcut()

	logger.LogSuccess(fmt.Sprintf("Minecraft %s %s Native Service deployed successfully!", cfg.Engine, cfg.Version))
	tui.EnterAltScreen()

	serverIP := system.GetLocalIP()
	lines := []string{
		core.GREEN + "[SUCCESS] Minecraft Java Server is ACTIVE and running!" + core.NC,
		"",
		tui.FormatKV("Engine & Version", fmt.Sprintf("%s %s", cfg.Engine, cfg.Version), core.CYAN),
		tui.FormatKV("Java Runtime", cfg.JavaBin, core.CYAN),
		tui.FormatKV("Server Address", fmt.Sprintf("%s:%s", serverIP, cfg.Port), core.YELLOW),
		tui.FormatKV("Allocated Memory", fmt.Sprintf("-Xms%s -Xmx%s", minRAM, cfg.RAM), core.CYAN),
		tui.FormatKV("Server Directory", core.NativeServerDir, core.CYAN),
		tui.FormatKV("Systemd Service", "systemctl status "+core.ServiceName, core.CYAN),
		"",
		core.BOLD + "Join in Minecraft Client:" + core.NC,
		fmt.Sprintf("  Multiplayer" + " " + core.Sym.ArrowRight + " " + "Direct Connection" + " " + core.Sym.ArrowRight + " " + "%s:%s", serverIP, cfg.Port),
	}
	tui.PrintInBox("Minecraft Server Ready", lines)
	return true
}

// dockerImageTag selects the itzg image tag matching the Java line the Minecraft version needs.
// Legacy versions need an older JVM; current ones use the image's rolling default.
func dockerImageTag(javaVersion int) string {
	switch javaVersion {
	case 8, 11, 17:
		return fmt.Sprintf("java%d", javaVersion)
	}
	return "latest"
}

// ExecuteDockerInstall deploys the server as a docker compose stack. Returns true on success.
func ExecuteDockerInstall(cfg InstallConfig) bool {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Deploying Minecraft %s Docker Stack ===%s\r\n\r\n", core.BOLD+core.GREEN, cfg.Engine, core.NC)
	logger.LogInfo(fmt.Sprintf("Starting Docker %s %s installation (RAM: %s, Port: %s)...", cfg.Engine, cfg.Version, cfg.RAM, cfg.Port))

	if !system.EnsureDockerEngine() {
		return false
	}

	_ = os.MkdirAll(core.DockerComposeDir, 0755)
	_ = os.MkdirAll(core.DockerDataDir, 0755)
	_ = os.MkdirAll(core.BackupDir, 0700)

	dockerType := map[string]string{
		"paper": "PAPER", "purpur": "PURPUR", "vanilla": "VANILLA", "fabric": "FABRIC", "neoforge": "NEOFORGE",
	}[cfg.EngineType]
	if dockerType == "" {
		dockerType = "PAPER"
	}

	composeYaml := fmt.Sprintf(`services:
  minecraft:
    image: itzg/minecraft-server:%s
    container_name: %s
    restart: unless-stopped
    tty: true
    stdin_open: true
    ports:
      - "%s:25565"
    environment:
      EULA: "TRUE"
      TYPE: "%s"
      VERSION: "%s"
      MEMORY: "%s"
      ENABLE_AUTOSTOP: "FALSE"
    volumes:
      - %s:/data
`, dockerImageTag(cfg.JavaVersion), core.DockerContainerName, cfg.Port, dockerType, cfg.Version, cfg.RAM, core.DockerDataDir)

	_ = system.WriteFileNoFollow(filepath.Join(core.DockerComposeDir, "docker-compose.yml"), []byte(composeYaml), 0600, "")

	// Universal firewall hardening for the game port only (Bedrock port opens with the Geyser addon)
	system.ConfigureFirewallRules(cfg.Port + "/tcp")

	if !system.RunDockerCompose(core.DockerComposeDir, "Starting Minecraft Docker Container", true, "up", "-d") {
		tui.EnterAltScreen()
		return false
	}

	system.SaveState(map[string]string{
		"ENV_TYPE":      "docker",
		"ENGINE":        cfg.Engine,
		"ENGINE_TYPE":   cfg.EngineType,
		"MC_VERSION":    cfg.Version,
		"ALLOCATED_RAM": cfg.RAM,
		"SERVER_PORT":   cfg.Port,
		"COMPOSE_DIR":   core.DockerComposeDir,
		"DATA_DIR":      core.DockerDataDir,
	})
	system.CreateGlobalShortcut()

	logger.LogSuccess(fmt.Sprintf("Minecraft %s %s Docker container deployed successfully!", cfg.Engine, cfg.Version))
	tui.EnterAltScreen()

	serverIP := system.GetLocalIP()
	lines := []string{
		core.GREEN + "[SUCCESS] Minecraft Docker Container is running!" + core.NC,
		"",
		tui.FormatKV("Engine & Version", fmt.Sprintf("%s %s", cfg.Engine, cfg.Version), core.CYAN),
		tui.FormatKV("Server Address", fmt.Sprintf("%s:%s", serverIP, cfg.Port), core.YELLOW),
		tui.FormatKV("Allocated Memory", cfg.RAM, core.CYAN),
		tui.FormatKV("Data Directory", core.DockerDataDir, core.CYAN),
		tui.FormatKV("Container Name", core.DockerContainerName, core.CYAN),
		"",
		core.BOLD + "Join in Minecraft Client:" + core.NC,
		fmt.Sprintf("  Multiplayer" + " " + core.Sym.ArrowRight + " " + "Direct Connection" + " " + core.Sym.ArrowRight + " " + "%s:%s", serverIP, cfg.Port),
	}
	tui.PrintInBox("Minecraft Container Ready", lines)
	return true
}

func runDeploy(cfg InstallConfig) bool {
	if cfg.Mode == "docker" {
		return ExecuteDockerInstall(cfg)
	}
	return ExecuteNativeInstall(cfg)
}

// buildConfig assembles a complete InstallConfig for an engine at a given Minecraft version.
func buildConfig(engineType, mcVersion, ram, port, mode string, build ResolvedBuild) InstallConfig {
	return InstallConfig{
		Engine:         EngineDisplayName(engineType),
		EngineType:     engineType,
		Version:        mcVersion,
		JarDownloadURL: build.URL,
		BuildID:        build.BuildID,
		LoaderVersion:  build.LoaderVersion,
		SHA256:         build.SHA256,
		JavaVersion:    DetermineRequiredJava(mcVersion),
		RAM:            ram,
		Port:           port,
		Mode:           mode,
	}
}

// StartGuidedInstall runs the interactive installation wizard.
func StartGuidedInstall(preferredMode string) {
	PromptExistingConfigAction("Minecraft Game Server", filepath.Join(core.NativeServerDir, "server.properties"))

	mode := preferredMode
	if mode == "" {
		modeOpts := []string{
			"Native Linux System Service (Recommended - Direct Hardware Performance)",
			"Docker Containerized Deployment (Isolated Stack)",
			"Cancel / Back",
		}
		idx, opt, st := tui.SelectMenu("KROTEK MINECRAFT" + " " + core.Sym.EmDash + " " + "Deployment Mode Selection", modeOpts)
		if st == 2 || strings.Contains(opt, "Cancel") {
			return
		}
		if idx == 0 {
			mode = "native"
		} else {
			mode = "docker"
		}
	}

	// --- Express Install shortcut ---
	expressOpts := []string{
		"Express Install (PaperMC - latest stable - auto RAM - default port)",
		"Custom Install (choose engine, version, RAM, and port manually)",
		"Cancel / Back",
	}
	_, eOpt, eSt := tui.SelectMenu("KROTEK MINECRAFT" + " " + core.Sym.EmDash + " " + "Installation Mode", expressOpts)
	if eSt == 2 || strings.Contains(eOpt, "Cancel") {
		return
	}

	if strings.Contains(core.StripANSI(eOpt), "Express Install") {
		tui.StartSpinner("Resolving latest stable PaperMC release...")
		latest, err := LatestEngineVersion("paper")
		var build ResolvedBuild
		if err == nil {
			build, err = ResolveBuild("paper", latest)
		}
		tui.StopSpinner(0)
		if err != nil {
			logger.LogError("Express install failed: " + err.Error())
			tui.PrintInBox("Express Install Failed", []string{
				core.RED + "Could not resolve the latest PaperMC build: " + err.Error() + core.NC,
				"Check the server's internet connection or use Custom Install.",
			})
			return
		}
		runDeploy(buildConfig("paper", latest, AutoRAM(), core.DefaultServerPort, mode, build))
		return
	}

	// --- Custom install: engine selection ---
	engineOpts := []string{
		"PaperMC (Recommended - Best Performance, Bug Fixes & Plugins)",
		"Fabric (Mods - Create, Alex's Mods, Lithium, Spark)",
		"NeoForge (Mods - Forge-compatible mod ecosystem)",
		"Purpur (Feature-Rich - Highly Configurable Paper Fork)",
		"Vanilla Mojang (Official Pure Minecraft Server)",
		"Cancel / Back",
	}
	eIdx, eEngineOpt, eEngSt := tui.SelectMenu("KROTEK MINECRAFT" + " " + core.Sym.EmDash + " " + "Server Engine Selection", engineOpts)
	if eEngSt == 2 || strings.Contains(eEngineOpt, "Cancel") {
		return
	}
	engineType := []string{"paper", "fabric", "neoforge", "purpur", "vanilla"}[eIdx%5]
	engineName := EngineDisplayName(engineType)

	version, build, vSt := SelectMinecraftVersion(engineName, engineType)
	if vSt != 0 || version == "" {
		return
	}

	ram, rSt := SelectRAM()
	if rSt != 0 || ram == "" {
		return
	}

	port, pSt := SelectPort()
	if pSt != 0 || port == "" {
		return
	}

	cfg := buildConfig(engineType, version, ram, port, mode, build)
	summary := []string{
		core.BOLD + core.YELLOW + "=== INSTALLATION CONFIGURATION SUMMARY ===" + core.NC,
		"",
		tui.FormatKV("Deployment Mode", strings.ToUpper(mode), core.CYAN),
		tui.FormatKV("Server Engine", engineName, core.CYAN),
		tui.FormatKV("Minecraft Version", version, core.YELLOW),
		tui.FormatKV("Required Java", fmt.Sprintf("Java %d OpenJDK", cfg.JavaVersion), core.CYAN),
		tui.FormatKV("Memory (RAM)", ram, core.CYAN),
		tui.FormatKV("Server Port", port+" TCP", core.CYAN),
		"",
		core.DIM + "Mojang EULA will be automatically agreed (eula=true)." + core.NC,
		"",
		core.BOLD + core.YELLOW + "Do you want to proceed with installation?" + core.NC,
	}

	if !core.AutoYes {
		tui.ClearScreen()
		idx, _, status := tui.SelectMenuWithContent("Step 4/4: Installation Summary", summary, []string{"YES", "NO"})
		if status == 2 || idx != 0 {
			logger.LogInfo("Installation aborted by user.")
			return
		}
	}

	runDeploy(cfg)
}

// RunHeadlessInstall performs an unattended installation driven purely by environment
// variables (set from CLI flags in main.go). Returns false when the deployment failed.
//
//	KROTEK_ENGINE      paper | purpur | vanilla | fabric | neoforge   (default: paper)
//	KROTEK_MC_VERSION  Minecraft version                               (default: latest for the engine)
//	KROTEK_RAM         8G / 4096M / 8                                  (default: derived from host RAM)
//	KROTEK_PORT        server port                                     (default: 25565)
func RunHeadlessInstall(mode string) bool {
	engineType := strings.ToLower(strings.TrimSpace(os.Getenv("KROTEK_ENGINE")))
	if engineType == "" || engineType == "papermc" {
		engineType = "paper"
	}
	switch engineType {
	case "paper", "purpur", "vanilla", "fabric", "neoforge":
	default:
		logger.LogError("Unknown KROTEK_ENGINE '" + engineType + "'. Allowed: paper, purpur, vanilla, fabric, neoforge.")
		return false
	}

	mcVersion := strings.TrimSpace(os.Getenv("KROTEK_MC_VERSION"))
	if mcVersion == "" {
		logger.LogInfo("Resolving latest " + EngineDisplayName(engineType) + " version...")
		latest, err := LatestEngineVersion(engineType)
		if err != nil {
			logger.LogError("Could not resolve the latest version: " + err.Error() + " (set KROTEK_MC_VERSION to continue offline-resolvable)")
			return false
		}
		mcVersion = latest
	}

	build, err := ResolveBuild(engineType, mcVersion)
	if err != nil {
		logger.LogError(err.Error())
		return false
	}

	ram := AutoRAM()
	if raw := os.Getenv("KROTEK_RAM"); raw != "" {
		n, ok := NormalizeRAM(raw)
		if !ok {
			logger.LogError("Invalid KROTEK_RAM '" + raw + "'. Use e.g. 4G, 8G or 4096M.")
			return false
		}
		ram = n
	}

	port := core.DefaultServerPort
	if raw := strings.TrimSpace(os.Getenv("KROTEK_PORT")); raw != "" {
		port = raw
	}

	if mode == "" {
		mode = "native"
	}
	return runDeploy(buildConfig(engineType, mcVersion, ram, port, mode, build))
}
