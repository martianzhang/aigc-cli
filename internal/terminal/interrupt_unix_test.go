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
