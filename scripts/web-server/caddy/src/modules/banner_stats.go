// ==============================================================================
// KROTEK SCRIPT HUB — MULTI-SITE BANNER TELEMETRY
// Service: CADDY (web-server)
// File:    web-server/caddy/src/modules/banner_stats.go
//
// Aggregates live, multi-site aware statistics for the header banner: number of
// enabled/disabled virtual hosts, TLS coverage, nearest certificate expiry,
// reverse-proxy upstreams and the listening state of the service ports.
// Expensive probes (x509 parsing, socket tables) are cached between redraws.
// ==============================================================================

package modules

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"krotek-caddy-installer/core"
)

const (
	// sslWarnDays: Let's Encrypt renews 30 days before expiry, so a certificate
	// with 14 days or less left means automatic renewal is failing.
	sslWarnDays     = 14
	statsCacheTTL   = 15 * time.Second
	versionCacheTTL = 5 * time.Minute
	maxSiteFileSize = 1 << 20
)

// siteSource describes a directory where the web server keeps virtual host definitions.
type siteSource struct {
	EnabledDir   string // scanned for active sites
	AvailableDir string // optional: sites present here but missing in EnabledDir are reported as disabled
	Pattern      string // glob pattern, e.g. "*" or "*.conf"
	DirsAsSites  bool   // each site is a directory (OpenLiteSpeed vhosts)
}

// webProfile describes how a specific web server stores sites and certificates.
type webProfile struct {
	Sources          []siteSource
	IgnoreStems      []string         // file stems never counted as user sites (distro defaults)
	SiteMarker       *regexp.Regexp   // file must match to count as a site (nil = every file counts)
	SSLMarker        *regexp.Regexp   // match => site serves TLS (nil = TLS coverage ratio unavailable)
	CertDirective    *regexp.Regexp   // capture group 1 = certificate file path
	UpstreamMarker   *regexp.Regexp   // counts reverse proxy upstream directives
	CertGlobs        []string         // certificate stores scanned regardless of site configs
	ExtraConfigFiles []string         // scanned for certs/upstreams, never counted as sites
	Ports            []string         // ports shown in the banner
	PostCollect      func(*WebStats)  // optional per-server hook (e.g. Caddyfile blocks)
}

type certEntry struct {
	Name     string
	NotAfter time.Time
}

// WebStats is the aggregated multi-site snapshot rendered in the banner.
type WebStats struct {
	SitesEnabled  int
	SitesDisabled int
	SitesSecured  int
	RatioKnown    bool
	Certs         []certEntry
	ExpiredCerts  int
	ExpiredName   string
	NearestDays   int // -1 when no valid certificate was found
	NearestName   string
	Upstreams     int
	Ports         []string
	Listening     map[string]bool
}

var (
	cachedStats    WebStats
	statsCheckedAt time.Time
	cachedVersion  string
	versionChecked time.Time
)

// CollectWebStats returns the cached multi-site snapshot, refreshing it every statsCacheTTL.
func CollectWebStats() WebStats {
	if !statsCheckedAt.IsZero() && time.Since(statsCheckedAt) < statsCacheTTL {
		return cachedStats
	}
	cachedStats = collectWebStats(bannerProfile())
	statsCheckedAt = time.Now()
	return cachedStats
}

func collectWebStats(p webProfile) WebStats {
	s := WebStats{NearestDays: -1, Ports: p.Ports, RatioKnown: p.SSLMarker != nil}

	ignored := map[string]bool{}
	for _, n := range p.IgnoreStems {
		ignored[strings.ToLower(n)] = true
	}

	enabled := map[string]bool{}
	secured := map[string]bool{}
	var configFiles []string

	// Pass 1: enabled sites
	for _, src := range p.Sources {
		for _, m := range globSites(src.EnabledDir, src.Pattern, src.DirsAsSites) {
			stem := siteStem(m)
			canon := strings.TrimSuffix(stem, "-le-ssl")
			if ignored[stem] || ignored[canon] {
				continue
			}
			files := siteFiles(m, src.DirsAsSites)
			content := readFiles(files)
			if p.SiteMarker != nil && !p.SiteMarker.MatchString(content) {
				continue
			}
			if !enabled[canon] {
				enabled[canon] = true
				s.SitesEnabled++
			}
			if p.SSLMarker != nil && !secured[canon] && p.SSLMarker.MatchString(content) {
				secured[canon] = true
				s.SitesSecured++
			}
			configFiles = append(configFiles, files...)
		}
	}

	// Pass 2: disabled sites (available but not enabled)
	disabled := map[string]bool{}
	for _, src := range p.Sources {
		if src.AvailableDir == "" || src.AvailableDir == src.EnabledDir {
			continue
		}
		for _, m := range globSites(src.AvailableDir, src.Pattern, src.DirsAsSites) {
			stem := siteStem(m)
			canon := strings.TrimSuffix(stem, "-le-ssl")
			if ignored[stem] || ignored[canon] || enabled[canon] || disabled[canon] {
				continue
			}
			if p.SiteMarker != nil && !p.SiteMarker.MatchString(readFiles(siteFiles(m, src.DirsAsSites))) {
				continue
			}
			disabled[canon] = true
			s.SitesDisabled++
		}
	}

	// Certificates and upstreams referenced by configuration
	certPaths := []string{}
	for _, g := range p.CertGlobs {
		if matches, err := filepath.Glob(g); err == nil {
			certPaths = append(certPaths, matches...)
		}
	}
	allConfig := append(append([]string{}, configFiles...), p.ExtraConfigFiles...)
	for _, f := range allConfig {
		content := readFiles([]string{f})
		if content == "" {
			continue
		}
		if p.CertDirective != nil {
			for _, m := range p.CertDirective.FindAllStringSubmatch(content, -1) {
				if len(m) > 1 && filepath.IsAbs(m[1]) && !strings.Contains(m[1], "$") {
					certPaths = append(certPaths, m[1])
				}
			}
		}
		if p.UpstreamMarker != nil {
			s.Upstreams += len(p.UpstreamMarker.FindAllStringIndex(content, -1))
		}
	}
	scanCertificates(&s, certPaths, time.Now())

	s.Listening = listeningPorts(p.Ports)
	if p.PostCollect != nil {
		p.PostCollect(&s)
	}
	return s
}

func globSites(dir, pattern string, dirsAsSites bool) []string {
	if dir == "" {
		return nil
	}
	if pattern == "" {
		pattern = "*"
	}
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range matches {
		fi, err := os.Stat(m) // follows symlinks, skips broken links
		if err != nil || fi.IsDir() != dirsAsSites {
			continue
		}
		base := filepath.Base(m)
		if strings.HasPrefix(base, ".") || strings.HasSuffix(base, "~") ||
			strings.HasSuffix(base, ".bak") || strings.HasSuffix(base, ".disabled") ||
			strings.HasSuffix(base, ".swp") || strings.HasSuffix(base, ".dpkg-old") {
			continue
		}
		out = append(out, m)
	}
	return out
}

func siteFiles(path string, isDir bool) []string {
	if !isDir {
		return []string{path}
	}
	matches, _ := filepath.Glob(filepath.Join(path, "*.conf"))
	return matches
}

func siteStem(path string) string {
	base := strings.ToLower(filepath.Base(path))
	for _, ext := range []string{".conf", ".caddy", ".vhost"} {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

func readFiles(files []string) string {
	var b strings.Builder
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil || fi.IsDir() || fi.Size() > maxSiteFileSize {
			continue
		}
		if data, err := os.ReadFile(f); err == nil {
			b.Write(data)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// scanCertificates parses leaf certificates, de-duplicating by issuer+serial
// (cert.pem and fullchain.pem of the same Let's Encrypt lineage are one cert).
func scanCertificates(s *WebStats, paths []string, now time.Time) {
	seen := map[string]bool{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var block *pem.Block
		rest := data
		for {
			block, rest = pem.Decode(rest)
			if block == nil || block.Type == "CERTIFICATE" {
				break
			}
		}
		if block == nil {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		key := cert.Issuer.String() + "|" + cert.SerialNumber.String()
		if seen[key] {
			continue
		}
		seen[key] = true

		name := cert.Subject.CommonName
		if len(cert.DNSNames) > 0 {
			name = cert.DNSNames[0]
		}
		s.Certs = append(s.Certs, certEntry{Name: name, NotAfter: cert.NotAfter})

		if now.After(cert.NotAfter) {
			s.ExpiredCerts++
			if s.ExpiredName == "" {
				s.ExpiredName = name
			}
			continue
		}
		days := int(cert.NotAfter.Sub(now).Hours() / 24)
		if s.NearestDays < 0 || days < s.NearestDays {
			s.NearestDays = days
			s.NearestName = name
		}
	}
}

// listeningPorts checks the socket table once for all requested ports.
func listeningPorts(ports []string) map[string]bool {
	res := map[string]bool{}
	var out []byte
	if path, err := exec.LookPath("ss"); err == nil {
		out, _ = exec.Command(path, "-tuln").Output()
	} else if path, err := exec.LookPath("netstat"); err == nil {
		out, _ = exec.Command(path, "-tuln").Output()
	}
	return parseListeningPorts(string(out), ports, res)
}

func parseListeningPorts(table string, ports []string, res map[string]bool) map[string]bool {
	for _, line := range strings.Split(table, "\n") {
		for _, field := range strings.Fields(line) {
			idx := strings.LastIndex(field, ":")
			if idx < 0 {
				continue
			}
			port := field[idx+1:]
			for _, want := range ports {
				if port == want {
					res[want] = true
				}
			}
		}
	}
	return res
}

// cachedServerVersion memoizes the (relatively slow) version probe.
func cachedServerVersion(probe func() string) string {
	if cachedVersion != "" && time.Since(versionChecked) < versionCacheTTL {
		return cachedVersion
	}
	if !versionChecked.IsZero() && cachedVersion == "" && time.Since(versionChecked) < 30*time.Second {
		return ""
	}
	cachedVersion = probe()
	versionChecked = time.Now()
	return cachedVersion
}

var versionRe = regexp.MustCompile(`\d+\.\d+(\.\d+)*`)

// commandVersion runs a command and extracts the first version-like token
// found after the given marker (e.g. "nginx/").
func commandVersion(marker string, name string, args ...string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	out, _ := exec.Command(path, args...).CombinedOutput()
	text := string(out)
	if marker != "" {
		idx := strings.Index(text, marker)
		if idx < 0 {
			return ""
		}
		text = text[idx+len(marker):]
	}
	return versionRe.FindString(text)
}

// ---------------------------------------------------------------- formatting

// FormatSitesCell renders e.g. "4 active · 1 off" or "Default only".
func FormatSitesCell(s WebStats, installed bool) string {
	if !installed && s.SitesEnabled == 0 && s.SitesDisabled == 0 {
		return core.DIM + "N/A" + core.NC
	}
	off := ""
	if s.SitesDisabled > 0 {
		off = fmt.Sprintf("%s %s %d off%s", core.DIM, core.Sym.DotSep, s.SitesDisabled, core.NC)
	}
	if s.SitesEnabled == 0 {
		return core.DIM + "Default only" + core.NC + off
	}
	return fmt.Sprintf("%s%d active%s%s", core.BOLD+core.GREEN, s.SitesEnabled, core.NC, off)
}

// FormatSSLCell renders TLS coverage and the nearest expiry, coloured by the worst state.
func FormatSSLCell(s WebStats) string {
	if s.ExpiredCerts > 0 {
		label := fmt.Sprintf("%d expired", s.ExpiredCerts)
		if s.ExpiredCerts == 1 && s.ExpiredName != "" {
			name := s.ExpiredName
			if len(name) > 15 {
				name = name[:12] + "..."
			}
			label = "EXPIRED " + name
		}
		return core.BOLD + core.RED + label + core.NC
	}
	if len(s.Certs) == 0 && s.SitesSecured == 0 {
		if s.SitesEnabled > 0 {
			return core.YELLOW + fmt.Sprintf("0/%d sites", s.SitesEnabled) + core.NC
		}
		return core.DIM + "No certs" + core.NC
	}

	var main string
	warn := false
	if s.RatioKnown && s.SitesEnabled > 0 {
		main = fmt.Sprintf("%d/%d sites", s.SitesSecured, s.SitesEnabled)
		warn = s.SitesSecured < s.SitesEnabled
	} else {
		n := len(s.Certs)
		unit := "certs"
		if n == 1 {
			unit = "cert"
		}
		main = fmt.Sprintf("%d %s", n, unit)
	}
	exp := ""
	if s.NearestDays >= 0 {
		exp = fmt.Sprintf(" %s exp %dd", core.Sym.DotSep, s.NearestDays)
		if s.NearestDays <= sslWarnDays {
			warn = true
		}
	}
	color := core.BOLD + core.GREEN
	if warn {
		color = core.BOLD + core.YELLOW
	}
	return color + main + exp + core.NC
}

// FormatPortsCell renders e.g. "80 ✓ 443 ✗" based on the live socket table.
func FormatPortsCell(s WebStats) string {
	parts := make([]string, 0, len(s.Ports))
	for _, p := range s.Ports {
		if s.Listening[p] {
			parts = append(parts, core.BOLD+p+core.NC+core.GREEN+" "+core.Sym.Check+core.NC)
		} else {
			parts = append(parts, core.BOLD+p+core.NC+core.RED+" "+core.Sym.CrossMark+core.NC)
		}
	}
	if len(parts) == 0 {
		return core.DIM + "N/A" + core.NC
	}
	return strings.Join(parts, " ")
}

// FormatConfigCell renders "Syntax OK · v1.26.2" or "Syntax ERROR".
func FormatConfigCell(installed, syntaxOK bool, details ...string) string {
	if !installed {
		return core.DIM + "N/A" + core.NC
	}
	if !syntaxOK {
		return core.BOLD + core.RED + "Syntax ERROR" + core.NC
	}
	extra := ""
	for _, d := range details {
		if d != "" {
			extra += " " + core.Sym.DotSep + " " + d
		}
	}
	if extra != "" {
		extra = core.DIM + extra + core.NC
	}
	return core.BOLD + core.GREEN + "Syntax OK" + core.NC + extra
}

// FormatUpstreamsCell renders the number of reverse proxy upstream directives.
func FormatUpstreamsCell(s WebStats) string {
	switch s.Upstreams {
	case 0:
		return core.DIM + "None" + core.NC
	case 1:
		return core.BOLD + "1 upstream" + core.NC
	default:
		return fmt.Sprintf("%s%d upstreams%s", core.BOLD, s.Upstreams, core.NC)
	}
}
