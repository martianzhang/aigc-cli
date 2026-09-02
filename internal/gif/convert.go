package gif

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ConvertOptions 是一次视频→GIF 转换的输入参数。
// FPS / MaxColors / Dither 固化在 buildFilter 常量中，不在此暴露。
type ConvertOptions struct {
	Input     string   // 输入视频路径
	Output    string   // 输出 GIF 路径
	Width     int      // 输出宽度（px），<=0 保持原尺寸
	Verbose   bool     // 打印额外调试信息
	ExtraArgs []string // 用户追加的 ffmpeg 参数（--ffmpeg-flags 逃生门），追加在 filter 之后
}

// Convert 把单个视频转换为 GIF。返回输出路径。
// 始终把实际执行的 ffmpeg 命令打印到 stdout（命令行等价物）。
func Convert(opts ConvertOptions) (string, error) {
	if !Available() {
		return "", MissingHint()
	}
	out := opts.Output
	if out == "" {
		out = defaultOutput(opts.Input, opts.Width)
	}

	args := []string{"-y", "-i", opts.Input, "-vf", buildFilter(opts.Width)}
	args = append(args, opts.ExtraArgs...)
	args = append(args, out)

	// 把命令行回显到 stdout，用户可直接复制复现。
	fmt.Printf("ffmpeg %s\n", strings.Join(quoteAll(args), " "))

	cmd := exec.Command("ffmpeg", args...)
	if outBuf, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg gif conversion failed: %w\n%s", err, string(outBuf))
	}
	return out, nil
}

// defaultOutput 依据输入文件名生成输出路径：<stem>_<width>px.gif。
// width<=0 时用 <stem>.gif。
func defaultOutput(input string, width int) string {
	ext := filepath.Ext(input)
	stem := strings.TrimSuffix(filepath.Base(input), ext)
	if width > 0 {
		return stem + fmt.Sprintf("_%dpx.gif", width)
	}
	return stem + ".gif"
}

// quoteAll 给参数切片逐项加引号。
func quoteAll(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = quote(a)
	}
	return out
}
