// Package background 的背景色自动检测算法。
//
// 核心思路：
//  1. 采样图片边缘区域（通常为外沿 5%）的像素
//  2. 用 K-Means (k=5) 聚类，找到 5 个主要颜色簇
//  3. 用角点颜色做验证：只保留在角点中也出现过的簇（排除"主体恰好延伸到边缘"的干扰）
//  4. 对每个像素，计算到所有有效背景色簇的最小 ΔE
//  5. tolerance 使用 P90 百分位（排除前景污染的离群值）
//  6. 当有多个簇时（渐变背景），根据簇间距离自动放大容差
//
// K-Means 初始化策略：k-means++（按距离远近来选初始中心），15 次迭代收敛。
// 角点验证阈值 ΔE < 20（≈ 肉眼能看出明显差异的边界）。
package background

import (
	"image"
	"math"
	"sort"
)

func sampleEdgePixels(img image.Image, regionFrac float64) []uint8 {
	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}

	rw := int(float64(min(w, h)) * regionFrac / 100.0)
	if rw < 1 {
		rw = 1
	}
	if rw*2 >= w || rw*2 >= h {
		rw = min(w, h) / 4
		if rw < 1 {
			rw = 1
		}
	}

	maxSamples := (w*2 + h*2) * rw
	if maxSamples > 1000000 {
		maxSamples = 1000000
	}
	data := make([]uint8, 0, maxSamples*4)

	addPixel := func(x, y int) {
		if x < 0 || x >= w || y < 0 || y >= h {
			return
		}
		r, g, bb, a := img.At(x, y).RGBA()
		data = append(data, uint8(r>>8), uint8(g>>8), uint8(bb>>8), uint8(a>>8))
	}

	for y := 0; y < rw; y++ {
		for x := 0; x < w; x += max(1, w/100) {
			addPixel(x, y)
			addPixel(x, h-1-y)
		}
	}

	for x := 0; x < rw; x++ {
		for y := rw; y < h-rw; y += max(1, h/100) {
			addPixel(x, y)
			addPixel(w-1-x, y)
		}
	}

	return data
}

type labColor struct{ L, a, b float64 }

func detectBackgroundColors(data []uint8, img image.Image, regionFrac float64) (primaryL, primaryA, primaryB float64, bgLabs [][3]float64, bgRGBs [][3]uint8) {
	if len(data) < 8 {
		return 0, 0, 0, nil, nil
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	rw := int(float64(min(w, h)) * regionFrac / 100.0)
	if rw < 1 {
		rw = 1
	}

	const maxPixels = 5000
	step := max(1, len(data)/4/maxPixels)
	if step < 4 {
		step = 4
	}

	samples := make([]labColor, 0, len(data)/4/step)
	var sampleRGBA [][3]uint8
	for i := 0; i < len(data); i += step * 4 {
		if i+3 < len(data) {
			l, aa, bb := rgbToLab(data[i], data[i+1], data[i+2])
			samples = append(samples, labColor{l, aa, bb})
			sampleRGBA = append(sampleRGBA, [3]uint8{data[i], data[i+1], data[i+2]})
		}
	}

	if len(samples) < 6 {
		if len(samples) == 0 {
			return 0, 0, 0, nil, nil
		}
		return samples[0].L, samples[0].a, samples[0].b,
			[][3]float64{{samples[0].L, samples[0].a, samples[0].b}},
			[][3]uint8{sampleRGBA[0]}
	}

	// K-Means with k=5, initialized via k-means++ (spread by L distance)
	const k = 5
	n := len(samples)
	centroids := make([]labColor, k)
	centroids[0] = samples[0]
	for j := 1; j < k; j++ {
		maxD := -1.0
		best := 0
		for i := 0; i < n; i++ {
			minD := math.MaxFloat64
			for p := 0; p < j; p++ {
				d := deltaE(samples[i].L, samples[i].a, samples[i].b,
					centroids[p].L, centroids[p].a, centroids[p].b)
				if d < minD {
					minD = d
				}
			}
			if minD > maxD {
				maxD = minD
				best = i
			}
		}
		centroids[j] = samples[best]
	}

	labels := make([]int, n)
	var count [k]int
	for iter := 0; iter < 15; iter++ {
		for i := 0; i < n; i++ {
			bestD := deltaE(samples[i].L, samples[i].a, samples[i].b,
				centroids[0].L, centroids[0].a, centroids[0].b)
			bestL := 0
			for j := 1; j < k; j++ {
				d := deltaE(samples[i].L, samples[i].a, samples[i].b,
					centroids[j].L, centroids[j].a, centroids[j].b)
				if d < bestD {
					bestD = d
					bestL = j
				}
			}
			labels[i] = bestL
		}
		var sumL, suma, sumb [k]float64
		count = [k]int{}
		for i := 0; i < n; i++ {
			label := labels[i]
			sumL[label] += samples[i].L
			suma[label] += samples[i].a
			sumb[label] += samples[i].b
			count[label]++
		}
		for j := 0; j < k; j++ {
			if count[j] > 0 {
				centroids[j] = labColor{
					sumL[j] / float64(count[j]),
					suma[j] / float64(count[j]),
					sumb[j] / float64(count[j]),
				}
			}
		}
	}

	// Compute RGB centroids for passing to despill
	rgbCentroids := make([][3]uint8, k)
	for j := 0; j < k; j++ {
		if count[j] == 0 {
			continue
		}
		var rSum, gSum, bSum float64
		var cnt int
		for si, label := range labels {
			if label == j {
				rSum += float64(sampleRGBA[si][0])
				gSum += float64(sampleRGBA[si][1])
				bSum += float64(sampleRGBA[si][2])
				cnt++
			}
		}
		if cnt > 0 {
			rgbCentroids[j] = [3]uint8{
				uint8(rSum / float64(cnt)),
				uint8(gSum / float64(cnt)),
				uint8(bSum / float64(cnt)),
			}
		}
	}

	// Corner verification: average color of each corner 5% region
	cornerAvgs := make([][3]float64, 4)
	for ci, cx := range []int{0, w - rw, 0, w - rw} {
		cy := 0
		if ci >= 2 {
			cy = h - rw
		}
		var rS, gS, bS float64
		var cnt int
		for dy := 0; dy < rw && cy+dy < h; dy++ {
			for dx := 0; dx < rw && cx+dx < w; dx++ {
				r, g, bb, _ := img.At(cx+dx, cy+dy).RGBA()
				rS += float64(r >> 8)
				gS += float64(g >> 8)
				bS += float64(bb >> 8)
				cnt++
			}
		}
		if cnt > 0 {
			L, a, b := rgbToLab(uint8(rS/float64(cnt)), uint8(gS/float64(cnt)), uint8(bS/float64(cnt)))
			cornerAvgs[ci] = [3]float64{L, a, b}
		}
	}

	// Score each cluster by corner matches (ΔE < 20)
	type scoredCluster struct {
		idx   int
		count int
		score int
		lab   labColor
		rgb   [3]uint8
	}
	var candidates []scoredCluster
	for j := 0; j < k; j++ {
		if count[j] < n/25 {
			continue
		}
		score := 0
		for _, ca := range cornerAvgs {
			if ca[0] == 0 && ca[1] == 0 && ca[2] == 0 {
				continue
			}
			de := deltaE(centroids[j].L, centroids[j].a, centroids[j].b, ca[0], ca[1], ca[2])
			if de < 20 {
				score++
			}
		}
		candidates = append(candidates, scoredCluster{
			idx: j, count: count[j], score: score,
			lab: centroids[j], rgb: rgbCentroids[j],
		})
	}

	sort.Slice(candidates, func(a, b int) bool {
		if candidates[a].score != candidates[b].score {
			return candidates[a].score > candidates[b].score
		}
		return candidates[a].count > candidates[b].count
	})

	if len(candidates) == 0 {
		return centroids[0].L, centroids[0].a, centroids[0].b,
			[][3]float64{{centroids[0].L, centroids[0].a, centroids[0].b}},
			[][3]uint8{rgbCentroids[0]}
	}

	best := candidates[0]
	primaryL, primaryA, primaryB = best.lab.L, best.lab.a, best.lab.b
	bgLabs = append(bgLabs, [3]float64{primaryL, primaryA, primaryB})
	bgRGBs = append(bgRGBs, best.rgb)

	for _, c := range candidates[1:] {
		if c.score >= 1 {
			bgLabs = append(bgLabs, [3]float64{c.lab.L, c.lab.a, c.lab.b})
			bgRGBs = append(bgRGBs, c.rgb)
		}
	}
	return
}

func autoToleranceMulti(data []uint8, bgLabs [][3]float64) float64 {
	if len(data) < 8 || len(bgLabs) == 0 {
		return 10.0
	}

	const maxPixels = 3000
	step := max(1, len(data)/4/maxPixels)
	if step < 4 {
		step = 4
	}

	deValues := make([]float64, 0, len(data)/4/step)
	for i := 0; i < len(data); i += step * 4 {
		if i+3 < len(data) {
			l, a, b := rgbToLab(data[i], data[i+1], data[i+2])
			minDE := math.MaxFloat64
			for _, bg := range bgLabs {
				de := deltaE(l, a, b, bg[0], bg[1], bg[2])
				if de < minDE {
					minDE = de
				}
			}
			deValues = append(deValues, minDE)
		}
	}

	if len(deValues) == 0 {
		return 10.0
	}

	sort.Float64s(deValues)
	p90 := deValues[int(float64(len(deValues))*0.90)]

	// For gradient backgrounds with multiple clusters, scale up tolerance
	// based on the spread between cluster centroids.
	if len(bgLabs) > 2 {
		maxSpread := 0.0
		for i := 0; i < len(bgLabs); i++ {
			for j := i + 1; j < len(bgLabs); j++ {
				de := deltaE(bgLabs[i][0], bgLabs[i][1], bgLabs[i][2],
					bgLabs[j][0], bgLabs[j][1], bgLabs[j][2])
				if de > maxSpread {
					maxSpread = de
				}
			}
		}
		scale := 1.0 + maxSpread/50.0
		if scale > 2.0 {
			scale = 2.0
		}
		p90 *= scale
	}

	if p90 < 3.0 {
		p90 = 3.0
	}
	if p90 > 60.0 {
		p90 = 60.0
	}
	return p90
}
