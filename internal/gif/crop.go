package gif

import (
	"fmt"
	"strconv"
	"strings"
)

// CropMargins 表示从视频四边各裁掉的像素数（CSS margin 语义）。
// 零值表示不裁切。
type CropMargins struct {
	Top    int
	Right  int
	Bottom int
	Left   int
}

// Zero 报告四边是否都为 0（即不裁切）。
func (m CropMargins) Zero() bool {
	return m.Top == 0 && m.Right == 0 && m.Bottom == 0 && m.Left == 0
}

// String 以 CSS margin 简写形式输出：1 值（四边相等）、2 值（上下,左右）、4 值（上,右,下,左）。
func (m CropMargins) String() string {
	if m.Top == m.Right && m.Right == m.Bottom && m.Bottom == m.Left {
		return strconv.Itoa(m.Top)
	}
	if m.Top == m.Bottom && m.Left == m.Right {
		return fmt.Sprintf("%d,%d", m.Top, m.Left)
	}
	return fmt.Sprintf("%d,%d,%d,%d", m.Top, m.Right, m.Bottom, m.Left)
}

// ParseCropMargin 解析 --crop-margin 的值（CSS margin 简写，逗号或空格分隔）：
//
//	1 个值 = 四边相同       如 "40"
//	2 个值 = 上下,左右      如 "40,0"（只裁上下）
//	3 个值 = 上,左右,下     如 "40,0,60"
//	4 个值 = 上,右,下,左    如 "40,30,20,10"
//
// 空串返回零值（不裁切）。
func ParseCropMargin(s string) (CropMargins, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return CropMargins{}, nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
	if len(parts) > 4 {
		return CropMargins{}, fmt.Errorf("crop-margin %q: expected 1, 2, 3 or 4 values (CSS margin shorthand), got %d", s, len(parts))
	}
	vals := make([]int, len(parts))
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return CropMargins{}, fmt.Errorf("crop-margin %q: value %q must be a non-negative integer", s, p)
		}
		vals[i] = v
	}
	switch len(vals) {
	case 1:
		return CropMargins{Top: vals[0], Right: vals[0], Bottom: vals[0], Left: vals[0]}, nil
	case 2:
		return CropMargins{Top: vals[0], Right: vals[1], Bottom: vals[0], Left: vals[1]}, nil
	case 3:
		return CropMargins{Top: vals[0], Right: vals[1], Bottom: vals[2], Left: vals[1]}, nil
	default:
		return CropMargins{Top: vals[0], Right: vals[1], Bottom: vals[2], Left: vals[3]}, nil
	}
}
