package ocr

import (
	"math"
	"sort"
)

type ocrPoint struct{ x, y int }

// detPostProcess converts the DBNet probability map into text bounding boxes.
// Pipeline: threshold → dilation → connected components → box extraction → box_thresh → unclip → NMS.
func detPostProcess(probMap []float32, h, w int, scaleX, scaleY float64, padLeft, padTop int, origW, origH int) [][4][2]int {
	const threshold = 0.3
	const boxThresh = 0.5
	const unclipRatio = 1.6
	const nmsThreshold = 0.5

	// Step 1: Threshold the probability map to binary mask
	binary := make([][]bool, h)
	for y := 0; y < h; y++ {
		binary[y] = make([]bool, w)
		for x := 0; x < w; x++ {
			idx := y*w + x
			binary[y][x] = probMap[idx] > threshold
		}
	}

	// Step 1b: Morphological dilation (2x2 kernel) to merge nearby text pixels
	dilated := make([][]bool, h)
	for y := 0; y < h; y++ {
		dilated[y] = make([]bool, w)
		for x := 0; x < w; x++ {
			if binary[y][x] {
				dilated[y][x] = true
				continue
			}
			// Check 2x2 neighborhood: if any neighbor is set, dilate
			if (x > 0 && binary[y][x-1]) ||
				(y > 0 && binary[y-1][x]) ||
				(x > 0 && y > 0 && binary[y-1][x-1]) {
				dilated[y][x] = true
			}
		}
	}

	// Step 2: Connected components (BFS on dilated mask)
	type component struct {
		points []ocrPoint
	}
	visited := make([][]bool, h)
	for y := 0; y < h; y++ {
		visited[y] = make([]bool, w)
	}

	var components []component
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !dilated[y][x] || visited[y][x] {
				continue
			}
			var comp component
			queue := []ocrPoint{{x, y}}
			visited[y][x] = true
			for len(queue) > 0 {
				p := queue[0]
				queue = queue[1:]
				comp.points = append(comp.points, p)
				for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
					nx, ny := p.x+d[0], p.y+d[1]
					if nx >= 0 && nx < w && ny >= 0 && ny < h && dilated[ny][nx] && !visited[ny][nx] {
						visited[ny][nx] = true
						queue = append(queue, ocrPoint{nx, ny})
					}
				}
			}
			if len(comp.points) >= 3 {
				components = append(components, comp)
			}
		}
	}

	// Step 3: Convert each component to a box, compute average score, apply box_thresh + unclip
	var boxes [][4][2]int
	var scores []float32
	for _, comp := range components {
		// Get minimum bounding rectangle
		box := minAreaRect(comp.points)
		if box == nil {
			continue
		}

		// Compute average probability within the box (box_thresh filtering)
		avgScore := avgBoxScore(probMap, *box, w, h)
		if avgScore < boxThresh {
			continue
		}

		// Unclip: expand the box by unclipRatio
		expanded := unclipBox(*box, unclipRatio)
		if expanded == nil {
			continue
		}

		// Get the minimum bounding rectangle of the expanded points
		finalBox := minAreaRect(expanded)
		if finalBox == nil {
			continue
		}

		boxes = append(boxes, *finalBox)
		scores = append(scores, avgScore)
	}

	if len(boxes) == 0 {
		return nil
	}

	// Step 4: NMS
	keep := nms(boxes, scores, nmsThreshold)

	// Step 5: Map boxes back to original image coordinates
	result := make([][4][2]int, 0, len(keep))
	for _, idx := range keep {
		box := boxes[idx]
		for i := 0; i < 4; i++ {
			x := int(math.Round(float64(box[i][0]-padLeft) * float64(origW) / (scaleX * DetInputSize)))
			y := int(math.Round(float64(box[i][1]-padTop) * float64(origH) / (scaleY * DetInputSize)))
			if x < 0 {
				x = 0
			}
			if y < 0 {
				y = 0
			}
			box[i][0] = x
			box[i][1] = y
		}
		for i := 0; i < 4; i++ {
			if box[i][0] < 0 {
				box[i][0] = 0
			}
			if box[i][1] < 0 {
				box[i][1] = 0
			}
		}
		result = append(result, box)
	}
	return result
}

// avgBoxScore computes the average probability map value within the bounding box.
func avgBoxScore(probMap []float32, box [4][2]int, mapW, mapH int) float32 {
	minX, maxX := box[0][0], box[0][0]
	minY, maxY := box[0][1], box[0][1]
	for _, p := range box[1:] {
		if p[0] < minX {
			minX = p[0]
		}
		if p[0] > maxX {
			maxX = p[0]
		}
		if p[1] < minY {
			minY = p[1]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}
	// Clamp to map bounds
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX >= mapW {
		maxX = mapW - 1
	}
	if maxY >= mapH {
		maxY = mapH - 1
	}
	if maxX <= minX || maxY <= minY {
		return 0
	}

	var sum float32
	var count int
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			sum += probMap[y*mapW+x]
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float32(count)
}

// unclipBox expands a bounding box outward by a factor proportional to
// area/perimeter * unclipRatio. This gives the recognition model more context.
// Returns the expanded box as 4 corner points.
func unclipBox(box [4][2]int, ratio float64) []ocrPoint {
	// Calculate the minimum bounding rectangle dimensions
	minX, maxX := box[0][0], box[0][0]
	minY, maxY := box[0][1], box[0][1]
	for _, p := range box[1:] {
		if p[0] < minX {
			minX = p[0]
		}
		if p[0] > maxX {
			maxX = p[0]
		}
		if p[1] < minY {
			minY = p[1]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}

	w := maxX - minX
	h := maxY - minY
	if w <= 0 || h <= 0 {
		return nil
	}

	// area = w * h, perimeter = 2 * (w + h)
	// distance = area * ratio / perimeter
	area := float64(w * h)
	perimeter := float64(2 * (w + h))
	if perimeter <= 0 {
		return nil
	}
	distance := area * ratio / perimeter

	// Expand the rectangle by distance on all sides
	expMinX := minX - int(distance)
	expMinY := minY - int(distance)
	expMaxX := maxX + int(distance)
	expMaxY := maxY + int(distance)

	return []ocrPoint{
		{expMinX, expMinY},
		{expMaxX, expMinY},
		{expMaxX, expMaxY},
		{expMinX, expMaxY},
	}
}

// minAreaRect finds the minimum-area axis-aligned rectangle for a set of points.
func minAreaRect(points []ocrPoint) *[4][2]int {
	if len(points) < 3 {
		return nil
	}
	minX, maxX := points[0].x, points[0].x
	minY, maxY := points[0].y, points[0].y
	for _, p := range points[1:] {
		if p.x < minX {
			minX = p.x
		}
		if p.x > maxX {
			maxX = p.x
		}
		if p.y < minY {
			minY = p.y
		}
		if p.y > maxY {
			maxY = p.y
		}
	}
	return &[4][2]int{
		{minX, minY},
		{maxX, minY},
		{maxX, maxY},
		{minX, maxY},
	}
}

// nms performs Non-Maximum Suppression on bounding boxes.
func nms(boxes [][4][2]int, scores []float32, threshold float32) []int {
	if len(boxes) == 0 {
		return nil
	}
	idx := make([]int, len(boxes))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool {
		return scores[idx[i]] > scores[idx[j]]
	})
	var keep []int
	for len(idx) > 0 {
		best := idx[0]
		keep = append(keep, best)
		idx = idx[1:]
		var remaining []int
		bestRect := boxToRect(boxes[best])
		for _, i := range idx {
			rect := boxToRect(boxes[i])
			iou := iouRect(bestRect, rect)
			if iou <= threshold {
				remaining = append(remaining, i)
			}
		}
		idx = remaining
	}
	return keep
}

type rect struct{ x0, y0, x1, y1 int }

func boxToRect(box [4][2]int) rect {
	minX, maxX := box[0][0], box[0][0]
	minY, maxY := box[0][1], box[0][1]
	for _, p := range box[1:] {
		if p[0] < minX {
			minX = p[0]
		}
		if p[0] > maxX {
			maxX = p[0]
		}
		if p[1] < minY {
			minY = p[1]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}
	return rect{minX, minY, maxX, maxY}
}

func iouRect(a, b rect) float32 {
	ix0 := maxInt(a.x0, b.x0)
	iy0 := maxInt(a.y0, b.y0)
	ix1 := minInt(a.x1, b.x1)
	iy1 := minInt(a.y1, b.y1)
	if ix0 >= ix1 || iy0 >= iy1 {
		return 0
	}
	inter := (ix1 - ix0) * (iy1 - iy0)
	areaA := (a.x1 - a.x0) * (a.y1 - a.y0)
	areaB := (b.x1 - b.x0) * (b.y1 - b.y0)
	union := areaA + areaB - inter
	if union <= 0 {
		return 0
	}
	return float32(inter) / float32(union)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
