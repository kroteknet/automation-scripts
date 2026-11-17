package system

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAndAppendFileNoFollow_Symlinks(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.conf")
	if err := os.WriteFile(victim, []byte("keep-intact"), 0644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "evil.conf")
	if err := os.Symlink(victim, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteFileNoFollow(linkPath, []byte("pwned"), 0644, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject symlink")
	}
	if err := AppendFileNoFollow(linkPath, []byte("pwned"), 0644); err == nil {
		t.Fatalf("expected AppendFileNoFollow to reject symlink")
	}
	if got, _ := os.ReadFile(victim); string(got) != "keep-intact" {
		t.Fatalf("victim modified via symlink: %q", string(got))
	}
}

func TestValidateProxyTarget(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:3000", "https://api.internal:8443/v1", "http://[::1]:8080"} {
		if !ValidateProxyTarget(u) {
			t.Errorf("ValidateProxyTarget(%q) = false, want true", u)
		}
	}
	for _, u := range []string{
		"", "ftp://127.0.0.1:21", "http://127.0.0.1:3000; malicious",
		"http://127.0.0.1:3000\r\nHost: evil", "http://127.0.0.1:3000\" }",
		"http://169.254.169.254/latest/meta-data/", "http://[::ffff:169.254.169.254]/",
		"http://metadata.google.internal/", "http://user:pass@127.0.0.1:3000",
	} {
		if ValidateProxyTarget(u) {
			t.Errorf("ValidateProxyTarget(%q) = true, want false", u)
		}
	}
}

func TestHttpFetchBytesLimits(t *testing.T) {
	if _, _, err := HttpFetchBytes("http://example.com", 1024); err == nil {
		t.Fatalf("expected HTTP scheme to be rejected")
	}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("Z", 1024)))
	}))
	defer ts.Close()
	if _, _, err := httpFetchBytesWithClient(ts.Client(), ts.URL, 128); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("expected body limit error, got: %v", err)
	}
}

func TestIsLocalOrPrivateDomainAndEnvFallback(t *testing.T) {
	localCases := []string{
		"demo.krotek.local",
		"app.local",
		"localhost",
		"mybox.localhost",
		"nas.lan",
		"router.home",
		"svc.internal",
		"127.0.0.1",
		"intranet",
		"https://demo.krotek.local:8443/path",
	}
	for _, d := range localCases {
		if !IsLocalOrPrivateDomain(d) {
			t.Errorf("IsLocalOrPrivateDomain(%q) = false, want true", d)
		}
	}

	publicCases := []string{
		"example.com",
		"api.service.org",
		"sub.domain.io",
		"https://next-app.com/",
	}
	for _, d := range publicCases {
		if IsLocalOrPrivateDomain(d) {
			t.Errorf("IsLocalOrPrivateDomain(%q) = true, want false", d)
		}
	}

	t.Setenv("KROTEK_SSL", "certbot")
	t.Setenv("KROTEK_SSL_EMAIL", "admin@demo.krotek.local")
	if strat := ParseSSLStrategyFromEnv("demo.krotek.local"); strat.Mode != "none" {
		t.Fatalf("expected certbot env fallback to 'none' for .local domain, got %q", strat.Mode)
	}
	if strat := ParseSSLStrategyFromEnv("example.com"); strat.Mode != "certbot" {
		t.Fatalf("expected certbot mode for public domain, got %q", strat.Mode)
	}
}

func TestExtractRecentLogErrors(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "install.log")
	content := strings.Join([]string{
		"[2026-11-15T12:22:31+01:00] [WARN] Ensuring domain points to this server's IP address",
		"Detail: DNS problem: NXDOMAIN looking up A for example.com",
		"Some unrelated output",
		"[2026-11-15T12:22:32+01:00] [ERROR] Failed! Check log file: cat /var/log/nginx/install.log",
	}, "\n")
	if err := os.WriteFile(logFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	got := ExtractRecentLogErrors(logFile, 4)
	if len(got) != 1 || !strings.Contains(got[0], "NXDOMAIN") {
		t.Fatalf("expected NXDOMAIN diagnostic line and no generic 'Check log file' line, got: %#v", got)
	}
}
