package redact

import (
	"fmt"
	"strings"
)

// propagateRules are high-confidence, context-anchored rules whose detected
// value should also be redacted at other exact occurrences in the document.
// Model spans are intentionally excluded: a model can legitimately flag one
// ambiguous occurrence (高峰) without wanting every occurrence replaced.
var propagateRules = map[string]bool{
	"role_name":      true,
	"role_name_verb": true,
	"org_suffix":     true,
	"company_alias":  true,
	"relative_name":  true,
	"name_assign":    true,
	"work_title":     true,
}

func propagateExactSpans(doc *Document, spans []Span) []Span {
	out := append([]Span(nil), spans...)
	seen := make(map[string]bool, len(spans))
	for _, span := range spans {
		seen[spanKey(span)] = true
	}
	for _, span := range spans {
		if !propagateRules[span.Rule] {
			continue
		}
		minRunes := 3
		if span.Type == "ORG" || span.Type == "WORK" {
			minRunes = 4
		}
		if len([]rune(span.Text)) < minRunes {
			continue
		}
		for start := 0; start < len(doc.Text); {
			idx := strings.Index(doc.Text[start:], span.Text)
			if idx < 0 {
				break
			}
			at := start + idx
			candidate := span
			candidate.Start, candidate.End = at, at+len(span.Text)
			candidate.Text = doc.Text[candidate.Start:candidate.End]
			key := spanKey(candidate)
			if !seen[key] {
				seen[key] = true
				out = append(out, candidate)
			}
			start = at + len(span.Text)
		}
	}
	return MergeSpans(out)
}

func spanKey(span Span) string {
	return fmt.Sprintf("%d:%d:%s", span.Start, span.End, span.Type)
}
