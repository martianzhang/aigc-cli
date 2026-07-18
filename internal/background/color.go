// Package background provides solid-color background removal and replacement
// using CIELAB ΔE chroma keying — pure Go, no external models or APIs.
// Package background 的色彩空间转换工具。
//
// 整个去背算法基于 CIELAB 色彩空间，而不是 RGB。
// 原因：RGB 的欧氏距离与人眼感知不成正比（比如 RGB(255,0,0) 和 RGB(0,255,0) 距离很大，
// 但人眼看起来都是"红色系"和"绿色系"的区别远大于数值差）。
// CIELAB 是感知均匀的色彩空间——ΔE=10 在任何颜色方向上的"视觉差异"是相同的。
//
// 转换链路：
//
//	sRGB (gamma 校正) → linear RGB → CIE XYZ (D65 白点) → CIE L*a*b*
//	DeltaE (CIE76)：两个 Lab 颜色之间的欧氏距离
package background

import "math"

// rgbToLinear converts sRGB [0,255] to linear RGB [0,1].
func rgbToLinear(c uint8) float64 {
	v := float64(c) / 255.0
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// xyzToLab converts CIE XYZ (D65) to CIE L*a*b*.
func xyzToLab(x, y, z float64) (L, a, b float64) {
	// D65 reference white
	const refX = 0.95047
	const refY = 1.00000
	const refZ = 1.08883

	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return (903.3*t + 16.0) / 116.0
	}

	fx := f(x / refX)
	fy := f(y / refY)
	fz := f(z / refZ)

	L = 116.0*fy - 16.0
	a = 500.0 * (fx - fy)
	b = 200.0 * (fy - fz)
	return
}

// rgbToLab converts an sRGB pixel (0-255) to CIE L*a*b*.
func rgbToLab(r, g, b uint8) (L, a, bb float64) {
	rL := rgbToLinear(r)
	gL := rgbToLinear(g)
	bL := rgbToLinear(b)

	// Linear RGB → CIE XYZ (D65, sRGB matrix)
	x := 0.4124564*rL + 0.3575761*gL + 0.1804375*bL
	y := 0.2126729*rL + 0.7151522*gL + 0.0721750*bL
	z := 0.0193339*rL + 0.1191920*gL + 0.9503041*bL

	return xyzToLab(x, y, z)
}

// deltaE computes CIELAB ΔE (CIE76) between two L*a*b* colors.
func deltaE(L1, a1, b1, L2, a2, b2 float64) float64 {
	dL := L1 - L2
	da := a1 - a2
	db := b1 - b2
	return math.Sqrt(dL*dL + da*da + db*db)
}

// getPixelRGBA reads RGBA values from pixel data.
func getPixelRGBA(data []uint8, w, h, x, y int) (r, g, b, a uint8) {
	idx := y*w*4 + x*4
	if idx+3 >= len(data) {
		return 0, 0, 0, 0
	}
	return data[idx], data[idx+1], data[idx+2], data[idx+3]
}
