// ==============================================================================
// KROTEK SCRIPT HUB — PLUGIN & MOD COMPATIBILITY ENGINE
// Service: MINECRAFT (game-servers)
// File:    game-servers/minecraft/src/system/compat.go
//
// Scans installed plugins and mods against a target Minecraft version before
// applying server upgrades. Uses three tiers of detection:
//   1. Local JAR inspection (plugin.yml / fabric.mod.json inside ZIP archive)
//   2. Modrinth project version API (exact or fuzzy version matching)
//   3. Hangar (PaperMC) project version API
// Produces a human-readable compatibility report shown to the user before
// any server.jar replacement is committed to disk.
// ==============================================================================

package system

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"krotek-minecraft-installer/core"
	"krotek-minecraft-installer/tui"
)

// CompatStatus classifies whether a plugin or mod is safe to use after an upgrade.
type CompatStatus int

const (
	CompatOK      CompatStatus = iota // confirmed compatible via API or version range
	CompatFuzzy                       // same minor series; likely compatible but unconfirmed
	CompatUnknown                     // no remote data; cannot determine
	CompatRisk                        // last known version is older than current server minor
)

// PluginCompatReport holds the result of a single plugin compatibility check.
type PluginCompatReport struct {
	FileName     string
	PluginName   string
	LocalVersion string
	APIVersion   string // min api-version declared in plugin.yml
	Status       CompatStatus
	Reason       string
}

// pluginYMLFields captures the relevant fields from a Bukkit plugin.yml.
type pluginYMLFields struct {
	Name       string
	Version    string
	APIVersion string // "api-version" key
}

// FabricModFields captures relevant fields from a Fabric fabric.mod.json.
type FabricModFields struct {
	ID      string
	Version string
	Depends map[string]interface{} // depends.minecraft may be string or array
}

// parseVersionPartsCompat splits a semantic version string into integer parts.
// Non-numeric segments are treated as zero so that "1.21.1" yields [1, 21, 1].
func parseVersionPartsCompat(v string) []int {
	clean := regexp.MustCompile(`[^0-9.]`).ReplaceAllString(v, "")
	parts := strings.Split(clean, ".")
	var res []int
	for _, p := range parts {
		if val, err := strconv.Atoi(p); err == nil {
			res = append(res, val)
		} else {
			res = append(res, 0)
		}
	}
	return res
}

// IsSameMinorVersion returns true when v1 and v2 share the same minor release
// series (e.g. 1.21.1 and 1.21.4 both belong to the 1.21.x series).
// For the new Minecraft versioning scheme (26.x) it compares only the major.
func IsSameMinorVersion(v1, v2 string) bool {
	p1 := parseVersionPartsCompat(v1)
	p2 := parseVersionPartsCompat(v2)
	if len(p1) < 2 || len(p2) < 2 {
		return false
	}
	// Legacy 1.x.y versioning
	if p1[0] == 1 && p2[0] == 1 {
		return p1[1] == p2[1]
	}
	// New-style versioning (26.x, 27.x, ...)
	return p1[0] == p2[0]
}

// IsNewerOrEqualMinor returns true when v1 is in the same minor series as v2
// or an older minor series (i.e. the plugin targets an API that is still
// backward-compatible with v2).
func IsNewerOrEqualMinor(pluginAPIVer, serverVer string) bool {
	p1 := parseVersionPartsCompat(pluginAPIVer)
	p2 := parseVersionPartsCompat(serverVer)
	if len(p1) < 2 || len(p2) < 2 {
		return false
	}
	// Same major, plugin minor <= server minor → compatible
	if p1[0] == p2[0] && p1[1] <= p2[1] {
		return true
	}
	return false
}

// readSimpleYAMLKey extracts a scalar value for the given key from a minimal
// YAML-like text (key: value lines). Does not handle nested blocks.
func readSimpleYAMLKey(data []byte, key string) string {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		prefix := key + ":"
		if strings.HasPrefix(trimmed, prefix) {
			val := strings.TrimSpace(trimmed[len(prefix):])
			val = strings.Trim(val, "\"'")
			return val
		}
	}
	return ""
}

// readPluginYML opens a .jar file (ZIP archive) and extracts the Bukkit
// plugin.yml metadata. Returns an empty struct if not found.
func readPluginYML(jarPath string) pluginYMLFields {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return pluginYMLFields{}
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name != "plugin.yml" && f.Name != "bungee.yml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(rc, 1<<20))
		_ = rc.Close()
		if err != nil {
			continue
		}
		return pluginYMLFields{
			Name:       readSimpleYAMLKey(data, "name"),
			Version:    readSimpleYAMLKey(data, "version"),
			APIVersion: readSimpleYAMLKey(data, "api-version"),
		}
	}
	return pluginYMLFields{}
}

// ReadFabricModJSON opens a .jar file and extracts fabric.mod.json metadata.
func ReadFabricModJSON(jarPath string) FabricModFields {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return FabricModFields{}
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name != "fabric.mod.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(rc, 1<<20))
		_ = rc.Close()
		if err != nil {
			continue
		}
		var out FabricModFields
		var raw map[string]interface{}
		if json.Unmarshal(data, &raw) == nil {
			if v, ok := raw["id"].(string); ok {
				out.ID = v
			}
			if v, ok := raw["version"].(string); ok {
				out.Version = v
			}
			if dep, ok := raw["depends"].(map[string]interface{}); ok {
				out.Depends = dep
			}
		}
		return out
	}
	return FabricModFields{}
}

// modrinthVersionEntry represents one version returned by the Modrinth API.
type modrinthVersionEntry struct {
	GameVersions  []string `json:"game_versions"`
	Loaders       []string `json:"loaders"`
	VersionNumber string   `json:"version_number"`
}

// checkModrinthCompat queries the Modrinth project version list filtered to
// the specified game version and server type (paper/fabric/purpur/vanilla).
// Returns (confirmedExact, confirmedFuzzy, latestKnownVersion, error).
func checkModrinthCompat(slug, mcVersion, loader string) (bool, bool, string, error) {
	client := &http.Client{Timeout: 6 * time.Second}
	url := fmt.Sprintf(
		"https://api.modrinth.com/v2/project/%s/version?game_versions=[\"%s\"]&loaders=[\"%s\"]",
		slug, mcVersion, loader,
	)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, false, "", err
	}
	req.Header.Set("User-Agent", core.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return false, false, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return false, false, "", fmt.Errorf("project not found on Modrinth")
	}
	if resp.StatusCode != 200 {
		return false, false, "", fmt.Errorf("Modrinth API returned HTTP %d", resp.StatusCode)
	}

	var versions []modrinthVersionEntry
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return false, false, "", err
	}

	// Exact version confirmed
	if len(versions) > 0 {
		return true, false, versions[0].VersionNumber, nil
	}

	// Exact not found; query all versions to check fuzzy compatibility
	allURL := fmt.Sprintf("https://api.modrinth.com/v2/project/%s/version", slug)
	req2, err := http.NewRequest("GET", allURL, nil)
	if err != nil {
		return false, false, "", nil
	}
	req2.Header.Set("User-Agent", core.UserAgent)

	resp2, err := client.Do(req2)
	if err != nil {
		return false, false, "", nil
	}
	defer resp2.Body.Close()

	var allVersions []modrinthVersionEntry
	if err := json.NewDecoder(resp2.Body).Decode(&allVersions); err != nil {
		return false, false, "", nil
	}

	latestKnown := ""
	for _, v := range allVersions {
		if latestKnown == "" && v.VersionNumber != "" {
			latestKnown = v.VersionNumber
		}
		for _, gv := range v.GameVersions {
			if IsSameMinorVersion(gv, mcVersion) {
				return false, true, v.VersionNumber, nil
			}
		}
	}

	return false, false, latestKnown, nil
}

// slugFromFilename derives a Modrinth-compatible slug candidate from a .jar
// filename by stripping version suffixes and lowercasing.
// Example: "EssentialsX-2.22.0.jar" -> "essentialsx"
func slugFromFilename(filename string) string {
	base := strings.TrimSuffix(filepath.Base(filename), ".jar")
	base = strings.TrimSuffix(base, ".jar.disabled")
	// Strip trailing version patterns like -2.22.0 or -Bukkit-5.5.81
	re := regexp.MustCompile(`[-_][\d].*$`)
	base = re.ReplaceAllString(base, "")
	re2 := regexp.MustCompile(`[-_](Bukkit|Paper|Spigot|Fabric|Forge|v[\d].*)$`)
	base = re2.ReplaceAllString(base, "")
	return strings.ToLower(base)
}

// loaderForEngine maps an engine name to a Modrinth loader identifier.
func loaderForEngine(engine string) string {
	switch EngineKey(engine) {
	case "fabric":
		return "fabric"
	case "neoforge":
		return "neoforge"
	case "purpur":
		return "purpur"
	case "vanilla":
		return "vanilla"
	default:
		return "paper"
	}
}

// statusLabel returns a short bracketed label for display in the TUI report.
func statusLabel(s CompatStatus) string {
	switch s {
	case CompatOK:
		return core.GREEN + "[CONFIRMED]" + core.NC
	case CompatFuzzy:
		return core.YELLOW + "[LIKELY OK ]" + core.NC
	case CompatUnknown:
		return core.DIM + "[UNKNOWN   ]" + core.NC
	case CompatRisk:
		return core.RED + "[RISK      ]" + core.NC
	}
	return core.DIM + "[UNKNOWN   ]" + core.NC
}

// CheckPluginCompatibility scans all .jar files in pluginsDir and evaluates
// each against targetVersion. The engine parameter adjusts the Modrinth loader
// filter (e.g. "PaperMC", "Fabric", "Purpur").
// Returns a slice of reports, one per plugin/mod found.
func CheckPluginCompatibility(pluginsDir, targetVersion, engine string) []PluginCompatReport {
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return nil
	}

	loader := loaderForEngine(engine)
	var reports []PluginCompatReport

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fname := e.Name()
		if !strings.HasSuffix(fname, ".jar") && !strings.HasSuffix(fname, ".jar.disabled") {
			continue
		}

		fullPath := filepath.Join(pluginsDir, fname)
		report := PluginCompatReport{FileName: fname}

		// Determine whether this is a Fabric mod or Bukkit plugin
		isFabric := false
		if loader == "fabric" {
			meta := ReadFabricModJSON(fullPath)
			if meta.ID != "" {
				isFabric = true
				report.PluginName = meta.ID
				report.LocalVersion = meta.Version

				// Fabric mods declare depends.minecraft with semver ranges
				report.Status, report.Reason = checkFabricModCompat(meta, targetVersion)
				if report.Status == CompatOK || report.Status == CompatFuzzy {
					reports = append(reports, report)
					continue
				}
				// Fall through to Modrinth check
			}
		}

		if !isFabric {
			pluginMeta := readPluginYML(fullPath)
			if pluginMeta.Name != "" {
				report.PluginName = pluginMeta.Name
			} else {
				report.PluginName = slugFromFilename(fname)
			}
			report.LocalVersion = pluginMeta.Version
			report.APIVersion = pluginMeta.APIVersion

			// Fast-path: api-version declared in plugin.yml
			if pluginMeta.APIVersion != "" {
				if IsNewerOrEqualMinor(pluginMeta.APIVersion, targetVersion) {
					report.Status = CompatOK
					report.Reason = fmt.Sprintf(
						"plugin.yml api-version %s is compatible with server %s",
						pluginMeta.APIVersion, targetVersion,
					)
					reports = append(reports, report)
					continue
				}
			}
		}

		// Remote check via Modrinth
		slug := slugFromFilename(fname)
		exact, fuzzy, remoteVer, apiErr := checkModrinthCompat(slug, targetVersion, loader)

		if apiErr != nil {
			// Network or not found: unknown status
			report.Status = CompatUnknown
			report.Reason = fmt.Sprintf("no Modrinth data found for slug '%s'", slug)
		} else if exact {
			report.Status = CompatOK
			report.Reason = fmt.Sprintf("Modrinth confirms build %s for Minecraft %s", remoteVer, targetVersion)
		} else if fuzzy {
			report.Status = CompatFuzzy
			report.Reason = fmt.Sprintf(
				"Modrinth build %s targets same minor series as %s; usually compatible",
				remoteVer, targetVersion,
			)
		} else {
			// No known build for this minor series at all
			if remoteVer != "" {
				report.Status = CompatRisk
				report.Reason = fmt.Sprintf(
					"last known Modrinth build (%s) may not support Minecraft %s",
					remoteVer, targetVersion,
				)
			} else {
				report.Status = CompatUnknown
				report.Reason = "no version information found on Modrinth"
			}
		}

		reports = append(reports, report)
	}

	return reports
}

// checkFabricModCompat evaluates a Fabric mod's declared minecraft dependency
// range against the target server version.
func checkFabricModCompat(meta FabricModFields, targetVersion string) (CompatStatus, string) {
	if meta.Depends == nil {
		return CompatUnknown, "fabric.mod.json has no depends block"
	}

	raw, ok := meta.Depends["minecraft"]
	if !ok {
		return CompatUnknown, "fabric.mod.json does not declare minecraft dependency"
	}

	// The minecraft dependency can be a string (">=1.21") or an array
	var ranges []string
	switch v := raw.(type) {
	case string:
		ranges = []string{v}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				ranges = append(ranges, s)
			}
		}
	}

	for _, r := range ranges {
		if fabricRangeMatches(r, targetVersion) {
			return CompatOK, fmt.Sprintf(
				"fabric.mod.json declares minecraft %q which covers %s", r, targetVersion,
			)
		}
		// Check fuzzy: same minor series even if range does not exactly match
		clean := regexp.MustCompile(`[^0-9.]`).ReplaceAllString(r, "")
		if IsSameMinorVersion(clean, targetVersion) {
			return CompatFuzzy, fmt.Sprintf(
				"fabric.mod.json declares minecraft %q; same minor series as %s", r, targetVersion,
			)
		}
	}

	return CompatRisk, fmt.Sprintf(
		"fabric.mod.json minecraft dependency %v does not cover %s", ranges, targetVersion,
	)
}

// fabricRangeMatches evaluates a Fabric semver range expression against version.
// Supports: exact ("1.21.4"), prefix-wildcard ("1.21.x"), >=1.21, ~1.21, ^1.21.
func fabricRangeMatches(rangeExpr, version string) bool {
	r := strings.TrimSpace(rangeExpr)

	// Exact match
	if regexp.MustCompile(`^\d`).MatchString(r) {
		return r == version || IsSameMinorVersion(r, version)
	}

	// >= operator: mod works on version equal or newer
	if strings.HasPrefix(r, ">=") {
		minVer := strings.TrimSpace(r[2:])
		rp := parseVersionPartsCompat(minVer)
		vp := parseVersionPartsCompat(version)
		return versionGTE(vp, rp)
	}

	// ~1.21 or ^1.21: patch-level or minor-level compatibility
	if strings.HasPrefix(r, "~") || strings.HasPrefix(r, "^") {
		base := strings.TrimSpace(r[1:])
		return IsSameMinorVersion(base, version)
	}

	// Wildcard: "1.21.x" or "1.21.*"
	if strings.Contains(r, "x") || strings.Contains(r, "*") {
		prefix := regexp.MustCompile(`[x*].*$`).ReplaceAllString(r, "")
		prefix = strings.TrimSuffix(prefix, ".")
		return strings.HasPrefix(version, prefix)
	}

	return false
}

// versionGTE returns true when v >= min (element-wise numeric comparison).
func versionGTE(v, min []int) bool {
	maxLen := len(v)
	if len(min) > maxLen {
		maxLen = len(min)
	}
	for i := 0; i < maxLen; i++ {
		vi := 0
		if i < len(v) {
			vi = v[i]
		}
		mi := 0
		if i < len(min) {
			mi = min[i]
		}
		if vi > mi {
			return true
		}
		if vi < mi {
			return false
		}
	}
	return true
}

// ShowCompatibilityReport renders a full-screen compatibility report box in the
// TUI. Returns true when the user acknowledges the results and wants to proceed
// with the update, false when they choose to abort.
func ShowCompatibilityReport(reports []PluginCompatReport, targetVersion string) bool {
	tui.ClearScreen()

	riskCount := 0
	unknownCount := 0
	for _, r := range reports {
		switch r.Status {
		case CompatRisk:
			riskCount++
		case CompatUnknown:
			unknownCount++
		}
	}

	var lines []string
	lines = append(lines,
		core.BOLD+core.CYAN+"PLUGIN / MOD COMPATIBILITY CHECK" + " " + core.Sym.EmDash + " " + "TARGET: "+targetVersion+core.NC,
		"",
		fmt.Sprintf(
			"  Scanned %d installed plugins / mods against Minecraft %s",
			len(reports), targetVersion,
		),
		"",
	)

	for _, r := range reports {
		name := r.PluginName
		if name == "" {
			name = r.FileName
		}
		lines = append(lines, fmt.Sprintf(
			"  %s  %-28s  %s",
			statusLabel(r.Status),
			name,
			core.DIM+r.Reason+core.NC,
		))
	}

	lines = append(lines, "")

	if riskCount > 0 {
		lines = append(lines,
			core.RED+core.BOLD+fmt.Sprintf(
				"  WARNING: %d plugin(s) flagged as high risk" + " " + core.Sym.EmDash + " " + "they may crash the server.",
				riskCount,
			)+core.NC,
			core.DIM+"  Update or remove those plugins before upgrading the server version."+core.NC,
		)
	} else if unknownCount > 0 {
		lines = append(lines,
			core.YELLOW+fmt.Sprintf(
				"  NOTE: %d plugin(s) could not be verified; they might still work correctly.",
				unknownCount,
			)+core.NC,
		)
	} else {
		lines = append(lines,
			core.GREEN+"  All installed plugins appear compatible with Minecraft "+targetVersion+"."+core.NC,
		)
	}

	tui.PrintInBox("Plugin Compatibility Report", lines)

	choices := []string{
		"Proceed with server update",
		"Abort" + " " + core.Sym.EmDash + " " + "review plugins first",
	}
	if riskCount > 0 {
		choices = []string{
			"Proceed anyway (NOT recommended" + " " + core.Sym.EmDash + " " + "risk of crashes)",
			"Abort" + " " + core.Sym.EmDash + " " + "update plugins first (Recommended)",
		}
	}

	_, opt, st := tui.SelectMenu("Plugin Compatibility" + " " + core.Sym.EmDash + " " + "Choose Action", choices)
	if st != 0 {
		return false
	}
	cleanOpt := core.StripANSI(opt)
	return strings.HasPrefix(cleanOpt, "Proceed")
}
