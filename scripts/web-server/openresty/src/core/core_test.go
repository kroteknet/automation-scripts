package core

import (
	"strings"
	"testing"
)

func TestSanitizeTerminalText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantSub  string
		forbid   []string
		wantExact string
	}{
		{
			name:      "strips ANSI color codes",
			input:     "\x1b[31mERROR\x1b[0m failed",
			wantExact: "ERROR failed",
		},
		{
			name:      "strips OSC title escape",
			input:     "prefix \x1b]0;PWNED\x07suffix",
			wantExact: "prefix suffix",
		},
		{
			name:      "strips OSC with ST terminator",
			input:     "a\x1b]2;evil\x1b\\b",
			wantExact: "ab",
		},
		{
			name:      "strips C0 and DEL controls but turns tab to spaces",
			input:     "a\tb\x00\x07\x08\x1b[31m\x7fc\r\n",
			wantExact: "a  bc",
		},
		{
			name:      "strips C1 control range U+0080..U+009F",
			input:     "hello\u009bworld\u0085!",
			wantExact: "helloworld!",
		},
		{
			name:      "preserves UTF-8 text",
			input:     "OK żóły plik config.yml",
			wantExact: "OK żóły plik config.yml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeTerminalText(tc.input)
			if tc.wantExact != "" && got != tc.wantExact {
				t.Fatalf("SanitizeTerminalText(%q) = %q, want %q", tc.input, got, tc.wantExact)
			}
			for _, f := range tc.forbid {
				if strings.Contains(got, f) {
					t.Fatalf("SanitizeTerminalText output unexpectedly contains %q: %q", f, got)
				}
			}
		})
	}
}
