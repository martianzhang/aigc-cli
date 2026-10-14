package gif

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExtractLastFrameOptions 是一次「抽取视频尾帧」的输入参数。
type ExtractLastFrameOptions struct {
	Input   string // 输入视频路径
	Output  string // 输出图片路径，空时默认 <stem>_lastframe.png
	Verbose bool   // 打印额外调试信息
}

// ExtractLastFrame 把视频的最后一帧存为图片（默认 PNG）。返回输出路径。
// 不修改输入文件。始终把实际执行的 ffmpeg 命令打印到 stdout（命令行等价物）。
//
// 实现：先 -sseof -1 只解码最后 1 秒，再用 reverse 滤镜反转（只缓冲这 1 秒，
// 内存廉价），取反转后的第 1 帧即为真正的尾帧。
func ExtractLastFrame(opts ExtractLastFrameOptions) (string, error) {
	if !Available() {
		return "", MissingHint()
	}
	input := strings.TrimSpace(opts.Input)
	if input == "" {
		return "", fmt.Errorf("input is required")
	}
	out := opts.Output
	if out == "" {
		out = defaultLastFrameOutput(input)
	}

	args := []string{"-y", "-sseof", "-1", "-i", input,
		"-vf", "reverse", "-frames:v", "1", "-update", "1", out}

	fmt.Printf("ffmpeg %s\n", strings.Join(quoteAll(args), " "))

	cmd := exec.Command("ffmpeg", args...)
	if outBuf, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg last-frame extraction failed: %w\n%s", err, string(outBuf))
	}
	return out, nil
}

// defaultLastFrameOutput 依据输入文件名生成输出路径：<stem>_lastframe.png
// （与输入同目录）。
func defaultLastFrameOutput(input string) string {
	ext := filepath.Ext(input)
	stem := strings.TrimSuffix(filepath.Base(input), ext)
	return filepath.Join(filepath.Dir(input), stem+"_lastframe.png")
}
