package system

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestFirewallBackupAndWriteSafety(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.rules")
	if err := os.WriteFile(victim, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "link.rules")
	if err := os.Symlink(victim, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteFileNoFollow(linkPath, []byte("evil"), 0600, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject symlink")
	}

	tarPath := filepath.Join(dir, "evil.tar.gz")
	f, _ := os.Create(tarPath)
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{Name: "etc/ufw/link", Mode: 0777, Typeflag: tar.TypeSymlink, Linkname: "../../../etc/shadow"})
	_ = tw.Close()
	_ = gw.Close()
	_ = f.Close()
	if err := ValidateTarGzArchive(tarPath, 1024*1024); err == nil {
		t.Fatalf("expected symlink entry in firewall tar backup to be rejected")
	}
}
