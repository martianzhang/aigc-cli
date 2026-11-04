package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Bash parses a {"command": "..."} tool call and executes it via ShellCommand.
// Privileged primitive (requires explicit tools_enable opt-in).
func Bash(argsJSON string) string {
	return BashContext(context.Background(), argsJSON)
}

// BashContext is Bash with a caller-supplied context, so a cancelled turn can
// stop the child process instead of waiting for the 30s fallback timeout.
func BashContext(ctx context.Context, argsJSON string) string {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if strings.TrimSpace(args.Command) == "" {
		return "Error: command is required"
	}
	return ShellCommandContext(ctx, args.Command)
}

func ShellCommand(cmdLine string) string {
	return ShellCommandContext(context.Background(), cmdLine)
}

// ShellCommandContext runs cmdLine with a 30s timeout derived from ctx.
//
// The shell is resolved per platform (see detectShell): a POSIX shell wherever
// available — including Git Bash/busybox on Windows — otherwise PowerShell or
// cmd.exe. Non-POSIX shells get the small compatibility shim in
// shell_platform.go so common Unix idioms (/dev/null, head/tail, …) still work.
func ShellCommandContext(ctx context.Context, cmdLine string) string {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	spec := detectShell()
	command := prepareCommand(spec, cmdLine)

	args := make([]string, 0, len(spec.args)+1)
	args = append(args, spec.args...)
	args = append(args, command)

	cmd := exec.CommandContext(ctx, spec.path, args...)
	// Child shells only understand proxy environment variables; propagate the
	// effective --http-proxy / config http_proxy / --no-proxy choice.
	cmd.Env = shellCommandEnv()
	// Keep at most MaxToolOutputBytes of combined stdout/stderr. CombinedOutput
	// would buffer the entire result, so a command that writes gigabytes before
	// the 30s timeout could exhaust memory before any cap is applied.
	out := newCappedOutput(MaxToolOutputBytes - shellOutputMarkerReserve)
	cmd.Stdout = out
	cmd.Stderr = out
	err := cmd.Run()
	text := out.String()
	if err != nil {
		return fmt.Sprintf("Error: %v\n%s%s", err, text, shellFailureHint(spec))
	}
	return text
}
