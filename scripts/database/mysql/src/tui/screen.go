// ==============================================================================
// KROTEK SCRIPT HUB — TERMINAL DIMENSION & RAW MODE CONTROLLER
// Service: MYSQL (database)
// File:    web-server/nginx/src/tui/screen.go
//
// Manages POSIX and Windows console modes, terminal window size discovery, cursor visibility, and alternate screen buffer transitions.
// ==============================================================================

package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"krotek-mysql-installer/core"
)

var (
	altScreenActive  bool
	termCacheMu      sync.Mutex
	cachedTermWidth  int
	cachedTermHeight int
	lastTermCheck    time.Time
	forcedViewMode   string // "", "compact", "expanded"
)

// EnterAltScreen enters the terminal private alternate screen buffer to prevent visual tearing and leave no history on exit.
func EnterAltScreen() {
	if core.AutoYes {
		return
	}
	if !altScreenActive {
		fmt.Print("\033[?1049h\033[H\033[2J")
		altScreenActive = true
	}
}

// LeaveAltScreen exits the terminal alternate screen buffer, restoring the previous shell contents and cursor position.
func LeaveAltScreen() {
	ResetInputCard()
	if altScreenActive {
		fmt.Print("\033[?1049l\033[?25h")
		altScreenActive = false
	}
}

func ClearScreen() {
	ResetInputCard()
	boxNeedsHeader = !core.AutoYes
	if core.AutoYes {
		return
	}
	fmt.Print("\033[H\033[2J")
}

func HideCursor() {
	fmt.Print("\033[?25l")
}

func ShowCursor() {
	fmt.Print("\033[?25h")
}

func IsAltScreenActive() bool {
	return altScreenActive
}

func IsDesktopEnvironment() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

func IsExpandedMode() bool {
	if forcedViewMode == "compact" {
		return false
	}
	if forcedViewMode == "expanded" {
		return true
	}
	envMode := strings.ToLower(os.Getenv("KROTEK_MODE"))
	if envMode == "expanded" {
		cols, lines := GetRawTermDimensions()
		return cols >= 100 && lines >= 20
	}
	if envMode == "compact" {
		return false
	}

	// Default to compact mode initially. Pressing [Tab] toggles to expanded dashboard.
	return false
}

func ToggleViewMode() {
	if IsExpandedMode() {
		forcedViewMode = "compact"
	} else {
		forcedViewMode = "expanded"
	}
	cachedTermWidth = 0
	cachedTermHeight = 0
}

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

func GetRawTermDimensions() (int, int) {
	// 1. Direct native IOCTL (TIOCGWINSZ on Linux/Unix PTY)
	if c, r := getIoctlDimensions(); c > 0 && r > 0 {
		return c, r
	}

	// 2. Try stty size with stdin attached to controlling terminal
	cmdStty := exec.Command("stty", "size")
	cmdStty.Stdin = os.Stdin
	if out, err := cmdStty.Output(); err == nil {
		fields := strings.Fields(string(out))
		if len(fields) >= 2 {
			r, err1 := strconv.Atoi(fields[0])
			c, err2 := strconv.Atoi(fields[1])
			if err1 == nil && err2 == nil && r > 0 && c > 0 {
				return c, r
			}
		}
	}

	// 3. Try tput cols and lines with stdin attached
	cols := 0
	lines := 0
	cmdTputC := exec.Command("tput", "cols")
	cmdTputC.Stdin = os.Stdin
	if out, err := cmdTputC.Output(); err == nil {
		if c, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil && c > 0 {
			cols = c
		}
	}
	cmdTputL := exec.Command("tput", "lines")
	cmdTputL.Stdin = os.Stdin
	if out, err := cmdTputL.Output(); err == nil {
		if l, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil && l > 0 {
			lines = l
		}
	}
	if cols > 0 && lines > 0 {
		return cols, lines
	}

	// 4. Fallback default
	return 120, 30
}

// InvalidateTermCache forces recalculation of terminal dimensions on next call.
func InvalidateTermCache() {
	termCacheMu.Lock()
	cachedTermWidth = 0
	cachedTermHeight = 0
	lastTermCheck = time.Time{}
	termCacheMu.Unlock()
}

func getCachedTermDimensions() (int, int) {
	termCacheMu.Lock()
	defer termCacheMu.Unlock()

	if cachedTermWidth > 0 && cachedTermHeight > 0 && time.Since(lastTermCheck) < 200*time.Millisecond {
		return cachedTermWidth, cachedTermHeight
	}

	cols, lines := GetRawTermDimensions()
	w := cols - 2
	if w < 10 {
		w = 10
	}
	expanded := forcedViewMode == "expanded" || (forcedViewMode == "" && strings.ToLower(os.Getenv("KROTEK_MODE")) == "expanded" && cols >= 100 && lines >= 20)
	if expanded {
		if w < 76 && cols >= 78 {
			w = 76
		}
	} else if w > 105 {
		w = 105
	}
	if lines < 5 {
		lines = 5
	}

	cachedTermWidth = w
	cachedTermHeight = lines
	lastTermCheck = time.Now()
	return cachedTermWidth, cachedTermHeight
}

func GetTermWidth() int {
	w, _ := getCachedTermDimensions()
	return w
}

func GetTermHeight() int {
	_, h := getCachedTermDimensions()
	return h
}

var _ = core.NC
