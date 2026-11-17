// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: MongoDB NoSQL Document Database (mongodb)
//
// This package orchestrates automated pre-flight checks, package repository setup,
// configuration templating, service enablement (systemd/openrc), and firewall rules.
// Supports both interactive TUI step-by-step guidance and headless automation.
// ==============================================================================

package install

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"krotek-mongodb-installer/core"
	"krotek-mongodb-installer/logger"
	"krotek-mongodb-installer/modules"
	"krotek-mongodb-installer/system"
	"krotek-mongodb-installer/tui"
)

// FreshInstallWizard handles the end-to-end installation pipeline for MongoDB NoSQL Document Database.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	envType := targetEnv
	if envType == "" {
		envType = "native"
	}

	accessScope := "local" // "local" (127.0.0.1) or "network" (0.0.0.0)
	port := "27017"
	adminUser := "admin"
	adminPassword, pErr := modules.GenerateRandomPass(16)
	if pErr != nil {
		logger.LogError(fmt.Sprintf("Failed to generate initial MongoDB admin password: %v", pErr))
		tui.PrintInBox("Security Error", []string{core.RED + "[ERROR] CSPRNG failure generating admin password." + core.NC})
		return
	}
	isCustomPass := false

	createAppDB := false
	appDBName := "app_db"
	appDBUser := "app_user"
	appDBPass, pErr := modules.GenerateRandomPass(16)
	if pErr != nil {
		logger.LogError(fmt.Sprintf("Failed to generate initial app password: %v", pErr))
		tui.PrintInBox("Security Error", []string{core.RED + "[ERROR] CSPRNG failure generating app password." + core.NC})
		return
	}

	composeDir := "/etc/mongodb-docker"

	PromptExistingConfigAction("MongoDB NoSQL Database", "/etc/mongod.conf")

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/4: Choose Access Scope & Network Binding (MongoDB)"
			if envType == "docker" {
				wizardTitle = "Step 1/4: Choose Access Scope (Docker MongoDB Stack)"
			}

			options := []string{
				"Localhost Only (127.0.0.1 - Maximum Security for local stacks/apps)",
				"Local Network / LAN (0.0.0.0 - Allow remote NoSQL connections across LAN)",
			}

			idx, _, status := tui.SelectMenu(wizardTitle, options)
			if status != 0 {
				return
			}
			if idx == 0 {
				accessScope = "local"
			} else {
				accessScope = "network"
			}
			currentStep = 2

		case 2:
			tui.ClearScreen()
			cardLines := []string{
				core.BOLD + core.CYAN + "MongoDB Network Port Configuration" + core.NC,
				"",
				"  Specify the TCP port for MongoDB client and replica connections.",
				"  " + core.BOLD + "Default Port:" + core.NC + " " + core.CYAN + "27017" + core.NC,
				"  Automatic conflict detection will verify port availability.",
			}
			tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/4: Port Selection", cardLines)

			portIn, st := tui.ReadTextInput("MongoDB Port", port)
			if st != 0 {
				currentStep = 1
				continue
			}
			if resolvedPort, ok := system.ResolvePortConflict(portIn, "tcp", true); !ok {
				currentStep = 1
				continue
			} else {
				port = resolvedPort
			}
			currentStep = 3

		case 3:
			tui.ClearScreen()
			credOptions := []string{
				"Generate Secure Random Admin Password (Recommended)",
				"Enter Custom Administrative Password",
			}
			cIdx, _, cStatus := tui.SelectMenu("Step 3/4: Administrative Root Credentials", credOptions)
			if cStatus == 1 || cStatus == 2 {
				currentStep = 2
				continue
			}

			if cIdx == 0 {
				if !isCustomPass {
					genPass, gErr := modules.GenerateRandomPass(16)
					if gErr != nil {
						logger.LogError(fmt.Sprintf("Failed to generate admin password: %v", gErr))
						return
					}
					adminPassword = genPass
				}
			} else {
				tui.ClearScreen()
				passCardLines := []string{
					core.BOLD + core.CYAN + "Administrative Root Password Setup" + core.NC,
					"",
					"  Enter a secure administrative password for MongoDB 'admin' user.",
					"  Input is masked with bullets for privacy.",
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back", "Step 3/4: Root Password", passCardLines)

				passIn, pSt := modules.ReadPasswordInput("Enter MongoDB Admin Password")
				if pSt != 0 || passIn == "" {
					continue
				}
				adminPassword = passIn
				isCustomPass = true
			}

			// Step 3.2: Optional Application Database & User
			tui.ClearScreen()
			appPromptLines := []string{
				core.BOLD + core.CYAN + "Application Database & User Provisioning (Optional)" + core.NC,
				"",
				"  You can automatically create a dedicated database and readWrite user",
				"  for your application or Node.js / Python web framework.",
				"",
				core.BOLD + core.YELLOW + "Provision a dedicated application database and user now?" + core.NC,
			}
			appIdx, _, appStatus := tui.SelectMenuWithContent("Step 3/4: Application Database Setup", appPromptLines, []string{"YES", "NO"})
			if appStatus == 1 || appStatus == 2 {
				currentStep = 2
				continue
			}

			createAppDB = (appIdx == 0)
			if createAppDB {
				tui.ClearScreen()
				appDBCardLines := []string{
					core.BOLD + core.CYAN + "Application Database Parameters" + core.NC,
					"",
					"  Provide database name and dedicated role credentials.",
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back", "Step 3/4: Application DB Parameters", appDBCardLines)

				dbNameIn, st := tui.ReadTextInput("Database Name", appDBName)
				if st != 0 {
					currentStep = 2
					continue
				}
				if dbNameIn != "" {
					appDBName = dbNameIn
				}

				dbUserIn, st2 := tui.ReadTextInput("Database Username", appDBUser)
				if st2 != 0 {
					currentStep = 2
					continue
				}
				if dbUserIn != "" {
					appDBUser = dbUserIn
				}

				dbPassIn, st3 := tui.ReadPasswordInput("Database User Password (blank for default)")
				if st3 != 0 {
					currentStep = 2
					continue
				}
				if dbPassIn != "" {
					appDBPass = dbPassIn
				}
			}

			currentStep = 4

		case 4:
			tui.ClearScreen()
			summaryLines := []string{
				tui.FormatKV("Deployment Target", func() string {
					if envType == "docker" {
						return "Docker Compose Container Stack"
					}
					return "Native Linux Service"
				}(), core.GREEN),
				tui.FormatKV("Access Scope", func() string {
					if accessScope == "network" {
						return "Local Network / LAN (0.0.0.0)"
					}
					return "Localhost Only (127.0.0.1)"
				}(), core.GREEN),
				tui.FormatKV("MongoDB Port", port, core.CYAN),
				tui.FormatKV("Admin User", adminUser, core.CYAN),
				tui.FormatKV("Admin Password", adminPassword, core.YELLOW),
			}

			if createAppDB {
				summaryLines = append(summaryLines, "")
				summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"APPLICATION DATABASE:"+core.NC)
				summaryLines = append(summaryLines, tui.FormatKV("Database Name", appDBName, core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("Database User", appDBUser, core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("User Password", appDBPass, core.YELLOW))
			}

			if envType == "native" {
				summaryLines = append(summaryLines, "")
				summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PERFORMANCE & STORAGE:"+core.NC)
				summaryLines = append(summaryLines, tui.FormatKV("Storage Engine", "WiredTiger NoSQL", core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("Data Directory", "/var/lib/mongodb", core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("Config Path", "/etc/mongod.conf", core.CYAN))
			} else {
				summaryLines = append(summaryLines, "")
				summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"DOCKER STORAGE & STACK:"+core.NC)
				summaryLines = append(summaryLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("Data Volume", "mongodb_data -> /data/db", core.CYAN))
			}

			summaryLines = append(summaryLines, "")
			summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"Do you want to proceed with installation?"+core.NC)

			choices := []string{"YES", "NO"}

			idx, _, status := tui.SelectMenuWithContent("Step 4/4: Installation Summary", summaryLines, choices)
			if status == 2 || idx != 0 {
				logger.LogInfo("Installation aborted by user.")
				return
			}
			if status == 1 {
				currentStep = 3
				continue
			}

			if envType == "native" {
				// On Alpine, the official MongoDB server package was removed from Alpine repos
				// due to SSPL licensing. Warn the user and offer Docker as the recommended path.
				if core.SysInfo.Family == "alpine" {
					tui.ClearScreen()
					alpineWarningLines := []string{
						core.BOLD + core.YELLOW + "ALPINE LINUX" + " " + core.Sym.EmDash + " " + "NATIVE MONGODB NOT AVAILABLE" + core.NC,
						"",
						"  The official MongoDB server package (mongod) was removed from",
						"  Alpine Linux repositories due to SSPL licensing restrictions.",
						"",
						"  " + core.BOLD + "Recommended:" + core.NC + " Use Docker Compose deployment instead.",
						"  Docker installation provides the full, official MongoDB image",
						"  without any repository limitation.",
						"",
						core.BOLD + core.CYAN + "Would you like to switch to Docker-based installation?" + core.NC,
					}
					swIdx, _, swStatus := tui.SelectMenuWithContent("Alpine Compatibility Notice", alpineWarningLines, []string{
						"YES" + " " + core.Sym.EmDash + " " + "Switch to Docker Installation (Recommended)",
						"NO" + " " + core.Sym.EmDash + " " + "Attempt Native Installation Anyway (May Fail)",
					})
					if swStatus == 2 {
						return
					}
					if swIdx == 0 || swStatus == 1 {
						ExecuteDockerInstall(accessScope, port, adminUser, adminPassword, createAppDB, appDBName, appDBUser, appDBPass, composeDir)
						return
					}
					// User chose to attempt native anyway — fall through
				}
				ExecuteNativeInstall(accessScope, port, adminUser, adminPassword, createAppDB, appDBName, appDBUser, appDBPass)
			} else {
				ExecuteDockerInstall(accessScope, port, adminUser, adminPassword, createAppDB, appDBName, appDBUser, appDBPass, composeDir)
			}
			return
		}
	}
}

// ExecuteNativeInstall installs MongoDB server natively on the host OS.
func ExecuteNativeInstall(accessScope, port, adminUser, adminPassword string, createAppDB bool, appDBName, appDBUser, appDBPass string) {
	// Mutual conflict check: Check if Docker MongoDB container is active
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", "krotek-mongod").Output(); err == nil && strings.TrimSpace(string(out)) == "running" {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "CONTAINER CONFLICT DETECTED" + core.NC,
			"",
			"  Docker MongoDB Container 'krotek-mongod' is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Native MongoDB, the Docker container stack must be stopped",
			"  to free network ports (Port " + port + ").",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop Docker MongoDB container stack to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Container Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Docker MongoDB container and continue Native deployment?")
		if res != 0 {
			logger.LogInfo("Native installation cancelled by user due to container conflict.")
			return
		}
		_ = exec.Command("docker", "stop", "krotek-mongod").Run()
	}

	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}

	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing MongoDB NoSQL Database (Native Service) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native MongoDB deployment...")

	// 1. Package Installation
	// On Alpine, the official mongod server package was removed from APK repos due to SSPL licensing.
	// Native MongoDB installation is not possible — abort immediately with guidance.
	if core.SysInfo.Family == "alpine" {
		tui.EnterAltScreen()
		tui.ClearScreen()
		tui.PrintInBox("MongoDB Native \u2014 Alpine Linux Not Supported", []string{
			core.BOLD + core.RED + "Native MongoDB server (mongod) cannot be installed on Alpine Linux." + core.NC,
			"",
			"  The official mongod package was removed from Alpine's APK repository",
			"  due to SSPL licensing restrictions. This affects all Alpine versions.",
			"  No partial or fallback native installation is available.",
			"",
			core.BOLD + core.YELLOW + "To run a MongoDB server on this machine, use Docker:" + core.NC,
			"  Run this tool again \u2192 select 'Install MongoDB via Docker Container Stack'",
			"  Docker provides the full official MongoDB image without any restrictions.",
			"",
			tui.FormatKV("Recommended Command", "mongodb-manager \u2192 Docker Install", core.GREEN),
		})
		logger.LogInfo("Alpine: native mongod not available. User directed to Docker path.")
		return
	}

	if core.SysInfo.Family == "rhel" {
		mongoVersion := "7.0"
		repoFile := fmt.Sprintf(`[mongodb-org-%s]
name=MongoDB Repository
baseurl=https://repo.mongodb.org/yum/redhat/$releasever/mongodb-org/%s/$basearch/
gpgcheck=1
enabled=1
gpgkey=https://pgp.mongodb.com/server-%s.asc
`, mongoVersion, mongoVersion, mongoVersion)
		_ = system.WriteFileNoFollow(fmt.Sprintf("/etc/yum.repos.d/mongodb-org-%s.repo", mongoVersion), []byte(repoFile), 0644, "")
	}

	var pkgs []string
	switch core.SysInfo.Family {
	case "debian":
		pkgs = []string{"mongodb-org", "mongodb-org-server", "mongodb-org-shell", "mongodb-org-tools"}
	case "rhel":
		pkgs = []string{"mongodb-org"}
	default:
		pkgs = []string{"mongodb-org"}
	}

	if !system.InstallPackages(pkgs...) {
		// Fallback for standard Ubuntu/Debian distro repos
		if core.SysInfo.Family == "debian" {
			_ = system.InstallPackages("mongodb")
		}
	}

	// 2. Configure /etc/mongod.conf or /etc/mongodb.conf
	bindIp := "127.0.0.1"
	if accessScope == "network" {
		bindIp = "0.0.0.0"
	}

	confFiles := []string{"/etc/mongod.conf", "/etc/mongodb.conf"}
	for _, cf := range confFiles {
		if data, err := os.ReadFile(cf); err == nil {
			content := string(data)
			content = regexp.MustCompile(`(?m)^\s*port:\s*[0-9]+`).ReplaceAllString(content, fmt.Sprintf("  port: %s", port))
			content = regexp.MustCompile(`(?m)^\s*bindIp:\s*.*`).ReplaceAllString(content, fmt.Sprintf("  bindIp: %s", bindIp))
			_ = system.WriteFileNoFollow(cf, []byte(content), 0644, "")
		}
	}

	// 3. Service Enablement
	if !system.StartService("mongod") {
		_ = system.StartService("mongodb")
	}

	time.Sleep(3 * time.Second)

	// 4. Create Administrative User & App DB via mongosh / mongo safely using JSON marshaling
	adminUserJSON, _ := json.Marshal(adminUser)
	adminPassJSON, _ := json.Marshal(adminPassword)

	jsCmd := fmt.Sprintf(`
db = db.getSiblingDB('admin');
db.createUser({
  user: %s,
  pwd: %s,
  roles: [ { role: 'userAdminAnyDatabase', db: 'admin' }, { role: 'readWriteAnyDatabase', db: 'admin' }, 'root' ]
});
`, string(adminUserJSON), string(adminPassJSON))

	if createAppDB && appDBName != "" && appDBUser != "" {
		appDBNameJSON, _ := json.Marshal(appDBName)
		appDBUserJSON, _ := json.Marshal(appDBUser)
		appDBPassJSON, _ := json.Marshal(appDBPass)
		jsCmd += fmt.Sprintf(`
db = db.getSiblingDB(%s);
db.createUser({
  user: %s,
  pwd: %s,
  roles: [ { role: 'readWrite', db: %s }, { role: 'dbAdmin', db: %s } ]
});
`, string(appDBNameJSON), string(appDBUserJSON), string(appDBPassJSON), string(appDBNameJSON), string(appDBNameJSON))
	}

	tmpDir, err := os.MkdirTemp("", "krotek-mongo-*")
	if err != nil {
		logger.LogError(fmt.Sprintf("Failed to create private temporary directory for MongoDB setup: %v", err))
		return
	}
	_ = os.Chmod(tmpDir, 0700)
	defer os.RemoveAll(tmpDir)

	jsFile := filepath.Join(tmpDir, "init.js")
	if wErr := system.WriteFileNoFollow(jsFile, []byte(jsCmd), 0600, ""); wErr != nil {
		logger.LogError(fmt.Sprintf("Failed to write MongoDB bootstrap script safely: %v", wErr))
		return
	}
	defer os.Remove(jsFile)

	if system.CommandExists("mongosh") {
		_ = exec.Command("mongosh", "--port", port, "--file", jsFile).Run()
	} else if system.CommandExists("mongo") {
		_ = exec.Command("mongo", "--port", port, jsFile).Run()
	}

	// 4b. Mandatory Security Hardening: Enable Authentication in mongod.conf
	for _, cf := range confFiles {
		if data, err := os.ReadFile(cf); err == nil {
			content := string(data)
			if !strings.Contains(content, "authorization: enabled") {
				if strings.Contains(content, "#security:") {
					content = strings.Replace(content, "#security:", "security:\n  authorization: enabled", 1)
				} else if regexp.MustCompile(`(?m)^security:`).MatchString(content) {
					content = regexp.MustCompile(`(?m)^security:.*`).ReplaceAllString(content, "security:\n  authorization: enabled")
				} else {
					content += "\nsecurity:\n  authorization: enabled\n"
				}
				_ = system.WriteFileNoFollow(cf, []byte(content), 0644, "")
			}
		}
	}

	// Restart daemon so authentication takes effect immediately
	if !system.RestartService("mongod") {
		_ = system.RestartService("mongodb")
	}

	// 5. Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	if accessScope == "network" {
		system.ConfigureFirewallRules(port)
	}

	// 6. Save State & Global Shortcut
	stateData := map[string]string{
		"ACTION":       "installed",
		"ENV_TYPE":     "native",
		"ACCESS_SCOPE": accessScope,
		"PORT":         port,
		"ADMIN_USER":   adminUser,
		"ADMIN_PASS":   adminPassword,
		"APP_DB":       appDBName,
		"APP_USER":     appDBUser,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	// 7. Display Success Box
	localIP := system.GetLocalIP()
	summaryLines := []string{
		tui.FormatKV("Deployment Mode", "Native Linux System Service", core.GREEN),
		tui.FormatKV("Access Scope", func() string {
			if accessScope == "network" {
				return fmt.Sprintf("LAN Network (Bound to %s:%s)", localIP, port)
			}
			return fmt.Sprintf("Localhost Only (Bound to 127.0.0.1:%s)", port)
		}(), core.GREEN),
		tui.FormatKV("Local Connection URI", fmt.Sprintf("mongodb://%s:%s@127.0.0.1:%s/admin", adminUser, adminPassword, port), core.CYAN),
	}

	if accessScope == "network" {
		summaryLines = append(summaryLines, tui.FormatKV("Remote Connection URI", fmt.Sprintf("mongodb://%s:%s@%s:%s/admin", adminUser, adminPassword, localIP, port), core.GREEN))
	}

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"CREDENTIALS & SECURITY:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Admin User", adminUser, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Admin Password", adminPassword, core.YELLOW+core.BOLD))

	if createAppDB {
		summaryLines = append(summaryLines, tui.FormatKV("App Database", appDBName, core.CYAN))
		summaryLines = append(summaryLines, tui.FormatKV("App User", appDBUser, core.CYAN))
		summaryLines = append(summaryLines, tui.FormatKV("App Password", appDBPass, core.YELLOW))
	}

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Data Storage", "/var/lib/mongodb", core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Installer Log", core.LogFilePath, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "mongodb-manager", core.GREEN))

	tui.EnterAltScreen()
	tui.ClearScreen()
	tui.PrintInBox("MongoDB Native Stack Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Native MongoDB installation completed.")
}

// detectCPUAVX returns (hasAVX2, hasAVX) by parsing the exact CPU flag fields
// from /proc/cpuinfo. Uses field-level matching to avoid false positives from
// flags like "avx512f" being mistaken for "avx".
func detectCPUAVX() (avx2 bool, avx bool) {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		lower := strings.ToLower(line)
		// "flags" on x86, "Features" on ARM
		if !strings.HasPrefix(lower, "flags") && !strings.HasPrefix(lower, "features") {
			continue
		}
		for _, field := range strings.Fields(line) {
			switch strings.ToLower(field) {
			case "avx2":
				avx2 = true
			case "avx":
				avx = true
			}
		}
		if avx2 || avx {
			break
		}
	}
	return
}

// fetchLatestMongoTag queries Docker Hub for the newest stable tag whose major version
// is within [minMajor, maxMajor]. Returns empty string on any error.
func fetchLatestMongoTag(minMajor, maxMajor int) string {
	client := &http.Client{Timeout: 5 * time.Second}
	url := "https://hub.docker.com/v2/repositories/library/mongo/tags?page_size=100&ordering=last_updated"

	resp, err := client.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var payload struct {
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ""
	}

	// semverRe matches tags like "7.0", "6.0.14", "7", but not "latest", "windowsservercore", etc.
	semverRe := regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\.\d+)*$`)

	type candidate struct {
		tag   string
		major int
		minor int
	}
	var candidates []candidate

	for _, r := range payload.Results {
		m := semverRe.FindStringSubmatch(r.Name)
		if m == nil {
			continue
		}
		maj, _ := strconv.Atoi(m[1])
		if maj < minMajor || maj > maxMajor {
			continue
		}
		min := 0
		if len(m) > 2 && m[2] != "" {
			min, _ = strconv.Atoi(m[2])
		}
		candidates = append(candidates, candidate{r.Name, maj, min})
	}

	if len(candidates) == 0 {
		return ""
	}

	// Sort: highest major first, then highest minor, prefer X.Y tags over plain X
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].major != candidates[j].major {
			return candidates[i].major > candidates[j].major
		}
		if candidates[i].minor != candidates[j].minor {
			return candidates[i].minor > candidates[j].minor
		}
		// Prefer tags with a dot (X.Y) over plain major-only tags
		return strings.Contains(candidates[i].tag, ".") && !strings.Contains(candidates[j].tag, ".")
	})

	return candidates[0].tag
}

// SelectMongoImage picks the best-compatible MongoDB Docker image based on CPU capabilities.
// It queries Docker Hub for the latest available tag and falls back to generic major-version
// tags (mongo:7, mongo:6) if offline. No version numbers are hardcoded.
func SelectMongoImage() string {
	// ARM64 — Docker's multi-arch manifest picks the right binary automatically
	if core.SysInfo.Architecture == "arm64" || core.SysInfo.Architecture == "aarch64" {
		if tag := fetchLatestMongoTag(6, 99); tag != "" {
			return "mongo:" + tag
		}
		return "mongo:latest"
	}

	avx2, avx := detectCPUAVX()

	switch {
	case avx2:
		// AVX2 — latest MongoDB (7+) is safe
		if tag := fetchLatestMongoTag(7, 99); tag != "" {
			return "mongo:" + tag
		}
		return "mongo:latest"

	case avx:
		// AVX without AVX2 — MongoDB 6.x is the last supported series
		if tag := fetchLatestMongoTag(6, 6); tag != "" {
			return "mongo:" + tag
		}
		return "mongo:6"

	default:
		// No AVX — only MongoDB 4.x runs without AVX (5.0+ requires AVX).
		if tag := fetchLatestMongoTag(4, 4); tag != "" {
			return "mongo:" + tag
		}
		return "mongo:4"
	}
}

// ExecuteDockerInstall provisions MongoDB via Docker Compose.
func ExecuteDockerInstall(accessScope, port, adminUser, adminPassword string, createAppDB bool, appDBName, appDBUser, appDBPass, composeDir string) {
	// Mutual conflict check: Check if Native MongoDB service is active
	if exec.Command("systemctl", "is-active", "--quiet", "mongod").Run() == nil || exec.Command("systemctl", "is-active", "--quiet", "mongodb").Run() == nil {
		tui.ClearScreen()
		conflictCard := []string{
			core.BOLD + core.YELLOW + "SERVICE CONFLICT DETECTED" + core.NC,
			"",
			"  Native MongoDB Linux Service is currently " + core.GREEN + "[ACTIVE / RUNNING]" + core.NC + ".",
			"  To deploy Docker MongoDB, the Native service must be stopped & disabled",
			"  to free network ports (Port " + port + ").",
			"",
			core.BOLD + core.CYAN + "Action Required:" + core.NC + " Stop & Disable Native MongoDB service to proceed?",
		}
		tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Cancel Installation", "Service Conflict Notice", conflictCard)

		res := tui.AskYesNo("Stop Native MongoDB service and continue Docker deployment?")
		if res != 0 {
			logger.LogInfo("Docker installation cancelled by user due to service conflict.")
			return
		}
		_ = exec.Command("systemctl", "stop", "mongod").Run()
		_ = exec.Command("systemctl", "stop", "mongodb").Run()
		_ = exec.Command("systemctl", "disable", "mongod").Run()
		_ = exec.Command("systemctl", "disable", "mongodb").Run()
	}

	if resolvedPort, ok := system.ResolvePortConflict(port, "tcp", true); !ok {
		return
	} else {
		port = resolvedPort
	}

	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing MongoDB (Docker Container Stack) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker MongoDB deployment...")

	if !system.EnsureDockerEngine() {
		return
	}

	_ = os.MkdirAll(composeDir, 0755)

	portBinding := fmt.Sprintf(`"%s:27017"`, port)
	if accessScope == "local" {
		portBinding = fmt.Sprintf(`"127.0.0.1:%s:27017"`, port)
	}

	envVars := fmt.Sprintf(`      MONGO_INITDB_ROOT_USERNAME: "%s"
      MONGO_INITDB_ROOT_PASSWORD: "%s"`, adminUser, adminPassword)

	if createAppDB && appDBName != "" {
		envVars += fmt.Sprintf("\n      MONGO_INITDB_DATABASE: \"%s\"", appDBName)
	}

	mongoImage := SelectMongoImage()

	composeYaml := fmt.Sprintf(`version: '3.8'

services:
  mongodb:
    image: %s
    container_name: krotek-mongod
    restart: unless-stopped
    ports:
      - %s
    environment:
%s
    volumes:
      - mongodb_data:/data/db

volumes:
  mongodb_data:
    name: krotek_mongodb_data
`, mongoImage, portBinding, envVars)

	composeFile := filepath.Join(composeDir, "docker-compose.yml")
	if err := system.WriteFileNoFollow(composeFile, []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write compose file: %v", err))
		return
	}

	if !system.RunDockerCompose(composeDir, "Deploying MongoDB Docker Stack", true, "up", "-d") {
		return
	}

	// Universal Firewall Configuration (UFW, Firewalld, iptables, ip6tables)
	if accessScope == "network" {
		system.ConfigureFirewallRules(port)
	}

	// Save State & Shortcut
	stateData := map[string]string{
		"ACTION":       "installed",
		"ENV_TYPE":     "docker",
		"COMPOSE_DIR":  composeDir,
		"ACCESS_SCOPE": accessScope,
		"PORT":         port,
		"ADMIN_USER":   adminUser,
		"ADMIN_PASS":   adminPassword,
		"APP_DB":       appDBName,
		"APP_USER":     appDBUser,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	// Display Success Box
	localIP := system.GetLocalIP()
	summaryLines := []string{
		tui.FormatKV("Deployment Mode", "Docker Compose Container Stack", core.GREEN),
		tui.FormatKV("Container Name", "krotek-mongod ("+mongoImage+")", core.GREEN),
		tui.FormatKV("Port Binding", func() string {
			if accessScope == "network" {
				return fmt.Sprintf("0.0.0.0:%s -> 27017/tcp", port)
			}
			return fmt.Sprintf("127.0.0.1:%s -> 27017/tcp", port)
		}(), core.GREEN),
		tui.FormatKV("Local Connection URI", fmt.Sprintf("mongodb://%s:%s@127.0.0.1:%s/admin", adminUser, adminPassword, port), core.CYAN),
	}

	if accessScope == "network" {
		summaryLines = append(summaryLines, tui.FormatKV("Remote Connection URI", fmt.Sprintf("mongodb://%s:%s@%s:%s/admin", adminUser, adminPassword, localIP, port), core.GREEN))
	}

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"CREDENTIALS & SECURITY:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Admin User", adminUser, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Admin Password", adminPassword, core.YELLOW+core.BOLD))

	if createAppDB {
		summaryLines = append(summaryLines, tui.FormatKV("App Database", appDBName, core.CYAN))
		summaryLines = append(summaryLines, tui.FormatKV("App User", appDBUser, core.CYAN))
		summaryLines = append(summaryLines, tui.FormatKV("App Password", appDBPass, core.YELLOW))
	}

	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"PATHS & MANAGEMENT:"+core.NC)
	summaryLines = append(summaryLines, tui.FormatKV("Compose Directory", composeDir, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Docker Volume", "krotek_mongodb_data", core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Installer Log", core.LogFilePath, core.CYAN))
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "mongodb-manager", core.GREEN))

	tui.EnterAltScreen()
	tui.ClearScreen()
	tui.PrintInBox("MongoDB Docker Stack Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Docker MongoDB installation completed.")
}

// ExecuteNativeToolsInstall installs MongoDB client utilities only (no server daemon).
// This is the appropriate action for Alpine and any system where only client tools are needed.
func ExecuteNativeToolsInstall() {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing MongoDB Client Tools ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting MongoDB client tools installation...")

	var toolPkgs []string
	switch core.SysInfo.Family {
	case "alpine":
		// On Alpine, only mongodb-tools is available in APK (mongod server was removed due to SSPL)
		toolPkgs = []string{"mongodb-tools"}
	case "debian":
		// mongodb-org-tools provides mongodump/restore/import/export; mongosh is separate
		toolPkgs = []string{"mongodb-org-tools", "mongosh"}
	case "rhel":
		toolPkgs = []string{"mongodb-org-tools", "mongodb-mongosh"}
	default:
		toolPkgs = []string{"mongodb-tools"}
	}

	system.UpdatePackageIndex()
	ok := system.InstallPackages(toolPkgs...)

	tui.EnterAltScreen()
	tui.ClearScreen()

	if !ok {
		tui.PrintInBox("MongoDB Client Tools" + " " + core.Sym.EmDash + " " + "Installation Warning", []string{
			core.BOLD + core.YELLOW + "Some packages may not have installed correctly." + core.NC,
			"",
			"  This can happen if the MongoDB APT/APK/YUM repository is not configured",
			"  or the package names differ on your distribution.",
			"",
			core.BOLD + core.CYAN + "Attempted packages:" + core.NC,
			"  " + strings.Join(toolPkgs, ", "),
			"",
			core.BOLD + core.YELLOW + "To connect to a MongoDB server, use Docker:" + core.NC,
			"  Run this tool again" + " " + core.Sym.ArrowRight + " " + "select 'Install MongoDB via Docker Container Stack'",
			tui.FormatKV("Recommended", "mongodb-manager" + " " + core.Sym.ArrowRight + " " + "Docker Install", core.GREEN),
		})
		logger.LogInfo("MongoDB client tools install attempted with partial/no success.")
		return
	}

	// Build list of detected tools
	toolChecks := []struct{ name, bin string }{
		{"mongodump", "mongodump"},
		{"mongorestore", "mongorestore"},
		{"mongoimport", "mongoimport"},
		{"mongoexport", "mongoexport"},
		{"mongosh / mongo", "mongosh"},
	}
	var installedTools []string
	for _, t := range toolChecks {
		if system.CommandExists(t.bin) {
			installedTools = append(installedTools, "  \u2714 "+t.name)
		}
	}

	summaryLines := []string{
		tui.FormatKV("Installation Type", "MongoDB Client Tools (No Server Daemon)", core.GREEN),
		tui.FormatKV("Operating System", system.GetOSPrettyVersion(), core.CYAN),
		"",
		core.BOLD + core.CYAN + "Installed Tools:" + core.NC,
	}
	if len(installedTools) > 0 {
		summaryLines = append(summaryLines, installedTools...)
	} else {
		summaryLines = append(summaryLines, "  (verify manually" + " " + core.Sym.EmDash + " " + "tools may require shell reload)")
	}
	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"Usage Examples:"+core.NC)
	summaryLines = append(summaryLines, "  mongodump --host <host> --port 27017 --out /backup/")
	summaryLines = append(summaryLines, "  mongorestore --host <host> --port 27017 /backup/")
	summaryLines = append(summaryLines, "  mongosh mongodb://<user>:<pass>@<host>:27017/admin")
	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, core.BOLD+core.GREEN+"To run a MongoDB SERVER on this machine:"+core.NC)
	summaryLines = append(summaryLines, "  Run this tool again \u2192 select 'Install MongoDB via Docker Container Stack'")

	tui.PrintInBox("MongoDB Client Tools Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("MongoDB client tools installation completed.")
}

// PromptExistingConfigAction checks if prior configuration files exist on the host system.
// In interactive TUI mode, it prompts the administrator in English to decide whether to preserve existing custom settings
// or perform a clean factory default reset. In unattended mode (-y / CI/CD), it automatically preserves settings (safe default).
func PromptExistingConfigAction(serviceTitle string, configPaths ...string) {
	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" {
		return
	}

	hasExisting := false
	for _, p := range configPaths {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				hasExisting = true
				break
			}
		}
	}
	if !hasExisting {
		return
	}

	tui.ClearScreen()
	cardLines := []string{
		core.BOLD + core.YELLOW + "EXISTING CONFIGURATION DETECTED" + core.NC,
		"",
		fmt.Sprintf("  Prior configuration files were detected for %s on this host.", serviceTitle),
		"  Choose how the deployment wizard should handle existing settings:",
		"",
		"  " + core.GREEN + "[1] Keep Existing Configuration (Recommended)" + core.NC,
		"      Preserves all existing configuration, virtual hosts, and custom data intact.",
		"",
		"  " + core.YELLOW + "[2] Clean Reset to Factory Default (Overwrite)" + core.NC,
		"      Overwrites configuration with fresh, production-hardened defaults.",
	}

	choices := []string{
		"Keep Existing Configuration (Preserve custom settings & virtual hosts)",
		"Clean Reset to Factory Default (Overwrite with fresh configurations)",
	}

	idx, _, status := tui.SelectMenuWithContent("Configuration Conflict Handler", cardLines, choices)
	if status == 2 {
		tui.LeaveAltScreen()
		logger.LogInfo("Setup cancelled by user at configuration conflict prompt.")
		os.Exit(0)
	}
	if status == 0 && idx == 1 {
		core.OverwriteExistingConfig = true
		logger.LogInfo("User selected: Clean reset to factory default configuration.")
	} else {
		core.OverwriteExistingConfig = false
		logger.LogInfo("User selected: Preserve existing configuration.")
	}
}
