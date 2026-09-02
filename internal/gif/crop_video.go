package gif

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// CropVideoOptions 是一次视频裁边（去水印）的输入参数。
type CropVideoOptions struct {
	Input      string      // 输入视频路径
	Output     string      // 输出视频路径，空时默认 <stem>_crop<ext>
	CropMargin CropMargins // 从各边裁掉的像素数（CSS margin 语义），零值不裁
	Verbose    bool        // 打印额外调试信息
	ExtraArgs  []string    // 用户追加的 ffmpeg 参数，追加在 filter 之后
}

// CropVideo 把单个视频按 CropMargins 从各边精确裁切，输出新的 mp4 文件。
// 不修改输入文件。返回输出路径。
// 始终把实际执行的 ffmpeg 命令打印到 stdout（命令行等价物）。
func CropVideo(opts CropVideoOptions) (string, error) {
	if !Available() {
		return "", MissingHint()
	}
	out := opts.Output
	if out == "" {
		out = defaultCropOutput(opts.Input)
	}

	srcW, srcH := 0, 0
	if !opts.CropMargin.Zero() {
		srcW, srcH = probeVideoSize(opts.Input)
		if srcW > 0 && srcH > 0 &&
			(opts.CropMargin.Left+opts.CropMargin.Right >= srcW || opts.CropMargin.Top+opts.CropMargin.Bottom >= srcH) {
			return "", fmt.Errorf("crop-margin %s too large for %dx%d video", opts.CropMargin, srcW, srcH)
		}
	}

	args := []string{"-y", "-i", opts.Input, "-vf", buildCropFilter(opts.CropMargin),
		"-c:v", "libx264", "-preset", "medium", "-crf", "23", "-pix_fmt", "yuv420p", "-movflags", "+faststart"}
	args = append(args, opts.ExtraArgs...)
	args = append(args, out)

	fmt.Printf("ffmpeg %s\n", strings.Join(quoteAll(args), " "))

	cmd := exec.Command("ffmpeg", args...)
	if outBuf, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg video crop failed: %w\n%s", err, string(outBuf))
	}
	return out, nil
}

// defaultCropOutput 依据输入文件名生成输出路径：<stem>_crop<ext>（与输入同目录）。
func defaultCropOutput(input string) string {
	ext := filepath.Ext(input)
	stem := strings.TrimSuffix(filepath.Base(input), ext)
	return filepath.Join(filepath.Dir(input), stem+"_crop"+ext)
}
