package system

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteAndAppendFileNoFollow_Symlinks(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.yml")
	if err := os.WriteFile(victim, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "link.yml")
	if err := os.Symlink(victim, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteFileNoFollow(linkPath, []byte("evil"), 0600, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject symlink")
	}
	if got, _ := os.ReadFile(victim); string(got) != "original" {
		t.Fatalf("victim modified: %q", string(got))
	}
}

func TestBuildMongoDumpConfig(t *testing.T) {
	tmpDir, cfgPath, err := BuildMongoDumpConfig("s\"ecret\nline")
	if err != nil {
		t.Fatalf("BuildMongoDumpConfig failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "password: ") || strings.Count(string(data), "\n") != 1 {
		t.Fatalf("unexpected mongodump.yml format: %q", string(data))
	}
	if runtime.GOOS == "linux" {
		if fi, err := os.Stat(tmpDir); err == nil && fi.Mode().Perm() != 0700 {
			t.Fatalf("tmpDir mode = %o, want 0700", fi.Mode().Perm())
		}
		if fi, err := os.Stat(cfgPath); err == nil && fi.Mode().Perm() != 0600 {
			t.Fatalf("cfgPath mode = %o, want 0600", fi.Mode().Perm())
		}
		t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing-parent"))
		if _, _, err := BuildMongoDumpConfig("pw"); err == nil {
			t.Fatalf("expected BuildMongoDumpConfig to fail closed on missing TMPDIR")
		}
	}
}
