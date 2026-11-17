package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileNoFollowAndSecureWebRootPermissions(t *testing.T) {
	dir := t.TempDir()
	outsideVictim := filepath.Join(dir, "outside.txt")
	if err := os.WriteFile(outsideVictim, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	webRoot := filepath.Join(dir, "wp")
	if err := os.MkdirAll(webRoot, 0755); err != nil {
		t.Fatal(err)
	}
	linkInside := filepath.Join(webRoot, "evil-config.php")
	if err := os.Symlink(outsideVictim, linkInside); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteFileNoFollow(linkInside, []byte("pwned"), 0600, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject symlink")
	}
	SecureWebRootPermissions(webRoot, "")
	if got, _ := os.ReadFile(outsideVictim); string(got) != "keep" {
		t.Fatalf("outside victim modified via symlink: %q", string(got))
	}
}
