package modules

import (
	"testing"
)

func TestSyncWireGuardConfig_RejectsUnsafeInterface(t *testing.T) {
	bad := []string{
		"",
		"wg0; rm -rf /",
		"wg0$(id)",
		"wg0`id`",
		"../wg0",
		"wg 0",
		"wg0\nid",
		"this-interface-name-is-way-too-long",
	}
	for _, iface := range bad {
		if err := SyncWireGuardConfig(iface); err == nil {
			t.Errorf("SyncWireGuardConfig(%q) succeeded, expected validation error", iface)
		}
	}
}
