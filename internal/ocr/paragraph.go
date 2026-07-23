package ocr

import (
	"strings"
	"unicode"
)

// ParagraphParse 段落分析 — 移植自 Umi-OCR (hiroi-sora)
//
// 对已经过 GapTree 排序的文本块，判断其段落关系。
// 基于左右边缘对齐程度分组，处理单行段归并，
// 并在 CJK / Latin 字符间智能插入空格。

const paraTH = 1.2 // 行高对比阈值

// ppUnit wraps an OCRLine with its first/last characters for spacing decisions.
type ppUnit struct {
	x0, y0, x2, y2 int
	first, last    rune
	line           OCRLine
}

// paragraphParse analyzes a list of OCR lines (sorted in reading order within
// a single column) and returns the text with proper paragraph breaks and spacing.
func paragraphParse(lines []OCRLine) string {
	if len(lines) == 0 {
		return ""
	}

	units := make([]*ppUnit, len(lines))
	for i, l := range lines {
		x0, y0, x2, y2 := lineBBox(l)
		first := rune(0)
		last := rune(0)
		if len(l.Text) > 0 {
			runes := []rune(l.Text)
			first = runes[0]
			last = runes[len(runes)-1]
		}
		units[i] = &ppUnit{x0: x0, y0: y0, x2: x2, y2: y2, first: first, last: last, line: l}
	}

	// Step 1: Group into paragraphs based on left/right edge alignment.
	type para struct {
		units     []*ppUnit
		lineSpace float64
	}
	var paras []para

	paraL, paraTop, paraR, paraBottom := units[0].x0, units[0].y0, units[0].x2, units[0].y2
	paraLineH := float64(paraBottom - paraTop)
	var paraLineS *float64
	nowPara := []*ppUnit{units[0]}

	for i := 1; i < len(units); i++ {
		l, top, r, bottom := units[i].x0, units[i].y0, units[i].x2, units[i].y2
		h := float64(bottom - top)
		ls := float64(top - paraBottom)

		// Check if this block is on the same visual row as the previous block.
		// If they are on the same row (vertical overlap > 50%) but have a large
		// horizontal gap, they are separate UI elements — force a paragraph break.
		sameRow := float64(bottom-top) > 0 && float64(top) < float64(paraBottom) &&
			float64(bottom) > float64(paraTop)
		sameRow = sameRow && float64(minInt(bottom, paraBottom)-maxInt(top, paraTop))*2 >
			float64(bottom-top+paraBottom-paraTop)*0.5
		horizGap := float64(l) - float64(paraR)
		if sameRow && horizGap > paraLineH*0.5 {
			paras = append(paras, para{units: nowPara, lineSpace: paraLineSVal(paraLineS)})
			nowPara = []*ppUnit{units[i]}
			paraL, paraR, paraLineH = l, r, h
			paraLineS = nil
			paraBottom = bottom
			continue
		}

		samePara := absFloat(float64(paraL)-float64(l)) <= paraLineH*paraTH &&
			absFloat(float64(paraR)-float64(r)) <= paraLineH*paraTH &&
			(paraLineS == nil || ls < *paraLineS+paraLineH*0.5)

		if samePara {
			paraL = int((float64(paraL) + float64(l)) / 2)
			paraR = int((float64(paraR) + float64(r)) / 2)
			paraLineH = (paraLineH + h) / 2
			if paraLineS == nil {
				s := ls
				paraLineS = &s
			} else {
				s := (*paraLineS + ls) / 2
				paraLineS = &s
			}
			nowPara = append(nowPara, units[i])
		} else {
			paras = append(paras, para{units: nowPara, lineSpace: paraLineSVal(paraLineS)})
			nowPara = []*ppUnit{units[i]}
			paraL, paraR, paraLineH = l, r, h
			paraLineS = nil
		}
		paraBottom = bottom
	}
	paras = append(paras, para{units: nowPara, lineSpace: paraLineSVal(paraLineS)})

	// Step 2: Merge single-line paragraphs into adjacent paragraphs.
	for i := len(paras) - 1; i >= 0; i-- {
		if len(paras[i].units) != 1 {
			continue
		}
		u := paras[i].units[0]
		l, top, r, bottom := u.x0, u.y0, u.x2, u.y2
		upFlag, downFlag := false, false

		if i > 0 {
			prev := paras[i-1].units[len(paras[i-1].units)-1]
			upH := float64(prev.y2 - prev.y0)
			upDist := absFloat(float64(prev.x0) - float64(l))
			upFlag = upDist <= upH*paraTH && float64(r) <= float64(prev.x2)+upH*paraTH
			vertGap := float64(top - prev.y2)
			if vertGap > upH {
				upFlag = false
			} else if paras[i-1].lineSpace >= 0 && vertGap > paras[i-1].lineSpace+upH*0.5 {
				upFlag = false
			}
		}

		if i < len(paras)-1 {
			next := paras[i+1].units[0]
			downH := float64(next.y2 - next.y0)
			downL := float64(next.x0)
			if downL-downH*paraTH <= float64(l) && float64(l) <= downL+downH*(1+paraTH) {
				if len(paras[i+1].units) > 1 {
					downFlag = absFloat(float64(next.x2)-float64(r)) <= downH*paraTH
				} else {
					downFlag = float64(next.x2)-downH*paraTH < float64(r)
				}
			}
			vertGap := float64(next.y0 - bottom)
			if vertGap > downH*1.5 {
				downFlag = false
			} else if paras[i+1].lineSpace >= 0 && vertGap > paras[i+1].lineSpace+downH*0.5 {
				downFlag = false
			}
		}

		if upFlag && downFlag {
			if float64(top-paras[i-1].units[len(paras[i-1].units)-1].y2) < float64(paras[i+1].units[0].y0-bottom) {
				paras[i-1].units = append(paras[i-1].units, u)
			} else {
				paras[i+1].units = append([]*ppUnit{u}, paras[i+1].units...)
			}
			paras = append(paras[:i], paras[i+1:]...)
		} else if upFlag {
			paras[i-1].units = append(paras[i-1].units, u)
			paras = append(paras[:i], paras[i+1:]...)
		} else if downFlag {
			paras[i+1].units = append([]*ppUnit{u}, paras[i+1].units...)
			paras = append(paras[:i], paras[i+1:]...)
		}
	}

	// Step 3: Build output with proper separators.
	var result strings.Builder
	for pi, para := range paras {
		for li := 0; li < len(para.units); li++ {
			if li > 0 {
				sep := wordSeparator(para.units[li-1].last, para.units[li].first)
				result.WriteString(sep)
			}
			result.WriteString(para.units[li].line.Text)
		}
		if pi < len(paras)-1 {
			result.WriteString("\n\n")
		}
	}
	out := result.String()
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	// Apply CJK spacing: insert spaces between CJK and Latin characters.
	return cjkLatinSpacing(out)
}

// lineBBox returns the bounding box of an OCR line.
func lineBBox(l OCRLine) (x0, y0, x2, y2 int) {
	x0, x2 = l.BBox[0][0], l.BBox[0][0]
	y0, y2 = l.BBox[0][1], l.BBox[0][1]
	for _, p := range l.BBox[1:] {
		if p[0] < x0 {
			x0 = p[0]
		}
		if p[0] > x2 {
			x2 = p[0]
		}
		if p[1] < y0 {
			y0 = p[1]
		}
		if p[1] > y2 {
			y2 = p[1]
		}
	}
	return
}

// wordSeparator determines the spacing between two characters.
// CJK + CJK → no space; CJK + Latin → space; hyphen → no space;
// before punctuation → no space.
func wordSeparator(a, b rune) string {
	if isCJK(a) && isCJK(b) {
		return ""
	}
	if a == '-' {
		return ""
	}
	if unicode.IsPunct(b) {
		return ""
	}
	return " "
}

// paraLineSVal safely dereferences a *float64.
func paraLineSVal(s *float64) float64 {
	if s == nil {
		return -1 // sentinel: no data
	}
	return *s
}

// absFloat returns the absolute value of a float64.
func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
