package terminal

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteTableAlignsCJKWidths(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"TYPE", "TEXT", "RULE", "POS"}
	rows := [][]string{
		{"PERSON", "张*", "chatner", "3..5"},
		{"ID_CARD", "110105********1234", "id_card", "10..28"},
		{"ADDRESS", "北京市朝阳区******", "chatner", "44..56"},
	}
	if err := WriteTable(&buf, headers, rows, TableOptions{}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4", len(lines))
	}
	// Every line must put its final column at the same display column.
	lastCol := []string{"POS", "3..5", "10..28", "44..56"}
	var want int
	for i, line := range lines {
		idx := strings.Index(line, lastCol[i])
		if idx < 0 {
			t.Fatalf("line %q has no %q", line, lastCol[i])
		}
		got := DisplayWidth(line[:idx])
		if i == 0 {
			want = got
			continue
		}
		if got != want {
			t.Fatalf("last column starts at cell %d, want %d in %q", got, want, line)
		}
	}
}

func TestWriteTableLastColumnNotPadded(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteTable(&buf, []string{"A", "B"}, [][]string{{"x", "y"}}, TableOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n") {
		if strings.HasSuffix(line, " ") {
			t.Fatalf("line has trailing spaces: %q", line)
		}
	}
}

func TestWriteTableSeparatorAndIndent(t *testing.T) {
	var buf bytes.Buffer
	err := WriteTable(&buf, []string{"ID", "Title"},
		[][]string{{"abc", "中文标题"}, {"def", "x"}},
		TableOptions{Indent: " ", Separator: true})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(lines), buf.String())
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, " ") {
			t.Fatalf("line missing indent: %q", line)
		}
	}
	if !strings.Contains(lines[1], "─") {
		t.Fatalf("missing separator: %q", lines[1])
	}
	// The separator must be as wide as the widest cell (ID=3, 中文标题=8 cells).
	if got := DisplayWidth(lines[1]); got != 1+3+2+8 {
		t.Fatalf("separator width = %d", got)
	}
}

func TestPadRight(t *testing.T) {
	if got := PadRight("中文", 6); got != "中文  " {
		t.Fatalf("PadRight = %q", got)
	}
	if got := PadRight("abcdef", 3); got != "abcdef" {
		t.Fatalf("PadRight should not truncate, got %q", got)
	}
}
