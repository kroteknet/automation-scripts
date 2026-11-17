package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"krotek-openlitespeed-installer/core"
	"krotek-openlitespeed-installer/system"
)

func bannerProfile() webProfile {
	state := system.LoadState()
	return webProfile{
		Sources: []siteSource{
			{EnabledDir: "/usr/local/lsws/conf/vhosts", DirsAsSites: true},
		},
		IgnoreStems:   []string{"example", "localhost"},
		SiteMarker:    regexp.MustCompile(`\S`),
		SSLMarker:     regexp.MustCompile(`(?m)\bvhssl\b|\bcertFile\s+`),
		CertDirective: regexp.MustCompile(`(?m)^\s*certFile\s+([^\s]+)`),
		CertGlobs: []string{
			"/etc/letsencrypt/live/*/fullchain.pem",
			"/etc/letsencrypt/live/*/cert.pem",
		},
		Ports: uniqueBannerPorts(configuredBannerPort(state, "80"), "443", "7080"),
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

func probeLSPHP() string {
	dirs, err := filepath.Glob("/usr/local/lsws/lsphp*")
	if err != nil {
		return ""
	}
	var found []string
	for _, d := range dirs {
		fi, err := os.Stat(d)
		if err != nil || !fi.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(d, "bin", "php")); err == nil {
			found = append(found, filepath.Base(d))
		} else if _, err := os.Stat(filepath.Join(d, "bin", "lsphp")); err == nil {
			found = append(found, filepath.Base(d))
		}
	}
	if len(found) == 0 {
		return ""
	}
	sort.Sort(sort.Reverse(sort.StringSlice(found)))
	if len(found) == 1 {
		return found[0]
	}
	if len(found) == 2 {
		return fmt.Sprintf("%s (+%s)", found[0], strings.TrimPrefix(found[1], "lsphp"))
	}
	return fmt.Sprintf("%s (+%d)", found[0], len(found)-1)
}

func formatLSPHPCell(installed bool) string {
	if !installed {
		return core.DIM + "N/A" + core.NC
	}
	val := cachedServerVersion(probeLSPHP)
	if val == "" {
		return core.DIM + "Not installed" + core.NC
	}
	return core.BOLD + core.GREEN + val + core.NC
}

func formatAdminUICell(ipAddr string, installed, listening bool) string {
	url := fmt.Sprintf("https://%s:7080", ipAddr)
	if !installed {
		return core.DIM + "N/A" + core.NC
	}
	if listening {
		return core.BOLD + core.GREEN + url + core.NC
	}
	return core.YELLOW + url + " (off)" + core.NC
}

func formatOLSVHostsCell(s WebStats, installed bool) string {
	base := FormatSitesCell(s, installed)
	if !installed || s.SitesEnabled == 0 {
		return base
	}
	if s.ExpiredCerts > 0 {
		return base + core.DIM + " " + core.Sym.DotSep + " " + core.NC + core.BOLD + core.RED + "SSL EXP" + core.NC
	}
	if s.SitesSecured > 0 {
		color := core.GREEN
		if s.SitesSecured < s.SitesEnabled || (s.NearestDays >= 0 && s.NearestDays <= sslWarnDays) {
			color = core.YELLOW
		}
		sslTag := fmt.Sprintf("%d/%d SSL", s.SitesSecured, s.SitesEnabled)
		if s.NearestDays >= 0 && s.NearestDays <= sslWarnDays {
			sslTag += fmt.Sprintf(" (%dd)", s.NearestDays)
		}
		return base + core.DIM + " " + core.Sym.DotSep + " " + core.NC + color + sslTag + core.NC
	}
	return base
}

