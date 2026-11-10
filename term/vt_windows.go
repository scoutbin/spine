//go:build windows

package term

import (
	"os"
	"syscall"
)

const enableVirtualTerminalProcessing = 0x0004

var procSetConsoleMode = syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleMode")

// enableVT turns on ANSI escape processing for the console behind f.
func enableVT(f *os.File) bool {
	handle := syscall.Handle(f.Fd())
	var mode uint32
	if err := syscall.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	ret, _, _ := procSetConsoleMode.Call(uintptr(handle), uintptr(mode|enableVirtualTerminalProcessing))
	return ret != 0
}
