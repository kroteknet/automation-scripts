// ==============================================================================
// KROTEK SCRIPT HUB — SERVER ENGINE RESOLVER (PAPER / PURPUR / VANILLA / FABRIC / NEOFORGE)
// Service: Minecraft (game-servers/minecraft)
// File:    game-servers/minecraft/src/install/engines.go
//
// Every version number, build id, loader/installer version and Java requirement is
// resolved live from the upstream APIs. Nothing in this file pins a release.
// ==============================================================================

package install

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"krotek-minecraft-installer/core"
	"krotek-minecraft-installer/logger"
	"krotek-minecraft-installer/system"
	"krotek-minecraft-installer/tui"
)

const (
	paperAPI          = "https://fill.papermc.io/v3/projects/paper"
	purpurAPI         = "https://api.purpurmc.org/v2/purpur"
	fabricMetaAPI     = "https://meta.fabricmc.net/v2/versions"
	neoForgeVersions  = "https://maven.neoforged.net/api/maven/versions/releases/net/neoforged/neoforge"
	neoForgeRepo      = "https://maven.neoforged.net/releases/net/neoforged/neoforge"
	apiTimeout        = 8 * time.Second
	stableVersionExpr = `^\d+(\.\d+)*$`
)

var stableVersionRe = regexp.MustCompile(stableVersionExpr)

// ResolvedBuild describes exactly what will be downloaded for a chosen engine + Minecraft version.
type ResolvedBuild struct {
	URL           string // direct download URL (server jar, Fabric launcher or NeoForge installer)
	BuildID       string // upstream build identifier used later to detect updates
	LoaderVersion string // Fabric loader / NeoForge version
	SHA256        string // optional integrity hash published by the upstream API
}

// EngineDisplayName returns the human readable engine label persisted in the state file.
func EngineDisplayName(engineType string) string {
	switch engineType {
	case "purpur":
		return "Purpur"
	case "vanilla":
		return "Vanilla"
	case "fabric":
		return "Fabric"
	case "neoforge":
		return "NeoForge"
	}
	return "PaperMC"
}

// sortVersionsDesc orders dotted Minecraft versions newest-first (works for 1.x.y and 26.x schemes).
func sortVersionsDesc(v []string) {
	sort.SliceStable(v, func(i, j int) bool { return compareVersions(v[i], v[j]) > 0 })
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ── PaperMC ─────────────────────────────────────────────────────────────────────

type paperProject struct {
	Versions map[string][]string `json:"versions"`
}

type paperBuild struct {
	ID        int    `json:"id"`
	Channel   string `json:"channel"`
	Downloads map[string]struct {
		Name      string `json:"name"`
		URL       string `json:"url"`
		Checksums struct {
			SHA256 string `json:"sha256"`
		} `json:"checksums"`
	} `json:"downloads"`
}

func fetchPaperVersions() ([]string, error) {
	var data paperProject
	if err := system.HTTPGetJSON(paperAPI, apiTimeout, &data); err != nil {
		return nil, err
	}
	var all []string
	for _, list := range data.Versions {
		for _, v := range list {
			if stableVersionRe.MatchString(v) {
				all = append(all, v)
			}
		}
	}
	all = dedupe(all)
	sortVersionsDesc(all)
	return all, nil
}

func resolvePaperBuild(mc string) (ResolvedBuild, error) {
	var b paperBuild
	if err := system.HTTPGetJSON(fmt.Sprintf("%s/versions/%s/builds/latest", paperAPI, mc), apiTimeout, &b); err != nil {
		return ResolvedBuild{}, fmt.Errorf("PaperMC has no build for %s: %w", mc, err)
	}
	dl, ok := b.Downloads["server:default"]
	if !ok || dl.URL == "" {
		return ResolvedBuild{}, fmt.Errorf("PaperMC build %d for %s has no server download", b.ID, mc)
	}
	if b.Channel != "" && !strings.EqualFold(b.Channel, "STABLE") {
		logger.LogWarn(fmt.Sprintf("PaperMC build #%d for %s is on the %s channel (no STABLE build published yet).", b.ID, mc, b.Channel))
	}
	return ResolvedBuild{URL: dl.URL, BuildID: strconv.Itoa(b.ID), SHA256: dl.Checksums.SHA256}, nil
}

// ── Purpur ──────────────────────────────────────────────────────────────────────

func fetchPurpurVersions() ([]string, error) {
	var data struct {
		Versions []string `json:"versions"`
	}
	if err := system.HTTPGetJSON(purpurAPI, apiTimeout, &data); err != nil {
		return nil, err
	}
	var all []string
	for _, v := range data.Versions {
		if stableVersionRe.MatchString(v) {
			all = append(all, v)
		}
	}
	all = dedupe(all)
	sortVersionsDesc(all)
	return all, nil
}

func resolvePurpurBuild(mc string) (ResolvedBuild, error) {
	var data struct {
		Builds struct {
			Latest string `json:"latest"`
		} `json:"builds"`
	}
	if err := system.HTTPGetJSON(fmt.Sprintf("%s/%s", purpurAPI, mc), apiTimeout, &data); err != nil || data.Builds.Latest == "" {
		if err == nil {
			err = fmt.Errorf("no builds listed")
		}
		return ResolvedBuild{}, fmt.Errorf("Purpur has no build for %s: %w", mc, err)
	}
	return ResolvedBuild{
		URL:     fmt.Sprintf("%s/%s/%s/download", purpurAPI, mc, data.Builds.Latest),
		BuildID: data.Builds.Latest,
	}, nil
}

// ── Vanilla ─────────────────────────────────────────────────────────────────────

func resolveVanillaBuild(mc string) (ResolvedBuild, error) {
	u, err := system.MojangServerJarURL(mc)
	if err != nil {
		return ResolvedBuild{}, err
	}
	return ResolvedBuild{URL: u, BuildID: mc}, nil
}

// ── Fabric ──────────────────────────────────────────────────────────────────────

func fetchFabricVersions() ([]string, error) {
	var data []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := system.HTTPGetJSON(fabricMetaAPI+"/game", apiTimeout, &data); err != nil {
		return nil, err
	}
	var out []string
	for _, v := range data {
		if v.Stable {
			out = append(out, v.Version)
		}
	}
	out = dedupe(out)
	sortVersionsDesc(out)
	return out, nil
}

func resolveFabricBuild(mc string) (ResolvedBuild, error) {
	// Loader list scoped to this Minecraft version: guarantees the loader actually supports it.
	var loaders []struct {
		Loader struct {
			Version string `json:"version"`
			Stable  bool   `json:"stable"`
		} `json:"loader"`
	}
	if err := system.HTTPGetJSON(fmt.Sprintf("%s/loader/%s", fabricMetaAPI, mc), apiTimeout, &loaders); err != nil || len(loaders) == 0 {
		if err == nil {
			err = fmt.Errorf("no loader published")
		}
		return ResolvedBuild{}, fmt.Errorf("Fabric does not support Minecraft %s: %w", mc, err)
	}
	loaderVer := loaders[0].Loader.Version
	for _, l := range loaders {
		if l.Loader.Stable {
			loaderVer = l.Loader.Version
			break
		}
	}

	var installers []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := system.HTTPGetJSON(fabricMetaAPI+"/installer", apiTimeout, &installers); err != nil || len(installers) == 0 {
		if err == nil {
			err = fmt.Errorf("no installer published")
		}
		return ResolvedBuild{}, fmt.Errorf("could not resolve Fabric installer version: %w", err)
	}
	installerVer := installers[0].Version
	for _, i := range installers {
		if i.Stable {
			installerVer = i.Version
			break
		}
	}

	return ResolvedBuild{
		URL:           fmt.Sprintf("%s/loader/%s/%s/%s/server/jar", fabricMetaAPI, mc, loaderVer, installerVer),
		BuildID:       loaderVer,
		LoaderVersion: loaderVer,
	}, nil
}

// ── NeoForge ────────────────────────────────────────────────────────────────────

// neoForgeToMinecraft converts a NeoForge version into the Minecraft version it targets.
//
//	legacy scheme: 21.1.172 -> 1.21.1, 21.0.5 -> 1.21, 20.4.237 -> 1.20.4
//	new scheme   : 26.3.0.99 -> 26.3, 26.1.2.5 -> 26.1.2
func neoForgeToMinecraft(nf string) (string, bool) {
	if strings.Contains(nf, "alpha") || strings.Contains(nf, "snapshot") {
		return "", false
	}
	base := strings.SplitN(nf, "-", 2)[0]
	parts := strings.Split(base, ".")
	if len(parts) < 2 {
		return "", false
	}
	a, errA := strconv.Atoi(parts[0])
	b, errB := strconv.Atoi(parts[1])
	if errA != nil || errB != nil {
		return "", false
	}
	if a < 25 { // legacy "1.<a>.<b>" mapping
		if b == 0 {
			return fmt.Sprintf("1.%d", a), true
		}
		return fmt.Sprintf("1.%d.%d", a, b), true
	}
	patch := 0
	if len(parts) >= 4 {
		p, err := strconv.Atoi(parts[2])
		if err != nil {
			return "", false
		}
		patch = p
	}
	if patch == 0 {
		return fmt.Sprintf("%d.%d", a, b), true
	}
	return fmt.Sprintf("%d.%d.%d", a, b, patch), true
}

var neoForgeIndexCache map[string]string

// fetchNeoForgeIndex returns a map Minecraft version -> best NeoForge version (stable preferred over beta).
func fetchNeoForgeIndex() (map[string]string, error) {
	if neoForgeIndexCache != nil {
		return neoForgeIndexCache, nil
	}
	var data struct {
		Versions []string `json:"versions"`
	}
	if err := system.HTTPGetJSON(neoForgeVersions, 15*time.Second, &data); err != nil {
		return nil, err
	}

	bestStable := map[string]string{}
	bestBeta := map[string]string{}
	for _, v := range data.Versions {
		mc, ok := neoForgeToMinecraft(v)
		if !ok {
			continue
		}
		target := bestStable
		if strings.Contains(v, "-") {
			target = bestBeta
		}
		if cur, exists := target[mc]; !exists || compareVersions(strings.SplitN(v, "-", 2)[0], strings.SplitN(cur, "-", 2)[0]) > 0 {
			target[mc] = v
		}
	}

	index := make(map[string]string, len(bestStable)+len(bestBeta))
	for mc, v := range bestBeta {
		index[mc] = v
	}
	for mc, v := range bestStable {
		index[mc] = v
	}
	if len(index) == 0 {
		return nil, fmt.Errorf("NeoForge Maven metadata contained no usable versions")
	}
	neoForgeIndexCache = index
	return index, nil
}

func fetchNeoForgeVersionList() ([]string, error) {
	idx, err := fetchNeoForgeIndex()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(idx))
	for mc := range idx {
		out = append(out, mc)
	}
	sortVersionsDesc(out)
	return out, nil
}

func resolveNeoForgeBuild(mc string) (ResolvedBuild, error) {
	idx, err := fetchNeoForgeIndex()
	if err != nil {
		return ResolvedBuild{}, err
	}
	nf, ok := idx[mc]
	if !ok {
		return ResolvedBuild{}, fmt.Errorf("NeoForge has no release for Minecraft %s", mc)
	}
	return ResolvedBuild{
		URL:           fmt.Sprintf("%s/%s/neoforge-%s-installer.jar", neoForgeRepo, nf, nf),
		BuildID:       nf,
		LoaderVersion: nf,
	}, nil
}

// ── Public engine API ──────────────────────────────────────────────────────────

// FetchEngineVersions lists the Minecraft versions an engine currently supports, newest first.
func FetchEngineVersions(engineType string) ([]string, error) {
	switch engineType {
	case "purpur":
		return fetchPurpurVersions()
	case "vanilla":
		return system.MojangReleaseVersions()
	case "fabric":
		return fetchFabricVersions()
	case "neoforge":
		return fetchNeoForgeVersionList()
	default:
		return fetchPaperVersions()
	}
}

// ResolveBuild resolves the download for engineType at the given Minecraft version.
func ResolveBuild(engineType, mcVersion string) (ResolvedBuild, error) {
	switch engineType {
	case "purpur":
		return resolvePurpurBuild(mcVersion)
	case "vanilla":
		return resolveVanillaBuild(mcVersion)
	case "fabric":
		return resolveFabricBuild(mcVersion)
	case "neoforge":
		return resolveNeoForgeBuild(mcVersion)
	default:
		return resolvePaperBuild(mcVersion)
	}
}

// LatestEngineVersion returns the newest Minecraft version supported by the engine.
func LatestEngineVersion(engineType string) (string, error) {
	versions, err := FetchEngineVersions(engineType)
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("%s API returned no versions", EngineDisplayName(engineType))
	}
	return versions[0], nil
}

// ── Java requirement ───────────────────────────────────────────────────────────

// DetermineRequiredJava asks Mojang which Java major the version needs; the offline
// heuristic below is only used when the metadata API is unreachable.
func DetermineRequiredJava(mcVersion string) int {
	if v, err := system.MojangJavaMajor(mcVersion); err == nil && v > 0 {
		return system.NormalizeJavaLTS(v)
	}
	return determineRequiredJavaOffline(mcVersion)
}

func determineRequiredJavaOffline(mcVersion string) int {
	parts := parseVersionParts(mcVersion)
	at := func(i int) int {
		if i < len(parts) {
			return parts[i]
		}
		return 0
	}
	major, minor, patch := at(0), at(1), at(2)

	switch {
	case major >= 26: // year-based releases
		return 25
	case major == 1 && (minor > 20 || (minor == 20 && patch >= 5)):
		return 21
	case major == 1 && minor >= 17:
		return 17
	case major == 1 && minor >= 12:
		return 11
	}
	return 8
}

// ── Interactive selection ──────────────────────────────────────────────────────

func matchVersionFromOption(cleanOpt, engineName string, versions []string) string {
	cleanOpt = strings.TrimSpace(cleanOpt)
	for _, v := range versions {
		pattern1 := engineName + " " + v + " ("
		pattern2 := engineName + " " + v
		if strings.HasPrefix(cleanOpt, pattern1) || cleanOpt == pattern2 || cleanOpt == v {
			return v
		}
	}
	return ""
}

// SelectMinecraftVersion drives the interactive version picker and returns the chosen
// version together with the fully resolved upstream build. Status 2 means cancelled.
func SelectMinecraftVersion(engineName, engineType string) (string, ResolvedBuild, int) {
	tui.StartSpinner("Fetching available versions live from " + engineName + " API...")
	versions, fetchErr := FetchEngineVersions(engineType)
	tui.StopSpinner(0)

	if fetchErr != nil || len(versions) == 0 {
		logger.LogWarn(fmt.Sprintf("Could not query the %s API (%v). Falling back to the Mojang release list.", engineName, fetchErr))
		if mojang, err := system.MojangReleaseVersions(); err == nil && len(mojang) > 0 {
			versions = mojang
		} else {
			logger.LogWarn("Mojang manifest is unreachable too - enter the Minecraft version manually.")
			versions = nil
		}
	}

	topCount := 4
	if len(versions) < topCount {
		topCount = len(versions)
	}
	top := versions[:topCount]
	remaining := versions[topCount:]

	for {
		var menuOptions []string
		if len(top) > 0 {
			menuOptions = append(menuOptions, "─── LATEST STABLE RELEASES (RECOMMENDED) ───")
			for i, v := range top {
				badge := "Stable Release"
				if i == 0 {
					badge = "Latest Stable" + " " + core.Sym.Bullet + " " + "Recommended"
				}
				menuOptions = append(menuOptions, fmt.Sprintf("%s %s (%s)", engineName, v, badge))
			}
		}

		menuOptions = append(menuOptions, "─── VERSION ARCHIVE & CUSTOM ───")
		if len(remaining) > 0 {
			menuOptions = append(menuOptions, fmt.Sprintf("[+] Expand All Available Versions (%d releases in archive)", len(versions)))
		}
		menuOptions = append(menuOptions, "[>] Enter Custom Version Manually")
		menuOptions = append(menuOptions, "Back / Cancel")

		_, opt, status := tui.SelectMenu(fmt.Sprintf("KROTEK MINECRAFT" + " " + core.Sym.EmDash + " " + "Select %s Version", engineName), menuOptions)
		if status == 2 || strings.Contains(opt, "Back / Cancel") {
			return "", ResolvedBuild{}, 2
		}

		var selectedVersion string

		switch {
		case strings.Contains(opt, "Expand All Available Versions"):
			archiveOptions := []string{"─── ALL AVAILABLE ARCHIVED RELEASES ───"}
			for _, v := range versions {
				archiveOptions = append(archiveOptions, fmt.Sprintf("%s %s", engineName, v))
			}
			archiveOptions = append(archiveOptions, "Back to Recommended Versions")

			_, aOpt, aSt := tui.SelectMenu(fmt.Sprintf("%s" + " " + core.Sym.EmDash + " " + "Full Version Archive", engineName), archiveOptions)
			if aSt == 2 || strings.Contains(aOpt, "Back to Recommended") {
				continue
			}
			selectedVersion = matchVersionFromOption(core.StripANSI(aOpt), engineName, versions)

		case strings.Contains(opt, "Enter Custom Version Manually"):
			def := ""
			if len(top) > 0 {
				def = top[0]
			}
			custom, cSt := tui.ReadTextInput("Enter Minecraft version", def)
			if cSt != 0 {
				continue
			}
			selectedVersion = strings.TrimSpace(custom)

		default:
			selectedVersion = matchVersionFromOption(core.StripANSI(opt), engineName, versions)
		}

		if selectedVersion == "" {
			continue
		}

		tui.StartSpinner(fmt.Sprintf("Resolving %s build for Minecraft %s...", engineName, selectedVersion))
		build, err := ResolveBuild(engineType, selectedVersion)
		tui.StopSpinner(0)
		if err != nil {
			logger.LogError(err.Error())
			continue
		}
		return selectedVersion, build, 0
	}
}

// ── Helpers shared by interactive and headless flows ───────────────────────────

// NormalizeRAM turns "8", "8g", "8G", "4096m" into the JVM notation (8G / 4096M).
func NormalizeRAM(raw string) (string, bool) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" {
		return "", false
	}
	if _, err := strconv.Atoi(s); err == nil {
		s += "G"
	}
	re := regexp.MustCompile(`^[1-9][0-9]*[GM]$`)
	if !re.MatchString(s) {
		return "", false
	}
	return s, true
}

// AutoRAM picks a sensible heap from host memory (leaves headroom for the OS).
func AutoRAM() string {
	total := getHostTotalRAMGB()
	switch {
	case total >= 32:
		return "12G"
	case total >= 16:
		return "8G"
	case total >= 8:
		return "4G"
	case total >= 4:
		return "2G"
	}
	return fmt.Sprintf("%dM", total*512)
}

// ramToMB converts a JVM heap string into megabytes (0 when unparsable).
func ramToMB(ram string) int {
	s := strings.ToUpper(strings.TrimSpace(ram))
	if len(s) < 2 {
		return 0
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil {
		return 0
	}
	if strings.HasSuffix(s, "G") {
		return n * 1024
	}
	return n
}
