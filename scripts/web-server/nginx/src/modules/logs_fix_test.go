package modules

import (
	"fmt"
	"strings"
	"testing"

	"krotek-nginx-installer/core"
)

func TestWrapANSIPreservesAndResetsStyle(t *testing.T) {
	styled := core.BOLD + core.RED + "alpha beta gamma delta" + core.NC
	lines := wrapANSI(styled, 12)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped lines, got %v", lines)
	}
	if !strings.HasPrefix(lines[1], core.BOLD+core.RED) {
		t.Fatalf("expected wrapped continuation to preserve BOLD+RED, got %q", lines[1])
	}

	mixed := core.RED + "error" + core.NC + " plain continuation words"
	mixedLines := wrapANSI(mixed, 12)
	if len(mixedLines) < 2 {
		t.Fatalf("expected wrapped mixed lines, got %v", mixedLines)
	}
	if strings.Contains(mixedLines[1], core.RED) {
		t.Fatalf("expected color reset after NC on continuation line, got %q", mixedLines[1])
	}
}

func TestBuildLogViewerFrameHeightAndBorders(t *testing.T) {
	origMode := core.RichTerminal
	defer core.SetSymbolMode(origMode)

	var raw []string
	for i := 1; i <= 50; i++ {
		raw = append(raw, fmt.Sprintf("Nov 15 00:06:26 DESKTOP systemd[1]: Line %d A high performance web server and a reverse proxy server.", i))
	}
	lf := LogFileTarget{Name: "System Daemon Journal", Path: "journalctl -u nginx", IsCmd: true}

	for _, rich := range []bool{false, true} {
		core.SetSymbolMode(rich)
		for _, dims := range [][2]int{{80, 24}, {105, 30}, {52, 16}} {
			innerW, termH := dims[0], dims[1]
			for _, offset := range []int{-1, 0, 5} {
				frame, clamped, totalRows, availH := buildLogViewerFrame("nginx", lf, "/var/log/very/long/path/that/wraps/across/multiple/lines/nginx.log", raw, innerW, termH, offset)
				if totalRows < availH {
					t.Fatalf("expected overflow log rows for %dx%d, got total=%d avail=%d", innerW, termH, totalRows, availH)
				}
				if clamped < 0 {
					t.Fatalf("expected non-negative clamped offset, got %d", clamped)
				}
				if offset == -1 && !strings.Contains(frame, core.Sym.ArrowUp+" [Up: scroll up for more lines]") {
					t.Fatalf("frame (%dx%d, rich=%v) snapped to bottom should show top scroll arrow", innerW, termH, rich)
				}
				if offset == 0 && !strings.Contains(frame, core.Sym.ArrowDown+" [Down: scroll down for more lines]") {
					t.Fatalf("frame (%dx%d, rich=%v) at offset=0 should show bottom scroll arrow", innerW, termH, rich)
				}

				if !strings.HasPrefix(frame, "\033[?7l\033[H") {
					t.Fatalf("frame (%dx%d) missing ?7l / home prefix", innerW, termH)
				}
				if strings.HasSuffix(frame, "\n") {
					t.Fatalf("frame (%dx%d) must NOT end with newline (causes top scroll-off)", innerW, termH)
				}
				if !strings.HasSuffix(frame, "\033[K\033[J\033[?7h") {
					t.Fatalf("frame (%dx%d) missing erase-below and ?7h suffix", innerW, termH)
				}

				body := strings.TrimPrefix(frame, "\033[?7l\033[H")
				body = strings.TrimSuffix(body, "\033[?7h")
				lines := strings.Split(body, "\r\n")
				if len(lines) > termH {
					t.Fatalf("frame (%dx%d, off=%d) rendered %d lines, exceeding termH=%d", innerW, termH, offset, len(lines), termH)
				}
				for idx, l := range lines {
					clean := core.StripANSI(l)
					if w := core.RuneWidth(clean); w != innerW+2 {
						t.Fatalf("frame (%dx%d, rich=%v) line %d width=%d, expected %d: %q", innerW, termH, rich, idx, w, innerW+2, clean)
					}
				}
			}
		}
	}
}
