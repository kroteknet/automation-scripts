package modules

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"krotek-nginx-installer/core"
)

func writeTestCertPEM(t *testing.T, path, cn string, serial int64, notAfter time.Time) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     []string{cn},
		NotBefore:    notAfter.Add(-90 * 24 * time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, pemBytes, 0644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
}

func testNginxProfile(root string) webProfile {
	return webProfile{
		Sources: []siteSource{
			{
				EnabledDir:   filepath.Join(root, "sites-enabled"),
				AvailableDir: filepath.Join(root, "sites-available"),
			},
			{
				EnabledDir: filepath.Join(root, "conf.d"),
				Pattern:    "*.conf",
			},
		},
		IgnoreStems:   []string{"default", "default-ssl"},
		SiteMarker:    regexp.MustCompile(`(?m)\bserver\s*\{`),
		SSLMarker:     regexp.MustCompile(`(?m)^\s*listen\s+[^;#]*\bssl\b|^\s*ssl_certificate\s+`),
		CertDirective: regexp.MustCompile(`(?m)^\s*ssl_certificate\s+([^;\s]+)\s*;`),
		CertGlobs: []string{
			filepath.Join(root, "letsencrypt", "live", "*", "fullchain.pem"),
			filepath.Join(root, "letsencrypt", "live", "*", "cert.pem"),
		},
		Ports: []string{"80", "443"},
	}
}

func TestCollectWebStats_DefaultOnly(t *testing.T) {
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

	mustWrite("sites-enabled/default", "server { listen 80; }")
	mustWrite("conf.d/default.conf", "server { listen 80; }")

	stats := collectWebStats(testNginxProfile(tmp))
	if stats.SitesEnabled != 0 || stats.SitesDisabled != 0 {
		t.Fatalf("expected 0 user sites, got enabled=%d disabled=%d", stats.SitesEnabled, stats.SitesDisabled)
	}
	if got := core.StripANSI(FormatSitesCell(stats, true)); got != "Default only" {
		t.Fatalf("expected Default only, got %q", got)
	}
	if got := core.StripANSI(FormatSSLCell(stats)); got != "No certs" {
		t.Fatalf("expected No certs, got %q", got)
	}
}

func TestCollectWebStats_MultiSitePartialSSLAndDisabled(t *testing.T) {
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

	certA := filepath.Join(tmp, "letsencrypt", "live", "app.example.com", "fullchain.pem")
	certAFallback := filepath.Join(tmp, "letsencrypt", "live", "app.example.com", "cert.pem")
	writeTestCertPEM(t, certA, "app.example.com", 101, time.Now().Add(45*24*time.Hour))
	// Copy exact same cert bytes as cert.pem to verify serial deduplication
	data, _ := os.ReadFile(certA)
	_ = os.WriteFile(certAFallback, data, 0644)

	certB := filepath.Join(tmp, "letsencrypt", "live", "api.example.com", "fullchain.pem")
	writeTestCertPEM(t, certB, "api.example.com", 102, time.Now().Add(10*24*time.Hour))

	mustWrite("sites-enabled/default", "server { listen 80; }")
	mustWrite("sites-enabled/app.example.com", "server {\n listen 443 ssl;\n ssl_certificate "+certA+";\n}")
	mustWrite("sites-enabled/api.example.com", "server {\n listen 443 ssl;\n ssl_certificate "+certB+";\n}")
	mustWrite("conf.d/blog.example.com.conf", "server {\n listen 80;\n server_name blog.example.com;\n}")
	mustWrite("sites-available/staging.example.com", "server {\n listen 80;\n}")

	stats := collectWebStats(testNginxProfile(tmp))
	if stats.SitesEnabled != 3 || stats.SitesDisabled != 1 {
		t.Fatalf("expected enabled=3 disabled=1, got enabled=%d disabled=%d", stats.SitesEnabled, stats.SitesDisabled)
	}
	if stats.SitesSecured != 2 {
		t.Fatalf("expected secured=2, got %d", stats.SitesSecured)
	}
	if len(stats.Certs) != 2 {
		t.Fatalf("expected 2 deduplicated certs, got %d", len(stats.Certs))
	}
	if stats.NearestDays < 9 || stats.NearestDays > 10 {
		t.Fatalf("expected nearest expiry ~9-10d, got %d", stats.NearestDays)
	}

	origMode := core.RichTerminal
	defer core.SetSymbolMode(origMode)
	core.SetSymbolMode(true)

	sitesText := core.StripANSI(FormatSitesCell(stats, true))
	if sitesText != "3 active · 1 off" {
		t.Fatalf("unexpected sites cell: %q", sitesText)
	}

	sslRaw := FormatSSLCell(stats)
	sslText := core.StripANSI(sslRaw)
	if !strings.HasPrefix(sslText, "2/3 sites · exp ") {
		t.Fatalf("unexpected ssl cell: %q", sslText)
	}
	if !strings.Contains(sslRaw, core.YELLOW) {
		t.Fatalf("expected warning yellow color for partial SSL / <14d expiry")
	}

	core.SetSymbolMode(false)
	if asciiSites := core.StripANSI(FormatSitesCell(stats, true)); asciiSites != "3 active | 1 off" {
		t.Fatalf("unexpected ASCII sites cell: %q", asciiSites)
	}
	if asciiSSL := core.StripANSI(FormatSSLCell(stats)); !strings.HasPrefix(asciiSSL, "2/3 sites | exp ") {
		t.Fatalf("unexpected ASCII ssl cell: %q", asciiSSL)
	}
}

func TestCollectWebStats_CertbotLeSslPairAndExpired(t *testing.T) {
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

	certExp := filepath.Join(tmp, "ssl", "shop.example.com.pem")
	writeTestCertPEM(t, certExp, "shop.example.com", 201, time.Now().Add(-48*time.Hour))

	mustWrite("sites-enabled/shop.example.com.conf", "server { listen 80; }")
	mustWrite("sites-enabled/shop.example.com-le-ssl.conf", "server {\n listen 443 ssl;\n ssl_certificate "+certExp+";\n}")

	stats := collectWebStats(testNginxProfile(tmp))
	if stats.SitesEnabled != 1 || stats.SitesSecured != 1 {
		t.Fatalf("expected certbot pair to collapse to 1/1, got enabled=%d secured=%d", stats.SitesEnabled, stats.SitesSecured)
	}
	if stats.ExpiredCerts != 1 {
		t.Fatalf("expected 1 expired cert, got %d", stats.ExpiredCerts)
	}
	sslRaw := FormatSSLCell(stats)
	if !strings.Contains(sslRaw, core.RED) || !strings.Contains(core.StripANSI(sslRaw), "EXPIRED") {
		t.Fatalf("expected red EXPIRED badge, got %q", core.StripANSI(sslRaw))
	}
}

func TestParseListeningPortsAndFormatting(t *testing.T) {
	table := `Netid State  Recv-Q Send-Q Local Address:Port Peer Address:Port
tcp   LISTEN 0      511          0.0.0.0:8080      0.0.0.0:*
tcp   LISTEN 0      511             [::]:443          [::]:*`

	res := parseListeningPorts(table, []string{"80", "443", "8080"}, map[string]bool{})
	if res["80"] {
		t.Fatalf("port 80 must not match :8080")
	}
	if !res["443"] || !res["8080"] {
		t.Fatalf("expected 443 and 8080 listening, got %+v", res)
	}

	origMode := core.RichTerminal
	defer core.SetSymbolMode(origMode)

	core.SetSymbolMode(true)
	cell := core.StripANSI(FormatPortsCell(WebStats{
		Ports:     []string{"80", "443"},
		Listening: res,
	}))
	if cell != "80 × 443 ✓" {
		t.Fatalf("unexpected fancy ports cell: %q", cell)
	}

	if got := core.StripANSI(FormatConfigCell(true, true, "v1.26.2")); got != "Syntax OK · v1.26.2" {
		t.Fatalf("unexpected fancy config ok cell: %q", got)
	}
	if got := core.StripANSI(FormatConfigCell(true, false, "v1.26.2")); got != "Syntax ERROR" {
		t.Fatalf("unexpected config err cell: %q", got)
	}

	core.SetSymbolMode(false)
	asciiCell := core.StripANSI(FormatPortsCell(WebStats{
		Ports:     []string{"80", "443"},
		Listening: res,
	}))
	if asciiCell != "80 x 443 +" {
		t.Fatalf("unexpected ascii ports cell: %q", asciiCell)
	}
	if got := core.StripANSI(FormatConfigCell(true, true, "v1.26.2")); got != "Syntax OK | v1.26.2" {
		t.Fatalf("unexpected ascii config ok cell: %q", got)
	}
}

