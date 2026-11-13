package gif

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/argutil"
)

// Available reports whether ffmpeg is on PATH.
func Available() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// MissingHint returns a two-line, copy-pasteable error. Keep it short: the CLI
// prints "Error: <line 1>" and "Hint: <line 2>", and agents should not need to
// scroll through per-platform install guides.
func MissingHint() error {
	return fmt.Errorf("ffmpeg not found in PATH\nHint: install ffmpeg (macOS: brew install ffmpeg; Ubuntu/Debian: sudo apt install ffmpeg; Windows: winget install Gyan.FFmpeg)")
}

// SplitExtraArgs 把空格分隔的额外 ffmpeg 参数拆为切片（支持引号包裹的值）。
func SplitExtraArgs(s string) []string {
	return argutil.Split(s)
}

// quote 给含空格/引号的参数加引号，用于可读的 stdout 回显。
func quote(s string) string {
	if strings.ContainsAny(s, " \t\"'") {
		return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
	}
	return s
}
