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
	"runtime"
	"strings"
	"testing"
)

func TestWriteAndAppendFileNoFollow_Symlinks(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(victim, []byte("keep-intact"), 0644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "evil.properties")
	if err := os.Symlink(victim, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteFileNoFollow(linkPath, []byte("overwritten"), 0600, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject symlink")
	}
	if err := AppendFileNoFollow(linkPath, []byte("appended"), 0600); err == nil {
		t.Fatalf("expected AppendFileNoFollow to reject symlink")
	}
	if got, _ := os.ReadFile(victim); string(got) != "keep-intact" {
		t.Fatalf("victim altered via symlink: %q", string(got))
	}
}

func TestExtractZipSafe_ZipSlipSymlinkAndBomb(t *testing.T) {
	dir := t.TempDir()
	buildZip := func(name, entryName string, mode os.FileMode, body []byte) string {
		p := filepath.Join(dir, name)
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		h := &zip.FileHeader{Name: entryName, Method: zip.Deflate}
		if mode != 0 {
			h.SetMode(mode)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
		_ = zw.Close()
		if err := os.WriteFile(p, buf.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	slip := buildZip("slip.zip", "../../etc/cron.d/pwn", 0644, []byte("x"))
	if err := ExtractZipSafe(slip, filepath.Join(dir, "out1"), 10, 1024, 2048); err == nil {
		t.Fatalf("expected zip-slip traversal to be rejected")
	}
	sym := buildZip("sym.zip", "evil-link", os.ModeSymlink|0777, []byte("/etc/shadow"))
	if err := ExtractZipSafe(sym, filepath.Join(dir, "out2"), 10, 1024, 2048); err == nil {
		t.Fatalf("expected zip symlink entry to be rejected")
	}
	bomb := buildZip("bomb.zip", "huge.jar", 0644, bytes.Repeat([]byte("A"), 2048))
	if err := ExtractZipSafe(bomb, filepath.Join(dir, "out3"), 10, 512, 1024); err == nil {
		t.Fatalf("expected zip bomb entry exceeding maxFileBytes to be rejected")
	}
	good := buildZip("good.zip", "plugins/test.txt", 0644, []byte("ok"))
	outDir := filepath.Join(dir, "out_ok")
	if err := ExtractZipSafe(good, outDir, 10, 1024, 2048); err != nil {
		t.Fatalf("valid zip failed: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(outDir, "plugins", "test.txt")); err != nil || string(got) != "ok" {
		t.Fatalf("unexpected extracted content: %q (%v)", string(got), err)
	}
}

func TestValidateTarGzArchive(t *testing.T) {
	dir := t.TempDir()
	buildTar := func(name string, hdr *tar.Header, body []byte) string {
		p := filepath.Join(dir, name)
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		gw := gzip.NewWriter(f)
		tw := tar.NewWriter(gw)
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if len(body) > 0 {
			if _, err := tw.Write(body); err != nil {
				t.Fatal(err)
			}
		}
		_ = tw.Close()
		_ = gw.Close()
		_ = f.Close()
		return p
	}

	evil := buildTar("evil.tar.gz", &tar.Header{Name: "../outside", Mode: 0644, Size: 2, Typeflag: tar.TypeReg}, []byte("hi"))
	if err := ValidateTarGzArchive(evil, 1024*1024); err == nil {
		t.Fatalf("expected path traversal tar to be rejected")
	}
	sym := buildTar("sym.tar.gz", &tar.Header{Name: "link", Mode: 0777, Typeflag: tar.TypeSymlink, Linkname: "../../etc/passwd"}, nil)
	if err := ValidateTarGzArchive(sym, 1024*1024); err == nil {
		t.Fatalf("expected symlink tar to be rejected")
	}
	ok := buildTar("ok.tar.gz", &tar.Header{Name: "world/level.dat", Mode: 0644, Size: 2, Typeflag: tar.TypeReg}, []byte("ok"))
	if err := ValidateTarGzArchive(ok, 1024*1024); err != nil {
		t.Fatalf("valid tar rejected: %v", err)
	}
}

func TestHttpFetchBytesLimitsAndComposeDir(t *testing.T) {
	if _, _, err := HttpFetchBytes("http://example.com/file", 1024); err == nil {
		t.Fatalf("expected non-HTTPS URL to be rejected")
	}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("M", 2048)))
	}))
	defer ts.Close()
	if _, _, err := httpFetchBytesWithClient(ts.Client(), ts.URL, 256); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("expected size limit failure, got: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := ValidateSafeComposeDir("/etc/minecraft-docker"); err != nil {
			t.Errorf("valid compose dir failed: %v", err)
		}
	}
	if err := ValidateSafeComposeDir("/etc/minecraft;id"); err == nil {
		t.Errorf("unsafe compose dir accepted")
	}
}
