package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndValidateComposeDir(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(victim, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "link.txt")
	if err := os.Symlink(victim, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteFileNoFollow(linkPath, []byte("bad"), 0644, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject symlink")
	}
	if err := ValidateSafeComposeDir("/opt/hestia;rm -rf /"); err == nil {
		t.Fatalf("expected ValidateSafeComposeDir to reject shell metacharacters")
	}
}
