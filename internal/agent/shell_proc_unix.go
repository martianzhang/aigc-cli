//go:build unix

package agent

import (
	"os/exec"
	"syscall"
)

// configureShellProcess puts the shell in its own process group so a timeout or
// cancellation can terminate the whole tree (shell + children), not just the
// shell itself.
func configureShellProcess(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killShellProcess sends SIGKILL to the shell's process group.
func killShellProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
