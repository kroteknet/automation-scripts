package modules

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"krotek-apache-installer/core"
	"krotek-apache-installer/system"
)

func bannerProfile() webProfile {
	state := system.LoadState()
	sources := []siteSource{
		{EnabledDir: "/etc/apache2/sites-enabled", AvailableDir: "/etc/apache2/sites-available", Pattern: "*.conf"},
		{EnabledDir: "/etc/apache2/conf.d", Pattern: "*.conf"},
		{EnabledDir: "/etc/httpd/conf.d", Pattern: "*.conf"},
	}
	if composeDir := strings.TrimSpace(state["COMPOSE_DIR"]); composeDir != "" {
		sources = append(sources, siteSource{
			EnabledDir: filepath.Join(composeDir, "conf.d"),
			Pattern:    "*.conf",
		})
	}

	return webProfile{
		Sources:       sources,
		IgnoreStems:   []string{"000-default", "default-ssl", "000-default-le-ssl", "default", "ssl"},
		SiteMarker:    regexp.MustCompile(`(?is)<VirtualHost\b`),
		SSLMarker:     regexp.MustCompile(`(?im)^\s*SSLEngine\s+on\b|^\s*SSLCertificateFile\s+`),
		CertDirective: regexp.MustCompile(`(?im)^\s*SSLCertificateFile\s+"?([^"\s#]+)"?`),
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

func probeApacheEngine(envType string) string {
	var ctlArgs [][]string
	if _, err := exec.LookPath("apache2ctl"); err == nil {
		ctlArgs = append(ctlArgs, []string{"apache2ctl"})
	} else if _, err := exec.LookPath("httpd"); err == nil {
		ctlArgs = append(ctlArgs, []string{"httpd"})
	} else if envType == "docker" {
		ctlArgs = append(ctlArgs, []string{"docker", "exec", "krotek-apache", "httpd"})
	}
	if len(ctlArgs) == 0 {
		return ""
	}
	base := ctlArgs[0]
	run := func(flag string) string {
		args := append(append([]string{}, base[1:]...), flag)
		out, _ := exec.Command(base[0], args...).CombinedOutput()
		return string(out)
	}

	mpm := ""
	for _, line := range strings.Split(run("-V"), "\n") {
		if strings.Contains(line, "Server MPM:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				mpm = strings.ToLower(strings.TrimSpace(parts[1]))
			}
			break
		}
	}

	mods := run("-M")
	proto := "HTTP/1.1"
	if strings.Contains(mods, "http2_module") {
		proto = "HTTP/2"
	}

	if mpm == "" {
		return proto
	}
	return fmt.Sprintf("%s " + core.Sym.DotSep + " %s", mpm, proto)
}

func formatApacheMPMCell(installed, syntaxOK bool, envType string) string {
	if !installed {
		return core.DIM + "N/A" + core.NC
	}
	if !syntaxOK {
		return core.BOLD + core.RED + "Syntax ERROR" + core.NC
	}
	detail := cachedServerVersion(func() string { return probeApacheEngine(envType) })
	if detail == "" {
		return core.BOLD + core.GREEN + "Syntax OK" + core.NC
	}
	if strings.Contains(detail, "HTTP/2") {
		return core.BOLD + core.GREEN + detail + core.NC
	}
	return core.BOLD + detail + core.NC
}

