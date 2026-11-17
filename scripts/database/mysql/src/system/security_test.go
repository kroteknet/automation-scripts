package system

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	_, _ = fmt.Fprint(os.Stdout, "mysql-dump-ok\n")
	os.Exit(0)
}

func TestWriteAndAppendFileNoFollow_Symlinks(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.cnf")
	if err := os.WriteFile(victim, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "link.cnf")
	if err := os.Symlink(victim, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteFileNoFollow(linkPath, []byte("evil"), 0600, ""); err == nil {
		t.Fatalf("expected WriteFileNoFollow to reject symlink")
	}
	if got, _ := os.ReadFile(victim); string(got) != "original" {
		t.Fatalf("symlink target modified: %q", string(got))
	}
}

func TestBuildMySQLClientCnf(t *testing.T) {
	tmpDir, cnfPath, err := BuildMySQLClientCnf("admin", "pass\"word\n")
	if err != nil {
		t.Fatalf("BuildMySQLClientCnf failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	data, err := os.ReadFile(cnfPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "[client]") || !strings.Contains(string(data), `user="admin"`) {
		t.Fatalf("unexpected client.cnf: %q", string(data))
	}
	if runtime.GOOS == "linux" {
		if fi, err := os.Stat(cnfPath); err == nil && fi.Mode().Perm() != 0600 {
			t.Fatalf("cnf file mode = %o, want 0600", fi.Mode().Perm())
		}
		t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing-parent"))
		if _, _, err := BuildMySQLClientCnf("admin", "pw"); err == nil {
			t.Fatalf("expected error when TMPDIR is invalid")
		}
	}
}

func TestStreamCommandGzipNoFollow(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "dump.sql.gz")
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	if err := StreamCommandGzipNoFollow(dest, cmd); err != nil {
		t.Fatalf("StreamCommandGzipNoFollow failed: %v", err)
	}
	f, err := os.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()
	out, err := io.ReadAll(gr)
	if err != nil || string(out) != "mysql-dump-ok\n" {
		t.Fatalf("unexpected gunzipped content: %q (%v)", string(out), err)
	}
}
