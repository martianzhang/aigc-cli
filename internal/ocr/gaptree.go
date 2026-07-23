package ocr

import (
	"math"
	"sort"
)

// GapTree 排序算法 — 移植自 Umi-OCR (hiroi-sora)
// https://github.com/hiroi-sora/GapTree_Sort_Algorithm
//
// 原理：基于行间隙树构建布局树，前序遍历得到自然阅读顺序，
// 天然支持多栏排版（论文、报纸、代码截图等）。

type boxUnit struct {
	x0, y0, x2, y2 int
	idx            int
}

type gtUnit struct {
	x0, y0, x2, y2 int
}

type gtGap struct {
	l, r   int
	rowBeg int
}

type gtCut struct {
	l, r   int
	rowBeg int
	rowEnd int
}

type gtNode struct {
	xLeft    int
	xRight   int
	rTop     int
	rBottom  int
	units    []int // indices into the original unit list
	children []*gtNode
}

// sortBoxesGapTreeRects sorts raw [4][2]int bboxes using the GapTree algorithm.
// Applies global rotation normalization before sorting.
func sortBoxesGapTreeRects(boxes [][4][2]int) {
	if len(boxes) < 2 {
		return
	}
	// Global rotation estimation: compute median angle and normalize.
	angle := estimateBoxesRotation(boxes)
	normalizeBoxesRotation(boxes, angle)

	units := make([]boxUnit, len(boxes))
	for i, b := range boxes {
		minX, maxX := b[0][0], b[0][0]
		minY, maxY := b[0][1], b[0][1]
		for _, p := range b[1:] {
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
		units[i] = boxUnit{x0: minX, y0: minY, x2: maxX, y2: maxY, idx: i}
	}
	sorted := gapTreeSort(units)
	result := make([][4][2]int, len(sorted))
	for i, u := range sorted {
		result[i] = boxes[u.idx]
	}
	// Final Y-order correction: within the same visual column (x-overlapping),
	// ensure higher y0 comes later. This fixes GapTree edge cases where
	// a narrow box below a full-width box gets placed before it.
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			// Check if boxes overlap horizontally (same column).
			xi1, xi2 := result[i][0][0], result[i][1][0]
			xj1, xj2 := result[j][0][0], result[j][1][0]
			if xi2 <= xj1 || xj2 <= xi1 {
				continue // different columns, skip
			}
			// Same column: if the later box starts higher (lower y0), swap.
			if result[j][0][1] < result[i][0][1] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	copy(boxes, result)
}

// gapTreeSort is the core GapTree sort logic.
func gapTreeSort(units []boxUnit) []boxUnit {
	if len(units) < 2 {
		return units
	}
	// Wrap into internal units.
	gtUnits := make([]gtUnit, len(units))
	pageL, pageR := int(1e9), -1
	for i, u := range units {
		gtUnits[i] = gtUnit{x0: u.x0, y0: u.y0, x2: u.x2, y2: u.y2}
		if u.x0 < pageL {
			pageL = u.x0
		}
		if u.x2 > pageR {
			pageR = u.x2
		}
	}
	// Stable sort: use a permutation to avoid modifying the original.
	order := make([]int, len(gtUnits))
	for i := range order {
		order[i] = i
	}
	sortGTSlice(order, func(i, j int) bool {
		return gtUnits[order[i]].y0 < gtUnits[order[j]].y0
	})

	cuts, rowIndices := gapTreeGetCutsRows(gtUnits, order, pageL, pageR)
	root := gapTreeBuildLayout(cuts, rowIndices, order)
	nodes := gapTreePreorder(root)

	var result []boxUnit
	for _, node := range nodes {
		for _, ui := range node.units {
			result = append(result, units[ui])
		}
	}
	return result
}

// gapTreeGetCutsRows finds rows and vertical cuts from unit positions.
func gapTreeGetCutsRows(units []gtUnit, order []int, pageL, pageR int) ([]gtCut, [][]int) {
	pageL--
	pageR++

	var rows [][]int // each row holds indices into units
	var completedCuts []gtCut
	var gaps []gtGap
	rowIdx := 0

	i := 0
	for i < len(order) {
		ui := order[i]
		uH := units[ui].y2 - units[ui].y0
		uBottom := units[ui].y2
		row := []int{ui}
		for j := i + 1; j < len(order); j++ {
			nH := units[order[j]].y2 - units[order[j]].y0
			minH := nH
			if uH < minH {
				minH = uH
			}
			overlap := uBottom - units[order[j]].y0
			if overlap < 1 || overlap*4 < minH {
				break
			}
			row = append(row, order[j])
			i = j
		}

		// Sort row left-to-right.
		sortGTSlice(row, func(a, b int) bool {
			if units[a].x0 != units[b].x0 {
				return units[a].x0 < units[b].x0
			}
			return units[a].x2 < units[b].x2
		})

		// Find gaps in this row.
		var rowGaps []gtGap
		searchStart := pageL
		for _, ri := range row {
			u := units[ri]
			if u.x0 > searchStart {
				rowGaps = append(rowGaps, gtGap{l: searchStart, r: u.x0, rowBeg: rowIdx})
			}
			if u.x2 > searchStart {
				searchStart = u.x2
			}
		}
		rowGaps = append(rowGaps, gtGap{l: searchStart, r: pageR, rowBeg: rowIdx})

		// Update persistent gaps.
		gaps, completedCuts = gapTreeUpdateGaps(gaps, rowGaps, completedCuts, rowIdx)

		rows = append(rows, row)
		i++
		rowIdx++
	}

	rowMax := len(rows) - 1
	for _, g := range gaps {
		completedCuts = append(completedCuts, gtCut{l: g.l, r: g.r, rowBeg: g.rowBeg, rowEnd: rowMax})
	}
	sortGTSlice(completedCuts, func(a, b gtCut) bool { return a.l < b.l })
	return completedCuts, rows
}

func gapTreeUpdateGaps(gaps, rowGaps []gtGap, cuts []gtCut, rowIdx int) ([]gtGap, []gtCut) {
	flags1 := make([]bool, len(gaps))
	flags2 := make([]bool, len(rowGaps))
	for i := range flags2 {
		flags2[i] = true
	}

	var newGaps []gtGap
	for i1, g1 := range gaps {
		for i2, g2 := range rowGaps {
			interL := maxInt(g1.l, g2.l)
			interR := minInt(g1.r, g2.r)
			if interL <= interR {
				newGaps = append(newGaps, gtGap{l: interL, r: interR, rowBeg: g1.rowBeg})
				flags1[i1] = true
				flags2[i2] = false
			}
		}
	}
	for i2, f2 := range flags2 {
		if f2 {
			newGaps = append(newGaps, rowGaps[i2])
		}
	}

	var removed []gtGap
	for i1, f1 := range flags1 {
		if !f1 {
			removed = append(removed, gaps[i1])
		}
	}

	rowMax := rowIdx - 1
	for _, dg := range removed {
		cuts = append(cuts, gtCut{l: dg.l, r: dg.r, rowBeg: dg.rowBeg, rowEnd: rowMax})
	}
	return newGaps, cuts
}

func gapTreeBuildLayout(cuts []gtCut, rows [][]int, order []int) *gtNode {
	// Build per-row gap lists.
	rowsGaps := make([][]gtGap, len(rows))
	for _, cut := range cuts {
		for ri := cut.rowBeg; ri <= cut.rowEnd && ri < len(rows); ri++ {
			rowsGaps[ri] = append(rowsGaps[ri], gtGap{l: cut.l, r: cut.r})
		}
	}

	root := &gtNode{
		xLeft:  cuts[0].l - 1,
		xRight: cuts[len(cuts)-1].r + 1,
		rTop:   -1, rBottom: -1,
	}
	completedNodes := []*gtNode{root}
	var nowNodes []*gtNode

	complete := func(node *gtNode) {
		nodeR := node.xRight - 2
		var maxNodes []*gtNode
		maxR := -2
		for _, cn := range completedNodes {
			if nodeR < cn.xLeft || nodeR > cn.xRight+1 {
				continue
			}
			if cn.rBottom >= node.rTop {
				continue
			}
			if cn.rBottom > maxR {
				maxR = cn.rBottom
				maxNodes = []*gtNode{cn}
				continue
			}
			if cn.rBottom == maxR {
				maxNodes = append(maxNodes, cn)
			}
		}
		parent := maxNodes[0]
		for _, n := range maxNodes[1:] {
			if n.xRight > parent.xRight {
				parent = n
			}
		}
		parent.children = append(parent.children, node)
		completedNodes = append(completedNodes, node)
	}

	for ri, row := range rows {
		rowGaps := rowsGaps[ri]
		// Check which nodes continue.
		var newNow []*gtNode
		for _, node := range nowNodes {
			lFlag, rFlag, compFlag := false, false, false
			for _, gap := range rowGaps {
				if gap.r == node.xLeft {
					lFlag = true
				}
				if gap.l == node.xRight {
					rFlag = true
				}
				if (node.xLeft < gap.l && gap.l < node.xRight) ||
					(node.xLeft < gap.r && gap.r < node.xRight) {
					compFlag = true
					break
				}
			}
			if !lFlag || !rFlag {
				compFlag = true
			}
			if compFlag {
				complete(node)
			} else {
				node.rBottom = ri
				newNow = append(newNow, node)
			}
		}
		nowNodes = newNow

		// Place block indices into columns.
		uIdx := 0
		gIdx := 0
		units := row
		for uIdx < len(units) {
			ui := units[uIdx]
			xL := rowGaps[gIdx].r
			xR := rowGaps[gIdx+1].l
			placed := false
			for _, node := range nowNodes {
				if node.xLeft == xL && node.xRight == xR {
					node.units = append(node.units, ui)
					placed = true
					break
				}
			}
			if placed {
				uIdx++
				continue
			}
			nowNodes = append(nowNodes, &gtNode{
				xLeft: xL, xRight: xR,
				rTop: ri, rBottom: ri,
				units: []int{ui},
			})
			uIdx++
		}
	}

	for _, node := range nowNodes {
		complete(node)
	}

	for _, node := range completedNodes {
		sortGTSlice(node.children, func(a, b *gtNode) bool { return a.xLeft < b.xLeft })
	}
	return root
}

func gapTreePreorder(root *gtNode) []*gtNode {
	if root == nil {
		return nil
	}
	var result []*gtNode
	stack := []*gtNode{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		result = append(result, node)
		for i := len(node.children) - 1; i >= 0; i-- {
			stack = append(stack, node.children[i])
		}
	}
	return result
}

// sortGTSlice sorts a slice using insertion sort (best for small OCR outputs).
func sortGTSlice[T any](s []T, less func(a, b T) bool) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && less(s[j], s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// estimateBoxesRotation computes the median rotation angle (radians) of bboxes.
// Uses the longer edge of each box for stability.
func estimateBoxesRotation(boxes [][4][2]int) float64 {
	if len(boxes) == 0 {
		return 0
	}
	angles := make([]float64, 0, len(boxes))
	for _, b := range boxes {
		dx1 := float64(b[1][0] - b[0][0])
		dy1 := float64(b[1][1] - b[0][1])
		dx2 := float64(b[2][0] - b[1][0])
		dy2 := float64(b[2][1] - b[1][1])
		w := math.Hypot(dx1, dy1)
		h := math.Hypot(dx2, dy2)
		var angle float64
		if w >= h {
			angle = math.Atan2(dy1, dx1)
		} else {
			angle = math.Atan2(dy2, dx2)
		}
		const angleThresh = 0.05
		if angle < -math.Pi/2+angleThresh {
			angle += math.Pi
		} else if angle >= math.Pi/2+angleThresh {
			angle -= math.Pi
		}
		angles = append(angles, angle)
	}
	sort.Float64s(angles)
	return angles[len(angles)/2]
}

// normalizeBoxesRotation rotates all bboxes by -angleRad to make them axis-aligned.
// Used as preprocessing before GapTree sorting.
func normalizeBoxesRotation(boxes [][4][2]int, angleRad float64) {
	const angleThresh = 0.05
	if math.Abs(angleRad) <= angleThresh {
		return
	}
	cosA := math.Cos(-angleRad)
	sinA := math.Sin(-angleRad)
	type pt struct{ x, y float64 }
	rotated := make([][4]pt, len(boxes))
	minX, minY := math.MaxFloat64, math.MaxFloat64
	for i, b := range boxes {
		for j := 0; j < 4; j++ {
			rx := float64(b[j][0])*cosA - float64(b[j][1])*sinA
			ry := float64(b[j][0])*sinA + float64(b[j][1])*cosA
			rotated[i][j] = pt{rx, ry}
			if rx < minX {
				minX = rx
			}
			if ry < minY {
				minY = ry
			}
		}
	}
	for i, b := range boxes {
		for j := 0; j < 4; j++ {
			b[j][0] = int(math.Round(rotated[i][j].x - minX))
			b[j][1] = int(math.Round(rotated[i][j].y - minY))
		}
		boxes[i] = b
	}
}
