package webfetch

import (
	"strings"
	"testing"
)

func TestShouldUseFullDocument(t *testing.T) {
	tests := []struct {
		name     string
		readable string
		full     string
		want     bool
	}{
		{
			name:     "footer-only result falls back to the full page",
			readable: strings.Repeat("a", 600),
			full:     strings.Repeat("b", 12000),
			want:     true,
		},
		{
			name:     "short but representative article stays readable",
			readable: strings.Repeat("a", 900),
			full:     strings.Repeat("b", 1800),
			want:     false,
		},
		{
			name:     "long article stays readable",
			readable: strings.Repeat("a", 3000),
			full:     strings.Repeat("b", 30000),
			want:     false,
		},
		{
			name:     "empty readable uses the full page",
			readable: "",
			full:     "content",
			want:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldUseFullDocument(tc.readable, tc.full); got != tc.want {
				t.Errorf("shouldUseFullDocument() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSanitizeMarkdownRemovesDataURIs(t *testing.T) {
	input := `[![](data:image/png;base64,AAA)](https://example.com) ` +
		`[text](data:text/plain,abc) ![alt](data:image/png;base64,BBB) <data:text/plain,x> ` +
		`[设置](javascript:;) ` +
		"[![](https://example.com/icon.png)\n翻译](https://fanyi.example.com)"
	out := sanitizeMarkdown(input)
	if strings.Contains(out, "data:") || strings.Contains(out, "javascript:") {
		t.Errorf("sanitized markdown still contains a junk URI:\n%s", out)
	}
	if strings.Contains(out, "!https://") {
		t.Errorf("sanitized markdown corrupted an image link:\n%s", out)
	}
	for _, want := range []string{"https://example.com", "text", "alt", "设置", "翻译", "https://fanyi.example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("sanitized markdown lost %q:\n%s", want, out)
		}
	}
}

func TestStripChromeHTMLKeepsNavRemovesFooter(t *testing.T) {
	input := `<html><head><style>body{color:red}</style></head><body>
		<nav><a href="/news">新闻</a></nav>
		<main><p>正文</p></main>
		<footer>版权信息</footer>
		<div id="ftCon">备案信息</div>
	</body></html>`

	out := stripChromeHTML(input)
	for _, want := range []string{"新闻", "正文"} {
		if !strings.Contains(out, want) {
			t.Errorf("stripped HTML lost %q:\n%s", want, out)
		}
	}
	for _, notWant := range []string{"版权信息", "备案信息", "color:red"} {
		if strings.Contains(out, notWant) {
			t.Errorf("stripped HTML kept %q:\n%s", notWant, out)
		}
	}
}
