package pdf

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	gopdf "github.com/razvandimescu/gopdf/pdf"
)

func TestFreeMeaningfulChars(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty string", "", 0},
		{"all whitespace", "   \t\n", 0},
		{"all digits", "1234567890", 0},
		{"all punctuation", "!?,.;:()[]{}", 0},
		{"symbols are neither punct nor digit", "!@#$%^&*()", 2},
		{"hello", "hello", 5},
		{"only letters of a1b2c", "a1b2c", 3},
		{"mixed text", "Hello, World!", 10},
		{"letters separated by whitespace", "a b\tc\nd", 4},
		{"CJK text", "你好世界", 4},
		{"CJK with fullwidth punctuation", "你好，世界！", 4},
		{"mixed CJK and digits", "日本語 テスト 123", 6},
		{"Arabic-Indic digits are digits", "٣٤٥", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := meaningfulChars(c.input)
			if got != c.expected {
				t.Errorf("meaningfulChars(%q) = %d, want %d", c.input, got, c.expected)
			}
		})
	}
}

func TestFreeIsScanned(t *testing.T) {
	cases := []struct {
		name     string
		pages    []PageText
		expected bool
	}{
		{
			name:     "empty pages",
			pages:    []PageText{},
			expected: true,
		},
		{
			name:     "nil pages",
			pages:    nil,
			expected: true,
		},
		{
			name:     "all pages below threshold",
			pages:    []PageText{{Page: 1, Text: "abc"}, {Page: 2, Text: strings.Repeat("a", 49)}},
			expected: true,
		},
		{
			name:     "all pages empty text",
			pages:    []PageText{{Page: 1, Text: ""}, {Page: 2, Text: "   "}},
			expected: true,
		},
		{
			name:     "single page at threshold",
			pages:    []PageText{{Page: 1, Text: strings.Repeat("a", 50)}},
			expected: false,
		},
		{
			name:     "one page above threshold among many",
			pages:    []PageText{{Page: 1, Text: "scan"}, {Page: 2, Text: "x"}, {Page: 3, Text: strings.Repeat("word ", 20)}},
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

func TestFreeIsSingleCJK(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected bool
	}{
		{"empty string", "", false},
		{"space only", " ", false},
		{"ascii lower", "a", false},
		{"ascii upper", "A", false},
		{"digit", "1", false},
		{"single hanzi", "中", true},
		{"two hanzi", "你好", false},
		{"hanzi with surrounding spaces", " 中 ", true},
		{"CJK Unified start U+4E00", "\u4E00", true},
		{"CJK Unified end U+9FFF", "\u9FFF", true},
		{"above unified range U+A000", "\uA000", false},
		{"Extension A start U+3400", "\u3400", true},
		{"Extension A end U+4DBF", "\u4DBF", true},
		{"above extension A U+4DC0", "\u4DC0", false},
		{"Compatibility Ideographs start U+F900", "\uF900", true},
		{"Compatibility Ideographs end U+FAFF", "\uFAFF", true},
		{"above compatibility range U+FB00", "\uFB00", false},
		{"fullwidth form start U+FF01", "\uFF01", true},
		{"fullwidth form end U+FF60", "\uFF60", true},
		{"above fullwidth range U+FF61", "\uFF61", false},
		{"emoji", "🙂", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := isSingleCJK(c.input)
			if got != c.expected {
				t.Errorf("isSingleCJK(%q) = %v, want %v", c.input, got, c.expected)
			}
		})
	}
}

// freeSpan builds a TextSpan with X and EndX given explicitly, so spacing
// decisions can be exercised without gopdf's own width estimation.
func freeSpan(x, endX, fontSize float64, text string) gopdf.TextSpan {
	return gopdf.TextSpan{X: x, EndX: endX, FontSize: fontSize, Text: text}
}

func TestFreeJoinSpansWithSpacing(t *testing.T) {
	cases := []struct {
		name  string
		spans []gopdf.TextSpan
		want  string
	}{
		{
			name:  "empty spans",
			spans: []gopdf.TextSpan{},
			want:  "",
		},
		{
			name:  "nil spans",
			spans: nil,
			want:  "",
		},
		{
			name:  "single span returns trimmed text",
			spans: []gopdf.TextSpan{freeSpan(10, 60, 10, "  hello  ")},
			want:  "hello ",
		},
		{
			name: "two spans close together stay joined",
			// gap = 55 - 50 = 5 < 10*2 → no space
			spans: []gopdf.TextSpan{
				freeSpan(0, 50, 10, "foo"),
				freeSpan(55, 105, 10, "bar"),
			},
			want: "foobar ",
		},
		{
			name: "two spans far apart get a space",
			// gap = 200 - 50 = 150 > 20 → space
			spans: []gopdf.TextSpan{
				freeSpan(0, 50, 10, "foo"),
				freeSpan(200, 250, 10, "bar"),
			},
			want: "foo bar ",
		},
		{
			name: "small font keeps kerning gap joined",
			// threshold = max(2, 6*2) = 12; gap = 10 < 12 → no space
			spans: []gopdf.TextSpan{
				freeSpan(0, 30, 6, "abc"),
				freeSpan(40, 70, 6, "def"),
			},
			want: "abcdef ",
		},
		{
			name: "small font spaces a clear gap",
			// threshold = max(2, 6*2) = 12; gap = 15 > 12 → space
			spans: []gopdf.TextSpan{
				freeSpan(0, 30, 6, "abc"),
				freeSpan(45, 75, 6, "def"),
			},
			want: "abc def ",
		},
		{
			name: "endX equals X falls back to text length estimate",
			// estimated end of "ab" = 100 + 2*10*0.5 = 110; gap = 120-110 = 10 < 20 → joined
			spans: []gopdf.TextSpan{
				freeSpan(100, 100, 10, "ab"),
				freeSpan(120, 120, 10, "cd"),
			},
			want: "abcd ",
		},
		{
			name: "endX below X falls back to text length estimate",
			// estimated end of "ab" = 100 + 2*10*0.5 = 110; gap = 200-110 = 90 > 20 → space
			spans: []gopdf.TextSpan{
				freeSpan(100, 60, 10, "ab"),
				freeSpan(200, 250, 10, "cd"),
			},
			want: "ab cd ",
		},
		{
			name: "endX zero uses fallback for CJK text",
			// estimated end of "中文" = 100 + 2*20*0.5 = 120; gap = 200-120 = 80 > 40 → space
			spans: []gopdf.TextSpan{
				freeSpan(100, 0, 20, "中文"),
				freeSpan(200, 260, 20, "ef"),
			},
			want: "中文 ef ",
		},
		{
			name:  "whitespace-only spans are skipped",
			spans: []gopdf.TextSpan{freeSpan(0, 10, 10, "   "), freeSpan(20, 40, 10, "hi")},
			want:  "hi ",
		},
		{
			name:  "all spans whitespace returns empty",
			spans: []gopdf.TextSpan{freeSpan(0, 10, 10, "   "), freeSpan(20, 30, 10, "\t\n")},
			want:  "",
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

func TestFreeExtractPageNum(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected int
	}{
		{"path with page prefix", "path/page-3.png", 3},
		{"bare page prefix", "page-42.png", 42},
		{"zero padded frame", "output/frame-001.png", 1},
		{"no page number", "nopage.png", 0},
		{"absolute path", "/tmp/out/page-7.PNG", 7},
		{"non numeric suffix", "page-abc.png", 0},
		{"no extension", "page-9", 9},
		{"empty string", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractPageNum(c.input)
			if got != c.expected {
				t.Errorf("extractPageNum(%q) = %d, want %d", c.input, got, c.expected)
			}
		})
	}
}

func TestFreeAbs(t *testing.T) {
	cases := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"positive", 42, 42},
		{"negative", -42, 42},
		{"positive fraction", 3.5, 3.5},
		{"negative fraction", -3.5, 3.5},
		{"zero", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := abs(c.input)
			if got != c.expected {
				t.Errorf("abs(%v) = %v, want %v", c.input, got, c.expected)
			}
		})
	}
}

func TestFreeConfigBinDir(t *testing.T) {
	t.Run("home set", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		got := configBinDir()
		want := filepath.Join(home, ".config", "aigc-cli", "bin")
		if got != want {
			t.Errorf("configBinDir() = %q, want %q", got, want)
		}
	})

	t.Run("home unset", func(t *testing.T) {
		t.Setenv("HOME", "")

		if got := configBinDir(); got != "" {
			t.Errorf("configBinDir() = %q, want empty", got)
		}
	})
}

// fakeMutoolName mirrors the executable name exec.LookPath resolves for the
// "mutool" command on the current platform.
func fakeMutoolName() string {
	if runtime.GOOS == "windows" {
		return "mutool.exe"
	}
	return "mutool"
}

func TestFreeFindMutool(t *testing.T) {
	t.Run("found in config bin dir", func(t *testing.T) {
		home := t.TempDir()
		binDir := filepath.Join(home, ".config", "aigc-cli", "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) failed: %v", binDir, err)
		}
		candidate := filepath.Join(binDir, fakeMutoolName())
		if err := os.WriteFile(candidate, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", candidate, err)
		}

		t.Setenv("HOME", home)
		t.Setenv("PATH", t.TempDir()) // no mutool on PATH

		got, err := findMutool()
		if err != nil {
			t.Fatalf("findMutool() error = %v, want nil", err)
		}
		if got != candidate {
			t.Errorf("findMutool() = %q, want %q", got, candidate)
		}
	})

	t.Run("falls back to PATH lookup", func(t *testing.T) {
		home := t.TempDir() // config bin dir has no mutool
		pathDir := t.TempDir()
		fake := filepath.Join(pathDir, fakeMutoolName())
		if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", fake, err)
		}

		t.Setenv("HOME", home)
		t.Setenv("PATH", pathDir)

		got, err := findMutool()
		if err != nil {
			t.Fatalf("findMutool() error = %v, want nil", err)
		}
		if got != fake {
			t.Errorf("findMutool() = %q, want %q", got, fake)
		}
	})

	t.Run("not found returns error", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("PATH", t.TempDir())

		got, err := findMutool()
		if err == nil {
			t.Fatalf("findMutool() error = nil, want error")
		}
		if got != "" {
			t.Errorf("findMutool() = %q, want empty string", got)
		}
	})
}
