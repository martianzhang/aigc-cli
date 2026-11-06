//go:build !windows

package terminal

import (
	"bytes"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestClearOnInterruptCleansUp(t *testing.T) {
	var buf bytes.Buffer
	rep := &Progress{w: &buf, enabled: true, bar: true}
	rep.Percent(30)
	buf.Reset()
	handled := make(chan struct{})
	clearOnInterrupt(rep, func() { close(handled) })

	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}
	select {
	case <-handled:
	case <-time.After(2 * time.Second):
		t.Fatal("interrupt handler never ran")
	}
	if got, want := buf.String(), "\x1b]9;4;0\x07\x1b]0;aigc-cli\x07"; got != want {
		t.Errorf("interrupt cleanup = %q, want %q", got, want)
	}
}

// TestClearOnInterruptRunsHookBeforeReraise guards that child cleanup happens
// while the process is still alive.
func TestClearOnInterruptRunsHookBeforeReraise(t *testing.T) {
	var buf bytes.Buffer
	rep := &Progress{w: &buf, enabled: true, bar: true}
	hookRan := false
	InterruptHook = func() { hookRan = true }
	t.Cleanup(func() { InterruptHook = nil })

	handled := make(chan struct{})
	clearOnInterrupt(rep, func() {
		if !hookRan {
			t.Error("InterruptHook did not run before SIGINT was re-raised")
		}
		close(handled)
	})

	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}
	select {
	case <-handled:
	case <-time.After(2 * time.Second):
		t.Fatal("interrupt handler never ran")
	}
}
