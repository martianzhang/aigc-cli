//go:build !unix && !windows

package agent

import "os/exec"

// configureShellProcess is a no-op on platforms without process groups.
func configureShellProcess(_ *exec.Cmd) {}

// killShellProcess falls back to killing the shell process alone.
func killShellProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
