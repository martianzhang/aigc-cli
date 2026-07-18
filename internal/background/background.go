// Package background 实现纯色背景去除与替换。
//
// 整体流程（pipeline）如下：
//
//	输入图片 → detectAndTune (自动检测背景色)
//	        → buildAlphaMask/Multi (构建 alpha 遮罩)
//	        → floodFill (连通性修正，解决主体与背景同色问题)
//	        → postProcessAlpha (平滑 + 羽化 + 腐蚀 + 闭合)
//	        → edgeRefineAlpha (Sobel 边缘引导锐化)
//	        → applyAlpha (将 alpha 写入像素)
//	        → despillPixels (去除边缘背景色沾染)
//	        → autocrop (可选裁剪) / shadow (可选投影) / replace (可选换背景)
//	        → 输出 NRGBA 图片
//
// 核心算法是色度键控（Chroma Key），基于 CIELAB ΔE 色彩距离。
// 所有数值都经过调优，纯 Go 实现，不依赖任何外部库或模型。
package background

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"strconv"
	"strings"
)

// Options controls background removal/replacement behavior.
// Zero values trigger smart defaults (auto-detect).
type Options struct {
	// Tolerance is the ΔE threshold for background detection.
	// 0 = auto-detect from edge pixel variance.
	Tolerance float64

	// Feather is the edge feathering radius in pixels.
	// -1 = auto-detect based on image diagonal.
	Feather int

	// BgColor manually specifies the background color.
	// nil = auto-detect via K-Means.
	BgColor color.Color

	// FgThreshold is the multiplier for foreground cutoff.
	// Pixels with ΔE > Tolerance * FgThreshold are fully opaque.
	// Default: 1.5
	FgThreshold float64

	// Smooth is the number of alpha mask smoothing passes (3x3 mean filter).
	// Default: 1
	Smooth int

	// Erode is the morphological erosion radius in pixels.
	// Default: 0
	Erode int

	// Close is the morphological closing radius (dilate→erode).
	// Fills small holes in the foreground caused by color similarity.
	// Default: 0 (disabled)
	Close int

	// SampleRegion is the edge sampling width as percentage of min(w,h).
	// Default: 5
	SampleRegion float64

	// Autocrop enables cropping to the foreground bounding box.
	Autocrop bool

	// Padding expands the autocrop bounds: [top, right, bottom, left] in pixels.
	Padding [4]int

	// AspectRatio forces output aspect ratio (e.g. "16:9", "1:1").
	// Empty string means no constraint.
	AspectRatio string

	// Shadow enables drop shadow behind the subject.
	Shadow bool

	// ShadowOffset is the shadow offset [dx, dy] in pixels.
	ShadowOffset [2]int

	// ShadowBlur is the shadow blur radius in pixels.
	ShadowBlur int

	// ShadowColor is the shadow color.
	ShadowColor color.Color

	// ShadowOpacity is the shadow opacity 0-100.
	ShadowOpacity float64
}

// Defaults returns an Options with sensible defaults.
func Defaults() Options {
	return Options{
		Tolerance:     0,  // auto
		Feather:       -1, // auto
		BgColor:       nil,
		FgThreshold:   1.5,
		Smooth:        1,
		Erode:         0,
		Close:         0,
		SampleRegion:  5,
		Autocrop:      false,
		AspectRatio:   "",
		Shadow:        false,
		ShadowOffset:  [2]int{4, 4},
		ShadowBlur:    6,
		ShadowColor:   color.NRGBA{0, 0, 0, 255},
		ShadowOpacity: 40,
	}
}

// Result holds metadata about a background removal operation.
type Result struct {
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	DetectedBgColor string  `json:"detected_bg_color,omitempty"`
	ToleranceUsed   float64 `json:"tolerance_used"`
}

// RemoveBackground removes the background, returning an NRGBA with transparency.
func RemoveBackground(img image.Image, opts *Options) (*image.NRGBA, *Result, error) {
	return processImage(img, opts, false, nil, nil)
}

// ReplaceColor removes background and replaces with a solid color.
func ReplaceColor(img image.Image, bgColor color.Color, opts *Options) (*image.NRGBA, *Result, error) {
	return processImage(img, opts, true, bgColor, nil)
}

// ReplaceImage removes background and composites onto another image.
func ReplaceImage(img image.Image, bgImg image.Image, opts *Options) (*image.NRGBA, *Result, error) {
	return processImage(img, opts, true, nil, bgImg)
}

// MaskOnly generates the alpha mask as grayscale (white=foreground, black=background).
func MaskOnly(img image.Image, opts *Options) (*image.Gray, *Result, error) {
	if opts == nil {
		d := Defaults()
		opts = &d
	}
	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()
	rgba := imageToRGBA(img)

	primaryL, primaryA, primaryB, bgLabs, _ := detectAndTune(img, opts)

	tolerance := opts.Tolerance
	if tolerance <= 0 {
		samples := sampleEdgePixels(img, opts.SampleRegion)
		tolerance = autoToleranceMulti(samples, bgLabs)
	}

	var alpha []uint8
	if len(bgLabs) > 1 {
		alpha = buildAlphaMaskMulti(rgba.Pix, w, h, bgLabs, tolerance, opts.FgThreshold)
	} else {
		alpha = buildAlphaMask(rgba.Pix, w, h, primaryL, primaryA, primaryB, tolerance, opts.FgThreshold)
	}
	alpha = postProcessAlpha(alpha, w, h, opts)

	gray := image.NewGray(b)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			gray.SetGray(x, y, color.Gray{Y: alpha[y*w+x]})
		}
	}

	result := &Result{Width: w, Height: h, ToleranceUsed: tolerance}
	if opts.BgColor == nil {
		result.DetectedBgColor = labToHex(primaryL, primaryA, primaryB)
	}
	return gray, result, nil
}

func processImage(img image.Image, opts *Options, doReplace bool, repColor color.Color, repImg image.Image) (*image.NRGBA, *Result, error) {
	if opts == nil {
		d := Defaults()
		opts = &d
	}
	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()
	rgba := imageToRGBA(img)

	primaryL, primaryA, primaryB, bgLabs, bgRGBs := detectAndTune(img, opts)
	tolerance := autoToleranceMulti(sampleEdgePixels(img, opts.SampleRegion), bgLabs)

	var alpha []uint8
	if len(bgLabs) > 1 {
		alpha = buildAlphaMaskMulti(rgba.Pix, w, h, bgLabs, tolerance, opts.FgThreshold)
	} else {
		alpha = buildAlphaMask(rgba.Pix, w, h, primaryL, primaryA, primaryB, tolerance, opts.FgThreshold)
	}
	alpha = postProcessAlpha(alpha, w, h, opts)

	// Phase 2: Sobel-guided edge refinement
	if w > 2 && h > 2 {
		// Build grayscale float64 buffer for Sobel
		grayPix := make([]float64, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r, g, bb, _ := getPixelRGBA(rgba.Pix, w, h, x, y)
				grayPix[y*w+x] = float64(r)*0.299 + float64(g)*0.587 + float64(bb)*0.114
			}
		}
		alpha = edgeRefineAlpha(alpha, grayPix, w, h)
	}

	// Save a copy of alpha for shadow (before applyAlpha consumes it)
	var shadowAlpha []uint8
	if opts.Shadow {
		shadowAlpha = make([]uint8, len(alpha))
		copy(shadowAlpha, alpha)
	}

	outPixels := applyAlpha(rgba.Pix, w, h, alpha)

	// Phase 1: Color spill suppression (remove background contamination from edge pixels)
	if len(bgRGBs) > 0 {
		despillPixels(outPixels, w, h, bgRGBs)
	}

	// Apply shadow if enabled (before autocrop so shadow is included in bounds)
	if opts.Shadow {
		cfg := parseShadowConfig(opts)
		applyShadow(outPixels, w, h, shadowAlpha, cfg)
	}

	outW, outH := w, h
	if opts.Autocrop {
		x0, y0, x1, y1, ok := findBounds(alpha, w, h)
		if ok {
			x0, y0, x1, y1 = applyPadding(x0, y0, x1, y1, w, h, opts.Padding)
			x0, y0, x1, y1 = applyAspectRatio(x0, y0, x1, y1, w, h, opts.AspectRatio)
			outPixels, outW, outH = cropImage(outPixels, w, h, x0, y0, x1, y1)
		}
	}

	outImg := image.NewNRGBA(image.Rect(0, 0, outW, outH))
	copy(outImg.Pix, outPixels)
	outImg.Stride = outW * 4

	if doReplace {
		if repColor != nil {
			outImg = compositeOnColor(outImg, repColor)
		} else if repImg != nil {
			outImg = compositeOnImage(outImg, repImg)
		}
	}

	result := &Result{Width: outW, Height: outH, ToleranceUsed: tolerance}
	if opts.BgColor == nil {
		result.DetectedBgColor = labToHex(primaryL, primaryA, primaryB)
	}
	return outImg, result, nil
}

func detectAndTune(img image.Image, opts *Options) (primaryL, primaryA, primaryB float64, bgLabs [][3]float64, bgRGBs [][3]uint8) {
	if opts.BgColor != nil {
		r, g, bb, _ := opts.BgColor.RGBA()
		primaryL, primaryA, primaryB = rgbToLab(uint8(r>>8), uint8(g>>8), uint8(bb>>8))
		bgLabs = [][3]float64{{primaryL, primaryA, primaryB}}
		bgRGBs = [][3]uint8{{uint8(r >> 8), uint8(g >> 8), uint8(bb >> 8)}}
		return
	}
	samples := sampleEdgePixels(img, opts.SampleRegion)
	primaryL, primaryA, primaryB, bgLabs, bgRGBs = detectBackgroundColors(samples, img, opts.SampleRegion)
	return
}

func postProcessAlpha(alpha []uint8, w, h int, opts *Options) []uint8 {
	if opts.Close > 0 {
		alpha = closeAlpha(alpha, w, h, opts.Close)
	}
	if opts.Smooth > 0 {
		alpha = smoothAlpha(alpha, w, h, opts.Smooth)
	}
	featherR := opts.Feather
	if featherR < 0 {
		featherR = autoFeatherRadius(w, h)
	}
	if featherR > 0 {
		alpha = featherAlpha(alpha, w, h, featherR)
	}
	if opts.Erode > 0 {
		alpha = erodeAlpha(alpha, w, h, opts.Erode)
	}
	return alpha
}

// despillPixels removes background color contamination from edge pixels
// (the "spill" that occurs when a semi-transparent edge blends with the
// background color). Uses reverse alpha blending per pixel.
func despillPixels(outPixels []uint8, w, h int, bgRGBs [][3]uint8) {
	if len(bgRGBs) == 0 {
		return
	}
	bgR := float64(bgRGBs[0][0])
	bgG := float64(bgRGBs[0][1])
	bgB := float64(bgRGBs[0][2])
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := (y*w + x) * 4
			a := outPixels[idx+3]
			if a < 40 || a > 240 {
				continue
			}
			alpha := float64(a) / 255.0
			invAlpha := 1.0 / alpha

			r := float64(outPixels[idx])
			g := float64(outPixels[idx+1])
			b := float64(outPixels[idx+2])

			nr := (r - bgR*(1-alpha)) * invAlpha
			ng := (g - bgG*(1-alpha)) * invAlpha
			nb := (b - bgB*(1-alpha)) * invAlpha

			if nr < -10 || nr > 265 || ng < -10 || ng > 265 || nb < -10 || nb > 265 {
				continue
			}

			outPixels[idx] = clampByte(nr)
			outPixels[idx+1] = clampByte(ng)
			outPixels[idx+2] = clampByte(nb)
		}
	}
}

func imageToRGBA(img image.Image) *image.RGBA {
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, img, b.Min, draw.Src)
	return rgba
}

func compositeOnColor(fg *image.NRGBA, bg color.Color) *image.NRGBA {
	b := fg.Bounds()
	out := image.NewNRGBA(b)
	draw.Draw(out, b, &image.Uniform{bg}, image.Point{}, draw.Src)
	draw.Draw(out, b, fg, b.Min, draw.Over)
	return out
}

func compositeOnImage(fg *image.NRGBA, bg image.Image) *image.NRGBA {
	b := fg.Bounds()
	out := image.NewNRGBA(b)
	draw.Draw(out, b, bg, b.Min, draw.Src)
	draw.Draw(out, b, fg, b.Min, draw.Over)
	return out
}

func labToHex(L, a, b float64) string {
	r, g, bb := labToRGBApprox(L, a, b)
	return fmt.Sprintf("#%02X%02X%02X", r, g, bb)
}

func labToRGBApprox(L, a, b float64) (r, g, bb uint8) {
	const refX = 0.95047
	const refY = 1.00000
	const refZ = 1.08883
	fy := (L + 16.0) / 116.0
	fx := a/500.0 + fy
	fz := fy - b/200.0
	x := refX * labfInv(fx)
	y := refY * labfInv(fy)
	z := refZ * labfInv(fz)
	rL := 3.2404542*x - 1.5371385*y - 0.4985314*z
	gL := -0.9692660*x + 1.8760108*y + 0.0415560*z
	bL := 0.0556434*x - 0.2040259*y + 1.0572252*z
	r = linearToSRGB(rL)
	g = linearToSRGB(gL)
	bb = linearToSRGB(bL)
	return
}

func labfInv(t float64) float64 {
	if t > 0.206897 {
		return t * t * t
	}
	return (t - 16.0/116.0) / 7.787
}

func linearToSRGB(v float64) uint8 {
	if v <= 0.0031308 {
		v *= 12.92
	} else {
		v = 1.055*math.Pow(v, 1.0/2.4) - 0.055
	}
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return uint8(v*255 + 0.5)
}

// ParsePadding parses padding string "20" or "10,20,30,40".
func ParsePadding(s string) ([4]int, error) {
	parts := strings.Split(s, ",")
	var p [4]int
	switch len(parts) {
	case 1:
		v, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return p, fmt.Errorf("invalid padding: %s", s)
		}
		p[0], p[1], p[2], p[3] = v, v, v, v
	case 4:
		for i := 0; i < 4; i++ {
			v, err := strconv.Atoi(strings.TrimSpace(parts[i]))
			if err != nil {
				return p, fmt.Errorf("invalid padding at position %d: %s", i, parts[i])
			}
			p[i] = v
		}
	default:
		return p, fmt.Errorf("padding must be 1 or 4 values, got %d", len(parts))
	}
	return p, nil
}

// SavePNG saves image.Image to a PNG file.
func SavePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
