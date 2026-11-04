package session

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func BenchmarkTruncateToolContent(b *testing.B) {
	cases := []struct {
		name string
		size int
	}{
		{"32KB", DefaultToolOutputMaxBytes},
		{"64KB", 64 << 10},
		{"8MB", 8 << 20},
	}
	for _, tc := range cases {
		s := strings.Repeat("x", tc.size)
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(tc.size))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _, _ = truncateToolContent(s, DefaultToolOutputMaxBytes)
			}
		})
	}
}

func BenchmarkAppendMessage(b *testing.B) {
	cases := []struct {
		name string
		size int
	}{
		{"32KB", DefaultToolOutputMaxBytes},
		{"64KB", 64 << 10},
		{"8MB", 8 << 20},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			path := filepath.Join(b.TempDir(), "bench.jsonl")
			s, err := New(path, Header{ID: "bench", Model: "bench"})
			if err != nil {
				b.Fatal(err)
			}
			content := strings.Repeat("x", tc.size)
			b.SetBytes(int64(tc.size))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Keep the file and in-memory projection small so the benchmark
				// measures one append+truncate, not unbounded growth.
				if err := s.f.Truncate(0); err != nil {
					b.Fatal(err)
				}
				if _, err := s.f.Seek(0, io.SeekStart); err != nil {
					b.Fatal(err)
				}
				s.messages = s.messages[:0]
				msg := types.ChatMessage{Role: "tool", Content: content}
				if err := s.AppendMessage(msg, "bash", DefaultToolOutputMaxBytes); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			if err := s.Close("bench"); err != nil {
				b.Fatal(err)
			}
		})
	}
}

func BenchmarkHistory(b *testing.B) {
	s, err := New(filepath.Join(b.TempDir(), "history.jsonl"), Header{ID: "bench"})
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		msg := types.ChatMessage{Role: "user", Content: fmt.Sprintf("message %d", i)}
		if err := s.AppendMessage(msg, "", 0); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.History()
	}
}
