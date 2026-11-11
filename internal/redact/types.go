// Package redact implements offline PII detection, masking and reversible
// redaction for plain text. It has no network dependency: rules and checksum
// validators are pure Go, and optional model engines are injected by the CLI.
package redact

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"unicode/utf8"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// Source identifies where a span came from.
type Source string

const (
	SourceRule   Source = "rule"
	SourceModel  Source = "model"
	SourceDeny   Source = "deny"
	SourceManual Source = "manual"
)

// Span is one detected sensitive range. Start/End are byte offsets into the
// original text; JSON output converts them to rune offsets.
type Span struct {
	Start    int
	End      int
	Type     string
	Text     string
	Rule     string
	Source   Source
	Score    float64
	Priority int
	// Style and Mask carry a per-rule override; empty/nil means use the
	// global style and the per-type mask policy.
	Style string
	Mask  *types.RedactMaskPolicy
}

// Result is the output of a detection pass. Review holds occurrences a model
// found but could not anchor confidently; they are never applied automatically.
type Result struct {
	Spans  []Span
	Review []Span
}

// Document wraps the input text with a byte<->rune index.
type Document struct {
	ID         string
	Text       string
	runeStarts []int
}

// NewDocument builds a document and its rune index.
func NewDocument(id, text string) *Document {
	starts := make([]int, 0, utf8.RuneCountInString(text)+1)
	for i := 0; i < len(text); {
		starts = append(starts, i)
		_, size := utf8.DecodeRuneInString(text[i:])
		if size <= 0 {
			size = 1
		}
		i += size
	}
	starts = append(starts, len(text))
	return &Document{ID: id, Text: text, runeStarts: starts}
}

// RuneOffset converts a byte offset to a rune (character) offset.
func (d *Document) RuneOffset(byteOff int) int {
	if byteOff <= 0 {
		return 0
	}
	if byteOff >= len(d.Text) {
		return len(d.runeStarts) - 1
	}
	i := sort.SearchInts(d.runeStarts, byteOff)
	if i < len(d.runeStarts) && d.runeStarts[i] == byteOff {
		return i
	}
	return i - 1
}

// ByteOffset converts a rune offset to a byte offset.
func (d *Document) ByteOffset(runeOff int) int {
	if runeOff <= 0 {
		return 0
	}
	if runeOff >= len(d.runeStarts)-1 {
		return len(d.Text)
	}
	return d.runeStarts[runeOff]
}

// SHA256 returns the hex digest of the document text.
func (d *Document) SHA256() string {
	sum := sha256.Sum256([]byte(d.Text))
	return hex.EncodeToString(sum[:])
}

// Entity is one canonical value and its placeholder.
type Entity struct {
	Type        string   `json:"type"`
	Value       string   `json:"value"`
	Placeholder string   `json:"placeholder"`
	Aliases     []string `json:"aliases,omitempty"`
	Count       int      `json:"count"`
}

// Occurrence records one replaced range in the output document. Start/End are
// rune offsets in the output (redacted) text.
type Occurrence struct {
	Doc         string `json:"doc"`
	Start       int    `json:"start"`
	End         int    `json:"end"`
	Placeholder string `json:"placeholder"`
	Masked      string `json:"masked,omitempty"`
}

// Mapping is the reversible dictionary written next to a redacted document.
type Mapping struct {
	Version      int          `json:"version"`
	CreatedAt    string       `json:"created_at"`
	Doc          string       `json:"doc,omitempty"`
	Style        string       `json:"style"`
	Placeholder  string       `json:"placeholder"`
	SourceSHA256 string       `json:"source_sha256,omitempty"`
	OutputSHA256 string       `json:"output_sha256,omitempty"`
	Entities     []Entity     `json:"entities"`
	Occurrences  []Occurrence `json:"occurrences"`
}

// Residual is one leftover sensitive value found by Check.
type Residual struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Start int    `json:"start"` // rune offset
	End   int    `json:"end"`   // rune offset
}

// lenRunes is a small helper used across mask/mapping code.
func lenRunes(s string) int { return utf8.RuneCountInString(s) }
