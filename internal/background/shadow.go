// Package background 的投影渲染。
//
// 在主体背后叠加投影。原理：
//  1. 取 alpha 遮罩，向右下偏移（--shadow-offset）
//  2. 高斯模糊（--shadow-blur）
//  3. 乘以不透明度（--shadow-opacity）和颜色（--shadow-color）
//  4. 用 Porter-Duff Over 合成到底层：foreground OVER shadow
//
// 特别注意：合成方向是 foreground OVER shadow（影子在主体后面），
// 而不是 shadow OVER foreground（影子盖在主体上）。
// 前者影子出现在主体外部（正确），后者影子出现在主体内部（错误）。
package background

import (
	"image/color"
)

// shadowConfig holds the parsed shadow parameters.
type shadowConfig struct {
	enabled bool
	offsetX int
	offsetY int
	blur    int
	col     color.NRGBA
}

// parseShadowConfig extracts shadow settings from Options.
func parseShadowConfig(opts *Options) shadowConfig {
	if !opts.Shadow {
		return shadowConfig{}
	}
	dx, dy := opts.ShadowOffset[0], opts.ShadowOffset[1]
	blur := opts.ShadowBlur
	if blur < 0 {
		blur = 0
	}
	opacity := opts.ShadowOpacity
	if opacity <= 0 {
		opacity = 40
	}
	if opacity > 100 {
		opacity = 100
	}

	c := opts.ShadowColor
	if c == nil {
		c = color.NRGBA{0, 0, 0, 255}
	}
	r, g, b, _ := c.RGBA()

	return shadowConfig{
		enabled: true,
		offsetX: dx,
		offsetY: dy,
		blur:    blur,
		col:     color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(opacity * 255 / 100)},
	}
}

// applyShadow composites a drop shadow behind the foreground image.
// The shadow is offset to the right/down and blurred, sitting behind the subject.
// It modifies outPixels (NRGBA format, w*h*4 bytes) in place.
// alpha is the alpha mask used to generate the shadow shape.
func applyShadow(outPixels []uint8, w, h int, alpha []uint8, cfg shadowConfig) {
	if !cfg.enabled {
		return
	}

	// 1. Build shadow mask from alpha mask.
	// The shadow is at offset (dx,dy) from the subject.
	// Read original alpha at (sx,sy), write to shadow at (sx+dx, sy+dy).
	shadowAlpha := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx := x - cfg.offsetX
			sy := y - cfg.offsetY
			if sx >= 0 && sx < w && sy >= 0 && sy < h {
				shadowAlpha[y*w+x] = alpha[sy*w+sx]
			}
		}
	}

	// 2. Blur shadow mask
	if cfg.blur > 0 {
		shadowAlpha = featherAlpha(shadowAlpha, w, h, cfg.blur)
	}

	// 3. Apply shadow opacity
	opacity := float64(cfg.col.A) / 255.0
	sR := float64(cfg.col.R)
	sG := float64(cfg.col.G)
	sB := float64(cfg.col.B)

	// 4. Composite: foreground OVER shadow (shadow sits behind the subject).
	// Porter-Duff Over: A_over_B = A + B * (1 - A.alpha)
	// Here A = foreground, B = shadow
	for i := 0; i < len(outPixels); i += 4 {
		sA := float64(shadowAlpha[i/4]) * opacity / 255.0
		fR := float64(outPixels[i])
		fG := float64(outPixels[i+1])
		fB := float64(outPixels[i+2])
		fA := float64(outPixels[i+3]) / 255.0

		if fA <= 0 && sA <= 0 {
			continue
		}
		if fA <= 0 {
			// Only shadow visible (outside the subject)
			outPixels[i] = clampByte(sR * sA)
			outPixels[i+1] = clampByte(sG * sA)
			outPixels[i+2] = clampByte(sB * sA)
			outPixels[i+3] = clampByte(sA * 255)
			continue
		}
		if sA <= 0 {
			// Only foreground, no shadow
			continue
		}

		// Foreground OVER shadow
		outA := fA + sA*(1-fA)
		outPixels[i] = clampByte((fR*fA + sR*sA*(1-fA)) / outA)
		outPixels[i+1] = clampByte((fG*fA + sG*sA*(1-fA)) / outA)
		outPixels[i+2] = clampByte((fB*fA + sB*sA*(1-fA)) / outA)
		outPixels[i+3] = clampByte(outA * 255)
	}
}
