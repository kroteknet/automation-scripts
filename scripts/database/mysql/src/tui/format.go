// ==============================================================================
// KROTEK SCRIPT HUB — SHARED OUTPUT & SUBMENU FORMATTERS
// Package: tui
//
// Provides consistent ANSI formatting helpers for command output, systemd status,
// Docker container tables, structured key-value telemetry, and service log lines.
// ==============================================================================

package tui

import (
	"fmt"
	"regexp"
	"strings"

	"krotek-mysql-installer/core"
)

var (
	http2xxLogRegex   = regexp.MustCompile(`" 2[0-9]{2} `)
	http3xxLogRegex   = regexp.MustCompile(`" 3[0-9]{2} `)
	http4xxLogRegex   = regexp.MustCompile(`" 4[0-9]{2} `)
	http5xxLogRegex   = regexp.MustCompile(`" 5[0-9]{2} `)
	isoTimestampRegex = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?)\s+(.*)$`)
)

// FormatInfoNotice returns a consistently styled informational or warning single-block message.
func FormatInfoNotice(badge, msg, color string) []string {
	if color == "" {
		color = core.YELLOW
	}
	return []string{
		fmt.Sprintf("  %s%s[%s]%s %s", color, core.BOLD, badge, core.NC, msg),
	}
}

func formatLogSeverityLine(raw string) string {
	clean := strings.TrimSpace(core.SanitizeTerminalText(raw))
	if clean == "" {
		return ""
	}

	prefix := ""
	rest := clean
	if pipeIdx := strings.Index(rest, " | "); pipeIdx > 0 && pipeIdx < 36 && !strings.Contains(rest[:pipeIdx], " ") {
		svc := strings.TrimSpace(rest[:pipeIdx])
		prefix = fmt.Sprintf("%s%s%s %s|%s ", core.PURPLE+core.BOLD, svc, core.NC, core.DIM, core.NC)
		rest = strings.TrimSpace(rest[pipeIdx+3:])
	}

	if m := isoTimestampRegex.FindStringSubmatch(rest); len(m) == 3 {
		prefix += fmt.Sprintf("%s%s%s ", core.DIM+core.CYAN, m[1], core.NC)
		rest = m[2]
	}

	upper := strings.ToUpper(rest)
	styled := rest
	switch {
	case strings.Contains(upper, "FATAL") || strings.Contains(upper, "CRITICAL") || strings.Contains(upper, "PANIC") || strings.Contains(upper, "ERROR") || strings.Contains(upper, "[ERR"):
		styled = core.BOLD + core.RED + rest + core.NC
	case strings.Contains(upper, "WARN") || strings.Contains(upper, "[WRN"):
		styled = core.YELLOW + rest + core.NC
	case strings.Contains(upper, "SUCCESS") || strings.Contains(upper, "STARTED") || strings.Contains(upper, "READY") || strings.Contains(upper, "LISTENING"):
		styled = core.GREEN + rest + core.NC
	case strings.Contains(upper, "INFO"):
		styled = core.CYAN + rest + core.NC
	}

	styled = http2xxLogRegex.ReplaceAllString(styled, core.GREEN+"$0"+core.NC)
	styled = http3xxLogRegex.ReplaceAllString(styled, core.CYAN+"$0"+core.NC)
	styled = http4xxLogRegex.ReplaceAllString(styled, core.YELLOW+"$0"+core.NC)
	styled = http5xxLogRegex.ReplaceAllString(styled, core.RED+"$0"+core.NC)

	return "  " + prefix + styled
}

// FormatStyledLogLines parses raw log output into ANSI-highlighted lines with a styled empty fallback.
func FormatStyledLogLines(raw string, emptyMsg string) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if formatted := formatLogSeverityLine(line); formatted != "" {
			out = append(out, formatted)
		}
	}
	if len(out) == 0 {
		if emptyMsg == "" {
			emptyMsg = "No log entries available yet."
		}
		return FormatInfoNotice("INFO", emptyMsg, core.YELLOW)
	}
	return out
}

// FormatDockerContainerStatus styles tabular output from docker ps or docker compose ps.
func FormatDockerContainerStatus(raw string, emptyMsg string) []string {
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(raw), "\n") {
		sanitized := strings.TrimSpace(core.SanitizeTerminalText(l))
		if sanitized != "" {
			lines = append(lines, sanitized)
		}
	}
	if len(lines) == 0 {
		if emptyMsg == "" {
			emptyMsg = "No Docker containers found for this service."
		}
		return FormatInfoNotice("INFO", emptyMsg, core.YELLOW)
	}

	var out []string
	for i, l := range lines {
		upper := strings.ToUpper(l)
		if i == 0 && (strings.HasPrefix(upper, "NAME") || strings.HasPrefix(upper, "CONTAINER ID")) {
			out = append(out, "  "+core.BOLD+core.CYAN+l+core.NC)
			out = append(out, "  "+core.DIM+strings.Repeat(core.Sym.BoxH, 70)+core.NC)
			continue
		}
		styled := l
		switch {
		case strings.Contains(upper, "EXITED") || strings.Contains(upper, "DEAD") || strings.Contains(upper, "UNHEALTHY"):
			styled = core.RED + l + core.NC
		case strings.Contains(upper, "RESTARTING") || strings.Contains(upper, "PAUSED") || strings.Contains(upper, "CREATED"):
			styled = core.YELLOW + l + core.NC
		case strings.Contains(upper, "UP ") || strings.Contains(upper, "RUNNING") || strings.Contains(upper, "(HEALTHY)"):
			styled = core.GREEN + l + core.NC
		}
		out = append(out, fmt.Sprintf("  %s%s%s %s", core.CYAN, core.Sym.Pointer, core.NC, styled))
	}
	return out
}

// FormatKeyValueOutput styles key: value, key = value, [section], and wg show style outputs.
func FormatKeyValueOutput(raw string, emptyMsg string) []string {
	trimmedRaw := strings.TrimSpace(raw)
	if trimmedRaw == "" {
		if emptyMsg == "" {
			emptyMsg = "No output returned."
		}
		return FormatInfoNotice("INFO", emptyMsg, core.YELLOW)
	}

	var out []string
	for _, line := range strings.Split(trimmedRaw, "\n") {
		clean := core.SanitizeTerminalText(strings.TrimRight(line, "\r"))
		t := strings.TrimSpace(clean)
		if t == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}

		if strings.HasPrefix(t, "#") || strings.HasPrefix(t, ";") {
			out = append(out, "  "+core.DIM+t+core.NC)
			continue
		}

		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			out = append(out, core.BOLD+core.YELLOW+t+core.NC)
			continue
		}

		if strings.HasPrefix(t, "interface:") || strings.HasPrefix(t, "peer:") {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			parts := strings.SplitN(t, ":", 2)
			label := strings.ToUpper(strings.TrimSpace(parts[0]))
			val := strings.TrimSpace(parts[1])
			headerColor := core.CYAN
			if label == "PEER" {
				headerColor = core.YELLOW
			}
			out = append(out, fmt.Sprintf("%s%s%s %s:%s %s%s%s", core.BOLD, headerColor, core.Sym.Pointer, label, core.NC, core.BOLD+core.WHITE, val, core.NC))
			continue
		}

		idx := strings.Index(t, ":")
		eqIdx := strings.Index(t, "=")
		if eqIdx != -1 && (idx == -1 || eqIdx < idx) {
			idx = eqIdx
		}

		if idx > 0 && idx < 32 && !strings.Contains(t[:idx], "://") {
			k := strings.TrimSpace(t[:idx])
			v := strings.TrimSpace(t[idx+1:])
			valColor := core.WHITE
			lowK := strings.ToLower(k)
			switch {
			case v == "(hidden)" || v == "(none)":
				valColor = core.DIM
			case strings.Contains(lowK, "error") || strings.Contains(lowK, "fail"):
				valColor = core.RED
			case strings.Contains(lowK, "ip") || strings.Contains(lowK, "endpoint") || strings.Contains(lowK, "status") || strings.Contains(lowK, "enable"):
				valColor = core.GREEN
			case strings.Contains(lowK, "port") || strings.Contains(lowK, "handshake"):
				valColor = core.YELLOW
			case strings.Contains(lowK, "key") || strings.Contains(lowK, "transfer") || strings.Contains(lowK, "path") || strings.Contains(lowK, "dir"):
				valColor = core.CYAN
			}
			out = append(out, FormatKV(k, v, valColor))
			continue
		}

		out = append(out, "  "+t)
	}

	if len(out) == 0 {
		return FormatInfoNotice("INFO", emptyMsg, core.YELLOW)
	}
	return out
}

// FormatTableRows renders a header row, subtle rule, and aligned data rows.
func FormatTableRows(headers []string, rows [][]string, widths []int, emptyMsg string) []string {
	if len(rows) == 0 {
		if emptyMsg == "" {
			emptyMsg = "No records found."
		}
		return FormatInfoNotice("INFO", emptyMsg, core.YELLOW)
	}

	var out []string
	var hdrParts []string
	totalW := 0
	for i, h := range headers {
		w := 16
		if i < len(widths) && widths[i] > 0 {
			w = widths[i]
		}
		totalW += w + 2
		hdrParts = append(hdrParts, PadOrTruncate(h, w))
	}
	out = append(out, "  "+core.BOLD+core.CYAN+strings.Join(hdrParts, "  ")+core.NC)
	if totalW > 74 {
		totalW = 74
	}
	out = append(out, "  "+core.DIM+strings.Repeat(core.Sym.BoxH, totalW)+core.NC)

	for _, r := range rows {
		var cols []string
		for i := range headers {
			w := 16
			if i < len(widths) && widths[i] > 0 {
				w = widths[i]
			}
			cell := ""
			if i < len(r) {
				cell = r[i]
			}
			cols = append(cols, PadOrTruncate(cell, w))
		}
		out = append(out, "  "+strings.Join(cols, "  "))
	}
	return out
}
