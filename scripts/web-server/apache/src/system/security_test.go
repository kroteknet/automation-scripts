package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteNoFollowAndValidateProxyTarget(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.conf")
	if err := os.WriteFile(victim, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "evil.conf")
	if err := os.Symlink(victim, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteFileNoFollow(linkPath, []byte("bad"), 0644, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject symlink")
	}
	if got, _ := os.ReadFile(victim); string(got) != "keep" {
		t.Fatalf("victim modified: %q", string(got))
	}
	if !ValidateProxyTarget("http://127.0.0.1:8080") || ValidateProxyTarget("http://169.254.169.254/latest/meta-data/") {
		t.Fatalf("ValidateProxyTarget failed validation checks")
	}
}

func TestValidateDomainName(t *testing.T) {
	valid := []string{"example.com", "demo.krotek.local", "sub.domain-1.io"}
	for _, d := range valid {
		if !ValidateDomainName(d) {
			t.Errorf("ValidateDomainName(%q) = false, want true", d)
		}
	}
	invalid := []string{"example_com", "exampleXcom", "", "-bad.com", "bad-.com"}
	for _, d := range invalid {
		if ValidateDomainName(d) {
			t.Errorf("ValidateDomainName(%q) = true, want false", d)
		}
	}
}

func TestIsLocalOrPrivateDomainAndEnvFallback(t *testing.T) {
	localCases := []string{
		"demo.krotek.local", "app.local", "localhost", "mybox.localhost",
		"nas.lan", "router.home", "svc.internal", "127.0.0.1", "intranet",
		"https://demo.krotek.local:8443/path",
	}
	for _, d := range localCases {
		if !IsLocalOrPrivateDomain(d) {
			t.Errorf("IsLocalOrPrivateDomain(%q) = false, want true", d)
		}
	}
	publicCases := []string{"example.com", "api.service.org", "sub.domain.io", "https://next-app.com/"}
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
	logPath := filepath.Join(t.TempDir(), "apache.log")
	content := strings.Join([]string{
		"[2026-11-15T12:00:00Z] [INFO] Starting task",
		"[2026-11-15T12:00:01Z] [ERROR] Certbot failed: DNS problem: NXDOMAIN",
		"[2026-11-15T12:00:02Z] [INFO] Cleanup complete",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	errs := ExtractRecentLogErrors(logPath, 5)
	if len(errs) == 0 || !strings.Contains(strings.Join(errs, "\n"), "NXDOMAIN") {
		t.Fatalf("expected NXDOMAIN in extracted errors, got %v", errs)
	}
}
