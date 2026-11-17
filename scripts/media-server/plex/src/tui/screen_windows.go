//go:build windows

// ==============================================================================
// KROTEK SCRIPT HUB — WINDOWS CONSOLE API FALLBACK HANDLERS
// Service: PLEX (media-server)
// File:    web-server/nginx/src/tui/screen_windows.go
//
// Provides fallback terminal mode stubs for Windows development environments and cross-compilation.
// ==============================================================================

package tui

import "time"

func getIoctlDimensions() (int, int) {
	return 0, 0
}

func waitInputTimeout(fd int, timeout time.Duration) bool {
	return true
}
