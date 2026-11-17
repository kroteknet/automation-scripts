// ==============================================================================
// KROTEK SCRIPT HUB — DYNAMIC VERSION METADATA (MOJANG) & HTTP HELPERS
// Service: Minecraft Server (game-servers/minecraft)
// File:    game-servers/minecraft/src/system/versions.go
//
// Resolves everything that used to be hardcoded (latest Minecraft release, the Java
// major version a given Minecraft version needs, the official server.jar URL) from
// Mojang's live version manifest.
// ==============================================================================

package system

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"krotek-minecraft-installer/core"
)

const mojangManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

// MojangVersionEntry is a single version record from the Mojang manifest.
type MojangVersionEntry struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

// MojangManifest mirrors the fields of version_manifest_v2.json used by this tool.
type MojangManifest struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []MojangVersionEntry `json:"versions"`
}

var (
	mojangManifestMu    sync.Mutex
	mojangManifestCache *MojangManifest
)

// HTTPGetJSON performs a GET request with the Krotek User-Agent and decodes a JSON body into out.
func HTTPGetJSON(url string, timeout time.Duration, out interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", core.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out)
}

// HTTPGetText performs a GET request with the Krotek User-Agent and returns the trimmed body.
func HTTPGetText(url string, timeout time.Duration) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", core.UserAgent)

	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

// FetchMojangManifest returns the (cached per-process) official Mojang version manifest.
func FetchMojangManifest() (*MojangManifest, error) {
	mojangManifestMu.Lock()
	defer mojangManifestMu.Unlock()

	if mojangManifestCache != nil {
		return mojangManifestCache, nil
	}
	var m MojangManifest
	if err := HTTPGetJSON(mojangManifestURL, 8*time.Second, &m); err != nil {
		return nil, err
	}
	if len(m.Versions) == 0 {
		return nil, fmt.Errorf("Mojang manifest contained no versions")
	}
	mojangManifestCache = &m
	return mojangManifestCache, nil
}

// LatestMinecraftRelease returns the newest stable Minecraft release id (e.g. "1.21.4" or "26.1").
func LatestMinecraftRelease() (string, error) {
	m, err := FetchMojangManifest()
	if err != nil {
		return "", err
	}
	if m.Latest.Release == "" {
		return "", fmt.Errorf("Mojang manifest did not report a latest release")
	}
	return m.Latest.Release, nil
}

// MojangReleaseVersions returns all stable release ids, newest first.
func MojangReleaseVersions() ([]string, error) {
	m, err := FetchMojangManifest()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, v := range m.Versions {
		if v.Type == "release" {
			out = append(out, v.ID)
		}
	}
	return out, nil
}

type mojangPackage struct {
	Downloads struct {
		Server struct {
			URL  string `json:"url"`
			SHA1 string `json:"sha1"`
		} `json:"server"`
	} `json:"downloads"`
	JavaVersion struct {
		MajorVersion int `json:"majorVersion"`
	} `json:"javaVersion"`
}

func fetchMojangPackage(mcVersion string) (*mojangPackage, error) {
	m, err := FetchMojangManifest()
	if err != nil {
		return nil, err
	}
	for _, v := range m.Versions {
		if v.ID == mcVersion {
			var pkg mojangPackage
			if err := HTTPGetJSON(v.URL, 8*time.Second, &pkg); err != nil {
				return nil, err
			}
			return &pkg, nil
		}
	}
	return nil, fmt.Errorf("Minecraft version %q is not listed in the Mojang manifest", mcVersion)
}

// MojangServerJarURL resolves the official server.jar download URL for a Minecraft version.
func MojangServerJarURL(mcVersion string) (string, error) {
	pkg, err := fetchMojangPackage(mcVersion)
	if err != nil {
		return "", err
	}
	if pkg.Downloads.Server.URL == "" {
		return "", fmt.Errorf("Mojang publishes no server download for %s", mcVersion)
	}
	return pkg.Downloads.Server.URL, nil
}

// MojangJavaMajor returns the Java major version Mojang declares for a Minecraft version.
func MojangJavaMajor(mcVersion string) (int, error) {
	pkg, err := fetchMojangPackage(mcVersion)
	if err != nil {
		return 0, err
	}
	if pkg.JavaVersion.MajorVersion <= 0 {
		return 0, fmt.Errorf("Mojang metadata for %s has no javaVersion", mcVersion)
	}
	return pkg.JavaVersion.MajorVersion, nil
}

// NormalizeJavaLTS maps a declared Java major to the nearest installable LTS release,
// because distro / Adoptium packages only exist for LTS lines (8, 11, 17, 21, 25...).
func NormalizeJavaLTS(v int) int {
	switch {
	case v <= 8:
		return 8
	case v <= 11:
		return 11
	case v <= 17:
		return 17
	case v <= 21:
		return 21
	case v <= 25:
		return 25
	default:
		return v
	}
}

// EngineKey converts a persisted display name ("PaperMC", "NeoForge"...) to a stable engine type id.
func EngineKey(engine string) string {
	e := strings.ToLower(engine)
	switch {
	case strings.Contains(e, "paper"):
		return "paper"
	case strings.Contains(e, "purpur"):
		return "purpur"
	case strings.Contains(e, "vanilla"):
		return "vanilla"
	case strings.Contains(e, "neoforge"):
		return "neoforge"
	case strings.Contains(e, "fabric"):
		return "fabric"
	}
	return "paper"
}
