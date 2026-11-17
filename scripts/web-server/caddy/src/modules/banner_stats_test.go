package modules

import (
	"os"
	"path/filepath"
	"testing"

	"krotek-caddy-installer/core"
)

func TestCountCaddyfileSiteBlocks(t *testing.T) {
	tmp := t.TempDir()
	caddyfile := filepath.Join(tmp, "Caddyfile")
	content := `{
    email admin@example.com
}

(common_headers) {
    header Strict-Transport-Security "max-age=31536000;"
}

:80 {
    root * /var/www/html
    file_server
}

# User sites
app.example.com {
    reverse_proxy 127.0.0.1:3000
}

api.example.com, ws.example.com {
    reverse_proxy 127.0.0.1:4000
}

import /etc/caddy/conf.d/*.caddy
`
	if err := os.WriteFile(caddyfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if got := countCaddyfileSiteBlocks(caddyfile); got != 2 {
		t.Fatalf("expected 2 site blocks (ignoring global, snippet, :80), got %d", got)
	}
}

func TestFormatCaddyCells(t *testing.T) {
	s := WebStats{
		SitesEnabled: 3,
		Upstreams:    2,
		NearestDays:  42,
		Certs:        []certEntry{{Name: "a.com"}, {Name: "b.com"}},
	}
	origMode := core.RichTerminal
	defer core.SetSymbolMode(origMode)
	for _, rich := range []bool{true, false} {
		core.SetSymbolMode(rich)
		if got := core.StripANSI(formatCaddySitesCell(s, true, true)); got != "3 active" {
			t.Fatalf("unexpected sites cell (rich=%v): %q", rich, got)
		}
		if got := core.StripANSI(formatCaddySitesCell(s, true, false)); got != "3 active "+core.Sym.DotSep+" Syntax ERR" {
			t.Fatalf("unexpected syntax error sites cell (rich=%v): %q", rich, got)
		}
		if got := core.StripANSI(formatCaddyAutoHTTPSCell(s, true)); got != "2 certs "+core.Sym.DotSep+" exp 42d" {
			t.Fatalf("unexpected auto-https cell (rich=%v): %q", rich, got)
		}
		if got := core.StripANSI(FormatUpstreamsCell(s)); got != "2 upstreams" {
			t.Fatalf("unexpected proxy cell (rich=%v): %q", rich, got)
		}
	}
}

