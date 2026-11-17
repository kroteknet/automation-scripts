package system

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteAndAppendFileNoFollow_Symlinks(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.conf")
	if err := os.WriteFile(victim, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "wg0.conf")
	if err := os.Symlink(victim, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteFileNoFollow(linkPath, []byte("[Interface]\n"), 0600, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject symlink")
	}
	if err := AppendFileNoFollow(linkPath, []byte("[Peer]\n"), 0600); err == nil {
		t.Fatalf("expected AppendFileNoFollow to reject symlink")
	}
	if got, _ := os.ReadFile(victim); string(got) != "keep" {
		t.Fatalf("victim altered via symlink: %q", string(got))
	}

	target := filepath.Join(dir, "wg0-real.conf")
	if err := WriteFileNoFollow(target, []byte("[Interface]\n"), 0600, ""); err != nil {
		t.Fatalf("WriteFileNoFollow failed: %v", err)
	}
	if err := AppendFileNoFollow(target, []byte("[Peer]\n"), 0600); err != nil {
		t.Fatalf("AppendFileNoFollow failed: %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "[Interface]\n[Peer]\n" {
		t.Fatalf("unexpected file content: %q (%v)", string(got), err)
	}
}

func TestValidateSafeComposeDir(t *testing.T) {
	if runtime.GOOS != "windows" {
		if err := ValidateSafeComposeDir("/etc/wireguard-docker"); err != nil {
			t.Errorf("expected valid compose dir, got: %v", err)
		}
	}
	for _, bad := range []string{"", "relative/path", "/etc/foo;rm -rf /", "/etc/foo$(id)", "/etc", "/root"} {
		if err := ValidateSafeComposeDir(bad); err == nil {
			t.Errorf("ValidateSafeComposeDir(%q) = nil, want error", bad)
		}
	}
}
