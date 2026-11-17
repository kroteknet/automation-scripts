package core

import "testing"

func TestDetectRichTerminalFromEnv(t *testing.T) {
	tests := []struct {
		name  string
		env   map[string]string
		isWSL bool
		want  bool
	}{
		{
			name:  "Windows Terminal WSL -> fancy",
			env:   map[string]string{"TERM": "xterm-256color", "LANG": "C.UTF-8", "WT_SESSION": "abc-123"},
			isWSL: true,
			want:  true,
		},
		{
			name:  "Legacy cmd.exe/conhost WSL without WT_SESSION or COLORTERM -> ascii",
			env:   map[string]string{"TERM": "xterm-256color", "LANG": "C.UTF-8", "WSL_DISTRO_NAME": "Ubuntu"},
			isWSL: true,
			want:  false,
		},
		{
			name:  "VS Code integrated terminal in WSL -> fancy",
			env:   map[string]string{"TERM": "xterm-256color", "LANG": "en_US.UTF-8", "TERM_PROGRAM": "vscode"},
			isWSL: true,
			want:  true,
		},
		{
			name:  "Standard SSH xterm-256color UTF-8 -> fancy",
			env:   map[string]string{"TERM": "xterm-256color", "LANG": "en_US.UTF-8"},
			isWSL: false,
			want:  true,
		},
		{
			name:  "Linux virtual console -> ascii",
			env:   map[string]string{"TERM": "linux", "LANG": "C.UTF-8"},
			isWSL: false,
			want:  false,
		},
		{
			name:  "Legacy vt100 or dumb -> ascii",
			env:   map[string]string{"TERM": "vt100", "LANG": "C.UTF-8"},
			isWSL: false,
			want:  false,
		},
		{
			name:  "Non-UTF8 C locale -> ascii",
			env:   map[string]string{"TERM": "xterm-256color", "LC_ALL": "C"},
			isWSL: false,
			want:  false,
		},
		{
			name:  "Asciinema recording -> fancy",
			env:   map[string]string{"TERM": "xterm-256color", "LANG": "C.UTF-8", "ASCIINEMA_REC": "1"},
			isWSL: true,
			want:  true,
		},
		{
			name:  "Explicit KROTEK_SYMBOLS=ascii overrides WT_SESSION",
			env:   map[string]string{"TERM": "xterm-256color", "LANG": "C.UTF-8", "WT_SESSION": "abc", "KROTEK_SYMBOLS": "ascii"},
			isWSL: false,
			want:  false,
		},
		{
			name:  "Explicit KROTEK_SYMBOLS=fancy overrides TERM=linux",
			env:   map[string]string{"TERM": "linux", "LC_ALL": "C", "KROTEK_SYMBOLS": "fancy"},
			isWSL: true,
			want:  true,
		},
		{
			name:  "Explicit KROTEK_ASCII=1 forces ascii",
			env:   map[string]string{"TERM": "xterm-256color", "LANG": "C.UTF-8", "KROTEK_ASCII": "1"},
			isWSL: false,
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectRichTerminalFromEnv(tc.env, tc.isWSL); got != tc.want {
				t.Fatalf("detectRichTerminalFromEnv(%v, wsl=%v) = %v, want %v", tc.env, tc.isWSL, got, tc.want)
			}
		})
	}
}

func TestSymbolSetsSingleWidthAndASCIIClean(t *testing.T) {
	// Every ASCIISymbols field must be strict 7-bit ASCII.
	asciiFields := []string{
		ASCIISymbols.BoxTL, ASCIISymbols.BoxTR, ASCIISymbols.BoxBL, ASCIISymbols.BoxBR,
		ASCIISymbols.BoxH, ASCIISymbols.BoxV, ASCIISymbols.BoxML, ASCIISymbols.BoxMR,
		ASCIISymbols.BoxTM, ASCIISymbols.BoxBM, ASCIISymbols.BoxCross,
		ASCIISymbols.Pointer, ASCIISymbols.Bullet, ASCIISymbols.Section,
		ASCIISymbols.DotSep, ASCIISymbols.EmDash, ASCIISymbols.ArrowRight, ASCIISymbols.ArrowLeft,
		ASCIISymbols.ArrowUp, ASCIISymbols.ArrowDown, ASCIISymbols.ActiveDot, ASCIISymbols.InactiveDot,
		ASCIISymbols.Check, ASCIISymbols.CrossMark, ASCIISymbols.GaugeFull, ASCIISymbols.GaugeEmpty,
		ASCIISymbols.IconOS, ASCIISymbols.IconHost, ASCIISymbols.IconUp, ASCIISymbols.IconService,
		ASCIISymbols.IconRAM, ASCIISymbols.IconCPU, ASCIISymbols.IconDisk, ASCIISymbols.IconNet,
		ASCIISymbols.IconPorts, ASCIISymbols.IconSites, ASCIISymbols.IconData, ASCIISymbols.IconSec,
		ASCIISymbols.IconDB, ASCIISymbols.IconUsers, ASCIISymbols.IconAlert, ASCIISymbols.IconTarget, ASCIISymbols.IconCmd,
	}
	for _, s := range asciiFields {
		for i := 0; i < len(s); i++ {
			if s[i] < 0x20 || s[i] > 0x7e {
				t.Fatalf("ASCIISymbols contains non-ASCII byte %#x in %q", s[i], s)
			}
		}
	}

	// Single-cell FancySymbols glyphs must all have display width 1.
	fancySingleCell := []string{
		FancySymbols.BoxTL, FancySymbols.BoxTR, FancySymbols.BoxBL, FancySymbols.BoxBR,
		FancySymbols.BoxH, FancySymbols.BoxV, FancySymbols.BoxML, FancySymbols.BoxMR,
		FancySymbols.BoxTM, FancySymbols.BoxBM, FancySymbols.BoxCross,
		FancySymbols.Pointer, FancySymbols.Bullet, FancySymbols.Section,
		FancySymbols.DotSep, FancySymbols.EmDash, FancySymbols.ArrowRight, FancySymbols.ArrowLeft,
		FancySymbols.ArrowUp, FancySymbols.ArrowDown, FancySymbols.ActiveDot, FancySymbols.InactiveDot,
		FancySymbols.Check, FancySymbols.CrossMark, FancySymbols.GaugeFull, FancySymbols.GaugeEmpty,
		FancySymbols.IconOS, FancySymbols.IconHost, FancySymbols.IconUp, FancySymbols.IconService,
		FancySymbols.IconRAM, FancySymbols.IconCPU, FancySymbols.IconDisk, FancySymbols.IconNet,
		FancySymbols.IconPorts, FancySymbols.IconSites, FancySymbols.IconData, FancySymbols.IconSec,
		FancySymbols.IconDB, FancySymbols.IconUsers, FancySymbols.IconAlert, FancySymbols.IconTarget, FancySymbols.IconCmd,
	}
	for _, g := range fancySingleCell {
		if w := RuneWidth(g); w != 1 {
			t.Fatalf("FancySymbols glyph %q has RuneWidth=%d, want 1", g, w)
		}
	}
	for _, frame := range FancySymbols.Spinner {
		if w := RuneWidth(frame); w != 1 {
			t.Fatalf("FancySymbols spinner frame %q has RuneWidth=%d, want 1", frame, w)
		}
	}
}

func TestSetSymbolModeAndRenderGauge(t *testing.T) {
	orig := RichTerminal
	defer SetSymbolMode(orig)

	SetSymbolMode(false)
	asciiGauge := StripANSI(RenderGauge(50, 10))
	if asciiGauge != "[#####-----] 50.0%" {
		t.Fatalf("ASCII RenderGauge = %q, want %q", asciiGauge, "[#####-----] 50.0%")
	}

	SetSymbolMode(true)
	fancyGauge := StripANSI(RenderGauge(50, 10))
	if fancyGauge != "[█████░░░░░] 50.0%" {
		t.Fatalf("Fancy RenderGauge = %q, want %q", fancyGauge, "[█████░░░░░] 50.0%")
	}
}
