//go:build !windows

// ==============================================================================
// KROTEK SCRIPT HUB — UNIX/LINUX TERMINAL SYSCALL HANDLERS
// Service: REDIS (database)
// File:    web-server/nginx/src/tui/screen_unix.go
//
// Provides low-level termios and ioctl syscall wrappers for Linux/BSD systems to control terminal raw mode and terminal dimensions.
// ==============================================================================

package tui

import (
	"os"
	"syscall"
	"time"
	"unsafe"
)

func getIoctlDimensions() (int, int) {
	ws := &winsize{}
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(os.Stdout.Fd()), uintptr(0x5413), uintptr(unsafe.Pointer(ws))); err == 0 {
		if ws.Col > 0 && ws.Row > 0 {
			return int(ws.Col), int(ws.Row)
		}
	}
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(os.Stdin.Fd()), uintptr(0x5413), uintptr(unsafe.Pointer(ws))); err == 0 {
		if ws.Col > 0 && ws.Row > 0 {
			return int(ws.Col), int(ws.Row)
		}
	}
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(os.Stderr.Fd()), uintptr(0x5413), uintptr(unsafe.Pointer(ws))); err == 0 {
		if ws.Col > 0 && ws.Row > 0 {
			return int(ws.Col), int(ws.Row)
		}
	}
	return 0, 0
}

func waitInputTimeout(fd int, timeout time.Duration) bool {
	if timeout <= 0 {
		return true
	}
	var fds syscall.FdSet
	fds.Bits[fd/64] |= 1 << (uint(fd) % 64)
	tv := syscall.NsecToTimeval(timeout.Nanoseconds())
	n, err := syscall.Select(fd+1, &fds, nil, nil, &tv)
	return err == nil && n > 0
}
