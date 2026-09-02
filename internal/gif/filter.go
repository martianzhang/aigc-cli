package gif

import "fmt"

// 默认转换配方（用户验证过的最优参数，固化为常量，不暴露 flag）。
// 对应 ffmpeg 两遍调色板链：fps → scale → palettegen → paletteuse。
const (
	// DefaultFPS 抽帧帧率（每秒帧数）。
	DefaultFPS = 6
	// DefaultMaxColors palettegen 调色板最大色数。
	DefaultMaxColors = 128
	// DefaultDither paletteuse 抖动模式。AI 视频平滑渐变下用 none 避免抖动脉冲噪声。
	DefaultDither = "none"
)

// buildFilter 构造 GIF 转换的 -vf filter 链。
// width<=0 时省略 scale（保持原尺寸），其余参数由常量注入。
//
// crop 非零时先按 CropMargins 从各边精确裁切（crop 滤镜），之后只做等比缩放，
// 不恢复源宽高比——只裁上下时左右保持不动，裁切后画面比例由剩余内容决定。
//
// 返回与下列手工命令等价的 filter 串：
//
//	fps=6,scale=160:-2:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=none
//	fps=6,crop=iw-0:ih-128:0:64,scale=160:-2:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=none
func buildFilter(width int, crop CropMargins) string {
	chain := fmt.Sprintf("fps=%d", DefaultFPS)
	if !crop.Zero() {
		chain += "," + buildCropFilter(crop)
	}
	if width > 0 {
		chain += fmt.Sprintf(",scale=%d:-2:flags=lanczos", width)
	}
	return fmt.Sprintf("%s,split[s0][s1];[s0]palettegen=max_colors=%d[p];[s1][p]paletteuse=dither=%s",
		chain, DefaultMaxColors, DefaultDither)
}

// buildCropFilter 构造按 CropMargins 精确裁切的 filter 片段：crop=iw-L-R:ih-T-B:L:T
// （从指定边各裁掉指定像素，不做任何额外裁切）。
func buildCropFilter(crop CropMargins) string {
	return fmt.Sprintf("crop=iw-%d:ih-%d:%d:%d",
		crop.Left+crop.Right, crop.Top+crop.Bottom, crop.Left, crop.Top)
}
