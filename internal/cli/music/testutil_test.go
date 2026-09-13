package music

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func ptr[T any](v T) *T { return &v }

func TestMusicTrackURL(t *testing.T) {
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
