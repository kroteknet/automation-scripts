package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"krotek-caddy-installer/core"
	"krotek-caddy-installer/system"
)

func bannerProfile() webProfile {
	state := system.LoadState()
	extra := []string{"/etc/caddy/Caddyfile"}
	var dockerCaddyfile string
	if composeDir := strings.TrimSpace(state["COMPOSE_DIR"]); composeDir != "" {
		dockerCaddyfile = filepath.Join(composeDir, "Caddyfile")
		extra = append(extra, dockerCaddyfile)
	}

	return webProfile{
		Sources: []siteSource{
			{EnabledDir: "/etc/caddy/conf.d", Pattern: "*.caddy"},
		},
		IgnoreStems:      []string{"default"},
		SiteMarker:       regexp.MustCompile(`\{`),
		CertDirective:    regexp.MustCompile(`(?m)^\s*tls\s+(/[^\s]+)\s+[^\s]+`),
		UpstreamMarker:   regexp.MustCompile(`(?m)^\s*reverse_proxy\s+`),
		ExtraConfigFiles: extra,
		CertGlobs: []string{
			"/var/lib/caddy/.local/share/caddy/certificates/*/*/*.crt",
			"/root/.local/share/caddy/certificates/*/*/*.crt",
			"/etc/letsencrypt/live/*/fullchain.pem",
			"/etc/letsencrypt/live/*/cert.pem",
		},
		Ports: uniqueBannerPorts(configuredBannerPort(state, "80"), "443"),
		PostCollect: func(s *WebStats) {
			s.SitesEnabled += countCaddyfileSiteBlocks("/etc/caddy/Caddyfile")
			if dockerCaddyfile != "" && dockerCaddyfile != "/etc/caddy/Caddyfile" {
				s.SitesEnabled += countCaddyfileSiteBlocks(dockerCaddyfile)
			}
		},
	}
}

// countCaddyfileSiteBlocks counts explicit domain/site blocks inside a Caddyfile,
// ignoring global options (`{`), snippets (`(name)`), and the default `:80` block.
func countCaddyfileSiteBlocks(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	count := 0
	depth := 0
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}
		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		if depth == 0 && opens > closes && strings.HasSuffix(line, "{") {
			header := strings.TrimSpace(strings.TrimSuffix(line, "{"))
			if isCaddySiteHeader(header) {
				count++
			}
		}
		depth += opens - closes
		if depth < 0 {
			depth = 0
		}
	}
	return count
}

func isCaddySiteHeader(header string) bool {
	if header == "" || strings.HasPrefix(header, "(") || strings.HasPrefix(header, ":") || strings.HasPrefix(header, "http://:") {
		return false
	}
	return true
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

func formatCaddySitesCell(s WebStats, installed, syntaxOK bool) string {
	base := FormatSitesCell(s, installed)
	if installed && !syntaxOK {
		return base + core.DIM + " " + core.Sym.DotSep + " " + core.NC + core.BOLD + core.RED + "Syntax ERR" + core.NC
	}
	return base
}

func formatCaddyAutoHTTPSCell(s WebStats, installed bool) string {
	if !installed {
		return core.DIM + "N/A" + core.NC
	}
	if s.ExpiredCerts > 0 {
		return FormatSSLCell(s)
	}
	if len(s.Certs) > 0 {
		n := len(s.Certs)
		unit := "certs"
		if n == 1 {
			unit = "cert"
		}
		exp := ""
		warn := false
		if s.NearestDays >= 0 {
			exp = fmt.Sprintf(" " + core.Sym.DotSep + " exp %dd", s.NearestDays)
			if s.NearestDays <= sslWarnDays {
				warn = true
			}
		}
		color := core.BOLD + core.GREEN
		if warn {
			color = core.BOLD + core.YELLOW
		}
		return fmt.Sprintf("%s%d %s%s%s", color, n, unit, exp, core.NC)
	}
	if s.SitesEnabled > 0 {
		return core.YELLOW + "ACME pending" + core.NC
	}
	return core.DIM + "Standby" + core.NC
}

