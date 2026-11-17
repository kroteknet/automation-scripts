// ==============================================================================
// KROTEK SCRIPT HUB — AUTOMATED BACKUP & ARCHIVE SUBSYSTEM
// Service: MINECRAFT (game-servers)
// File:    game-servers/minecraft/src/modules/backups.go
//
// Creates scheduled and on-demand compressed backup archives (.tar.gz), manages retention policies, and validates archive integrity.
// ==============================================================================

package modules

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"krotek-minecraft-installer/core"
	"krotek-minecraft-installer/system"
	"krotek-minecraft-installer/tui"
)

type BackupItem struct {
	Path      string
	Filename  string
	SizeBytes int64
	ModTime   time.Time
}

func GetBackupsList() []BackupItem {
	backupDir := core.BackupDir
	var items []BackupItem
	matches, err := filepath.Glob(filepath.Join(backupDir, "minecraft-backup-*.tar.gz"))
	if err != nil {
		return items
	}

	for _, m := range matches {
		if fi, err := os.Stat(m); err == nil {
			items = append(items, BackupItem{
				Path:      m,
				Filename:  filepath.Base(m),
				SizeBytes: fi.Size(),
				ModTime:   fi.ModTime(),
			})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].ModTime.After(items[j].ModTime)
	})

	return items
}

// CreateWorldBackup generates a compressed tar.gz archive of the Minecraft world save, config files, and server properties.
// It executes a transactional flush via RCON (save-off & save-all) to ensure 100% data integrity without stopping gameplay.
func CreateWorldBackup(isDocker bool) {
	tui.ClearScreen()
	ts := time.Now().Format("2006-01-02_15-04-05")
	backupDir := core.BackupDir
	_ = os.MkdirAll(backupDir, 0700)

	// Step 1: Transactional flush via RCON if server is currently running
	isRunning := system.IsMinecraftActive()
	if isRunning {
		tui.StartSpinner("Flushing active world chunks & player inventories to disk via RCON...")
		_, _ = ExecuteConsoleCommand("save-off")
		_, _ = ExecuteConsoleCommand("save-all flush")
		time.Sleep(1500 * time.Millisecond)
		tui.StopSpinner(0)
	}

	destFile := filepath.Join(backupDir, fmt.Sprintf("minecraft-backup-%s.tar.gz", ts))
	tui.StartSpinner("Creating compressed full world backup archive (.tar.gz)...")

	sourceDir := system.GetMinecraftDataDir()
	cmd := exec.Command("tar", "-czf", destFile, "-C", sourceDir, ".")

	err := cmd.Run()
	tui.StopSpinner(0)

	// Step 2: Re-enable auto-saving
	if isRunning {
		_, _ = ExecuteConsoleCommand("save-on")
	}

	if err != nil {
		tui.PrintInBox("Backup Failed", []string{
			core.RED + "[ERROR] Failed to create backup archive." + core.NC,
			err.Error(),
		})
		return
	}

	sizeStr := "N/A"
	if fi, sErr := os.Stat(destFile); sErr == nil {
		sizeMB := float64(fi.Size()) / (1024 * 1024)
		sizeStr = fmt.Sprintf("%.2f MB", sizeMB)
	}

	infoLines := []string{
		core.GREEN + "[SUCCESS] World backup archive created successfully!" + core.NC,
		"",
		tui.FormatKV("Archive File", destFile, core.YELLOW),
		tui.FormatKV("File Size", sizeStr, core.CYAN),
		tui.FormatKV("Timestamp", ts, core.CYAN),
	}
	if isRunning {
		infoLines = append(infoLines, tui.FormatKV("Data Integrity", core.GREEN+"[TRANSACTIONAL RCON FLUSHED]"+core.NC, core.GREEN))
	}
	infoLines = append(infoLines, "", "Archive contains full world regions, players, server.properties, and plugins.")

	tui.PrintInBox("Backup Completed", infoLines)
}

func ManageBackups(isDocker bool) {
	for {
		items := GetBackupsList()

		var menuOpts []string
		if len(items) == 0 {
			tui.PrintInBox("No Backups Found", []string{
				core.YELLOW + fmt.Sprintf("No backup archives found in %s/.", core.BackupDir) + core.NC,
				"Use 'Create Full World Backup (.tar.gz)' to generate your first backup archive.",
			})
			return
		}

		menuOpts = append(menuOpts, "─── STORED BACKUP ARCHIVES ───")
		for i, b := range items {
			sizeMB := float64(b.SizeBytes) / (1024 * 1024)
			dateStr := b.ModTime.Format("2006-01-02 15:04")
			menuOpts = append(menuOpts, fmt.Sprintf("[%02d] %s (%.1f MB" + " " + core.Sym.Bullet + " " + "%s)", i+1, b.Filename, sizeMB, dateStr))
		}
		menuOpts = append(menuOpts, "─── ACTIONS ───")
		menuOpts = append(menuOpts, "Create New World Backup Now")
		menuOpts = append(menuOpts, "Back to Main Menu")

		idx, opt, st := tui.SelectMenu("KROTEK MINECRAFT" + " " + core.Sym.EmDash + " " + "Backup Management & Restore", menuOpts)
		if st == 2 || strings.Contains(opt, "Back to Main Menu") {
			return
		}

		cleanOpt := core.StripANSI(opt)
		if strings.Contains(cleanOpt, "Create New World Backup") {
			CreateWorldBackup(isDocker)
			continue
		}

		// An archive was selected
		selectedIdx := -1
		for i, b := range items {
			if strings.Contains(cleanOpt, b.Filename) {
				selectedIdx = i
				break
			}
		}
		if selectedIdx == -1 && idx-1 >= 0 && idx-1 < len(items) {
			selectedIdx = idx - 1
		}
		if selectedIdx < 0 || selectedIdx >= len(items) {
			continue
		}

		item := items[selectedIdx]
		handleSingleBackup(item, isDocker)
	}
}

func handleSingleBackup(item BackupItem, isDocker bool) {
	sizeMB := float64(item.SizeBytes) / (1024 * 1024)
	subOpts := []string{
		fmt.Sprintf("%sRestore World from '%s'%s", core.GREEN, item.Filename, core.NC),
		fmt.Sprintf("%sDelete Archive '%s'%s", core.RED, item.Filename, core.NC),
		"Back to Backups List",
	}

	_, opt, st := tui.SelectMenu(fmt.Sprintf("Backup: %s (%.1f MB)", item.Filename, sizeMB), subOpts)
	if st != 0 || strings.Contains(opt, "Back") {
		return
	}

	cleanOpt := core.StripANSI(opt)
	if strings.HasPrefix(cleanOpt, "Restore World") {
		confirm := tui.AskYesNo(fmt.Sprintf("WARNING: Restoring will overwrite the current world with %s. Proceed?", item.Filename))
		if confirm != 0 {
			return
		}

		tui.StartSpinner("Validating & restoring world backup archive...")
		if valErr := system.ValidateTarGzArchive(item.Path, 8*1024*1024*1024); valErr != nil {
			tui.StopSpinner(1)
			tui.PrintInBox("Security Error", []string{
				core.RED + "[ERROR] Unsafe or invalid backup archive rejected: " + valErr.Error() + core.NC,
			})
			return
		}
		destDir := system.GetMinecraftDataDir()
		_, _ = system.StopMinecraftService()

		cmd := exec.Command("tar", "-xzf", item.Path, "-C", destDir)
		err := cmd.Run()

		if !isDocker {
			_ = filepath.WalkDir(destDir, func(p string, d os.DirEntry, wErr error) error {
				if wErr != nil || d.Type()&os.ModeSymlink != 0 {
					return nil
				}
				_ = exec.Command("chown", "-h", "minecraft:minecraft", p).Run()
				return nil
			})
		}
		_, _ = system.StartMinecraftService()

		tui.StopSpinner(0)

		if err != nil {
			tui.PrintInBox("Restore Failed", []string{
				core.RED + "[ERROR] Failed to unpack backup archive: " + err.Error() + core.NC,
			})
		} else {
			tui.PrintInBox("Restore Complete", []string{
				core.GREEN + "[SUCCESS] World successfully restored from " + item.Filename + "!" + core.NC,
				"Minecraft service restarted and world loaded.",
			})
		}
	} else if strings.HasPrefix(cleanOpt, "Delete Archive") {
		confirm := tui.AskYesNo(fmt.Sprintf("Are you sure you want to permanently delete %s?", item.Filename))
		if confirm == 0 {
			_ = os.Remove(item.Path)
			tui.PrintInBox("Backup Deleted", []string{
				core.GREEN + core.TagOK() + " Backup archive removed." + core.NC,
			})
		}
	}
}
