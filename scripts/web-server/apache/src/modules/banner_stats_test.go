package modules

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"krotek-apache-installer/core"
)

func TestApacheVirtualHostAndCertbotPair(t *testing.T) {
	tmp := t.TempDir()
	mustWrite := func(rel, content string) {
		t.Helper()
		p := filepath.Join(tmp, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("sites-enabled/000-default.conf", "<VirtualHost *:80>\n</VirtualHost>\n")
	mustWrite("sites-enabled/shop.example.com.conf", "<VirtualHost *:80>\n  ServerName shop.example.com\n</VirtualHost>\n")
	mustWrite("sites-enabled/shop.example.com-le-ssl.conf", "<VirtualHost *:443>\n  SSLEngine on\n</VirtualHost>\n")
	mustWrite("conf.d/welcome.conf", "# Non-vhost Apache snippet\nAlias /noindex /usr/share/httpd/noindex\n")
	mustWrite("sites-available/old.example.com.conf", "<VirtualHost *:80>\n  ServerName old.example.com\n</VirtualHost>\n")

	stats := collectWebStats(webProfile{
		Sources: []siteSource{
			{
				EnabledDir:   filepath.Join(tmp, "sites-enabled"),
				AvailableDir: filepath.Join(tmp, "sites-available"),
				Pattern:      "*.conf",
			},
			{
				EnabledDir: filepath.Join(tmp, "conf.d"),
				Pattern:    "*.conf",
			},
		},
		IgnoreStems: []string{"000-default", "default-ssl", "000-default-le-ssl", "default", "ssl"},
		SiteMarker:  regexp.MustCompile(`(?is)<VirtualHost\b`),
		SSLMarker:   regexp.MustCompile(`(?im)^\s*SSLEngine\s+on\b|^\s*SSLCertificateFile\s+`),
		Ports:       []string{"80", "443"},
	})

	if stats.SitesEnabled != 1 || stats.SitesDisabled != 1 || stats.SitesSecured != 1 {
		t.Fatalf("unexpected apache stats: %+v", stats)
	}
	origModeSites := core.RichTerminal; core.SetSymbolMode(true); defer core.SetSymbolMode(origModeSites)
	if got := core.StripANSI(FormatSitesCell(stats, true)); got != (func() string { return "1 active " + core.Sym.DotSep + " 1 off" })() {
		t.Fatalf("unexpected sites cell: %q", got)
	}
}

