package core

import "testing"

func TestSanitizeTerminalText(t *testing.T) {
	if got := SanitizeTerminalText("\x1b[31mERR\x1b[0m\x1b]0;TITLE\x07ok\t\u009b!\x7f"); got != "ERRok  !" {
		t.Fatalf("SanitizeTerminalText returned %q, want %q", got, "ERRok  !")
	}
}
