package ocr

import "testing"

// ----- expandBox -----

func TestExpandBox(t *testing.T) {
	tests := []struct {
		name       string
		box        [4][2]int
		imgW, imgH int
		ratio      float64
		want       [4][2]int
	}{
		{
			name: "expand 10 percent within bounds",
			// 100x50 box at (10,10): padX=10, padY=5
			box:  [4][2]int{{10, 10}, {110, 10}, {110, 60}, {10, 60}},
			imgW: 200, imgH: 100,
			ratio: 0.1,
			want:  [4][2]int{{0, 5}, {120, 5}, {120, 65}, {0, 65}},
		},
		{
			name: "clamped to left and top image edges",
			// padX=50, padY=25 → min side clamps to 0
			box:  [4][2]int{{0, 0}, {100, 0}, {100, 50}, {0, 50}},
			imgW: 120, imgH: 80,
			ratio: 0.5,
			want:  [4][2]int{{0, 0}, {120, 0}, {120, 75}, {0, 75}},
		},
		{
			name: "clamped to right and bottom image edges",
			// 10x10 box at (90,40): pad=10 → max side clamps to imgW/imgH
			box:  [4][2]int{{90, 40}, {100, 40}, {100, 50}, {90, 50}},
			imgW: 100, imgH: 50,
			ratio: 1.0,
			want:  [4][2]int{{80, 30}, {100, 30}, {100, 50}, {80, 50}},
		},
		{
			name: "unordered corners are normalized with zero ratio",
			box:  [4][2]int{{30, 10}, {10, 10}, {10, 50}, {30, 50}},
			imgW: 200, imgH: 200,
			ratio: 0,
			want:  [4][2]int{{10, 10}, {30, 10}, {30, 50}, {10, 50}},
		},
		{
			name: "degenerate point box gets no padding",
			box:  [4][2]int{{50, 50}, {50, 50}, {50, 50}, {50, 50}},
			imgW: 100, imgH: 100,
			ratio: 0.5,
			want:  [4][2]int{{50, 50}, {50, 50}, {50, 50}, {50, 50}},
		},
		{
			name: "box partially outside image clamps to bounds",
			// 30x30 box starting at (-20,-20): padX=padY=3
			box:  [4][2]int{{-20, -20}, {10, -20}, {10, 10}, {-20, 10}},
			imgW: 100, imgH: 100,
			ratio: 0.1,
			want:  [4][2]int{{0, 0}, {13, 0}, {13, 13}, {0, 13}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := expandBox(tc.box, tc.imgW, tc.imgH, tc.ratio)
			if got != tc.want {
				t.Errorf("expandBox(%v, %d, %d, %g) = %v, want %v",
					tc.box, tc.imgW, tc.imgH, tc.ratio, got, tc.want)
			}
		})
	}
}

// ----- isCJK -----

func TestIsCJK(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		// CJK Unified Ideographs (4E00-9FFF)
		{"U+4E00 first unified ideograph", 0x4E00, true},
		{"U+9FFF last unified ideograph", 0x9FFF, true},
		{"common 中", '中', true},
		{"common 融", '融', true},
		// Hiragana + Katakana (3040-30FF)
		{"U+3042 hiragana a", 0x3042, true},
		{"U+30A2 katakana a", 0x30A2, true},
		{"U+30FF last katakana", 0x30FF, true},
		// Hangul (AC00-D7AF)
		{"U+AC00 first hangul syllable", 0xAC00, true},
		{"U+D7AF last hangul syllable", 0xD7AF, true},
		// CJK Symbols and Punctuation (3000-303F)
		{"U+3000 ideographic space", 0x3000, true},
		{"U+3002 ideographic full stop", 0x3002, true},
		{"U+3010 left black lenticular bracket", 0x3010, true},
		// Fullwidth forms (FF00-FFEF)
		{"U+FF01 fullwidth exclamation", 0xFF01, true},
		{"U+FF08 fullwidth left paren", 0xFF08, true},
		{"U+FF60 fullwidth right white paren", 0xFF60, true},
		{"U+FFEF fullwidth last", 0xFFEF, true},
		// Non-CJK
		{"ASCII letter A", 'A', false},
		{"ASCII digit 0", '0', false},
		{"ASCII space", ' ', false},
		{"ASCII punctuation", '.', false},
		{"U+00E9 latin e acute", 0x00E9, false},
		{"U+2FFF below CJK symbols", 0x2FFF, false},
		{"U+4DFF below unified ideographs", 0x4DFF, false},
		{"U+A000 above unified ideographs", 0xA000, false},
		{"U+3100 below hiragana block", 0x3100, false},
		{"U+D7B0 above hangul block", 0xD7B0, false},
		{"U+FFF0 above fullwidth forms", 0xFFF0, false},
		// Documented gap: the implementation does NOT cover these CJK blocks.
		{"CJK Extension A U+3400 not covered", 0x3400, false},
		{"U+4DBF end of CJK Extension A not covered", 0x4DBF, false},
		{"CJK compatibility ideograph U+F900 not covered", 0xF900, false},
		{"CJK compatibility ideograph U+FAFF not covered", 0xFAFF, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCJK(tc.r); got != tc.want {
				t.Errorf("isCJK(%U) = %v, want %v", tc.r, got, tc.want)
			}
		})
	}
}

// ----- isEnglishLine -----

func TestIsEnglishLine(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{"pure ascii", "hello world", true},
		{"ascii with punctuation", "hello, world!", true},
		{"digits count as ascii", "12345", true},
		{"ascii plus filler 极 skipped", "极hello极", true},
		{"80 percent ascii", "abcdefgh你好吗", true},                 // 8/11 ≈ 0.727
		{"exactly 70 percent is not enough", "abcdefg你好吗", false}, // 7/10 == 0.7
		{"under 70 percent ascii", "abcd你好吗", false},              // 4/7 ≈ 0.571
		{"cjk only", "你好世界", false},
		{"hiragana only is not counted", "こんにちは", false},
		{"empty string", "", false},
		{"spaces only", "   ", false},
		{"filler only", "极极极", false},
		{"fullwidth punctuation is ignored", "hello！", true}, // ！ is neither ascii nor 4E00-9FFF
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line := OCRLine{Text: tc.text}
			if got := isEnglishLine(line); got != tc.want {
				t.Errorf("isEnglishLine(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// ----- fixEnglishOCRErrors -----

func TestFixEnglishOCRErrors(t *testing.T) {
	e := &Engine{} // nil Corrections map must be tolerated
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"digit 1 between letters becomes l", "Test1ng", "Testlng"},
		{"digit 1 between letters A1B", "A1B", "AlB"},
		{"trailing 1 after letter becomes l", "chapter1", "chapterl"},
		{"digit 0 between letters becomes O", "A0B", "AOB"},
		{"trailing 0 after letter becomes o", "chapter0", "chaptero"},
		{"all digits unchanged", "1234", "1234"},
		{"10 unchanged", "10", "10"},
		{"single 1 unchanged", "1", "1"},
		{"leading 1 unchanged", "1abc", "1abc"},
		{"trailing 1 before space unchanged", "abc1 ", "abc1 "},
		{"1 surrounded by non-letters unchanged", "1.0", "1.0"},
		{"cjk neighbours are not letters", "版本1.0", "版本1.0"},
		{"empty string", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := e.fixEnglishOCRErrors(tc.in); got != tc.want {
				t.Errorf("fixEnglishOCRErrors(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFixEnglishOCRErrors_AppliesCorrections(t *testing.T) {
	e := &Engine{Corrections: map[string]string{"mecp": "mcp", "jana": "jina"}}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"correction table applied", "the mecp server", "the mcp server"},
		{"multiple corrections applied", "mecp and jana", "mcp and jina"},
		{"character fix then correction", "mecp1", "mcpl"},
		{"no match left untouched", "nothing to fix", "nothing to fix"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := e.fixEnglishOCRErrors(tc.in); got != tc.want {
				t.Errorf("fixEnglishOCRErrors(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ----- cjkLatinSpacing -----

func TestCJKLatinSpacing(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"latin before cjk", "AI融资", "AI 融资"},
		{"cjk before latin", "你好world", "你好 world"},
		{"already spaced stays single spaced", "AI 融资", "AI 融资"},
		{"digit between cjk", "第3章", "第 3 章"},
		{"mixed both directions", "中文English混合", "中文 English 混合"},
		{"pure cjk unchanged", "纯中文", "纯中文"},
		{"pure latin unchanged", "pureEnglish", "pureEnglish"},
		{"latin with space unchanged", "hello world", "hello world"},
		{"fullwidth parens suppress spaces", "（AI）", "（AI）"},
		{"black lenticular brackets", "《AI》", "《AI》"},
		{"corner brackets", "「AI」", "「AI」"},
		{"lenticular brackets", "【AI】", "【AI】"},
		{"ascii parens do not suppress", "中文(AI)混合", "中文(AI)混合"},
		{"space after fullwidth close paren before latin", "中文（AI）English", "中文（AI） English"},
		{"empty string", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cjkLatinSpacing(tc.in); got != tc.want {
				t.Errorf("cjkLatinSpacing(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ----- isCJKOpenParen / isCJKCloseParen -----

func TestIsCJKOpenParen(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{"U+FF08 fullwidth left paren", 0xFF08, true},
		{"U+300A left double angle bracket", 0x300A, true},
		{"U+300C left corner bracket", 0x300C, true},
		{"U+3010 left black lenticular bracket", 0x3010, true},
		{"ascii left paren", '(', false},
		{"matching close paren", 0xFF09, false},
		{"matching close angle", 0x300B, false},
		{"letter", 'a', false},
		{"cjk ideograph", '中', false},
		{"space", ' ', false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCJKOpenParen(tc.r); got != tc.want {
				t.Errorf("isCJKOpenParen(%U) = %v, want %v", tc.r, got, tc.want)
			}
		})
	}
}

func TestIsCJKCloseParen(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{"U+FF09 fullwidth right paren", 0xFF09, true},
		{"U+300B right double angle bracket", 0x300B, true},
		{"U+300D right corner bracket", 0x300D, true},
		{"U+3011 right black lenticular bracket", 0x3011, true},
		{"ascii right paren", ')', false},
		{"matching open paren", 0xFF08, false},
		{"matching open angle", 0x300A, false},
		{"letter", 'z', false},
		{"cjk ideograph", '文', false},
		{"space", ' ', false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCJKCloseParen(tc.r); got != tc.want {
				t.Errorf("isCJKCloseParen(%U) = %v, want %v", tc.r, got, tc.want)
			}
		})
	}
}

// ----- isLetter / isDigit -----

func TestIsLetter(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{"lowercase a", 'a', true},
		{"lowercase z", 'z', true},
		{"uppercase A", 'A', true},
		{"uppercase Z", 'Z', true},
		{"digit", '1', false},
		{"underscore", '_', false},
		{"space", ' ', false},
		{"cjk ideograph", '中', false},
		{"non-ascii letter e acute", 0x00E9, false},
		{"fullwidth A", 0xFF21, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLetter(tc.r); got != tc.want {
				t.Errorf("isLetter(%U) = %v, want %v", tc.r, got, tc.want)
			}
		})
	}
}

func TestIsDigit(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{"zero", '0', true},
		{"five", '5', true},
		{"nine", '9', true},
		{"slash before digits", '/', false},
		{"colon after digits", ':', false},
		{"letter", 'a', false},
		{"space", ' ', false},
		{"fullwidth five", 0xFF15, false},
		{"cjk ideograph", '一', false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDigit(tc.r); got != tc.want {
				t.Errorf("isDigit(%U) = %v, want %v", tc.r, got, tc.want)
			}
		})
	}
}

// ----- detInputSizeFor -----

func TestDetInputSizeFor(t *testing.T) {
	e := &Engine{maxSide: DefaultDetMaxSide} // 960
	tests := []struct {
		name string
		w, h int
		want int
	}{
		{"tiny image rounds up to 64 minimum", 10, 10, 64},
		{"zero size rounds up to 64 minimum", 0, 0, 64},
		{"exactly 64 stays 64", 64, 64, 64},
		{"33 rounds up to 64", 33, 10, 64},
		{"65 rounds up to 96", 65, 65, 96},
		{"100 rounds up to 128", 100, 100, 128},
		{"132 rounds up to 160", 132, 132, 160},
		{"longest side drives rounding", 100, 132, 160},
		{"exactly maxSide is not downscaled", 960, 100, 960},
		{"over maxSide downscales to maxSide", 961, 100, 960},
		{"1920x1080 downscales to maxSide", 1920, 1080, 960},
		{"longest side over maxSide", 100, 2000, 960},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := e.detInputSizeFor(tc.w, tc.h); got != tc.want {
				t.Errorf("detInputSizeFor(%d, %d) = %d, want %d", tc.w, tc.h, got, tc.want)
			}
		})
	}
}

func TestDetInputSizeFor_CustomMaxSide(t *testing.T) {
	e := &Engine{maxSide: 256}
	tests := []struct {
		name string
		w, h int
		want int
	}{
		{"below custom maxSide rounds to multiple of 32", 200, 100, 224},
		{"above custom maxSide downscales to custom maxSide", 1000, 500, 256},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := e.detInputSizeFor(tc.w, tc.h); got != tc.want {
				t.Errorf("detInputSizeFor(%d, %d) = %d, want %d", tc.w, tc.h, got, tc.want)
			}
		})
	}
}
