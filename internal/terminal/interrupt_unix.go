//go:build !windows

package terminal

import (
	"os"
	"os/signal"
	"syscall"
)

// reRaiseInterrupt restores the default SIGINT disposition and delivers it
// again, so the process dies the way the shell expects (exit status 130)
// instead of being swallowed by our handler.
func reRaiseInterrupt() {
	signal.Reset(os.Interrupt)
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
}
