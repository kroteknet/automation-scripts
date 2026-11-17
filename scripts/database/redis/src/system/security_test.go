package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndAppendFileNoFollow_Symlinks(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.conf")
	if err := os.WriteFile(victim, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "link.conf")
	if err := os.Symlink(victim, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteFileNoFollow(linkPath, []byte("evil"), 0600, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject symlink")
	}
	if err := AppendFileNoFollow(linkPath, []byte("evil"), 0600); err == nil {
		t.Fatalf("expected AppendFileNoFollow to reject symlink")
	}
	if got, _ := os.ReadFile(victim); string(got) != "keep" {
		t.Fatalf("victim altered: %q", string(got))
	}
}
