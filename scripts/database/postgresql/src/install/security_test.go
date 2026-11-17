package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendPGHBALanRules_ValidFormattingAndSymlinkSafety(t *testing.T) {
	root := t.TempDir()
	confDir := filepath.Join(root, "16", "main")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		t.Fatal(err)
	}
	realHBA := filepath.Join(confDir, "pg_hba.conf")
	if err := os.WriteFile(realHBA, []byte("local all postgres peer\n"), 0640); err != nil {
		t.Fatal(err)
	}

	// Symlink pg_hba.conf pointing outside must be skipped without modifying target.
	victim := filepath.Join(root, "victim.txt")
	if err := os.WriteFile(victim, []byte("unchanged"), 0644); err != nil {
		t.Fatal(err)
	}
	symlinkDir := filepath.Join(root, "17", "main")
	if err := os.MkdirAll(symlinkDir, 0755); err != nil {
		t.Fatal(err)
	}
	symlinkHBA := filepath.Join(symlinkDir, "pg_hba.conf")
	symlinkReady := os.Symlink(victim, symlinkHBA) == nil

	if err := AppendPGHBALanRules(root, filepath.Join(root, "nonexistent")); err != nil {
		t.Fatalf("AppendPGHBALanRules unexpected error: %v", err)
	}

	got, err := os.ReadFile(realHBA)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{
		"host all all 10.0.0.0/8 scram-sha-256",
		"host all all 172.16.0.0/12 scram-sha-256",
		"host all all 192.168.0.0/16 scram-sha-256",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("pg_hba.conf missing %q; content:\n%s", want, text)
		}
	}
	if strings.Contains(text, `\n`) || strings.Contains(text, `\t`) {
		t.Fatalf("pg_hba.conf contains literal backslash escape sequences:\n%s", text)
	}

	// Calling again must be idempotent.
	if err := AppendPGHBALanRules(root); err != nil {
		t.Fatalf("second AppendPGHBALanRules error: %v", err)
	}
	gotAgain, _ := os.ReadFile(realHBA)
	if string(gotAgain) != text {
		t.Fatalf("expected idempotent append, got diff:\n%s", string(gotAgain))
	}

	if symlinkReady {
		if v, err := os.ReadFile(victim); err != nil || string(v) != "unchanged" {
			t.Fatalf("victim altered via symlink pg_hba.conf: %q (%v)", string(v), err)
		}
	}
}
