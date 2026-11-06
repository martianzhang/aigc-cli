package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// shellWaitDelay bounds how long Wait keeps waiting for stdout/stderr after the
// process was killed. A detached grandchild that inherited the pipes could
// otherwise keep the tool call blocked long past its deadline.
const shellWaitDelay = 5 * time.Second

// Bash parses a {"command": "...", "timeout": N} tool call and executes it via
// shellCommandContext. Privileged primitive (requires explicit tools_enable
// opt-in).
func Bash(argsJSON string) string {
	return BashContext(context.Background(), argsJSON)
}

// BashContext is Bash with a caller-supplied context, so a cancelled turn can
// stop the child process instead of waiting for the default timeout.
//
// timeout (seconds) is optional: when present it overrides the user-configured
// default (defaults.chat.bash.timeout, 300s unless set).
func BashContext(ctx context.Context, argsJSON string) string {
	var args struct {
		Command string  `json:"command"`
		Timeout float64 `json:"timeout"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if strings.TrimSpace(args.Command) == "" {
		return "Error: command is required"
	}
	timeout, err := parseShellTimeout(args.Timeout)
	if err != nil {
		return "Error: " + err.Error()
	}
	return shellCommandContext(ctx, args.Command, timeout)
}

func ShellCommand(cmdLine string) string {
	return ShellCommandContext(context.Background(), cmdLine)
}

// ShellCommandContext runs cmdLine with the configured default timeout derived
// from ctx. It is used by the TUI `!command` shortcut and by callers that do not
// have a model-supplied timeout.
//
// The shell is resolved per platform (see detectShell): a POSIX shell wherever
// available — including Git Bash/busybox on Windows — otherwise PowerShell or
// cmd.exe. Non-POSIX shells get the small compatibility shim in
// shell_platform.go so common Unix idioms (/dev/null, head/tail, …) still work.
func ShellCommandContext(ctx context.Context, cmdLine string) string {
	return shellCommandContext(ctx, cmdLine, 0)
}

// shellCommandContext runs cmdLine with requested (when positive) or the
// configured default timeout.
func shellCommandContext(ctx context.Context, cmdLine string, requested time.Duration) string {
	timeout := resolveShellTimeout(requested)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	spec := detectShell()
	command := prepareCommand(spec, cmdLine)

	args := make([]string, 0, len(spec.args)+1)
	args = append(args, spec.args...)
	args = append(args, command)

	cmd := exec.CommandContext(ctx, spec.path, args...)
	// Run the shell in its own process group so a timeout or cancellation kills
	// the whole tree. Killing only the shell leaves `sh -c` children (and
	// anything they spawned) running, possibly still holding stdout/stderr.
	configureShellProcess(cmd)
	cmd.Cancel = func() error { return killShellProcess(cmd) }
	// A detached grandchild can outlive the group kill and keep the output
	// pipes open; bound that wait so the tool always returns at its deadline.
	cmd.WaitDelay = shellWaitDelay
	// Child shells only understand proxy environment variables; propagate the
	// effective --http-proxy / config http_proxy / --no-proxy choice.
	cmd.Env = shellCommandEnv()
	// Keep at most MaxToolOutputBytes of combined stdout/stderr. CombinedOutput
	// would buffer the entire result, so a command that writes gigabytes before
	// the timeout could exhaust memory before any cap is applied.
	out := newCappedOutput(MaxToolOutputBytes - shellOutputMarkerReserve)
	cmd.Stdout = out
	cmd.Stderr = out
	err := runShellCommand(cmd)
	text := out.String()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Sprintf(
				"Error: command timed out after %s. If it needs longer, pass `timeout` (seconds) in the bash call or raise defaults.chat.bash.timeout.\n%s%s",
				timeout, text, shellFailureHint(spec))
		}
		return fmt.Sprintf("Error: %v\n%s%s", err, text, shellFailureHint(spec))
	}
	return text
}
