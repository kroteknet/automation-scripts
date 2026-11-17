// ==============================================================================
// KROTEK SCRIPT HUB — REAL-TIME SERVICE LOG STREAMER & VIEWER
// Service: EMBY (media-server)
// File:    media-server/emby/src/modules/logs.go
//
// Aggregates and displays real-time log output from systemd journalctl, container engines, and service-specific access/error log files with ANSI severity formatting.
// ==============================================================================

package modules

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"krotek-emby-installer/core"
	"krotek-emby-installer/tui"
)

type LogFileTarget struct {
	Name  string
	Path  string
	IsCmd bool
}

func getServiceLogTargets() []LogFileTarget {
	return []LogFileTarget{
		{Name: "Emby Server Journal", Path: "journalctl -u emby-server", IsCmd: true},
		{Name: "Emby Server Log", Path: "/var/lib/emby/logs/*.log|/var/log/emby-server.log", IsCmd: false},
		{Name: "Docker Container Logs", Path: "docker logs --tail 100 krotek-emby", IsCmd: true},
		{Name: "Krotek Installer Log", Path: "/var/log/emby-manager/install.log", IsCmd: false},
	}
}

var (
	http2xxRegex = regexp.MustCompile(`" 2[0-9]{2} `)
	http4xxRegex = regexp.MustCompile(`" 4[0-9]{2} `)
	http5xxRegex = regexp.MustCompile(`" 5[0-9]{2} `)
)

// parseAndFormatLogLine parses a single raw log line and returns an ANSI-styled
// string and a severity label (INFO, WARN, ERROR, DEBUG).
// Supports structured JSON logs and plain text with HTTP status code colouring.
func parseAndFormatLogLine(raw string) (string, string) {
	trimmed := strings.TrimSpace(core.SanitizeTerminalText(raw))
	if trimmed == "" {
		return "", ""
	}

	severity := "INFO"

	// Check if structured JSON log (MongoDB, Docker, Caddy, etc.)
	if strings.HasPrefix(trimmed, "{") && (strings.Contains(trimmed, "\"msg\"") || strings.Contains(trimmed, "\"message\"")) {
		var timeStr, levelStr, compStr, msgStr, extraStr string

		// Extract date/time
		if idx := strings.Index(trimmed, "\"$date\":\""); idx != -1 {
			datePart := trimmed[idx+9:]
			if endQ := strings.Index(datePart, "\""); endQ != -1 {
				fullDate := datePart[:endQ]
				if tIdx := strings.Index(fullDate, "T"); tIdx != -1 && len(fullDate) >= tIdx+9 {
					timeStr = fullDate[tIdx+1 : tIdx+9]
				} else if len(fullDate) >= 19 {
					timeStr = fullDate[:19]
				}
			}
		}

		// Extract severity
		if idx := strings.Index(trimmed, "\"s\":\""); idx != -1 {
			sPart := trimmed[idx+5:]
			if endQ := strings.Index(sPart, "\""); endQ != -1 {
				sCode := sPart[:endQ]
				switch sCode {
				case "E", "F":
					levelStr = "ERROR"
					severity = "ERROR"
				case "W":
					levelStr = "WARN"
					severity = "WARN"
				case "D":
					levelStr = "DEBUG"
					severity = "DEBUG"
				default:
					levelStr = "INFO"
				}
			}
		}

		// Extract component
		if idx := strings.Index(trimmed, "\"c\":\""); idx != -1 {
			cPart := trimmed[idx+5:]
			if endQ := strings.Index(cPart, "\""); endQ != -1 {
				compStr = cPart[:endQ]
			}
		}

		// Extract msg
		if idx := strings.Index(trimmed, "\"msg\":\""); idx != -1 {
			mPart := trimmed[idx+7:]
			if endQ := strings.Index(mPart, "\""); endQ != -1 {
				msgStr = mPart[:endQ]
			}
		}

		// Extract remote/client
		if idx := strings.Index(trimmed, "\"remote\":\""); idx != -1 {
			rPart := trimmed[idx+10:]
			if endQ := strings.Index(rPart, "\""); endQ != -1 {
				extraStr = "remote: " + rPart[:endQ]
			}
		}

		var formatted strings.Builder
		if timeStr != "" {
			formatted.WriteString(fmt.Sprintf("%s[%s]%s ", core.DIM+core.CYAN, timeStr, core.NC))
		}
		switch levelStr {
		case "ERROR":
			formatted.WriteString(fmt.Sprintf("%s[ERROR]%s ", core.BOLD+core.RED, core.NC))
		case "WARN":
			formatted.WriteString(fmt.Sprintf("%s[WARN]%s  ", core.BOLD+core.YELLOW, core.NC))
		case "DEBUG":
			formatted.WriteString(fmt.Sprintf("%s[DEBUG]%s ", core.DIM, core.NC))
		default:
			formatted.WriteString(fmt.Sprintf("%s[INFO]%s  ", core.GREEN, core.NC))
		}
		if compStr != "" {
			formatted.WriteString(fmt.Sprintf("%s[%s]%s ", core.PURPLE, compStr, core.NC))
		}
		if msgStr != "" {
			formatted.WriteString(msgStr)
		} else {
			formatted.WriteString(trimmed)
		}
		if extraStr != "" {
			formatted.WriteString(fmt.Sprintf(" %s(%s)%s", core.DIM, extraStr, core.NC))
		}
		return formatted.String(), severity
	}

	// Plain text log line
	upper := strings.ToUpper(trimmed)
	if strings.Contains(upper, "ERROR") || strings.Contains(upper, "FATAL") || strings.Contains(upper, "CRITICAL") {
		severity = "ERROR"
	} else if strings.Contains(upper, "WARN") || strings.Contains(upper, "WARNING") {
		severity = "WARN"
	}

	styled := trimmed
	if severity == "ERROR" {
		styled = core.BOLD + core.RED + styled + core.NC
	} else if severity == "WARN" {
		styled = core.BOLD + core.YELLOW + styled + core.NC
	} else if strings.Contains(upper, "INFO") {
		styled = core.CYAN + styled + core.NC
	}

	styled = http2xxRegex.ReplaceAllString(styled, "\" "+core.GREEN+"$0"+core.NC+" ")
	styled = http4xxRegex.ReplaceAllString(styled, "\" "+core.YELLOW+"$0"+core.NC+" ")
	styled = http5xxRegex.ReplaceAllString(styled, "\" "+core.RED+"$0"+core.NC+" ")

	return styled, severity
}

type ansiStyleState struct {
	bold  bool
	dim   bool
	color string
}

func (s *ansiStyleState) applySeq(seq string) {
	if seq == "\033[0m" || seq == "\033[m" {
		s.bold = false
		s.dim = false
		s.color = ""
		return
	}
	if seq == "\033[1m" {
		s.bold = true
		return
	}
	if seq == "\033[2m" {
		s.dim = true
		return
	}
	if strings.Contains(seq, "1;") {
		s.bold = true
	}
	s.color = seq
}

func (s ansiStyleState) prefix() string {
	var b strings.Builder
	if s.bold {
		b.WriteString(core.BOLD)
	}
	if s.dim {
		b.WriteString(core.DIM)
	}
	b.WriteString(s.color)
	return b.String()
}

// wrapANSI wraps ANSI-coloured text to maxW visible characters per line,
// tracking the cumulative active SGR style across line boundaries.
func wrapANSI(text string, maxW int) []string {
	if maxW <= 5 {
		maxW = 5
	}
	clean := core.StripANSI(text)
	if core.RuneWidth(clean) <= maxW {
		return []string{text}
	}

	var result []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	curLine := ""
	curLen := 0
	var style ansiStyleState

	for _, w := range words {
		prefixAtStart := style.prefix()
		for _, seq := range core.AnsiRegex.FindAllString(w, -1) {
			style.applySeq(seq)
		}
		wClean := core.StripANSI(w)
		wCleanLen := core.RuneWidth(wClean)

		if wCleanLen > maxW {
			if curLine != "" {
				result = append(result, curLine+core.NC)
				curLine = ""
				curLen = 0
			}
			chunk := ""
			chunkLen := 0
			for _, r := range wClean {
				chunk += string(r)
				chunkLen++
				if chunkLen >= maxW {
					result = append(result, prefixAtStart+chunk+core.NC)
					chunk = ""
					chunkLen = 0
				}
			}
			if chunk != "" {
				curLine = style.prefix() + chunk
				curLen = chunkLen
			}
			continue
		}

		if curLen == 0 {
			if prefixAtStart != "" && !strings.HasPrefix(w, "\033") {
				curLine = prefixAtStart + w
			} else {
				curLine = w
			}
			curLen = wCleanLen
		} else if curLen+1+wCleanLen <= maxW {
			curLine += " " + w
			curLen += 1 + wCleanLen
		} else {
			result = append(result, curLine+core.NC)
			if prefixAtStart != "" && !strings.HasPrefix(w, "\033") {
				curLine = prefixAtStart + w
			} else {
				curLine = w
			}
			curLen = wCleanLen
		}
	}

	if curLine != "" {
		result = append(result, curLine+core.NC)
	}
	return result
}

// showLogsMenu presents the log file selector menu and dispatches to renderLogViewer
// for the chosen log target.
func ShowLogsMenu() {
	logTargets := getServiceLogTargets()
	for {
		var options []string
		options = append(options, "─── SELECT LOG TARGET ───")
		for _, lf := range logTargets {
			displayPath := lf.Path
			if !lf.IsCmd && strings.Contains(displayPath, "|") {
				for _, alt := range strings.Split(displayPath, "|") {
					alt = strings.TrimSpace(alt)
					if _, err := os.Stat(alt); err == nil {
						displayPath = alt
						break
					}
				}
				if strings.Contains(displayPath, "|") {
					displayPath = strings.Split(displayPath, "|")[0]
				}
			}
			options = append(options, fmt.Sprintf("%-28s  ->  %s", lf.Name, displayPath))
		}
		options = append(options, "─── NAVIGATION ───")
		options = append(options, "Back to Main Menu")

		titleStr := "emby-manager  -  Log File Inspector"
		_, selectedOpt, status := tui.SelectMenu(titleStr, options)
		if status == 2 || (status == 0 && core.StripANSI(selectedOpt) == "Back to Main Menu") {
			return
		}

		cleanOpt := core.StripANSI(selectedOpt)
		for _, lf := range logTargets {
			if strings.Contains(cleanOpt, lf.Name) {
				renderLogViewer("emby", lf)
				break
			}
		}
	}
}

func getShell() string {
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash"
	}
	return "sh"
}

func buildLogViewerFrame(srvName string, lf LogFileTarget, targetDisplayPath string, rawLogLines []string, innerW, termH, scrollOffset int) (string, int, int, int) {
	if innerW < 10 {
		innerW = 10
	}
	border := strings.Repeat(core.Sym.BoxH, innerW)

	var nonEmptyLines []string
	for _, l := range rawLogLines {
		if strings.TrimSpace(l) != "" {
			nonEmptyLines = append(nonEmptyLines, l)
		}
	}
	if len(nonEmptyLines) > 1000 {
		nonEmptyLines = nonEmptyLines[len(nonEmptyLines)-1000:]
	}

	errCount := 0
	warnCount := 0

	maxContentW := innerW - 10
	if maxContentW < 12 {
		maxContentW = 12
	}

	var allLogRows []string
	entryIdx := 1
	for _, line := range nonEmptyLines {
		formatted, sev := parseAndFormatLogLine(line)
		if sev == "ERROR" {
			errCount++
		} else if sev == "WARN" {
			warnCount++
		}

		subLines := wrapANSI(formatted, maxContentW)
		for subIdx, subL := range subLines {
			var rowText string
			if subIdx == 0 {
				lineNumStr := fmt.Sprintf("%s%3d%s%s ", core.DIM, entryIdx, core.Sym.BoxV, core.NC)
				rowText = lineNumStr + subL
			} else {
				indentStr := fmt.Sprintf("%s   %s%s   ", core.DIM, core.Sym.BoxV, core.NC)
				rowText = indentStr + subL
			}
			allLogRows = append(allLogRows, " "+tui.PadOrTruncate(rowText, innerW-2)+" ")
		}
		entryIdx++
	}

	if len(allLogRows) == 0 {
		msg := fmt.Sprintf("%sNo log records found for %s at %s%s", core.DIM+core.YELLOW, lf.Name, targetDisplayPath, core.NC)
		allLogRows = append(allLogRows, " "+tui.PadOrTruncate(msg, innerW-2)+" ")
	}

	boxTitle := fmt.Sprintf("%s - %s (Tail View)", strings.ToUpper(srvName), lf.Name)
	titleWrapped := tui.WrapTextLines(boxTitle, innerW)
	if len(titleWrapped) == 0 {
		titleWrapped = []string{""}
	}

	availValW := innerW - core.DefaultKeyWidth - 8
	if availValW < 8 {
		availValW = innerW - 4
		if availValW < 8 {
			availValW = 8
		}
	}

	var metaRows []string
	appendWrappedMeta := func(label, value, valColor string) {
		valLines := wrapANSI(value, availValW)
		for vIdx, vL := range valLines {
			var rowStr string
			if vIdx == 0 {
				rowStr = tui.FormatKV(label, vL, valColor)
			} else {
				indent := strings.Repeat(" ", core.DefaultKeyWidth+6)
				rowStr = indent + valColor + vL + core.NC
			}
			metaRows = append(metaRows, " "+tui.PadOrTruncate(rowStr, innerW-2)+" ")
		}
	}

	appendWrappedMeta("Log Target Name", lf.Name, core.GREEN+core.BOLD)
	appendWrappedMeta("Log Target Path", targetDisplayPath, core.CYAN)

	errStatus := fmt.Sprintf("%d (Clean)", errCount)
	errColor := core.GREEN
	if errCount > 0 {
		errStatus = fmt.Sprintf("%d (Detected)", errCount)
		errColor = core.RED + core.BOLD
	}
	appendWrappedMeta("Logged Errors", errStatus, errColor)

	warnStatus := fmt.Sprintf("%d (Normal)", warnCount)
	warnColor := core.GREEN
	if warnCount > 0 {
		warnStatus = fmt.Sprintf("%d (Detected)", warnCount)
		warnColor = core.YELLOW + core.BOLD
	}
	appendWrappedMeta("Logged Warnings", warnStatus, warnColor)

	fixedLines := 7 + len(titleWrapped) + len(metaRows)
	availHeight := termH - fixedLines
	if availHeight < 2 {
		availHeight = 2
	}

	totalRows := len(allLogRows)
	isOverflow := totalRows > availHeight

	if !isOverflow {
		scrollOffset = 0
	} else {
		maxOffset := totalRows - (availHeight - 1)
		if maxOffset < 0 {
			maxOffset = 0
		}
		if scrollOffset < 0 || scrollOffset > maxOffset {
			scrollOffset = maxOffset
		}
	}

	showTopArrow := isOverflow && scrollOffset > 0
	visibleBudget := availHeight
	if showTopArrow {
		visibleBudget--
	}

	endRowIdx := scrollOffset + visibleBudget
	showBotArrow := isOverflow && endRowIdx < totalRows
	if showBotArrow {
		visibleBudget--
		endRowIdx = scrollOffset + visibleBudget
	}
	if endRowIdx > totalRows {
		endRowIdx = totalRows
	}

	var buf strings.Builder
	buf.WriteString("\033[?7l\033[H")
	buf.WriteString(fmt.Sprintf("%s%s%s%s%s\033[K\r\n", core.BLUE, core.Sym.BoxTL, border, core.Sym.BoxTR, core.NC))

	for _, tRow := range titleWrapped {
		if core.RuneWidth(tRow) > innerW {
			tRow = core.PadOrTruncateANSI(tRow, innerW)
		}
		tPadTotal := innerW - core.RuneWidth(tRow)
		if tPadTotal < 0 {
			tPadTotal = 0
		}
		tPadL := tPadTotal / 2
		tPadR := tPadTotal - tPadL
		buf.WriteString(core.BLUE + core.Sym.BoxV + strings.Repeat(" ", tPadL) + core.BOLD + core.BLUE + tRow + core.NC + strings.Repeat(" ", tPadR) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
	}
	buf.WriteString(core.BLUE + core.Sym.BoxML + border + core.Sym.BoxMR + core.NC + "\033[K\r\n")

	for _, mRow := range metaRows {
		buf.WriteString(core.BLUE + core.Sym.BoxV + core.NC + mRow + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
	}

	buf.WriteString(core.BLUE + core.Sym.BoxV + strings.Repeat(" ", innerW) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
	headerLabel := core.BOLD + core.YELLOW + "RECENT LOG ENTRIES (GRAPHICALLY FORMATTED):" + core.NC
	buf.WriteString(core.BLUE + core.Sym.BoxV + core.NC + " " + tui.PadOrTruncate(headerLabel, innerW-2) + " " + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")

	writeIndicator := func(msg string) {
		if core.RuneWidth(msg) > innerW {
			msg = core.PadOrTruncateANSI(msg, innerW)
		}
		pad := innerW - core.RuneWidth(msg)
		if pad < 0 {
			pad = 0
		}
		pL := pad / 2
		pR := pad - pL
		buf.WriteString(core.BLUE + core.Sym.BoxV + strings.Repeat(" ", pL) + "\033[38;5;214m" + msg + "\033[0m" + strings.Repeat(" ", pR) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
	}

	if showTopArrow {
		writeIndicator(core.Sym.ArrowUp + " [Up: scroll up for more lines]")
	}

	for rIdx := scrollOffset; rIdx < endRowIdx && rIdx < totalRows; rIdx++ {
		buf.WriteString(core.BLUE + core.Sym.BoxV + allLogRows[rIdx] + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
	}

	if showBotArrow {
		writeIndicator(core.Sym.ArrowDown + " [Down: scroll down for more lines]")
	}

	helpStr := "[R] Refresh   [T] Live Tail Stream (tail -f)   [Enter / Esc] Return"
	if isOverflow {
		helpStr = "[Up/Down] Scroll   [PgUp/PgDn] Page   [R] Refresh   [T] Tail   [Esc] Return"
	}
	if core.RuneWidth(helpStr) > innerW {
		if isOverflow {
			helpStr = "[Up/Down] Scroll  [R] Refresh  [T] Tail  [Esc] Return"
		} else {
			helpStr = "[R] Refresh  [T] Live Tail  [Enter/Esc] Return"
		}
		if core.RuneWidth(helpStr) > innerW {
			helpStr = core.PadOrTruncateANSI(helpStr, innerW)
		}
	}
	buf.WriteString(core.BLUE + core.Sym.BoxML + border + core.Sym.BoxMR + core.NC + "\033[K\r\n")
	hPadTotal := innerW - core.RuneWidth(helpStr)
	if hPadTotal < 0 {
		hPadTotal = 0
	}
	hPadL := hPadTotal / 2
	hPadR := hPadTotal - hPadL
	buf.WriteString(core.BLUE + core.Sym.BoxV + strings.Repeat(" ", hPadL) + "\033[2m" + core.YELLOW + helpStr + core.NC + "\033[0m" + strings.Repeat(" ", hPadR) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
	buf.WriteString(core.BLUE + core.Sym.BoxBL + border + core.Sym.BoxBR + core.NC + "\033[K\033[J\033[?7h")

	return buf.String(), scrollOffset, totalRows, availHeight
}

func fetchLogLines(lf LogFileTarget) ([]string, string) {
	var rawLogLines []string
	targetDisplayPath := lf.Path

	if lf.IsCmd {
		if strings.HasPrefix(lf.Path, "journalctl") {
			if out, err := exec.Command(getShell(), "-c", lf.Path+" -n 200 --no-pager 2>/dev/null").CombinedOutput(); err == nil && len(out) > 0 {
				rawLogLines = strings.Split(string(out), "\n")
			}
		} else {
			if out, err := exec.Command(getShell(), "-c", lf.Path+" 2>/dev/null").CombinedOutput(); err == nil && len(out) > 0 {
				rawLogLines = strings.Split(string(out), "\n")
			}
		}
		return rawLogLines, targetDisplayPath
	}

	targetPath := lf.Path
	if strings.Contains(targetPath, "|") {
		for _, alt := range strings.Split(targetPath, "|") {
			alt = strings.TrimSpace(alt)
			if _, err := os.Stat(alt); err == nil {
				targetPath = alt
				break
			}
		}
		if strings.Contains(targetPath, "|") {
			targetPath = strings.Split(targetPath, "|")[0]
		}
	} else if strings.Contains(targetPath, "*") {
		matches, _ := filepath.Glob(targetPath)
		if len(matches) > 0 {
			targetPath = matches[len(matches)-1]
		}
	}
	targetDisplayPath = targetPath
	if data, err := os.ReadFile(targetPath); err == nil {
		rawLogLines = strings.Split(string(data), "\n")
	}
	return rawLogLines, targetDisplayPath
}

// renderLogViewer renders a full-screen scrollable graphical log viewer for the given log target.
// Key bindings: [Up/Down/PgUp/PgDn] scroll, [R] refresh, [T] live tail stream, [Enter/Esc/q] exit viewer.
func renderLogViewer(srvName string, lf LogFileTarget) {
	tui.HideCursor()
	fmt.Print("\033[?7l")
	defer func() {
		fmt.Print("\033[?7h")
		tui.ShowCursor()
	}()

	rawLogLines, targetDisplayPath := fetchLogLines(lf)
	scrollOffset := -1

	for {
		frame, clampedOffset, _, availHeight := buildLogViewerFrame(
			srvName, lf, targetDisplayPath, rawLogLines,
			tui.GetTermWidth(), tui.GetTermHeight(), scrollOffset,
		)
		scrollOffset = clampedOffset
		fmt.Print(frame)

		key := tui.ReadKey()
		switch key {
		case "up", "k":
			if scrollOffset > 0 {
				scrollOffset--
			}
		case "down", "j":
			scrollOffset++
		case "pgup":
			scrollOffset -= availHeight
			if scrollOffset < 0 {
				scrollOffset = 0
			}
		case "pgdn", " ":
			scrollOffset += availHeight
		case "home", "g":
			scrollOffset = 0
		case "end", "G":
			scrollOffset = -1
		case "r", "R":
			rawLogLines, targetDisplayPath = fetchLogLines(lf)
			scrollOffset = -1
		case "t", "T":
			fmt.Print("\033[?7h")
			tui.ShowCursor()
			tui.LeaveAltScreen()
			tui.ClearScreen()
			fmt.Printf("%sStreaming %s (%s)... Press Ctrl+C to exit.%s\r\n\r\n", core.BOLD+core.GREEN, lf.Name, targetDisplayPath, core.NC)
			var streamCmd *exec.Cmd
			if lf.IsCmd {
				if strings.HasPrefix(lf.Path, "journalctl") {
					streamCmd = exec.Command(getShell(), "-c", lf.Path+" -f")
				} else {
					streamCmd = exec.Command(getShell(), "-c", lf.Path)
				}
			} else {
				streamCmd = exec.Command("tail", "-n", "50", "-f", targetDisplayPath)
			}
			streamCmd.Stdin = os.Stdin
			streamCmd.Stdout = os.Stdout
			streamCmd.Stderr = os.Stderr
			_ = streamCmd.Run()
			tui.EnterAltScreen()
			tui.HideCursor()
			fmt.Print("\033[?7l")
			rawLogLines, targetDisplayPath = fetchLogLines(lf)
			scrollOffset = -1
		case "enter", "esc", "q", "Q", "backspace":
			return
		}
	}
}

// ==============================================================================
// ENTRY POINT
// ==============================================================================

// main is the program entry point. It validates root privileges, parses CLI flags
// (-y/--yes for non-interactive mode, -h/--help for usage info), initialises the
// logger and system detection, then routes to the management or installer menu.
