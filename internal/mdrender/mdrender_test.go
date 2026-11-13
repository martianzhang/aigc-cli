package mdrender

import (
	"regexp"
	"strings"
	"testing"
)

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiEscape.ReplaceAllString(s, "") }

// TestSplitMarkdownTables guards the segmentation the table rendering relies on.
func TestSplitMarkdownTables(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []mdSegment
	}{
		{
			name: "prose and table",
			src:  "前文\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n后文",
			want: []mdSegment{
				{src: "前文\n"},
				{src: "| a | b |\n|---|---|\n| 1 | 2 |", table: true},
				{src: "\n后文"},
			},
		},
		{
			name: "table inside a list item stays put",
			src:  "1. 步骤：\n   | a | b |\n   |---|---|\n2. 下一步",
			want: []mdSegment{{src: "1. 步骤：\n   | a | b |\n   |---|---|\n2. 下一步"}},
		},
		{
			name: "table glued to a paragraph stays put",
			src:  "看这个\n| a | b |\n|---|---|",
			want: []mdSegment{{src: "看这个\n| a | b |\n|---|---|"}},
		},
		{
			name: "table inside a code fence stays put",
			src:  "```\n| a | b |\n|---|---|\n```",
			want: []mdSegment{{src: "```\n| a | b |\n|---|---|\n```"}},
		},
		{
			name: "two tables",
			src:  "| a | b |\n|---|---|\n\n中间\n\n| c | d |\n|---|---|",
			want: []mdSegment{
				{src: "| a | b |\n|---|---|", table: true},
				{src: "\n中间\n"},
				{src: "| c | d |\n|---|---|", table: true},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitMarkdownTables(tc.src)
			if len(got) != len(tc.want) {
				t.Fatalf("segments = %d (%+v), want %d (%+v)", len(got), got, len(tc.want), tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("segment %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestTrimMarkdownPaddingKeepsIndent guards that the leading spaces glamour
// uses to line up a table's first row survive the padding cleanup.
func TestTrimMarkdownPaddingKeepsIndent(t *testing.T) {
	got := trimMarkdownPadding("\n\n  a  \n  b  \n\n")
	if want := "  a\n  b"; stripANSI(got) != want {
		t.Errorf("trimMarkdownPadding = %q, want %q", stripANSI(got), want)
	}
}

// TestRenderPlainTextIsUntouched guards that prose without markdown syntax is
// not sent through glamour (which would add blank rows around it).
func TestRenderPlainTextIsUntouched(t *testing.T) {
	src := "\n  plain text without syntax  \n"
	if got := Render(src, 80); got != strings.TrimSpace(src) {
		t.Errorf("Render = %q, want %q", got, strings.TrimSpace(src))
	}
	if got := Render("   \n\n", 80); got != "" {
		t.Errorf("Render(blank) = %q, want empty", got)
	}
}

// TestRenderWrapsToContentWidth guards the pipeline preview relies on: Render
// leaves wrapping to WrapANSI, which must keep every line inside the width.
func TestRenderWrapsToContentWidth(t *testing.T) {
	src := "# 标题\n\n" + strings.Repeat("这是一段很长的说明文字。", 12) + "\n"
	rendered := Render(src, 40)
	wrapped, _ := WrapANSI(rendered, 40)

	if widest := widestLine(wrapped); widest > 40 {
		t.Errorf("widest line = %d columns, want <= 40:\n%s", widest, stripANSI(wrapped))
	}
	if !strings.Contains(stripANSI(wrapped), "标题") {
		t.Errorf("rendered output lost the heading: %q", wrapped)
	}
}

// TestRenderKeepsNarrowTableNarrow guards the shared table sizing: a table
// sized from its content is not spread across the whole content area.
func TestRenderKeepsNarrowTableNarrow(t *testing.T) {
	rendered := stripANSI(Render("对比：\n\n| 名称 | 说明 |\n|---|---|\n| alpha | 第一个 |\n| beta | 第二个 |\n", 100))
	if widest := widestLine(rendered); widest > 20 {
		t.Errorf("narrow table rendered %d columns wide, want <= 20:\n%s", widest, rendered)
	}
	if !strings.Contains(rendered, "对比：") {
		t.Errorf("prose around the table was lost:\n%s", rendered)
	}
}

// TestRenderWrapsWideTable guards the other side: a table wider than the
// content area is rendered at that width so its cells wrap.
func TestRenderWrapsWideTable(t *testing.T) {
	body := "| a | b |\n|---|---|\n| 1 | " + strings.Repeat("很长的说明 ", 20) + " |\n"
	rendered := stripANSI(Render(body, 60))
	if widest := widestLine(rendered); widest > tableWidth(60) {
		t.Errorf("wide table rendered %d columns wide, want <= %d:\n%s", widest, tableWidth(60), rendered)
	}
}
