package types

import (
	"testing"
	"time"
)

func TestDefaultTaskWait(t *testing.T) {
	tests := []struct {
		media string
		want  time.Duration
	}{
		{MediaImage, DefaultImageTaskWait},
		{MediaVideo, DefaultVideoTaskWait},
		{MediaMusic, DefaultMusicTaskWait},
		{MediaMidjourney, FallbackTaskWait},
		{"unknown", FallbackTaskWait},
		{"", FallbackTaskWait},
	}
	for _, tc := range tests {
		if got := DefaultTaskWait(tc.media); got != tc.want {
			t.Errorf("DefaultTaskWait(%q) = %v, want %v", tc.media, got, tc.want)
		}
	}
}

func TestDefaultTaskWait_mediaDiffer(t *testing.T) {
	// The whole point of per-media defaults: video jobs queue longer than images.
	if DefaultVideoTaskWait <= DefaultImageTaskWait {
		t.Errorf("video wait (%v) should exceed image wait (%v)", DefaultVideoTaskWait, DefaultImageTaskWait)
	}
	if DefaultVideoTaskWait <= DefaultMusicTaskWait {
		t.Errorf("video wait (%v) should exceed music wait (%v)", DefaultVideoTaskWait, DefaultMusicTaskWait)
	}
}
