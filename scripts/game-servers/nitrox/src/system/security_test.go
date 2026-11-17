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
	victim := filepath.Join(dir, "victim.cfg")
	if err := os.WriteFile(victim, []byte("keep-intact"), 0600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "server.cfg")
	if err := os.Symlink(victim, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteFileNoFollow(linkPath, []byte("pwned"), 0600, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject symlink")
	}
	if got, _ := os.ReadFile(victim); string(got) != "keep-intact" {
		t.Fatalf("victim altered via symlink: %q", string(got))
	}
}

func TestExtractZipAndValidateTarGzSafe(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../../escape.dll")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("evil"))
	_ = zw.Close()
	slipZip := filepath.Join(dir, "slip.zip")
	if err := os.WriteFile(slipZip, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ExtractZipSafe(slipZip, filepath.Join(dir, "out"), 100, 1024, 4096); err == nil {
		t.Fatalf("expected zip-slip archive to be rejected")
	}

	tarPath := filepath.Join(dir, "sym.tar.gz")
	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{Name: "badlink", Mode: 0777, Typeflag: tar.TypeSymlink, Linkname: "../../etc/passwd"})
	_ = tw.Close()
	_ = gw.Close()
	_ = f.Close()
	if err := ValidateTarGzArchive(tarPath, 1024*1024); err == nil {
		t.Fatalf("expected symlink tar to be rejected")
	}
}

func TestHttpFetchBytesLimits(t *testing.T) {
	if _, _, err := HttpFetchBytes("http://example.com/nitrox.zip", 1024); err == nil {
		t.Fatalf("expected HTTP URL to be rejected")
	}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("N", 1024)))
	}))
	defer ts.Close()
	if _, _, err := httpFetchBytesWithClient(ts.Client(), ts.URL, 64); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("expected size limit error, got: %v", err)
	}
}
