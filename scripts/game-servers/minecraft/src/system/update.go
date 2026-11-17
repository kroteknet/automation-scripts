// ==============================================================================
// KROTEK SCRIPT HUB — MINECRAFT SERVER SOFTWARE UPDATE & AUDIT ENGINE
// Service: Minecraft Server (game-servers/minecraft)
// File:    game-servers/minecraft/src/system/update.go
//
// Audits upstream PaperMC / Purpur / Vanilla builds and Docker container tags,
// creates pre-update safe backups, seamlessly replaces server.jar with automated rollback.
// ==============================================================================

package system

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"krotek-minecraft-installer/core"
	"krotek-minecraft-installer/tui"
)

// CompatCheckHook is a pluggable function called by RunServiceUpdateFlow to perform
// a plugin / mod compatibility scan before applying an update. It is set by main()
// during startup so that the system package does not need to import modules directly
// (which would create a circular import).
var (
	CompatCheckHook   func(pluginsDir, targetVersion, engine string) bool
	BuildResolverHook func(engineType, mcVersion string) (url string, buildID string, sha256 string, err error)
)

// UpdateCheckResult encapsulates update audit telemetry.
type UpdateCheckResult struct {
	HasUpdate     bool
	Engine        string
	CurrentVer    string
	CandidateVer  string
	CurrentBuild  string
	LatestBuild   string
	DownloadURL   string
	RepoSource    string
	StatusMessage string
}

// isSameMinorVersion returns true when v1 and v2 share the same minor release
// series (e.g. 1.21.1 and 1.21.4 both belong to the 1.21.x series).
func isSameMinorVersion(v1, v2 string) bool {
	return IsSameMinorVersion(v1, v2)
}

// CheckAvailableUpdate audits upstream APIs for available server builds or container updates.
// Build resolution is delegated to BuildResolverHook (registered by main) so every engine
// is handled by the same code path used at install time.
func CheckAvailableUpdate() UpdateCheckResult {
	state := LoadState()
	envType := GetMinecraftEnvType()
	engine := state["ENGINE"]
	if engine == "" {
		engine = "PaperMC"
	}
	engineKey := state["ENGINE_TYPE"]
	if engineKey == "" {
		engineKey = EngineKey(engine)
	}
	mcVer := state["MC_VERSION"]
	if mcVer == "" {
		if latest, err := LatestMinecraftRelease(); err == nil {
			mcVer = latest
		}
	}
	currentBuild := state["BUILD_ID"]

	res := UpdateCheckResult{
		Engine:       engine,
		CurrentVer:   mcVer,
		CurrentBuild: currentBuild,
	}

	if envType == "docker" {
		res.CurrentVer = fmt.Sprintf("%s (%s container)", mcVer, engine)
		res.CandidateVer = "itzg/minecraft-server (latest image)"
		res.RepoSource = "Docker Hub (itzg/minecraft-server)"
		res.StatusMessage = "Container image stack can be refreshed and recreated via docker compose pull"
		res.HasUpdate = true
		return res
	}

	res.RepoSource = fmt.Sprintf("%s Official Build API", engine)

	if engineKey == "neoforge" {
		res.CandidateVer = mcVer
		res.StatusMessage = "NeoForge is installed through its own installer; use the Guided Setup / Reinstallation Wizard to move to a newer build (world data is kept)."
		return res
	}

	if BuildResolverHook != nil && mcVer != "" {
		url, buildID, _, err := BuildResolverHook(engineKey, mcVer)
		if err == nil && url != "" {
			res.DownloadURL = url
			res.LatestBuild = buildID
			res.CandidateVer = mcVer
			if buildID != "" && buildID != mcVer {
				res.CandidateVer = fmt.Sprintf("%s (Build #%s)", mcVer, buildID)
			}
			switch {
			case engineKey == "vanilla":
				res.StatusMessage = fmt.Sprintf("Vanilla Minecraft %s release (official jar is immutable per version).", mcVer)
			case currentBuild == "" || currentBuild != buildID:
				res.HasUpdate = true
				res.StatusMessage = fmt.Sprintf("New %s build #%s available for Minecraft %s", engine, buildID, mcVer)
			default:
				res.StatusMessage = fmt.Sprintf("%s build #%s is already the latest for Minecraft %s.", engine, currentBuild, mcVer)
			}
		} else if err != nil {
			res.StatusMessage = "Upstream build lookup failed: " + err.Error()
		}
	}

	if res.CandidateVer == "" {
		res.CandidateVer = res.CurrentVer
		if res.StatusMessage == "" {
			res.StatusMessage = "Could not reach upstream build API; using installed server.jar version."
		}
	}

	// Surface newer Minecraft releases without silently crossing minor versions.
	if latest, err := LatestMinecraftRelease(); err == nil && mcVer != "" && latest != mcVer &&
		versionGTE(parseVersionPartsCompat(latest), parseVersionPartsCompat(mcVer)) {
		res.StatusMessage += fmt.Sprintf(" Newer Minecraft release %s exists" + " " + core.Sym.EmDash + " " + "use the Guided Setup wizard to upgrade across versions.", latest)
	}

	return res
}

// CheckFreeDiskSpaceMB returns available free disk space in megabytes for /opt or root.
func CheckFreeDiskSpaceMB() int {
	out, err := exec.Command("df", "-m", "/opt").Output()
	if err != nil {
		out, _ = exec.Command("df", "-m", "/").Output()
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) >= 2 {
		fields := strings.Fields(lines[1])
		if len(fields) >= 4 {
			if mb, err := strconv.Atoi(fields[3]); err == nil {
				return mb
			}
		}
	}
	return 1024
}

// RunServiceUpdateFlow runs the interactive update & maintenance wizard for Minecraft Server.
// It now performs a plugin/mod compatibility scan before committing any server.jar swap.
func RunServiceUpdateFlow() {
	serviceTitle := "Minecraft Dedicated Game Server"
	envType := GetMinecraftEnvType()
	serverDir := GetMinecraftDataDir()
	jarPath := filepath.Join(serverDir, "server.jar")
	bakJarPath := filepath.Join(serverDir, "server.jar.bak")

	tui.ClearScreen()
	tui.StartBoxSpinner("Checking for available " + serviceTitle + " upstream updates...")
	audit := CheckAvailableUpdate()
	tui.StopBoxSpinner(0, "Build audit completed.")

	state := LoadState()
	engine := state["ENGINE"]
	if engine == "" {
		engine = "PaperMC"
	}

	// Render Information Card
	var cardLines []string
	statusBadge := core.GREEN + "[UP-TO-DATE]" + core.NC
	if audit.HasUpdate {
		statusBadge = core.YELLOW + core.BOLD + "[UPDATE AVAILABLE]" + core.NC
	}

	currDisplay := audit.CurrentVer
	if audit.CurrentBuild != "" {
		currDisplay = fmt.Sprintf("%s (Build #%s)", audit.CurrentVer, audit.CurrentBuild)
	}

	cardLines = append(cardLines, core.BOLD+core.CYAN+strings.ToUpper(serviceTitle)+" SOFTWARE UPDATE CENTER"+core.NC, "")
	cardLines = append(cardLines, tui.FormatKV("Deployment Mode", strings.ToUpper(envType), core.BOLD+core.WHITE))
	cardLines = append(cardLines, tui.FormatKV("Server Engine", audit.Engine, core.CYAN))
	cardLines = append(cardLines, tui.FormatKV("Current Version", currDisplay, core.YELLOW))
	cardLines = append(cardLines, tui.FormatKV("Candidate Target", audit.CandidateVer, core.GREEN))
	cardLines = append(cardLines, tui.FormatKV("Update Status", statusBadge, core.BOLD))
	cardLines = append(cardLines, tui.FormatKV("Upstream Source", audit.RepoSource, core.DIM))

	// Show version-change severity banner when crossing a minor or major boundary
	if audit.HasUpdate && audit.CandidateVer != "" {
		candidateClean := strings.Fields(audit.CandidateVer)[0] // strip "(Build #NNN)"
		if !IsSameMinorVersion(audit.CurrentVer, candidateClean) {
			cardLines = append(cardLines, "")
			cardLines = append(cardLines, core.YELLOW+core.BOLD+
				"  [VERSION UPGRADE DETECTED] You are upgrading across minor versions."+core.NC)
			cardLines = append(cardLines, core.YELLOW+
				"  Plugins and mods may need to be updated before the server will start."+core.NC)
		} else {
			cardLines = append(cardLines, "")
			cardLines = append(cardLines, core.GREEN+
				"  [PATCH UPDATE] Same minor version series" + " " + core.Sym.EmDash + " " + "plugins should remain compatible."+core.NC)
		}
	}

	cardLines = append(cardLines, "")
	cardLines = append(cardLines, core.DIM+"Status: "+audit.StatusMessage+core.NC)

	tui.PrintInBox(serviceTitle+" Update Audit", cardLines, "--no-wait")

	var options []string
	if envType == "docker" {
		options = append(options,
			"Pull Latest Docker Images & Recreate Container Stack",
			"Back / Cancel",
		)
	} else {
		updateLabel := fmt.Sprintf("Download & Apply Latest %s Build (Hotfixes & Stability)", audit.Engine)
		if audit.LatestBuild != "" {
			updateLabel = fmt.Sprintf("Upgrade %s to Build #%s (Recommended)", audit.Engine, audit.LatestBuild)
		}
		options = append(options, updateLabel)
		if _, err := os.Stat(bakJarPath); err == nil {
			options = append(options, "Rollback to Previous server.jar.bak")
		}
		options = append(options, "Back / Cancel")
	}

	_, opt, st := tui.SelectMenu("Select Update Action", options)
	if st != 0 || strings.Contains(opt, "Back") {
		return
	}

	cleanOpt := core.StripANSI(opt)

	if strings.Contains(cleanOpt, "Rollback") {
		executeJarRollback(jarPath, bakJarPath)
		return
	}

	// --- Plugin / mod compatibility scan ---
	// Only run for native installs; Docker image handles its own dependency chain.
	if envType != "docker" {
		pluginsDir := filepath.Join(serverDir, "plugins")
		modsDir := filepath.Join(serverDir, "mods")

		// Determine which directory to scan based on engine type
		scanDir := pluginsDir
		if strings.EqualFold(engine, "fabric") || strings.EqualFold(engine, "neoforge") {
			scanDir = modsDir
		}

		if entries, err := os.ReadDir(scanDir); err == nil && len(entries) > 0 {
			candidateClean := audit.CandidateVer
			if idx := strings.Index(candidateClean, " "); idx != -1 {
				candidateClean = candidateClean[:idx]
			}
			// Delegate to the compat hook registered by main() to avoid circular imports.
			// The hook runs the full plugin/mod scan and shows the compatibility report TUI.
			// Returns false if the user chose to abort the update.
			if CompatCheckHook != nil {
				if !CompatCheckHook(scanDir, candidateClean, engine) {
					return
				}
			}
		}
	}

	// Safety backup prompt
	if tui.AskYesNo("Create a full world backup before updating? (Recommended)") == 0 {
		tui.ClearScreen()
		freeMB := CheckFreeDiskSpaceMB()
		if freeMB < 200 {
			tui.PrintInBox("Insufficient Disk Space", []string{
				fmt.Sprintf("Available disk space (%d MB) is below the 200 MB safety threshold.", freeMB),
				"Update aborted to prevent disk saturation.",
			})
			return
		}
		tui.StartBoxSpinner("Flushing world chunks and creating backup archive...")
		if _, err := CreateSafeBackup(); err != nil {
			tui.StopBoxSpinner(1, "Backup failed: "+err.Error())
			tui.PrintInBox("Backup Error", []string{core.RED + core.BOLD + "[ERROR] Backup failed: " + err.Error() + core.NC, core.DIM + "Cannot proceed with update without verified backup." + core.NC})
			return
		}
		tui.StopBoxSpinner(0, "Backup created successfully.")
	}

	if envType == "docker" {
		executeDockerUpdate()
		return
	}

	executeNativeJarUpdate(jarPath, bakJarPath, audit)
}

func executeDockerUpdate() {
	composeDir := GetMinecraftComposeDir()
	tui.ClearScreen()
	tui.StartBoxSpinner("Pulling latest container images and updating stack...")
	cmdPull := exec.Command("sh", "-c", fmt.Sprintf("cd %s && (docker compose pull || docker-compose pull)", composeDir))
	_ = cmdPull.Run()

	cmdUp := exec.Command("sh", "-c", fmt.Sprintf("cd %s && (docker compose up -d || docker-compose up -d)", composeDir))
	err := cmdUp.Run()
	tui.StopBoxSpinner(0, "Container stack refreshed.")

	if err != nil {
		tui.PrintInBox("Update Failed", []string{core.RED + "Docker compose update failed." + core.NC})
	} else {
		tui.PrintInBox("Update Complete", []string{
			core.GREEN + "[SUCCESS] Minecraft Docker container stack updated and running!" + core.NC,
		})
	}
}

func executeNativeJarUpdate(jarPath, bakJarPath string, audit UpdateCheckResult) {
	if audit.DownloadURL == "" {
		tui.PrintInBox("Update Error", []string{
			core.RED + "[ERROR] No valid download URL resolved for " + audit.Engine + core.NC,
		})
		return
	}

	tmpJar := jarPath + ".download"
	_ = os.Remove(tmpJar)

	tui.ClearScreen()
	tui.StartBoxSpinner(fmt.Sprintf("Downloading latest %s server.jar...", audit.Engine))

	client := &http.Client{Timeout: 90 * time.Second}
	req, err := http.NewRequest("GET", audit.DownloadURL, nil)
	if err != nil {
		tui.StopBoxSpinner(1, "Download request failed.")
		return
	}
	req.Header.Set("User-Agent", core.UserAgent)

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		tui.StopBoxSpinner(1, "Failed to download server jar.")
		return
	}
	defer resp.Body.Close()

	out, err := os.Create(tmpJar)
	if err != nil {
		tui.StopBoxSpinner(1, "Could not create temporary jar file.")
		return
	}
	_, err = io.Copy(out, resp.Body)
	_ = out.Close()

	if err != nil {
		_ = os.Remove(tmpJar)
		tui.StopBoxSpinner(1, "File stream interrupted during download.")
		return
	}
	tui.StopBoxSpinner(0, "Download completed.")

	// Gracefully stop service
	tui.StartBoxSpinner("Stopping Minecraft service gracefully...")
	_, _ = StopMinecraftService()
	time.Sleep(2 * time.Second)
	tui.StopBoxSpinner(0, "Service stopped.")

	// Backup existing jar
	if _, err := os.Stat(jarPath); err == nil {
		_ = os.Remove(bakJarPath)
		_ = os.Rename(jarPath, bakJarPath)
	}

	// Move new jar into place
	if err := os.Rename(tmpJar, jarPath); err != nil {
		tui.PrintInBox("File Swap Error", []string{
			core.RED + "[ERROR] Failed to replace server.jar: " + err.Error() + core.NC,
		})
		// Attempt restore
		if _, err := os.Stat(bakJarPath); err == nil {
			_ = os.Rename(bakJarPath, jarPath)
		}
		_, _ = StartMinecraftService()
		return
	}

	_ = exec.Command("chown", "minecraft:minecraft", jarPath).Run()
	_ = exec.Command("chmod", "644", jarPath).Run()

	// Restart service
	tui.StartBoxSpinner("Starting Minecraft service with new server.jar...")
	_, _ = StartMinecraftService()

	healthy := WaitForMinecraftHealthy(12)
	tui.StopBoxSpinner(0, "Startup verification complete.")

	if healthy {
		st := LoadState()
		if audit.LatestBuild != "" {
			st["BUILD_ID"] = audit.LatestBuild
		}
		// Persist the new MC_VERSION when an upgrade crosses version boundaries
		if audit.CandidateVer != "" {
			candidateClean := audit.CandidateVer
			if idx := strings.Index(candidateClean, " "); idx != -1 {
				candidateClean = candidateClean[:idx]
			}
			if candidateClean != "" {
				st["MC_VERSION"] = candidateClean
			}
		}
		SaveState(st)

		tui.PrintInBox("Update Succeeded", []string{
			fmt.Sprintf("%s[SUCCESS] Minecraft %s updated successfully!%s", core.GREEN, audit.Engine, core.NC),
			"",
			tui.FormatKV("Active Version", audit.CandidateVer, core.CYAN),
			tui.FormatKV("Service Health", core.GREEN+"[ACTIVE & RUNNING]"+core.NC, core.GREEN),
			tui.FormatKV("Safety Backup", bakJarPath, core.DIM),
		})
	} else {
		tui.PrintInBox("Startup Failed" + " " + core.Sym.EmDash + " " + "Rollback Available", []string{
			core.RED + "[CRITICAL] Minecraft service failed to stay active after jar upgrade!" + core.NC,
			"The new build may have incompatibility with existing Java or plugins.",
		})
		if tui.AskYesNo("Do you want to immediately ROLLBACK to the previous working server.jar?") == 0 {
			executeJarRollback(jarPath, bakJarPath)
		}
	}
}

func executeJarRollback(jarPath, bakJarPath string) {
	if _, err := os.Stat(bakJarPath); os.IsNotExist(err) {
		tui.PrintInBox("Rollback Failed", []string{core.RED + "No backup file found at " + bakJarPath + core.NC})
		return
	}

	tui.StartBoxSpinner("Rolling back to previous server.jar.bak...")
	_, _ = StopMinecraftService()
	time.Sleep(1 * time.Second)

	_ = os.Remove(jarPath)
	_ = os.Rename(bakJarPath, jarPath)
	_ = exec.Command("chown", "minecraft:minecraft", jarPath).Run()

	_, _ = StartMinecraftService()
	healthy := WaitForMinecraftHealthy(10)
	tui.StopBoxSpinner(0, "Rollback finished.")

	if healthy {
		tui.PrintInBox("Rollback Successful", []string{
			core.GREEN + "[SUCCESS] Restored previous server.jar. Service is running normally." + core.NC,
		})
	} else {
		tui.PrintInBox("Rollback Error", []string{
			core.RED + "[WARN] Rolled back jar file, but service status requires manual inspection." + core.NC,
		})
	}
}
