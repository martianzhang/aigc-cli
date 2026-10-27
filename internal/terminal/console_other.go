//go:build !windows

package terminal

import "os"

// enableVirtualTerminal is a no-op off Windows: a POSIX terminal already speaks
// ANSI/OSC, and term.IsTerminal has vetted the writer.
func enableVirtualTerminal(*os.File) bool { return true }
