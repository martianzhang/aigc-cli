package ocr

import (
	"fmt"
	"strings"
	"testing"
)

// ----- helpers -----

// mkOCRLine builds an axis-aligned OCRLine from two opposite corners.
// BBox order is [top-left, top-right, bottom-right, bottom-left], matching
// the detection stage output.
func mkOCRLine(text string, x0, y0, x2, y2 int) OCRLine {
	return OCRLine{
		Text:       text,
		BBox:       [4][2]int{{x0, y0}, {x2, y0}, {x2, y2}, {x0, y2}},
		Confidence: 0.9,
	}
}

// isEnglishWord is a test-side alias for the production dictionary predicate
// Engine.isKnownWord (word_split.go); production code exposes no separate
// isEnglishWord symbol.
func isEnglishWord(e *Engine, word string) bool { return e.isKnownWord(word) }

// testDictWords supplements the curated shortWords list so the test dictionary
// exceeds the 100-entry threshold required by splitEnglishWords, without
// depending on the host system dictionary.
var testDictWords = []string{
	"hello", "world", "quick", "brown", "fox",
	"program", "programming", "testing", "function", "package",
	"hidden", "debt", "these", "companies", "increased", "eightfold",
	"artificial", "fervor", "amid", "company", "cat", "walk", "tall", "fast",
	"open", "close", "input", "output", "model", "engine", "image", "text",
	"split", "word", "words", "english", "dictionary", "segment", "dynamic",
	"values",
}

// newTestEngineWithDict returns a minimal Engine whose dictionary is fully
// deterministic (no host filesystem access, no ONNX models).
func newTestEngineWithDict(t *testing.T, extra ...string) *Engine {
	t.Helper()
	e := &Engine{wsWordSet: make(map[string]bool)}
	for _, w := range strings.Fields(shortWords) {
		e.wsWordSet[w] = true
	}
	for _, w := range testDictWords {
		e.wsWordSet[strings.ToLower(w)] = true
	}
	for _, w := range extra {
		e.wsWordSet[strings.ToLower(w)] = true
	}
	// splitEnglishWords bails out below 100 entries; pad deterministically so
	// the split path is always exercised.
	for i := len(e.wsWordSet); i < 100; i++ {
		e.wsWordSet[fmt.Sprintf("pad%03d", i)] = true
	}
	return e
}

// ----- lineBBox -----

func TestLineBBox(t *testing.T) {
	tests := []struct {
		name           string
		bbox           [4][2]int
		x0, y0, x2, y2 int
	}{
		{
			name: "axis aligned box",
			bbox: [4][2]int{{10, 20}, {30, 20}, {30, 50}, {10, 50}},
			x0:   10, y0: 20, x2: 30, y2: 50,
		},
		{
			name: "unordered corners are reduced to min/max",
			bbox: [4][2]int{{30, 10}, {10, 10}, {10, 50}, {30, 50}},
			x0:   10, y0: 10, x2: 30, y2: 50,
		},
		{
			name: "diamond shape",
			bbox: [4][2]int{{15, 5}, {25, 15}, {15, 25}, {5, 15}},
			x0:   5, y0: 5, x2: 25, y2: 25,
		},
		{
			name: "degenerate point",
			bbox: [4][2]int{{7, 7}, {7, 7}, {7, 7}, {7, 7}},
			x0:   7, y0: 7, x2: 7, y2: 7,
		},
		{
			name: "all zeros",
			bbox: [4][2]int{{0, 0}, {0, 0}, {0, 0}, {0, 0}},
			x0:   0, y0: 0, x2: 0, y2: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x0, y0, x2, y2 := lineBBox(OCRLine{BBox: tc.bbox})
			if x0 != tc.x0 || y0 != tc.y0 || x2 != tc.x2 || y2 != tc.y2 {
				t.Errorf("lineBBox(%v) = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					tc.bbox, x0, y0, x2, y2, tc.x0, tc.y0, tc.x2, tc.y2)
			}
		})
	}
}

// ----- paragraphParse -----

func TestParagraphParse(t *testing.T) {
	tests := []struct {
		name  string
		lines []OCRLine
		want  string
	}{
		{
			name:  "nil input returns empty",
			lines: nil,
			want:  "",
		},
		{
			name:  "empty slice returns empty",
			lines: []OCRLine{},
			want:  "",
		},
		{
			name:  "single line returns its text",
			lines: []OCRLine{mkOCRLine("Hello world", 0, 0, 100, 20)},
			want:  "Hello world",
		},
		{
			name: "aligned lines are grouped into one paragraph",
			lines: []OCRLine{
				mkOCRLine("Hello", 10, 0, 110, 20),
				mkOCRLine("world", 10, 30, 110, 50),
			},
			want: "Hello world",
		},
		{
			name: "three aligned lines stay in one paragraph",
			lines: []OCRLine{
				mkOCRLine("The", 0, 0, 100, 20),
				mkOCRLine("quick", 0, 30, 100, 50),
				mkOCRLine("fox", 0, 60, 100, 80),
			},
			want: "The quick fox",
		},
		{
			name: "different left/right edges force a paragraph break",
			lines: []OCRLine{
				mkOCRLine("First", 0, 0, 100, 20),
				mkOCRLine("Second", 50, 30, 150, 50),
			},
			want: "First\n\nSecond",
		},
		{
			name: "right edge mismatch alone forces a break",
			lines: []OCRLine{
				mkOCRLine("Title", 0, 0, 100, 20),
				mkOCRLine("Much longer line", 0, 30, 400, 50),
			},
			want: "Title\n\nMuch longer line",
		},
		{
			name: "horizontal gap on the same row forces a break",
			lines: []OCRLine{
				mkOCRLine("Alpha", 0, 0, 100, 20),
				mkOCRLine("Beta", 200, 0, 300, 20),
			},
			want: "Alpha\n\nBeta",
		},
		{
			name:  "CJK/Latin spacing is applied to the result",
			lines: []OCRLine{mkOCRLine("AI融资", 0, 0, 100, 20)},
			want:  "AI 融资",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := paragraphParse(tc.lines); got != tc.want {
				t.Errorf("paragraphParse() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ----- isKnownWord (English word predicate) -----

func TestIsKnownWord(t *testing.T) {
	e := &Engine{wsWordSet: map[string]bool{
		"a": true, "i": true, "ai": true,
		"hello": true, "world": true,
		"cat": true, "company": true, "walk": true, "tall": true, "fast": true,
	}}
	tests := []struct {
		name string
		word string
		want bool
	}{
		{"exact dictionary word", "hello", true},
		{"uppercase is case-insensitive", "Hello", true},
		{"all caps", "WORLD", true},
		{"single letter a", "a", true},
		{"single letter i", "i", true},
		{"plural via s", "cats", true},
		{"plural via ies to y", "companies", true},
		{"past tense via ed", "walked", true},
		{"gerund via ing", "walking", true},
		{"comparative via er", "taller", true},
		{"superlative via est", "fastest", true},
		{"possessive via 's", "ai's", true},
		{"empty string", "", false},
		{"unknown word", "zzzz", false},
		{"single letter x", "x", false},
		{"word with trailing punctuation", "hello.", false},
		{"unrelated long token", "qwertyuiop", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isEnglishWord(e, tc.word); got != tc.want {
				t.Errorf("isEnglishWord(%q) = %v, want %v", tc.word, got, tc.want)
			}
		})
	}
}

// ----- splitEnglishWords -----

func TestSplitEnglishWords_deterministicDict(t *testing.T) {
	e := newTestEngineWithDict(t)
	if len(e.wsWordSet) < 100 {
		t.Fatalf("test dictionary has %d entries, need >= 100", len(e.wsWordSet))
	}
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "joined hello world is split",
			input: "helloworld",
			want:  "hello world",
		},
		{
			name:  "longer joined phrase is split",
			input: "thequickbrownfox",
			want:  "the quick brown fox",
		},
		{
			name:  "already spaced text is unchanged",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "single valid word is unchanged",
			input: "programming",
			want:  "programming",
		},
		{
			name:  "token shorter than 10 chars is untouched",
			input: "test",
			want:  "test",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "newlines are preserved",
			input: "helloworld\nhelloworld",
			want:  "hello world\nhello world",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := e.splitEnglishWords(tc.input); got != tc.want {
				t.Errorf("splitEnglishWords(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSplitEnglishWords_smallDictGuard(t *testing.T) {
	// Fewer than 100 words → the guard returns the input unchanged.
	e := &Engine{wsWordSet: map[string]bool{"hello": true, "world": true}}
	if got := e.splitEnglishWords("helloworld"); got != "helloworld" {
		t.Errorf("splitEnglishWords with small dict = %q, want unchanged %q", got, "helloworld")
	}
}
