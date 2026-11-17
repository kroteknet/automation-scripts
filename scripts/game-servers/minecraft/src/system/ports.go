// ==============================================================================
// KROTEK SCRIPT HUB — PORT CONFLICT RESOLVER & FIREWALL MANAGER
// Service: MINECRAFT (game-servers)
// File:    game-servers/minecraft/src/system/ports.go
//
// Performs socket-level TCP/UDP port availability probes, identifies conflicting processes and containers, offers automated conflict resolution, and configures firewall rules.
// ==============================================================================

package system

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"krotek-minecraft-installer/core"
	"krotek-minecraft-installer/logger"
	"krotek-minecraft-installer/tui"
)

// PortConflictInfo holds detailed socket conflict diagnostic telemetry for TCP/UDP ports.
type PortConflictInfo struct {
	Port      string
	Protocol  string
	InUse     bool
	Process   string
	PID       string
	Unit      string
	Container string
	Address   string
}

// CheckPortConflict inspects whether a port (TCP or UDP) is in use and identifies the owning process/unit.
func CheckPortConflict(port string, proto string) PortConflictInfo {
	info := PortConflictInfo{
		Port:     port,
		Protocol: strings.ToLower(proto),
	}
	if info.Protocol == "" {
		info.Protocol = "tcp"
	}
	if port == "" {
		return info
	}

	pNum, err := strconv.Atoi(port)
	if err != nil || pNum <= 0 || pNum > 65535 {
		return info
	}

	// 1. Direct Socket Probe (authoritative check if port is bindable)
	if info.Protocol == "tcp" {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
		if err != nil {
			info.InUse = true
		} else {
			_ = ln.Close()
			return info // Port is completely free!
		}
	} else if info.Protocol == "udp" {
		conn, err := net.ListenPacket("udp", fmt.Sprintf(":%s", port))
		if err != nil {
			info.InUse = true
		} else {
			_ = conn.Close()
			return info // Port is completely free!
		}
	}

	hexPort := fmt.Sprintf("%04X", pNum)

	// 2. Direct Linux /proc/net probe (zero dependencies, 100% reliable across Linux & Alpine BusyBox)
	var inodes []string
	procFiles := []string{"/proc/net/tcp", "/proc/net/tcp6"}
	if info.Protocol == "udp" {
		procFiles = []string{"/proc/net/udp", "/proc/net/udp6"}
	}

	for _, pFile := range procFiles {
		data, err := os.ReadFile(pFile)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) >= 10 {
				localAddr := fields[1]
				state := fields[3]
				// 0A is TCP_LISTEN in /proc/net/tcp
				if (info.Protocol == "udp" || state == "0A") && strings.HasSuffix(localAddr, ":"+hexPort) {
					inode := fields[9]
					if inode != "0" && inode != "" {
						inodes = append(inodes, inode)
					}
				}
			}
		}
	}

	// Scan /proc/[pid]/fd to map socket inodes to process PID and comm
	if len(inodes) > 0 {
		if procDirs, err := os.ReadDir("/proc"); err == nil {
			inodeMap := make(map[string]bool)
			for _, in := range inodes {
				inodeMap[in] = true
			}

			for _, pDir := range procDirs {
				if !pDir.IsDir() {
					continue
				}
				pid := pDir.Name()
				// Only inspect numeric PIDs and NEVER treat PID 1 (init) as the target process
				if _, err := strconv.Atoi(pid); err != nil || pid == "1" || pid == "0" {
					continue
				}

				fdPath := filepath.Join("/proc", pid, "fd")
				fds, err := os.ReadDir(fdPath)
				if err != nil {
					continue
				}

				matched := false
				for _, fd := range fds {
					link, err := os.Readlink(filepath.Join(fdPath, fd.Name()))
					if err != nil {
						continue
					}
					if strings.HasPrefix(link, "socket:[") {
						sockInode := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
						if inodeMap[sockInode] {
							info.PID = pid
							matched = true
							break
						}
					}
				}

				if matched {
					if commBytes, err := os.ReadFile(filepath.Join("/proc", pid, "comm")); err == nil {
						info.Process = strings.TrimSpace(string(commBytes))
					}
					break
				}
			}
		}
	}

	// 3. Fallback: netstat -tlpn / -ulpn (standard in BusyBox on Alpine & GNU net-tools)
	if info.InUse && (info.PID == "" || info.Process == "") {
		netstatArgs := []string{"-tlpn"}
		if info.Protocol == "udp" {
			netstatArgs = []string{"-ulpn"}
		}
		if out, err := exec.Command("netstat", netstatArgs...).Output(); err == nil {
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				if strings.Contains(line, ":"+port+" ") || strings.Contains(line, ":"+port+"\t") {
					fields := strings.Fields(line)
					if len(fields) >= 7 {
						lastCol := fields[len(fields)-1] // e.g. "1423/nginx" or "1423/nginx: master"
						if strings.Contains(lastCol, "/") {
							parts := strings.SplitN(lastCol, "/", 2)
							pidCand := strings.TrimSpace(parts[0])
							if pidCand != "1" && pidCand != "0" {
								if info.PID == "" {
									info.PID = pidCand
								}
								if info.Process == "" {
									info.Process = strings.TrimSpace(parts[1])
								}
							}
						}
					}
					break
				}
			}
		}
	}

	// 4. Fallback: ss -tulpn (iproute2)
	if info.InUse && (info.PID == "" || info.Process == "") && CommandExists("ss") {
		ssArgs := "-tulpn"
		if out, err := exec.Command("ss", ssArgs).Output(); err == nil {
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				if strings.Contains(line, ":"+port+" ") || strings.Contains(line, ":"+port+"\t") {
					if idx := strings.Index(line, "users:(("); idx != -1 {
						usersPart := line[idx+8:]
						if endIdx := strings.Index(usersPart, "))"); endIdx != -1 {
							usersPart = usersPart[:endIdx]
						}
						parts := strings.Split(usersPart, ",")
						for _, p := range parts {
							p = strings.Trim(p, "\" ")
							if strings.HasPrefix(p, "pid=") {
								pidCand := strings.TrimPrefix(p, "pid=")
								if pidCand != "1" && pidCand != "0" && info.PID == "" {
									info.PID = pidCand
								}
							} else if !strings.Contains(p, "=") && info.Process == "" {
								info.Process = p
							}
						}
					}
					break
				}
			}
		}
	}

	// 5. Fallback process name via /proc/<pid>/comm
	if info.PID != "" && info.PID != "1" && info.Process == "" {
		if commBytes, err := os.ReadFile(filepath.Join("/proc", info.PID, "comm")); err == nil {
			info.Process = strings.TrimSpace(string(commBytes))
		}
	}

	// 6. Service Unit Detection (OpenRC & systemd)
	if info.Process != "" {
		// OpenRC (Alpine Linux)
		if CommandExists("rc-service") {
			if exec.Command("rc-service", info.Process, "status").Run() == nil {
				info.Unit = info.Process
			}
		}
		// systemd (Debian, Ubuntu, RHEL)
		if info.Unit == "" && CommandExists("systemctl") {
			if exec.Command("systemctl", "is-active", "--quiet", info.Process).Run() == nil ||
				exec.Command("systemctl", "status", info.Process).Run() == nil {
				info.Unit = info.Process + ".service"
			}
		}
	}

	// Also check common web service names if port is 80 or 443
	if info.Unit == "" && (port == "80" || port == "443") {
		commonServices := []string{"nginx", "apache2", "httpd", "caddy", "lighttpd"}
		if CommandExists("rc-service") {
			for _, s := range commonServices {
				if exec.Command("rc-service", s, "status").Run() == nil {
					info.Unit = s
					if info.Process == "" {
						info.Process = s
					}
					break
				}
			}
		}
		if info.Unit == "" && CommandExists("systemctl") {
			for _, s := range commonServices {
				if exec.Command("systemctl", "is-active", "--quiet", s).Run() == nil {
					info.Unit = s + ".service"
					if info.Process == "" {
						info.Process = s
					}
					break
				}
			}
			if info.Unit == "" && CommandExists("service") {
				for _, s := range commonServices {
					if exec.Command("service", s, "status").Run() == nil {
						info.Unit = s
						if info.Process == "" {
							info.Process = s
						}
						break
					}
				}
			}
		}
	}

	// 7. Docker container detection
	if (strings.Contains(info.Process, "docker") || strings.Contains(info.Process, "docker-proxy")) && CommandExists("docker") {
		if out, err := exec.Command("docker", "ps", "--format", "{{.Names}}\t{{.Ports}}").Output(); err == nil {
			for _, l := range strings.Split(string(out), "\n") {
				if strings.Contains(l, ":"+port+"->") || strings.Contains(l, ":"+port+"/") {
					cols := strings.Split(l, "\t")
					if len(cols) > 0 {
						info.Container = cols[0]
						info.Process = fmt.Sprintf("docker (%s)", cols[0])
					}
					break
				}
			}
		}
	}

	// Safeguard: PID 1 is NEVER a killable process
	if info.PID == "1" || info.PID == "0" {
		info.PID = ""
	}

	return info
}

// ResolvePortConflict checks for port conflict and presents an interactive resolver if needed.
// Returns the resolved port and true if free/cleared, or ("", false) if aborted.
func ResolvePortConflict(port string, proto string, allowCustomPort bool) (string, bool) {
	currentPort := strings.TrimSpace(port)
	if currentPort == "" {
		return "", false
	}

	pNum, err := strconv.Atoi(currentPort)
	if err != nil || pNum < 1 || pNum > 65535 {
		if !allowCustomPort {
			logger.LogError(fmt.Sprintf("Invalid port: '%s'. Port must be an integer between 1 and 65535.", currentPort))
			return "", false
		}
		for {
			tui.PrintInBox("Invalid Port", []string{core.RED + fmt.Sprintf("Port '%s' is invalid. Port must be an integer between 1 and 65535.", currentPort) + core.NC})
			newP, st := tui.ReadTextInput("Enter New Port (1-65535)", "")
			if st != 0 {
				return "", false
			}
			newP = strings.TrimSpace(newP)
			num, err := strconv.Atoi(newP)
			if err == nil && num >= 1 && num <= 65535 {
				currentPort = strconv.Itoa(num)
				break
			}
		}
	}

	failedAttempts := 0

	for {
		info := CheckPortConflict(currentPort, proto)
		if !info.InUse {
			return currentPort, true
		}

		tui.ClearScreen()
		var cardLines []string
		cardLines = append(cardLines, core.BOLD+core.RED+fmt.Sprintf("[WARNING] PORT CONFLICT DETECTED: %s/%s IS OCCUPIED", currentPort, strings.ToUpper(proto))+core.NC, "")

		procDesc := "Unknown background process"
		if info.Process != "" {
			procDesc = info.Process
		}
		if info.PID != "" && info.PID != "1" {
			procDesc += fmt.Sprintf(" (PID: %s)", info.PID)
		}
		cardLines = append(cardLines, tui.FormatKV("Conflicting Process", procDesc, core.YELLOW+core.BOLD))

		if info.Unit != "" {
			serviceLabel := "Service Unit"
			if CommandExists("rc-service") && !strings.HasSuffix(info.Unit, ".service") {
				serviceLabel = "OpenRC Service"
			} else if CommandExists("systemctl") {
				serviceLabel = "Systemd Service"
			}
			cardLines = append(cardLines, tui.FormatKV(serviceLabel, info.Unit, core.CYAN))
		}
		if info.Container != "" {
			cardLines = append(cardLines, tui.FormatKV("Docker Container", info.Container, core.CYAN))
		}
		if info.Address != "" {
			cardLines = append(cardLines, tui.FormatKV("Listening Address", info.Address, core.DIM))
		}

		if failedAttempts > 0 {
			cardLines = append(cardLines, "", core.BOLD+core.YELLOW+"[!] Warning: Previous attempt could not free port. Service may be auto-respawning."+core.NC)
		}

		cardLines = append(cardLines, "", core.BOLD+"Choose an action to resolve this port conflict:"+core.NC)

		var options []string
		var actionTypes []string

		if info.Unit != "" {
			options = append(options, fmt.Sprintf("Stop conflicting service (%s)", info.Unit))
			actionTypes = append(actionTypes, "stop_unit")
		} else if info.Container != "" {
			options = append(options, fmt.Sprintf("Stop Docker container (%s)", info.Container))
			actionTypes = append(actionTypes, "stop_container")
		}

		if info.Process != "" && info.Process != "init" && info.Process != "systemd" {
			options = append(options, fmt.Sprintf("Kill all '%s' processes", info.Process))
			actionTypes = append(actionTypes, "kill_process")
		} else if info.PID != "" && info.PID != "1" {
			options = append(options, fmt.Sprintf("Kill conflicting process (PID: %s)", info.PID))
			actionTypes = append(actionTypes, "kill_pid")
		} else {
			options = append(options, fmt.Sprintf("Force release port %s (fuser)", currentPort))
			actionTypes = append(actionTypes, "fuser_kill")
		}

		if allowCustomPort {
			options = append(options, "Choose a different port for this installation")
			actionTypes = append(actionTypes, "change_port")
		}

		options = append(options, "Abort / Cancel Installation")
		actionTypes = append(actionTypes, "abort")

		idx, _, status := tui.SelectMenuWithContent("Port Conflict Resolution Panel", cardLines, options)
		if status == 1 || status == 2 {
			fmt.Println()
			logger.LogWarn(fmt.Sprintf("Installation cancelled due to port %s conflict.", currentPort))
			return "", false
		}

		chosenAction := actionTypes[idx]
		switch chosenAction {
		case "stop_unit":
			tui.StartBoxSpinner(fmt.Sprintf("Stopping service %s...", info.Unit))
			StopService(strings.TrimSuffix(info.Unit, ".service"))
			time.Sleep(1 * time.Second)
			tui.StopBoxSpinner(0, fmt.Sprintf("Service %s stopped successfully.", info.Unit))

		case "stop_container":
			tui.StartBoxSpinner(fmt.Sprintf("Stopping container %s...", info.Container))
			_ = exec.Command("docker", "stop", info.Container).Run()
			time.Sleep(1 * time.Second)
			tui.StopBoxSpinner(0, fmt.Sprintf("Container %s stopped successfully.", info.Container))

		case "kill_process":
			tui.StartBoxSpinner(fmt.Sprintf("Terminating %s...", info.Process))
			myPid := strconv.Itoa(os.Getpid())
			if info.Unit != "" {
				StopService(strings.TrimSuffix(info.Unit, ".service"))
			}
			if out, err := exec.Command("pgrep", "-x", info.Process).Output(); err == nil {
				for _, line := range strings.Split(string(out), "\n") {
					p := strings.TrimSpace(line)
					if p != "" && p != "1" && p != "0" && p != myPid {
						_ = exec.Command("kill", "-9", p).Run()
					}
				}
			}
			if info.PID != "" && info.PID != "1" && info.PID != "0" && info.PID != myPid {
				_ = exec.Command("kill", "-9", info.PID).Run()
			}
			binName := filepath.Base(os.Args[0])
			if info.Process != binName && !strings.Contains(binName, info.Process) {
				_ = exec.Command("killall", "-9", "-e", info.Process).Run()
				_ = exec.Command("pkill", "-9", "-x", info.Process).Run()
			}
			time.Sleep(1 * time.Second)
			tui.StopBoxSpinner(0, fmt.Sprintf("Process %s terminated.", info.Process))

		case "kill_pid":
			tui.StartBoxSpinner(fmt.Sprintf("Killing PID %s...", info.PID))
			myPid := strconv.Itoa(os.Getpid())
			if info.PID != "" && info.PID != "1" && info.PID != "0" && info.PID != myPid {
				_ = exec.Command("kill", "-9", info.PID).Run()
			}
			time.Sleep(1 * time.Second)
			tui.StopBoxSpinner(0, fmt.Sprintf("Process PID %s killed.", info.PID))

		case "fuser_kill":
			tui.StartBoxSpinner(fmt.Sprintf("Releasing port %s...", currentPort))
			_ = exec.Command("fuser", "-k", "-9", fmt.Sprintf("%s/%s", currentPort, proto)).Run()
			_ = exec.Command("busybox", "fuser", "-k", "-9", fmt.Sprintf("%s/%s", currentPort, proto)).Run()
			time.Sleep(1 * time.Second)
			tui.StopBoxSpinner(0, fmt.Sprintf("Port %s released.", currentPort))

		case "change_port":
			tui.ClearScreen()
			newP, st := tui.ReadTextInput("Enter New Port (1-65535)", "")
			if st == 0 && strings.TrimSpace(newP) != "" {
				pNum, err := strconv.Atoi(strings.TrimSpace(newP))
				if err != nil || pNum < 1 || pNum > 65535 {
					tui.PrintInBox("Invalid Port", []string{core.RED + "Port must be an integer between 1 and 65535." + core.NC})
					continue
				}
				currentPort = strconv.Itoa(pNum)
				failedAttempts = 0
			}

		case "abort":
			fmt.Println()
			logger.LogWarn(fmt.Sprintf("Installation cancelled due to port %s conflict.", currentPort))
			return "", false
		}

		// Verify whether the action actually released the port
		checkAfter := CheckPortConflict(currentPort, proto)
		if !checkAfter.InUse {
			time.Sleep(600 * time.Millisecond)
			tui.ClearScreen()
			logger.LogSuccess(fmt.Sprintf("Port %s has been successfully released.", currentPort))
			return currentPort, true
		}
		failedAttempts++
	}
}

// ConfigureFirewallRules ensures specified ports are universally unblocked across
// UFW, Firewalld, AND raw iptables (including Oracle Cloud OCI and cloud VPS defaults).
// Supports both "80" (defaults to tcp) and "25565/udp".
func ConfigureFirewallRules(ports ...string) {
	for _, raw := range ports {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		proto := "tcp"
		port := raw
		if strings.Contains(raw, "/") {
			parts := strings.Split(raw, "/")
			port = parts[0]
			proto = strings.ToLower(parts[1])
		}
		if proto != "tcp" && proto != "udp" {
			continue
		}

		var cleanPort string
		if strings.Contains(port, "-") || strings.Contains(port, ":") {
			delim := "-"
			if strings.Contains(port, ":") {
				delim = ":"
			}
			parts := strings.Split(port, delim)
			if len(parts) == 2 {
				p1, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
				p2, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
				if err1 == nil && err2 == nil && p1 >= 1 && p1 <= 65535 && p2 >= 1 && p2 <= 65535 && p1 <= p2 {
					cleanPort = fmt.Sprintf("%d:%d", p1, p2)
				}
			}
		} else {
			p, err := strconv.Atoi(port)
			if err == nil && p >= 1 && p <= 65535 {
				cleanPort = strconv.Itoa(p)
			}
		}
		if cleanPort == "" {
			continue
		}

		ufwPort := strings.ReplaceAll(cleanPort, "-", ":")
		fwdPort := strings.ReplaceAll(cleanPort, ":", "-")
		iptPort := strings.ReplaceAll(cleanPort, "-", ":")

		// 1. UFW
		if CommandExists("ufw") {
			_ = exec.Command("ufw", "allow", fmt.Sprintf("%s/%s", ufwPort, proto)).Run()
		}
		// 2. Firewalld
		if CommandExists("firewall-cmd") {
			_ = exec.Command("firewall-cmd", "--permanent", fmt.Sprintf("--add-port=%s/%s", fwdPort, proto)).Run()
		}
		// 3. Raw iptables (essential for Oracle Cloud OCI where iptables drops non-SSH packets)
		if CommandExists("iptables") {
			_ = exec.Command("iptables", "-I", "INPUT", "1", "-p", proto, "--dport", iptPort, "-j", "ACCEPT").Run()
		}
		if CommandExists("ip6tables") {
			_ = exec.Command("ip6tables", "-I", "INPUT", "1", "-p", proto, "--dport", iptPort, "-j", "ACCEPT").Run()
		}
	}

	if CommandExists("ufw") {
		_ = exec.Command("ufw", "reload").Run()
	}
	if CommandExists("firewall-cmd") {
		_ = exec.Command("firewall-cmd", "--reload").Run()
	}
	if CommandExists("iptables") {
		if CommandExists("netfilter-persistent") {
			_ = exec.Command("netfilter-persistent", "save").Run()
		} else if fi, err := os.Lstat("/etc/iptables/rules.v4"); err == nil && fi.Mode().IsRegular() {
			if out, err := exec.Command("iptables-save").Output(); err == nil {
				_ = WriteFileNoFollow("/etc/iptables/rules.v4", out, 0600, "")
			}
		}
	}
}
