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
	_, _ = fmt.Fprint(os.Stdout, "mariadb-dump-ok\n")
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
	if err := AppendFileNoFollow(linkPath, []byte("evil"), 0600); err == nil {
		t.Fatalf("expected AppendFileNoFollow to reject symlink")
	}
	if got, _ := os.ReadFile(victim); string(got) != "original" {
		t.Fatalf("symlink target modified: %q", string(got))
	}
}

func TestBuildMySQLClientCnf(t *testing.T) {
	tmpDir, cnfPath, err := BuildMySQLClientCnf("root", "p\"a\\s\ns")
	if err != nil {
		t.Fatalf("BuildMySQLClientCnf failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	data, err := os.ReadFile(cnfPath)
	if err != nil {
		t.Fatalf("failed reading cnf file: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "[client]") || !strings.Contains(s, `user="root"`) {
		t.Fatalf("unexpected client.cnf content: %q", s)
	}
	if strings.Contains(s, "p\"a\\s\ns") {
		t.Fatalf("raw unescaped newline/quote leaked into client.cnf: %q", s)
	}
	if runtime.GOOS == "linux" {
		if fi, err := os.Stat(tmpDir); err == nil && fi.Mode().Perm() != 0700 {
			t.Fatalf("temp dir mode = %o, want 0700", fi.Mode().Perm())
		}
		if fi, err := os.Stat(cnfPath); err == nil && fi.Mode().Perm() != 0600 {
			t.Fatalf("cnf file mode = %o, want 0600", fi.Mode().Perm())
		}
		// Must fail closed instead of falling back when TMPDIR is invalid.
		t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing-parent"))
		if _, _, err := BuildMySQLClientCnf("root", "pw"); err == nil {
			t.Fatalf("expected BuildMySQLClientCnf to fail when TMPDIR is missing")
		}
	}
}

func TestStreamCommandGzipNoFollow(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.sql.gz")
	if err := os.WriteFile(victim, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "evil.sql.gz")
	if err := os.Symlink(victim, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	newHelper := func() *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}

	if err := StreamCommandGzipNoFollow(linkPath, newHelper()); err == nil {
		t.Fatalf("expected StreamCommandGzipNoFollow to reject symlink destination")
	}
	if got, _ := os.ReadFile(victim); string(got) != "keep" {
		t.Fatalf("victim altered via symlink: %q", string(got))
	}

	dest := filepath.Join(dir, "backup.sql.gz")
	if err := StreamCommandGzipNoFollow(dest, newHelper()); err != nil {
		t.Fatalf("StreamCommandGzipNoFollow failed: %v", err)
	}
	f, err := os.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("invalid gzip output: %v", err)
	}
	defer gr.Close()
	out, err := io.ReadAll(gr)
	if err != nil || string(out) != "mariadb-dump-ok\n" {
		t.Fatalf("unexpected gunzipped content: %q (%v)", string(out), err)
	}
}
