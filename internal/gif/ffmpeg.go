package gif

import (
	"os/exec"
	"strings"
)

// Available reports whether ffmpeg is on PATH.
func Available() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// MissingHint returns an error describing how to install ffmpeg on this platform.
func MissingHint() error {
	return &missingFFmpegError{}
}

// SplitExtraArgs 把空格分隔的额外 ffmpeg 参数拆为切片（支持引号包裹的值）。
func SplitExtraArgs(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var args []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}

// missingFFmpegError 标记 ffmpeg 未安装这一可识别错误。
type missingFFmpegError struct{}

func (e *missingFFmpegError) Error() string {
	return `ffmpeg not found in PATH.

GIF conversion requires ffmpeg (aigc-cli does NOT bundle it).
Install it on your platform:

  macOS:    brew install ffmpeg
  Ubuntu:   sudo apt install ffmpeg
  Debian:   sudo apt-get install ffmpeg
  Fedora:   sudo dnf install ffmpeg
  Arch:     sudo pacman -S ffmpeg
  Windows:  winget install Gyan.FFmpeg
            # or: choco install ffmpeg

Verify with: ffmpeg -version`
}

// quote 给含空格/引号的参数加引号，用于可读的 stdout 回显。
func quote(s string) string {
	if strings.ContainsAny(s, " \t\"'") {
		return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
	}
	return s
}
