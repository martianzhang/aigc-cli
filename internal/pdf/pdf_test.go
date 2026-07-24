package pdf

import "testing"

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
		{"你好abc", 5},
		{"Hello, World!", 10},
		{"a b c", 3},
		{"测试 text 123", 6},
	}
	for _, c := range cases {
		got := meaningfulChars(c.input)
		if got != c.expected {
			t.Errorf("meaningfulChars(%q) = %d, want %d", c.input, got, c.expected)
		}
	}
}

func TestCleanupPDFText(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "vertical CJK merge",
			input:    "一\n二\n三\n四\n五",
			expected: "一二三四五",
		},
		{
			name:     "no vertical merge less than 3",
			input:    "一\n二",
			expected: "一\n二",
		},
		{
			name:     "blank line collapse",
			input:    "line1\n\n\n\n\n\nline2",
			expected: "line1\n\n\nline2",
		},
		{
			name:     "space collapsing",
			input:    "line1\na   b     c\nline2",
			expected: "line1\na b c\nline2",
		},
		{
			name:     "no-op simple ASCII",
			input:    "Hello world\nThis is a test.",
			expected: "Hello world\nThis is a test.",
		},
		{
			name:     "short input unchanged",
			input:    "ab",
			expected: "ab",
		},
		{
			name:     "mixed vertical CJK and normal text",
			input:    "Header\n一\n二\n三\n四\n五\nFooter",
			expected: "Header\n一二三四五\nFooter",
		},
		{
			name:     "tab collapsing",
			input:    "line1\na\t\tb\t\t\tc\nline2",
			expected: "line1\na b c\nline2",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := cleanupPDFText(c.input)
			if got != c.expected {
				t.Errorf("cleanupPDFText(%q) = %q, want %q", c.input, got, c.expected)
			}
		})
	}
}

func TestIsSingleCJK(t *testing.T) {
	cases := []struct {
		input    string
		expected bool
	}{
		{"", false},
		{"a", false},
		{"1", false},
		{" ", false},
		{"你", true},
		{"好", true},
		{" 你 ", true},
		{"你好", false},
		{"你a", false},
		{"\u4E00", true},  // CJK start
		{"\u9FFF", true},  // CJK end
		{"\u3400", true},  // CJK Ext A start
		{"\u4DBF", true},  // CJK Ext A end
		{"\uF900", true},  // CJK compatibility start
		{"\uFAFF", true},  // CJK compatibility end
		{"\uFF01", true},  // Fullwidth forms start
		{"\uFF60", true},  // Fullwidth forms end
		{"\u3040", false}, // Hiragana
		{"\u30A0", false}, // Katakana
		{"\uAC00", false}, // Hangul
	}
	for _, c := range cases {
		got := isSingleCJK(c.input)
		if got != c.expected {
			t.Errorf("isSingleCJK(%q) = %v, want %v", c.input, got, c.expected)
		}
	}
}

func TestCollapseSpaces(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"a", "a"},
		{"a  b   c", "a b c"},
		{"a\t\tb\t\t\tc", "a b c"},
		{"你好  世界", "你好 世界"},
		{"already normalized", "already normalized"},
		{"  leading", " leading"},
		{"trailing  ", "trailing "},
		{"a\tb\tc", "a b c"},
	}
	for _, c := range cases {
		got := collapseSpaces(c.input)
		if got != c.expected {
			t.Errorf("collapseSpaces(%q) = %q, want %q", c.input, got, c.expected)
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
