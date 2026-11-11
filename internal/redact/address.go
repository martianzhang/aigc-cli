package redact

import "strings"

// Address models often stop at a road name or house number and leave the
// building/room suffix (3幢802室, 5幢1201室) outside the span. Extending the
// span improves redaction without guessing: only extend when the following
// text (up to punctuation) contains an address marker and no role word.
var (
	addressBoundaries = map[rune]bool{
		'，': true, ',': true, '。': true, '；': true, ';': true,
		'！': true, '!': true, '？': true, '?': true, '、': true,
		'\n': true, '\r': true, '\t': true,
	}
	addressMarkers = []string{
		"号", "幢", "栋", "座", "单元", "室", "楼", "层", "户", "弄", "巷",
		"花园", "小区", "大厦", "广场", "公寓", "别墅", "写字楼", "中心",
		"新村", "家园", "苑", "园", "城",
	}
	addressStopWords = []string{
		"电话", "联系电话", "手机", "统一社会信用代码", "法定代表人",
		"身份证", "邮箱", "邮编", "被告", "原告", "住址", "住所地",
	}
)

func extendAddressSpans(doc *Document, spans []Span) []Span {
	for i := range spans {
		if strings.EqualFold(spans[i].Type, "ADDRESS") {
			spans[i] = extendAddressSpan(doc, spans[i])
		}
	}
	return spans
}

func extendAddressSpan(doc *Document, span Span) Span {
	if span.End <= span.Start || span.End >= len(doc.Text) {
		return span
	}
	rest := doc.Text[span.End:]
	limit := len(rest)
	runes := 0
	for i, r := range rest {
		if addressBoundaries[r] || runes >= 60 {
			limit = i
			break
		}
		runes++
	}
	if limit <= 0 {
		return span
	}
	tail := rest[:limit]
	if strings.TrimSpace(tail) == "" || containsAny(tail, addressStopWords) || !containsAny(tail, addressMarkers) {
		return span
	}
	span.End += limit
	span.Text = doc.Text[span.Start:span.End]
	return span
}

func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
