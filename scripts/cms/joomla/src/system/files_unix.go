//go:build !windows

// ==============================================================================
// KROTEK SCRIPT HUB — SAFE SYMLINK-RESISTANT FILE OPERATIONS (UNIX)
// Service: OpenResty (web-server)
// File:    web-server/openresty/src/system/files_unix.go
// ==============================================================================

package system

import (
	"os"
	"syscall"
)

func openFileNoFollow(filePath string, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, perm)
}

