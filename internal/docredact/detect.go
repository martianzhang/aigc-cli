package docredact

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
)

// detectPage runs detection on the raw line text and on a version with line
// breaks removed. The joined pass recovers IDs/phones/addresses split across a
// visual line break; detected spans are mapped back to the original offsets.
func detectPage(ctx context.Context, pipeline *redactlib.Pipeline, lines []ocrLineBox, text string) ([]redactlib.Span, []redactlib.Span, error) {
	rawDoc := redactlib.NewDocument("page", text)
	raw, err := pipeline.Detect(ctx, rawDoc)
	if err != nil {
		return nil, nil, err
	}
	joined, toOrig := joinLines(lines)
	if joined == "" || joined == text {
		return raw.Spans, raw.Review, nil
	}
	jres, err := pipeline.Detect(ctx, redactlib.NewDocument("joined", joined))
	if err != nil {
		return nil, nil, err
	}
	spans := append([]redactlib.Span(nil), raw.Spans...)
	for _, span := range jres.Spans {
		if mapped, ok := mapJoinedSpan(span, toOrig, text); ok {
			spans = append(spans, mapped)
		}
	}
	review := append([]redactlib.Span(nil), raw.Review...)
	for _, span := range jres.Review {
		if mapped, ok := mapJoinedSpan(span, toOrig, text); ok {
			review = append(review, mapped)
		}
	}
	return redactlib.MergeSpans(spans), dedupeReview(review), nil
}

func joinLines(lines []ocrLineBox) (string, []int) {
	if len(lines) == 0 {
		return "", nil
	}
	var b strings.Builder
	toOrig := make([]int, 0)
	for i, line := range lines {
		if i > 0 && needsJoinSpace(lines[i-1].text, line.text) {
			toOrig = append(toOrig, line.start)
			b.WriteByte(' ')
		}
		for j := 0; j < len(line.text); j++ {
			toOrig = append(toOrig, line.start+j)
			b.WriteByte(line.text[j])
		}
	}
	toOrig = append(toOrig, len(lines[len(lines)-1].text)+lines[len(lines)-1].start)
	return b.String(), toOrig
}

func needsJoinSpace(prev, next string) bool {
	if prev == "" || next == "" {
		return false
	}
	last, _ := utf8.DecodeLastRuneInString(prev)
	first, _ := utf8.DecodeRuneInString(next)
	return unicode.IsLetter(last) && last < utf8.RuneSelf &&
		unicode.IsLetter(first) && first < utf8.RuneSelf
}

func mapJoinedSpan(span redactlib.Span, toOrig []int, text string) (redactlib.Span, bool) {
	if span.Start < 0 || span.End > len(toOrig)-1 || span.Start >= span.End {
		return redactlib.Span{}, false
	}
	start := toOrig[span.Start]
	end := toOrig[span.End]
	if start < 0 || end > len(text) || start >= end {
		return redactlib.Span{}, false
	}
	span.Start, span.End = start, end
	span.Text = text[start:end]
	return span, true
}

func dedupeReview(spans []redactlib.Span) []redactlib.Span {
	seen := map[string]bool{}
	out := spans[:0]
	for _, span := range spans {
		key := fmt.Sprintf("%d:%d:%s", span.Start, span.End, span.Type)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, span)
	}
	return out
}
