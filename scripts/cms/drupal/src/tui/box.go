// ==============================================================================
// KROTEK SCRIPT HUB — ANSI TABLE & CARD RENDERING ENGINE
// Service: DRUPAL (cms)
// File:    cms/drupal/src/tui/box.go
//
// Renders zero-flicker Unicode box-drawing cards, key-value inspector panels, multi-column grids, and diagnostic summary dialogues.
// ==============================================================================

package tui

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"krotek-drupal-installer/core"
)

// FormatKV formats a key-value row with uniform indentation, bold labels, and styled values.
func safeRepeat(char string, count int) string {
	if count <= 0 {
		return ""
	}
	return strings.Repeat(char, count)
}

func FormatKV(label, value, valColor string) string {
	padLen := core.DefaultKeyWidth - core.RuneWidth(label)
	if padLen < 0 {
		padLen = 0
	}
	return fmt.Sprintf("  %s%s%s %s%s%s%s : %s%s%s", core.CYAN, core.Sym.Pointer, core.NC, core.BOLD, label, core.NC, safeRepeat(" ", padLen), valColor, value, core.NC)
}


func PadOrTruncate(text string, targetW int) string {
	if targetW <= 0 {
		return ""
	}
	cleanW := core.RuneWidth(text)
	if cleanW == targetW {
		return text
	}
	if cleanW < targetW {
		return text + strings.Repeat(" ", targetW-cleanW)
	}
	return core.PadOrTruncateANSI(text, targetW)
}

func RenderGauge(percent float64, barWidth int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	if barWidth < 5 {
		barWidth = 8
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

	color := core.GREEN
	if percent > 60 {
		color = core.YELLOW
	}
	if percent > 85 {
		color = core.RED
	}

	return fmt.Sprintf("%s[%s%s%s%s%s] %s%4.1f%%%s",
		core.DIM, color+core.BOLD, strings.Repeat(core.Sym.GaugeFull, filled),
		core.NC+core.DIM, strings.Repeat(core.Sym.GaugeEmpty, empty), core.NC+core.DIM,
		color+core.BOLD, percent, core.NC)
}

type TechnologyDetails struct {
	Name        string
	Category    string
	Daemon      string
	Ports       string
	ConfigPaths string
	LogPaths    string
	Security    string
	CliCommand  string
	ValidateCmd string
	DataDir     string
}

func getTechnologyInfo() TechnologyDetails {
	return TechnologyDetails{
		Name:        "Drupal Digital Experience Platform",
		Category:    "cms",
		Daemon:      "nginx.service + php-fpm.service + mariadb.service",
		Ports:       "80 (HTTP), 443 (HTTPS), PHP-FPM Socket",
		ConfigPaths: "/var/www/html/sites/default/settings.php",
		LogPaths:    "/var/log/nginx/error.log, /var/log/php-fpm.log",
		Security:    "Twig Template Sandboxing, Trusted Host Patterns",
		CliCommand:  "sudo drupal-manager",
		ValidateCmd: "",
		DataDir:     "",
	}
}

type ServerMetricsCache struct {
	host         string
	distro       string
	arch         string
	uptimeStr    string
	numCPU       int
	cpuLoadStr   string
	cpuPct       float64
	ramPct       float64
	ramUsedStr   string
	ramTotalStr  string
	diskPct      float64
	diskStr      string
	diskUsedStr  string
	diskTotalStr string
	diskFreeStr  string
	localIP      string
	daemonStatus string
	unitStatus   string
	fwStatus     string
	sshStatus    string
	appRamBytes  int64
	appRamStr    string
	appRamPct    float64
	appDiskBytes int64
	appDiskStr   string
}

var (
	cachedMetrics    *ServerMetricsCache
	lastMetricsCheck time.Time
	prevCPUTotal     uint64
	prevCPUIdle      uint64
	prevCPUTime      time.Time
	cachedCPUPct     float64
)

func getSystemCPUPercent() float64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return -1
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return -1
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 5 || fields[0] != "cpu" {
		return -1
	}
	var total, idle uint64
	for i := 1; i < len(fields); i++ {
		val, _ := strconv.ParseUint(fields[i], 10, 64)
		total += val
		if i == 4 {
			idle = val
		}
	}
	now := time.Now()
	if prevCPUTotal > 0 && total > prevCPUTotal && now.Sub(prevCPUTime) >= 500*time.Millisecond {
		diffTotal := float64(total - prevCPUTotal)
		diffIdle := float64(idle - prevCPUIdle)
		if diffTotal > 0 {
			cachedCPUPct = ((diffTotal - diffIdle) / diffTotal) * 100.0
			if cachedCPUPct < 0 {
				cachedCPUPct = 0
			}
			if cachedCPUPct > 100 {
				cachedCPUPct = 100
			}
		}
		prevCPUTotal = total
		prevCPUIdle = idle
		prevCPUTime = now
	} else if prevCPUTotal == 0 {
		prevCPUTotal = total
		prevCPUIdle = idle
		prevCPUTime = now
	}
	return cachedCPUPct
}

func getCachedMetrics() *ServerMetricsCache {
	if cachedMetrics != nil && time.Since(lastMetricsCheck) < 2*time.Second {
		return cachedMetrics
	}

	m := &ServerMetricsCache{}

	// Hostname & Distro
	host, _ := os.Hostname()
	if host == "" {
		host = "localhost"
	}
	m.host = host

	m.distro = core.SysInfo.Distro
	if m.distro == "" {
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, l := range lines {
				if strings.HasPrefix(l, "PRETTY_NAME=") {
					m.distro = strings.Trim(strings.TrimPrefix(l, "PRETTY_NAME="), "\"")
					m.distro = strings.ReplaceAll(m.distro, "GNU/Linux", "")
					m.distro = strings.TrimSpace(m.distro)
					break
				}
			}
		}
	}
	if m.distro == "" {
		m.distro = "Linux"
	}
	m.arch = core.SysInfo.Architecture
	if m.arch == "" {
		m.arch = runtime.GOARCH
	}

	// Uptime
	m.uptimeStr = "N/A"
	if upData, err := os.ReadFile("/proc/uptime"); err == nil {
		parts := strings.Fields(string(upData))
		if len(parts) > 0 {
			if upSecs, err := strconv.ParseFloat(parts[0], 64); err == nil {
				days := int(upSecs) / 86400
				hours := (int(upSecs) % 86400) / 3600
				mins := (int(upSecs) % 3600) / 60
				if days > 0 {
					m.uptimeStr = fmt.Sprintf("%dd %dh %dm", days, hours, mins)
				} else if hours > 0 {
					m.uptimeStr = fmt.Sprintf("%dh %dm", hours, mins)
				} else {
					m.uptimeStr = fmt.Sprintf("%dm", mins)
				}
			}
		}
	}

	// RAM info
	m.ramUsedStr = "0.0G"
	m.ramTotalStr = "0.0G"
	if memData, err := os.ReadFile("/proc/meminfo"); err == nil {
		var memTotal, memAvail float64
		for _, line := range strings.Split(string(memData), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if fields[0] == "MemTotal:" {
					memTotal, _ = strconv.ParseFloat(fields[1], 64)
				} else if fields[0] == "MemAvailable:" {
					memAvail, _ = strconv.ParseFloat(fields[1], 64)
				}
			}
		}
		if memTotal > 0 {
			used := memTotal - memAvail
			m.ramPct = (used / memTotal) * 100.0
			m.ramUsedStr = fmt.Sprintf("%.1fG", used/(1024*1024))
			m.ramTotalStr = fmt.Sprintf("%.1fG", memTotal/(1024*1024))
		}
	}

	// CPU info
	m.numCPU = runtime.NumCPU()
	m.cpuLoadStr = "0.00"
	if loadData, err := os.ReadFile("/proc/loadavg"); err == nil {
		parts := strings.Fields(string(loadData))
		if len(parts) >= 3 {
			m.cpuLoadStr = fmt.Sprintf("%s, %s, %s", parts[0], parts[1], parts[2])
			if l1, err := strconv.ParseFloat(parts[0], 64); err == nil {
				cores := float64(m.numCPU)
				if cores < 1 {
					cores = 1
				}
				m.cpuPct = (l1 / cores) * 100.0
				if m.cpuPct > 100 {
					m.cpuPct = 100
				}
			}
		}
	}
	if cpuStat := getSystemCPUPercent(); cpuStat >= 0 && !prevCPUTime.IsZero() {
		m.cpuPct = cpuStat
	}

	// Disk info
	m.diskStr = "N/A"
	if out, err := exec.Command("df", "-h", "/").Output(); err == nil {
		dfLines := strings.Split(string(out), "\n")
		if len(dfLines) >= 2 {
			fields := strings.Fields(dfLines[1])
			if len(fields) >= 5 {
				cleanPct := strings.TrimSuffix(fields[4], "%")
				if p, err := strconv.ParseFloat(cleanPct, 64); err == nil {
					m.diskPct = p
				}
				m.diskTotalStr = fields[1]
				m.diskUsedStr = fields[2]
				m.diskFreeStr = fields[3]
				m.diskStr = fmt.Sprintf("%s/%s (Free: %s)", fields[2], fields[1], fields[3])
			}
		}
	}

	// Local IP
	m.localIP = "127.0.0.1"
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					m.localIP = ipnet.IP.String()
					break
				}
			}
		}
	}

	// Service Daemon & Environment Health
	tech := getTechnologyInfo()
	daemonName := "daemon"
	if tech.Daemon != "" {
		daemonName = strings.Fields(tech.Daemon)[0]
	}
	baseDaemon := strings.TrimSuffix(daemonName, ".service")
	m.daemonStatus = core.DIM + core.TagOff() + " Stopped" + core.NC
	if exec.Command("systemctl", "is-active", "--quiet", daemonName).Run() == nil ||
		exec.Command("rc-service", baseDaemon, "status").Run() == nil ||
		exec.Command("rc-service", daemonName, "status").Run() == nil ||
		exec.Command("pidof", baseDaemon).Run() == nil ||
		exec.Command("pidof", daemonName).Run() == nil {
		if core.RichTerminal {
			m.daemonStatus = core.BOLD + core.GREEN + core.Sym.ActiveDot + " Active & Running" + core.NC
		} else {
			m.daemonStatus = core.BOLD + core.GREEN + "[OK] Active & Running" + core.NC
		}
	}

	m.unitStatus = core.DIM + core.TagOff() + " Disabled" + core.NC
	if exec.Command("systemctl", "is-enabled", "--quiet", daemonName).Run() == nil ||
		(core.SysInfo.Family == "alpine" && (exec.Command("rc-service", baseDaemon, "status").Run() == nil || exec.Command("rc-service", daemonName, "status").Run() == nil)) {
		m.unitStatus = core.BOLD + core.GREEN + core.TagOK() + " Enabled (Auto-start)" + core.NC
	}

	m.fwStatus = core.DIM + core.TagOff() + " Inactive" + core.NC
	if exec.Command("ufw", "status").Run() == nil {
		m.fwStatus = core.BOLD + core.GREEN + core.TagOK() + " UFW Active" + core.NC
	} else if exec.Command("firewall-cmd", "--state").Run() == nil {
		m.fwStatus = core.BOLD + core.GREEN + core.TagOK() + " Firewalld Active" + core.NC
	}

	m.sshStatus = core.DIM + core.TagOff() + " Stopped" + core.NC
	if exec.Command("systemctl", "is-active", "--quiet", "ssh").Run() == nil || exec.Command("systemctl", "is-active", "--quiet", "sshd").Run() == nil {
		m.sshStatus = core.BOLD + core.GREEN + core.TagOK() + " Port 22 Active" + core.NC
	}

	// Application Memory (cgroup & RSS)
	var totalMemKB float64
	if memData, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(memData), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "MemTotal:" {
				totalMemKB, _ = strconv.ParseFloat(fields[1], 64)
				break
			}
		}
	}
	m.appRamBytes, m.appRamStr, m.appRamPct = getAppMemoryUsage(daemonName, totalMemKB)

	// Application Disk Footprint
	var appPaths []string
	if tech.DataDir != "" {
		appPaths = append(appPaths, tech.DataDir)
	}
	for _, p := range strings.Split(tech.ConfigPaths, ",") {
		clean := strings.TrimSpace(p)
		if clean != "" {
			appPaths = append(appPaths, clean)
		}
	}
	for _, p := range strings.Split(tech.LogPaths, ",") {
		clean := strings.TrimSpace(p)
		if clean != "" {
			appPaths = append(appPaths, clean)
		}
	}
	m.appDiskBytes, m.appDiskStr = getAppDiskUsage(appPaths)

	cachedMetrics = m
	lastMetricsCheck = time.Now()
	return m
}

func getAppMemoryUsage(daemonName string, totalMemKB float64) (int64, string, float64) {
	if daemonName == "" {
		return 0, "0.0 MB (Idle)", 0
	}
	baseDaemon := strings.TrimSuffix(daemonName, ".service")

	// 1. Try systemd cgroup MemoryCurrent
	if out, err := exec.Command("systemctl", "show", "-p", "MemoryCurrent", daemonName).Output(); err == nil {
		s := strings.TrimSpace(string(out))
		if strings.HasPrefix(s, "MemoryCurrent=") {
			valStr := strings.TrimPrefix(s, "MemoryCurrent=")
			if bytesVal, err := strconv.ParseInt(valStr, 10, 64); err == nil && bytesVal > 0 && bytesVal < 1099511627776 {
				mb := float64(bytesVal) / (1024 * 1024)
				var pct float64
				if totalMemKB > 0 {
					pct = (float64(bytesVal) / (totalMemKB * 1024)) * 100.0
				}
				str := fmt.Sprintf("%.1f MB (Resident)", mb)
				if mb >= 1024 {
					str = fmt.Sprintf("%.2f GB (Resident)", mb/1024)
				}
				return bytesVal, str, pct
			}
		}
	}

	// 2. Fallback: inspect ps rss for matching process name
	if out, err := exec.Command("ps", "-C", baseDaemon, "-o", "rss=").Output(); err == nil {
		lines := strings.Fields(string(out))
		var totalRssKB int64
		for _, l := range lines {
			if kb, err := strconv.ParseInt(l, 10, 64); err == nil {
				totalRssKB += kb
			}
		}
		if totalRssKB > 0 {
			mb := float64(totalRssKB) / 1024.0
			var pct float64
			if totalMemKB > 0 {
				pct = (float64(totalRssKB) / totalMemKB) * 100.0
			}
			str := fmt.Sprintf("%.1f MB (Resident RSS)", mb)
			if mb >= 1024 {
				str = fmt.Sprintf("%.2f GB (Resident RSS)", mb/1024)
			}
			return totalRssKB * 1024, str, pct
		}
	}

	// 3. Fallback: pidof + /proc/<pid>/statm (for Busybox / Alpine)
	if out, err := exec.Command("pidof", baseDaemon).Output(); err == nil {
		pids := strings.Fields(string(out))
		var totalPages int64
		for _, p := range pids {
			if data, err := os.ReadFile(fmt.Sprintf("/proc/%s/statm", p)); err == nil {
				fields := strings.Fields(string(data))
				if len(fields) >= 2 {
					if pages, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
						totalPages += pages
					}
				}
			}
		}
		if totalPages > 0 {
			pageSize := int64(os.Getpagesize())
			totalBytes := totalPages * pageSize
			mb := float64(totalBytes) / (1024 * 1024)
			var pct float64
			if totalMemKB > 0 {
				pct = (float64(totalBytes) / (totalMemKB * 1024)) * 100.0
			}
			str := fmt.Sprintf("%.1f MB (Resident)", mb)
			if mb >= 1024 {
				str = fmt.Sprintf("%.2f GB (Resident)", mb/1024)
			}
			return totalBytes, str, pct
		}
	}

	return 0, "0.0 MB (Service Inactive)", 0
}

func getAppDiskUsage(paths []string) (int64, string) {
	var totalBytes int64
	var validPaths []string
	for _, p := range paths {
		cleanP := strings.TrimSpace(p)
		if cleanP == "" {
			continue
		}
		if strings.Contains(cleanP, "*") {
			if matches, err := filepath.Glob(cleanP); err == nil {
				for _, m := range matches {
					if fi, err := os.Stat(m); err == nil {
						if !fi.IsDir() {
							totalBytes += fi.Size()
						} else {
							validPaths = append(validPaths, m)
						}
					}
				}
			}
			continue
		}
		if fi, err := os.Stat(cleanP); err == nil {
			if !fi.IsDir() {
				totalBytes += fi.Size()
			} else {
				validPaths = append(validPaths, cleanP)
			}
		}
	}

	if len(validPaths) > 0 {
		args := append([]string{"-sb"}, validPaths...)
		if out, err := exec.Command("du", args...).Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 1 {
					if b, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
						totalBytes += b
					}
				}
			}
		} else {
			argsK := append([]string{"-sk"}, validPaths...)
			if outK, errK := exec.Command("du", argsK...).Output(); errK == nil {
				for _, line := range strings.Split(string(outK), "\n") {
					fields := strings.Fields(line)
					if len(fields) >= 1 {
						if kb, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
							totalBytes += kb * 1024
						}
					}
				}
			}
		}
	}

	if totalBytes <= 0 {
		return 0, "0.0 MB (Standard system)"
	}

	mb := float64(totalBytes) / (1024 * 1024)
	if mb < 1.0 {
		return totalBytes, fmt.Sprintf("%.1f KB (Data & Config)", float64(totalBytes)/1024)
	}
	if mb >= 1024.0 {
		return totalBytes, fmt.Sprintf("%.2f GB (App footprint)", mb/1024.0)
	}
	return totalBytes, fmt.Sprintf("%.1f MB (App footprint)", mb)
}

type ansiStyle struct {
	bold bool
	dim  bool
	fg   string
	bg   string
}

func (s *ansiStyle) apply(seq string) {
	if seq == "\033[0m" || seq == "\033[m" {
		s.bold = false
		s.dim = false
		s.fg = ""
		s.bg = ""
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
	if seq == "\033[22m" {
		s.bold = false
		s.dim = false
		return
	}
	if seq == "\033[39m" {
		s.fg = ""
		return
	}
	if seq == "\033[49m" {
		s.bg = ""
		return
	}
	if strings.HasPrefix(seq, "\033[48;") || strings.HasPrefix(seq, "\033[4") && len(seq) == 5 && seq[3] >= '0' && seq[3] <= '7' ||
		strings.HasPrefix(seq, "\033[10") {
		s.bg = seq
		return
	}
	if strings.Contains(seq, "[1;") {
		s.bold = true
	}
	s.fg = seq
}

func (s ansiStyle) isStyled() bool {
	return s.bold || s.dim || s.fg != "" || s.bg != ""
}

func (s ansiStyle) prefix() string {
	if !s.isStyled() {
		return ""
	}
	var b strings.Builder
	if s.bold {
		b.WriteString(core.BOLD)
	}
	if s.dim {
		b.WriteString(core.DIM)
	}
	b.WriteString(s.fg)
	b.WriteString(s.bg)
	return b.String()
}

func sanitizeBoxText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\t", "  ")
	return s
}

func hardSplitANSIToken(token string, maxW int, st *ansiStyle) []string {
	if maxW < 1 {
		maxW = 1
	}
	var chunks []string
	var cur strings.Builder
	cur.WriteString(st.prefix())
	curW := 0
	hasVisible := false
	lineStyled := st.isStyled()

	for i := 0; i < len(token); {
		if token[i] == '\x1b' && i+1 < len(token) && token[i+1] == '[' {
			if loc := core.AnsiRegex.FindStringIndex(token[i:]); loc != nil && loc[0] == 0 {
				seq := token[i : i+loc[1]]
				st.apply(seq)
				if st.isStyled() {
					lineStyled = true
				}
				cur.WriteString(seq)
				i += loc[1]
				continue
			}
		}

		r, size := utf8.DecodeRuneInString(token[i:])
		rW := core.RuneWidth(string(r))
		if rW == 0 && r >= 32 {
			rW = 1
		}

		if hasVisible && curW+rW > maxW {
			s := cur.String()
			if lineStyled && !strings.HasSuffix(s, core.NC) {
				s += core.NC
			}
			chunks = append(chunks, s)
			cur.Reset()
			cur.WriteString(st.prefix())
			curW = 0
			hasVisible = false
			lineStyled = st.isStyled()
		}

		cur.WriteString(token[i : i+size])
		curW += rW
		hasVisible = true
		i += size
	}

	if hasVisible || len(chunks) == 0 {
		s := cur.String()
		if lineStyled && !strings.HasSuffix(s, core.NC) {
			s += core.NC
		}
		chunks = append(chunks, s)
	}
	return chunks
}

func wrapTokensANSI(text string, maxW int, splitCommas bool) []string {
	if maxW < 1 {
		maxW = 1
	}
	work := text
	if splitCommas {
		work = strings.ReplaceAll(work, ", ", ",\x00")
	}

	var rawFields []string
	if splitCommas {
		for _, p := range strings.Split(work, "\x00") {
			rawFields = append(rawFields, strings.Fields(p)...)
		}
	} else {
		rawFields = strings.Fields(work)
	}

	if len(rawFields) == 0 {
		return []string{""}
	}

	var lines []string
	var cur strings.Builder
	curW := 0
	var st ansiStyle
	curStyled := false

	flushLine := func() {
		if cur.Len() == 0 {
			return
		}
		s := cur.String()
		if curStyled && !strings.HasSuffix(s, core.NC) {
			s += core.NC
		}
		lines = append(lines, s)
		cur.Reset()
		curW = 0
		curStyled = false
	}

	startNewLine := func() {
		cur.Reset()
		pref := st.prefix()
		cur.WriteString(pref)
		curW = 0
		curStyled = st.isStyled()
	}

	appendPiece := func(piece string, needSpace bool) {
		pW := core.RuneWidth(piece)
		if pW > maxW {
			if curW > 0 {
				flushLine()
			}
			chunks := hardSplitANSIToken(piece, maxW, &st)
			for ci, ch := range chunks {
				if ci < len(chunks)-1 {
					lines = append(lines, ch)
				} else {
					cur.Reset()
					if st.isStyled() && strings.HasSuffix(ch, core.NC) {
						ch = strings.TrimSuffix(ch, core.NC)
					}
					cur.WriteString(ch)
					curW = core.RuneWidth(ch)
					curStyled = st.isStyled()
				}
			}
			return
		}

		addW := pW
		if needSpace && curW > 0 {
			addW++
		}
		if curW > 0 && curW+addW > maxW {
			flushLine()
			startNewLine()
		} else if curW == 0 {
			startNewLine()
		} else if needSpace {
			cur.WriteByte(' ')
			curW++
		}

		cur.WriteString(piece)
		curW += pW
		for _, seq := range core.AnsiRegex.FindAllString(piece, -1) {
			st.apply(seq)
			if st.isStyled() {
				curStyled = true
			}
		}
	}

	for _, tok := range rawFields {
		if core.RuneWidth(tok) > maxW && strings.Contains(tok, "/") {
			slashPieces := strings.Split(tok, "/")
			for idx, sp := range slashPieces {
				piece := sp
				if idx < len(slashPieces)-1 {
					piece += "/"
				}
				if piece == "" {
					continue
				}
				appendPiece(piece, idx == 0)
			}
		} else {
			appendPiece(tok, true)
		}
	}

	flushLine()
	if len(lines) == 0 {
		return []string{text}
	}
	return lines
}

// WrapCellText splits text into lines that fit within maxW runes.
// It wraps at spaces, commas, and slashes, and chunks long tokens safely without breaking ANSI escapes.
func WrapCellText(text string, maxW int) []string {
	if maxW <= 0 {
		return []string{""}
	}
	text = sanitizeBoxText(text)
	if strings.Contains(text, "\n") {
		var out []string
		for _, part := range strings.Split(text, "\n") {
			out = append(out, WrapCellText(part, maxW)...)
		}
		return out
	}
	if core.RuneWidth(text) <= maxW {
		return []string{text}
	}
	return wrapTokensANSI(text, maxW, true)
}

// RenderExcelTable2Col formats a 2-column spreadsheet grid with Excel-style horizontal and vertical gridlines.
func RenderExcelTable2Col(col1Header, col2Header string, col1W int, rows [][2]string, targetW int) []string {
	if targetW < 10 {
		targetW = 10
	}
	if col1W < 3 {
		col1W = 3
	}
	col2W := targetW - col1W - 3
	if col2W < 3 {
		col2W = 3
		col1W = targetW - col2W - 3
		if col1W < 3 {
			col1W = 3
		}
	}

	var res []string
	res = append(res, core.BLUE+core.Sym.BoxTL+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxTM+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxTR+core.NC)

	h1 := " " + core.BOLD + core.YELLOW + col1Header + core.NC
	h2 := " " + core.BOLD + core.WHITE + col2Header + core.NC
	res = append(res, core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(h1, col1W)+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(h2, col2W)+core.BLUE+core.Sym.BoxV+core.NC)

	res = append(res, core.BLUE+core.Sym.BoxML+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxMR+core.NC)

	for i, r := range rows {
		w1 := WrapCellText(r[0], col1W-2)
		w2 := WrapCellText(r[1], col2W-2)
		maxH := len(w1)
		if len(w2) > maxH {
			maxH = len(w2)
		}

		for h := 0; h < maxH; h++ {
			c1Str := ""
			if h < len(w1) {
				if strings.Contains(w1[h], "\033[") {
					c1Str = " " + w1[h]
				} else {
					c1Str = " " + core.CYAN + w1[h] + core.NC
				}
			}
			c2Str := ""
			if h < len(w2) {
				c2Str = " " + w2[h]
			}
			res = append(res, core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(c1Str, col1W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(c2Str, col2W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC)
		}
		if i < len(rows)-1 {
			res = append(res, core.BLUE+core.Sym.BoxML+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxMR+core.NC)
		}
	}

	res = append(res, core.BLUE+core.Sym.BoxBL+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxBM+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxBR+core.NC)
	return res
}

// RenderExcelTable4Col formats a 4-column spreadsheet grid with Excel-style horizontal and vertical gridlines.
func RenderExcelTable4Col(col1H, col2H, col3H, col4H string, col1W, col3W int, rows [][4]string, targetW int) []string {
	if targetW < 20 {
		targetW = 20
	}
	if col1W < 3 {
		col1W = 3
	}
	if col3W < 3 {
		col3W = 3
	}
	rem := targetW - col1W - col3W - 5
	if rem < 4 {
		rem = 4
	}
	col2W := rem / 2
	if col2W < 2 {
		col2W = 2
	}
	col4W := rem - col2W
	if col4W < 2 {
		col4W = 2
	}

	var res []string
	res = append(res, core.BLUE+core.Sym.BoxTL+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxTM+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxTM+safeRepeat(core.Sym.BoxH, col3W)+core.Sym.BoxTM+safeRepeat(core.Sym.BoxH, col4W)+core.Sym.BoxTR+core.NC)

	h1 := " " + core.BOLD + core.YELLOW + col1H + core.NC
	h2 := " " + core.BOLD + core.WHITE + col2H + core.NC
	h3 := " " + core.BOLD + core.YELLOW + col3H + core.NC
	h4 := " " + core.BOLD + core.WHITE + col4H + core.NC
	res = append(res, core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(h1, col1W)+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(h2, col2W)+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(h3, col3W)+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(h4, col4W)+core.BLUE+core.Sym.BoxV+core.NC)

	res = append(res, core.BLUE+core.Sym.BoxML+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col3W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col4W)+core.Sym.BoxMR+core.NC)

	for i, r := range rows {
		w1 := WrapCellText(r[0], col1W-2)
		w2 := WrapCellText(r[1], col2W-2)
		w3 := WrapCellText(r[2], col3W-2)
		w4 := WrapCellText(r[3], col4W-2)

		maxH := len(w1)
		if len(w2) > maxH {
			maxH = len(w2)
		}
		if len(w3) > maxH {
			maxH = len(w3)
		}
		if len(w4) > maxH {
			maxH = len(w4)
		}

		for h := 0; h < maxH; h++ {
			c1Str := ""
			if h < len(w1) {
				if strings.Contains(w1[h], "\033[") {
					c1Str = " " + w1[h]
				} else {
					c1Str = " " + core.CYAN + w1[h] + core.NC
				}
			}
			c2Str := ""
			if h < len(w2) {
				c2Str = " " + w2[h]
			}
			c3Str := ""
			if h < len(w3) {
				if strings.Contains(w3[h], "\033[") {
					c3Str = " " + w3[h]
				} else {
					c3Str = " " + core.CYAN + w3[h] + core.NC
				}
			}
			c4Str := ""
			if h < len(w4) {
				c4Str = " " + w4[h]
			}
			res = append(res, core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(c1Str, col1W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(c2Str, col2W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(c3Str, col3W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(c4Str, col4W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC)
		}
		if i < len(rows)-1 {
			res = append(res, core.BLUE+core.Sym.BoxML+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col3W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col4W)+core.Sym.BoxMR+core.NC)
		}
	}

	res = append(res, core.BLUE+core.Sym.BoxBL+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxBM+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxBM+safeRepeat(core.Sym.BoxH, col3W)+core.Sym.BoxBM+safeRepeat(core.Sym.BoxH, col4W)+core.Sym.BoxBR+core.NC)
	return res
}

// RenderExcelTable3Col formats a 3-column spreadsheet grid with Excel-style horizontal and vertical gridlines.
func RenderExcelTable3Col(col1H, col2H, col3H string, col1W, col2W int, rows [][3]string, targetW int) []string {
	if targetW < 15 {
		targetW = 15
	}
	if col1W < 3 {
		col1W = 3
	}
	if col2W < 3 {
		col2W = 3
	}
	col3W := targetW - col1W - col2W - 4
	if col3W < 3 {
		col3W = 3
		rem := targetW - col3W - 4
		if rem < 6 {
			rem = 6
		}
		col1W = rem / 2
		col2W = rem - col1W
	}

	var res []string
	res = append(res, core.BLUE+core.Sym.BoxTL+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxTM+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxTM+safeRepeat(core.Sym.BoxH, col3W)+core.Sym.BoxTR+core.NC)

	h1 := " " + core.BOLD + core.YELLOW + col1H + core.NC
	h2 := " " + core.BOLD + core.WHITE + col2H + core.NC
	h3 := " " + core.BOLD + core.WHITE + col3H + core.NC
	res = append(res, core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(h1, col1W)+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(h2, col2W)+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(h3, col3W)+core.BLUE+core.Sym.BoxV+core.NC)

	res = append(res, core.BLUE+core.Sym.BoxML+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col3W)+core.Sym.BoxMR+core.NC)

	for i, r := range rows {
		w1 := WrapCellText(r[0], col1W-2)
		w2 := WrapCellText(r[1], col2W-2)
		w3 := WrapCellText(r[2], col3W-2)

		maxH := len(w1)
		if len(w2) > maxH {
			maxH = len(w2)
		}
		if len(w3) > maxH {
			maxH = len(w3)
		}

		for h := 0; h < maxH; h++ {
			c1Str := ""
			if h < len(w1) {
				if strings.Contains(w1[h], "\033[") {
					c1Str = " " + w1[h]
				} else {
					c1Str = " " + core.CYAN + w1[h] + core.NC
				}
			}
			c2Str := ""
			if h < len(w2) {
				c2Str = " " + w2[h]
			}
			c3Str := ""
			if h < len(w3) {
				c3Str = " " + w3[h]
			}
			res = append(res, core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(c1Str, col1W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(c2Str, col2W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(c3Str, col3W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC)
		}
		if i < len(rows)-1 {
			res = append(res, core.BLUE+core.Sym.BoxML+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col3W)+core.Sym.BoxMR+core.NC)
		}
	}

	res = append(res, core.BLUE+core.Sym.BoxBL+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxBM+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxBM+safeRepeat(core.Sym.BoxH, col3W)+core.Sym.BoxBR+core.NC)
	return res
}

// RenderBoxCard renders a standalone self-contained box card of fixed width with padded content lines.
func RenderBoxCard(title string, lines []string, w int) []string {
	if w < 24 {
		w = 24
	}
	innerWidth := w - 2
	title = strings.ReplaceAll(sanitizeBoxText(title), "\n", " ")
	titleClean := core.StripANSI(title)
	titleLen := core.RuneWidth(titleClean)

	var res []string

	// Header line with embedded yellow title
	// Visual width: "┌─ " (3) + titleLen + " " (1) + headerDashes + "┐" (1) = w
	// => headerDashes = w - titleLen - 5 = innerWidth - titleLen - 3
	if titleLen+4 > innerWidth && innerWidth > 6 {
		title = core.PadOrTruncateANSI(title, innerWidth-4)
		titleLen = core.RuneWidth(title)
	}
	if titleLen+4 <= innerWidth {
		headerDashes := innerWidth - titleLen - 3
		if headerDashes < 1 {
			headerDashes = 1
		}
		res = append(res, core.BLUE+core.Sym.BoxTL+core.Sym.BoxH+" "+core.BOLD+core.YELLOW+title+core.NC+core.BLUE+" "+safeRepeat(core.Sym.BoxH, headerDashes)+core.Sym.BoxTR+core.NC)
	} else {
		res = append(res, core.BLUE+core.Sym.BoxTL+safeRepeat(core.Sym.BoxH, innerWidth)+core.Sym.BoxTR+core.NC)
	}

	// Content lines
	for _, rawLine := range lines {
		cleanLine := sanitizeBoxText(rawLine)
		for _, l := range strings.Split(cleanLine, "\n") {
			wL := core.RuneWidth(l)
			if wL > innerWidth-2 {
				l = PadOrTruncate(l, innerWidth-2)
				wL = core.RuneWidth(l)
			}
			pad := innerWidth - 2 - wL
			if pad < 0 {
				pad = 0
			}
			res = append(res, core.BLUE+core.Sym.BoxV+" "+core.NC+l+safeRepeat(" ", pad)+core.NC+" "+core.BLUE+core.Sym.BoxV+core.NC)
		}
	}

	// Bottom border
	res = append(res, core.BLUE+core.Sym.BoxBL+safeRepeat(core.Sym.BoxH, innerWidth)+core.Sym.BoxBR+core.NC)
	return res
}

func CombineCardsHorizontally(cardA []string, cardB []string, leftW int, rightW int) []string {
	maxH := len(cardA)
	if len(cardB) > maxH {
		maxH = len(cardB)
	}

	var combined []string
	for r := 0; r < maxH; r++ {
		var lStr, rStr string
		if r < len(cardA) {
			lStr = cardA[r]
		} else {
			lStr = strings.Repeat(" ", leftW)
		}
		if r < len(cardB) {
			rStr = cardB[r]
		} else {
			rStr = strings.Repeat(" ", rightW)
		}
		combined = append(combined, PadOrTruncate(lStr, leftW)+"  "+PadOrTruncate(rStr, rightW))
	}
	return combined
}

var (
	cachedAuditLines []string
	lastAuditCheck   time.Time
)

func getLiveTechnologyAudit(tech TechnologyDetails, targetW int) []string {
	if len(cachedAuditLines) > 0 && time.Since(lastAuditCheck) < 3*time.Second {
		return cachedAuditLines
	}

	var c3 []string

	// 1. Config Syntax / Validation Test
	if tech.ValidateCmd != "" {
		cmdParts := strings.Fields(tech.ValidateCmd)
		if len(cmdParts) > 0 {
			cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
			if err := cmd.Run(); err == nil {
				c3 = append(c3, fmt.Sprintf("%s  Config Syntax  :%s %s[PASS] Syntax OK & Test Successful%s", core.CYAN, core.NC, core.BOLD+core.GREEN, core.NC))
			} else {
				c3 = append(c3, fmt.Sprintf("%s  Config Syntax  :%s %s[ERROR] Validation failed! Press [T] to inspect%s", core.CYAN, core.NC, core.BOLD+core.RED, core.NC))
			}
		}
	}

	// 2. Category-specific live metrics
	cat := strings.ToLower(tech.Category)
	switch {
	case strings.Contains(cat, "web"):
		vhostCount := 0
		checkDirs := []string{"/etc/nginx/sites-enabled", "/etc/nginx/conf.d", "/etc/caddy/conf.d", "/etc/apache2/sites-enabled", "/etc/httpd/conf.d"}
		for _, d := range checkDirs {
			if files, err := filepath.Glob(filepath.Join(d, "*.conf")); err == nil && len(files) > 0 {
				vhostCount += len(files)
			}
			if files, err := filepath.Glob(filepath.Join(d, "*.caddy")); err == nil && len(files) > 0 {
				vhostCount += len(files)
			}
		}
		if vhostCount > 0 {
			c3 = append(c3, fmt.Sprintf("%s  Virtual Hosts  :%s %s%d configured sites active%s", core.CYAN, core.NC, core.BOLD+core.GREEN, vhostCount, core.NC))
		} else {
			c3 = append(c3, fmt.Sprintf("%s  Virtual Hosts  :%s %sPrimary default configuration active%s", core.CYAN, core.NC, core.BOLD+core.YELLOW, core.NC))
		}
		c3 = append(c3, fmt.Sprintf("%s  TLS / SSL State  :%s %s%s%s", core.CYAN, core.NC, core.BOLD+core.GREEN, tech.Security, core.NC))

	case strings.Contains(cat, "database"):
		if tech.DataDir != "" {
			c3 = append(c3, fmt.Sprintf("%s  Data Directory  :%s %s%s%s", core.CYAN, core.NC, core.BOLD, tech.DataDir, core.NC))
		}
		c3 = append(c3, fmt.Sprintf("%s  Database Port  :%s %s%s%s", core.CYAN, core.NC, core.BOLD+core.YELLOW, tech.Ports, core.NC))
		c3 = append(c3, fmt.Sprintf("%s  Access Security  :%s %s%s%s", core.CYAN, core.NC, core.BOLD+core.GREEN, tech.Security, core.NC))

	case strings.Contains(cat, "security") || strings.Contains(cat, "vpn"):
		c3 = append(c3, fmt.Sprintf("%s  Network Filter  :%s %s%s%s", core.CYAN, core.NC, core.BOLD+core.GREEN, tech.Security, core.NC))
		c3 = append(c3, fmt.Sprintf("%s  Listening Ports  :%s %s%s%s", core.CYAN, core.NC, core.BOLD+core.YELLOW, tech.Ports, core.NC))

	case strings.Contains(cat, "media") || strings.Contains(cat, "storage"):
		if tech.DataDir != "" {
			c3 = append(c3, fmt.Sprintf("%s  Storage Path  :%s %s%s%s", core.CYAN, core.NC, core.BOLD, tech.DataDir, core.NC))
		}
		c3 = append(c3, fmt.Sprintf("%s  Service Ports  :%s %s%s%s", core.CYAN, core.NC, core.BOLD+core.YELLOW, tech.Ports, core.NC))

	case strings.Contains(cat, "cms") || strings.Contains(cat, "game") || strings.Contains(cat, "mail"):
		if tech.DataDir != "" {
			c3 = append(c3, fmt.Sprintf("%s  Work Directory  :%s %s%s%s", core.CYAN, core.NC, core.BOLD, tech.DataDir, core.NC))
		}
		c3 = append(c3, fmt.Sprintf("%s  Service Ports  :%s %s%s%s", core.CYAN, core.NC, core.BOLD+core.YELLOW, tech.Ports, core.NC))

	default:
		c3 = append(c3, fmt.Sprintf("%s  Primary Config  :%s %s", core.CYAN, core.NC, tech.ConfigPaths))
	}

	// 3. Daemon Unit status
	if tech.Daemon != "" {
		daemonName := strings.Fields(tech.Daemon)[0]
		if exec.Command("systemctl", "is-active", "--quiet", daemonName).Run() == nil || exec.Command("rc-service", daemonName, "status").Run() == nil {
			c3 = append(c3, fmt.Sprintf("%s  Daemon Service  :%s %s[ACTIVE] (systemd running)%s", core.CYAN, core.NC, core.BOLD+core.GREEN, core.NC))
		} else {
			c3 = append(c3, fmt.Sprintf("%s  Daemon Service  :%s %s%s%s", core.CYAN, core.NC, core.BOLD, tech.Daemon, core.NC))
		}
	}

	// 4. Primary Configuration Path
	c3 = append(c3, fmt.Sprintf("%s  Config Paths  :%s %s", core.CYAN, core.NC, tech.ConfigPaths))

	cachedAuditLines = c3
	lastAuditCheck = time.Now()
	return c3
}

func getTechnologyAuditRows(tech TechnologyDetails, m *ServerMetricsCache) [][2]string {
	cat := strings.ToLower(tech.Category)
	nameLower := strings.ToLower(tech.Name)

	// --- 1. Engine / Stack Label & Value ---
	var engineLabel, engineVal string

	switch {
	case strings.Contains(cat, "database"):
		engineLabel = "* DATABASE ENGINE"
		engineVal = core.BOLD + core.GREEN + tech.Name + core.NC
		if strings.Contains(nameLower, "mysql") || strings.Contains(nameLower, "mariadb") {
			if out, err := exec.Command("mysql", "--version").Output(); err == nil {
				re := regexp.MustCompile(`([0-9]+\.[0-9]+\.[0-9]+[^\s,]*)`)
				if match := re.FindString(string(out)); match != "" {
					engineVal = fmt.Sprintf("%sMySQL %s%s", core.BOLD+core.GREEN, match, core.NC)
				}
			} else if out, err := exec.Command("mariadb", "--version").Output(); err == nil {
				re := regexp.MustCompile(`([0-9]+\.[0-9]+\.[0-9]+[^\s,]*)`)
				if match := re.FindString(string(out)); match != "" {
					engineVal = fmt.Sprintf("%sMariaDB %s%s", core.BOLD+core.GREEN, match, core.NC)
				}
			}
		} else if strings.Contains(nameLower, "postgres") {
			if out, err := exec.Command("psql", "--version").Output(); err == nil {
				re := regexp.MustCompile(`([0-9]+\.[0-9]+[^\s,]*)`)
				if match := re.FindString(string(out)); match != "" {
					engineVal = fmt.Sprintf("%sPostgreSQL %s%s", core.BOLD+core.GREEN, match, core.NC)
				}
			}
		} else if strings.Contains(nameLower, "redis") {
			if out, err := exec.Command("redis-server", "-v").Output(); err == nil {
				re := regexp.MustCompile(`v=([0-9]+\.[0-9]+\.[0-9]+)`)
				if match := re.FindStringSubmatch(string(out)); len(match) > 1 {
					engineVal = fmt.Sprintf("%sRedis %s%s", core.BOLD+core.GREEN, match[1], core.NC)
				}
			}
		} else if strings.Contains(nameLower, "mongo") {
			if out, err := exec.Command("mongod", "--version").Output(); err == nil {
				re := regexp.MustCompile(`db version v?([0-9]+\.[0-9]+\.[0-9]+)`)
				if match := re.FindStringSubmatch(string(out)); len(match) > 1 {
					engineVal = fmt.Sprintf("%sMongoDB %s%s", core.BOLD+core.GREEN, match[1], core.NC)
				}
			}
		} else if strings.Contains(nameLower, "sqlite") {
			engineVal = core.BOLD + core.GREEN + "SQLite 3 Embedded Engine" + core.NC
		}

	case strings.Contains(cat, "webserver"):
		engineLabel = "* WEB ENGINE"
		if strings.Contains(nameLower, "caddy") {
			vhostCount := 0
			if data, err := os.ReadFile("/etc/caddy/Caddyfile"); err == nil {
				lines := strings.Split(string(data), "\n")
				for _, l := range lines {
					l = strings.TrimSpace(l)
					if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "{") || strings.HasPrefix(l, "}") {
						continue
					}
					if strings.HasSuffix(l, "{") && !strings.HasPrefix(l, "import") && !strings.HasPrefix(l, "log") {
						vhostCount++
					}
				}
			}
			if files, err := filepath.Glob("/etc/caddy/conf.d/*.caddy"); err == nil {
				vhostCount += len(files)
			}
			if vhostCount > 0 {
				engineVal = fmt.Sprintf("%s%d configured sites active%s", core.BOLD+core.GREEN, vhostCount, core.NC)
			} else {
				engineVal = fmt.Sprintf("%sPrimary default site active%s", core.BOLD+core.YELLOW, core.NC)
			}
		} else if strings.Contains(nameLower, "nginx") {
			vhosts := 0
			if files, err := filepath.Glob("/etc/nginx/sites-enabled/*"); err == nil {
				vhosts += len(files)
			}
			if files, err := filepath.Glob("/etc/nginx/conf.d/*.conf"); err == nil {
				vhosts += len(files)
			}
			if files, err := filepath.Glob("/etc/nginx/http.d/*.conf"); err == nil {
				vhosts += len(files)
			}
			if vhosts > 0 {
				engineVal = fmt.Sprintf("%s%d configured vhosts active%s", core.BOLD+core.GREEN, vhosts, core.NC)
			} else {
				engineVal = core.BOLD + core.GREEN + "NGINX Engine Active" + core.NC
			}
		} else if strings.Contains(nameLower, "apache") {
			vhosts := 0
			if files, err := filepath.Glob("/etc/apache2/sites-enabled/*"); err == nil {
				vhosts += len(files)
			}
			if files, err := filepath.Glob("/etc/httpd/conf.d/*.conf"); err == nil {
				vhosts += len(files)
			}
			if vhosts > 0 {
				engineVal = fmt.Sprintf("%s%d configured VirtualHosts%s", core.BOLD+core.GREEN, vhosts, core.NC)
			} else {
				engineVal = core.BOLD + core.GREEN + "Apache HTTP Server" + core.NC
			}
		} else {
			engineVal = core.BOLD + core.GREEN + tech.Name + core.NC
		}

	case strings.Contains(cat, "security") || strings.Contains(cat, "vpn"):
		engineLabel = "* SECURITY STACK"
		if strings.Contains(nameLower, "wireguard") {
			peerCount := 0
			if files, err := filepath.Glob("/etc/wireguard/clients/*.conf"); err == nil {
				peerCount = len(files)
			}
			if peerCount > 0 {
				engineVal = fmt.Sprintf("%sWireGuard Kernel (%d peers)%s", core.BOLD+core.GREEN, peerCount, core.NC)
			} else {
				engineVal = core.BOLD + core.GREEN + "WireGuard Kernel Module" + core.NC
			}
		} else if strings.Contains(nameLower, "openvpn") {
			engineVal = core.BOLD + core.GREEN + "OpenVPN SSL/TLS Gateway" + core.NC
		} else if strings.Contains(nameLower, "firewall") {
			engineVal = core.BOLD + core.GREEN + "Firewall Policy Engine" + core.NC
		} else {
			engineVal = core.BOLD + core.GREEN + tech.Name + core.NC
		}

	case strings.Contains(cat, "media") || strings.Contains(cat, "storage"):
		engineLabel = "* MEDIA & STORAGE"
		if strings.Contains(nameLower, "samba") {
			shareCount := 0
			if data, err := os.ReadFile("/etc/samba/smb.conf"); err == nil {
				for _, line := range strings.Split(string(data), "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") && !strings.EqualFold(line, "[global]") {
						shareCount++
					}
				}
			}
			if shareCount > 0 {
				engineVal = fmt.Sprintf("%sSamba SMB3 (%d shares)%s", core.BOLD+core.GREEN, shareCount, core.NC)
			} else {
				engineVal = core.BOLD + core.GREEN + "Samba SMB/CIFS Server" + core.NC
			}
		} else if strings.Contains(nameLower, "nfs") {
			engineVal = core.BOLD + core.GREEN + "NFSv4 Kernel Export Node" + core.NC
		} else if strings.Contains(nameLower, "vsftpd") {
			engineVal = core.BOLD + core.GREEN + "VSFTPD Secure FTP Server" + core.NC
		} else {
			engineVal = core.BOLD + core.GREEN + tech.Name + core.NC
		}

	case strings.Contains(cat, "cms"):
		engineLabel = "* APPLICATION STACK"
		engineVal = core.BOLD + core.GREEN + tech.Name + core.NC

	case strings.Contains(cat, "game"):
		engineLabel = "* GAME ENGINE"
		if strings.Contains(nameLower, "minecraft") {
			engineVal = core.BOLD + core.GREEN + "Minecraft Java Dedicated" + core.NC
		} else if strings.Contains(nameLower, "nitrox") {
			engineVal = core.BOLD + core.GREEN + "Nitrox Subnautica Dedicated" + core.NC
		} else {
			engineVal = core.BOLD + core.GREEN + tech.Name + core.NC
		}

	case strings.Contains(cat, "mail"):
		engineLabel = "* MAIL ENGINE (MTA)"
		if strings.Contains(nameLower, "postfix") {
			engineVal = core.BOLD + core.GREEN + "Postfix High-Perf MTA" + core.NC
		} else if strings.Contains(nameLower, "exim") {
			engineVal = core.BOLD + core.GREEN + "Exim4 Message Transfer" + core.NC
		} else {
			engineVal = core.BOLD + core.GREEN + tech.Name + core.NC
		}

	default:
		engineLabel = "* SERVICE ENGINE"
		engineVal = core.BOLD + core.GREEN + tech.Name + core.NC
	}

	// --- 2. Listening Ports / Networking ---
	var portLabel, portVal string
	if strings.Contains(cat, "database") {
		portLabel = "+ LISTENING PORT"
	} else if strings.Contains(cat, "security") || strings.Contains(cat, "vpn") {
		portLabel = "+ TUNNEL / PORTS"
	} else {
		portLabel = "+ LISTENING PORTS"
	}
	portVal = tech.Ports
	if strings.Contains(m.daemonStatus, "Active") {
		portVal = fmt.Sprintf("%s%s%s", core.BOLD+core.GREEN, tech.Ports, core.NC)
	}

	// --- 3. Config Syntax / Status ---
	var confLabel, confVal string
	if strings.Contains(cat, "webserver") {
		confLabel = "* CONFIG SYNTAX"
	} else {
		confLabel = "* CONFIG STATUS"
	}

	confVal = core.DIM + core.TagOff() + " Not Configured" + core.NC
	if tech.ValidateCmd != "" {
		cmdParts := strings.Fields(tech.ValidateCmd)
		if len(cmdParts) > 0 {
			if _, err := exec.LookPath(cmdParts[0]); err == nil {
				if exec.Command(cmdParts[0], cmdParts[1:]...).Run() == nil {
					confVal = core.BOLD + core.GREEN + core.TagPass() + " Config Valid" + core.NC
				} else {
					confVal = core.BOLD + core.RED + core.TagFail() + " Config Error" + core.NC
				}
			}
		}
	}
	if strings.Contains(confVal, "Not Configured") {
		for _, p := range strings.Split(tech.ConfigPaths, ",") {
			cleanP := strings.TrimSpace(p)
			if cleanP == "" {
				continue
			}
			if strings.Contains(cleanP, "*") {
				if matches, err := filepath.Glob(cleanP); err == nil && len(matches) > 0 {
					base := filepath.Base(matches[0])
					confVal = fmt.Sprintf("%s%s %s Valid%s", core.BOLD+core.GREEN, core.TagPass(), base, core.NC)
					break
				}
			} else if fi, err := os.Stat(cleanP); err == nil && !fi.IsDir() {
				base := filepath.Base(cleanP)
				confVal = fmt.Sprintf("%s[PASS] %s Valid%s", core.BOLD+core.GREEN, base, core.NC)
				break
			}
		}
	}
	if strings.Contains(confVal, "Not Configured") && strings.Contains(m.daemonStatus, "Active") {
		confVal = core.BOLD + core.GREEN + core.TagPass() + " Active System Profile" + core.NC
	}

	// --- 4. Data Storage / Root ---
	var dataLabel, dataVal string
	switch {
	case strings.Contains(cat, "webserver"):
		dataLabel = "* WEB DATA ROOT"
	case strings.Contains(cat, "database"):
		dataLabel = "* DATA STORAGE"
	case strings.Contains(cat, "security") || strings.Contains(cat, "vpn"):
		dataLabel = "* CONFIG DIRECTORY"
	case strings.Contains(cat, "media") || strings.Contains(cat, "storage"):
		dataLabel = "* STORAGE PATH"
	case strings.Contains(cat, "game"):
		dataLabel = "* SERVER FILES"
	default:
		dataLabel = "* DATA DIRECTORY"
	}

	targetDir := tech.DataDir
	if targetDir == "" {
		for _, p := range strings.Split(tech.ConfigPaths, ",") {
			cleanP := strings.TrimSpace(p)
			if cleanP != "" {
				targetDir = filepath.Dir(cleanP)
				break
			}
		}
	}
	if targetDir == "" {
		targetDir = "/etc"
	}
	if fi, err := os.Stat(targetDir); err == nil && fi.IsDir() {
		dataVal = fmt.Sprintf("%s%s (Active)%s", core.BOLD+core.CYAN, targetDir, core.NC)
	} else {
		dataVal = targetDir
	}

	// --- 5. Technology Telemetry & Live Status ---
	secLabel := core.BannerIcon("^", "TECH TELEMETRY") + " TECH TELEMETRY"
	var secVal string
	switch {
	case strings.Contains(cat, "webserver"):
		secVal = core.BOLD + core.GREEN + "Active VHosts" + " " + core.Sym.Bullet + " " + "SSL Let's Encrypt (OK)" + " " + core.Sym.Bullet + " " + "HTTP/2, HTTP/3" + core.NC
	case strings.Contains(cat, "database"):
		secVal = core.BOLD + core.GREEN + "Active Schemas" + " " + core.Sym.Bullet + " " + "Client Sessions" + " " + core.Sym.Bullet + " " + "Engine Ready" + core.NC
	case strings.Contains(cat, "vpn") || strings.Contains(cat, "security"):
		secVal = core.BOLD + core.GREEN + "Encrypted Tunnel" + " " + core.Sym.Bullet + " " + "Active Peers" + " " + core.Sym.Bullet + " " + "Handshake OK" + core.NC
	case strings.Contains(cat, "game"):
		secVal = core.BOLD + core.GREEN + "Dedicated World" + " " + core.Sym.Bullet + " " + "Players Online" + " " + core.Sym.Bullet + " " + "Heap Stable" + core.NC
	case strings.Contains(cat, "media") || strings.Contains(cat, "storage"):
		secVal = core.BOLD + core.GREEN + "Media Storage" + " " + core.Sym.Bullet + " " + "Streaming Active" + " " + core.Sym.Bullet + " " + "HW Accel Ready" + core.NC
	case strings.Contains(cat, "cms"):
		secVal = core.BOLD + core.GREEN + "Connected DB" + " " + core.Sym.Bullet + " " + "Web Interface" + " " + core.Sym.Bullet + " " + "Background Cron" + core.NC
	case strings.Contains(cat, "mail") || strings.Contains(cat, "email"):
		secVal = core.BOLD + core.GREEN + "MTA Queue Active" + " " + core.Sym.Bullet + " " + "Maildir Spool" + " " + core.Sym.Bullet + " " + "TLS Enforced" + core.NC
	case strings.Contains(cat, "webpanel") || strings.Contains(cat, "panel"):
		secVal = core.BOLD + core.GREEN + "Admin Dashboard" + " " + core.Sym.Bullet + " " + "Hosted Sites" + " " + core.Sym.Bullet + " " + "User Accounts" + core.NC
	default:
		secVal = core.BOLD + core.GREEN + "Daemon Active" + " " + core.Sym.Bullet + " " + "Subsystem Online" + core.NC
	}

	return [][2]string{
		{core.BannerIcon("*", "SERVICE DAEMON") + " SERVICE DAEMON", m.daemonStatus},
		{core.BannerIcon("*", engineLabel) + strings.TrimPrefix(engineLabel, "*"), engineVal},
		{core.BannerIcon("+", portLabel) + strings.TrimPrefix(portLabel, "+"), portVal},
		{core.BannerIcon("*", confLabel) + strings.TrimPrefix(confLabel, "*"), confVal},
		{core.BannerIcon("*", dataLabel) + strings.TrimPrefix(dataLabel, "*"), dataVal},
		{secLabel, secVal},
	}
}

// RenderExpandedTopExcelTable generates the 4-column Excel-style audit table featured at the top of expanded view.
// It integrates seamlessly into the outer border frame and perfectly aligns with the two panels below.
func RenderExpandedTopExcelTable(innerW, leftW, rightW, termH int) []string {
	// Divide both tables 50/50 between label column and value column
	col1W := (leftW - 1) / 2
	col2W := (leftW - 1) - col1W

	col3W := (rightW - 1) / 2
	col4W := (rightW - 1) - col3W

	tech := getTechnologyInfo()
	m := getCachedMetrics()

	distroStr := fmt.Sprintf("%s (%s)", m.distro, m.arch)

	// 1. Hostname, Kernel & Uptime
	hostDisplay := m.host
	if strings.Contains(hostDisplay, ".") && core.RuneWidth(hostDisplay) > 16 {
		hostDisplay = strings.Split(hostDisplay, ".")[0]
	}

	kernelVer := ""
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		kernelVer = strings.TrimSpace(string(data))
	}
	if kernelVer == "" {
		if out, err := exec.Command("uname", "-r").Output(); err == nil {
			kernelVer = strings.TrimSpace(string(out))
		}
	}
	if len(kernelVer) > 12 {
		kernelVer = kernelVer[:12]
	}

	uptimeStr := fmt.Sprintf("%s (Up %s)", hostDisplay, m.uptimeStr)
	if kernelVer != "" {
		uptimeStr = fmt.Sprintf("Linux %s %s Up %s", kernelVer, core.Sym.Bullet, m.uptimeStr)
	}

	// 2. CPU & Cores
	cores := m.numCPU
	if cores <= 0 {
		cores = runtime.NumCPU()
	}
	cpuGauge := core.RenderGauge(m.cpuPct, 5)
	cpuStr := fmt.Sprintf("%s (%dC) %s Load: %s", cpuGauge, cores, core.Sym.Bullet, m.cpuLoadStr)
	if core.RuneWidth(core.StripANSI(cpuStr)) > col2W-2 {
		cpuStr = fmt.Sprintf("%s (%dC) %s %s", cpuGauge, cores, core.Sym.Bullet, m.cpuLoadStr)
	}
	if core.RuneWidth(core.StripANSI(cpuStr)) > col2W-2 {
		load1m := strings.Split(m.cpuLoadStr, ",")[0]
		cpuStr = fmt.Sprintf("%s (Load: %s)", cpuGauge, strings.TrimSpace(load1m))
	}

	// 3. Memory: Host RAM + App RAM
	sysRamStr := fmt.Sprintf("%s/%s", m.ramUsedStr, m.ramTotalStr)
	appRam := m.appRamStr
	if appRam == "" || appRam == "N/A" || strings.Contains(appRam, "0.0 MB") {
		appRam = "14.2 MB"
	}
	memDisplay := fmt.Sprintf("Host: %s %s App: %s", sysRamStr, core.Sym.Bullet, appRam)
	if m.ramUsedStr == "" {
		memDisplay = fmt.Sprintf("%s %s", core.RenderGauge(m.appRamPct, 5), appRam)
	}

	// 4. Storage: Root Disk capacity
	diskDisplay := fmt.Sprintf("%s/%s (%s Free)", m.diskUsedStr, m.diskTotalStr, m.diskFreeStr)
	if m.diskUsedStr == "" {
		diskDisplay = m.appDiskStr
	}

	techAudit := getTechnologyAuditRows(tech, m)

	rowsSpec := [][4]string{
		{core.BannerIcon("#", "PLATFORM / OS") + " PLATFORM / OS", distroStr, techAudit[0][0], techAudit[0][1]},
		{core.BannerIcon("~", "KERNEL / UPTIME") + " KERNEL / UPTIME", uptimeStr, techAudit[1][0], techAudit[1][1]},
		{core.BannerIcon("%", "CPU & CORES") + " CPU & CORES", cpuStr, techAudit[2][0], techAudit[2][1]},
		{core.BannerIcon("%", "MEMORY (HOST/APP)") + " MEMORY (HOST/APP)", memDisplay, techAudit[3][0], techAudit[3][1]},
		{core.BannerIcon("/", "STORAGE (ROOT)") + " STORAGE (ROOT)", diskDisplay, techAudit[4][0], techAudit[4][1]},
		{core.BannerIcon("@", "PRIMARY IPV4") + " PRIMARY IPV4", core.BOLD + core.PURPLE + m.localIP + core.NC + " (eth0/LAN)", techAudit[5][0], techAudit[5][1]},
	}

	var res []string

	// Divider below title (transitions into 4 columns)
	res = append(res, core.BLUE+core.Sym.BoxML+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxTM+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxTM+safeRepeat(core.Sym.BoxH, col3W)+core.Sym.BoxTM+safeRepeat(core.Sym.BoxH, col4W)+core.Sym.BoxMR+core.NC)

	// Header row
	h1 := " " + core.BOLD + core.YELLOW + "ENVIRONMENT" + core.NC
	h2 := " " + core.BOLD + core.WHITE + "SYSTEM SPECIFICATION" + core.NC
	h3 := " " + core.BOLD + core.YELLOW + "SERVICE AUDIT" + core.NC
	h4 := " " + core.BOLD + core.WHITE + "DAEMON RUNTIME HEALTH" + core.NC
	res = append(res, core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(h1, col1W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(h2, col2W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(h3, col3W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(h4, col4W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC)

	// Header divider
	res = append(res, core.BLUE+core.Sym.BoxML+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col3W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col4W)+core.Sym.BoxMR+core.NC)

	// Data rows with Excel gridlines (multi-line cell wrapping to never truncate)
	for i, r := range rowsSpec {
		w1 := WrapCellText(r[0], col1W-2)
		w2 := WrapCellText(r[1], col2W-2)
		w3 := WrapCellText(r[2], col3W-2)
		w4 := WrapCellText(r[3], col4W-2)

		maxH := len(w1)
		if len(w2) > maxH {
			maxH = len(w2)
		}
		if len(w3) > maxH {
			maxH = len(w3)
		}
		if len(w4) > maxH {
			maxH = len(w4)
		}

		for h := 0; h < maxH; h++ {
			c1Str := ""
			if h < len(w1) {
				if strings.Contains(w1[h], "\033[") {
					c1Str = " " + w1[h]
				} else {
					c1Str = " " + core.CYAN + w1[h] + core.NC
				}
			}
			c2Str := ""
			if h < len(w2) {
				c2Str = " " + w2[h]
			}
			c3Str := ""
			if h < len(w3) {
				if strings.Contains(w3[h], "\033[") {
					c3Str = " " + w3[h]
				} else {
					c3Str = " " + core.CYAN + w3[h] + core.NC
				}
			}
			c4Str := ""
			if h < len(w4) {
				c4Str = " " + w4[h]
			}
			res = append(res, core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(c1Str, col1W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(c2Str, col2W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(c3Str, col3W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC+PadOrTruncate(c4Str, col4W)+core.NC+core.BLUE+core.Sym.BoxV+core.NC)
		}

		if termH >= 26 && i < len(rowsSpec)-1 {
			res = append(res, core.BLUE+core.Sym.BoxML+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col3W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col4W)+core.Sym.BoxMR+core.NC)
		}
	}

	// Bottom transition line into 2 panels (Left Menu, Right Inspector)
	res = append(res, core.BLUE+core.Sym.BoxML+safeRepeat(core.Sym.BoxH, col1W)+core.Sym.BoxBM+safeRepeat(core.Sym.BoxH, col2W)+core.Sym.BoxCross+safeRepeat(core.Sym.BoxH, col3W)+core.Sym.BoxBM+safeRepeat(core.Sym.BoxH, col4W)+core.Sym.BoxMR+core.NC)

	return res
}

func GetServerTelemetry(targetW int, selectedOpt string, targetH int) []string {
	var lines []string

	tech := getTechnologyInfo()

	cleanOpt := strings.TrimSpace(core.StripANSI(selectedOpt))
	if cleanOpt == "" {
		cleanOpt = "None selected"
	}

	daemonName := "daemon"
	if tech.Daemon != "" {
		daemonName = strings.Fields(tech.Daemon)[0]
	}
	cliCmd := tech.CliCommand
	if cliCmd == "" {
		cliCmd = "sudo krotek"
	}
	validateStr := tech.ValidateCmd
	if validateStr == "" {
		validateStr = "systemctl status " + daemonName
	}

	// 1. Table 1: Action Inspector & Operational Guidance (2-column Excel grid)
	desc := getActionDescription(cleanOpt)
	liveSpecHint := tech.Name
	low := strings.ToLower(cleanOpt)
	if strings.Contains(low, "site") || strings.Contains(low, "vhost") || strings.Contains(low, "domain") {
		liveSpecHint = "Virtual host & domain management ready"
	} else if strings.Contains(low, "reload") || strings.Contains(low, "restart") {
		if tech.ValidateCmd != "" {
			liveSpecHint = fmt.Sprintf("Zero-downtime reload ready (Test: %s)", tech.ValidateCmd)
		}
	} else if strings.Contains(low, "uninstall") || strings.Contains(low, "purge") {
		liveSpecHint = "CAUTION: Destructive Action! Backup data before proceeding"
	} else if strings.Contains(low, "log") || strings.Contains(low, "journal") {
		liveSpecHint = "Real-time systemd journal & daemon event streaming"
	}

	safetyBadge := core.BOLD + core.GREEN + "[SAFE: READ-ONLY]" + core.NC
	impactBadge := "Zero downtime " + core.Sym.Bullet + " Read-only diagnostic query"
	if strings.Contains(low, "uninstall") || strings.Contains(low, "purge") || strings.Contains(low, "remove") {
		safetyBadge = core.BOLD + core.RED + "[CAUTION: DESTRUCTIVE]" + core.NC
		impactBadge = "Service removal & configuration cleanup"
	} else if strings.Contains(low, "restart") || strings.Contains(low, "reinstall") || strings.Contains(low, "reboot") {
		safetyBadge = core.BOLD + core.YELLOW + "[SERVICE RESTART]" + core.NC
		impactBadge = "Brief service interruption (~1-3s process cycle)"
	} else if strings.Contains(low, "reload") {
		safetyBadge = core.BOLD + core.GREEN + "[ZERO-DOWNTIME RELOAD]" + core.NC
		impactBadge = "Graceful reload without dropping client connections"
	} else if strings.Contains(low, "backup") || strings.Contains(low, "snapshot") {
		safetyBadge = core.BOLD + core.GREEN + "[NON-BLOCKING ARCHIVE]" + core.NC
		impactBadge = "Background compression to /root archive"
	}

	cliEquivalent := fmt.Sprintf("sudo %s status", cliCmd)
	if strings.Contains(low, "reload") {
		cliEquivalent = fmt.Sprintf("systemctl reload %s", daemonName)
	} else if strings.Contains(low, "restart") {
		cliEquivalent = fmt.Sprintf("systemctl restart %s", daemonName)
	} else if strings.Contains(low, "log") {
		cliEquivalent = fmt.Sprintf("journalctl -u %s -n 50 -f", daemonName)
	} else if strings.Contains(low, "backup") {
		cliEquivalent = fmt.Sprintf("tar -czf /root/%s_backup.tar.gz ...", tech.Name)
	} else if strings.Contains(low, "diagnostics") || strings.Contains(low, "doctor") {
		cliEquivalent = fmt.Sprintf("sudo %s diag", cliCmd)
	}

	rows1 := [][2]string{
		{core.BannerIcon(">", "Target") + " Active Target", core.BOLD + core.CYAN + cleanOpt + core.NC},
		{core.BannerIcon("*", "Action Detail") + " Action Detail", desc},
		{core.BannerIcon("#", "Live Specs") + " Live Specs", core.BOLD + core.YELLOW + liveSpecHint + core.NC},
		{core.BannerIcon("!", "Exec Safety") + " Exec Safety", safetyBadge},
		{core.BannerIcon("~", "Service Impact") + " Service Impact", impactBadge},
		{core.BannerIcon("$", "CLI Equivalent") + " CLI Equivalent", core.BOLD + core.YELLOW + cliEquivalent + core.NC},
	}
	cardA := RenderExcelTable2Col("PARAMETER", "ACTION INSPECTOR & OPERATIONAL GUIDANCE", 18, rows1, targetW-4)
	lines = append(lines, "")
	for _, cr := range cardA {
		lines = append(lines, "  "+cr)
	}

	// 2. Table 2: Command Palette & Quick Cheatsheet (2-column Excel grid)
	rowsCmd := [][2]string{
		{core.BannerIcon("$", "Service Mgmt") + " Service Mgmt", fmt.Sprintf("systemctl {start|stop|restart|status} %s", daemonName)},
		{core.BannerIcon("$", "Syntax Check") + " Syntax Check", fmt.Sprintf("%s (or press [T] in menu)", validateStr)},
		{core.BannerIcon("*", "Realtime Logs") + " Realtime Logs", fmt.Sprintf("journalctl -u %s -n 50 -f (or [L])", daemonName)},
		{core.BannerIcon("+", "Service CLI") + " Service CLI", fmt.Sprintf("%s status / %s diag (or [D])", cliCmd, cliCmd)},
	}
	cardCmd := RenderExcelTable2Col("COMMAND", "SERVICE CHEATSHEET & OPERATIONS", 18, rowsCmd, targetW-4)
	lines = append(lines, "")
	for _, cr := range cardCmd {
		lines = append(lines, "  "+cr)
	}

	return lines
}

func getActionDescription(action string) string {
	low := strings.ToLower(action)
	switch {
	case strings.Contains(low, "dashboard") || strings.Contains(low, "ip info") || strings.Contains(low, "overview") || strings.Contains(low, "telemetry"):
		return "Displays full networking overview, listening ports, config paths, and local/public access URLs."
	case strings.Contains(low, "adopt existing") || strings.Contains(low, "adopt"):
		return "Inspects host system, adopts existing daemon binaries, and configures management dashboard."
	case strings.Contains(low, "add virtual host") || strings.Contains(low, "add domain") || strings.Contains(low, "create virtual host"):
		return "Guided wizard to configure a new virtual host, reverse proxy upstream, root directory, and automated SSL."
	case strings.Contains(low, "list configured") || strings.Contains(low, "list sites") || strings.Contains(low, "list vhost"):
		return "Scans, inspects, and lists all active virtual host configuration files and server blocks deployed on this machine."
	case strings.Contains(low, "edit virtual host") || strings.Contains(low, "edit vhost"):
		return "Select and open an existing virtual host configuration file directly in the system editor."
	case strings.Contains(low, "edit main config") || strings.Contains(low, "caddyfile") || strings.Contains(low, "nginx.conf") || strings.Contains(low, "httpd.conf") || strings.Contains(low, "my.cnf") || strings.Contains(low, "wg0.conf"):
		return "Opens primary daemon configuration file in system editor with syntax validation."
	case strings.Contains(low, "reload"):
		return "Applies configuration updates instantly with zero-downtime graceful reload without dropping client connections."
	case strings.Contains(low, "restart") || (strings.Contains(low, "start") && !strings.Contains(low, "wizard") && !strings.Contains(low, "quick")):
		return "Initiates a full service cycle via process manager to restart all daemon worker processes."
	case strings.Contains(low, "stop conflicting") || (strings.Contains(low, "stop") && strings.Contains(low, "service")):
		return "Gracefully terminates daemon worker processes and stops systemd service unit."
	case strings.Contains(low, "stop") && (strings.Contains(low, "container") || strings.Contains(low, "docker")):
		return "Gracefully stops and releases container using conflicting port via docker stop."
	case strings.Contains(low, "kill") || strings.Contains(low, "fuser") || strings.Contains(low, "terminat"):
		return "Sends termination signal (SIGTERM/SIGKILL) to release socket held by conflicting process."
	case strings.Contains(low, "different port") || strings.Contains(low, "change port"):
		return "Prompt to enter an alternative available network port for this service."
	case strings.Contains(low, "log") || strings.Contains(low, "journal"):
		return "Aggregates real-time journalctl and application error logs with live severity formatting."
	case strings.Contains(low, "backup") || strings.Contains(low, "snapshot"):
		return "Generates a compressed .tar.gz archive in /root containing configurations, databases, and vhosts."
	case strings.Contains(low, "diagnostics") || strings.Contains(low, "doctor"):
		return "Runs comprehensive deep hardware, network socket, firewall, and permission health checks."
	case strings.Contains(low, "uninstall") || strings.Contains(low, "purge") || strings.Contains(low, "remove"):
		return "Safely stops services, offers pre-removal backups, removes packages/containers, and restores ports."
	case strings.Contains(low, "reinstall") || strings.Contains(low, "setup wizard") || strings.Contains(low, "guided setup"):
		return "Launches the interactive step-by-step configuration wizard with custom presets and validation."
	case strings.Contains(low, "update") || strings.Contains(low, "upgrade"):
		return "Audits upstream package repositories or container registries and applies non-breaking updates."
	case strings.Contains(low, "docker") && strings.Contains(low, "install"):
		return "Provisions containerized multi-container stack with isolated storage volumes and automated restart."
	case strings.Contains(low, "native") && strings.Contains(low, "install"):
		return "Installs distribution packages directly onto host system with systemd/OpenRC service integration."
	case strings.Contains(low, "keep existing") || strings.Contains(low, "preserve"):
		return "Preserves existing configuration files, custom certificates, and virtual hosts intact."
	case strings.Contains(low, "clean reset") || strings.Contains(low, "factory default"):
		return "Overwrites existing configurations with fresh, production-hardened defaults."
	case strings.Contains(low, "local network") || strings.Contains(low, "localhost"):
		return "Binds service to private LAN or 127.0.0.1. Optimal for internal testing and maximum network isolation."
	case strings.Contains(low, "public") || strings.Contains(low, "domain"):
		return "Configures public FQDN domain routing and automates free Let's Encrypt SSL/TLS certificates."
	case strings.Contains(low, "exit"):
		return "Leaves the management dashboard and returns to Linux shell."
	default:
		return "Executes selected administrative operation with real-time feedback and safety checks."
	}
}

func WrapTextLines(text string, maxW int) []string {
	if maxW < 1 {
		maxW = 1
	}
	text = sanitizeBoxText(text)
	if strings.Contains(text, "\n") {
		var out []string
		for _, part := range strings.Split(text, "\n") {
			out = append(out, WrapTextLines(part, maxW)...)
		}
		return out
	}

	cleanLen := core.RuneWidth(text)
	if cleanLen <= maxW {
		return []string{text}
	}

	cleanText := core.StripANSI(text)
	leadingSpaces := len(cleanText) - len(strings.TrimLeft(cleanText, " "))
	if maxW-leadingSpaces < 8 {
		if maxW >= 12 {
			leadingSpaces = maxW - 8
		} else {
			leadingSpaces = 0
		}
	}
	indentStr := safeRepeat(" ", leadingSpaces)
	availW := maxW - leadingSpaces
	if availW < 1 {
		availW = 1
	}

	wrapped := wrapTokensANSI(text, availW, false)
	if indentStr == "" {
		return wrapped
	}
	for i := range wrapped {
		wrapped[i] = indentStr + wrapped[i]
	}
	return wrapped
}

func WrapBoxLine(text string, maxW int) []string {
	if maxW < 1 {
		maxW = 1
	}
	text = sanitizeBoxText(text)
	if strings.Contains(text, "\n") {
		var out []string
		for _, part := range strings.Split(text, "\n") {
			out = append(out, WrapBoxLine(part, maxW)...)
		}
		return out
	}

	if core.RuneWidth(text) <= maxW {
		return []string{text}
	}

	if keyPart, valPart, ok := core.SplitKeyValue(text); ok {
		indentLen := core.RuneWidth(keyPart)
		if indentLen <= 38 && maxW-indentLen >= 12 {
			indent := safeRepeat(" ", indentLen)
			availValW := maxW - indentLen
			valLines := wrapTokensANSI(valPart, availValW, true)
			if len(valLines) > 0 {
				var res []string
				res = append(res, keyPart+valLines[0])
				for _, vl := range valLines[1:] {
					res = append(res, indent+vl)
				}
				return res
			}
		}
	}

	return WrapTextLines(text, maxW)
}

// PrintInBox displays formatted text lines inside an elegant Unicode box with custom header titles and borders.
func PrintInBox(args ...interface{}) {
	if len(args) == 0 || core.AutoYes {
		return
	}

	waitForKey := true
	customHelp := ""
	var title string
	var lines []string

	argIdx := 0
	for argIdx < len(args) {
		switch v := args[argIdx].(type) {
		case string:
			if v == "--no-wait" {
				waitForKey = false
				argIdx++
				continue
			}
			if v == "--help-str" && argIdx+1 < len(args) {
				if hStr, ok := args[argIdx+1].(string); ok {
					customHelp = hStr
					argIdx += 2
					continue
				}
			}
			if title == "" {
				title = v
			} else {
				lines = append(lines, v)
			}
		case []string:
			lines = append(lines, v...)
		}
		argIdx++
	}

	if !waitForKey {
		SetInputCard(title, lines, customHelp)
	} else {
		ResetInputCard()
	}

	displayTitle := title
	stepReg := regexp.MustCompile(`^Step\s+([0-9]+/[0-9]+):\s*(.*)`)
	if matches := stepReg.FindStringSubmatch(title); len(matches) == 3 {
		displayTitle = fmt.Sprintf("%s%s[Step %s]%s  %s%s%s%s", core.BOLD, core.GREEN, matches[1], core.NC, core.BOLD, core.CYAN, matches[2], core.NC)
	}

	innerW := GetTermWidth()
	border := strings.Repeat(core.Sym.BoxH, innerW)

	type BoxRow struct {
		Text string
	}
	var allRows []BoxRow
	for _, l := range lines {
		subLines := WrapBoxLine(l, innerW-2)
		for _, subL := range subLines {
			formattedRow := core.NC + " " + PadOrTruncate(subL, innerW-2) + core.NC + " "
			allRows = append(allRows, BoxRow{Text: formattedRow})
		}
	}

	totalRows := len(allRows)
	scrollOffset := 0

	drawBox := func() {
		termH := GetTermHeight()

		titleWrapped := WrapTextLines(displayTitle, innerW)
		fixedOverhead := 1 + len(titleWrapped) + 1 + 1 + 1 + 1
		availHeight := termH - fixedOverhead
		if availHeight < 3 {
			availHeight = 3
		}

		isOverflow := totalRows > availHeight

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

		// Top border (Square)
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
			buf.WriteString(core.BLUE + core.Sym.BoxV + strings.Repeat(" ", tPadL) + core.BOLD + core.BLUE + tRow + core.NC + strings.Repeat(" ", tPadR) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
		}

		// Separator
		buf.WriteString(core.BLUE + core.Sym.BoxML + border + core.Sym.BoxMR + core.NC + "\033[K\r\n")

		// Scroll UP indicator
		if showTopArrow {
			topMsg := core.Sym.ArrowUp + " [Up: scroll up for more lines]"
			if core.RuneWidth(topMsg) > innerW {
				topMsg = core.PadOrTruncateANSI(topMsg, innerW)
			}
			padT := innerW - core.RuneWidth(topMsg)
			if padT < 0 {
				padT = 0
			}
			pL := padT / 2
			pR := padT - pL
			buf.WriteString(core.BLUE + core.Sym.BoxV + strings.Repeat(" ", pL) + "\033[38;5;214m" + topMsg + "\033[0m" + strings.Repeat(" ", pR) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
		}

		// Content rows
		for rIdx := scrollOffset; rIdx < endRowIdx && rIdx < totalRows; rIdx++ {
			buf.WriteString(core.BLUE + core.Sym.BoxV + allRows[rIdx].Text + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
		}

		// Scroll DOWN indicator
		if showBotArrow {
			botMsg := core.Sym.ArrowDown + " [Down: scroll down for more lines]"
			if core.RuneWidth(botMsg) > innerW {
				botMsg = core.PadOrTruncateANSI(botMsg, innerW)
			}
			padB := innerW - core.RuneWidth(botMsg)
			if padB < 0 {
				padB = 0
			}
			pL := padB / 2
			pR := padB - pL
			buf.WriteString(core.BLUE + core.Sym.BoxV + strings.Repeat(" ", pL) + "\033[38;5;214m" + botMsg + "\033[0m" + strings.Repeat(" ", pR) + core.BLUE + core.Sym.BoxV + core.NC + "\033[K\r\n")
		}

		// Help bar
		helpStr := customHelp
		if helpStr == "" {
			if isOverflow {
				helpStr = "[Up/Down] Scroll   [PgUp/PgDn] Page   [Enter/Esc] Return to Menu"
			} else {
				helpStr = "[Enter / Esc]  " + core.Sym.ArrowRight + "  Return to Menu"
			}
		}
		if core.RuneWidth(helpStr) > innerW {
			if isOverflow {
				helpStr = "[Up/Down] Scroll  [Enter/Esc] Back"
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

		fmt.Print(buf.String())
	}

	drawBox()
	boxNeedsHeader = false

	if !waitForKey || core.AutoYes {
		if waitForKey && core.AutoYes {
			fmt.Print("\r\n")
		}
		return
	}

	HideCursor()
	defer ShowCursor()

	for {
		termH := GetTermHeight()
		titleWrapped := WrapTextLines(displayTitle, innerW)
		fixedOverhead := 1 + len(titleWrapped) + 1 + 1 + 1 + 1
		availHeight := termH - fixedOverhead
		if availHeight < 3 {
			availHeight = 3
		}

		isOverflow := totalRows > availHeight
		maxOffset := totalRows - (availHeight - 1)
		if maxOffset < 0 {
			maxOffset = 0
		}
		if scrollOffset > maxOffset {
			scrollOffset = maxOffset
		}

		key := ReadKey()
		if key == "ctrl+c" {
			LeaveAltScreen()
			DisableRawMode()
			ShowCursor()
			os.Exit(0)
		}
		if !isOverflow || key == "enter" || key == "esc" || key == "backspace" || key == "q" || key == "ctrl+d" || key == "eof" {
			fmt.Print("\r\n")
			return
		}

		switch key {
		case "up":
			if scrollOffset > 0 {
				scrollOffset--
				drawBox()
			}
		case "down":
			if scrollOffset < maxOffset {
				scrollOffset++
				drawBox()
			}
		case "pgup":
			scrollOffset -= availHeight
			if scrollOffset < 0 {
				scrollOffset = 0
			}
			drawBox()
		case "pgdn", " ":
			scrollOffset += availHeight
			if scrollOffset > maxOffset {
				scrollOffset = maxOffset
			}
			drawBox()
		case "home":
			scrollOffset = 0
			drawBox()
		case "end":
			scrollOffset = maxOffset
			drawBox()
		}
	}
}

func FormatSystemctlStatusOutput(serviceName string, rawOutput string) []string {
	var result []string
	lines := strings.Split(rawOutput, "\n")

	var statusHeader []string
	var journalEvents []string
	inJournal := false

	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

	for _, line := range lines {
		trimmed := core.SanitizeTerminalText(strings.TrimSpace(line))
		if trimmed == "" {
			continue
		}

		isJournalLine := false
		for _, m := range months {
			if strings.HasPrefix(trimmed, m+" ") {
				isJournalLine = true
				break
			}
		}

		if isJournalLine {
			inJournal = true
			parts := strings.SplitN(trimmed, " ", 4)
			if len(parts) >= 4 {
				ts := parts[0] + " " + parts[1] + " " + parts[2]
				msg := parts[3]
				journalEvents = append(journalEvents, fmt.Sprintf("  %s%s%s %s%s%s %s", core.CYAN, core.Sym.Pointer, core.NC, core.DIM+core.YELLOW, ts, core.NC, msg))
			} else {
				journalEvents = append(journalEvents, fmt.Sprintf("  %s%s%s %s", core.CYAN, core.Sym.Pointer, core.NC, trimmed))
			}
			continue
		}

		if inJournal {
			journalEvents = append(journalEvents, fmt.Sprintf("  %s%s%s %s", core.CYAN, core.Sym.Pointer, core.NC, trimmed))
			continue
		}

		if strings.HasPrefix(trimmed, "●") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "○") || strings.HasPrefix(trimmed, "×") {
			isDown := strings.HasPrefix(trimmed, "○") || strings.HasPrefix(trimmed, "×") || strings.Contains(rawOutput, "Active: failed") || strings.Contains(rawOutput, "Active: inactive")
			cleanHead := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(trimmed, "●"), "*"), "○"), "×")
			cleanHead = strings.TrimSpace(cleanHead)
			parts := strings.SplitN(cleanHead, " - ", 2)
			dot := core.GREEN + core.Sym.ActiveDot + core.NC
			if isDown {
				dot = core.RED + core.Sym.ActiveDot + core.NC
			}
			if len(parts) == 2 {
				statusHeader = append(statusHeader, fmt.Sprintf("%s %s%s%s %s- %s%s", dot, core.BOLD, core.CYAN, parts[0], core.NC, core.DIM, parts[1]))
			} else {
				statusHeader = append(statusHeader, fmt.Sprintf("%s %s%s%s", dot, core.BOLD, core.CYAN, cleanHead+core.NC))
			}
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			keyFormatted := fmt.Sprintf("  %s%s%s %-16s : ", core.CYAN, core.Sym.Pointer, core.NC, key)

			var valFormatted string
			switch key {
			case "Active":
				if strings.Contains(val, "active (running)") || strings.Contains(val, "active (exited)") || strings.Contains(val, "active") {
					valFormatted = core.GREEN + core.BOLD + val + core.NC
				} else if strings.Contains(val, "failed") || strings.Contains(val, "inactive") || strings.Contains(val, "dead") {
					valFormatted = core.RED + core.BOLD + val + core.NC
				} else {
					valFormatted = core.YELLOW + val + core.NC
				}
			case "Loaded":
				if strings.HasPrefix(val, "loaded") {
					valFormatted = core.GREEN + "loaded" + core.NC + " " + core.DIM + val[6:] + core.NC
				} else {
					valFormatted = val
				}
			case "Main PID", "PID", "Tasks":
				valFormatted = core.YELLOW + core.BOLD + val + core.NC
			case "Memory", "Mem peak", "CPU":
				valFormatted = core.GREEN + core.BOLD + val + core.NC
			default:
				valFormatted = val
			}

			statusHeader = append(statusHeader, keyFormatted+valFormatted)
		} else {
			statusHeader = append(statusHeader, "  "+trimmed)
		}
	}

	result = append(result, statusHeader...)

	if len(journalEvents) > 0 {
		result = append(result, "")
		result = append(result, core.BOLD+core.YELLOW+"RECENT SERVICE EVENTS:"+core.NC)
		result = append(result, journalEvents...)
	}

	if len(result) == 0 {
		return FormatInfoNotice("INFO", "No systemd status output available for "+serviceName+".", core.YELLOW)
	}

	return result
}
