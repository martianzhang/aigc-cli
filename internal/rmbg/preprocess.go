// Package rmbg 提供基于 RMBG 2.0 (Remove Background 2.0) ONNX 模型的语义分割去背功能。
//
// 整体流程：
//
//	输入图片 → resize 到 1024x1024 → ImageNet 归一化 (mean/std)
//	        → ONNX 推理 (pure-onnx)
//	        → sigmoid → mask resize 回原图尺寸 → 合成 alpha → 输出
//
// 模型信息：
//
//	模型: briaai/RMBG-2.0 (CC BY-NC 4.0 — 非商业用途)
//	架构: BiRefNet (Swin-V1 Large backbone)
//	输入: "pixel_values" — [1, 3, 1024, 1024] float32 NCHW
//	输出: 最后一个 decoder 输出 — [1, 1, 1024, 1024] float32 (sigmoid 已 fused)
package rmbg

import (
	"image"
	"math"

	"golang.org/x/image/draw"
)

const (
	// ModelInputSize is the expected image size (width/height) in pixels.
	ModelInputSize = 1024
	// ModelChannels is the expected number of color channels.
	ModelChannels = 3
)

// ImageNet normalization constants (from BRIA RMBG-2.0 official inference code).
var (
	imagenetMean = [3]float32{0.485, 0.456, 0.406}
	imagenetStd  = [3]float32{0.229, 0.224, 0.225}
)

// Preprocess 将图片转换为适合 RMBG 2.0 模型的 float32 tensor。
//
// 步骤：
//  1. Bilinear resize 到 targetSize×targetSize（默认 1024）
//  2. CHW float32 格式 (NCHW layout, batch N=1)
//  3. ImageNet 归一化: pixel = (pixel/255 - mean) / std
//
// 返回值长度 = 3 × targetSize × targetSize。
func Preprocess(img image.Image, targetSize int) []float32 {
	// Resize 到 targetSize×targetSize
	srcBounds := img.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	resized := image.NewRGBA(image.Rect(0, 0, targetSize, targetSize))
	if srcW == targetSize && srcH == targetSize {
		draw.Copy(resized, image.Point{}, img, srcBounds, draw.Src, nil)
	} else {
		draw.BiLinear.Scale(resized, resized.Bounds(), img, srcBounds, draw.Src, nil)
	}

	// CHW float32 + ImageNet 归一化
	pixels := make([]float32, ModelChannels*targetSize*targetSize)
	idx := 0
	for c := 0; c < ModelChannels; c++ {
		mean := imagenetMean[c]
		std := imagenetStd[c]
		for y := 0; y < targetSize; y++ {
			for x := 0; x < targetSize; x++ {
				r, g, b, _ := resized.At(x, y).RGBA()
				var val float32
				switch c {
				case 0: // R
					val = float32(r) / 65535.0
				case 1: // G
					val = float32(g) / 65535.0
				case 2: // B
					val = float32(b) / 65535.0
				}
				pixels[idx] = (val - mean) / std
				idx++
			}
		}
	}

	return pixels
}

// Sigmoid 对 float32 切片逐元素应用 sigmoid 函数。
// 如果模型输出已经 fused sigmoid，跳过此函数。
func Sigmoid(data []float32) []float32 {
	out := make([]float32, len(data))
	for i, v := range data {
		if v < -20 {
			out[i] = 0
		} else if v > 20 {
			out[i] = 1
		} else {
			out[i] = 1.0 / (1.0 + float32(math.Exp(float64(-v))))
		}
	}
	return out
}

// ResizeMask 将单通道 mask (srcW×srcH) resize 到目标尺寸 (dstW×dstH)。
// 使用 nearest-neighbor 插值（mask 是二值/连续值，不需要平滑）。
// 返回 [0, 255] 的 uint8 切片。
func ResizeMask(mask []float32, srcW, srcH, dstW, dstH int) []uint8 {
	if srcW == dstW && srcH == dstH {
		out := make([]uint8, dstW*dstH)
		for i, v := range mask {
			out[i] = clampU8(v * 255)
		}
		return out
	}

	out := make([]uint8, dstW*dstH)
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
			val := mask[sy*srcW+sx]
			out[dy*dstW+dx] = clampU8(val * 255)
		}
	}
	return out
}

// ApplyAlpha 将 alpha 遮罩应用到 RGBA 像素数据上。
// rgba: 原始图片的 RGBA 像素 (w*h*4 bytes)
// alpha: alpha 遮罩 (w*h bytes, 0=透明, 255=不透明)
// 返回新的 NRGBA 像素数据 (w*h*4 bytes, pre-multiplied alpha)
func ApplyAlpha(rgba []uint8, w, h int, alpha []uint8) []uint8 {
	out := make([]uint8, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w*4 + x*4
			a := alpha[y*w+x]
			out[idx] = rgba[idx]
			out[idx+1] = rgba[idx+1]
			out[idx+2] = rgba[idx+2]
			out[idx+3] = a
		}
	}
	return out
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
