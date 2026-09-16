//go:build windows

package app

import (
	"os"
	"syscall"
	"unsafe"
)

// enableVirtualTerminal turns on ANSI escape processing for the console so
// live stream previews can clear/redraw the current line (`\r\x1b[2K`).
// Without this, every stream delta appends a new visual line and thinking
// looks like it is printed repeatedly.
func enableVirtualTerminal() {
	h := os.Stdout.Fd()
	if h == 0 || h == uintptr(syscall.InvalidHandle) {
		return
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procGet := kernel32.NewProc("GetConsoleMode")
	procSet := kernel32.NewProc("SetConsoleMode")
	var mode uint32
	r, _, _ := procGet.Call(h, uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		return
	}
	const enableVT = 0x0004
	_, _, _ = procSet.Call(h, uintptr(mode|enableVT))
}
