package pdf

import (
	"testing"

	gopdf "github.com/razvandimescu/gopdf/pdf"
)

func TestMeaningfulChars(t *testing.T) {
	cases := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"   ", 0},
		{"123", 0},
		{"!@#", 0},
		{"123 !@#", 0},
		{"abc", 3},
		{"Hello, World!", 10},
		{"a b c", 3},
	}
	for _, c := range cases {
		got := meaningfulChars(c.input)
		if got != c.expected {
			t.Errorf("meaningfulChars(%q) = %d, want %d", c.input, got, c.expected)
		}
	}
}

func TestExtractPageNum(t *testing.T) {
	cases := []struct {
		input    string
		expected int
	}{
		{"page-3.png", 3},
		{"page-10.jpg", 10},
		{"notpage.png", 0},
		{"page-abc.jpg", 0},
		{"/path/to/page-42.PNG", 42},
		{"page-0.png", 0},
		{"page-007.bmp", 7},
		{"page-", 0},
		{"", 0},
	}
	for _, c := range cases {
		got := extractPageNum(c.input)
		if got != c.expected {
			t.Errorf("extractPageNum(%q) = %d, want %d", c.input, got, c.expected)
		}
	}
}

func TestIsScanned(t *testing.T) {
	cases := []struct {
		name     string
		pages    []PageText
		expected bool
	}{
		{
			name:     "nil pages is scanned",
			pages:    nil,
			expected: true,
		},
		{
			name:     "empty pages is scanned",
			pages:    []PageText{},
			expected: true,
		},
		{
			name:     "empty text is scanned",
			pages:    []PageText{{Page: 1, Text: ""}},
			expected: true,
		},
		{
			name:     "few chars is scanned",
			pages:    []PageText{{Page: 1, Text: "abc"}},
			expected: true,
		},
		{
			name:     "enough chars not scanned",
			pages:    []PageText{{Page: 1, Text: "Hello this is a test document with enough characters to pass the scanned threshold"}},
			expected: false,
		},
		{
			name:     "one page not scanned skips scanned",
			pages:    []PageText{{Page: 1, Text: "scan"}, {Page: 2, Text: "Hello this text has enough characters to pass the scanned threshold check"}},
			expected: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := IsScanned(c.pages)
			if got != c.expected {
				t.Errorf("IsScanned(_) = %v, want %v", got, c.expected)
			}
		})
	}
}

// span constructs a TextSpan with X and EndX explicitly set, where EndX
// is computed as X + (runeCount * fontSize * 0.5) to mimic the gopdf
// default for proportional fonts.
func span(x, fontSize float64, text string) gopdf.TextSpan {
	endX := x + float64(len([]rune(text)))*fontSize*0.5
	return gopdf.TextSpan{X: x, EndX: endX, FontSize: fontSize, Text: text}
}

func TestJoinSpansWithSpacing(t *testing.T) {
	cases := []struct {
		name  string
		spans []gopdf.TextSpan
		want  string
	}{
		{
			name:  "empty input",
			spans: nil,
			want:  "",
		},
		{
			name:  "single span",
			spans: []gopdf.TextSpan{span(100, 11.8, "hello")},
			want:  "hello ",
		},
		{
			name: "adjacent spans with no visible gap",
			// gap between "1" end (~293) and "199.6" start (300) = 7px
			// 7 < 11.8*2=23.6, so no space.
			spans: []gopdf.TextSpan{
				span(287, 11.8, "1"),
				{X: 300, EndX: 370, FontSize: 11.8, Text: "199.6"},
			},
			want: "1199.6 ",
		},
		{
			name: "invoice numeric cells separated by 40-70px gaps get spaces",
			// Real invoice line: "1 199.6 199.60"
			// - "1" at X=287, end ~293
			// - "199.6" at X=340, end ~370 (gap 47 > 23.6 → space)
			// - "199.60" at X=407, end ~437 (gap 37 > 23.6 → space)
			spans: []gopdf.TextSpan{
				span(287, 11.8, "1"),
				span(340, 11.8, "199.6"),
				span(407, 11.8, "199.60"),
			},
			want: "1 199.6 199.60 ",
		},
		{
			name: "two columns with large gap get one space",
			// "CompanyA" column ends near 300, "ChinaB" column starts at 500
			// gap = 200px > 23.6 → 1 space.
			spans: []gopdf.TextSpan{
				{X: 100, EndX: 200, FontSize: 12, Text: "CompanyA"},
				{X: 500, EndX: 600, FontSize: 12, Text: "ChinaB"},
			},
			want: "CompanyA ChinaB ",
		},
		{
			name: "small font: kerning gap stays joined",
			// fontSize 6 → threshold 12. gap of 5px < 12, no space.
			spans: []gopdf.TextSpan{
				{X: 0, EndX: 30, FontSize: 6, Text: "abc"},
				{X: 35, EndX: 65, FontSize: 6, Text: "def"},
			},
			want: "abcdef ",
		},
		{
			name: "small font: clear gap gets a space",
			// fontSize 6 → threshold 12. gap of 20px > 12, one space.
			spans: []gopdf.TextSpan{
				{X: 0, EndX: 30, FontSize: 6, Text: "abc"},
				{X: 50, EndX: 80, FontSize: 6, Text: "def"},
			},
			want: "abc def ",
		},
		{
			name: "fallback to estimated width when EndX equals X",
			// EndX == X means the gopdf library didn't track end position.
			// Helper should fall back to fontSize*0.5 per char estimation.
			spans: []gopdf.TextSpan{
				{X: 100, EndX: 100, FontSize: 10, Text: "ab"},
				{X: 120, EndX: 120, FontSize: 10, Text: "cd"},
			},
			// estimated end of "ab" = 100 + 2*5 = 110; gap = 10 < 20 → no space
			want: "abcd ",
		},
		{
			name: "fallback: large gap still gets space",
			spans: []gopdf.TextSpan{
				{X: 100, EndX: 100, FontSize: 10, Text: "ab"},
				{X: 200, EndX: 200, FontSize: 10, Text: "cd"},
			},
			// estimated end of "ab" = 110; gap = 90 > 20 → space
			want: "ab cd ",
		},
		{
			name: "mix of CJK and ASCII text",
			// Chinese phrase and English label on same line.
			spans: []gopdf.TextSpan{
				{X: 50, EndX: 200, FontSize: 12, Text: "购买方信息"},
				{X: 300, EndX: 380, FontSize: 12, Text: "Buyer"},
			},
			want: "购买方信息 Buyer ",
		},
		{
			name: "three spans with mixed gaps",
			spans: []gopdf.TextSpan{
				{X: 0, EndX: 50, FontSize: 10, Text: "项目"},
				{X: 100, EndX: 150, FontSize: 10, Text: "数量"},
				{X: 400, EndX: 450, FontSize: 10, Text: "金额"},
			},
			// 0→100 gap=50 > 20 → space; 150→400 gap=250 > 20 → space
			want: "项目 数量 金额 ",
		},
		{
			name: "very small font has 2px minimum threshold",
			// fontSize 0.5 → threshold = max(2, 1) = 2.
			spans: []gopdf.TextSpan{
				{X: 0, EndX: 5, FontSize: 0.5, Text: "a"},
				{X: 6, EndX: 11, FontSize: 0.5, Text: "b"},
			},
			// gap = 1 < 2 → no space
			want: "ab ",
		},
		{
			name: "touching spans (gap == 0) stay joined",
			spans: []gopdf.TextSpan{
				{X: 100, EndX: 110, FontSize: 10, Text: "ab"},
				{X: 110, EndX: 120, FontSize: 10, Text: "cd"},
			},
			want: "abcd ",
		},
		{
			name: "overlapping spans stay joined",
			spans: []gopdf.TextSpan{
				{X: 100, EndX: 120, FontSize: 10, Text: "ab"},
				{X: 115, EndX: 135, FontSize: 10, Text: "cd"},
			},
			// gap = 115 - 120 = -5 < 0 → no space
			want: "abcd ",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := joinSpansWithSpacing(c.spans)
			if got != c.want {
				t.Errorf("joinSpansWithSpacing() = %q, want %q", got, c.want)
			}
		})
	}
}
