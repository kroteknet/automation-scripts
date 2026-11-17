//go:build windows

// ==============================================================================
// KROTEK SCRIPT HUB — SAFE FILE OPERATIONS FALLBACK (WINDOWS)
// Service: OpenResty (web-server)
// File:    web-server/openresty/src/system/files_windows.go
// ==============================================================================

package system

import (
	"fmt"
	"os"
)

func openFileNoFollow(filePath string, perm os.FileMode) (*os.File, error) {
	if fi, err := os.Lstat(filePath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to follow symlink at %s", filePath)
	}
	return os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
}

