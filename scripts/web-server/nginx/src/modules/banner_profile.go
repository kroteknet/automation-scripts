package modules

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"krotek-nginx-installer/system"
)

func bannerProfile() webProfile {
	state := system.LoadState()
	sources := []siteSource{
		{EnabledDir: "/etc/nginx/sites-enabled", AvailableDir: "/etc/nginx/sites-available"},
		{EnabledDir: "/etc/nginx/conf.d", Pattern: "*.conf"},
		{EnabledDir: "/etc/nginx/http.d", Pattern: "*.conf"},
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
		Ports: uniqueBannerPorts(configuredBannerPort(state, "80"), "443"),
	}
}

func configuredBannerPort(state map[string]string, fallback string) string {
	p := strings.TrimSpace(state["PORT"])
	if num, err := strconv.Atoi(p); err == nil && num >= 1 && num <= 65535 {
		return p
	}
	return fallback
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

func nginxVersionDetail(envType string) string {
	v := commandVersion("nginx/", "nginx", "-v")
	if v == "" && envType == "docker" {
		v = commandVersion("nginx/", "docker", "exec", "krotek-nginx", "nginx", "-v")
	}
	if v != "" {
		return "v" + v
	}
	return ""
}

