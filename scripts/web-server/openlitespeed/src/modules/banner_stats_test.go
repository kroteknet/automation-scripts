package modules

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"krotek-openlitespeed-installer/core"
)

func TestOpenLiteSpeedDirsAsSitesAndFormatting(t *testing.T) {
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

	mustWrite("vhosts/Example/vhconf.conf", "docRoot $VH_ROOT/html\n")
	mustWrite("vhosts/app.example.com/vhconf.conf", "docRoot /var/www/app\nvhssl {\n  certFile /etc/letsencrypt/live/app.example.com/fullchain.pem\n}\n")
	mustWrite("vhosts/blog.example.com/vhconf.conf", "docRoot /var/www/blog\n")

	stats := collectWebStats(webProfile{
		Sources:     []siteSource{{EnabledDir: filepath.Join(tmp, "vhosts"), DirsAsSites: true}},
		IgnoreStems: []string{"example", "localhost"},
		SiteMarker:  regexp.MustCompile(`\S`),
		SSLMarker:   regexp.MustCompile(`(?m)\bvhssl\b|\bcertFile\s+`),
		Ports:       []string{"80", "443", "7080"},
	})

	if stats.SitesEnabled != 2 || stats.SitesSecured != 1 {
		t.Fatalf("expected 2 sites with 1 SSL, got enabled=%d secured=%d", stats.SitesEnabled, stats.SitesSecured)
	}
	t.Cleanup(core.RefreshSymbols)
	for _, tc := range []struct {
		rich bool
		want string
	}{
		{rich: true, want: "2 active · 1/2 SSL"},
		{rich: false, want: "2 active | 1/2 SSL"},
	} {
		core.SetSymbolMode(tc.rich)
		if got := core.StripANSI(formatOLSVHostsCell(stats, true)); got != tc.want {
			t.Fatalf("unexpected OLS vhosts cell (rich=%v): %q, want %q", tc.rich, got, tc.want)
		}
	}
}

