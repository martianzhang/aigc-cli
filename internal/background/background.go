// Package background 实现基于 RMBG 2.0 语义分割的图片去背与替换。
//
// 整体流程：
//
//	输入图片 → rmbg.Detector.RemoveBackground (ONNX 推理)
//	        → applyShadow (可选投影)
//	        → autocrop (可选裁剪)
//	        → compositeOnColor/Image (可选替换背景)
//	        → 输出 NRGBA 图片
//
// 核心算法是 RMBG 2.0 (BiRefNet) 语义分割，支持任意背景类型，不依赖颜色检测。
package background

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/rmbg"
)

// Options 控制去背/替换/后处理行为。
type Options struct {
	// Autocrop 启用裁剪到主体边界框。
	Autocrop bool
	// Padding 扩展裁剪边界: [top, right, bottom, left] (像素)。
	Padding [4]int
	// AspectRatio 强制输出宽高比 (如 "16:9", "1:1")。空字符串不约束。
	AspectRatio string

	// Shadow 启用主体背后的投影。
	Shadow bool
	// ShadowOffset 投影偏移 [dx, dy] (像素)。
	ShadowOffset [2]int
	// ShadowBlur 投影模糊半径 (像素)。
	ShadowBlur int
	// ShadowColor 投影颜色。
	ShadowColor color.Color
	// ShadowOpacity 投影不透明度 0-100。
	ShadowOpacity float64
}

// Defaults 返回带有合理默认值的 Options。
func Defaults() Options {
	return Options{
		Autocrop:      false,
		AspectRatio:   "",
		Shadow:        false,
		ShadowOffset:  [2]int{4, 4},
		ShadowBlur:    6,
		ShadowColor:   color.NRGBA{0, 0, 0, 255},
		ShadowOpacity: 40,
	}
}

// Result 保存去背操作的元数据。
type Result struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// RemoveBackground 使用 RMBG 模型去除图片背景。
// det 是已初始化的 RMBG Detector（不能为 nil）。
// 返回透明背景的 NRGBA 图片。
func RemoveBackground(img image.Image, opts *Options, det *rmbg.Detector) (*image.NRGBA, *Result, error) {
	return processImage(img, opts, det, false, nil, nil)
}

// ReplaceColor 去除背景并替换为纯色。
func ReplaceColor(img image.Image, bgColor color.Color, opts *Options, det *rmbg.Detector) (*image.NRGBA, *Result, error) {
	return processImage(img, opts, det, true, bgColor, nil)
}

// ReplaceImage 去除背景并合成到另一张图上。
func ReplaceImage(img image.Image, bgImg image.Image, opts *Options, det *rmbg.Detector) (*image.NRGBA, *Result, error) {
	return processImage(img, opts, det, true, nil, bgImg)
}

// MaskOnly 只输出灰度 alpha 遮罩（white=前景, black=背景）。
func MaskOnly(img image.Image, opts *Options, det *rmbg.Detector) (*image.Gray, *Result, error) {
	if det == nil {
		return nil, nil, fmt.Errorf("RMBG detector is nil — run 'aigc-cli background init' first")
	}

	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()

	// 直接用 rmbg 获取 mask
	_, gray, err := det.RemoveBackground(img)
	if err != nil {
		return nil, nil, fmt.Errorf("RMBG inference failed: %w", err)
	}

	result := &Result{Width: w, Height: h}
	return gray, result, nil
}

func processImage(img image.Image, opts *Options, det *rmbg.Detector,
	doReplace bool, repColor color.Color, repImg image.Image) (*image.NRGBA, *Result, error) {

	if det == nil {
		return nil, nil, fmt.Errorf("RMBG detector is nil — run 'aigc-cli background init' first")
	}
	if opts == nil {
		d := Defaults()
		opts = &d
	}

	b := img.Bounds()
	origW := b.Dx()
	origH := b.Dy()

	// Step 1: RMBG 推理 → NRGBA + 灰度 mask
	outImg, gray, err := det.RemoveBackground(img)
	if err != nil {
		return nil, nil, fmt.Errorf("RMBG inference failed: %w", err)
	}

	w, h := origW, origH
	outPixels := outImg.Pix

	// Step 2: 投影 (shadow) — 在 autocrop 之前，让影子纳入裁剪范围
	if opts.Shadow {
		cfg := parseShadowConfig(opts)
		applyShadow(outPixels, w, h, gray.Pix, cfg)
	}

	// Step 3: 自动裁剪 (autocrop)
	outW, outH := w, h
	if opts.Autocrop {
		x0, y0, x1, y1, ok := findBounds(gray.Pix, w, h)
		if ok {
			x0, y0, x1, y1 = applyPadding(x0, y0, x1, y1, w, h, opts.Padding)
			x0, y0, x1, y1 = applyAspectRatio(x0, y0, x1, y1, w, h, opts.AspectRatio)
			outPixels, outW, outH = cropImage(outPixels, w, h, x0, y0, x1, y1)
		}
	}

	// 构建输出图片
	out := image.NewNRGBA(image.Rect(0, 0, outW, outH))
	copy(out.Pix, outPixels)
	out.Stride = outW * 4

	// Step 4: 替换背景 (replace)
	if doReplace {
		if repColor != nil {
			out = compositeOnColor(out, repColor)
		} else if repImg != nil {
			out = compositeOnImage(out, repImg)
		}
	}

	result := &Result{Width: outW, Height: outH}
	return out, result, nil
}

func compositeOnColor(fg *image.NRGBA, bg color.Color) *image.NRGBA {
	brect := fg.Bounds()
	out := image.NewNRGBA(brect)
	draw.Draw(out, brect, &image.Uniform{bg}, image.Point{}, draw.Src)
	draw.Draw(out, brect, fg, brect.Min, draw.Over)
	return out
}

func compositeOnImage(fg *image.NRGBA, bg image.Image) *image.NRGBA {
	brect := fg.Bounds()
	out := image.NewNRGBA(brect)
	draw.Draw(out, brect, bg, brect.Min, draw.Src)
	draw.Draw(out, brect, fg, brect.Min, draw.Over)
	return out
}

// SavePNG 将 image.Image 保存为 PNG 文件。
func SavePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// ParsePadding 解析 padding 字符串 "20" 或 "10,20,30,40"。
func ParsePadding(s string) ([4]int, error) {
	var p [4]int
	if s == "" {
		return p, nil
	}
	if strings.Contains(s, ",") {
		if _, err := fmt.Sscanf(s, "%d,%d,%d,%d", &p[0], &p[1], &p[2], &p[3]); err == nil {
			return p, nil
		}
		return p, fmt.Errorf("invalid padding %q — use \"N\" or \"T,R,B,L\"", s)
	}
	var v int
	if _, err := fmt.Sscanf(s, "%d", &v); err == nil {
		p[0], p[1], p[2], p[3] = v, v, v, v
		return p, nil
	}
	return p, fmt.Errorf("invalid padding %q — use \"N\" or \"T,R,B,L\"", s)
}

// clampByte clamps a float64 to [0, 255] uint8.
func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
