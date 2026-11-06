package agent

import (
	"os/exec"
	"sync"
)

// activeShells tracks running bash commands so an interrupt can stop their
// process groups. Without this, Ctrl+C in a non-interactive run would kill the
// CLI while a command in its own process group kept running.
var activeShells struct {
	mu   sync.Mutex
	cmds map[*exec.Cmd]struct{}
}

// runShellCommand starts cmd (which gives it a PID), keeps it registered for
// interrupt cleanup, and waits for it to exit.
func runShellCommand(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	unregister := registerShellProcess(cmd)
	defer unregister()
	return cmd.Wait()
}

// registerShellProcess records cmd until the returned function is called.
func registerShellProcess(cmd *exec.Cmd) func() {
	activeShells.mu.Lock()
	if activeShells.cmds == nil {
		activeShells.cmds = make(map[*exec.Cmd]struct{})
	}
	activeShells.cmds[cmd] = struct{}{}
	activeShells.mu.Unlock()

	return func() {
		activeShells.mu.Lock()
		delete(activeShells.cmds, cmd)
		activeShells.mu.Unlock()
	}
}

// KillActiveShells terminates every bash command currently running. It is
// wired into the CLI's SIGINT handler so Ctrl+C does not leave process groups
// behind.
func KillActiveShells() {
	activeShells.mu.Lock()
	cmds := make([]*exec.Cmd, 0, len(activeShells.cmds))
	for cmd := range activeShells.cmds {
		cmds = append(cmds, cmd)
	}
	activeShells.mu.Unlock()

	for _, cmd := range cmds {
		_ = killShellProcess(cmd)
	}
}

// activeShellCount reports how many commands are registered (tests only).
func activeShellCount() int {
	activeShells.mu.Lock()
	defer activeShells.mu.Unlock()
	return len(activeShells.cmds)
}
