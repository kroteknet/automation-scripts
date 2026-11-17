package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// SymbolSet defines all TUI structural and semantic glyphs for a rendering tier.
// Every glyph in FancySymbols is a single-width (non-emoji) Unicode character
// verified present in JetBrains Mono and Cascadia Mono.
// Every glyph in ASCIISymbols is strictly 7-bit printable ASCII.
type SymbolSet struct {
	// Box-drawing characters
	BoxTL    string
	BoxTR    string
	BoxBL    string
	BoxBR    string
	BoxH     string
	BoxV     string
	BoxML    string
	BoxMR    string
	BoxTM    string
	BoxBM    string
	BoxCross string

	// Navigation, lists, gauges, and state indicators
	Pointer     string
	Bullet      string
	Section     string
	DotSep      string
	EmDash      string
	ArrowRight  string
	ArrowLeft   string
	ArrowUp     string
	ArrowDown   string
	ActiveDot   string
	InactiveDot string
	Check       string
	CrossMark   string
	GaugeFull   string
	GaugeEmpty  string
	Spinner     []string

	// Semantic banner & telemetry icons
	IconOS      string
	IconHost    string
	IconUp      string
	IconService string
	IconRAM     string
	IconCPU     string
	IconDisk    string
	IconNet     string
	IconPorts   string
	IconSites   string
	IconData    string
	IconSec     string
	IconDB      string
	IconUsers   string
	IconAlert   string
	IconTarget  string
	IconCmd     string
}

// FancySymbols uses crisp straight-corner box drawing and technical/geometric Unicode glyphs (no emojis).
var FancySymbols = SymbolSet{
	BoxTL:    "┌",
	BoxTR:    "┐",
	BoxBL:    "└",
	BoxBR:    "┘",
	BoxH:     "─",
	BoxV:     "│",
	BoxML:    "├",
	BoxMR:    "┤",
	BoxTM:    "┬",
	BoxBM:    "┴",
	BoxCross: "┼",

	Pointer:     "▸",
	Bullet:      "•",
	Section:     "◆",
	DotSep:      "·",
	EmDash:      "—",
	ArrowRight:  "→",
	ArrowLeft:   "←",
	ArrowUp:     "▲",
	ArrowDown:   "▼",
	ActiveDot:   "●",
	InactiveDot: "○",
	Check:       "✓",
	CrossMark:   "×",
	GaugeFull:   "█",
	GaugeEmpty:  "░",
	Spinner:     []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},

	IconOS:      "◆",
	IconHost:    "⌂",
	IconUp:      "◷",
	IconService: "●",
	IconRAM:     "▤",
	IconCPU:     "◈",
	IconDisk:    "▥",
	IconNet:     "⊕",
	IconPorts:   "◎",
	IconSites:   "⊞",
	IconData:    "≡",
	IconSec:     "◉",
	IconDB:      "▦",
	IconUsers:   "▸",
	IconAlert:   "▲",
	IconTarget:  "▸",
	IconCmd:     "›",
}

// ASCIISymbols uses 7-bit printable keyboard ASCII for basic terminals (cmd.exe, conhost WSL, Linux VT, vt100).
var ASCIISymbols = SymbolSet{
	BoxTL:    "+",
	BoxTR:    "+",
	BoxBL:    "+",
	BoxBR:    "+",
	BoxH:     "-",
	BoxV:     "|",
	BoxML:    "+",
	BoxMR:    "+",
	BoxTM:    "+",
	BoxBM:    "+",
	BoxCross: "+",

	Pointer:     ">",
	Bullet:      "*",
	Section:     "*",
	DotSep:      "|",
	EmDash:      "-",
	ArrowRight:  "->",
	ArrowLeft:   "<-",
	ArrowUp:     "^",
	ArrowDown:   "v",
	ActiveDot:   "*",
	InactiveDot: "o",
	Check:       "+",
	CrossMark:   "x",
	GaugeFull:   "#",
	GaugeEmpty:  "-",
	Spinner:     []string{"|", "/", "-", "\\"},

	IconOS:      "#",
	IconHost:    "@",
	IconUp:      "~",
	IconService: "*",
	IconRAM:     "%",
	IconCPU:     "%",
	IconDisk:    "/",
	IconNet:     "@",
	IconPorts:   ":",
	IconSites:   "+",
	IconData:    "/",
	IconSec:     "*",
	IconDB:      "#",
	IconUsers:   "^",
	IconAlert:   "!",
	IconTarget:  ">",
	IconCmd:     "$",
}

// RichTerminal is true when fancy Unicode symbols are active, false in ASCII fallback mode.
var RichTerminal bool

// Sym holds the active symbol set selected at startup or overridden via SetSymbolMode.
var Sym SymbolSet

func init() {
	RefreshSymbols()
}

// RefreshSymbols re-evaluates the runtime environment and updates Sym and RichTerminal.
func RefreshSymbols() {
	SetSymbolMode(DetectRichTerminal())
}

// SetSymbolMode explicitly switches between fancy Unicode symbols (rich=true) and ASCII symbols (rich=false).
func SetSymbolMode(rich bool) {
	RichTerminal = rich
	if rich {
		Sym = FancySymbols
	} else {
		Sym = ASCIISymbols
	}
}

// SymbolModeArg parses CLI override flags (--ascii, --fancy, --unicode).
// Returns (rich, true) when recognized, or (false, false) otherwise.
func SymbolModeArg(arg string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "--ascii":
		return false, true
	case "--fancy", "--unicode":
		return true, true
	default:
		return false, false
	}
}

// DetectRichTerminal inspects current environment variables and (on Linux) ancestor
// /proc/<pid>/environ entries to survive `sudo` env_reset stripping.
func DetectRichTerminal() bool {
	env := collectEffectiveTerminalEnv()
	return detectRichTerminalFromEnv(env, isWSLEnv(env))
}

func detectRichTerminalFromEnv(env map[string]string, isWSL bool) bool {
	// 1. Explicit user/session overrides always win.
	switch strings.ToLower(strings.TrimSpace(env["KROTEK_SYMBOLS"])) {
	case "ascii", "plain", "basic":
		return false
	case "fancy", "unicode", "utf8", "rich":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(env["KROTEK_ASCII"])) {
	case "1", "true", "yes", "on":
		return false
	case "0", "false", "no", "off":
		return true
	}

	// 2. Non-UTF-8 locale cannot safely render Unicode box/geometric glyphs.
	if !hasUTF8Locale(env) {
		return false
	}

	// 3. Known legacy / minimal terminals strictly stay ASCII.
	term := strings.ToLower(strings.TrimSpace(env["TERM"]))
	switch term {
	case "", "dumb", "unknown", "linux", "vt100", "vt102", "vt220", "ansi", "cons25", "cygwin":
		return false
	}

	// 4. Strong indicators of a modern Unicode-capable terminal emulator.
	hasModernEmulator := env["WT_SESSION"] != "" ||
		env["WT_PROFILE_ID"] != "" ||
		env["ConEmuPID"] != "" ||
		env["TERMINATOR_UUID"] != "" ||
		env["KITTY_WINDOW_ID"] != "" ||
		env["WEZTERM_EXECUTABLE"] != "" ||
		env["ALACRITTY_SOCKET"] != "" ||
		env["ALACRITTY_WINDOW_ID"] != "" ||
		env["GHOSTTY_RESOURCES_DIR"] != "" ||
		env["VTE_VERSION"] != "" ||
		env["ITERM_SESSION_ID"] != "" ||
		env["ASCIINEMA_REC"] != ""

	switch strings.ToLower(strings.TrimSpace(env["TERM_PROGRAM"])) {
	case "vscode", "iterm.app", "apple_terminal", "wezterm", "hyper", "alacritty", "ghostty", "mintty", "rio", "warpterminal", "tabby", "windows terminal":
		hasModernEmulator = true
	}

	// 5. Under WSL, legacy cmd.exe/conhost.exe sets TERM=xterm-256color too,
	// so require an explicit modern emulator marker (or COLORTERM).
	if isWSL {
		if hasModernEmulator {
			return true
		}
		colorterm := strings.ToLower(strings.TrimSpace(env["COLORTERM"]))
		if colorterm == "truecolor" || colorterm == "24bit" {
			return true
		}
		return false
	}

	if hasModernEmulator {
		return true
	}

	// 6. Standard modern SSH / Linux desktop terminals with UTF-8 locale.
	if strings.Contains(term, "256color") ||
		strings.Contains(term, "truecolor") ||
		strings.Contains(term, "direct") ||
		strings.HasPrefix(term, "xterm-kitty") ||
		strings.HasPrefix(term, "alacritty") ||
		strings.HasPrefix(term, "wezterm") ||
		strings.HasPrefix(term, "screen") ||
		strings.HasPrefix(term, "tmux") ||
		strings.HasPrefix(term, "rxvt-unicode") {
		return true
	}

	colorterm := strings.ToLower(strings.TrimSpace(env["COLORTERM"]))
	if colorterm == "truecolor" || colorterm == "24bit" {
		return true
	}

	return false
}

func hasUTF8Locale(env map[string]string) bool {
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		val := strings.TrimSpace(env[key])
		if val == "" {
			continue
		}
		lower := strings.ToLower(val)
		return strings.Contains(lower, "utf-8") || strings.Contains(lower, "utf8")
	}
	// On macOS/modern containers LANG may be unset while TERM_PROGRAM/WT_SESSION is modern.
	return env["WT_SESSION"] != "" || env["TERM_PROGRAM"] != "" || env["COLORTERM"] != ""
}

func isWSLEnv(env map[string]string) bool {
	if env["WSL_DISTRO_NAME"] != "" || env["WSL_INTEROP"] != "" || env["WSLENV"] != "" {
		return true
	}
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
			lower := strings.ToLower(string(data))
			if strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl") {
				return true
			}
		}
	}
	return false
}

var inspectedEnvKeys = []string{
	"KROTEK_SYMBOLS",
	"KROTEK_ASCII",
	"LC_ALL",
	"LC_CTYPE",
	"LANG",
	"TERM",
	"COLORTERM",
	"TERM_PROGRAM",
	"WT_SESSION",
	"WT_PROFILE_ID",
	"ConEmuPID",
	"TERMINATOR_UUID",
	"KITTY_WINDOW_ID",
	"WEZTERM_EXECUTABLE",
	"ALACRITTY_SOCKET",
	"ALACRITTY_WINDOW_ID",
	"GHOSTTY_RESOURCES_DIR",
	"VTE_VERSION",
	"ITERM_SESSION_ID",
	"ASCIINEMA_REC",
	"WSL_DISTRO_NAME",
	"WSL_INTEROP",
	"WSLENV",
}

func collectEffectiveTerminalEnv() map[string]string {
	env := make(map[string]string, len(inspectedEnvKeys))
	for _, k := range inspectedEnvKeys {
		if v := os.Getenv(k); v != "" {
			env[k] = v
		}
	}
	// Recover terminal variables stripped by `sudo` when running as root on Linux.
	if runtime.GOOS != "linux" {
		return env
	}
	pid := os.Getppid()
	for depth := 0; depth < 6 && pid > 1; depth++ {
		if raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ")); err == nil {
			for _, entry := range strings.Split(string(raw), "\x00") {
				idx := strings.IndexByte(entry, '=')
				if idx <= 0 {
					continue
				}
				k := entry[:idx]
				v := entry[idx+1:]
				if v != "" && env[k] == "" && isInspectedEnvKey(k) {
					env[k] = v
				}
			}
		}
		ppid, ok := readProcParentPID(pid)
		if !ok || ppid <= 1 || ppid == pid {
			break
		}
		pid = ppid
	}
	return env
}

func isInspectedEnvKey(k string) bool {
	for _, candidate := range inspectedEnvKeys {
		if k == candidate {
			return true
		}
	}
	return false
}

func readProcParentPID(pid int) (int, bool) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, false
	}
	// Format: pid (comm) state ppid ...
	rparen := strings.LastIndexByte(string(data), ')')
	if rparen < 0 || rparen+2 >= len(data) {
		return 0, false
	}
	fields := strings.Fields(string(data[rparen+2:]))
	if len(fields) < 2 {
		return 0, false
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, false
	}
	return ppid, true
}

// BannerIcon selects the semantic icon for a banner or table label.
// In ASCII mode it returns fallbackToken unchanged so ASCII layouts keep classic keyboard glyphs.
func BannerIcon(fallbackToken, label string) string {
	if !RichTerminal {
		return fallbackToken
	}
	l := strings.ToLower(strings.TrimSpace(label))
	switch {
	case strings.Contains(l, "host"), strings.Contains(l, "node"), strings.Contains(l, "domain"), strings.Contains(l, "server"):
		return Sym.IconHost
	case l == "os", strings.HasPrefix(l, "os "), strings.Contains(l, "/ os"), strings.Contains(l, "platform"), strings.Contains(l, "distro"), strings.Contains(l, "edition"), strings.Contains(l, "engine"):
		return Sym.IconOS
	case strings.Contains(l, "up"), strings.Contains(l, "kernel"), strings.Contains(l, "time"), strings.Contains(l, "cache"), strings.Contains(l, "cron"), strings.Contains(l, "mem"):
		return Sym.IconUp
	case strings.Contains(l, "ram"):
		return Sym.IconRAM
	case strings.Contains(l, "cpu"), strings.Contains(l, "load"), strings.Contains(l, "worker"):
		return Sym.IconCPU
	case strings.Contains(l, "disk"), strings.Contains(l, "stor"), strings.Contains(l, "dir"), strings.Contains(l, "log"), strings.Contains(l, "root"), strings.Contains(l, "content"), l == "site":
		return Sym.IconDisk
	case strings.Contains(l, "ipv4"), strings.Contains(l, "ipv6"), strings.Contains(l, "ip "), l == "ip", strings.Contains(l, "net"), strings.Contains(l, "endpoint"), strings.Contains(l, "iface"):
		return Sym.IconNet
	case strings.Contains(l, "port"), strings.Contains(l, "listen"), strings.Contains(l, "http"), strings.Contains(l, "proto"), strings.Contains(l, "url"), strings.Contains(l, "panel"):
		return Sym.IconPorts
	case strings.Contains(l, "service"), strings.Contains(l, "status"), strings.Contains(l, "daemon"):
		return Sym.IconService
	case strings.Contains(l, "site"), strings.Contains(l, "vhost"), strings.Contains(l, "proxy"), strings.Contains(l, "stack"), strings.Contains(l, "web"), strings.Contains(l, "module"):
		return Sym.IconSites
	case strings.Contains(l, "ssl"), strings.Contains(l, "tls"), strings.Contains(l, "cert"), strings.Contains(l, "auth"), strings.Contains(l, "key"), strings.Contains(l, "jail"), strings.Contains(l, "firewall"), strings.Contains(l, "rule"):
		if strings.Contains(l, "keys") {
			return Sym.IconDB
		}
		return Sym.IconSec
	case strings.Contains(l, "db"), strings.Contains(l, "sql"), strings.Contains(l, "redis"), strings.Contains(l, "store"), strings.Contains(l, "table"), strings.Contains(l, "coll"):
		return Sym.IconDB
	case strings.Contains(l, "user"), strings.Contains(l, "client"), strings.Contains(l, "peer"), strings.Contains(l, "conn"), strings.Contains(l, "backend"):
		return Sym.IconUsers
	case strings.Contains(l, "alert"), strings.Contains(l, "warn"), strings.Contains(l, "safety"), strings.Contains(l, "ban"):
		return Sym.IconAlert
	case strings.Contains(l, "target"):
		return Sym.IconTarget
	case strings.Contains(l, "cli"), strings.Contains(l, "cmd"):
		return Sym.IconCmd
	case strings.Contains(l, "conf"), strings.Contains(l, "data"), strings.Contains(l, "mod"), strings.Contains(l, "php"), strings.Contains(l, "ver"), strings.Contains(l, "pkg"), strings.Contains(l, "core"):
		return Sym.IconData
	}

	switch fallbackToken {
	case "#":
		return Sym.IconOS
	case "@":
		return Sym.IconNet
	case "~":
		return Sym.IconUp
	case "%":
		return Sym.IconCPU
	case "/":
		return Sym.IconDisk
	case ":":
		return Sym.IconPorts
	case "+":
		return Sym.IconSites
	case "*":
		return Sym.IconService
	case "^":
		return Sym.IconUsers
	case "!":
		return Sym.IconAlert
	case ">":
		return Sym.IconTarget
	case "$":
		return Sym.IconCmd
	default:
		return fallbackToken
	}
}

// ServiceBadge returns a compact status tag adapted to the active symbol tier.
func ServiceBadge(running, installed bool) (tag, label, color string) {
	if !installed {
		if RichTerminal {
			return Sym.CrossMark, "Not Installed", RED
		}
		return "[--]", "Not Installed", RED
	}
	if running {
		if RichTerminal {
			return Sym.ActiveDot, "Active", GREEN
		}
		return "[OK]", "Active", GREEN
	}
	if RichTerminal {
		return Sym.InactiveDot, "Stopped", YELLOW
	}
	return "[--]", "Stopped", YELLOW
}

// FormatServiceBadge returns a colorized status badge such as "● Active" or "[OK] Active".
func FormatServiceBadge(running, installed bool, suffix string) string {
	tag, label, color := ServiceBadge(running, installed)
	return fmt.Sprintf("%s%s %s%s%s", color, tag, label, suffix, NC)
}

// TagOK returns "✓" in fancy Unicode mode or "[OK]" in ASCII mode.
func TagOK() string {
	if RichTerminal {
		return Sym.Check
	}
	return "[OK]"
}

// TagFail returns "×" in fancy Unicode mode or "[FAIL]" in ASCII mode.
func TagFail() string {
	if RichTerminal {
		return Sym.CrossMark
	}
	return "[FAIL]"
}

// TagPass returns "✓" in fancy Unicode mode or "[PASS]" in ASCII mode.
func TagPass() string {
	if RichTerminal {
		return Sym.Check
	}
	return "[PASS]"
}

// TagOff returns "○" in fancy Unicode mode or "[--]" in ASCII mode.
func TagOff() string {
	if RichTerminal {
		return Sym.InactiveDot
	}
	return "[--]"
}

