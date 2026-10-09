package upscale

import "image"

// ExtractTile copies the tile×tile window at (x0,y0) of img into buf as CHW
// float32 in [0,1] (RGB); reads outside the image clamp to the edge. buf must
// have length 3*tile*tile.
func ExtractTile(img image.Image, x0, y0, tile int, buf []float32) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	plane := tile * tile
	const inv = 1.0 / 65535.0
	for ly := 0; ly < tile; ly++ {
		sy := clampInt(y0+ly, 0, h-1)
		row := ly * tile
		for lx := 0; lx < tile; lx++ {
			sx := clampInt(x0+lx, 0, w-1)
			r, g, bl, _ := img.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
			di := row + lx
			buf[di] = float32(r) * inv
			buf[plane+di] = float32(g) * inv
			buf[2*plane+di] = float32(bl) * inv
		}
	}
}

// BlendWeights returns per-axis feather weights for a tile. The first tile on an
// axis contributes fully (all 1); later tiles ramp linearly across the overlap
// so seams between adjacent tiles are invisible.
func BlendWeights(tile, scale, overlap int) (full, ramp []float32) {
	ow := tile * scale
	full = make([]float32, ow)
	for i := range full {
		full[i] = 1
	}
	ramp = make([]float32, ow)
	ov := float32(overlap * scale)
	for o := range ramp {
		v := float32(o) / ov
		if v > 1 {
			v = 1
		}
		ramp[o] = v
	}
	return full, ramp
}

// BlendTile composites one tile's network output (CHW float32, values ~[0,1])
// into dst, mapping source pixel (x0,y0) to dst pixel (x0*scale, y0*scale).
// ax/ay are the per-axis feather weights; overlapping regions are merged.
func BlendTile(dst *image.NRGBA, tile []float32, x0, y0, destW, destH, scale int, ax, ay []float32) {
	ow := len(ax)
	plane := ow * ow
	baseX, baseY := x0*scale, y0*scale
	for oy := 0; oy < ow; oy++ {
		gy := baseY + oy
		if gy >= destH {
			break
		}
		aY := ay[oy]
		row := oy * ow
		for ox := 0; ox < ow; ox++ {
			gx := baseX + ox
			if gx >= destW {
				break
			}
			ti := row + ox
			r := tile[ti] * 255
			g := tile[plane+ti] * 255
			b := tile[2*plane+ti] * 255
			di := (gy*destW + gx) * 4
			a := aY * ax[ox]
			if a >= 1 {
				dst.Pix[di] = clampU8(r)
				dst.Pix[di+1] = clampU8(g)
				dst.Pix[di+2] = clampU8(b)
			} else {
				inv := 1 - a
				dst.Pix[di] = clampU8(float32(dst.Pix[di])*inv + r*a)
				dst.Pix[di+1] = clampU8(float32(dst.Pix[di+1])*inv + g*a)
				dst.Pix[di+2] = clampU8(float32(dst.Pix[di+2])*inv + b*a)
			}
			dst.Pix[di+3] = 255
		}
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampU8(v float32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
