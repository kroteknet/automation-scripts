// ==============================================================================
// KROTEK SCRIPT HUB — FABRIC MOD MANAGER
// Service: MINECRAFT (game-servers)
// File:    game-servers/minecraft/src/modules/mods.go
//
// Manages Fabric and NeoForge mod archives stored in the server /mods/ directory.
// Reads fabric.mod.json metadata directly from .jar archives (ZIP inspection),
// provides 1-click popular mod downloads from Modrinth, and integrates with the
// compatibility engine before server version upgrades.
// ==============================================================================

package modules

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"krotek-minecraft-installer/core"
	"krotek-minecraft-installer/system"
	"krotek-minecraft-installer/tui"
)

// ModItem represents a single installed Fabric/NeoForge mod file.
type ModItem struct {
	Name     string
	ModID    string
	Filename string
	Path     string
	Enabled  bool
	Version  string
	SizeKB   int64
}

// PopularMod describes a well-known mod available for 1-click download.
type PopularMod struct {
	ID          string
	Name        string
	Category    string
	Description string
	ModrinthID  string // Modrinth project slug or ID
}

var popularModCatalog = []PopularMod{
	{
		ID:          "fabric-api",
		Name:        "Fabric API (Required Base Library)",
		Category:    "Core / API",
		Description: "Essential API library required by almost all Fabric mods. Must be installed first.",
		ModrinthID:  "fabric-api",
	},
	{
		ID:          "create",
		Name:        "Create (Automation & Machinery)",
		Category:    "Gameplay",
		Description: "Adds rotational mechanics, conveyor belts, automated crafting, and complex contraptions.",
		ModrinthID:  "create-fabric",
	},
	{
		ID:          "lithium",
		Name:        "Lithium (Server Performance)",
		Category:    "Performance",
		Description: "No-compromises game logic optimisation mod. Significantly reduces server tick time.",
		ModrinthID:  "lithium",
	},
	{
		ID:          "spark",
		Name:        "Spark (Profiler)",
		Category:    "Performance / Admin",
		Description: "In-game performance profiler. Identifies lag spikes, high-CPU plugins, and memory leaks.",
		ModrinthID:  "spark",
	},
	{
		ID:          "worldedit-fabric",
		Name:        "WorldEdit Fabric (In-Game Editor)",
		Category:    "Administration",
		Description: "Industry-standard in-game world editor and terraforming tool for server operators.",
		ModrinthID:  "worldedit",
	},
	{
		ID:          "luckperms-fabric",
		Name:        "LuckPerms Fabric (Permissions)",
		Category:    "Administration",
		Description: "Advanced permissions plugin for Fabric servers with web editor GUI.",
		ModrinthID:  "luckperms",
	},
	{
		ID:          "bluemap-fabric",
		Name:        "BlueMap Fabric (3D Web Map)",
		Category:    "Web Map",
		Description: "Generates an interactive 3D live map of your world accessible in a web browser.",
		ModrinthID:  "bluemap",
	},
	{
		ID:          "ftb-chunks",
		Name:        "FTB Chunks (Chunk Claiming & Protection)",
		Category:    "Protection",
		Description: "Allows players to claim chunks and protect their builds from griefing.",
		ModrinthID:  "ftb-chunks",
	},
}

// getModsDir returns the absolute path to the server's /mods/ directory,
// creating it if it does not already exist.
func getModsDir() string {
	serverDir := getServerDataDir()
	modsDir := filepath.Join(serverDir, "mods")
	_ = os.MkdirAll(modsDir, 0755)
	return modsDir
}

// GetInstalledMods scans the mods directory and returns metadata for every
// .jar (enabled) and .jar.disabled (disabled) mod file found.
func GetInstalledMods() []ModItem {
	mDir := getModsDir()
	var list []ModItem

	entries, err := os.ReadDir(mDir)
	if err != nil {
		return list
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fname := e.Name()
		fullPath := filepath.Join(mDir, fname)

		enabled := false
		if strings.HasSuffix(fname, ".jar") {
			enabled = true
		} else if !strings.HasSuffix(fname, ".jar.disabled") {
			continue
		}

		fi, _ := e.Info()
		sz := int64(0)
		if fi != nil {
			sz = fi.Size() / 1024
		}

		meta := system.ReadFabricModJSON(fullPath)
		displayName := meta.ID
		if displayName == "" {
			displayName = strings.TrimSuffix(strings.TrimSuffix(fname, ".jar.disabled"), ".jar")
		}

		list = append(list, ModItem{
			Name:     displayName,
			ModID:    meta.ID,
			Version:  meta.Version,
			Filename: fname,
			Path:     fullPath,
			Enabled:  enabled,
			SizeKB:   sz,
		})
	}

	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
	return list
}

// fetchModrinthLatestJarForLoader retrieves the download URL for the latest
// version of a Modrinth project that targets the given Minecraft version and
// loader (fabric/neoforge).
func fetchModrinthLatestJarForLoader(projectID, mcVersion, loader string) (string, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	apiURL := fmt.Sprintf(
		"https://api.modrinth.com/v2/project/%s/version?game_versions=%s&loaders=%s",
		url.PathEscape(projectID),
		url.QueryEscape(fmt.Sprintf(`["%s"]`, mcVersion)),
		url.QueryEscape(fmt.Sprintf(`["%s"]`, loader)),
	)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", core.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Modrinth API returned HTTP %d for project %s", resp.StatusCode, projectID)
	}

	var versions []struct {
		Files []struct {
			URL      string `json:"url"`
			Filename string `json:"filename"`
			Primary  bool   `json:"primary"`
		} `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4*1024*1024)).Decode(&versions); err != nil {
		return "", err
	}

	for _, v := range versions {
		for _, f := range v.Files {
			if strings.HasSuffix(f.Filename, ".jar") && strings.HasPrefix(f.URL, "https://") && (f.Primary || len(v.Files) == 1) {
				return f.URL, nil
			}
		}
		// Fallback: return first .jar regardless of primary flag
		for _, f := range v.Files {
			if strings.HasSuffix(f.Filename, ".jar") && strings.HasPrefix(f.URL, "https://") {
				return f.URL, nil
			}
		}
	}

	return "", fmt.Errorf("no .jar file found on Modrinth for %s targeting %s", projectID, mcVersion)
}

// ManageModsMenu provides the interactive Fabric / NeoForge mod management panel.
// It mirrors the structure of ManagePluginsMenu but targets the /mods/ directory.
func ManageModsMenu() {
	state := system.LoadState()
	mcVersion := state["MC_VERSION"]
	if mcVersion == "" {
		if latest, err := system.LatestMinecraftRelease(); err == nil {
			mcVersion = latest
		}
	}
	engineName := state["ENGINE"]
	if engineName == "" {
		engineName = "Fabric"
	}
	loader := "fabric"
	if strings.Contains(strings.ToLower(engineName), "neoforge") {
		loader = "neoforge"
	}

	for {
		mods := GetInstalledMods()

		var menuOpts []string
		menuOpts = append(menuOpts, "--- QUICK INSTALLERS & REPOSITORY ---")
		menuOpts = append(menuOpts, "[+] 1-Click Install Popular Mods (Modrinth)")
		menuOpts = append(menuOpts, "[>] Download Custom Mod from Direct URL (.jar)")
		menuOpts = append(menuOpts, "--- INSTALLED MODS ---")

		if len(mods) == 0 {
			menuOpts = append(menuOpts, "  (No mods currently installed in the /mods/ directory)")
		} else {
			for i, m := range mods {
				badge := core.GREEN + "[ENABLED] " + core.NC
				if !m.Enabled {
					badge = core.YELLOW + "[DISABLED]" + core.NC
				}
				verStr := ""
				if m.Version != "" {
					verStr = fmt.Sprintf(" v%s", m.Version)
				}
				sizeMB := float64(m.SizeKB) / 1024.0
				menuOpts = append(menuOpts, fmt.Sprintf("[%02d] %-28s %s%s (%.1f MB)", i+1, m.Name+verStr, badge, "", sizeMB))
			}
		}

		menuOpts = append(menuOpts, "--- NAVIGATION ---")
		menuOpts = append(menuOpts, "Back to Main Menu")

		title := fmt.Sprintf("KROTEK MINECRAFT" + " " + core.Sym.EmDash + " " + "Mod Manager [%s] (%d installed)", engineName, len(mods))
		idx, opt, st := tui.SelectMenu(title, menuOpts)
		if st == 2 || strings.Contains(opt, "Back to Main Menu") {
			return
		}

		cleanOpt := core.StripANSI(opt)
		if strings.Contains(cleanOpt, "1-Click Install Popular Mods") {
			showPopularModsMenu(mcVersion, loader)
			continue
		}
		if strings.Contains(cleanOpt, "Download Custom Mod from Direct URL") {
			downloadCustomMod()
			continue
		}

		// Mod item selected
		for _, m := range mods {
			if strings.Contains(cleanOpt, m.Name) {
				handleModAction(m)
				break
			}
		}
		_ = idx
	}
}

// handleModAction presents enable/disable/delete options for a single mod.
func handleModAction(m ModItem) {
	statusStr := "Enabled"
	toggleLabel := "Disable Mod"
	if !m.Enabled {
		statusStr = "Disabled"
		toggleLabel = "Enable Mod"
	}

	subOpts := []string{
		fmt.Sprintf("%s%s%s (%s -> %s)", core.YELLOW, toggleLabel, core.NC, statusStr, func() string {
			if m.Enabled {
				return "Disabled"
			}
			return "Enabled"
		}()),
		core.RED + "Delete Mod (.jar file)" + core.NC,
		"Back to Mods List",
	}

	title := fmt.Sprintf("Manage: %s (%s)", m.Filename, statusStr)
	_, opt, st := tui.SelectMenu(title, subOpts)
	if st != 0 || strings.Contains(opt, "Back") {
		return
	}

	cleanOpt := core.StripANSI(opt)
	mDir := getModsDir()

	if strings.HasPrefix(cleanOpt, "Disable Mod") {
		newPath := filepath.Join(mDir, m.Name+".jar.disabled")
		_ = os.Rename(m.Path, newPath)
		tui.PrintInBox("Mod Disabled", []string{
			fmt.Sprintf("%s[OK] '%s' is now DISABLED.%s", core.YELLOW, m.Name, core.NC),
			"Restart the Minecraft server for changes to take effect.",
		})
	} else if strings.HasPrefix(cleanOpt, "Enable Mod") {
		newPath := filepath.Join(mDir, m.Name+".jar")
		_ = os.Rename(m.Path, newPath)
		tui.PrintInBox("Mod Enabled", []string{
			fmt.Sprintf("%s[SUCCESS] '%s' is now ENABLED.%s", core.GREEN, m.Name, core.NC),
			"Restart the Minecraft server for changes to take effect.",
		})
	} else if strings.HasPrefix(cleanOpt, "Delete Mod") {
		confirm := tui.AskYesNo(fmt.Sprintf("Permanently delete %s?", m.Filename))
		if confirm == 0 {
			_ = os.Remove(m.Path)
			tui.PrintInBox("Mod Deleted", []string{
				fmt.Sprintf("%s[OK] Removed '%s' from mods folder.%s", core.GREEN, m.Filename, core.NC),
			})
		}
	}
}

// showPopularModsMenu lists popular mods available via Modrinth for 1-click install.
func showPopularModsMenu(mcVersion, loader string) {
	installed := GetInstalledMods()
	installedMap := make(map[string]bool)
	for _, m := range installed {
		installedMap[strings.ToLower(m.ModID)] = true
		installedMap[strings.ToLower(m.Name)] = true
	}

	var opts []string
	for i, pm := range popularModCatalog {
		status := core.YELLOW + "[INSTALL]  " + core.NC
		if installedMap[strings.ToLower(pm.ID)] || installedMap[strings.ToLower(pm.ModrinthID)] {
			status = core.GREEN + "[INSTALLED]" + core.NC
		}
		opts = append(opts, fmt.Sprintf("[%02d] %-40s %s  (%s)", i+1, pm.Name, status, pm.Category))
	}
	opts = append(opts, "Back to Mod Manager")

	idx, opt, st := tui.SelectMenu("KROTEK MINECRAFT" + " " + core.Sym.EmDash + " " + "1-Click Mod Repository (Modrinth)", opts)
	if st != 0 || strings.Contains(opt, "Back") || idx >= len(popularModCatalog) {
		return
	}

	selected := popularModCatalog[idx]
	installPopularMod(selected, mcVersion, loader)
}

// installPopularMod downloads a mod from Modrinth for the active server version.
func installPopularMod(pm PopularMod, mcVersion, loader string) {
	mDir := getModsDir()

	tui.StartSpinner(fmt.Sprintf("Resolving latest %s build for Minecraft %s...", pm.Name, mcVersion))
	dlURL, err := fetchModrinthLatestJarForLoader(pm.ModrinthID, mcVersion, loader)
	tui.StopSpinner(0)

	if err != nil || dlURL == "" {
		tui.PrintInBox("Download Failed", []string{
			core.RED + "[ERROR] Could not resolve download URL for " + pm.Name + core.NC,
			fmt.Sprintf("Modrinth slug: %s  |  Minecraft: %s  |  Loader: %s", pm.ModrinthID, mcVersion, loader),
			"",
			"The mod may not yet support this Minecraft version.",
			"Try downloading it manually from https://modrinth.com/mod/" + pm.ModrinthID,
		})
		return
	}

	// Derive filename from URL
	fname := filepath.Base(strings.Split(dlURL, "?")[0])
	if !strings.HasSuffix(fname, ".jar") {
		fname = pm.ID + ".jar"
	}
	destPath := filepath.Join(mDir, fname)

	dlErr := downloadFileWithSpinner(dlURL, destPath, fmt.Sprintf("Downloading %s...", pm.Name))
	if dlErr != nil {
		tui.PrintInBox("Download Error", []string{
			core.RED + "[ERROR] Failed to download " + pm.Name + core.NC,
			dlErr.Error(),
		})
		return
	}

	_ = exec.Command("chown", "-h", "minecraft:minecraft", destPath).Run()
	chownDirNoFollow(mDir, "minecraft:minecraft")

	tui.PrintInBox("Mod Installed", []string{
		fmt.Sprintf("%s[SUCCESS] %s installed successfully!%s", core.GREEN, pm.Name, core.NC),
		"",
		tui.FormatKV("Mod File", destPath, core.CYAN),
		tui.FormatKV("Minecraft Version", mcVersion, core.YELLOW),
		"",
		"Restart the Minecraft server to load the new mod.",
	})

	if tui.AskYesNo("Restart Minecraft server now to load the mod?") == 0 {
		_, _ = system.RestartMinecraftService()
	}
}

func chownDirNoFollow(dir, owner string) {
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		_ = exec.Command("chown", "-h", owner, p).Run()
		return nil
	})
}
func downloadCustomMod() {
	url, st := tui.ReadTextInput("Enter direct URL to mod .jar file", "https://")
	url = strings.TrimSpace(url)
	if st != 0 || url == "" || url == "https://" || !strings.HasPrefix(url, "https://") {
		if st == 0 && url != "" && url != "https://" && !strings.HasPrefix(url, "https://") {
			tui.PrintInBox("Security Error", []string{
				core.RED + "[ERROR] Only HTTPS download URLs (https://...) are permitted." + core.NC,
			})
		}
		return
	}

	fname := filepath.Base(strings.Split(url, "?")[0])
	if !strings.HasSuffix(fname, ".jar") {
		fname += ".jar"
	}

	customName, cSt := tui.ReadTextInput("Enter filename to save as", fname)
	if cSt == 0 && strings.TrimSpace(customName) != "" {
		fname = filepath.Base(strings.TrimSpace(customName))
	}

	fname = strings.ReplaceAll(fname, "..", "")
	fname = strings.ReplaceAll(fname, "/", "")
	fname = strings.ReplaceAll(fname, "\\", "")
	if !strings.HasSuffix(fname, ".jar") {
		fname += ".jar"
	}

	mDir := getModsDir()
	destPath := filepath.Join(mDir, fname)

	// Path traversal guard
	if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(mDir)+string(filepath.Separator)) {
		tui.PrintInBox("Security Error", []string{
			core.RED + "[CRITICAL] Invalid filename" + " " + core.Sym.EmDash + " " + "path traversal detected." + core.NC,
			"Mod file must be saved inside the server mods directory.",
		})
		return
	}

	dlErr := downloadFileWithSpinner(url, destPath, fmt.Sprintf("Downloading %s...", fname))
	if dlErr != nil {
		tui.PrintInBox("Download Error", []string{
			core.RED + "[ERROR] Failed to download mod." + core.NC,
			dlErr.Error(),
		})
		return
	}

	_ = exec.Command("chown", "-h", "minecraft:minecraft", destPath).Run()
	chownDirNoFollow(mDir, "minecraft:minecraft")

	tui.PrintInBox("Custom Mod Downloaded", []string{
		fmt.Sprintf("%s[SUCCESS] Downloaded '%s' successfully.%s", core.GREEN, fname, core.NC),
		fmt.Sprintf("Saved to: %s", destPath),
		"Restart the Minecraft server to load the mod.",
	})

	if tui.AskYesNo("Restart Minecraft server now to load the mod?") == 0 {
		_, _ = system.RestartMinecraftService()
	}
}
