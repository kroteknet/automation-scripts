// ==============================================================================
// KROTEK SCRIPT HUB — INTERACTIVE USER INPUT & DIALOG PROMPTS
// Service: CYBERPANEL (webpanel)
// File:    web-server/nginx/src/tui/input.go
//
// Implements interactive terminal text prompts, masked password inputs, and binary Yes/No confirmation dialogs with full validation.
// ==============================================================================

package tui

import (
	"fmt"
	"io"
	"krotek-cyberpanel-installer/core"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type CompletedInputItem struct {
	Label string
	Value string
}

type InputCardState struct {
	Active    bool
	Title     string
	Lines     []string
	HelpStr   string
	Completed []CompletedInputItem
}

var activeInputCard InputCardState

func SetInputCard(title string, lines []string, helpStr string) {
	cp := make([]string, len(lines))
	copy(cp, lines)
	activeInputCard = InputCardState{
		Active:    true,
		Title:     title,
		Lines:     cp,
		HelpStr:   helpStr,
		Completed: nil,
	}
}

func ResetInputCard() {
	activeInputCard = InputCardState{}
}

func EnableRawMode() {
	_ = exec.Command("stty", "-F", "/dev/tty", "cbreak", "-echo").Run()
}

func DisableRawMode() {
	_ = exec.Command("stty", "-F", "/dev/tty", "sane").Run()
}

func GetTTYReader() (*os.File, func()) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err == nil {
		return tty, func() { _ = tty.Close() }
	}
	return os.Stdin, func() {}
}

func ResolveConfigFile(candidates ...string) string {
	for _, cand := range candidates {
		if strings.Contains(cand, "*") {
			matches, err := filepath.Glob(cand)
			if err == nil && len(matches) > 0 {
				return matches[len(matches)-1]
			}
		} else {
			if _, err := os.Stat(cand); err == nil {
				return cand
			}
		}
	}
	if len(candidates) > 0 {
		first := candidates[0]
		dir := filepath.Dir(first)
		_ = os.MkdirAll(dir, 0755)
		return first
	}
	return ""
}

func RunEditor(filePath string) error {
	clean := filepath.Clean(strings.TrimSpace(filePath))
	dir := filepath.Dir(clean)
	_ = os.MkdirAll(dir, 0755)
	if _, err := os.Lstat(clean); os.IsNotExist(err) {
		if root, rErr := os.OpenRoot(dir); rErr == nil {
			if f, oErr := root.OpenFile(filepath.Base(clean), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644); oErr == nil {
				_, _ = f.Write([]byte("# Configuration File: " + clean + "\n"))
				_ = f.Close()
			}
			_ = root.Close()
		}
	}

	editor := getSafeEditorLocal()

	DisableRawMode()
	LeaveAltScreen()

	cmd := exec.Command(editor, filePath)
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err == nil {
		cmd.Stdin = tty
		cmd.Stdout = tty
		cmd.Stderr = tty
		defer tty.Close()
	} else {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	cmdErr := cmd.Run()
	EnterAltScreen()
	return cmdErr
}

func commandExistsLocal(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func getSafeEditorLocal() string {
	allowedEditors := map[string]bool{
		"nano":   true,
		"vim":    true,
		"vi":     true,
		"nvim":   true,
		"neovim": true,
		"micro":  true,
		"emacs":  true,
		"mcedit": true,
		"helix":  true,
		"joe":    true,
	}

	envEditor := strings.TrimSpace(os.Getenv("EDITOR"))
	if envEditor != "" {
		base := filepath.Base(envEditor)
		if allowedEditors[base] && !strings.ContainsAny(envEditor, ";|&$`\n\r\"' ") {
			if commandExistsLocal(envEditor) || commandExistsLocal(base) {
				return base
			}
		}
	}
	for _, ed := range []string{"nano", "vim", "vi"} {
		if commandExistsLocal(ed) {
			return ed
		}
	}
	return "nano"
}

func parseKeyBytes(b []byte, n int) string {
	if n == 0 {
		return ""
	}

	if b[0] == 27 {
		if n > 1 {
			s := string(b)
			if strings.Contains(s, "[A") || strings.Contains(s, "OA") {
				return "up"
			}
			if strings.Contains(s, "[B") || strings.Contains(s, "OB") {
				return "down"
			}
			if strings.Contains(s, "[C") || strings.Contains(s, "OC") {
				return "right"
			}
			if strings.Contains(s, "[D") || strings.Contains(s, "OD") {
				return "left"
			}
			if strings.Contains(s, "[3~") {
				return "delete"
			}
			if strings.Contains(s, "[5~") {
				return "pgup"
			}
			if strings.Contains(s, "[6~") {
				return "pgdn"
			}
			if strings.Contains(s, "[H") || strings.Contains(s, "[1~") || strings.Contains(s, "OH") {
				return "home"
			}
			if strings.Contains(s, "[F") || strings.Contains(s, "[4~") || strings.Contains(s, "OF") {
				return "end"
			}
			if strings.Contains(s, "[12~") || strings.Contains(s, "OQ") {
				return "f2"
			}
		}
		return "esc"
	}

	if b[0] == 1 {
		return "ctrl+a"
	}
	if b[0] == 3 {
		return "ctrl+c"
	}
	if b[0] == 4 {
		return "ctrl+d"
	}
	if b[0] == 5 {
		return "ctrl+e"
	}
	if b[0] == 21 {
		return "ctrl+u"
	}
	if b[0] == 23 {
		return "ctrl+w"
	}
	if b[0] == 9 {
		return "tab"
	}
	if b[0] == 10 || b[0] == 13 {
		return "enter"
	}
	if b[0] == 127 || b[0] == 8 {
		return "backspace"
	}

	r, _ := utf8.DecodeRune(b)
	if r != utf8.RuneError {
		return string(r)
	}

	return string(b[0])
}

func ReadKeyRaw() string {
	if core.AutoYes {
		return "enter"
	}
	rFile, cleanup := GetTTYReader()
	defer cleanup()

	buf := make([]byte, 64)
	n, err := rFile.Read(buf)
	if err != nil {
		if err == io.EOF {
			return "eof"
		}
		return ""
	}
	if n == 0 {
		return ""
	}

	return parseKeyBytes(buf[:n], n)
}

func ReadKeyRawWithTimeout(timeout time.Duration) string {
	if core.AutoYes {
		return "enter"
	}
	rFile, cleanup := GetTTYReader()
	defer cleanup()

	if timeout > 0 && !waitInputTimeout(int(rFile.Fd()), timeout) {
		return ""
	}

	buf := make([]byte, 64)
	n, err := rFile.Read(buf)
	if err != nil || n == 0 {
		return ""
	}

	return parseKeyBytes(buf[:n], n)
}

func ReadKey() string {
	if core.AutoYes {
		return "enter"
	}
	EnableRawMode()
	defer DisableRawMode()
	return ReadKeyRaw()
}

func getLiveInputStatus(promptLabel, currentVal, defaultVal string, isPassword, isDefaultUntouched bool, errMsg string) (string, string) {
	if errMsg != "" {
		return core.BOLD + core.RED + "[! " + errMsg + "]" + core.NC, core.RED
	}

	lowLabel := strings.ToLower(promptLabel)
	trimmed := strings.TrimSpace(currentVal)

	if isPassword {
		runeLen := len([]rune(currentVal))
		if runeLen == 0 {
			if strings.Contains(lowLabel, "blank") || strings.Contains(lowLabel, "random") || strings.Contains(lowLabel, "auto") {
				return core.DIM + "[Empty = Auto-gen]" + core.NC, core.CYAN
			}
			return core.YELLOW + "[Masked input]" + core.NC, core.CYAN
		}
		if runeLen < 8 {
			return fmt.Sprintf("%s[Length: %d]%s", core.YELLOW, runeLen, core.NC), core.YELLOW
		}
		return fmt.Sprintf("%s[OK: %d chars]%s", core.GREEN, runeLen, core.NC), core.GREEN
	}

	isPortField := strings.Contains(lowLabel, "port") && !strings.Contains(lowLabel, "url") && !strings.Contains(lowLabel, "portal") && !strings.Contains(lowLabel, "export") && !strings.Contains(lowLabel, "host") && !strings.Contains(lowLabel, "service") && !strings.Contains(lowLabel, "range") && !strings.Contains(lowLabel, "/tcp")
	if isPortField {
		if trimmed == "" && defaultVal != "" {
			return fmt.Sprintf("%s[Default: %s]%s", core.CYAN, defaultVal, core.NC), core.CYAN
		}
		if p, err := strconv.Atoi(trimmed); err == nil && p >= 1 && p <= 65535 {
			if isDefaultUntouched {
				return fmt.Sprintf("%s[Default: %d]%s", core.GREEN, p, core.NC), core.GREEN
			}
			return fmt.Sprintf("%s[Valid: %d]%s", core.GREEN, p, core.NC), core.GREEN
		}
		return core.BOLD + core.RED + "[! Invalid: 1-65535]" + core.NC, core.RED
	}

	if strings.Contains(lowLabel, "domain") || strings.Contains(lowLabel, "fqdn") {
		cleanDom := strings.TrimPrefix(strings.TrimPrefix(trimmed, "https://"), "http://")
		cleanDom = strings.TrimRight(cleanDom, "/")
		if cleanDom == "" {
			return core.YELLOW + "[e.g. example.com]" + core.NC, core.CYAN
		}
		if strings.Contains(cleanDom, ".") && len(cleanDom) >= 4 && !strings.ContainsAny(cleanDom, " 	") {
			return core.GREEN + "[Valid Domain]" + core.NC, core.GREEN
		}
		return core.YELLOW + "[Format: example.com]" + core.NC, core.YELLOW
	}

	if strings.Contains(lowLabel, "email") {
		if strings.Contains(trimmed, "@") && strings.Contains(trimmed, ".") && len(trimmed) >= 5 {
			return core.GREEN + "[Valid Email]" + core.NC, core.GREEN
		}
		return core.YELLOW + "[admin@example.com]" + core.NC, core.CYAN
	}

	if strings.Contains(lowLabel, "directory") || strings.Contains(lowLabel, "path") || strings.HasPrefix(defaultVal, "/") {
		if strings.HasPrefix(trimmed, "/") {
			return core.GREEN + "[Valid Path]" + core.NC, core.GREEN
		}
		if trimmed == "" && defaultVal != "" {
			return fmt.Sprintf("%s[Default: %s]%s", core.CYAN, defaultVal, core.NC), core.CYAN
		}
		return core.YELLOW + "[Must start with /]" + core.NC, core.YELLOW
	}

	if isDefaultUntouched && defaultVal != "" {
		return fmt.Sprintf("%s[Default: %s]%s", core.GREEN, defaultVal, core.NC), core.GREEN
	}
	if trimmed == "" && defaultVal != "" {
		return fmt.Sprintf("%s[Default: %s]%s", core.CYAN, defaultVal, core.NC), core.CYAN
	}
	if trimmed != "" {
		return core.GREEN + core.TagOK() + core.NC, core.GREEN
	}
	return core.DIM + "[Type value]" + core.NC, core.CYAN
}

func renderIntegratedInputBox(promptLabel, defaultVal string, input []rune, cursorPos int, isPassword, isDefaultUntouched bool, validationErr string) {
	innerW := GetTermWidth()
	termH := GetTermHeight()
	border := strings.Repeat(core.Sym.BoxH, innerW)

	cardTitle := activeInputCard.Title
	if cardTitle == "" {
		cardTitle = promptLabel
	}
	displayTitle := cardTitle
	stepReg := regexp.MustCompile(`^Step\s+([0-9]+/[0-9]+):\s*(.*)`)
	if matches := stepReg.FindStringSubmatch(cardTitle); len(matches) == 3 {
		displayTitle = fmt.Sprintf("%s%s[Step %s]%s  %s%s%s%s", core.BOLD, core.GREEN, matches[1], core.NC, core.BOLD, core.CYAN, matches[2], core.NC)
	}
	titleWrapped := WrapTextLines(displayTitle, innerW)

	var descLines []string
	descLines = append(descLines, activeInputCard.Lines...)
	if len(activeInputCard.Completed) > 0 {
		if len(descLines) > 0 && strings.TrimSpace(core.StripANSI(descLines[len(descLines)-1])) != "" {
			descLines = append(descLines, "")
		}
		for _, item := range activeInputCard.Completed {
			descLines = append(descLines, FormatKV(item.Label, item.Value+"  "+core.TagOK(), core.GREEN))
		}
	}

	var wrappedDesc []string
	if len(descLines) > 0 {
		wrappedDesc = append(wrappedDesc, safeRepeat(" ", innerW))
		for _, l := range descLines {
			cleanL := strings.TrimRight(l, " \t\r")
			if strings.TrimSpace(core.StripANSI(cleanL)) == "" {
				wrappedDesc = append(wrappedDesc, safeRepeat(" ", innerW))
				continue
			}
			descW := innerW - 6
			if descW < 4 {
				descW = 4
			}
			subLines := WrapBoxLine(cleanL, descW)
			for _, subL := range subLines {
				wrappedDesc = append(wrappedDesc, "   "+PadOrTruncate(subL, descW)+core.NC+"   ")
			}
		}
		wrappedDesc = append(wrappedDesc, safeRepeat(" ", innerW))
	}

	// Calculate available vertical space for description rows so input box never overflows terminal height
	// Overhead: top(1) + title(len) + sep(1) + desc_pad(2) + input_sep(1) + input_pad(2) + label(1) + input_box(3) + footer_sep(1) + footer(1) + bottom(1) = 14 + len(titleWrapped)
	fixedOverhead := 14 + len(titleWrapped)
	availDescRows := termH - fixedOverhead - 1
	if availDescRows < 1 {
		availDescRows = 1
	}
	if len(wrappedDesc) > availDescRows {
		wrappedDesc = wrappedDesc[len(wrappedDesc)-availDescRows:]
	}

	var buf strings.Builder
	buf.WriteString("\033[H")

	// Top border
	buf.WriteString(fmt.Sprintf("%s%s%s%s%s\033[K\r\n", core.BLUE, core.Sym.BoxTL, border, core.Sym.BoxTR, core.NC))

	// Title Section
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
		buf.WriteString(core.BLUE + core.Sym.BoxV + safeRepeat(" ", tPadL) + core.BOLD + core.BLUE + tRow + core.NC + safeRepeat(" ", tPadR) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
	}

	// Description Section
	if len(wrappedDesc) > 0 {
		buf.WriteString(core.BLUE + core.Sym.BoxML + border + core.Sym.BoxMR + core.NC + "\033[K\r\n")
		for _, dRow := range wrappedDesc {
			buf.WriteString(core.BLUE + core.Sym.BoxV + dRow + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
		}
	}

	// Input Zone Separator
	buf.WriteString(core.BLUE + core.Sym.BoxML + border + core.Sym.BoxMR + core.NC + "\033[K\r\n")

	// Top padding inside input section
	buf.WriteString(core.BLUE + core.Sym.BoxV + strings.Repeat(" ", innerW) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")

	statusBadge, fieldBorderColor := getLiveInputStatus(promptLabel, string(input), defaultVal, isPassword, isDefaultUntouched, validationErr)

	// Prompt Label (col 4) + Status Badge (col innerW-3)
	labelLeft := fmt.Sprintf("   %s%s%s %s%s%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, promptLabel, core.NC)
	leftW := core.RuneWidth(labelLeft)
	badgeW := core.RuneWidth(statusBadge)
	if leftW+badgeW+4 <= innerW {
		gap := innerW - leftW - badgeW - 3
		labelRow := labelLeft + strings.Repeat(" ", gap) + statusBadge + "   "
		buf.WriteString(core.BLUE + core.Sym.BoxV + PadOrTruncate(labelRow, innerW) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
	} else {
		buf.WriteString(core.BLUE + core.Sym.BoxV + PadOrTruncate(labelLeft, innerW) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
	}

	// Framed Input Field inside the card
	// Outer margin: 3 spaces left, 3 spaces right -> fieldInnerW = innerW - 8
	fieldInnerW := innerW - 8
	if fieldInnerW < 6 {
		fieldInnerW = 6
	}

	var displayRunes []rune
	if isPassword {
		displayRunes = []rune(strings.Repeat("*", len(input)))
	} else {
		displayRunes = input
	}

	if cursorPos < 0 {
		cursorPos = 0
	}
	if cursorPos > len(displayRunes) {
		cursorPos = len(displayRunes)
	}

	maxTextW := fieldInnerW - 2
	if maxTextW < 2 {
		maxTextW = 2
	}

	visStart := 0
	if cursorPos >= maxTextW {
		visStart = cursorPos - maxTextW + 1
	}

	var fieldContent string
	usedCols := 0

	if len(displayRunes) == 0 {
		cursorBlock := "\033[7m" + core.BOLD + core.CYAN + " " + "\033[27m" + core.NC
		placeholder := ""
		if defaultVal != "" && !isPassword {
			placeholder = defaultVal + "  (Enter for default)"
		} else if isPassword {
			placeholder = "Enter password..."
		} else {
			placeholder = "Type value here..."
		}
		phRunes := []rune(placeholder)
		if len(phRunes) > maxTextW-1 {
			phRunes = phRunes[:maxTextW-1]
		}
		fieldContent = cursorBlock + core.DIM + string(phRunes) + core.NC
		usedCols = 1 + len(phRunes)
	} else {
		var valBuilder strings.Builder
		textColor := core.BOLD + core.WHITE
		if isDefaultUntouched {
			textColor = core.BOLD + core.CYAN
		}

		for idx := visStart; idx <= len(displayRunes) && usedCols < maxTextW; idx++ {
			if idx == cursorPos {
				ch := " "
				if idx < len(displayRunes) {
					ch = string(displayRunes[idx])
				}
				cw := core.RuneWidth(ch)
				if cw < 1 {
					cw = 1
				}
				if usedCols+cw > maxTextW {
					break
				}
				valBuilder.WriteString("\033[7m" + core.BOLD + core.CYAN + ch + "\033[27m" + core.NC)
				usedCols += cw
			} else if idx < len(displayRunes) {
				ch := string(displayRunes[idx])
				cw := core.RuneWidth(ch)
				if cw < 1 {
					cw = 1
				}
				if usedCols+cw > maxTextW {
					break
				}
				valBuilder.WriteString(textColor + ch + core.NC)
				usedCols += cw
			}
		}

		// Subtle inline hint inside the box when default value is untouched and space allows
		remCols := maxTextW - usedCols
		if isDefaultUntouched && defaultVal != "" && remCols >= 25 {
			inlineHint := "   (Enter to accept)"
			if len(inlineHint) <= remCols {
				valBuilder.WriteString(core.DIM + inlineHint + core.NC)
				usedCols += len(inlineHint)
			}
		}
		fieldContent = valBuilder.String()
	}

	rightPad := maxTextW - usedCols
	if rightPad < 0 {
		rightPad = 0
	}

	topInputBorder := "   " + fieldBorderColor + core.Sym.BoxTL + strings.Repeat(core.Sym.BoxH, fieldInnerW) + core.Sym.BoxTR + core.NC + "   "
	midInputRow := "   " + fieldBorderColor + core.Sym.BoxV + core.NC + " " + fieldContent + strings.Repeat(" ", rightPad) + " " + fieldBorderColor + core.Sym.BoxV + core.NC + "   "
	if rowPad := innerW - core.RuneWidth(midInputRow); rowPad > 0 {
		midInputRow += strings.Repeat(" ", rowPad)
	}
	botInputBorder := "   " + fieldBorderColor + core.Sym.BoxBL + strings.Repeat(core.Sym.BoxH, fieldInnerW) + core.Sym.BoxBR + core.NC + "   "

	buf.WriteString(core.BLUE + core.Sym.BoxV + PadOrTruncate(topInputBorder, innerW) + core.NC + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
	buf.WriteString(core.BLUE + core.Sym.BoxV + PadOrTruncate(midInputRow, innerW) + core.NC + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
	buf.WriteString(core.BLUE + core.Sym.BoxV + PadOrTruncate(botInputBorder, innerW) + core.NC + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")

	// Bottom padding inside input section
	buf.WriteString(core.BLUE + core.Sym.BoxV + strings.Repeat(" ", innerW) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")

	// Footer Help Bar
	buf.WriteString(core.BLUE + core.Sym.BoxML + border + core.Sym.BoxMR + core.NC + "\033[K\r\n")

	escTarget := "Back / Cancel"
	if activeInputCard.HelpStr != "" {
		cleanH := core.StripANSI(activeInputCard.HelpStr)
		if parts := strings.Split(cleanH, "->"); len(parts) == 2 {
			target := strings.TrimSpace(parts[1])
			if target != "" {
				escTarget = target
			}
		}
	}

	var helpStr string
	if defaultVal != "" && !isPassword {
		helpStr = fmt.Sprintf("%s[Enter]%s Confirm   %s[Bksp]%s Edit   %s[Ctrl+U]%s Clear   %s[Tab]%s Default   %s[Esc]%s %s",
			core.BOLD+core.GREEN, core.NC,
			core.BOLD+core.YELLOW, core.NC,
			core.BOLD+core.CYAN, core.NC,
			core.BOLD+core.CYAN, core.NC,
			core.BOLD+core.RED, core.NC, escTarget)
	} else {
		helpStr = fmt.Sprintf("%s[Enter]%s Confirm   %s[Bksp]%s Edit   %s[Ctrl+U]%s Clear   %s[Esc]%s %s",
			core.BOLD+core.GREEN, core.NC,
			core.BOLD+core.YELLOW, core.NC,
			core.BOLD+core.CYAN, core.NC,
			core.BOLD+core.RED, core.NC, escTarget)
	}

	hPadTotal := innerW - core.RuneWidth(helpStr)
	if hPadTotal < 0 {
		helpStr = fmt.Sprintf("%s[Enter]%s Confirm   %s[Bksp]%s Edit   %s[Ctrl+U]%s Clear   %s[Esc]%s Back",
			core.BOLD+core.GREEN, core.NC,
			core.BOLD+core.YELLOW, core.NC,
			core.BOLD+core.CYAN, core.NC,
			core.BOLD+core.RED, core.NC)
		hPadTotal = innerW - core.RuneWidth(helpStr)
		if hPadTotal < 0 {
			helpStr = "[Enter] OK  [Bksp] Edit  [Esc] Back"
			hPadTotal = innerW - core.RuneWidth(helpStr)
			if hPadTotal < 0 {
				helpStr = core.PadOrTruncateANSI(helpStr, innerW)
				hPadTotal = 0
			}
		}
	}
	hPadL := hPadTotal / 2
	hPadR := hPadTotal - hPadL
	buf.WriteString(core.BLUE + core.Sym.BoxV + safeRepeat(" ", hPadL) + helpStr + core.NC + safeRepeat(" ", hPadR) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
	buf.WriteString(core.BLUE + core.Sym.BoxBL + border + core.Sym.BoxBR + core.NC + "\033[K\033[J")

	fmt.Print(buf.String())
}

func ReadTextInput(promptLabel string, defaultVal string) (string, int) {
	defer func() { boxNeedsHeader = false }()
	const maxInputLength = 256
	if core.AutoYes {
		return defaultVal, 0
	}
	if promptLabel == "" && defaultVal == "" {
		_ = ReadKey()
		return "", 0
	}

	if !activeInputCard.Active {
		var autoLines []string
		autoLines = append(autoLines, core.BOLD+core.CYAN+promptLabel+core.NC)
		autoLines = append(autoLines, "")
		if defaultVal != "" {
			autoLines = append(autoLines, "  Enter a custom value below or press Enter to accept the default configuration.")
			autoLines = append(autoLines, "  "+core.BOLD+"Default Value:"+core.NC+" "+core.CYAN+defaultVal+core.NC)
		} else {
			autoLines = append(autoLines, "  Enter the configuration value in the field below and press Enter to confirm.")
		}
		SetInputCard(promptLabel, autoLines, "[Esc]  ->  Back / Cancel")
	}

	HideCursor()
	fmt.Print("\033[?7l")
	EnableRawMode()
	defer func() {
		DisableRawMode()
		fmt.Print("\033[?7h")
		ShowCursor()
	}()

	input := []rune(defaultVal)
	cursorPos := len(input)
	isDefaultUntouched := defaultVal != ""
	validationErr := ""

	lowLabel := strings.ToLower(promptLabel)
	isPortField := strings.Contains(lowLabel, "port") && !strings.Contains(lowLabel, "url") && !strings.Contains(lowLabel, "portal") && !strings.Contains(lowLabel, "export") && !strings.Contains(lowLabel, "host") && !strings.Contains(lowLabel, "service") && !strings.Contains(lowLabel, "range") && !strings.Contains(lowLabel, "/tcp")
	isRequiredDomain := strings.Contains(lowLabel, "domain name") && !strings.Contains(lowLabel, "optional") && !strings.Contains(lowLabel, "blank")

	redraw := func() {
		renderIntegratedInputBox(promptLabel, defaultVal, input, cursorPos, false, isDefaultUntouched, validationErr)
	}

	redraw()

	for {
		k := ReadKeyRaw()
		if k == "" {
			continue
		}
		if k == "esc" || k == "ctrl+d" || k == "eof" {
			ResetInputCard()
			return "", 1
		}
		if k == "ctrl+c" {
			LeaveAltScreen()
			DisableRawMode()
			ShowCursor()
			os.Exit(0)
		}
		if k == "enter" {
			strVal := strings.TrimSpace(string(input))
			if strVal == "" && defaultVal != "" {
				strVal = defaultVal
			}
			if isPortField && strVal != "" {
				if p, err := strconv.Atoi(strVal); err != nil || p < 1 || p > 65535 {
					validationErr = "Invalid port! Enter a number between 1 and 65535"
					redraw()
					continue
				}
			}
			if isRequiredDomain {
				if strVal == "" {
					validationErr = "Domain name cannot be empty! Type a domain or press Esc to go back"
					redraw()
					continue
				}
				cleanDom := strings.TrimPrefix(strings.TrimPrefix(strVal, "https://"), "http://")
				cleanDom = strings.TrimRight(cleanDom, "/")
				if !strings.Contains(cleanDom, ".") || len(cleanDom) < 4 || strings.ContainsAny(cleanDom, " \t\r\n\"'`;$") {
					validationErr = "Invalid domain format! Enter a valid FQDN (e.g. example.com)"
					redraw()
					continue
				}
			}
			activeInputCard.Completed = append(activeInputCard.Completed, CompletedInputItem{
				Label: promptLabel,
				Value: strVal,
			})
			return strVal, 0
		}
		if k == "tab" {
			if defaultVal != "" {
				input = []rune(defaultVal)
				cursorPos = len(input)
				isDefaultUntouched = false
				validationErr = ""
				redraw()
			}
			continue
		}
		if k == "backspace" {
			isDefaultUntouched = false
			validationErr = ""
			if cursorPos > 0 {
				input = append(input[:cursorPos-1], input[cursorPos:]...)
				cursorPos--
				redraw()
			} else {
				redraw()
			}
			continue
		}
		if k == "delete" {
			isDefaultUntouched = false
			validationErr = ""
			if cursorPos < len(input) {
				input = append(input[:cursorPos], input[cursorPos+1:]...)
				redraw()
			}
			continue
		}
		if k == "left" {
			isDefaultUntouched = false
			if cursorPos > 0 {
				cursorPos--
			}
			redraw()
			continue
		}
		if k == "right" {
			isDefaultUntouched = false
			if cursorPos < len(input) {
				cursorPos++
			}
			redraw()
			continue
		}
		if k == "home" || k == "ctrl+a" {
			isDefaultUntouched = false
			cursorPos = 0
			redraw()
			continue
		}
		if k == "end" || k == "ctrl+e" {
			isDefaultUntouched = false
			cursorPos = len(input)
			redraw()
			continue
		}
		if k == "ctrl+u" {
			isDefaultUntouched = false
			validationErr = ""
			input = nil
			cursorPos = 0
			redraw()
			continue
		}
		if k == "ctrl+w" {
			isDefaultUntouched = false
			validationErr = ""
			if cursorPos > 0 {
				idx := cursorPos - 1
				for idx > 0 && input[idx] == ' ' {
					idx--
				}
				for idx > 0 && input[idx] != ' ' {
					idx--
				}
				if idx > 0 && input[idx] == ' ' {
					idx++
				}
				input = append(input[:idx], input[cursorPos:]...)
				cursorPos = idx
				redraw()
			}
			continue
		}
		runes := []rune(k)
		if len(runes) == 1 && runes[0] >= 32 {
			r := runes[0]
			validationErr = ""
			if isDefaultUntouched {
				// Typing a new character immediately replaces the pre-filled default value
				input = []rune{r}
				cursorPos = 1
				isDefaultUntouched = false
				redraw()
				continue
			}
			if len(input) >= maxInputLength {
				continue
			}
			if cursorPos == len(input) {
				input = append(input, r)
				cursorPos++
			} else {
				input = append(input[:cursorPos], append([]rune{r}, input[cursorPos:]...)...)
				cursorPos++
			}
			redraw()
		}
	}
}

func RunInteractiveCmd(cmd string, args ...string) {
	LeaveAltScreen()
	ClearScreen()
	c := exec.Command(cmd, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	_ = c.Run()
	EnterAltScreen()
}

// ReadPasswordInput prompts for masked password input without echoing plain characters to the terminal.
func ReadPasswordInput(promptLabel string) (string, int) {
	defer func() { boxNeedsHeader = false }()
	const maxInputLength = 256
	if core.AutoYes {
		if envPass := os.Getenv("KROTEK_ADMIN_PASS"); envPass != "" {
			return envPass, 0
		}
		return "", 1
	}

	if !activeInputCard.Active {
		autoLines := []string{
			core.BOLD + core.CYAN + promptLabel + core.NC,
			"",
			"  Enter the password in the masked field below and press Enter to confirm.",
		}
		SetInputCard(promptLabel, autoLines, "[Esc]  ->  Back / Cancel")
	}

	HideCursor()
	fmt.Print("\033[?7l")
	EnableRawMode()
	defer func() {
		DisableRawMode()
		fmt.Print("\033[?7h")
		ShowCursor()
	}()

	var input []rune
	cursorPos := 0

	redraw := func() {
		renderIntegratedInputBox(promptLabel, "", input, cursorPos, true, false, "")
	}

	redraw()

	for {
		k := ReadKeyRaw()
		if k == "" {
			continue
		}
		if k == "esc" || k == "ctrl+d" || k == "eof" {
			ResetInputCard()
			return "", 1
		}
		if k == "ctrl+c" {
			LeaveAltScreen()
			DisableRawMode()
			ShowCursor()
			os.Exit(0)
		}
		if k == "backspace" {
			if cursorPos > 0 {
				input = append(input[:cursorPos-1], input[cursorPos:]...)
				cursorPos--
				redraw()
			}
			continue
		}
		if k == "delete" {
			if cursorPos < len(input) {
				input = append(input[:cursorPos], input[cursorPos+1:]...)
				redraw()
			}
			continue
		}
		if k == "left" {
			if cursorPos > 0 {
				cursorPos--
				redraw()
			}
			continue
		}
		if k == "right" {
			if cursorPos < len(input) {
				cursorPos++
				redraw()
			}
			continue
		}
		if k == "ctrl+u" {
			input = nil
			cursorPos = 0
			redraw()
			continue
		}
		if k == "enter" {
			maskDisplay := "(auto-generated)"
			if len(input) > 0 {
				maskDisplay = strings.Repeat("*", len(input))
			}
			activeInputCard.Completed = append(activeInputCard.Completed, CompletedInputItem{
				Label: promptLabel,
				Value: maskDisplay,
			})
			return string(input), 0
		}
		runes := []rune(k)
		if len(runes) == 1 && runes[0] >= 32 {
			if len(input) >= maxInputLength {
				continue
			}
			r := runes[0]
			if cursorPos == len(input) {
				input = append(input, r)
				cursorPos++
			} else {
				input = append(input[:cursorPos], append([]rune{r}, input[cursorPos:]...)...)
				cursorPos++
			}
			redraw()
		}
	}
}
