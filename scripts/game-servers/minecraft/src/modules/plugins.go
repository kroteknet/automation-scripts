// ==============================================================================
// KROTEK SCRIPT HUB — PLUGIN & EXTENSION PACKAGE MANAGER
// Service: MINECRAFT (game-servers)
// File:    game-servers/minecraft/src/modules/plugins.go
//
// Automates plugin downloads, dependency verification, installation, and activation for extensible software stacks.
// ==============================================================================

package modules

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

type PluginItem struct {
	Name     string
	Filename string
	Path     string
	Enabled  bool
	SizeKB   int64
}

type PopularPlugin struct {
	ID          string
	Name        string
	Category    string
	Description string
	DownloadURL string
	Filename    string
	ExtraURL    string
	ExtraFile   string
	IsOpenUDP   bool
	IsOpenTCP   string
}

var popularPluginCatalog = []PopularPlugin{
	{
		ID:          "geyser",
		Name:        "GeyserMC + Floodgate (Bedrock Crossplay)",
		Category:    "Crossplay / Network",
		Description: "Allows iOS, Android, Xbox, PS5 & Windows Bedrock players to join on UDP port 19132 without needing Java Edition.",
		DownloadURL: "https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest/downloads/spigot",
		Filename:    "Geyser-Spigot.jar",
		ExtraURL:    "https://download.geysermc.org/v2/projects/floodgate/versions/latest/builds/latest/downloads/spigot",
		ExtraFile:   "floodgate-spigot.jar",
		IsOpenUDP:   true,
	},
	{
		ID:          "bluemap",
		Name:        "BlueMap (3D Web Map in Browser)",
		Category:    "Web Map / Visualization",
		Description: "Generates an interactive 3D live map of your Minecraft world in web browser on port 8100.",
		DownloadURL: "",
		Filename:    "BlueMap.jar",
		IsOpenTCP:   core.BlueMapPort,
	},
	{
		ID:          "viaversion",
		Name:        "ViaVersion (Multi-Client Compatibility)",
		Category:    "Compatibility",
		Description: "Allows players with older and newer Minecraft client versions to connect seamlessly.",
		DownloadURL: "",
		Filename:    "ViaVersion.jar",
	},
	{
		ID:          "essentialsx",
		Name:        "EssentialsX (Core Commands & Economy)",
		Category:    "Gameplay",
		Description: "Comprehensive suite of essential commands (/spawn, /home, /tpa, /warp, /back, economy & kits).",
		DownloadURL: "",
		Filename:    "EssentialsX.jar",
	},
	{
		ID:          "luckperms",
		Name:        "LuckPerms (Advanced Permissions & Ranks)",
		Category:    "Administration",
		Description: "Industry standard permissions plugin with web editor GUI (/lp editor) for player ranks and prefixes.",
		DownloadURL: "https://download.luckperms.net/1668/bukkit/loader/LuckPerms-Bukkit-5.5.81.jar",
		Filename:    "LuckPerms.jar",
	},
	{
		ID:          "vault",
		Name:        "Vault (Permissions & Economy Bridge)",
		Category:    "API / Bridge",
		Description: "Essential library required by most economy, shop, and chat formatting plugins to communicate with LuckPerms.",
		DownloadURL: "",
		Filename:    "Vault.jar",
	},
	{
		ID:          "chunky",
		Name:        "Chunky (Chunk Pre-Generator)",
		Category:    "Performance",
		Description: "Pre-generates world terrain and chunks in the background to completely eliminate exploration lag spikes.",
		DownloadURL: "",
		Filename:    "Chunky.jar",
	},
	{
		ID:          "coreprotect",
		Name:        "CoreProtect (Anti-Grief & Block Logger)",
		Category:    "Security",
		Description: "Fast data logging and inspection tool to inspect block placements, chest transactions, and rollback griefing.",
		DownloadURL: "",
		Filename:    "CoreProtect.jar",
	},
	{
		ID:          "worldedit",
		Name:        "WorldEdit (In-Game World Editor)",
		Category:    "Administration / Building",
		Description: "The industry-standard in-game world manipulation and terraforming tool for server admins.",
		DownloadURL: "",
		Filename:    "WorldEdit.jar",
	},
}

func getPluginDir() string {
	serverDir := getServerDataDir()
	pDir := filepath.Join(serverDir, "plugins")
	_ = os.MkdirAll(pDir, 0755)
	return pDir
}

func GetInstalledPlugins() []PluginItem {
	pDir := getPluginDir()
	var list []PluginItem

	entries, err := os.ReadDir(pDir)
	if err != nil {
		return list
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fname := e.Name()
		fullPath := filepath.Join(pDir, fname)

		if strings.HasSuffix(fname, ".jar") {
			fi, _ := e.Info()
			sz := int64(0)
			if fi != nil {
				sz = fi.Size() / 1024
			}
			cleanName := strings.TrimSuffix(fname, ".jar")
			list = append(list, PluginItem{
				Name:     cleanName,
				Filename: fname,
				Path:     fullPath,
				Enabled:  true,
				SizeKB:   sz,
			})
		} else if strings.HasSuffix(fname, ".jar.disabled") {
			fi, _ := e.Info()
			sz := int64(0)
			if fi != nil {
				sz = fi.Size() / 1024
			}
			cleanName := strings.TrimSuffix(fname, ".jar.disabled")
			list = append(list, PluginItem{
				Name:     cleanName,
				Filename: fname,
				Path:     fullPath,
				Enabled:  false,
				SizeKB:   sz,
			})
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})

	return list
}

// ManagePluginsMenu provides 1-click popular plugin downloads, activation, and custom URL plugin installation.
func ManagePluginsMenu() {
	for {
		plugins := GetInstalledPlugins()

		var menuOpts []string
		menuOpts = append(menuOpts, "─── QUICK INSTALLERS & REPOSITORY ───")
		menuOpts = append(menuOpts, "[+] 1-Click Install Popular Plugins (GeyserMC, ViaVersion, EssentialsX...)")
		menuOpts = append(menuOpts, "[>] Download Custom Plugin from Direct URL (.jar)")
		menuOpts = append(menuOpts, "─── INSTALLED PLUGINS ───")

		if len(plugins) == 0 {
			menuOpts = append(menuOpts, fmt.Sprintf("  (No plugins currently installed in %s/)", getPluginDir()))
		} else {
			for i, p := range plugins {
				badge := core.GREEN + "[ENABLED]" + core.NC
				if !p.Enabled {
					badge = core.YELLOW + "[DISABLED]" + core.NC
				}
				sizeMB := float64(p.SizeKB) / 1024.0
				menuOpts = append(menuOpts, fmt.Sprintf("[%02d] %-24s %s (%.1f MB)", i+1, p.Name, badge, sizeMB))
			}
		}

		menuOpts = append(menuOpts, "─── NAVIGATION ───")
		menuOpts = append(menuOpts, "Back to Main Menu")

		title := fmt.Sprintf("KROTEK MINECRAFT" + " " + core.Sym.EmDash + " " + "Plugin & Addon Manager (%d installed)", len(plugins))
		idx, opt, st := tui.SelectMenu(title, menuOpts)
		if st == 2 || strings.Contains(opt, "Back to Main Menu") {
			return
		}

		cleanOpt := core.StripANSI(opt)
		if strings.Contains(cleanOpt, "1-Click Install Popular Plugins") {
			showPopularPluginsMenu()
			continue
		} else if strings.Contains(cleanOpt, "Download Custom Plugin from Direct URL") {
			downloadCustomPlugin()
			continue
		}

		// Plugin item was clicked
		for _, p := range plugins {
			if strings.Contains(cleanOpt, p.Name) {
				handlePluginAction(p)
				break
			}
		}
		_ = idx
	}
}

func handlePluginAction(p PluginItem) {
	statusStr := "Enabled"
	toggleLabel := "Disable Plugin"
	if !p.Enabled {
		statusStr = "Disabled"
		toggleLabel = "Enable Plugin"
	}

	subOpts := []string{
		fmt.Sprintf("%s%s%s (%s -> %s)", core.YELLOW, toggleLabel, core.NC, statusStr, func() string {
			if p.Enabled {
				return "Disabled"
			}
			return "Enabled"
		}()),
		core.RED + "Delete Plugin (.jar file)" + core.NC,
		"Back to Plugins List",
	}

	title := fmt.Sprintf("Manage: %s (%s)", p.Filename, statusStr)
	_, opt, st := tui.SelectMenu(title, subOpts)
	if st != 0 || strings.Contains(opt, "Back") {
		return
	}

	cleanOpt := core.StripANSI(opt)
	pDir := getPluginDir()

	if strings.HasPrefix(cleanOpt, "Disable Plugin") {
		newPath := filepath.Join(pDir, p.Name+".jar.disabled")
		_ = os.Rename(p.Path, newPath)
		tui.PrintInBox("Plugin Disabled", []string{
			fmt.Sprintf("%s[OK] '%s' is now DISABLED.%s", core.YELLOW, p.Name, core.NC),
			"Restart Minecraft or reload plugins for changes to take effect.",
		})
	} else if strings.HasPrefix(cleanOpt, "Enable Plugin") {
		newPath := filepath.Join(pDir, p.Name+".jar")
		_ = os.Rename(p.Path, newPath)
		tui.PrintInBox("Plugin Enabled", []string{
			fmt.Sprintf("%s[SUCCESS] '%s' is now ENABLED.%s", core.GREEN, p.Name, core.NC),
			"Restart Minecraft or reload plugins for changes to take effect.",
		})
	} else if strings.HasPrefix(cleanOpt, "Delete Plugin") {
		confirm := tui.AskYesNo(fmt.Sprintf("Are you sure you want to permanently delete %s?", p.Filename))
		if confirm == 0 {
			_ = os.Remove(p.Path)
			tui.PrintInBox("Plugin Deleted", []string{
				fmt.Sprintf("%s[OK] Removed '%s' from plugins folder.%s", core.GREEN, p.Filename, core.NC),
			})
		}
	}
}

func showPopularPluginsMenu() {
	var opts []string
	installed := GetInstalledPlugins()
	installedMap := make(map[string]bool)
	for _, ip := range installed {
		installedMap[strings.ToLower(ip.Name)] = true
	}

	for i, pp := range popularPluginCatalog {
		status := core.YELLOW + "[INSTALL]" + core.NC
		baseClean := strings.ToLower(strings.TrimSuffix(pp.Filename, ".jar"))
		if installedMap[baseClean] {
			status = core.GREEN + "[INSTALLED]" + core.NC
		}
		opts = append(opts, fmt.Sprintf("[%02d] %-42s %s  (%s)", i+1, pp.Name, status, pp.Category))
	}
	opts = append(opts, "Back to Plugins Menu")

	idx, opt, st := tui.SelectMenu("KROTEK MINECRAFT" + " " + core.Sym.EmDash + " " + "1-Click Popular Plugin Repository", opts)
	if st != 0 || strings.Contains(opt, "Back") || idx >= len(popularPluginCatalog) {
		return
	}

	plugin := popularPluginCatalog[idx]
	installPopularPlugin(plugin)
}

func downloadFileWithSpinner(url, destPath, spinnerMsg string) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("refusing insecure non-HTTPS download URL: %s", url)
	}
	if system.IsSymlinkPath(destPath) {
		return fmt.Errorf("refusing to write downloaded file through symlink: %s", destPath)
	}
	tui.StartSpinner(spinnerMsg)
	defer tui.StopSpinner(0)

	client := &http.Client{
		Timeout: 120 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("refusing insecure redirect to %s", req.URL.Scheme)
			}
			req.Header.Set("User-Agent", core.UserAgent)
			return nil
		},
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", core.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP %s", resp.Status)
	}

	const maxPluginBytes int64 = 256 * 1024 * 1024
	root, rErr := os.OpenRoot(filepath.Dir(destPath))
	if rErr != nil {
		return rErr
	}
	defer root.Close()

	base := filepath.Base(destPath)
	out, err := root.OpenFile(base, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	n, copyErr := io.Copy(out, io.LimitReader(resp.Body, maxPluginBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		_ = root.Remove(base)
		return copyErr
	}
	if closeErr != nil {
		_ = root.Remove(base)
		return closeErr
	}
	if n > maxPluginBytes {
		_ = root.Remove(base)
		return fmt.Errorf("downloaded file exceeded maximum size limit (%d bytes)", maxPluginBytes)
	}
	return nil
}

func fetchLatestGitHubReleaseAsset(repo, assetPrefix, containsFilter string) (string, error) {
	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo), nil)
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
		return "", fmt.Errorf("GitHub API status %s", resp.Status)
	}

	var data struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	for _, a := range data.Assets {
		if strings.HasPrefix(a.Name, assetPrefix) && strings.HasSuffix(a.Name, ".jar") {
			if containsFilter == "" || strings.Contains(a.Name, containsFilter) {
				return a.BrowserDownloadURL, nil
			}
		}
	}
	return "", fmt.Errorf("no matching jar asset found for %s", assetPrefix)
}

func fetchModrinthLatestJar(projectID, filter string) (string, error) {
	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.modrinth.com/v2/project/%s/version", projectID), nil)
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
		return "", fmt.Errorf("Modrinth API status %s", resp.Status)
	}

	var versions []struct {
		Files []struct {
			Filename string `json:"filename"`
			URL      string `json:"url"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return "", err
	}

	for _, v := range versions {
		for _, f := range v.Files {
			if strings.HasSuffix(f.Filename, ".jar") {
				if filter == "" || strings.Contains(strings.ToLower(f.Filename), strings.ToLower(filter)) {
					return f.URL, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no matching jar found on Modrinth for %s", projectID)
}

func installPopularPlugin(p PopularPlugin) {
	pDir := getPluginDir()
	destPath := filepath.Join(pDir, p.Filename)

	downloadURL := p.DownloadURL
	if p.ID == "luckperms" {
		if body, code, err := system.HttpFetchBytes("https://metadata.luckperms.net/data/downloads", 2*1024*1024); err == nil && code == 200 {
			var meta struct {
				Downloads struct {
					Bukkit string `json:"bukkit"`
				} `json:"downloads"`
			}
			if json.Unmarshal(body, &meta) == nil && strings.HasPrefix(meta.Downloads.Bukkit, "https://") {
				downloadURL = meta.Downloads.Bukkit
			}
		}
	} else if p.ID == "chunky" {
		if body, code, err := system.HttpFetchBytes("https://hangar.papermc.io/api/v1/projects/Chunky/latestrelease", 1*1024*1024); err == nil && code == 200 {
			latestVer := strings.TrimSpace(string(body))
			if latestVer != "" && !strings.ContainsAny(latestVer, "/\\?&# \t\r\n") {
				downloadURL = fmt.Sprintf("https://hangarcdn.papermc.io/plugins/pop4959/Chunky/versions/%s/PAPER/Chunky-Bukkit-%s.jar", latestVer, latestVer)
			}
		}
	} else if p.ID == "viaversion" {
		if u, err := fetchModrinthLatestJar("viaversion", "ViaVersion-"); err == nil && u != "" {
			downloadURL = u
		} else if u, err := fetchLatestGitHubReleaseAsset("ViaVersion/ViaVersion", "ViaVersion-", ""); err == nil && u != "" {
			downloadURL = u
		}
	} else if p.ID == "essentialsx" {
		if u, err := fetchModrinthLatestJar("essentialsx", "EssentialsX-"); err == nil && u != "" {
			downloadURL = u
		} else if u, err := fetchLatestGitHubReleaseAsset("EssentialsX/Essentials", "EssentialsX-", ""); err == nil && u != "" {
			downloadURL = u
		}
	} else if p.ID == "bluemap" {
		if u, err := fetchModrinthLatestJar("bluemap", "paper"); err == nil && u != "" {
			downloadURL = u
		} else if u, err := fetchLatestGitHubReleaseAsset("BlueMap-Minecraft/BlueMap", "bluemap-", "-paper.jar"); err == nil && u != "" {
			downloadURL = u
		}
	} else if p.ID == "vault" {
		if u, err := fetchLatestGitHubReleaseAsset("MilkBowl/Vault", "Vault", ""); err == nil && u != "" {
			downloadURL = u
		} else {
			downloadURL = "https://github.com/MilkBowl/Vault/releases/latest/download/Vault.jar"
		}
	} else if p.ID == "coreprotect" {
		if u, err := fetchModrinthLatestJar("coreprotect", ""); err == nil && u != "" {
			downloadURL = u
		}
	} else if p.ID == "worldedit" {
		if u, err := fetchModrinthLatestJar("worldedit", "bukkit"); err == nil && u != "" {
			downloadURL = u
		}
	}

	if downloadURL == "" {
		tui.PrintInBox("Download Error", []string{
			core.RED + "[ERROR] Could not resolve download URL for " + p.Name + core.NC,
			"Upstream API lookup failed or repository is unreachable.",
		})
		return
	}

	err := downloadFileWithSpinner(downloadURL, destPath, fmt.Sprintf("Downloading %s...", p.Name))
	if err != nil {
		tui.PrintInBox("Download Failed", []string{
			core.RED + "[ERROR] Failed to download " + p.Name + core.NC,
			err.Error(),
		})
		return
	}

	// Download companion addon (e.g. Floodgate for GeyserMC)
	if p.ExtraURL != "" && p.ExtraFile != "" {
		extraPath := filepath.Join(pDir, p.ExtraFile)
		_ = downloadFileWithSpinner(p.ExtraURL, extraPath, fmt.Sprintf("Downloading companion %s...", p.ExtraFile))
	}

	// Open ports across all firewall daemons (ufw, iptables, nftables, firewalld)
	if p.IsOpenUDP {
		system.OpenUniversalFirewallPort(core.BedrockPort, "udp")
	}
	if p.IsOpenTCP != "" {
		system.OpenUniversalFirewallPort(p.IsOpenTCP, "tcp")
	}

	_ = exec.Command("chown", "-R", core.ServiceUser+":"+core.ServiceUser, pDir).Run()

	var lines []string
	lines = append(lines, fmt.Sprintf("%s[SUCCESS] %s installed successfully!%s", core.GREEN, p.Name, core.NC), "")
	lines = append(lines, tui.FormatKV("Plugin Location", destPath, core.CYAN))
	if p.ExtraFile != "" {
		lines = append(lines, tui.FormatKV("Companion Addon", p.ExtraFile, core.CYAN))
	}
	if p.IsOpenUDP {
		lines = append(lines, tui.FormatKV("Bedrock Port", "19132 (UDP Opened in Firewall)", core.YELLOW))
		lines = append(lines, "", core.BOLD+"Bedrock (Mobile/Console) Join Info:"+core.NC)
		lines = append(lines, fmt.Sprintf("  Server IP: %s " + " " + core.Sym.Bullet + " " + " Port: 19132", system.GetLocalIP()))
	}
	if p.IsOpenTCP == "8100" {
		lines = append(lines, tui.FormatKV("Web Map Port", "8100 (TCP Opened in Firewall)", core.YELLOW))
		lines = append(lines, "", core.BOLD+"BlueMap 3D Web Interface:"+core.NC)
		lines = append(lines, fmt.Sprintf("  Local Network : %shttp://%s:8100%s", core.BOLD+core.CYAN, system.GetLocalIP(), core.NC))
		lines = append(lines, fmt.Sprintf("  Public WAN    : %shttp://%s:8100%s", core.BOLD+core.GREEN, system.GetPublicIP(), core.NC))
	}
	lines = append(lines, "", "Restart Minecraft server to load the new plugin into memory.")

	tui.PrintInBox("Plugin Installed", lines)

	// Ask to restart server using unified service controller
	if tui.AskYesNo("Do you want to restart Minecraft Server now to load the plugin?") == 0 {
		_, _ = system.RestartMinecraftService()
	}
}

func downloadCustomPlugin() {
	url, st := tui.ReadTextInput("Enter direct URL to plugin .jar file", "https://")
	url = strings.TrimSpace(url)
	if st != 0 || url == "" || url == "https://" {
		return
	}

	fname := filepath.Base(url)
	if !strings.HasSuffix(fname, ".jar") {
		fname += ".jar"
	}

	customName, cSt := tui.ReadTextInput("Enter filename to save as", fname)
	if cSt == 0 && strings.TrimSpace(customName) != "" {
		fname = filepath.Base(strings.TrimSpace(customName))
	} else {
		fname = filepath.Base(fname)
	}

	fname = strings.ReplaceAll(fname, "..", "")
	fname = strings.ReplaceAll(fname, "/", "")
	fname = strings.ReplaceAll(fname, "\\", "")
	if !strings.HasSuffix(fname, ".jar") {
		fname += ".jar"
	}

	pDir := getPluginDir()
	destPath := filepath.Join(pDir, fname)
	cleanDest := filepath.Clean(destPath)
	cleanPDir := filepath.Clean(pDir)

	if !strings.HasPrefix(cleanDest, cleanPDir+string(filepath.Separator)) {
		tui.PrintInBox("Security Error", []string{
			core.RED + "[CRITICAL] Invalid plugin filename or path traversal detected." + core.NC,
			"Plugin file must reside strictly inside the server plugin directory.",
		})
		return
	}

	err := downloadFileWithSpinner(url, destPath, fmt.Sprintf("Downloading %s...", fname))
	if err != nil {
		tui.PrintInBox("Download Error", []string{
			core.RED + "[ERROR] Failed to download custom plugin." + core.NC,
			err.Error(),
		})
		return
	}

	_ = exec.Command("chown", "-R", "minecraft:minecraft", pDir).Run()

	tui.PrintInBox("Custom Plugin Downloaded", []string{
		fmt.Sprintf("%s[SUCCESS] Downloaded '%s' successfully!%s", core.GREEN, fname, core.NC),
		fmt.Sprintf("Saved to: %s", destPath),
		"Restart Minecraft server to load the plugin.",
	})

	if tui.AskYesNo("Do you want to restart Minecraft Server now to load the plugin?") == 0 {
		_, _ = system.RestartMinecraftService()
	}
}
