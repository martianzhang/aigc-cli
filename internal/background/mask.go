package background

import "math"

// featherAlpha 对 alpha 遮罩应用 box blur 边缘柔化。
// 注意: 这个函数只被 shadow 模块内部调用（模糊阴影），
// RMBG 生成的 mask 本身不需要额外羽化。
func featherAlpha(alpha []uint8, w, h, radius int) []uint8 {
	if radius <= 0 {
		return alpha
	}

	// 分离式 box blur (快速)
	// Pass 1: 水平方向
	tmp := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sum float64
			var count int
			x0 := x - radius
			if x0 < 0 {
				x0 = 0
			}
			x1 := x + radius
			if x1 >= w {
				x1 = w - 1
			}
			for cx := x0; cx <= x1; cx++ {
				sum += float64(alpha[y*w+cx])
				count++
			}
			tmp[y*w+x] = sum / float64(count)
		}
	}

	// Pass 2: 垂直方向
	out := make([]uint8, w*h)
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			var sum float64
			var count int
			y0 := y - radius
			if y0 < 0 {
				y0 = 0
			}
			y1 := y + radius
			if y1 >= h {
				y1 = h - 1
			}
			for cy := y0; cy <= y1; cy++ {
				sum += tmp[cy*w+x]
				count++
			}
			out[y*w+x] = clampByte(math.Round(sum / float64(count)))
		}
	}

	return out
}
