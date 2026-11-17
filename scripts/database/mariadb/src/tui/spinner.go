// ==============================================================================
// KROTEK SCRIPT HUB — ANIMATED TERMINAL SPINNER & TASK FEEDBACK
// Service: MARIADB (database)
// File:    database/mariadb/src/tui/spinner.go
//
// Displays a smooth animated braille terminal spinner for long-running asynchronous system operations (package installation, downloads, builds).
// ==============================================================================

package tui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"krotek-mariadb-installer/core"
)

var (
	spinnerMu       sync.Mutex
	spinnerActive   bool
	spinnerMsg      string
	spinnerStopChan chan struct{}
	spinnerWg       sync.WaitGroup

	boxSpinnerMu       sync.Mutex
	boxSpinnerActive   bool
	boxSpinnerAutoYes  bool
	boxSpinnerMsg      string
	boxSubStep         string
	boxSpinnerStopChan chan struct{}
	boxSpinnerWg       sync.WaitGroup
)

// StartSpinner starts a background animated braille spinner goroutine with a status message.
func StartSpinner(msg string) {
	if core.BoxSpinnerActive.Load() {
		boxSpinnerMu.Lock()
		boxSubStep = strings.TrimSpace(core.SanitizeTerminalText(msg))
		boxSpinnerMu.Unlock()
		return
	}
	if spinnerActive {
		StopSpinner(0)
	}
	spinnerActive = true
	core.SpinnerActive.Store(true)
	spinnerMsg = msg
	startOffset := getLogFileSize(core.LogFilePath)
	spinnerStopChan = make(chan struct{})
	spinnerWg.Add(1)

	go func() {
		defer spinnerWg.Done()
		idx := 0
		for {
			select {
			case <-spinnerStopChan:
				return
			default:
				lastLine := core.SanitizeTerminalText(getLastLogLineSince(core.LogFilePath, startOffset, 40))
				spinnerMu.Lock()
				if lastLine != "" {
					fmt.Printf("\r\033[K%s[INFO]%s %s... %s%s%s \033[2m%s\033[0m", core.BLUE, core.NC, msg, core.CYAN+core.BOLD, core.Sym.Spinner[idx%len(core.Sym.Spinner)], core.NC, lastLine)
				} else {
					fmt.Printf("\r\033[K%s[INFO]%s %s... %s%s%s", core.BLUE, core.NC, msg, core.CYAN+core.BOLD, core.Sym.Spinner[idx%len(core.Sym.Spinner)], core.NC)
				}
				spinnerMu.Unlock()
				idx++
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
}

func getLogFileSize(filePath string) int64 {
	if filePath == "" {
		return 0
	}
	fi, err := os.Stat(filePath)
	if err != nil {
		return 0
	}
	return fi.Size()
}

func getLastLogLineSince(filePath string, minOffset int64, maxLen int) string {
	if filePath == "" {
		return ""
	}
	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil || fi.Size() <= minOffset {
		return ""
	}

	available := fi.Size() - minOffset
	readSize := int64(2048)
	if available < readSize {
		readSize = available
	}

	buf := make([]byte, readSize)
	offset := fi.Size() - readSize
	n, err := f.ReadAt(buf, offset)
	if err != nil && n == 0 {
		return ""
	}

	lines := strings.Split(string(buf[:n]), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" {
			if len(trimmed) > maxLen {
				return trimmed[:maxLen]
			}
			return trimmed
		}
	}
	return ""
}

// StopSpinner terminates the active spinner, printing a success checkmark (0) or failure cross (non-zero).
func StopSpinner(exitCode int) {
	if core.BoxSpinnerActive.Load() && !spinnerActive {
		boxSpinnerMu.Lock()
		boxSubStep = ""
		boxSpinnerMu.Unlock()
		return
	}
	if !spinnerActive {
		return
	}
	spinnerActive = false
	close(spinnerStopChan)
	spinnerWg.Wait()

	spinnerMu.Lock()
	core.SpinnerActive.Store(false)
	if exitCode == 0 && spinnerMsg != "" {
		fmt.Printf("\r\033[K%s%s%s %s\r\n", core.GREEN, core.TagOK(), core.NC, spinnerMsg)
	} else if exitCode != 0 && spinnerMsg != "" {
		fmt.Printf("\r\033[K%s%s%s %s\r\n", core.RED, core.TagFail(), core.NC, spinnerMsg)
	}
	spinnerMsg = ""
	spinnerMu.Unlock()
}

// StartBoxSpinner starts an animated spinner embedded inside an active TUI box.
// It replaces the bottom-most navigation / help line (Line N-1) with the spinner
// and redraws the bottom border (Line N) without disrupting surrounding box borders.
func StartBoxSpinner(msg string) {
	if boxSpinnerActive {
		StopBoxSpinner(0, "")
	}
	boxSpinnerActive = true
	boxSpinnerMsg = msg
	if core.AutoYes {
		boxSpinnerAutoYes = true
		fmt.Printf("%s[INFO]%s %s...\r\n", core.BLUE, core.NC, msg)
		return
	}

	boxSpinnerAutoYes = false
	core.BoxSpinnerActive.Store(true)
	boxSpinnerStopChan = make(chan struct{})

	boxSpinnerMu.Lock()
	boxSubStep = ""
	HideCursor()
	fmt.Print("\033[?7l")
	if boxNeedsHeader {
		drawTaskBoxHeader()
	}
	boxNeedsHeader = false
	boxSpinnerMu.Unlock()

	boxSpinnerStartedAt = time.Now()
	startedAt := boxSpinnerStartedAt
	boxSpinnerWg.Add(1)

	go func() {
		defer boxSpinnerWg.Done()
				idx := 0

		for {
			select {
			case <-boxSpinnerStopChan:
				return
			default:
				innerW := GetTermWidth()
				border := strings.Repeat(core.Sym.BoxH, innerW)

				boxSpinnerMu.Lock()
				sub := boxSubStep
				spinText := fmt.Sprintf(" %s%s%s  %s%s%s", core.CYAN+core.BOLD, core.Sym.Spinner[idx%len(core.Sym.Spinner)], core.NC, core.BOLD, msg, core.NC)
				if sub != "" {
					spinText += fmt.Sprintf("  %s%s %s%s", core.DIM, core.Sym.EmDash, sub, core.NC)
				}
				if el := time.Since(startedAt); el >= 2*time.Second {
					spinText += fmt.Sprintf("  %s[%s]%s", core.DIM, formatElapsed(el), core.NC)
				}
				content := PadOrTruncate(spinText, innerW-2)
				lineStr := core.BLUE + core.Sym.BoxV + " " + content + core.NC + " " + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n"
				botBorder := core.BLUE + core.Sym.BoxBL + border + core.Sym.BoxBR + core.NC + "\033[K\r"
				fmt.Print("\033[1A\r" + lineStr + botBorder)
				boxSpinnerMu.Unlock()
				idx++
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
}

// StopBoxSpinner terminates the in-box spinner, prints the final result with an icon,
// closes the bottom border cleanly, and positions the cursor below the box.
func StopBoxSpinner(exitCode int, statusMsg string) {
	if !boxSpinnerActive {
		return
	}
	boxSpinnerActive = false

	msg := statusMsg
	if msg == "" {
		if exitCode == 0 {
			msg = boxSpinnerMsg + " completed successfully."
		} else {
			msg = boxSpinnerMsg + " failed."
		}
	}

	if boxSpinnerAutoYes {
		boxSpinnerAutoYes = false
		if exitCode == 0 {
			fmt.Printf("%s%s%s %s\r\n", core.GREEN, core.TagOK(), core.NC, msg)
		} else {
			fmt.Printf("%s%s%s %s\r\n", core.RED, core.TagFail(), core.NC, msg)
		}
		boxSpinnerMsg = ""
		return
	}

	close(boxSpinnerStopChan)
	boxSpinnerWg.Wait()

	boxSpinnerMu.Lock()
	innerW := GetTermWidth()
	border := strings.Repeat(core.Sym.BoxH, innerW)

	icon := core.GREEN + core.BOLD + core.TagOK() + core.NC
	textColor := core.BOLD + core.GREEN
	if exitCode != 0 {
		icon = core.RED + core.BOLD + core.TagFail() + core.NC
		textColor = core.BOLD + core.RED
	}

	if el := time.Since(boxSpinnerStartedAt); !boxSpinnerStartedAt.IsZero() && el >= 2*time.Second {
		msg += fmt.Sprintf(" (%s)", formatElapsed(el))
	}
	statusText := fmt.Sprintf(" %s  %s%s%s", icon, textColor, msg, core.NC)
	content := PadOrTruncate(statusText, innerW-2)
	lineStr := core.BLUE + core.Sym.BoxV + " " + content + core.NC + " " + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n"
	botBorder := core.BLUE + core.Sym.BoxBL + border + core.Sym.BoxBR + core.NC + "\033[K\r\n"
	fmt.Print("\033[1A\r" + lineStr + botBorder + "\033[?7h")
	ShowCursor()
	boxSubStep = ""
	boxSpinnerMsg = ""
	boxSpinnerMu.Unlock()
	core.BoxSpinnerActive.Store(false)
}

// ------------------------------------------------------------------------------
// Task progress box (auto-framed spinner rows)
// ------------------------------------------------------------------------------

var (
	// boxNeedsHeader is set by ClearScreen and cleared by any full-screen renderer.
	// When true, the next StartBoxSpinner first draws a complete box header so the
	// spinner rows are rendered inside a proper card instead of a bare line at the
	// top of an otherwise empty screen.
	boxNeedsHeader bool
	// taskBoxTitle optionally overrides the auto-generated progress box title.
	taskBoxTitle string
	// boxSpinnerStartedAt records when the active box spinner started (elapsed timer).
	boxSpinnerStartedAt time.Time
)

// SetTaskBoxTitle overrides the title of the next auto-drawn progress box ("" = default).
func SetTaskBoxTitle(title string) {
	taskBoxTitle = title
}

// drawTaskBoxHeader renders the top frame (border, title, separator) and a closing
// bottom border on a freshly cleared screen. StartBoxSpinner then replaces the bottom
// border with its row, so consecutive tasks stack neatly inside a single card.
func drawTaskBoxHeader() {
	innerW := GetTermWidth()
	border := strings.Repeat(core.Sym.BoxH, innerW)

	title := taskBoxTitle
	if title == "" {
		name := getTechnologyInfo().Name
		if name == "" {
			title = "Task Progress"
		} else {
			title = name + " " + core.Sym.EmDash + " Task Progress"
		}
	}
	title = PadOrTruncate(title, innerW-2)
	title = strings.TrimRight(title, " ")
	pad := innerW - core.RuneWidth(title)
	if pad < 0 {
		pad = 0
	}
	padL := pad / 2
	padR := pad - padL

	var buf strings.Builder
	buf.WriteString(core.BLUE + core.Sym.BoxTL + border + core.Sym.BoxTR + core.NC + "\033[K\r\n")
	buf.WriteString(core.BLUE + core.Sym.BoxV + strings.Repeat(" ", padL) + core.BOLD + core.CYAN + title + core.NC + strings.Repeat(" ", padR) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
	buf.WriteString(core.BLUE + core.Sym.BoxML + border + core.Sym.BoxMR + core.NC + "\033[K\r\n")
	buf.WriteString(core.BLUE + core.Sym.BoxBL + border + core.Sym.BoxBR + core.NC + "\033[K\r\n")
	fmt.Print(buf.String())
}

// formatElapsed renders a compact elapsed-time label (e.g. "42s", "3m 05s").
func formatElapsed(d time.Duration) string {
	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm %02ds", secs/60, secs%60)
}
