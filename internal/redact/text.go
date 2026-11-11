package redact

import (
	"strings"
	"unicode/utf8"
)

// Folded is the width-folded text plus a map from each folded byte offset back
// to the original byte offset. Folding is 1:1 in characters (full-width ASCII
// becomes half-width), so rune offsets are unchanged even though byte lengths
// differ.
type Folded struct {
	Text   string
	ToOrig []int
}

// FoldWidth folds full-width ASCII and the ideographic space to their
// half-width forms while preserving a byte-offset map.
func FoldWidth(s string) Folded {
	var b strings.Builder
	b.Grow(len(s))
	toOrig := make([]int, 0, len(s)+1)
	for i, r := range s {
		f := foldRune(r)
		start := b.Len()
		b.WriteRune(f)
		for j := start; j < b.Len(); j++ {
			toOrig = append(toOrig, i)
		}
	}
	toOrig = append(toOrig, len(s))
	return Folded{Text: b.String(), ToOrig: toOrig}
}

func foldRune(r rune) rune {
	switch {
	case r == '\u3000':
		return ' '
	case r >= '\uFF01' && r <= '\uFF5E':
		return r - 0xFEE0
	default:
		return r
	}
}

// FoldString folds width but discards the offset map (used by validators).
func FoldString(s string) string { return FoldWidth(s).Text }

// IsASCIIDigit reports whether r is an ASCII digit.
func IsASCIIDigit(r rune) bool { return r >= '0' && r <= '9' }

// IsASCIIWord reports whether r is an ASCII letter, digit or underscore.
func IsASCIIWord(r rune) bool {
	return r == '_' || (r >= '0' && r <= '9') ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// BoundaryOK reports whether the match at [start,end) in folded is allowed.
// Only ASCII neighbours block a match; Chinese characters do not, so
// "电话13812345678。" is a valid phone.
func BoundaryOK(folded string, start, end int, kind string) bool {
	if kind == "" || kind == "none" {
		return true
	}
	var before, after rune
	if start > 0 {
		before, _ = utf8.DecodeLastRuneInString(folded[:start])
	}
	if end < len(folded) {
		after, _ = utf8.DecodeRuneInString(folded[end:])
	}
	bad := func(r rune) bool {
		if r == 0 || r > utf8.RuneSelf {
			return false
		}
		if kind == "digit" {
			return IsASCIIDigit(r)
		}
		return IsASCIIWord(r)
	}
	return !bad(before) && !bad(after)
}

// StripSeparators removes characters that are commonly used to group digits:
// spaces, dashes, dots, slashes and middle dots. It is used by validators only;
// the original span text is never modified.
func StripSeparators(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range FoldString(s) {
		switch r {
		case ' ', '\t', '\n', '\r', '-', '\u2010', '\u2011', '\u2012', '\u2013',
			'\u2014', '.', '\u00b7', '/', '\\':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// IsSeparator reports whether a rune should stay visible when KeepSeparators
// is enabled during masking.
func IsSeparator(r rune) bool {
	switch r {
	case ' ', '\t', '-', '\u2010', '\u2011', '\u2012', '\u2013', '\u2014',
		'.', '\u00b7', '/', '\\', ':', '：':
		return true
	default:
		return false
	}
}
