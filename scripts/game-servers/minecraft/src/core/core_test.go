package core

import "testing"

func TestSanitizeTerminalText(t *testing.T) {
	if got := SanitizeTerminalText("\x1b[31mFAIL\x1b[0m \x1b]0;HIJACK\x07\u009bnote\t\x7f"); got != "FAIL note  " {
		t.Fatalf("SanitizeTerminalText returned %q, want %q", got, "FAIL note  ")
	}
}
