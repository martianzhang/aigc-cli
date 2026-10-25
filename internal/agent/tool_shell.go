package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
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
func ShellCommandContext(ctx context.Context, cmdLine string) string {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if runtime.GOOS == "windows" {
		// Windows 默认输出编码为 GBK/CP936，需切换到 UTF-8 避免中文乱码
		switch {
		case HasExecutable("pwsh"):
			cmdLine = "$OutputEncoding = [Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " + cmdLine
			cmd := exec.CommandContext(ctx, "pwsh", "-NoProfile", "-Command", cmdLine)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("Error: %v\n%s", err, string(out))
			}
			return string(out)
		case HasExecutable("powershell"):
			cmdLine = "$OutputEncoding = [Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " + cmdLine
			cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", cmdLine)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("Error: %v\n%s", err, string(out))
			}
			return string(out)
		default:
			cmdLine = "chcp 65001 >NUL & " + cmdLine
			cmd := exec.CommandContext(ctx, "cmd", "/c", cmdLine)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("Error: %v\n%s", err, string(out))
			}
			return string(out)
		}
	} else {
		switch {
		case HasExecutable("zsh"):
			cmd := exec.CommandContext(ctx, "zsh", "-c", cmdLine)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("Error: %v\n%s", err, string(out))
			}
			return string(out)
		case HasExecutable("bash"):
			cmd := exec.CommandContext(ctx, "bash", "-c", cmdLine)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("Error: %v\n%s", err, string(out))
			}
			return string(out)
		default:
			cmd := exec.CommandContext(ctx, "sh", "-c", cmdLine)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("Error: %v\n%s", err, string(out))
			}
			return string(out)
		}
	}
}
