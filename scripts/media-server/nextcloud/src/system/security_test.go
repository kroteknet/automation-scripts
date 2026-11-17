package system

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileNoFollowAndExtractZipSafe(t *testing.T) {
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

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("../escape.php")
	_, _ = w.Write([]byte("<?php ?>"))
	_ = zw.Close()
	zPath := filepath.Join(dir, "slip.zip")
	_ = os.WriteFile(zPath, buf.Bytes(), 0644)
	if err := ExtractZipSafe(zPath, filepath.Join(dir, "nc"), 100, 1024, 4096); err == nil {
		t.Fatalf("expected zip-slip to be rejected")
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
