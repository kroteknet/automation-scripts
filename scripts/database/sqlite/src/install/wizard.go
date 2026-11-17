// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED INSTALLATION WIZARD & PROVISIONER
// Service: SQLite Embedded Database Toolkit (sqlite)
//
// This package orchestrates automated pre-flight checks, package repository setup,
// configuration templating, service enablement (systemd/openrc), and firewall rules.
// Supports both interactive TUI step-by-step guidance and headless automation.
// ==============================================================================

package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"krotek-sqlite-installer/core"
	"krotek-sqlite-installer/logger"
	"krotek-sqlite-installer/system"
	"krotek-sqlite-installer/tui"
)

// FreshInstallWizard handles the end-to-end installation pipeline for SQLite Embedded Database Toolkit.
// In unattended mode, it consumes environment variables (KROTEK_PORT, KROTEK_MODE, etc.).
// In interactive mode, it presents prompt dialogs for user customization.
func FreshInstallWizard(targetEnv string) {
	envType := targetEnv
	if envType == "" {
		envType = "native"
	}

	dbStorageDir := "/var/lib/sqlite"
	sampleDBName := "app.db"
	createSampleDB := true
	composeDir := "/etc/sqlite-docker"
	webPort := "8080"

	// ─── UNATTENDED / NON-INTERACTIVE AUTOMATION MODE ─────────────────────────
	if core.AutoYes || os.Getenv("KROTEK_NON_INTERACTIVE") == "1" || os.Getenv("NON_INTERACTIVE") == "1" {
		if envMode := os.Getenv("KROTEK_MODE"); envMode != "" {
			envType = envMode
		}
		logger.LogInfo(fmt.Sprintf("Starting unattended installation: mode=%s", envType))
		if envType == "native" {
			ExecuteNativeInstall(dbStorageDir, createSampleDB, sampleDBName)
		} else {
			ExecuteDockerInstall(webPort, composeDir)
		}
		return
	}
	PromptExistingConfigAction("SQLite Database", "/etc/sqlite")

	currentStep := 1

	for {
		switch currentStep {
		case 1:
			tui.ClearScreen()
			wizardTitle := "Step 1/3: Choose Deployment Mode (SQLite 3)"

			options := []string{
				"Native SQLite3 CLI Engine & Dev Libraries (Recommended - System Native)",
				"Docker SQLite Web UI Management Stack (Browser GUI on Port 8080)",
			}

			idx, _, status := tui.SelectMenu(wizardTitle, options)
			if status != 0 {
				return
			}
			if idx == 0 {
				envType = "native"
			} else {
				envType = "docker"
			}
			currentStep = 2

		case 2:
			tui.ClearScreen()
			if envType == "native" {
				cardLines := []string{
					core.BOLD + core.CYAN + "SQLite Database Directory & Sample Storage" + core.NC,
					"",
					"  Specify the directory where SQLite database files (.db/.sqlite)",
					"  will be stored on this server.",
					"",
					"  " + core.BOLD + "Default Location:" + core.NC + " " + core.CYAN + "/var/lib/sqlite" + core.NC,
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/3: Database Path Setup", cardLines)

				dirIn, st := tui.ReadTextInput("Database Storage Path", dbStorageDir)
				if st != 0 {
					currentStep = 1
					continue
				}
				if dirIn != "" {
					dirIn = strings.TrimSpace(dirIn)
					if err := system.ValidateSafeDirectoryPath(dirIn); err != nil {
						tui.PrintInBox("Invalid Database Storage Path", []string{
							core.RED + err.Error() + core.NC,
						})
						continue
					}
					dbStorageDir = dirIn
				}

				res := tui.AskYesNo("Initialize a starter database (" + sampleDBName + ") with sample table?")
				createSampleDB = (res == 0)
			} else {
				cardLines := []string{
					core.BOLD + core.CYAN + "SQLite Web Browser Stack Port" + core.NC,
					"",
					"  Specify the HTTP web port for SQLite-Web GUI dashboard.",
					"  " + core.BOLD + "Default Port:" + core.NC + " " + core.CYAN + "8080" + core.NC,
				}
				tui.PrintInBox("--no-wait", "--help-str", "[Esc]  ->  Back to Step 1", "Step 2/3: Web Port Setup", cardLines)

				portIn, st := tui.ReadTextInput("Web UI Port", webPort)
				if st != 0 {
					currentStep = 1
					continue
				}
				if resolvedPort, ok := system.ResolvePortConflict(portIn, "tcp", true); !ok {
					currentStep = 1
					continue
				} else {
					webPort = resolvedPort
				}
			}
			currentStep = 3

		case 3:
			tui.ClearScreen()
			summaryLines := []string{
				tui.FormatKV("Deployment Target", func() string {
					if envType == "docker" {
						return "Docker SQLite-Web Container Stack"
					}
					return "Native SQLite3 C-Engine & Tools"
				}(), core.GREEN),
				tui.FormatKV("Architecture", core.SysInfo.Architecture, core.CYAN),
			}

			if envType == "native" {
				summaryLines = append(summaryLines, tui.FormatKV("Database Directory", dbStorageDir, core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("Sample Database", func() string {
					if createSampleDB {
						return filepath.Join(dbStorageDir, sampleDBName)
					}
					return "[NO] (Directory only)"
				}(), core.GREEN))
			} else {
				summaryLines = append(summaryLines, tui.FormatKV("Web UI Port", webPort, core.YELLOW))
				summaryLines = append(summaryLines, tui.FormatKV("Compose Dir", composeDir, core.CYAN))
				summaryLines = append(summaryLines, tui.FormatKV("Web UI URL", fmt.Sprintf("http://%s:%s", system.GetLocalIP(), webPort), core.GREEN))
			}

			summaryLines = append(summaryLines, "")
			summaryLines = append(summaryLines, core.BOLD+core.YELLOW+"Do you want to proceed with installation?"+core.NC)

			choices := []string{"YES", "NO"}

			idx, _, status := tui.SelectMenuWithContent("Step 3/3: Installation Summary", summaryLines, choices)
			if status == 2 || idx != 0 {
				logger.LogInfo("Installation aborted by user.")
				return
			}
			if status == 1 {
				currentStep = 2
				continue
			}

			if envType == "native" {
				ExecuteNativeInstall(dbStorageDir, createSampleDB, sampleDBName)
			} else {
				ExecuteDockerInstall(webPort, composeDir)
			}
			return
		}
	}
}

// ExecuteNativeInstall installs SQLite3 natively.
func ExecuteNativeInstall(dbStorageDir string, createSampleDB bool, sampleDBName string) {
	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing SQLite3 (Native System Engine & CLI) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Native SQLite3 deployment...")

	var pkgs []string
	switch core.SysInfo.Family {
	case "debian":
		pkgs = []string{"sqlite3", "libsqlite3-dev"}
	case "rhel":
		pkgs = []string{"sqlite", "sqlite-devel"}
	case "alpine":
		pkgs = []string{"sqlite", "sqlite-dev"}
	default:
		pkgs = []string{"sqlite3", "libsqlite3-dev"}
	}

	if !system.InstallPackages(pkgs...) {
		logger.LogError("Failed to install SQLite packages.")
		return
	}

	_ = os.MkdirAll(dbStorageDir, 0755)

	if createSampleDB {
		samplePath := filepath.Join(dbStorageDir, sampleDBName)
		initSQL := `CREATE TABLE IF NOT EXISTS system_info (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key TEXT UNIQUE NOT NULL,
    value TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
INSERT OR IGNORE INTO system_info (key, value) VALUES ('installer', 'Krotek Script Hub');
INSERT OR IGNORE INTO system_info (key, value) VALUES ('engine', 'SQLite3');
`
		if !system.IsSymlinkPath(samplePath) {
			initCmd := exec.Command("sqlite3", samplePath)
			initCmd.Stdin = strings.NewReader(initSQL)
			_ = initCmd.Run()
		}
	}

	stateData := map[string]string{
		"ACTION":    "installed",
		"ENV_TYPE":  "native",
		"DATA_DIR":  dbStorageDir,
		"SAMPLE_DB": sampleDBName,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	summaryLines := []string{
		tui.FormatKV("Deployment Mode", "Native SQLite3 Engine & CLI", core.GREEN),
		tui.FormatKV("CLI Binary Tool", "/usr/bin/sqlite3", core.CYAN),
		tui.FormatKV("Storage Directory", dbStorageDir, core.CYAN),
	}
	if createSampleDB {
		summaryLines = append(summaryLines, tui.FormatKV("Starter DB File", filepath.Join(dbStorageDir, sampleDBName), core.GREEN))
		summaryLines = append(summaryLines, tui.FormatKV("Quick Interactive CLI", fmt.Sprintf("sqlite3 %s", filepath.Join(dbStorageDir, sampleDBName)), core.CYAN))
	}
	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, tui.FormatKV("Management Command", "sqlite-manager", core.GREEN))

	tui.EnterAltScreen()
	tui.ClearScreen()
	tui.PrintInBox("SQLite3 Native Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Native SQLite3 installation completed.")
}

// ExecuteDockerInstall installs SQLite Web GUI via Docker.
func ExecuteDockerInstall(webPort, composeDir string) {
	if resolvedPort, ok := system.ResolvePortConflict(webPort, "tcp", true); !ok {
		return
	} else {
		webPort = resolvedPort
	}

	tui.LeaveAltScreen()
	tui.ClearScreen()
	fmt.Printf("\r\n%s=== Installing SQLite Web GUI (Docker Stack) ===%s\r\n\r\n", core.BOLD+core.YELLOW, core.NC)
	logger.LogInfo("Starting Docker SQLite deployment...")

	if !system.EnsureDockerEngine() {
		return
	}

	_ = os.MkdirAll(composeDir, 0755)
	dataDir := filepath.Join(composeDir, "data")
	_ = os.MkdirAll(dataDir, 0755)

	composeYaml := fmt.Sprintf(`version: '3.8'

services:
  sqlite-web:
    image: coleifer/sqlite-web:latest
    container_name: krotek-sqlite-web
    restart: unless-stopped
    ports:
      - "%s:8080"
    volumes:
      - ./data:/data
    environment:
      - SQLITE_DATABASE=/data/app.db

`, webPort)

	composeFile := filepath.Join(composeDir, "docker-compose.yml")
	if err := system.WriteFileNoFollow(composeFile, []byte(composeYaml), 0600, ""); err != nil {
		logger.LogError(fmt.Sprintf("Failed to write compose file: %v", err))
		return
	}

	if !system.RunDockerCompose(composeDir, "Deploying SQLite Web GUI Stack", true, "up", "-d") {
		return
	}

	stateData := map[string]string{
		"ACTION":      "installed",
		"ENV_TYPE":    "docker",
		"COMPOSE_DIR": composeDir,
		"WEB_PORT":    webPort,
	}
	system.SaveState(stateData)
	system.CreateGlobalShortcut()

	localIP := system.GetLocalIP()
	summaryLines := []string{
		tui.FormatKV("Deployment Mode", "Docker SQLite Web UI Stack", core.GREEN),
		tui.FormatKV("Web UI Dashboard", fmt.Sprintf("http://%s:%s", localIP, webPort), core.GREEN),
		tui.FormatKV("Localhost URL", fmt.Sprintf("http://127.0.0.1:%s", webPort), core.CYAN),
		tui.FormatKV("Compose Directory", composeDir, core.CYAN),
		tui.FormatKV("Database Mount", filepath.Join(dataDir, "app.db"), core.CYAN),
		tui.FormatKV("Management Command", "sqlite-manager", core.GREEN),
	}

	tui.EnterAltScreen()
	tui.ClearScreen()
	tui.PrintInBox("SQLite Web Stack Installed Successfully", summaryLines)

	fmt.Print("\r\n")
	logger.LogSuccess("Docker SQLite deployment completed.")
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
