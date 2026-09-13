package agent

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

func ShellCommand(cmdLine string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
