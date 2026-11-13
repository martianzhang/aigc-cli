package mdrender

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

// wrapToken is one escape sequence or one grapheme cluster of a line.
type wrapToken struct {
	seq string
	w   int
}

// WrapANSI wraps s to width display columns, preferring spaces as break points
// so English words stay intact; runs without spaces (CJK, long tokens) are
// hard-wrapped. Escape sequences are preserved and the active SGR style is
// re-applied after every break, so colours survive the wrap.
//
// It also returns one separator per line after the first, describing how to
// rebuild the original text: "\n" for a real line break, " " for a wrap at a
// space and "" for a hard wrap. The chat TUI's copy-on-select uses them so a
// wrapped line does not turn into an artificial newline.
func WrapANSI(s string, width int) (string, []string) {
	if width < 1 {
		_, seps := hardLineBreaks(s)
		return s, seps[1:]
	}

	// Split the source into hard lines of tokens (escape sequences included).
	var (
		srcLines [][]wrapToken
		cur      []wrapToken
		state    byte
		parser   = xansi.NewParser()
	)
	for i := 0; i < len(s); {
		seq, w, n, newState := xansi.DecodeSequence(s[i:], state, parser)
		if n <= 0 {
			seq, w, n = s[i:i+1], 1, 1
		}
		i += n
		state = newState
		if seq == "\n" {
			srcLines = append(srcLines, cur)
			cur = nil
			continue
		}
		cur = append(cur, wrapToken{seq: seq, w: w})
	}
	srcLines = append(srcLines, cur)

	var (
		out    strings.Builder
		joins  []string
		active string // logical SGR state, re-applied at the start of each row
		first  = true
	)
	for _, toks := range srcLines {
		visual, lineJoins := splitVisualLine(toks, width)
		for vi, row := range visual {
			if !first {
				out.WriteByte('\n')
				if vi == 0 {
					joins = append(joins, "\n")
				} else {
					joins = append(joins, lineJoins[vi-1])
				}
			}
			first = false
			if active != "" {
				out.WriteString(active)
			}
			for _, t := range row {
				out.WriteString(t.seq)
				if !isSGR(t.seq) {
					continue
				}
				if t.seq == "\x1b[0m" || t.seq == "\x1b[m" {
					active = ""
				} else {
					active += t.seq
				}
			}
			if active != "" {
				// Close the row so the style cannot leak past it; it is
				// re-applied above on the next row.
				out.WriteString("\x1b[0m")
			}
		}
	}
	return out.String(), joins
}

// splitVisualLine greedily wraps one hard line into visual rows, preferring the
// last space on the row as the break point. It returns the rows and, for every
// row but the last, the separator that rejoins it to the next one.
func splitVisualLine(toks []wrapToken, width int) ([][]wrapToken, []string) {
	if len(toks) == 0 {
		return [][]wrapToken{{}}, nil
	}
	var (
		visual [][]wrapToken
		joins  []string
	)
	for len(toks) > 0 {
		w, cut, lastSpace := 0, 0, -1
		for i, t := range toks {
			if t.w > 0 {
				if w+t.w > width && w > 0 {
					break
				}
				w += t.w
			}
			cut = i + 1
			if t.seq == " " {
				lastSpace = i
			}
		}
		if cut == 0 {
			cut = 1
		}
		join := ""
		if cut < len(toks) && lastSpace > 0 {
			cut = lastSpace
			join = " "
		}
		visual = append(visual, toks[:cut])
		toks = toks[cut:]
		if join == " " {
			toks = toks[1:] // the space moved into the join
		}
		joins = append(joins, join)
	}
	return visual, joins[:len(joins)-1]
}

// isSGR reports whether seq is an SGR (colour/attribute) escape sequence.
func isSGR(seq string) bool {
	return strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m")
}

// hardLineBreaks marks every line of s as separated by a real newline.
func hardLineBreaks(s string) (string, []string) {
	seps := make([]string, strings.Count(s, "\n")+1)
	for i := 1; i < len(seps); i++ {
		seps[i] = "\n"
	}
	return s, seps
}
