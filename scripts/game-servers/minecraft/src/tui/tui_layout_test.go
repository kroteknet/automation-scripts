package tui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"krotek-minecraft-installer/core"
)

func assertAllLinesFitWidth(t *testing.T, label string, lines []string, maxW int) {
	t.Helper()
	for i, l := range lines {
		if strings.ContainsAny(l, "\n\r\t") {
			t.Fatalf("%s line %d contains raw control char: %q", label, i, l)
		}
		if w := core.RuneWidth(l); w > maxW {
			t.Fatalf("%s line %d width %d > %d: %q", label, i, w, maxW, l)
		}
		if strings.Contains(l, "\x1b[") {
			// Verify no unclosed/broken CSI prefix left mid-escape
			cleaned := core.StripANSI(l)
			if strings.Contains(cleaned, "\x1b") {
				t.Fatalf("%s line %d contains broken ANSI escape: %q", label, i, l)
			}
		}
	}
}

func assertExactFrameWidth(t *testing.T, label string, lines []string, expectedW int) {
	t.Helper()
	if len(lines) == 0 {
		t.Fatalf("%s produced 0 lines", label)
	}
	for i, l := range lines {
		if w := core.RuneWidth(l); w != expectedW {
			t.Fatalf("%s line %d width %d != %d: %q", label, i, w, expectedW, core.StripANSI(l))
		}
	}
}

func TestWrapCellText_ANSIAndLongTokens(t *testing.T) {
	styledLong := core.BOLD + core.GREEN + "superlongunbrokentokenwithoutanywhitespaceorslashes" + core.NC
	lines := WrapCellText(styledLong, 16)
	if len(lines) < 3 {
		t.Fatalf("expected long token to wrap into >=3 lines, got %d: %v", len(lines), lines)
	}
	assertAllLinesFitWidth(t, "styledLong", lines, 16)
	for i, l := range lines {
		if !strings.Contains(l, core.GREEN) || !strings.Contains(l, core.BOLD) {
			t.Fatalf("line %d lost BOLD+GREEN styling: %q", i, l)
		}
		if !strings.HasSuffix(l, core.NC) {
			t.Fatalf("line %d did not reset ANSI at end: %q", i, l)
		}
	}

	deepPath := core.CYAN + "/var/log/very/deep/directory/superlongfilename_without_breaks.properties" + core.NC
	pathLines := WrapCellText(deepPath, 18)
	assertAllLinesFitWidth(t, "deepPath", pathLines, 18)

	multiLine := "line one\twith tab\nline two\n" + core.YELLOW + "line three is very long indeed" + core.NC
	mlLines := WrapCellText(multiLine, 14)
	assertAllLinesFitWidth(t, "multiLine", mlLines, 14)
}

func TestWrapTextLines_LeadingIndentAndLongWords(t *testing.T) {
	longURL := "   https://api.papermc.io/v2/projects/paper/versions/1.21.4/builds/123/downloads/paper-1.21.4-123.jar"
	lines := WrapTextLines(longURL, 32)
	assertAllLinesFitWidth(t, "longURL", lines, 32)

	heavyIndent := strings.Repeat(" ", 30) + "word1 word2 word3"
	hiLines := WrapTextLines(heavyIndent, 24)
	assertAllLinesFitWidth(t, "heavyIndent", hiLines, 24)
}

func TestWrapBoxLine_KVAndProseColon(t *testing.T) {
	kv := FormatKV("Configuration File", core.CYAN+"/etc/minecraft/plugins/Essentials/superlongconfigfilename.yml"+core.NC, core.CYAN)
	kvLines := WrapBoxLine(kv, 50)
	assertAllLinesFitWidth(t, "kvPath", kvLines, 50)

	prose := "  This is a long diagnostic sentence that happens to have a colon near the end: nginx (pid 12345) failed to bind"
	proseLines := WrapBoxLine(prose, 60)
	assertAllLinesFitWidth(t, "proseColon", proseLines, 60)
}

func TestTablesAndBoxCards_ExactClosedBorders(t *testing.T) {
	for _, w := range []int{40, 60, 80, 105} {
		t2 := RenderExcelTable2Col("PARAMETER", "VALUE", 18, [][2]string{
			{"Key 1", core.BOLD + core.GREEN + "Active VHosts • SSL Let's Encrypt (OK) • HTTP/2, HTTP/3" + core.NC},
			{"Long Key Name Here", "/opt/minecraft/plugins/BlueMap/maps/world/superlongfile.json\twith\ttab"},
		}, w)
		assertExactFrameWidth(t, "Excel2Col", t2, w)

		t3 := RenderExcelTable3Col("COL1", "COL2", "COL3", 12, 14, [][3]string{
			{"ItemA", core.YELLOW + "VeryLongUnbrokenValueWithoutSpaces123456" + core.NC, "Line1\nLine2"},
		}, w)
		assertExactFrameWidth(t, "Excel3Col", t3, w)

		t4 := RenderExcelTable4Col("A", "B", "C", "D", 10, 10, [][4]string{
			{"Env", "Debian 13 (amd64)", "Status", core.BOLD + core.RED + "[STOPPED / INACTIVE]" + core.NC},
		}, w)
		assertExactFrameWidth(t, "Excel4Col", t4, w)

		card := RenderBoxCard("VERY LONG CARD TITLE THAT EXCEEDS SMALL BOXES", []string{
			"Short line",
			"A line with\ttabs and much more text than will fit inside a narrow card width of 40 columns",
			core.BOLD + core.GREEN + "Styled line without reset",
		}, w)
		assertExactFrameWidth(t, "RenderBoxCard", card, w)
	}
}

func TestRenderExpandedTopExcelTable_NoDoublePercentAndAligned(t *testing.T) {
	innerW := 118
	leftW := innerW / 2
	rightW := innerW - 1 - leftW
	rows := RenderExpandedTopExcelTable(innerW, leftW, rightW, 35)
	assertExactFrameWidth(t, "ExpandedTopExcelTable", rows, innerW+2)
	for i, r := range rows {
		if strings.Contains(r, "%%") {
			t.Fatalf("row %d contains double %% literal: %q", i, core.StripANSI(r))
		}
	}
}

func captureStdoutBoxTest(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	<-done
	return buf.String()
}

func TestPrintInBox_NoWaitFrameAlignment(t *testing.T) {
	core.AutoYes = false
	out := captureStdoutBoxTest(func() {
		PrintInBox("--no-wait", "Step 1/4: Very Long Wizard Step Title With Details", []string{
			FormatKV("Config Path", "/etc/minecraft/very/deep/nested/directory/server.properties", core.CYAN),
			"  " + core.BOLD + core.YELLOW + "Unclosed bold line",
			"Line with\ttab and\nembedded newline",
		})
	})

	// Strip leading cursor/mode escapes and split rendered lines
	clean := core.StripANSI(out)
	clean = strings.ReplaceAll(clean, "\x1b[?7l", "")
	clean = strings.ReplaceAll(clean, "\x1b[?7h", "")
	clean = strings.ReplaceAll(clean, "\x1b[H", "")
	clean = strings.ReplaceAll(clean, "\x1b[K", "")
	clean = strings.ReplaceAll(clean, "\x1b[J", "")
	lines := strings.Split(strings.TrimSpace(clean), "\r\n")
	if len(lines) < 5 {
		t.Fatalf("expected >=5 box lines, got %d: %q", len(lines), clean)
	}
	expectedW := core.RuneWidth(lines[0])
	for i, l := range lines {
		if w := core.RuneWidth(l); w != expectedW {
			t.Fatalf("PrintInBox line %d width %d != %d: %q", i, w, expectedW, l)
		}
	}
}

func TestRenderIntegratedInputBox_Alignment(t *testing.T) {
	SetInputCard("Step 2/4: Long Configuration Title That Should Fit Inside Frame", []string{
		"  Indented instruction line with\ttab",
		FormatKV("Selected Engine", "PaperMC 1.21.4 (Latest Stable Build)", core.GREEN),
	}, "[Esc] -> Back / Cancel")
	activeInputCard.Completed = []CompletedInputItem{
		{Label: "Domain", Value: "play.example.com"},
		{Label: "Port", Value: "25565"},
	}
	defer ResetInputCard()

	out := captureStdoutBoxTest(func() {
		renderIntegratedInputBox("Enter Server MOTD (Message of the Day)", "A Minecraft Server", []rune("Welcome to Krotek Survival"), 10, false, false, "")
	})
	clean := core.StripANSI(out)
	clean = strings.ReplaceAll(clean, "\x1b[H", "")
	clean = strings.ReplaceAll(clean, "\x1b[K", "")
	clean = strings.ReplaceAll(clean, "\x1b[J", "")
	clean = strings.ReplaceAll(clean, "\x1b[7m", "")
	clean = strings.ReplaceAll(clean, "\x1b[27m", "")
	lines := strings.Split(strings.TrimSpace(clean), "\r\n")
	if len(lines) < 10 {
		t.Fatalf("expected >=10 input box lines, got %d", len(lines))
	}
	expectedW := core.RuneWidth(lines[0])
	for i, l := range lines {
		if w := core.RuneWidth(l); w != expectedW {
			t.Fatalf("InputBox line %d width %d != %d: %q", i, w, expectedW, l)
		}
	}
}

