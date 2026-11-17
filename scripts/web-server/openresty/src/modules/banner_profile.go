package modules

import (
	"path/filepath"
	"regexp"
	"strings"

	"krotek-openresty-installer/system"
)

func bannerProfile() webProfile {
	state := system.LoadState()
	availDir, enabledDir := system.GetOpenRestyVHostDirs()
	sources := []siteSource{
		{EnabledDir: enabledDir, AvailableDir: availDir},
		{EnabledDir: "/etc/nginx/conf.d", Pattern: "*.conf"},
		{EnabledDir: "/usr/local/openresty/nginx/conf/conf.d", Pattern: "*.conf"},
	}
	if composeDir := strings.TrimSpace(state["COMPOSE_DIR"]); composeDir != "" {
		sources = append(sources, siteSource{
			EnabledDir: filepath.Join(composeDir, "conf.d"),
			Pattern:    "*.conf",
		})
	}

	return webProfile{
		Sources:       sources,
		IgnoreStems:   []string{"default", "default-ssl"},
		SiteMarker:    regexp.MustCompile(`(?m)\bserver\s*\{`),
		SSLMarker:     regexp.MustCompile(`(?m)^\s*listen\s+[^;#]*\bssl\b|^\s*ssl_certificate\s+`),
		CertDirective: regexp.MustCompile(`(?m)^\s*ssl_certificate\s+([^;\s]+)\s*;`),
		CertGlobs: []string{
			"/etc/letsencrypt/live/*/fullchain.pem",
			"/etc/letsencrypt/live/*/cert.pem",
		},
		Ports: uniqueBannerPorts(system.GetConfiguredPort(), "443"),
	}
}

func uniqueBannerPorts(ports ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func openrestyVersionDetail(envType string) string {
	bin := system.GetOpenRestyBinPath()
	if bin == "" {
		bin = "openresty"
	}
	v := commandVersion("openresty/", bin, "-v")
	if v == "" {
		v = commandVersion("nginx/", bin, "-v")
	}
	if v == "" && envType == "docker" {
		v = commandVersion("openresty/", "docker", "exec", "krotek-openresty", "openresty", "-v")
	}
	if v != "" {
		return "v" + v
	}
	return ""
}

