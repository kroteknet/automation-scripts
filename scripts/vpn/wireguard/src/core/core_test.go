package core

import "testing"

func TestSanitizeTerminalText(t *testing.T) {
	if got := SanitizeTerminalText("\x1b[32mPEER\x1b[0m \x1b]2;EVIL\x1b\\\u0085ok\x00"); got != "PEER ok" {
		t.Fatalf("SanitizeTerminalText returned %q, want %q", got, "PEER ok")
	}
}
