package tui

import (
	"strings"
	"testing"

	"krotek-nitrox-installer/core"
)

func TestFormatStyledLogLines(t *testing.T) {
	empty := FormatStyledLogLines("   \n\n", "No logs")
	if len(empty) != 1 || !strings.Contains(core.StripANSI(empty[0]), "No logs") {
		t.Fatalf("unexpected empty fallback: %v", empty)
	}

	logs := FormatStyledLogLines("2026-01-01T12:00:00Z ERROR failed to bind\nGET \" 200 ok\nWARN slow query", "")
	if len(logs) != 3 {
		t.Fatalf("expected 3 formatted lines, got %d", len(logs))
	}
	if !strings.Contains(logs[0], core.RED) {
		t.Errorf("expected error line to use RED color: %q", logs[0])
	}
	if !strings.Contains(logs[1], core.GREEN) {
		t.Errorf("expected 200 status line to use GREEN color: %q", logs[1])
	}
	if !strings.Contains(logs[2], core.YELLOW) {
		t.Errorf("expected warn line to use YELLOW color: %q", logs[2])
	}
}

func TestFormatDockerContainerStatus(t *testing.T) {
	empty := FormatDockerContainerStatus("", "Nothing running")
	if len(empty) != 1 || !strings.Contains(core.StripANSI(empty[0]), "Nothing running") {
		t.Fatalf("unexpected empty docker fallback: %v", empty)
	}

	raw := "NAME                STATUS          PORTS\nkrotek-app          Up 2 hours      0.0.0.0:80->80/tcp\nkrotek-db           Exited (1)      "
	out := FormatDockerContainerStatus(raw, "")
	if len(out) != 4 {
		t.Fatalf("expected header + divider + 2 rows, got %d", len(out))
	}
	if !strings.Contains(out[0], core.CYAN) {
		t.Errorf("expected cyan header: %q", out[0])
	}
	if !strings.Contains(out[2], core.GREEN) {
		t.Errorf("expected green running row: %q", out[2])
	}
	if !strings.Contains(out[3], core.RED) {
		t.Errorf("expected red exited row: %q", out[3])
	}
}

func TestFormatKeyValueOutput(t *testing.T) {
	raw := "interface: wg0\n  public key: ABC=\n  private key: (hidden)\n  listening port: 51820\n"
	out := FormatKeyValueOutput(raw, "Empty")
	if len(out) != 4 {
		t.Fatalf("expected 4 formatted lines, got %d: %v", len(out), out)
	}
	if !strings.Contains(out[0], "INTERFACE") || !strings.Contains(out[0], core.CYAN) {
		t.Errorf("expected styled interface header: %q", out[0])
	}
	if !strings.Contains(out[2], core.DIM) {
		t.Errorf("expected dimmed hidden key: %q", out[2])
	}
}

func TestFormatSystemctlStatusOutput_EmptyAndInactive(t *testing.T) {
	empty := FormatSystemctlStatusOutput("demo", "")
	if len(empty) != 1 || !strings.Contains(core.StripANSI(empty[0]), "No systemd status output") {
		t.Fatalf("expected informative empty fallback, got: %v", empty)
	}

	inactive := FormatSystemctlStatusOutput("demo", "○ demo.service - Demo\n   Loaded: loaded\n   Active: inactive (dead)")
	if len(inactive) == 0 || !strings.Contains(inactive[0], core.RED) {
		t.Fatalf("expected red dot for inactive unit, got: %v", inactive)
	}
}
