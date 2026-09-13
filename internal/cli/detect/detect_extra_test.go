package detect

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/watermark"
)

func TestCleanPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"with extension", "/a/b/photo.jpg", "/a/b/photo_clean.jpg"},
		{"without extension", "/a/b/photo", "/a/b/photo_clean"},
		{"multi-dot", "/a/b/archive.tar.gz", "/a/b/archive.tar_clean.gz"},
		{"uppercase ext", "img.PNG", "img_clean.PNG"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanPath(tc.in); got != tc.want {
				t.Errorf("cleanPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestScoreTag(t *testing.T) {
	tests := []struct {
		name string
		in   watermark.SeedQualityLevel
		want string
	}{
		{"good", watermark.SeedGood, "[GOOD]"},
		{"warn", watermark.SeedWarn, "[WARN]"},
		{"fail", watermark.SeedFail, "[FAIL]"},
		{"unknown", watermark.SeedQualityLevel(99), "[?]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := scoreTag(tc.in); got != tc.want {
				t.Errorf("scoreTag(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
