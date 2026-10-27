//go:build windows

package terminal

import "os"

// reRaiseInterrupt cannot re-deliver a console Ctrl-C, so exit with the same
// status the shell would report for a SIGINT death.
func reRaiseInterrupt() { os.Exit(130) }
