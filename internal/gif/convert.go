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
	Input      string      // 输入视频路径
	Output     string      // 输出 GIF 路径
	Width      int         // 输出宽度（px），<=0 保持原尺寸
	CropMargin CropMargins // 从各边裁掉的像素数（CSS margin 语义），零值不裁
	Verbose    bool        // 打印额外调试信息
	ExtraArgs  []string    // 用户追加的 ffmpeg 参数（--ffmpeg-flags 逃生门），追加在 filter 之后
}

// Convert 把单个视频转换为 GIF。返回输出路径。
// 始终把实际执行的 ffmpeg 命令打印到 stdout（命令行等价物）。
// CropMargin 非零时先按 CropMargins 从各边精确裁切，之后只做等比缩放。
func Convert(opts ConvertOptions) (string, error) {
	if !Available() {
		return "", MissingHint()
	}
	out := opts.Output
	if out == "" {
		out = defaultOutput(opts.Input, opts.Width)
	}

	// 用 ffprobe 探测源尺寸以校验 crop-margin 是否过大（与 ffmpeg 同包安装，几乎总是可用）。
	srcW, srcH := 0, 0
	if !opts.CropMargin.Zero() {
		srcW, srcH = probeVideoSize(opts.Input)
		if srcW > 0 && srcH > 0 &&
			(opts.CropMargin.Left+opts.CropMargin.Right >= srcW || opts.CropMargin.Top+opts.CropMargin.Bottom >= srcH) {
			return "", fmt.Errorf("crop-margin %s too large for %dx%d video", opts.CropMargin, srcW, srcH)
		}
	}

	args := []string{"-y", "-i", opts.Input, "-vf", buildFilter(opts.Width, opts.CropMargin)}
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

// probeVideoSize 用 ffprobe 读取视频宽高；ffprobe 不可用或读取失败时返回 0,0。
func probeVideoSize(input string) (int, int) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return 0, 0
	}
	out, err := exec.Command("ffprobe", "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0", input).Output()
	if err != nil {
		return 0, 0
	}
	var w, h int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d,%d", &w, &h); err != nil {
		return 0, 0
	}
	return w, h
}

// quoteAll 给参数切片逐项加引号。
func quoteAll(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = quote(a)
	}
	return out
}
