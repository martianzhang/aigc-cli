package chat

import (
	"fmt"
	"strings"
	"testing"
)

func benchChatModel() *chatModel {
	m := newChatModel(nil, nil, 10, "bench", "", nil, false, 0, 0, 0)
	m.width = 120
	m.height = 40
	return m
}

func BenchmarkClipString(b *testing.B) {
	cases := []struct {
		name string
		size int
	}{
		{"80B", 80},
		{"4KB", 4 << 10},
		{"64KB", 64 << 10},
		{"256KB", 256 << 10},
		{"1MB", 1 << 20},
	}
	for _, tc := range cases {
		s := strings.Repeat("x", tc.size)
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(tc.size))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = clipString(s, 80)
			}
		})
	}

	cjk := strings.Repeat("你好", 64<<10)
	b.Run("wide-runes", func(b *testing.B) {
		b.SetBytes(int64(len(cjk)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = clipString(cjk, 80)
		}
	})
}

func BenchmarkSummarizeToolResult(b *testing.B) {
	cases := []struct {
		name string
		in   string
	}{
		{"short", "done"},
		{"multi-line", "first line\n" + strings.Repeat("x", 64<<10)},
		{"long-single-line", strings.Repeat("x", 64<<10)},
		{"leading-blank-line", "\n" + strings.Repeat("x", 64<<10)},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.in)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = summarizeToolResult("bash", tc.in)
			}
		})
	}
}

func BenchmarkSplitLinesLimited(b *testing.B) {
	text := strings.Repeat("line\n", 100000)
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = splitLinesLimited(text, toolOutputPreviewLines)
	}
}

func BenchmarkRenderToolMessage(b *testing.B) {
	m := benchChatModel()
	cases := []struct {
		name     string
		content  string
		expanded bool
	}{
		{"single-long-line-collapsed", strings.Repeat("x", 64<<10), false},
		{"single-long-line-expanded", strings.Repeat("x", 64<<10), true},
		{"100k-lines-collapsed", strings.Repeat("line\n", 100000), false},
		{"100k-lines-expanded", strings.Repeat("line\n", 100000), true},
	}
	for _, tc := range cases {
		msg := message{role: "tool", tool: "bash", content: tc.content, expanded: tc.expanded}
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.content)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = m.renderToolMessage(msg)
			}
		})
	}
}

func BenchmarkRenderMessages(b *testing.B) {
	m := benchChatModel()
	m.messages = append(m.messages, message{role: "user", content: "benchmark transcript"})
	for i := 0; i < 20; i++ {
		var body strings.Builder
		for line := 0; line < 40; line++ {
			fmt.Fprintf(&body, "output line %d: %s\n", line, strings.Repeat("x", 96))
		}
		m.messages = append(m.messages, message{
			role:    "tool",
			tool:    "bash",
			content: body.String(),
		})
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.renderMessages()
	}
}
