package face

import (
	"image"
	"image/color"
)

// Draw 把人脸检测框、眼睛和关键点绘制到图片上。
func Draw(dst *image.RGBA, faces []Face) {
	boxColor := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	eyeColor := color.RGBA{R: 0, G: 255, B: 0, A: 255}
	lmColor := color.RGBA{R: 0, G: 0, B: 255, A: 255}

	for _, f := range faces {
		// 检测框
		x1, y1 := int(f.Box[0]), int(f.Box[1])
		x2, y2 := int(f.Box[2]), int(f.Box[3])
		rect(dst, x1, y1, x2, y2, boxColor)

		// 眼睛
		if f.LeftEye != [2]float32{} {
			dot(dst, int(f.LeftEye[0]), int(f.LeftEye[1]), 3, eyeColor)
		}
		if f.RightEye != [2]float32{} {
			dot(dst, int(f.RightEye[0]), int(f.RightEye[1]), 3, eyeColor)
		}

		// 关键点（15 点稀疏集，仅画点）
		for _, lm := range f.Landmarks {
			if lm != [2]float32{} {
				dot(dst, int(lm[0]), int(lm[1]), 2, lmColor)
			}
		}
	}
}

// rect 画矩形框（2px 边框）。
func rect(img *image.RGBA, x1, y1, x2, y2 int, c color.RGBA) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for i := x1; i <= x2; i++ {
		for _, y := range []int{y1, y2} {
			for by := -1; by <= 1; by++ {
				px, py := i, y+by
				if px >= 0 && py >= 0 && px < w && py < h {
					img.Set(px, py, c)
				}
			}
		}
	}
	for i := y1; i <= y2; i++ {
		for _, x := range []int{x1, x2} {
			for bx := -1; bx <= 1; bx++ {
				px, py := x+bx, i
				if px >= 0 && py >= 0 && px < w && py < h {
					img.Set(px, py, c)
				}
			}
		}
	}
}

// dot 画一个实心圆点。
func dot(img *image.RGBA, x, y, r int, c color.RGBA) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				px, py := x+dx, y+dy
				if px >= 0 && py >= 0 && px < w && py < h {
					img.Set(px, py, c)
				}
			}
		}
	}
}
