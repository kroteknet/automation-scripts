// ==============================================================================
// KROTEK SCRIPT HUB — DYNAMIC TERMINAL BANNER & SYSTEM TELEMETRY
// Service: EMBY (media-server)
// File:    media-server/emby/src/modules/banner.go
//
// Renders the contextual header banner, showing real-time CPU utilization, RAM usage gauges, storage capacity, system uptime, and service status indicators.
// ==============================================================================

package modules

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"krotek-emby-installer/core"
	"krotek-emby-installer/system"
)

var (
	prevCPUTotal uint64
	prevCPUIdle  uint64
	prevCPUTime  time.Time
	cachedCPUStr string = "0%"
)

func formatCompactUptime(raw string) string {
	u := strings.TrimPrefix(strings.TrimSpace(raw), "up ")
	u = strings.ReplaceAll(u, " hours", "h")
	u = strings.ReplaceAll(u, " hour", "h")
	u = strings.ReplaceAll(u, " minutes", "m")
	u = strings.ReplaceAll(u, " minute", "m")
	u = strings.ReplaceAll(u, " days", "d")
	u = strings.ReplaceAll(u, " day", "d")
	u = strings.ReplaceAll(u, ", ", " ")
	return u
}

func GetOSPrettyVersion() string {
	osStr := ""
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, l := range lines {
			if strings.HasPrefix(l, "PRETTY_NAME=") {
				osStr = strings.Trim(strings.TrimPrefix(l, "PRETTY_NAME="), "\"")
				break
			}
		}
	}
	if osStr == "" {
		if data, err := os.ReadFile("/etc/debian_version"); err == nil {
			osStr = "Debian " + strings.TrimSpace(string(data))
		}
	}
	if osStr == "" {
		osStr = core.SysInfo.Distro + " " + core.SysInfo.Version
	}
	osStr = strings.ReplaceAll(osStr, "GNU/Linux", "")
	return strings.TrimSpace(osStr)
}

// GetCPUUsagePercent calculates real-time CPU utilization percent from /proc/stat or /proc/loadavg.
func GetCPUUsagePercent() string {
	cores := runtime.NumCPU()
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		if lData, lErr := os.ReadFile("/proc/loadavg"); lErr == nil {
			fields := strings.Fields(string(lData))
			if len(fields) >= 1 {
				if l1, parseErr := strconv.ParseFloat(fields[0], 64); parseErr == nil && cores > 0 {
					pct := (l1 / float64(cores)) * 100
					if pct > 100 {
						pct = 100
					}
					return fmt.Sprintf("%.0f%%", pct)
				}
			}
		}
		return "0%"
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return "0%"
	}

	fields := strings.Fields(lines[0])
	if len(fields) < 5 {
		return "0%"
	}

	var total, idle uint64
	for i := 1; i < len(fields); i++ {
		val, _ := strconv.ParseUint(fields[i], 10, 64)
		total += val
		if i == 4 || i == 5 {
			idle += val
		}
	}

	if prevCPUTotal > 0 && time.Since(prevCPUTime) < 400*time.Millisecond {
		return cachedCPUStr
	}

	if prevCPUTotal > 0 && total > prevCPUTotal {
		diffTotal := float64(total - prevCPUTotal)
		diffIdle := float64(idle - prevCPUIdle)
		if diffTotal > 0 {
			pct := (1.0 - diffIdle/diffTotal) * 100.0
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
			cachedCPUStr = fmt.Sprintf("%.0f%%", pct)
		}
	}

	prevCPUTotal = total
	prevCPUIdle = idle
	prevCPUTime = time.Now()
	return cachedCPUStr
}

func renderGrid(grid [][]string, targetW int) []string {
	rows := len(grid)
	if rows == 0 {
		return nil
	}

	maxCols := 0
	for _, r := range grid {
		if len(r) > maxCols {
			maxCols = len(r)
		}
	}
	if maxCols == 0 {
		return nil
	}

	colWidths := make([]int, maxCols)
	for c := 0; c < maxCols; c++ {
		for r := 0; r < rows; r++ {
			if c < len(grid[r]) {
				w := core.RuneWidth(grid[r][c])
				if w > colWidths[c] {
					colWidths[c] = w
				}
			}
		}
	}

	// Dynamic scaling: If total grid width exceeds targetW, proportionally scale down widest columns
	sep := "\033[2m\033[38;5;244m" + core.Sym.BoxV + "\033[0m"
	totalSepW := (maxCols - 1) * 3
	totalW := totalSepW
	for _, w := range colWidths {
		totalW += w
	}

	if targetW > 0 && totalW > targetW {
		excess := totalW - targetW
		for excess > 0 {
			widestIdx := -1
			maxW := 0
			for i, w := range colWidths {
				if w > maxW && w > 4 {
					maxW = w
					widestIdx = i
				}
			}
			if widestIdx == -1 {
				break
			}
			colWidths[widestIdx]--
			excess--
		}
	}

	var lines []string
	for r := 0; r < rows; r++ {
		var parts []string
		for c := 0; c < maxCols; c++ {
			val := ""
			if c < len(grid[r]) {
				val = grid[r][c]
			}
			if core.RuneWidth(val) > colWidths[c] {
				val = core.PadOrTruncateANSI(val, colWidths[c])
			} else {
				val = core.PadANSI(val, colWidths[c])
			}
			parts = append(parts, val)
		}
		lines = append(lines, strings.Join(parts, " "+sep+" "))
	}
	return lines
}

func DetectActiveFirewallCompact() string {
	if system.CommandExists("ufw") {
		if out, err := exec.Command("ufw", "status").CombinedOutput(); err == nil && strings.Contains(string(out), "active") {
			return core.GREEN + "UFW" + core.NC
		}
	}
	if system.CommandExists("firewall-cmd") {
		if out, err := exec.Command("firewall-cmd", "--state").CombinedOutput(); err == nil && strings.Contains(string(out), "running") {
			return core.GREEN + "Firewalld" + core.NC
		}
	}
	if system.CommandExists("nft") {
		return core.CYAN + "nftables" + core.NC
	}
	if system.CommandExists("iptables") {
		return core.CYAN + "iptables" + core.NC
	}
	return core.DIM + "Inactive" + core.NC
}

func DetectSSHStatusCompact() string {
	if exec.Command("systemctl", "is-active", "--quiet", "ssh").Run() == nil ||
		exec.Command("systemctl", "is-active", "--quiet", "sshd").Run() == nil ||
		exec.Command("rc-service", "sshd", "status").Run() == nil ||
		exec.Command("pidof", "sshd").Run() == nil ||
		exec.Command("pidof", "dropbear").Run() == nil {
		return core.GREEN + "Active (22)" + core.NC
	}
	if system.CommandExists("sshd") || system.CommandExists("ssh") || system.CommandExists("dropbear") {
		return core.YELLOW + "Stopped" + core.NC
	}
	return core.DIM + "Not Installed" + core.NC
}

// 1. GLOBAL LIVE BANNER
func GetLiveBanner(targetW int) []string {
	arch := core.SysInfo.Architecture
	if arch == "" {
		arch = runtime.GOARCH
	}

	uptime := "N/A"
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) > 0 {
			if secs, err := strconv.ParseFloat(parts[0], 64); err == nil {
				totalSecs := int(secs)
				days := totalSecs / 86400
				hours := (totalSecs % 86400) / 3600
				mins := (totalSecs % 3600) / 60
				if days > 0 {
					uptime = fmt.Sprintf("%dd %dh", days, hours)
				} else if hours > 0 {
					uptime = fmt.Sprintf("%dh %dm", hours, mins)
				} else {
					uptime = fmt.Sprintf("%dm", mins)
				}
			}
		}
	}
	if uptime == "N/A" {
		if out, err := exec.Command("uptime", "-p").Output(); err == nil {
			uptime = formatCompactUptime(string(out))
		}
	}

	cpuStr := GetCPUUsagePercent()
	cores := runtime.NumCPU()
	var cpuPct float64
	if cp, err := strconv.ParseFloat(strings.TrimSuffix(cpuStr, "%"), 64); err == nil {
		cpuPct = cp
	}

	upDisplay := uptime
	if cpuStr != "" {
		upDisplay = fmt.Sprintf("%s (CPU: %s)", uptime, cpuStr)
	}

	osVer := GetOSPrettyVersion()
	osVerClean := regexp.MustCompile("\\s*\\([^)]+\\)").ReplaceAllString(osVer, "")
	if len(osVerClean) > 22 {
		osVerClean = osVerClean[:22]
	}

	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "Linux"
	}
	if len(hostname) > 18 {
		hostname = hostname[:18]
	}

	var ramPct float64
	sysRamStr := "N/A"
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var total, free uint64
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					total, _ = strconv.ParseUint(fields[1], 10, 64)
				}
			}
			if strings.HasPrefix(line, "MemAvailable:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					free, _ = strconv.ParseUint(fields[1], 10, 64)
				}
			}
		}
		if total > 0 {
			used := total - free
			ramPct = float64(used*100) / float64(total)
			sysRamStr = fmt.Sprintf("%.1fG/%.1fG", float64(used)/1024/1024, float64(total)/1024/1024)
		}
	}

	var diskPct float64
	diskSt := "N/A"
	diskFree := ""
	if out, err := exec.Command("df", "-h", "/").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 5 {
				diskSt = fields[2] + "/" + fields[1]
				diskFree = fields[3] + " Free"
				pStr := strings.TrimSuffix(fields[4], "%")
				if pVal, pErr := strconv.ParseFloat(pStr, 64); pErr == nil {
					diskPct = pVal
				}
			}
		}
	}

	ipAddr := system.GetLocalIP()
	health := DetectEmbyHealth()
	statusBadge := core.FormatServiceBadge(health.IsRunning, health.Installed, "")

	if targetW >= 120 {
		gaugeW := 8
		grid := [][]string{
			{
				fmt.Sprintf("%s%s OS:%s %s%s%s \033[2m(%s)\033[0m", core.CYAN, core.BannerIcon("#", "OS"), core.NC, core.BOLD+core.YELLOW, osVerClean, core.NC, arch),
				fmt.Sprintf("%s%s Host:%s %s%s%s", core.CYAN, core.BannerIcon("@", "Host"), core.NC, core.BOLD, hostname, core.NC),
				fmt.Sprintf("%s%s Up:%s %s%s%s", core.CYAN, core.BannerIcon("~", "Up"), core.NC, core.BOLD, upDisplay, core.NC),
				fmt.Sprintf("%s%s Service:%s %s", core.CYAN, core.BannerIcon("*", "Service"), core.NC, statusBadge),
			},
			{
				fmt.Sprintf("%s %s %s(%s)%s", core.CYAN+core.BannerIcon("%", "RAM")+" RAM:"+core.NC, core.RenderGauge(ramPct, gaugeW), core.DIM, sysRamStr, core.NC),
				fmt.Sprintf("%s %s %s(%d Cores)%s", core.CYAN+core.BannerIcon("%", "CPU")+" CPU:"+core.NC, core.RenderGauge(cpuPct, gaugeW), core.DIM, cores, core.NC),
				fmt.Sprintf("%s%s Disk:%s %s %s(%s)%s", core.CYAN, core.BannerIcon("/", "Disk"), core.NC, core.RenderGauge(diskPct, gaugeW), core.DIM, diskFree, core.NC),
				fmt.Sprintf("%s%s Web UI:%s %shttp://%s:8096%s", core.CYAN, core.BannerIcon("*", "Web UI"), core.NC, core.BOLD+core.GREEN, ipAddr, core.NC),
			},
			{
				fmt.Sprintf("%s%s IPv4:%s %s%s%s \033[2m(Online)\033[0m", core.CYAN, core.BannerIcon("@", "IPv4"), core.NC, core.BOLD+core.PURPLE, ipAddr, core.NC),
				fmt.Sprintf("%s%s Media:%s %s/var/lib/emby%s", core.CYAN, core.BannerIcon("*", "Media"), core.NC, core.BOLD, core.NC),
				fmt.Sprintf("%s%s Sessions:%s %sActive Clients%s", core.CYAN, core.BannerIcon("*", "Sessions"), core.NC, core.BOLD+core.GREEN, core.NC),
				fmt.Sprintf("%s%s Mode:%s %sEmby Server%s", core.CYAN, core.BannerIcon("*", "Mode"), core.NC, core.BOLD, core.NC),
			},
		}
		return renderGrid(grid, targetW)
	}

	// 3-Column Symmetrical Standard Layout (Optimized for standard terminals with rich tech telemetry)
	hostShort := hostname
	if targetW < 85 && len(hostShort) > 12 {
		hostShort = hostShort[:12]
	} else if len(hostShort) > 16 {
		hostShort = hostShort[:16]
	}

	diskDisplay := diskSt
	if targetW >= 92 && diskFree != "" {
		diskDisplay = fmt.Sprintf("%s \033[2m(%s)\033[0m", diskSt, diskFree)
	}

	grid := [][]string{
		{
			fmt.Sprintf("%s%s OS:%s %s%s%s \033[2m(%s)\033[0m", core.CYAN, core.BannerIcon("#", "OS"), core.NC, core.BOLD+core.YELLOW, osVerClean, core.NC, arch),
			fmt.Sprintf("%s%s Host:%s %s%s%s", core.CYAN, core.BannerIcon("@", "Host"), core.NC, core.BOLD, hostShort, core.NC),
			fmt.Sprintf("%s%s Up:%s %s%s%s", core.CYAN, core.BannerIcon("~", "Up"), core.NC, core.BOLD, uptime, core.NC),
		},
		{
			fmt.Sprintf("%s %s%s%s", core.CYAN+core.BannerIcon("%", "RAM")+" RAM:"+core.NC, core.BOLD, sysRamStr, core.NC),
			fmt.Sprintf("%s %s%s%s \033[2m(%dC)\033[0m", core.CYAN+core.BannerIcon("%", "CPU")+" CPU:"+core.NC, core.BOLD, cpuStr, core.NC, cores),
			fmt.Sprintf("%s%s Disk:%s %s%s%s", core.CYAN, core.BannerIcon("/", "Disk"), core.NC, core.BOLD, diskDisplay, core.NC),
		},
		{
			fmt.Sprintf("%s%s Service:%s %s", core.CYAN, core.BannerIcon("*", "Service"), core.NC, statusBadge),
			fmt.Sprintf("%s%s IPv4:%s %s%s%s", core.CYAN, core.BannerIcon("@", "IPv4"), core.NC, core.BOLD+core.PURPLE, ipAddr, core.NC),
			fmt.Sprintf("%s%s Web UI:%s %shttp://%s:8096%s", core.CYAN, core.BannerIcon("*", "Web UI"), core.NC, core.BOLD+core.GREEN, ipAddr, core.NC),
		},
		{
			fmt.Sprintf("%s%s Media:%s %s/var/lib/emby%s", core.CYAN, core.BannerIcon("*", "Media"), core.NC, core.BOLD, core.NC),
			fmt.Sprintf("%s%s Sessions:%s %sActive Clients%s", core.CYAN, core.BannerIcon("*", "Sessions"), core.NC, core.BOLD+core.GREEN, core.NC),
			fmt.Sprintf("%s%s Mode:%s %sEmby Server%s", core.CYAN, core.BannerIcon("*", "Mode"), core.NC, core.BOLD, core.NC),
		},
	}
	return renderGrid(grid, targetW)
}

var (
	cachedBannerLines []string
	cachedBannerWidth int
	lastBannerCheck   time.Time
)

// GetContextualBanner renders the dynamic header banner card with live hardware telemetry, CPU/RAM gauges, and status badges.
func GetContextualBanner(title string, targetW int) []string {
	if len(cachedBannerLines) > 0 && cachedBannerWidth == targetW && time.Since(lastBannerCheck) < 2*time.Second {
		return cachedBannerLines
	}
	cachedBannerLines = GetLiveBanner(targetW)
	cachedBannerWidth = targetW
	lastBannerCheck = time.Now()
	return cachedBannerLines
}
