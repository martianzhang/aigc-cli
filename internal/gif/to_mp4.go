package gif

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ToMP4Options 是把任意 ffmpeg 可解码媒体（GIF/WebP/MOV/…）转换为 MP4 的输入参数。
type ToMP4Options struct {
	Input      string      // 输入媒体路径
	Output     string      // 输出 MP4 路径，空时默认 <stem>.mp4（与输入同目录）
	CropMargin CropMargins // 从各边裁掉的像素数（CSS margin 语义），零值不裁
	Verbose    bool        // 打印额外调试信息
	ExtraArgs  []string    // 用户追加的 ffmpeg 参数（--ffmpeg-flags 逃生门），追加在 filter 之后
}

// ToMP4 把单个媒体文件转换为 MP4（H.264 + yuv420p，兼容性最好）。返回输出路径。
// 保持源分辨率，仅把宽高规整为偶数（H.264 要求）。
// 不覆盖输入文件——输出路径与输入相同时返回错误。
// 源带音轨时自动保留（不传 -an），源无音轨则不产出音轨；不强制帧率（不传 -r），保留源时序。
// 始终把实际执行的 ffmpeg 命令打印到 stdout（命令行等价物）。
func ToMP4(opts ToMP4Options) (string, error) {
	if !Available() {
		return "", MissingHint()
	}
	out := opts.Output
	if out == "" {
		out = defaultMP4Output(opts.Input)
	}
	if samePath(opts.Input, out) {
		return "", fmt.Errorf("input is already MP4: %s", filepath.Base(opts.Input))
	}

	// 用 ffprobe 探测源尺寸以校验 crop-margin 是否过大（与 ffmpeg 同包安装，几乎总是可用）。
	if !opts.CropMargin.Zero() {
		srcW, srcH := probeVideoSize(opts.Input)
		if srcW > 0 && srcH > 0 &&
			(opts.CropMargin.Left+opts.CropMargin.Right >= srcW || opts.CropMargin.Top+opts.CropMargin.Bottom >= srcH) {
			return "", fmt.Errorf("crop-margin %s too large for %dx%d input", opts.CropMargin, srcW, srcH)
		}
	}

	args := []string{"-y", "-i", opts.Input,
		"-vf", buildMP4Filter(opts.CropMargin),
		"-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p",
		"-movflags", "+faststart"}
	args = append(args, opts.ExtraArgs...)
	args = append(args, out)

	// 把命令行回显到 stdout，用户可直接复制复现。
	fmt.Printf("ffmpeg %s\n", strings.Join(quoteAll(args), " "))

	cmd := exec.Command("ffmpeg", args...)
	if outBuf, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg mp4 conversion failed: %w\n%s%s", err, string(outBuf), decodeHint(opts.Input))
	}
	return out, nil
}

// defaultMP4Output 依据输入文件名生成输出路径：<stem>.mp4（与输入同目录，保留原文件）。
func defaultMP4Output(input string) string {
	ext := filepath.Ext(input)
	stem := strings.TrimSuffix(filepath.Base(input), ext)
	return filepath.Join(filepath.Dir(input), stem+".mp4")
}

// buildMP4Filter 构造媒体转 MP4 的 -vf filter 链。
// 保持源分辨率，仅把宽高规整为偶数（H.264/yuv420p 要求）。
// crop 非零时先按 CropMargins 精确裁切，再取偶。
func buildMP4Filter(crop CropMargins) string {
	var parts []string
	if !crop.Zero() {
		parts = append(parts, buildCropFilter(crop))
	}
	parts = append(parts, "scale=trunc(iw/2)*2:trunc(ih/2)*2:flags=lanczos")
	return strings.Join(parts, ",")
}

// samePath 判断两个路径是否指向同一位置（基于绝对路径 + Clean；解析失败时退化为字面比较）。
func samePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}

// classifyInput 按扩展名粗略归类输入，仅用于生成更友好的失败提示，不影响 ffmpeg 命令构造。
// ffmpeg 自身会探测真实格式，这里只是给少数对 ffmpeg build 敏感的格式分类。
func classifyInput(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".gif":
		return "gif"
	case ".webp":
		return "webp"
	case ".apng", ".png":
		return "apng"
	case ".mp4", ".mov", ".mkv", ".webm", ".avi", ".flv", ".m4v", ".ts", ".wmv", ".mpeg", ".mpg":
		return "video"
	default:
		return "media"
	}
}

// decodeHint 针对少数依赖 ffmpeg build 的格式给出额外排错提示（可能为空）。
func decodeHint(path string) string {
	switch classifyInput(path) {
	case "webp":
		return "\n  hint: animated WebP decoding depends on your ffmpeg build (libwebp); upgrade ffmpeg if this fails"
	case "apng":
		return "\n  hint: APNG decoding requires the apng demuxer in your ffmpeg build"
	default:
		return ""
	}
}
