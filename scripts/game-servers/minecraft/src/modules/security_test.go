package modules

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("rand unavailable")
}

func TestGenerateSecureTokenFromReader_FailsClosed(t *testing.T) {
	if tok, err := generateSecureTokenFromReader(errReader{}, 24); err == nil || tok != "" {
		t.Fatalf("expected error and empty token when rand fails, got tok=%q err=%v", tok, err)
	}
	if tok, err := generateSecureTokenFromReader(nil, 24); err == nil || tok != "" {
		t.Fatalf("expected error on nil reader, got tok=%q err=%v", tok, err)
	}
	if tok, err := generateSecureToken(0); err == nil || tok != "" {
		t.Fatalf("expected error on zero length, got tok=%q err=%v", tok, err)
	}

	t1, err := generateSecureToken(24)
	if err != nil {
		t.Fatalf("generateSecureToken(24) error: %v", err)
	}
	t2, err := generateSecureToken(24)
	if err != nil {
		t.Fatalf("generateSecureToken(24) error: %v", err)
	}
	if len(t1) != 24 || len(t2) != 24 || t1 == t2 {
		t.Fatalf("expected distinct 24-char tokens, got len=%d/%d eq=%v", len(t1), len(t2), t1 == t2)
	}
}

func TestDownloadFileWithSpinner_RejectsInsecureAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "plugin.jar")
	if err := downloadFileWithSpinner("http://example.com/plugin.jar", dest, "downloading"); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("expected non-HTTPS download rejection, got: %v", err)
	}

	victim := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(victim, []byte("untouched"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, dest); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := downloadFileWithSpinner("https://127.0.0.1:1/plugin.jar", dest, "downloading"); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink rejection before download, got: %v", err)
	}
	if got, err := os.ReadFile(victim); err != nil || string(got) != "untouched" {
		t.Fatalf("victim file altered: %q (%v)", string(got), err)
	}
}
