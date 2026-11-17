// ==============================================================================
// KROTEK SCRIPT HUB — FULL-SCREEN INTERACTIVE MENU NAVIGATION SYSTEM
// Service: EXIM (email-server)
// File:    web-server/nginx/src/tui/menu.go
//
// Powers the zero-flicker TUI menu system with keyboard navigation (arrows, numbers, Vim keys), split-screen action inspector panels, and scroll management.
// ==============================================================================

package tui

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"krotek-exim-installer/core"
)

func SelectMenuWithContent(title string, content []string, options []string) (int, string, int) {
	defer func() { boxNeedsHeader = false }()
	origTitle := title
	isYesNo := len(options) == 2 &&
		(strings.HasPrefix(strings.ToUpper(core.StripANSI(options[0])), "YES") &&
			strings.HasPrefix(strings.ToUpper(core.StripANSI(options[1])), "NO"))

	var inheritedPrompt string
	didInheritCard := false
	savedCard := activeInputCard
	ResetInputCard()

	if len(content) == 0 && savedCard.Active {
		if !(strings.HasPrefix(title, "Step ") && savedCard.Title != title) {
			didInheritCard = true
			var inherited []string
			inherited = append(inherited, savedCard.Lines...)
			if len(savedCard.Completed) > 0 {
				if len(inherited) > 0 && strings.TrimSpace(core.StripANSI(inherited[len(inherited)-1])) != "" {
					inherited = append(inherited, "")
				}
				for _, item := range savedCard.Completed {
					inherited = append(inherited, FormatKV(item.Label, item.Value+"  [OK]", core.GREEN))
				}
			}
			content = inherited
			if savedCard.Title != "" && savedCard.Title != title {
				inheritedPrompt = core.BOLD + core.YELLOW + title + core.NC
				title = savedCard.Title
			}
		}
	}

	restoreInheritedCard := func(selIdx int) {
		if !didInheritCard || selIdx < 0 || selIdx >= len(options) {
			return
		}
		cleanLabel := strings.TrimSuffix(strings.TrimSpace(core.StripANSI(origTitle)), "?")
		if strings.Contains(strings.ToLower(cleanLabel), "let's encrypt") && strings.Contains(strings.ToLower(cleanLabel), "ssl") {
			cleanLabel = "Let's Encrypt SSL"
		}
		cleanVal := strings.TrimSpace(core.StripANSI(options[selIdx]))
		if isYesNo {
			if selIdx == 0 {
				cleanVal = "YES"
			} else {
				cleanVal = "NO"
			}
		} else if parenIdx := strings.Index(cleanVal, " ("); parenIdx > 0 {
			cleanVal = strings.TrimSpace(cleanVal[:parenIdx])
		} else if dashIdx := strings.Index(cleanVal, " — "); dashIdx > 0 {
			cleanVal = strings.TrimSpace(cleanVal[:dashIdx])
		} else if dashIdx := strings.Index(cleanVal, " - "); dashIdx > 0 {
			cleanVal = strings.TrimSpace(cleanVal[:dashIdx])
		}
		savedCard.Completed = append(savedCard.Completed, CompletedInputItem{
			Label: cleanLabel,
			Value: cleanVal,
		})
		activeInputCard = savedCard
	}

	if core.AutoYes {
		for i, opt := range options {
			clean := strings.TrimSpace(core.StripANSI(opt))
			if strings.HasPrefix(clean, "---") || strings.HasPrefix(clean, "───") || strings.HasPrefix(clean, "• ") || clean == "" {
				continue
			}
			restoreInheritedCard(i)
			return i, opt, 0
		}
		if len(options) > 0 {
			restoreInheritedCard(0)
			return 0, options[0], 0
		}
		return 0, "", 0
	}

	HideCursor()
	fmt.Print("\033[?7l") // Disable auto-wrap at margin to prevent terminal scroll
	EnableRawMode()
	defer func() {
		DisableRawMode()
		fmt.Print("\033[?7h") // Re-enable auto-wrap
		ShowCursor()
	}()

	sel := 0
	n := len(options)

	for sel < n && (strings.HasPrefix(options[sel], "---") || strings.HasPrefix(options[sel], "───")) {
		sel++
	}

	validCount := 0
	for _, opt := range options {
		if !strings.HasPrefix(opt, "---") && !strings.HasPrefix(opt, "───") {
			validCount++
		}
	}

	keyRange := ""
	if validCount == 1 {
		keyRange = "[1]"
	} else if validCount > 1 {
		keyRange = fmt.Sprintf("[1-%d]", validCount)
	}

	scrollOffset := 0

	var displayContent []string
	var promptQuestion string

	if inheritedPrompt != "" {
		displayContent = content
		promptQuestion = inheritedPrompt
	} else if len(content) > 0 {
		for i, l := range content {
			cleanL := core.StripANSI(l)
			if strings.Contains(l, core.YELLOW) && (strings.Contains(cleanL, "?") || strings.Contains(strings.ToLower(cleanL), "proceed")) {
				promptQuestion = l
				prevLines := content[:i]
				if len(prevLines) > 0 && strings.TrimSpace(core.StripANSI(prevLines[len(prevLines)-1])) == "" {
					prevLines = prevLines[:len(prevLines)-1]
				}
				displayContent = prevLines
				break
			}
		}
		if promptQuestion == "" {
			displayContent = content
		}
	}

	focusedPanel := 0 // 0 = Left Panel (Menu), 1 = Right Panel (Inspector)
	rightScrollOffset := 0
	maxRightScroll := 0

	var cachedRightLines []string
	var lastTelemetryOpt string
	var lastTelemetryW int
	var lastTelemetryH int

	// -------------------------------------------------------------
	// Render Expanded 2-Column Dashboard (Wide Screen Server / SSH)
	// -------------------------------------------------------------
	drawExpanded := func() {
		innerW := GetTermWidth()
		termH := GetTermHeight()

		// Divide Left (Menu) and Right (Inspector) exactly 50/50
		leftW := innerW / 2
		rightW := innerW - 1 - leftW

		displayTitle := title
		stepReg := regexp.MustCompile(`^Step\s+([0-9]+/[0-9]+):\s*(.*)`)
		if matches := stepReg.FindStringSubmatch(title); len(matches) == 3 {
			displayTitle = fmt.Sprintf("%s%s[Step %s]%s  %s%s%s%s", core.BOLD, core.GREEN, matches[1], core.NC, core.BOLD, core.CYAN, matches[2], core.NC)
		}
		titleWrapped := WrapTextLines(displayTitle, innerW)

		// Top Audit Grid (Excel-style table replaces banner in expanded view)
		var topTableLines []string
		if !isYesNo && !strings.HasPrefix(title, "Step ") && !strings.HasPrefix(title, "Confirm") && !strings.HasPrefix(title, "Filter") {
			topTableLines = RenderExpandedTopExcelTable(innerW, leftW, rightW, termH)
		}

		sepOverhead := 0
		if len(topTableLines) == 0 {
			sepOverhead = 1
		}
		fixedOverhead := 6 + len(titleWrapped) + len(topTableLines) + sepOverhead

		availLines := termH - fixedOverhead - 1
		if availLines < 4 {
			availLines = 4
		}

		type OptionRow struct {
			OptIdx   int
			IsHeader bool
			LineText string
		}

		var allOptionRows []OptionRow
		optNum := 0

		for i := 0; i < n; i++ {
			opt := options[i]

			if strings.HasPrefix(opt, "---") || strings.HasPrefix(opt, "───") {
				cleanCategory := strings.Trim(opt, "-─ ")
				catText := " " + core.BOLD + core.YELLOW + core.Sym.Section + " " + cleanCategory + core.NC
				allOptionRows = append(allOptionRows, OptionRow{
					OptIdx:   i,
					IsHeader: true,
					LineText: catText,
				})
				continue
			}

			optNum++
			numPrefix := ""
			numPartW := 0
			cleanCheck := core.StripANSI(opt)

			if !regexp.MustCompile(`^\=[0-9]+=`).MatchString(cleanCheck) && !regexp.MustCompile(`^\[[0-9]+\]`).MatchString(cleanCheck) {
				if validCount >= 10 {
					numPrefix = fmt.Sprintf("[%02d]", optNum)
				} else {
					numPrefix = fmt.Sprintf("[%d]", optNum)
				}
				numPartW = len(numPrefix) + 1
			}

			maxOptW := leftW - 5 - numPartW
			if maxOptW < 10 {
				maxOptW = 10
			}

			optLower := strings.ToLower(cleanCheck)
			badge := ""
			switch {
			case strings.Contains(optLower, "uninstall") || strings.Contains(optLower, "purge") || strings.Contains(optLower, "remove"):
				badge = core.BOLD + core.RED + " [CAUTION]" + core.NC
			case strings.Contains(optLower, "reload"):
				badge = core.BOLD + core.GREEN + " [RELOAD]" + core.NC
			case strings.Contains(optLower, "restart") || (strings.Contains(optLower, "start") && !strings.Contains(optLower, "wizard")):
				badge = core.BOLD + core.YELLOW + " [SERVICE]" + core.NC
			case strings.Contains(optLower, "backup") || strings.Contains(optLower, "snapshot"):
				badge = core.BOLD + core.GREEN + " [BACKUP]" + core.NC
			case strings.Contains(optLower, "vhost") || strings.Contains(optLower, "domain") || strings.Contains(optLower, "config") || strings.Contains(optLower, "compose") || strings.Contains(optLower, "caddyfile"):
				badge = core.BOLD + core.PURPLE + " [CONF]" + core.NC
			case strings.Contains(optLower, "log") || strings.Contains(optLower, "journal"):
				badge = core.BOLD + core.YELLOW + " [LOGS]" + core.NC
			case strings.Contains(optLower, "syntax") || strings.Contains(optLower, "test") || strings.Contains(optLower, "check"):
				badge = core.BOLD + core.CYAN + " [TEST]" + core.NC
			case strings.Contains(optLower, "diagnostic") || strings.Contains(optLower, "doctor"):
				badge = core.BOLD + core.CYAN + " [DIAG]" + core.NC
			case strings.Contains(optLower, "dashboard") || strings.Contains(optLower, "ip info") || strings.Contains(optLower, "overview"):
				badge = core.BOLD + core.CYAN + " [INFO]" + core.NC
			case strings.Contains(optLower, "wizard") || strings.Contains(optLower, "setup"):
				badge = core.BOLD + core.PURPLE + " [SETUP]" + core.NC
			}

			optWrapped := WrapTextLines(opt, maxOptW)
			badgeIdx := -1
			if badge != "" && len(optWrapped) > 0 {
				badgeW := core.RuneWidth(badge)
				lastIdx := len(optWrapped) - 1
				if core.RuneWidth(optWrapped[lastIdx])+badgeW <= maxOptW {
					badgeIdx = lastIdx
				} else {
					optWrapped = append(optWrapped, strings.TrimLeft(badge, " "))
					badge = ""
				}
			}

			optColor := core.NC
			if i == sel {
				upperCheck := strings.ToUpper(cleanCheck)
				if upperCheck == "NO" || strings.Contains(upperCheck, "CANCEL") || strings.Contains(upperCheck, "ABORT") {
					optColor = core.BOLD + core.RED
				} else {
					optColor = core.BOLD + core.GREEN
				}
			}

			for owIdx, curOw := range optWrapped {
				rowBadge := ""
				if owIdx == badgeIdx {
					rowBadge = badge
				}

				var rowLine string
				if owIdx == 0 {
					if numPrefix != "" {
						if i == sel {
							rowLine = " " + core.GREEN + core.Sym.Pointer + " " + core.BOLD + core.GREEN + numPrefix + core.NC + " " + optColor + curOw + rowBadge + core.NC
						} else {
							rowLine = "   " + core.CYAN + numPrefix + core.NC + " " + curOw + rowBadge
						}
					} else {
						if i == sel {
							rowLine = " " + core.GREEN + core.Sym.Pointer + " " + optColor + curOw + rowBadge + core.NC
						} else {
							rowLine = "   " + curOw + rowBadge
						}
					}
				} else {
					subIndentPad := safeRepeat(" ", numPartW)
					rowLine = "   " + subIndentPad + optColor + curOw + rowBadge + core.NC
				}

				allOptionRows = append(allOptionRows, OptionRow{
					OptIdx:   i,
					IsHeader: false,
					LineText: rowLine,
				})
			}
		}

		totalRows := len(allOptionRows)
		isScrolling := totalRows > availLines

		selFirstRowIdx := -1
		selLastRowIdx := -1
		for rIdx, r := range allOptionRows {
			if r.OptIdx == sel {
				if selFirstRowIdx == -1 {
					selFirstRowIdx = rIdx
				}
				selLastRowIdx = rIdx
			}
		}
		if selFirstRowIdx == -1 {
			selFirstRowIdx = 0
			selLastRowIdx = 0
		}

		focusStartIdx := selFirstRowIdx
		if selFirstRowIdx > 0 && allOptionRows[selFirstRowIdx-1].IsHeader {
			focusStartIdx = selFirstRowIdx - 1
		}
		focusEndIdx := selLastRowIdx

		if isScrolling {
			if focusStartIdx < scrollOffset {
				scrollOffset = focusStartIdx
			}
			for scrollOffset < totalRows-1 && focusEndIdx >= scrollOffset+availLines {
				scrollOffset++
			}
			if scrollOffset < 0 {
				scrollOffset = 0
			}
			if scrollOffset > totalRows-1 {
				scrollOffset = totalRows - 1
			}
		} else {
			scrollOffset = 0
		}

		endRowIdx := scrollOffset + availLines
		if endRowIdx > totalRows {
			endRowIdx = totalRows
		}

		var leftLines []string
		for rIdx := scrollOffset; rIdx < endRowIdx; rIdx++ {
			leftLines = append(leftLines, allOptionRows[rIdx].LineText)
		}

		// Right Pane Telemetry & Context
		selectedOptText := ""
		if sel >= 0 && sel < len(options) {
			selectedOptText = options[sel]
		}

		maxContentRows := availLines
		if maxContentRows < 4 {
			maxContentRows = 4
		}

		var rightLines []string
		if len(cachedRightLines) > 0 && lastTelemetryOpt == selectedOptText && lastTelemetryW == rightW && lastTelemetryH == maxContentRows {
			rightLines = cachedRightLines
		} else {
			rightLines = GetServerTelemetry(rightW, selectedOptText, maxContentRows)
			if len(displayContent) > 0 {
				rightLines = append(rightLines, "")
				rightLines = append(rightLines, " "+core.BOLD+core.YELLOW+core.Sym.Section+" INSTANCE CONFIGURATION & DETAILS"+core.NC)
				rightLines = append(rightLines, " "+core.BLUE+safeRepeat(core.Sym.BoxH, rightW-4)+core.NC)
				for _, cl := range displayContent {
					rightLines = append(rightLines, WrapBoxLine(cl, rightW-4)...)
				}
			}
			cachedRightLines = rightLines
			lastTelemetryOpt = selectedOptText
			lastTelemetryW = rightW
			lastTelemetryH = maxContentRows
		}

		var buf strings.Builder
		buf.WriteString("\033[H")

		borderFull := strings.Repeat(core.Sym.BoxH, innerW)
		borderL := strings.Repeat(core.Sym.BoxH, leftW)
		borderR := strings.Repeat(core.Sym.BoxH, rightW)

		// Top border (Square)
		buf.WriteString(core.BLUE + core.Sym.BoxTL + borderFull + core.Sym.BoxTR + core.NC + "\033[K\r\n")

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

		// Excel Top Audit Table (Replaces banner in expanded view)
		if len(topTableLines) > 0 {
			for _, tblLine := range topTableLines {
				buf.WriteString(tblLine + "\033[K\r\n")
			}
		} else {
			buf.WriteString(core.BLUE + core.Sym.BoxML + borderL + core.Sym.BoxTM + borderR + core.Sym.BoxMR + core.NC + "\033[K\r\n")
		}

		// Right lines scrolling calculations
		totalRightLines := len(rightLines)
		maxRightScroll = totalRightLines - maxContentRows
		if maxRightScroll < 0 {
			maxRightScroll = 0
		}
		if rightScrollOffset > maxRightScroll {
			rightScrollOffset = maxRightScroll
		}
		if rightScrollOffset < 0 {
			rightScrollOffset = 0
		}

		// Column Title Bars with active filled background
		var leftHeader, rightHeader string
		if focusedPanel == 0 {
			// Left Panel is ACTIVE: solid slate-blue background
			bgActive := "\033[48;5;25m\033[38;5;231m\033[1m"
			titleL := fmt.Sprintf(" [%s ACTIVE] MENU ACTIONS & OPTIONS", core.Sym.ActiveDot)
			leftHeader = bgActive + PadOrTruncate(titleL, leftW) + core.NC

			// Right Panel is INACTIVE: dark charcoal background
			bgMuted := "\033[48;5;236m\033[38;5;246m"
			titleR := fmt.Sprintf(" [%s RIGHT TO INSPECT] ACTION INSPECTOR & CHEATSHEET", core.Sym.ArrowRight)
			if maxRightScroll > 0 {
				titleR = fmt.Sprintf(" [%s RIGHT TO SCROLL (%d lines)] INSPECTOR", core.Sym.ArrowRight, totalRightLines)
			}
			rightHeader = bgMuted + PadOrTruncate(titleR, rightW) + core.NC
		} else {
			// Left Panel is INACTIVE: dark charcoal background
			bgMuted := "\033[48;5;236m\033[38;5;246m"
			titleL := fmt.Sprintf(" [%s MENU] ACTIONS & OPTIONS", core.Sym.ArrowLeft)
			leftHeader = bgMuted + PadOrTruncate(titleL, leftW) + core.NC

			// Right Panel is ACTIVE: solid cyan/teal emerald background
			bgActive := "\033[48;5;30m\033[38;5;231m\033[1m"
			scrollPct := 0
			if maxRightScroll > 0 {
				scrollPct = (rightScrollOffset * 100) / maxRightScroll
			}
			titleR := fmt.Sprintf(" [%s ACTIVE: SCROLL MODE %d%%] ACTION INSPECTOR & CHEATSHEET", core.Sym.ActiveDot, scrollPct)
			rightHeader = bgActive + PadOrTruncate(titleR, rightW) + core.NC
		}

		buf.WriteString(core.BLUE + core.Sym.BoxV + leftHeader + core.BLUE + core.Sym.BoxV + rightHeader + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
		buf.WriteString(core.BLUE + core.Sym.BoxML + borderL + core.Sym.BoxCross + borderR + core.Sym.BoxMR + core.NC + "\033[K\r\n")

		// Render rows with right-side scroll offset
		for row := 0; row < maxContentRows; row++ {
			var lStr, rStr string
			if row < len(leftLines) {
				lStr = leftLines[row]
			}
			rIdx := rightScrollOffset + row
			if rIdx < totalRightLines {
				rStr = rightLines[rIdx]
			}

			buf.WriteString(core.BLUE + core.Sym.BoxV + core.NC + PadOrTruncate(lStr, leftW) + core.NC + core.BLUE + core.Sym.BoxV + core.NC + PadOrTruncate(rStr, rightW) + core.NC + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
		}

		// Bottom Split Separator
		buf.WriteString(core.BLUE + core.Sym.BoxML + borderL + core.Sym.BoxBM + borderR + core.Sym.BoxMR + core.NC + "\033[K\r\n")

		// Bottom Action Bar in Footer
		var helpStr string
		if focusedPanel == 0 {
			helpStr = fmt.Sprintf("%s[Up/Down]%s Move Menu   %s[Enter]%s Select   %s[Right]%s Focus Inspector   %s[Tab]%s Compact   %s[Esc]%s Exit",
				core.BOLD+core.YELLOW, core.NC,
				core.BOLD+core.YELLOW, core.NC,
				core.BOLD+core.CYAN, core.NC,
				core.BOLD+core.GREEN, core.NC,
				core.BOLD+core.RED, core.NC)
		} else {
			helpStr = fmt.Sprintf("%s[Up/Down]%s Scroll Inspector (%d/%d)   %s[PgUp/PgDn]%s Page   %s[Left]%s Focus Menu   %s[Enter]%s Select   %s[Esc]%s Exit",
				core.BOLD+core.GREEN, core.NC,
				rightScrollOffset+1, totalRightLines,
				core.BOLD+core.CYAN, core.NC,
				core.BOLD+core.YELLOW, core.NC,
				core.BOLD+core.YELLOW, core.NC,
				core.BOLD+core.RED, core.NC)
		}

		hPadTotal := innerW - core.RuneWidth(helpStr)
		if hPadTotal < 0 {
			if focusedPanel == 0 {
				helpStr = fmt.Sprintf("%s[Up/Down]%s Move  %s[Enter]%s OK  %s[Right]%s Inspector  %s[Tab]%s Compact  %s[Esc]%s Exit",
					core.BOLD+core.YELLOW, core.NC,
					core.BOLD+core.YELLOW, core.NC,
					core.BOLD+core.CYAN, core.NC,
					core.BOLD+core.GREEN, core.NC,
					core.BOLD+core.RED, core.NC)
			} else {
				helpStr = fmt.Sprintf("%s[Up/Down]%s Scroll (%d/%d)  %s[Left]%s Menu  %s[Enter]%s OK  %s[Esc]%s Exit",
					core.BOLD+core.GREEN, core.NC,
					rightScrollOffset+1, totalRightLines,
					core.BOLD+core.YELLOW, core.NC,
					core.BOLD+core.YELLOW, core.NC,
					core.BOLD+core.RED, core.NC)
			}
			hPadTotal = innerW - core.RuneWidth(helpStr)
			if hPadTotal < 0 {
				helpStr = core.PadOrTruncateANSI(helpStr, innerW)
				hPadTotal = 0
			}
		}

		hPadL := hPadTotal / 2
		hPadR := hPadTotal - hPadL
		buf.WriteString(core.BLUE + core.Sym.BoxV + safeRepeat(" ", hPadL) + helpStr + core.NC + safeRepeat(" ", hPadR) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")

		// Bottom Border (Square)
		buf.WriteString(core.BLUE + core.Sym.BoxBL + borderFull + core.Sym.BoxBR + core.NC + "\033[K\033[J")

		fmt.Print(buf.String())
	}

	// -------------------------------------------------------------
	// Render Classic Unified 1-Column Menu (GUI Desktop / Standard)
	// -------------------------------------------------------------
	drawCompact := func() {
		innerW := GetTermWidth()
		border := strings.Repeat(core.Sym.BoxH, innerW)
		termH := GetTermHeight()

		displayTitle := title
		stepReg := regexp.MustCompile(`^Step\s+([0-9]+/[0-9]+):\s*(.*)`)
		if matches := stepReg.FindStringSubmatch(title); len(matches) == 3 {
			displayTitle = fmt.Sprintf("%s%s[Step %s]%s  %s%s%s%s", core.BOLD, core.GREEN, matches[1], core.NC, core.BOLD, core.CYAN, matches[2], core.NC)
		}
		titleWrapped := WrapTextLines(displayTitle, innerW)

		isWizardOrModal := isYesNo || didInheritCard || strings.HasPrefix(title, "Step ") || strings.HasPrefix(title, "Confirm") || strings.HasPrefix(title, "Filter") || strings.Contains(title, "Conflict") || strings.Contains(title, "Summary") || strings.Contains(title, "Notice") || strings.Contains(title, "Strategy") || strings.Contains(title, "Template")

		var descLines []string
		if !isYesNo {
			selectedOptText := ""
			if sel >= 0 && sel < len(options) {
				selectedOptText = options[sel]
			}
			desc := getActionDescription(selectedOptText)
			maxDescW := innerW - 5
			if maxDescW < 15 {
				maxDescW = 15
			}
			wrapped := WrapTextLines(desc, maxDescW)
			for dIdx, dText := range wrapped {
				cleanText := strings.TrimSpace(core.StripANSI(dText))
				if cleanText == "" {
					continue
				}
				if dIdx == 0 {
					descLines = append(descLines, fmt.Sprintf(" %s>%s %s%s%s", core.CYAN, core.NC, core.DIM, cleanText, core.NC))
				} else {
					descLines = append(descLines, fmt.Sprintf("   %s%s%s", core.DIM, cleanText, core.NC))
				}
			}
			if len(descLines) == 0 {
				descLines = append(descLines, fmt.Sprintf(" %s>%s %s%s%s", core.CYAN, core.NC, core.DIM, desc, core.NC))
			}
		}

		var bannerLines []string
		if core.BannerProvider != nil && !isWizardOrModal {
			bannerW := innerW - 2
			if bannerW < 10 {
				bannerW = innerW
			}
			bannerLines = core.BannerProvider(title, bannerW)
		}

		var promptWrapped []string
		if promptQuestion != "" {
			promptWrapped = WrapTextLines(promptQuestion, innerW-4)
		}

		baseFixedLines := 1 + len(titleWrapped) + 1 + 1 + 1 + 1
		if !isYesNo {
			baseFixedLines += 1 + len(descLines)
		}
		if len(displayContent) > 0 {
			baseFixedLines += 1
			for _, l := range displayContent {
				baseFixedLines += len(WrapBoxLine(l, innerW-2))
			}
		}
		if len(promptWrapped) > 0 {
			baseFixedLines += len(promptWrapped) + 1
		}

		var activeBanner []string
		if len(bannerLines) > 0 {
			bannerLineCount := len(bannerLines)
			if termH-(baseFixedLines+1+bannerLineCount) >= 4 {
				activeBanner = bannerLines
			}
		}

		fixedLines := 1 + len(titleWrapped)
		if len(activeBanner) > 0 {
			fixedLines += 1 + len(activeBanner)
		}

		if len(displayContent) > 0 {
			fixedLines += 1
			for _, l := range displayContent {
				fixedLines += len(WrapBoxLine(l, innerW-2))
			}
		}

		if len(promptWrapped) > 0 {
			fixedLines += len(promptWrapped) + 1
		}

		if !isYesNo {
			fixedLines += 5 + len(descLines)
		} else {
			fixedLines += 4
		}
		availOptLines := termH - fixedLines - 1
		if availOptLines < 1 {
			availOptLines = 1
		}

		type OptionRow struct {
			OptIdx   int
			IsHeader bool
			LineText string
		}

		var allRows []OptionRow
		optNum := 0

		for i := 0; i < n; i++ {
			opt := options[i]

			if strings.HasPrefix(opt, "---") || strings.HasPrefix(opt, "───") {
				cleanCategory := strings.Trim(opt, "-─ ")
				catText := core.BOLD + core.YELLOW + core.Sym.Section + " " + cleanCategory + core.NC
				allRows = append(allRows, OptionRow{
					OptIdx:   i,
					IsHeader: true,
					LineText: catText,
				})
				continue
			}

			optNum++
			numPrefix := ""
			numPartW := 0
			cleanCheck := core.StripANSI(opt)

			if !regexp.MustCompile(`^\=[0-9]+=`).MatchString(cleanCheck) && !regexp.MustCompile(`^\[[0-9]+\]`).MatchString(cleanCheck) {
				if validCount >= 10 {
					numPrefix = fmt.Sprintf("[%02d]", optNum)
				} else {
					numPrefix = fmt.Sprintf("[%d]", optNum)
				}
				numPartW = len(numPrefix) + 2
			}

			maxOptW := innerW - 6 - numPartW
			if maxOptW < 10 {
				maxOptW = 10
			}

			optWrapped := WrapTextLines(opt, maxOptW)
			optColor := core.NC
			if i == sel {
				upperCheck := strings.ToUpper(cleanCheck)
				if upperCheck == "NO" || strings.Contains(upperCheck, "CANCEL") || strings.Contains(upperCheck, "ABORT") {
					optColor = core.BOLD + core.RED
				} else {
					optColor = core.BOLD + core.GREEN
				}
			}
			for owIdx, curOw := range optWrapped {

				var rowLine string
				if owIdx == 0 {
					if numPrefix != "" {
						if i == sel {
							rowLine = "  " + core.GREEN + core.Sym.Pointer + " " + core.BOLD + core.GREEN + numPrefix + core.NC + "  " + optColor + curOw + core.NC
						} else {
							rowLine = "    " + core.CYAN + numPrefix + core.NC + "  " + curOw
						}
					} else {
						if i == sel {
							rowLine = "  " + core.GREEN + core.Sym.Pointer + " " + optColor + curOw + core.NC
						} else {
							rowLine = "    " + curOw
						}
					}
				} else {
					subIndentPad := ""
					if numPartW > 0 {
						subIndentPad = strings.Repeat(" ", numPartW)
					}
					rowLine = "    " + subIndentPad + optColor + curOw + core.NC
				}

				allRows = append(allRows, OptionRow{
					OptIdx:   i,
					IsHeader: false,
					LineText: rowLine,
				})
			}
		}

		totalRows := len(allRows)
		isScrolling := totalRows > availOptLines

		selFirstRowIdx := -1
		selLastRowIdx := -1
		for rIdx, r := range allRows {
			if r.OptIdx == sel {
				if selFirstRowIdx == -1 {
					selFirstRowIdx = rIdx
				}
				selLastRowIdx = rIdx
			}
		}

		if selFirstRowIdx == -1 {
			selFirstRowIdx = 0
			selLastRowIdx = 0
		}

		focusStartIdx := selFirstRowIdx
		if selFirstRowIdx > 0 && allRows[selFirstRowIdx-1].IsHeader {
			focusStartIdx = selFirstRowIdx - 1
		}
		focusEndIdx := selLastRowIdx

		if isScrolling {
			calcCapacity := func(off int) int {
				cap := availOptLines
				if off > 0 {
					cap--
				}
				if off+cap < totalRows {
					cap--
				}
				if cap < 1 {
					cap = 1
				}
				return cap
			}

			if focusStartIdx < scrollOffset {
				scrollOffset = focusStartIdx
			}
			for scrollOffset < totalRows-1 {
				cap := calcCapacity(scrollOffset)
				if focusEndIdx >= scrollOffset+cap {
					scrollOffset++
				} else {
					break
				}
			}

			if scrollOffset < 0 {
				scrollOffset = 0
			}
			if scrollOffset > totalRows-1 {
				scrollOffset = totalRows - 1
			}
		} else {
			scrollOffset = 0
		}

		showTopArrow := isScrolling && scrollOffset > 0
		visibleRowsBudget := availOptLines
		if showTopArrow {
			visibleRowsBudget--
		}

		endRowIdx := scrollOffset + visibleRowsBudget
		showBotArrow := isScrolling && endRowIdx < totalRows
		if showBotArrow {
			visibleRowsBudget--
			endRowIdx = scrollOffset + visibleRowsBudget
		}

		if endRowIdx > totalRows {
			endRowIdx = totalRows
		}

		var buf strings.Builder
		buf.WriteString("\033[H")

		// Top border (Square)
		buf.WriteString(core.BLUE + core.Sym.BoxTL + border + core.Sym.BoxTR + core.NC + "\033[K\r\n")

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

		// Banner Section
		if len(activeBanner) > 0 {
			buf.WriteString(core.BLUE + core.Sym.BoxML + border + core.Sym.BoxMR + core.NC + "\033[K\r\n")
			for _, bLine := range activeBanner {
				bRow := bLine
				bPadTotal := innerW - core.RuneWidth(bRow)
				if bPadTotal < 0 {
					bRow = core.PadOrTruncateANSI(bRow, innerW)
					bPadTotal = 0
				}
				bPadL := bPadTotal / 2
				bPadR := bPadTotal - bPadL
				buf.WriteString(core.BLUE + core.Sym.BoxV + safeRepeat(" ", bPadL) + bRow + core.NC + safeRepeat(" ", bPadR) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
			}
		}

		// Content Lines
		if len(displayContent) > 0 {
			buf.WriteString(core.BLUE + core.Sym.BoxML + border + core.Sym.BoxMR + core.NC + "\033[K\r\n")
			for _, l := range displayContent {
				subLines := WrapBoxLine(l, innerW-2)
				for _, subL := range subLines {
					buf.WriteString(core.BLUE + core.Sym.BoxV + core.NC + " " + PadOrTruncate(subL, innerW-2) + core.NC + " " + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
				}
			}
		}

		// Separator
		buf.WriteString(core.BLUE + core.Sym.BoxML + border + core.Sym.BoxMR + core.NC + "\033[K\r\n")

		// Prompt Question
		if len(promptWrapped) > 0 {
			for _, pRow := range promptWrapped {
				buf.WriteString(core.BLUE + core.Sym.BoxV + core.NC + "  " + PadOrTruncate(pRow, innerW-4) + core.NC + "  " + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
			}
			buf.WriteString(core.BLUE + core.Sym.BoxV + safeRepeat(" ", innerW) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
		}

		// Top scroll indicator
		if showTopArrow {
			topMsg := core.Sym.ArrowUp + " [Up: more options above]"
			if core.RuneWidth(topMsg) > innerW {
				topMsg = core.PadOrTruncateANSI(topMsg, innerW)
			}
			padT := innerW - core.RuneWidth(topMsg)
			if padT < 0 {
				padT = 0
			}
			pL := padT / 2
			pR := padT - pL
			buf.WriteString(core.BLUE + core.Sym.BoxV + safeRepeat(" ", pL) + "\033[38;5;214m" + topMsg + "\033[0m" + safeRepeat(" ", pR) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
		}

		// Visible option rows
		headerIndent := 3

		for rIdx := scrollOffset; rIdx < endRowIdx; rIdx++ {
			r := allRows[rIdx]
			if r.IsHeader {
				hMaxW := innerW - headerIndent
				if hMaxW < 1 {
					hMaxW = 1
				}
				buf.WriteString(core.BLUE + core.Sym.BoxV + core.NC + safeRepeat(" ", headerIndent) + PadOrTruncate(r.LineText, hMaxW) + core.NC + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
			} else {
				buf.WriteString(core.BLUE + core.Sym.BoxV + core.NC + " " + PadOrTruncate(r.LineText, innerW-2) + core.NC + " " + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
			}
		}

		// Bottom scroll indicator
		if showBotArrow {
			botMsg := core.Sym.ArrowDown + " [Down: more options below]"
			if core.RuneWidth(botMsg) > innerW {
				botMsg = core.PadOrTruncateANSI(botMsg, innerW)
			}
			padB := innerW - core.RuneWidth(botMsg)
			if padB < 0 {
				padB = 0
			}
			pL := padB / 2
			pR := padB - pL
			buf.WriteString(core.BLUE + core.Sym.BoxV + safeRepeat(" ", pL) + "\033[38;5;214m" + botMsg + "\033[0m" + safeRepeat(" ", pR) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
		}

		// Action Context Preview Bar (Dynamic live operational guidance)
		if !isYesNo && len(descLines) > 0 {
			buf.WriteString(core.BLUE + core.Sym.BoxML + border + core.Sym.BoxMR + core.NC + "\033[K\r\n")
			for _, dLine := range descLines {
				buf.WriteString(core.BLUE + core.Sym.BoxV + core.NC + " " + PadOrTruncate(dLine, innerW-2) + core.NC + " " + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
			}
		}

		// Help line
		buf.WriteString(core.BLUE + core.Sym.BoxML + border + core.Sym.BoxMR + core.NC + "\033[K\r\n")

		var helpStr string
		if isYesNo {
			helpStr = fmt.Sprintf("%s[Up/Down]%s Move   %s[Enter]%s OK   %s[Y/N]%s Choose   %s[Esc]%s Exit",
				core.BOLD+core.YELLOW, core.NC,
				core.BOLD+core.YELLOW, core.NC,
				core.BOLD+core.GREEN, core.NC,
				core.BOLD+core.RED, core.NC)
		} else if isWizardOrModal {
			helpStr = fmt.Sprintf("%s[Up/Down]%s Move   %s%s%s Key   %s[Enter]%s OK   %s[Esc]%s Exit",
				core.BOLD+core.YELLOW, core.NC,
				core.BOLD+core.YELLOW, keyRange, core.NC,
				core.BOLD+core.YELLOW, core.NC,
				core.BOLD+core.RED, core.NC)
		} else {
			helpStr = fmt.Sprintf("%s[Up/Down]%s Move   %s%s%s Key   %s[Enter]%s OK   %s[T]%s Test   %s[R]%s Reload   %s[L]%s Logs   %s[D]%s Diag   %s[Tab]%s Expand   %s[Esc]%s Exit",
				core.BOLD+core.YELLOW, core.NC,
				core.BOLD+core.YELLOW, keyRange, core.NC,
				core.BOLD+core.YELLOW, core.NC,
				core.BOLD+core.CYAN, core.NC,
				core.BOLD+core.CYAN, core.NC,
				core.BOLD+core.CYAN, core.NC,
				core.BOLD+core.CYAN, core.NC,
				core.BOLD+core.GREEN, core.NC,
				core.BOLD+core.RED, core.NC)
		}

		hPadTotal := innerW - core.RuneWidth(helpStr)
		if hPadTotal < 0 {
			if isYesNo {
				helpStr = fmt.Sprintf("%s[Up/Down]%s Move  %s[Enter]%s OK  %s[Y/N]%s Choose  %s[Esc]%s Exit",
					core.BOLD+core.YELLOW, core.NC,
					core.BOLD+core.YELLOW, core.NC,
					core.BOLD+core.GREEN, core.NC,
					core.BOLD+core.RED, core.NC)
			} else if isWizardOrModal {
				helpStr = fmt.Sprintf("%s[Up/Down]%s Move  %s[Enter]%s OK  %s[Esc]%s Exit",
					core.BOLD+core.YELLOW, core.NC,
					core.BOLD+core.YELLOW, core.NC,
					core.BOLD+core.RED, core.NC)
			} else {
				helpStr = fmt.Sprintf("%s[Up/Down]%s Move  %s[Enter]%s OK  %s[T]%s Test  %s[L]%s Logs  %s[D]%s Diag  %s[Tab]%s Expand  %s[Esc]%s Exit",
					core.BOLD+core.YELLOW, core.NC,
					core.BOLD+core.YELLOW, core.NC,
					core.BOLD+core.CYAN, core.NC,
					core.BOLD+core.CYAN, core.NC,
					core.BOLD+core.CYAN, core.NC,
					core.BOLD+core.GREEN, core.NC,
					core.BOLD+core.RED, core.NC)
			}
			hPadTotal = innerW - core.RuneWidth(helpStr)
			if hPadTotal < 0 {
				if isYesNo || isWizardOrModal {
					helpStr = fmt.Sprintf("%s[Up/Down]%s Move  %s[Enter]%s OK  %s[Esc]%s Exit",
						core.BOLD+core.YELLOW, core.NC,
						core.BOLD+core.YELLOW, core.NC,
						core.BOLD+core.RED, core.NC)
				} else {
					helpStr = fmt.Sprintf("%s[Up/Down]%s Move  %s[Enter]%s OK  %s[T]%s Test  %s[Tab]%s Expand  %s[Esc]%s Exit",
						core.BOLD+core.YELLOW, core.NC,
						core.BOLD+core.YELLOW, core.NC,
						core.BOLD+core.CYAN, core.NC,
						core.BOLD+core.GREEN, core.NC,
						core.BOLD+core.RED, core.NC)
				}
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

		// Bottom border (Square)
		buf.WriteString(core.BLUE + core.Sym.BoxBL + border + core.Sym.BoxBR + core.NC + "\033[K\033[J")

		fmt.Print(buf.String())
	}

	draw := func() {
		isWizardOrModal := isYesNo || didInheritCard || strings.HasPrefix(title, "Step ") || strings.HasPrefix(title, "Confirm") || strings.HasPrefix(title, "Filter") || strings.Contains(title, "Conflict") || strings.Contains(title, "Summary") || strings.Contains(title, "Notice") || strings.Contains(title, "Strategy") || strings.Contains(title, "Template")
		if !isWizardOrModal && IsExpandedMode() {
			drawExpanded()
		} else {
			drawCompact()
		}
	}

	ClearScreen()
	draw()

	var numBuffer string
	var lastNumTime time.Time

	for {
		k := ReadKeyRaw()
		if k == "esc" || k == "ctrl+d" {
			return -2, "", 2
		}
		if k == "ctrl+c" {
			LeaveAltScreen()
			DisableRawMode()
			ShowCursor()
			os.Exit(0)
		}
		if k == "eof" {
			LeaveAltScreen()
			DisableRawMode()
			ShowCursor()
			os.Exit(0)
		}
		if k == "" {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if k == "backspace" {
			return -1, "", 1
		}
		if isYesNo {
			if strings.ToLower(k) == "y" {
				restoreInheritedCard(0)
				return 0, options[0], 0
			}
			if strings.ToLower(k) == "n" {
				restoreInheritedCard(1)
				return 1, options[1], 0
			}
		}
		if !isYesNo && (k == "tab" || k == "f2") {
			ToggleViewMode()
			focusedPanel = 0
			ClearScreen()
			draw()
			continue
		}
		if k == "enter" {
			if len(options) == 0 {
				return -1, "", 1
			}
			if sel < 0 {
				sel = 0
			}
			if sel >= len(options) {
				sel = len(options) - 1
			}
			restoreInheritedCard(sel)
			return sel, options[sel], 0
		}
		if k == "right" {
			if IsExpandedMode() {
				focusedPanel = 1
				draw()
				continue
			}
		}
		if k == "left" {
			if IsExpandedMode() {
				focusedPanel = 0
				draw()
				continue
			}
		}

		if focusedPanel == 1 && IsExpandedMode() {
			if k == "up" {
				if rightScrollOffset > 0 {
					rightScrollOffset--
					draw()
				}
				continue
			} else if k == "down" {
				if rightScrollOffset < maxRightScroll {
					rightScrollOffset++
					draw()
				}
				continue
			} else if k == "pgup" {
				rightScrollOffset -= 5
				if rightScrollOffset < 0 {
					rightScrollOffset = 0
				}
				draw()
				continue
			} else if k == "pgdn" {
				rightScrollOffset += 5
				if rightScrollOffset > maxRightScroll {
					rightScrollOffset = maxRightScroll
				}
				draw()
				continue
			} else if k == "home" {
				rightScrollOffset = 0
				draw()
				continue
			} else if k == "end" {
				rightScrollOffset = maxRightScroll
				draw()
				continue
			}
		} else {
			// focusedPanel == 0 (Left Menu navigation)
			if k == "up" {
				rightScrollOffset = 0
				sel = (sel - 1 + n) % n
				for strings.HasPrefix(options[sel], "---") || strings.HasPrefix(options[sel], "───") {
					sel = (sel - 1 + n) % n
				}
			} else if k == "down" {
				rightScrollOffset = 0
				sel = (sel + 1) % n
				for strings.HasPrefix(options[sel], "---") || strings.HasPrefix(options[sel], "───") {
					sel = (sel + 1) % n
				}
			} else if k == "home" {
				rightScrollOffset = 0
				sel = 0
				for sel < n && (strings.HasPrefix(options[sel], "---") || strings.HasPrefix(options[sel], "───")) {
					sel++
				}
			} else if k == "end" {
				rightScrollOffset = 0
				sel = n - 1
				for sel >= 0 && (strings.HasPrefix(options[sel], "---") || strings.HasPrefix(options[sel], "───")) {
					sel--
				}
			} else if k == "pgup" {
				rightScrollOffset = 0
				for step := 0; step < 5; step++ {
					sel = (sel - 1 + n) % n
					for strings.HasPrefix(options[sel], "---") || strings.HasPrefix(options[sel], "───") {
						sel = (sel - 1 + n) % n
					}
				}
			} else if k == "pgdn" {
				rightScrollOffset = 0
				for step := 0; step < 5; step++ {
					sel = (sel + 1) % n
					for strings.HasPrefix(options[sel], "---") || strings.HasPrefix(options[sel], "───") {
						sel = (sel + 1) % n
					}
				}
			}
		}

		if !isYesNo && strings.ToLower(k) == "r" {
			for idx, o := range options {
				if strings.HasPrefix(o, "---") || strings.HasPrefix(o, "───") {
					continue
				}
				cleanO := strings.ToLower(core.StripANSI(o))
				if strings.Contains(cleanO, "reload") || strings.Contains(cleanO, "restart") {
					restoreInheritedCard(idx)
					return idx, options[idx], 0
				}
			}
		} else if !isYesNo && strings.ToLower(k) == "l" {
			for idx, o := range options {
				if strings.HasPrefix(o, "---") || strings.HasPrefix(o, "───") {
					continue
				}
				cleanO := strings.ToLower(core.StripANSI(o))
				if strings.Contains(cleanO, "log") || strings.Contains(cleanO, "journal") {
					restoreInheritedCard(idx)
					return idx, options[idx], 0
				}
			}
			// Fallback: if current menu does not have a dedicated logs option, display live service logs modal
			tech := getTechnologyInfo()
			var logLines []string
			daemonName := ""
			if tech.Daemon != "" {
				daemonName = strings.Fields(tech.Daemon)[0]
			}
			if daemonName != "" {
				if out, err := exec.Command("journalctl", "-u", daemonName, "-n", "30", "--no-pager").CombinedOutput(); err == nil && len(out) > 0 {
					logLines = strings.Split(strings.TrimSpace(string(out)), "\n")
				}
			}
			if len(logLines) == 0 && tech.LogPaths != "" {
				for _, lp := range strings.Split(tech.LogPaths, ",") {
					lp = strings.TrimSpace(lp)
					if strings.Contains(lp, "journalctl") {
						fields := strings.Fields(lp)
						fields = append(fields, "-n", "30", "--no-pager")
						if out, err := exec.Command(fields[0], fields[1:]...).CombinedOutput(); err == nil && len(out) > 0 {
							logLines = strings.Split(strings.TrimSpace(string(out)), "\n")
							break
						}
					} else if _, err := os.Stat(lp); err == nil {
						if out, err := exec.Command("tail", "-n", "30", lp).CombinedOutput(); err == nil && len(out) > 0 {
							logLines = strings.Split(strings.TrimSpace(string(out)), "\n")
							break
						}
					}
				}
			}
			ClearScreen()
			var outBox []string
			if len(logLines) > 0 {
				outBox = FormatStyledLogLines(strings.Join(logLines, "\n"), "No active log streams found for this service daemon.")
			} else {
				outBox = append(outBox, "  "+core.YELLOW+"No active log streams found for this service daemon."+core.NC)
				if tech.LogPaths != "" {
					outBox = append(outBox, "  "+core.DIM+"Configured Log Paths: "+tech.LogPaths+core.NC)
				}
			}
			PrintInBox(tech.Name+" - Realtime Service Logs", outBox)
			ClearScreen()
			draw()
			continue
		} else if !isYesNo && strings.ToLower(k) == "t" {
			tech := getTechnologyInfo()
			if tech.ValidateCmd != "" {
				ClearScreen()
				var outLines []string
				outLines = append(outLines, FormatKV("Testing Command", tech.ValidateCmd, core.CYAN))
				cmdParts := strings.Fields(tech.ValidateCmd)
				cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
				out, err := cmd.CombinedOutput()
				if err == nil {
					outLines = append(outLines, FormatKV("Syntax Check", "SUCCESS - Configuration is valid!", core.GREEN))
				} else {
					outLines = append(outLines, FormatKV("Syntax Check", "FAILED - Syntax error detected!", core.RED))
				}
				if len(out) > 0 {
					outLines = append(outLines, "")
					outLines = append(outLines, core.BOLD+core.YELLOW+"Output Details:"+core.NC)
					if err != nil {
						for _, ol := range strings.Split(strings.TrimSpace(string(out)), "\n") {
							if s := core.SanitizeTerminalText(strings.TrimSpace(ol)); s != "" {
								outLines = append(outLines, "  "+core.RED+s+core.NC)
							}
						}
					} else {
						outLines = append(outLines, FormatKeyValueOutput(string(out), "")...)
					}
				}
				PrintInBox(tech.Name+" - Configuration Test", outLines)
				ClearScreen()
				draw()
				continue
			}
			for idx, o := range options {
				if strings.HasPrefix(o, "---") || strings.HasPrefix(o, "───") {
					continue
				}
				cleanO := strings.ToLower(core.StripANSI(o))
				if strings.Contains(cleanO, "syntax") || strings.Contains(cleanO, "test") {
					restoreInheritedCard(idx)
					return idx, options[idx], 0
				}
			}
		} else if !isYesNo && strings.ToLower(k) == "d" {
			for idx, o := range options {
				if strings.HasPrefix(o, "---") || strings.HasPrefix(o, "───") {
					continue
				}
				cleanO := strings.ToLower(core.StripANSI(o))
				if strings.Contains(cleanO, "diagnos") || strings.Contains(cleanO, "deep") {
					restoreInheritedCard(idx)
					return idx, options[idx], 0
				}
			}
			// Fallback: if current menu does not have a diagnostics option, display live technology & system audit
			tech := getTechnologyInfo()
			ClearScreen()
			var outBox []string
			outBox = append(outBox, getLiveTechnologyAudit(tech, GetTermWidth())...)
			m := getCachedMetrics()
			if m != nil {
				outBox = append(outBox, "")
				outBox = append(outBox, core.BOLD+core.YELLOW+"HOST SYSTEM & NETWORK TELEMETRY:"+core.NC)
				outBox = append(outBox, FormatKV("CPU Load", fmt.Sprintf("%.1f%% (%s)", m.cpuPct, m.cpuLoadStr), core.CYAN))
				outBox = append(outBox, FormatKV("Memory Usage", fmt.Sprintf("%s / %s (%.1f%%)", m.ramUsedStr, m.ramTotalStr, m.ramPct), core.GREEN))
				outBox = append(outBox, FormatKV("Disk Storage", fmt.Sprintf("%s (%.1f%%)", m.diskStr, m.diskPct), core.YELLOW))
				outBox = append(outBox, FormatKV("IPv4 Address", m.localIP, core.CYAN))
			}
			PrintInBox(tech.Name+" - Quick System Diagnostics", outBox)
			ClearScreen()
			draw()
			continue
		} else if _, err := strconv.Atoi(k); err == nil {
			if time.Since(lastNumTime) < 800*time.Millisecond {
				numBuffer += k
			} else {
				numBuffer = k
			}
			lastNumTime = time.Now()

			targetNum, _ := strconv.Atoi(numBuffer)
			validIdx := 0
			found := false
			for idx, o := range options {
				if !strings.HasPrefix(o, "---") && !strings.HasPrefix(o, "───") {
					validIdx++
					if validIdx == targetNum {
						sel = idx
						found = true
						break
					}
				}
			}
			if !found && len(numBuffer) > 1 {
				numBuffer = k
				if targetSingle, err2 := strconv.Atoi(numBuffer); err2 == nil {
					vIdx := 0
					for idx, o := range options {
						if !strings.HasPrefix(o, "---") && !strings.HasPrefix(o, "───") {
							vIdx++
							if vIdx == targetSingle {
								sel = idx
								break
							}
						}
					}
				}
			}
		}

		draw()
	}
}

// SelectMenu displays a full-screen interactive menu supporting keyboard navigation, split-screen inspector cards, and hotkeys.
func SelectMenu(title string, options []string) (int, string, int) {
	return SelectMenuWithContent(title, nil, options)
}

func AskYesNo(prompt string) int {
	if core.AutoYes {
		return 0
	}
	idx, _, status := SelectMenu(prompt, []string{"YES", "NO"})
	if status == 2 {
		return 2
	}
	if status == 1 {
		return -1
	}
	if idx == 0 {
		return 0
	}
	return 1
}
