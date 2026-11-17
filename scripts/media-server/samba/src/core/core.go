// ==============================================================================
// KROTEK SCRIPT HUB — CORE TYPES, STYLING PALETTE & ANSI UTILITIES
// Service: SAMBA (media-server)
// File:    media-server/samba/src/core/core.go
//
// Defines the system information data structures, global runtime configuration flags, muted 256-color ANSI terminal palette, and Unicode text measurement helpers.
// ==============================================================================

package core

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

const (
	NC     = "\033[0m"
	GREEN  = "\033[38;5;114m" // Muted sage/mint green
	RED    = "\033[38;5;203m" // Muted soft coral red
	YELLOW = "\033[38;5;215m" // Muted warm amber
	BLUE   = "\033[38;5;67m"  // Muted steel slate blue (calm borders)
	BOLD   = "\033[1m"
	CYAN   = "\033[38;5;110m" // Soft steel cyan
	PURPLE = "\033[38;5;139m" // Soft lavender
	WHITE  = "\033[38;5;253m" // Soft clean white
	DIM    = "\033[38;5;244m" // Charcoal gray dim
)

const DefaultKeyWidth = 22

// SystemInfo represents discovered host operating system, architecture, and init-system telemetry.
type SystemInfo struct {
	Distro       string
	Family       string
	Version      string
	Architecture string
	InitSystem   string
}

var (
	AnsiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)
	AutoYes   bool
	// OverwriteExistingConfig controls whether Debian dpkg and configuration templates preserve existing host settings (--force-confold) or overwrite with clean factory defaults (--force-confnew).
	OverwriteExistingConfig bool
	LogFilePath             string
	LogFile                 *os.File
	SysInfo                 SystemInfo
	StateFile               = "/etc/.samba_state"

	BannerProvider   func(string, int) []string
	BoxSpinnerActive atomic.Bool
	SpinnerActive    atomic.Bool
)

// StripANSI removes ANSI escape sequences from a string to determine true visible character width.
func StripANSI(str string) string {
	return AnsiRegex.ReplaceAllString(str, "")
}

var (
	oscSeqRegex = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	escSeqRegex = regexp.MustCompile(`\x1b(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~])`)
)

// SanitizeTerminalText strips ANSI/OSC/terminal escape sequences and unsafe control characters
// from untrusted text (e.g. HTTP logs, external tool output) before terminal rendering.
func SanitizeTerminalText(str string) string {
	s := oscSeqRegex.ReplaceAllString(str, "")
	s = escSeqRegex.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\t' {
			b.WriteString("  ")
		} else if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			continue
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func runeDisplayWidth(r rune) int {
	if (r >= 0x1100 &&
		(r <= 0x115f ||
			r == 0x2329 || r == 0x232a ||
			(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
			(r >= 0xac00 && r <= 0xd7a3) ||
			(r >= 0xf900 && r <= 0xfaff) ||
			(r >= 0xfe10 && r <= 0xfe19) ||
			(r >= 0xfe30 && r <= 0xfe6f) ||
			(r >= 0xff00 && r <= 0xff60) ||
			(r >= 0xffe0 && r <= 0xffe6) ||
			(r >= 0x20000 && r <= 0x2fffd) ||
			(r >= 0x30000 && r <= 0x3fffd))) ||
		(r >= 0x1f300 && r <= 0x1f64f) ||
		(r >= 0x1f900 && r <= 0x1f9ff) {
		return 2
	}
	if r < 32 || (r >= 127 && r < 160) {
		return 0
	}
	return 1
}

// RuneWidth calculates the visual monospace column width of a string, accounting for wide East Asian and emoji runes.
func RuneWidth(str string) int {
	clean := StripANSI(str)
	w := 0
	for _, r := range clean {
		w += runeDisplayWidth(r)
	}
	_ = utf8.RuneError
	return w
}

func PadANSI(str string, width int) string {
	w := RuneWidth(str)
	if w >= width {
		return str
	}
	return str + strings.Repeat(" ", width-w)
}

func PadOrTruncateANSI(str string, width int) string {
	w := RuneWidth(str)
	if w == width {
		return str
	}
	if w < width {
		return str + strings.Repeat(" ", width-w)
	}
	if width <= 0 {
		return ""
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}

	target := width - 3
	var buf strings.Builder
	curW := 0
	inEsc := false

	for i := 0; i < len(str); i++ {
		b := str[i]
		if b == 0x1b {
			inEsc = true
			buf.WriteByte(b)
			continue
		}
		if inEsc {
			buf.WriteByte(b)
			if b >= 0x40 && b <= 0x7e && b != '[' {
				inEsc = false
			}
			continue
		}

		r, size := utf8.DecodeRuneInString(str[i:])
		rW := runeDisplayWidth(r)

		if curW+rW > target {
			break
		}

		buf.WriteString(str[i : i+size])
		curW += rW
		i += size - 1
	}

	buf.WriteString("...")
	rem := width - (curW + 3)
	if rem > 0 {
		buf.WriteString(strings.Repeat(" ", rem))
	}
	buf.WriteString("\033[0m")
	return buf.String()
}

// ExtractColor collects all leading active ANSI SGR sequences preceding the first visible non-space character.
func ExtractColor(text string) string {
	var active []string
	i := 0
	for i < len(text) {
		if text[i] == '\x1b' && i+1 < len(text) && text[i+1] == '[' {
			loc := AnsiRegex.FindStringIndex(text[i:])
			if loc != nil && loc[0] == 0 {
				seq := text[i : i+loc[1]]
				if seq == "\033[0m" || seq == "\033[m" {
					active = nil
				} else {
					active = append(active, seq)
				}
				i += loc[1]
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == ' ' || r == '\t' {
			i += size
			continue
		}
		break
	}
	return strings.Join(active, "")
}

func SplitKeyValue(text string) (string, string, bool) {
	idx := strings.Index(text, ": ")
	if idx == -1 {
		return "", "", false
	}
	return text[:idx+2], text[idx+2:], true
}

// RenderGauge formats a graphical ANSI gauge bar [■■■□□□□□] with percentage.
func RenderGauge(percent float64, barWidth int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	if barWidth < 4 {
		barWidth = 6
	}
	filled := int((percent / 100.0) * float64(barWidth))
	if percent > 0.5 && filled == 0 {
		filled = 1
	}
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled
	if empty < 0 {
		empty = 0
	}

	color := GREEN
	if percent > 60 {
		color = YELLOW
	}
	if percent > 85 {
		color = RED
	}

	return DIM + "[" + NC + color + strings.Repeat(Sym.GaugeFull, filled) +
		DIM + strings.Repeat(Sym.GaugeEmpty, empty) + DIM + "]" + NC + " " +
		color + fmt.Sprintf("%4.1f%%", percent) + NC
}
