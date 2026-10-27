//go:build windows

package terminal

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVirtualTerminal turns on ANSI escape processing for the console behind f.
//
// Windows Terminal always has it on, but a legacy conhost window (plain
// powershell.exe/cmd.exe) starts with it off — writing OSC sequences there would
// print them as garbage instead of setting the title. When the mode cannot be
// enabled we simply stay silent.
func enableVirtualTerminal(f *os.File) bool {
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
