package wmremove

import (
	"image"
	"image/color"
)

func preprocessImage(img image.Image, w, h int) []uint8 {
	data := make([]uint8, 3*w*h)
	idx := 0
	for c := range 3 {
		for y := range h {
			for x := range w {
				r, g, b, _ := img.At(x, y).RGBA()
				switch c {
				case 0:
					data[idx] = uint8(r >> 8)
				case 1:
					data[idx] = uint8(g >> 8)
				case 2:
					data[idx] = uint8(b >> 8)
				}
				idx++
			}
		}
	}
	return data
}

func preprocessMask(mask *image.Gray, w, h int) []uint8 {
	data := make([]uint8, w*h)
	for y := range h {
		for x := range w {
			if mask.GrayAt(x, y).Y > 128 {
				data[y*w+x] = 0
			} else {
				data[y*w+x] = 255
			}
		}
	}
	return data
}

func GenerateMask(w, h, x, y, mw, mh int) *image.Gray {
	mask := image.NewGray(image.Rect(0, 0, w, h))
	for iy := y; iy < y+mh && iy < h; iy++ {
		for ix := x; ix < x+mw && ix < w; ix++ {
			mask.SetGray(ix, iy, color.Gray{Y: 255})
		}
	}
	return mask
}
