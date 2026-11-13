//go:build windows

package agent

import (
	"context"
	"os/exec"
	"strconv"
)

// configureShellProcess is a no-op on Windows: taskkill /T walks the child tree
// by parent PID, so no special creation flags are needed.
func configureShellProcess(_ *exec.Cmd) {}

// killShellProcess terminates the shell and its descendants. taskkill is part
// of Windows itself, so this does not add an external dependency.
func killShellProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	killer := exec.CommandContext(context.Background(), "taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	if err := killer.Run(); err == nil {
		return nil
	}
	return cmd.Process.Kill()
}
