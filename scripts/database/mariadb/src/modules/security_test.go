package modules

import (
	"errors"
	"strings"
	"testing"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy source failed")
}

func TestVerifyAdminPassword(t *testing.T) {
	cases := []struct {
		name       string
		configured string
		candidate  string
		want       bool
	}{
		{name: "both empty must fail", configured: "", candidate: "", want: false},
		{name: "empty configured must fail", configured: "", candidate: "secret", want: false},
		{name: "empty candidate must fail", configured: "secret", candidate: "", want: false},
		{name: "mismatch must fail", configured: "secret1", candidate: "secret2", want: false},
		{name: "exact match succeeds", configured: "S3cure!Pass#99", candidate: "S3cure!Pass#99", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := VerifyAdminPassword(tc.configured, tc.candidate); got != tc.want {
				t.Fatalf("VerifyAdminPassword(%q, %q) = %v, want %v", tc.configured, tc.candidate, got, tc.want)
			}
		})
	}
}

func TestGenerateRandomPass_FailsClosedOnEntropyError(t *testing.T) {
	if p, err := GenerateRandomPassFromReader(failingReader{}, 16); err == nil || p != "" {
		t.Fatalf("expected error and empty password when reader fails, got pass=%q err=%v", p, err)
	}
	if p, err := GenerateRandomPassFromReader(nil, 16); err == nil || p != "" {
		t.Fatalf("expected error on nil reader, got pass=%q err=%v", p, err)
	}
	if p, err := GenerateRandomPass(0); err == nil || p != "" {
		t.Fatalf("expected error on zero length, got pass=%q err=%v", p, err)
	}

	p1, err := GenerateRandomPass(24)
	if err != nil {
		t.Fatalf("GenerateRandomPass(24) unexpected error: %v", err)
	}
	p2, err := GenerateRandomPass(24)
	if err != nil {
		t.Fatalf("GenerateRandomPass(24) second call error: %v", err)
	}
	if len(p1) != 24 || len(p2) != 24 || p1 == p2 {
		t.Fatalf("expected distinct 24-char passwords, got len=%d/%d equal=%v", len(p1), len(p2), p1 == p2)
	}
	if strings.ContainsAny(p1, " \t\r\n\"'") {
		t.Fatalf("generated password contains unexpected unsafe whitespace/quote chars: %q", p1)
	}
}
