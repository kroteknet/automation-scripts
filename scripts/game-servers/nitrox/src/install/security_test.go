package install

import (
	"errors"
	"strings"
	"testing"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("rand unavailable")
}

func TestGenerateSecureToken_NoTimeFallback(t *testing.T) {
	if tok, err := generateSecureTokenFromReader(errReader{}, 16); err == nil || tok != "" {
		t.Fatalf("expected error without time fallback when entropy fails, got tok=%q err=%v", tok, err)
	}
	if tok, err := generateSecureTokenFromReader(nil, 16); err == nil || tok != "" {
		t.Fatalf("expected error on nil reader, got tok=%q err=%v", tok, err)
	}
	if tok, err := generateSecureToken(0); err == nil || tok != "" {
		t.Fatalf("expected error on zero length, got tok=%q err=%v", tok, err)
	}

	t1, err := generateSecureToken(16)
	if err != nil || len(t1) != 16 {
		t.Fatalf("generateSecureToken(16) failed: len=%d err=%v", len(t1), err)
	}
	if strings.HasPrefix(t1, "AdminToken") {
		t.Fatalf("unexpected deterministic prefix on random token: %q", t1)
	}
}

func TestEscapeYamlString(t *testing.T) {
	got := escapeYamlString("hello \"world\"\nline2\\end")
	if strings.Contains(got, "\n") || strings.Contains(got, "\r") {
		t.Fatalf("escapeYamlString failed to strip newlines: %q", got)
	}
	if !strings.Contains(got, `\"world\"`) {
		t.Fatalf("escapeYamlString failed to escape quotes: %q", got)
	}
}
