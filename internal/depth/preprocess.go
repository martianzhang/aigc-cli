// Package depth 提供基于 Depth Anything V2 ONNX 模型的单目深度估计功能。
//
// 整体流程：
//
//	图片/视频帧 → resize 到 n×n（14 对齐）→ ImageNet 归一化 (mean/std)
//	       → ONNX 推理 (pure-onnx)
//	       → 逆深度 min-max 归一化 → 8-bit 灰度图
//	       → resize 回原尺寸 → 输出
//
// 模型信息：
//
//	模型: Depth Anything V2 (小/中/大, Apache-2.0 或 CC-BY-NC-4.0)
//	架构: DINOv2 ViT backbone + DPT head
//	输入: "pixel_values" — [1, 3, 518, 518] float32 NCHW
//	输出: "predicted_depth" — [1, 518, 518] float32 (逆深度, 无 channel 维)
package depth

import (
	"image"
	"image/color"
	"image/png"
	"io"

	"golang.org/x/image/draw"
)

const (
	// ModelInputSize is the official Depth Anything V2 inference resolution
	// (short side of the canvas; ViT patch size 14, 518=37×14).
	ModelInputSize = 518
	// DefaultInferenceSize is the default resolution for video conversion.
	// 280 (20×14) is a ~1.8× speedup over 378 with depth structure still
	// clearly readable — depth videos are motion/structure references, so
	// fine texture fidelity is not required. Users can raise it via
	// --depth-size (e.g. 378 or 518) for maximum quality.
	DefaultInferenceSize = 280
	// ModelChannels is the expected number of color channels.
	ModelChannels = 3
	// ModelPatchSize is the ViT patch size; model output dims are 14-aligned.
	ModelPatchSize = 14
)

// ImageNet normalization constants (Depth Anything V2 official inference code).
var (
	imagenetMean = [3]float32{0.485, 0.456, 0.406}
	imagenetStd  = [3]float32{0.229, 0.224, 0.225}
)

// Crop 描述等比缩放后在方形画布上的有效区域（不含 pad 黑边）。
// 推理输出需先裁掉 pad 再缩放回原尺寸，避免内容比例失真。
type Crop struct {
	X, Y, Width, Height int // 有效区域在 targetSize×targetSize 画布上的位置与尺寸
}

// Preprocess 将一帧图片转换为适合 Depth Anything V2 模型的 float32 tensor。
//
// 步骤：
//  1. 等比缩放：长边缩放到 ModelInputSize，保持宽高比（官方 keep_aspect_ratio 行为）
//  2. pad 到 ModelInputSize×ModelInputSize（黑边填充，14 对齐天然满足）
//  3. CHW float32 格式 (NCHW layout, batch N=1) + ImageNet 归一化
//
// 返回值长度 = 3 × ModelInputSize × ModelInputSize。
func Preprocess(img image.Image) []float32 {
	pixels, _ := PreprocessCrop(img, ModelInputSize)
	return pixels
}

// PreprocessCrop 与 Preprocess 相同，额外返回有效区域 Crop。
// 调用方必须用 Crop 裁掉模型输出中的 pad 区域，再缩放回原尺寸。
func PreprocessCrop(img image.Image, targetSize int) ([]float32, Crop) {
	srcBounds := img.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return make([]float32, ModelChannels*targetSize*targetSize), Crop{Width: targetSize, Height: targetSize}
	}

	// Aspect-preserving resize bounded by the canvas: scale so the LONGER
	// side fits targetSize, then pad the shorter side with black borders.
	// This preserves aspect ratio for any input (official behavior).
	scale := float64(targetSize) / float64(max(srcW, srcH))
	resizedW := max(1, int(float64(srcW)*scale))
	resizedH := max(1, int(float64(srcH)*scale))

	// Pad to a targetSize×targetSize canvas (black borders), matching the
	// official Depth Anything V2 preprocessing (keep_aspect_ratio + padding).
	canvas := image.NewRGBA(image.Rect(0, 0, targetSize, targetSize))
	offsetX := (targetSize - resizedW) / 2
	offsetY := (targetSize - resizedH) / 2
	draw.BiLinear.Scale(canvas, image.Rect(offsetX, offsetY, offsetX+resizedW, offsetY+resizedH),
		img, srcBounds, draw.Src, nil)

	// CHW float32 + ImageNet 归一化
	pixels := make([]float32, ModelChannels*targetSize*targetSize)
	idx := 0
	for c := 0; c < ModelChannels; c++ {
		mean := imagenetMean[c]
		std := imagenetStd[c]
		for y := 0; y < targetSize; y++ {
			row := y * canvas.Stride
			for x := 0; x < targetSize; x++ {
				p := row + x*4
				var val float32
				switch c {
				case 0: // R
					val = float32(canvas.Pix[p]) / 255.0
				case 1: // G
					val = float32(canvas.Pix[p+1]) / 255.0
				case 2: // B
					val = float32(canvas.Pix[p+2]) / 255.0
				}
				pixels[idx] = (val - mean) / std
				idx++
			}
		}
	}

	return pixels, Crop{X: offsetX, Y: offsetY, Width: resizedW, Height: resizedH}
}

// DepthToGray 将逆深度输出转换为 8-bit 灰度图（近亮远暗）。
//
// 逆深度语义：值越小表示越远（1/depth）。直接对逆深度做 min-max
// 归一化即可得到"近亮远暗"的直觉映射，无需取倒数——模型输出本身
// 已包含正确的相对关系，人眼对逆深度的对数间距更敏感。
//
// data 长度为 srcW×srcH（模型输出的 [1, H, W] 展平）。输出 resize
// 到 dstW×dstH（视频原帧分辨率）。
func DepthToGray(data []float32, srcW, srcH, dstW, dstH int) *image.Gray {
	// min-max 归一化到 [0, 255]
	lo, hi := float32(0), float32(0)
	if len(data) > 0 {
		lo, hi = data[0], data[0]
		for _, v := range data {
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
	}
	rangeVal := hi - lo
	if rangeVal < 1e-6 {
		rangeVal = 1e-6
	}

	src := make([]uint8, len(data))
	for i, v := range data {
		src[i] = clampU8((v - lo) / rangeVal * 255)
	}

	// 若尺寸一致直接拷贝，否则用最近邻 resize 回原帧尺寸
	dst := image.NewGray(image.Rect(0, 0, dstW, dstH))
	if srcW == dstW && srcH == dstH {
		copy(dst.Pix, src)
		return dst
	}
	for dy := 0; dy < dstH; dy++ {
		sy := dy * srcH / dstH
		if sy >= srcH {
			sy = srcH - 1
		}
		for dx := 0; dx < dstW; dx++ {
			sx := dx * srcW / dstW
			if sx >= srcW {
				sx = srcW - 1
			}
			dst.SetGray(dx, dy, color.Gray{Y: src[sy*srcW+sx]})
		}
	}
	return dst
}

// DepthToGrayCrop 从方形深度输出中裁出有效区域（Crop），再缩放回目标尺寸。
// 用于配合 PreprocessCrop：模型输出是含 pad 的方形，必须先裁掉 pad
// 再 resize，否则内容比例会失真（pad 区域被拉伸进画面）。
func DepthToGrayCrop(data []float32, canvasSize int, crop Crop, dstW, dstH int) *image.Gray {
	// min-max 归一化到 [0, 255]（对整个画布统计，保持跨帧稳定）
	lo, hi := float32(0), float32(0)
	if len(data) > 0 {
		lo, hi = data[0], data[0]
		for _, v := range data {
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
	}
	rangeVal := hi - lo
	if rangeVal < 1e-6 {
		rangeVal = 1e-6
	}

	src := make([]uint8, len(data))
	for i, v := range data {
		src[i] = clampU8((v - lo) / rangeVal * 255)
	}

	dst := image.NewGray(image.Rect(0, 0, dstW, dstH))
	cw, ch := crop.Width, crop.Height
	if cw <= 0 {
		cw = canvasSize
	}
	if ch <= 0 {
		ch = canvasSize
	}
	// 有效区域在画布中的起始像素索引
	cropBase := crop.Y*canvasSize + crop.X
	for dy := 0; dy < dstH; dy++ {
		sy := dy * ch / dstH
		if sy >= ch {
			sy = ch - 1
		}
		for dx := 0; dx < dstW; dx++ {
			sx := dx * cw / dstW
			if sx >= cw {
				sx = cw - 1
			}
			dst.SetGray(dx, dy, color.Gray{Y: src[cropBase+sy*canvasSize+sx]})
		}
	}
	return dst
}

// DepthToColorCrop 与 DepthToGrayCrop 相同，但输出 RGB 彩色深度图：
// 灰度值通过 Spectral_r colormap 着色（近处暖色红/橙，远处冷色蓝/紫），
// 与官方 Depth-Anything-V2 run.py 的可视化一致。
func DepthToColorCrop(data []float32, canvasSize int, crop Crop, dstW, dstH int) *image.RGBA {
	lo, hi := float32(0), float32(0)
	if len(data) > 0 {
		lo, hi = data[0], data[0]
		for _, v := range data {
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
	}
	rangeVal := hi - lo
	if rangeVal < 1e-6 {
		rangeVal = 1e-6
	}

	src := make([]uint8, len(data))
	for i, v := range data {
		src[i] = clampU8((v - lo) / rangeVal * 255)
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	cw, ch := crop.Width, crop.Height
	if cw <= 0 {
		cw = canvasSize
	}
	if ch <= 0 {
		ch = canvasSize
	}
	cropBase := crop.Y*canvasSize + crop.X
	for dy := 0; dy < dstH; dy++ {
		sy := dy * ch / dstH
		if sy >= ch {
			sy = ch - 1
		}
		for dx := 0; dx < dstW; dx++ {
			sx := dx * cw / dstW
			if sx >= cw {
				sx = cw - 1
			}
			idx := cropBase + sy*canvasSize + sx
			r, g, b := colorize(src[idx])
			dst.Set(dx, dy, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return dst
}

// SaveColorPNG 将 RGB 像素数据以 PNG 写入 w。
func SaveColorPNG(w io.Writer, pix []uint8, width, height int) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i, v := range pix {
		if i >= len(img.Pix) {
			break
		}
		img.Pix[i] = v
	}
	return png.Encode(w, img)
}

// SaveGrayPNG 将灰度像素数据以 PNG 写入 w。
func SaveGrayPNG(w io.Writer, pix []uint8, width, height int) error {
	g := image.NewGray(image.Rect(0, 0, width, height))
	if len(pix) == width*height {
		copy(g.Pix, pix)
	} else {
		for i, v := range pix {
			if i >= len(g.Pix) {
				break
			}
			g.Pix[i] = v
		}
	}
	return png.Encode(w, g)
}

// clampU8 将 float32 钳制到 [0, 255] 的 uint8。
func clampU8(v float32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
