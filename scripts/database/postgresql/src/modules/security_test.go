package modules

import (
	"errors"
	"testing"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy source failed")
}

func TestVerifyAdminPassword(t *testing.T) {
	if VerifyAdminPassword("", "") {
		t.Fatalf("VerifyAdminPassword must reject empty passwords")
	}
	if VerifyAdminPassword("", "anything") || VerifyAdminPassword("secret", "") {
		t.Fatalf("VerifyAdminPassword must reject partially empty passwords")
	}
	if VerifyAdminPassword("secret-a", "secret-b") {
		t.Fatalf("VerifyAdminPassword must reject mismatched passwords")
	}
	if !VerifyAdminPassword("StrongPass#2026", "StrongPass#2026") {
		t.Fatalf("VerifyAdminPassword must accept identical passwords")
	}
}

func TestGenerateRandomPass_FailsClosedOnEntropyError(t *testing.T) {
	if p, err := GenerateRandomPassFromReader(failingReader{}, 16); err == nil || p != "" {
		t.Fatalf("expected error on failing CSPRNG, got pass=%q err=%v", p, err)
	}
	p1, err := GenerateRandomPass(20)
	if err != nil || len(p1) != 20 {
		t.Fatalf("expected 20-char random password, got len=%d err=%v", len(p1), err)
	}
}
