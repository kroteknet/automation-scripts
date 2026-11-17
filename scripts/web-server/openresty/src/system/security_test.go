package system

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAndAppendFileNoFollow_Symlinks(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(victim, []byte("keep-intact"), 0644); err != nil {
		t.Fatalf("setup write failed: %v", err)
	}

	linkPath := filepath.Join(dir, "evil-link.conf")
	if err := os.Symlink(victim, linkPath); err != nil {
		t.Skipf("symlink creation unavailable in this environment: %v", err)
	}

	if err := WriteFileNoFollow(linkPath, []byte("overwritten"), 0644, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject existing symlink %s", linkPath)
	}
	if err := AppendFileNoFollow(linkPath, []byte("appended"), 0644); err == nil {
		t.Fatalf("expected AppendFileNoFollow to reject existing symlink %s", linkPath)
	}
	gotVictim, err := os.ReadFile(victim)
	if err != nil || string(gotVictim) != "keep-intact" {
		t.Fatalf("symlink target was modified! got=%q err=%v", string(gotVictim), err)
	}

	danglingLink := filepath.Join(dir, "dangling.conf")
	danglingTarget := filepath.Join(dir, "nonexistent-target.txt")
	if err := os.Symlink(danglingTarget, danglingLink); err != nil {
		t.Fatalf("dangling symlink setup failed: %v", err)
	}
	if err := WriteFileNoFollow(danglingLink, []byte("pwned"), 0644, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject dangling symlink")
	}
	if _, err := os.Stat(danglingTarget); !os.IsNotExist(err) {
		t.Fatalf("dangling symlink target should not exist, err=%v", err)
	}

	regularPath := filepath.Join(dir, "ok.conf")
	if err := WriteFileNoFollow(regularPath, []byte("line1\n"), 0600, ""); err != nil {
		t.Fatalf("WriteFileNoFollow on regular file failed: %v", err)
	}
	if err := AppendFileNoFollow(regularPath, []byte("line2\n"), 0600); err != nil {
		t.Fatalf("AppendFileNoFollow on regular file failed: %v", err)
	}
	got, err := os.ReadFile(regularPath)
	if err != nil || string(got) != "line1\nline2\n" {
		t.Fatalf("unexpected regular file content: %q (%v)", string(got), err)
	}

	if err := WriteFileNoFollow(dir, []byte("bad"), 0644, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject directory target")
	}
	if err := WriteFileNoFollow("", []byte("bad"), 0644, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject empty path")
	}
}

func TestValidateProxyTarget(t *testing.T) {
	valid := []string{
		"http://127.0.0.1:3000",
		"http://localhost:8000",
		"https://backend.internal:8443/api",
		"http://app-service:3000",
		"http://[::1]:8080",
		"http://192.168.10.20:5000",
	}
	for _, u := range valid {
		if !ValidateProxyTarget(u) {
			t.Errorf("ValidateProxyTarget(%q) = false, want true", u)
		}
	}

	invalid := []string{
		"",
		"   ",
		"ftp://127.0.0.1:21",
		"javascript:alert(1)",
		"http://127.0.0.1:3000; malicious_directive on;",
		"http://127.0.0.1:3000\nproxy_pass http://evil",
		"http://127.0.0.1:3000\r\nHost: evil",
		"http://127.0.0.1:3000\" }",
		"http://user:pass@127.0.0.1:3000",
		"http://127.0.0.1:3000#fragment",
		"http://127.0.0.1:99999",
		"http://169.254.169.254/latest/meta-data/",
		"http://169.254.0.5:8080",
		"http://[::ffff:169.254.169.254]/latest/meta-data/",
		"http://metadata.google.internal/computeMetadata/v1/",
		"http://metadata.goog/",
		"http://100.100.100.200/latest/meta-data/",
		"http://[fe80::1]:8080",
		"http://[fd00:ec2::254]/latest/api/token",
		"http://2852039166/latest/meta-data/",
		"http://0xa9fea9fe/latest/meta-data/",
		"http://0.0.0.0:3000",
	}
	for _, u := range invalid {
		if ValidateProxyTarget(u) {
			t.Errorf("ValidateProxyTarget(%q) = true, want false", u)
		}
	}
}

func TestValidateTarGzArchive(t *testing.T) {
	dir := t.TempDir()

	buildTar := func(name string, entries []struct {
		hdr *tar.Header
		val []byte
	}) string {
		p := filepath.Join(dir, name)
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		gw := gzip.NewWriter(f)
		tw := tar.NewWriter(gw)
		for _, e := range entries {
			if err := tw.WriteHeader(e.hdr); err != nil {
				t.Fatal(err)
			}
			if len(e.val) > 0 {
				if _, err := tw.Write(e.val); err != nil {
					t.Fatal(err)
				}
			}
		}
		_ = tw.Close()
		_ = gw.Close()
		_ = f.Close()
		return p
	}

	okTar := buildTar("ok.tar.gz", []struct {
		hdr *tar.Header
		val []byte
	}{
		{
			hdr: &tar.Header{Name: "etc/openresty/nginx.conf", Mode: 0644, Size: 4, Typeflag: tar.TypeReg},
			val: []byte("ok\n\n"),
		},
	})
	if err := ValidateTarGzArchive(okTar, 1024*1024); err != nil {
		t.Fatalf("expected valid tar.gz to pass: %v", err)
	}

	evilTraversal := buildTar("traversal.tar.gz", []struct {
		hdr *tar.Header
		val []byte
	}{
		{
			hdr: &tar.Header{Name: "../etc/cron.d/pwn", Mode: 0644, Size: 2, Typeflag: tar.TypeReg},
			val: []byte("x\n"),
		},
	})
	if err := ValidateTarGzArchive(evilTraversal, 1024*1024); err == nil {
		t.Fatalf("expected tar traversal to be rejected")
	}

	evilSymlink := buildTar("symlink.tar.gz", []struct {
		hdr *tar.Header
		val []byte
	}{
		{
			hdr: &tar.Header{Name: "tmp/link", Mode: 0777, Typeflag: tar.TypeSymlink, Linkname: "../../etc/shadow"},
		},
	})
	if err := ValidateTarGzArchive(evilSymlink, 1024*1024); err == nil {
		t.Fatalf("expected tar symlink traversal to be rejected")
	}
}

func TestExtractZipSafe(t *testing.T) {
	dir := t.TempDir()

	writeZip := func(name string, entries []struct {
		name string
		mode os.FileMode
		body []byte
	}) string {
		p := filepath.Join(dir, name)
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for _, e := range entries {
			h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
			if e.mode != 0 {
				h.SetMode(e.mode)
			}
			w, err := zw.CreateHeader(h)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write(e.body); err != nil {
				t.Fatal(err)
			}
		}
		_ = zw.Close()
		if err := os.WriteFile(p, buf.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	slipZip := writeZip("slip.zip", []struct {
		name string
		mode os.FileMode
		body []byte
	}{{name: "../../evil.txt", body: []byte("bad")}})
	if err := ExtractZipSafe(slipZip, filepath.Join(dir, "out1"), 10, 1024, 2048); err == nil {
		t.Fatalf("expected zip-slip archive to be rejected")
	}

	symZip := writeZip("sym.zip", []struct {
		name string
		mode os.FileMode
		body []byte
	}{{name: "link", mode: os.ModeSymlink | 0777, body: []byte("/etc/passwd")}})
	if err := ExtractZipSafe(symZip, filepath.Join(dir, "out2"), 10, 1024, 2048); err == nil {
		t.Fatalf("expected symlink entry in zip to be rejected")
	}

	bombZip := writeZip("bomb.zip", []struct {
		name string
		mode os.FileMode
		body []byte
	}{{name: "big.bin", body: bytes.Repeat([]byte("A"), 2048)}})
	if err := ExtractZipSafe(bombZip, filepath.Join(dir, "out3"), 10, 512, 1024); err == nil {
		t.Fatalf("expected oversized zip entry to be rejected")
	}

	goodZip := writeZip("good.zip", []struct {
		name string
		mode os.FileMode
		body []byte
	}{{name: "sub/hello.txt", body: []byte("world")}})
	outDir := filepath.Join(dir, "out_ok")
	if err := ExtractZipSafe(goodZip, outDir, 10, 1024, 2048); err != nil {
		t.Fatalf("expected valid zip to extract cleanly: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outDir, "sub", "hello.txt"))
	if err != nil || string(data) != "world" {
		t.Fatalf("unexpected extracted content: %q err=%v", string(data), err)
	}
}

func TestHttpFetchBytesLimits(t *testing.T) {
	if _, _, err := HttpFetchBytes("http://example.com", 1024); err == nil {
		t.Fatalf("expected plain HTTP URL to be rejected")
	}

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/downgrade" {
			http.Redirect(w, r, "http://example.com/", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(strings.Repeat("X", 2048)))
	}))
	defer ts.Close()

	client := ts.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return http.ErrUseLastResponse
		}
		return nil
	}

	if _, _, err := httpFetchBytesWithClient(client, ts.URL+"/big", 100); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("expected body size limit error, got: %v", err)
	}
	body, code, err := httpFetchBytesWithClient(client, ts.URL+"/big", 4096)
	if err != nil || code != http.StatusOK || len(body) != 2048 {
		t.Fatalf("expected valid bounded fetch, code=%d len=%d err=%v", code, len(body), err)
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
	logPath := filepath.Join(t.TempDir(), "openresty.log")
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
