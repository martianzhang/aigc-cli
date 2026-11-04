package agent

import (
	"strings"
	"testing"
)

func BenchmarkCapToolOutput(b *testing.B) {
	cases := []struct {
		name string
		size int
	}{
		{"64KB", 64 << 10},
		{"2MB", 2 << 20},
		{"8MB", 8 << 20},
	}
	for _, tc := range cases {
		s := strings.Repeat("x", tc.size)
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(tc.size))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = CapToolOutput(s)
			}
		})
	}
}

func BenchmarkCappedOutput(b *testing.B) {
	const total = 8 << 20
	chunk := []byte(strings.Repeat("x", 32<<10))
	b.SetBytes(total)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := newCappedOutput(MaxToolOutputBytes - shellOutputMarkerReserve)
		for written := 0; written < total; written += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				b.Fatal(err)
			}
		}
		_ = w.String()
	}
}
