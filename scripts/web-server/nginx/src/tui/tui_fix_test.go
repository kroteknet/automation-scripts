package tui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"krotek-nginx-installer/core"
	"krotek-nginx-installer/logger"
)

func TestPadOrTruncateWidthAndGuard(t *testing.T) {
	if got := PadOrTruncate("hello", 0); got != "" {
		t.Fatalf("expected empty string for targetW=0, got %q", got)
	}
	if got := PadOrTruncate("hello", -3); got != "" {
		t.Fatalf("expected empty string for targetW=-3, got %q", got)
	}
	got := PadOrTruncate(core.GREEN+"abcdefghij"+core.NC, 6)
	if w := core.RuneWidth(got); w != 6 {
		t.Fatalf("expected width 6, got %d (%q)", w, got)
	}
	if !strings.Contains(got, core.GREEN) {
		t.Fatalf("expected ANSI color preserved in %q", got)
	}
}

func captureStdout(fn func()) string {
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

func TestBoxSpinnerSuppressesNestedLoggerAndSpinner(t *testing.T) {
	core.AutoYes = false
	out := captureStdout(func() {
		SetTaskBoxTitle("Test Progress")
		boxNeedsHeader = true
		StartBoxSpinner("Creating backup")
		logger.LogSuccess("Safe backup archive created: /tmp/backup.tar.gz")
		StartSpinner("Sub-step running")
		time.Sleep(150 * time.Millisecond)
		StopSpinner(0)
		StopBoxSpinner(0, "Backup created successfully.")
	})

	if core.BoxSpinnerActive.Load() {
		t.Fatal("BoxSpinnerActive should be false after StopBoxSpinner")
	}
	if core.SpinnerActive.Load() {
		t.Fatal("SpinnerActive should be false")
	}
	if strings.Contains(out, "Safe backup archive created") {
		t.Fatalf("LogSuccess leaked to stdout while BoxSpinner was active: %q", out)
	}
	if !strings.Contains(out, "Sub-step running") {
		t.Fatalf("expected nested StartSpinner message inside box spinner output")
	}
	if !strings.Contains(out, "Backup created successfully.") {
		t.Fatalf("expected final StopBoxSpinner status in output")
	}
}

func TestGetLastLogLineSinceIgnoresPriorEntries(t *testing.T) {
	dir := t.TempDir()
	logPath := dir + string(os.PathSeparator) + "install.log"
	prior := "[2026-11-15T12:22:31+01:00] [WARN] Ensuring domain points to this server\n[2026-11-15T12:22:32+01:00] [ERROR] Failed! Check log file\n"
	if err := os.WriteFile(logPath, []byte(prior), 0600); err != nil {
		t.Fatal(err)
	}
	offset := getLogFileSize(logPath)
	if got := getLastLogLineSince(logPath, offset, 40); got != "" {
		t.Fatalf("expected empty line before new step output, got %q", got)
	}
	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("Reloading nginx worker processes\n")
	_ = f.Close()
	if got := getLastLogLineSince(logPath, offset, 40); got != "Reloading nginx worker processes" {
		t.Fatalf("expected new step line, got %q", got)
	}
}
