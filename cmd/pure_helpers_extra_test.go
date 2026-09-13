package cmd

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/watermark"
)

func TestCmdCleanPath(t *testing.T) {
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

func TestCmdExtractHost(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"host only", "https://example.com/a/b", "example.com"},
		{"host with port", "https://example.com:8080/x", "example.com:8080"},
		{"invalid", "://bad", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractHost(tc.in); got != tc.want {
				t.Errorf("extractHost(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCmdMusicTrackURL(t *testing.T) {
	tests := []struct {
		name  string
		track types.MusicTrack
		want  string
	}{
		{"empty", types.MusicTrack{}, ""},
		{"audio wins", types.MusicTrack{AudioURL: "a", WAVURL: "w", FileURL: "f", VideoURL: "v"}, "a"},
		{"wav falls through", types.MusicTrack{WAVURL: "w", FileURL: "f", VideoURL: "v"}, "w"},
		{"file falls through", types.MusicTrack{FileURL: "f", VideoURL: "v"}, "f"},
		{"video last", types.MusicTrack{VideoURL: "v"}, "v"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := musicTrackURL(tc.track); got != tc.want {
				t.Errorf("musicTrackURL(%+v) = %q, want %q", tc.track, got, tc.want)
			}
		})
	}
}

func TestCmdScoreTag(t *testing.T) {
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
