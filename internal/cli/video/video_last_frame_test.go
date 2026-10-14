package video

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestWantsLastFrame(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name string
		req  *types.VideoGenerateRequest
		want bool
	}{
		{"nil request", nil, false},
		{"unset", &types.VideoGenerateRequest{}, false},
		{"true", &types.VideoGenerateRequest{ReturnLastFrame: &yes}, true},
		{"false", &types.VideoGenerateRequest{ReturnLastFrame: &no}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wantsLastFrame(tt.req); got != tt.want {
				t.Errorf("wantsLastFrame() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLastFrameURLs(t *testing.T) {
	images := []types.ImageResult{
		{URL: []string{"https://cdn/f1.png"}},
		{URL: []string{"", "https://cdn/f2.png"}},
		{URL: nil},
	}
	got := lastFrameURLs(images)
	want := []string{"https://cdn/f1.png", "https://cdn/f2.png"}
	if len(got) != len(want) {
		t.Fatalf("lastFrameURLs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("lastFrameURLs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestSaveLastFramesPrefersProvider proves the provider-returned frames are kept
// as-is and no local extraction is attempted (the video path does not exist).
func TestSaveLastFramesPrefersProvider(t *testing.T) {
	out := &RunResult{
		Videos:     []string{"does-not-exist.mp4"},
		LastFrames: []string{"provider-frame.png"},
	}
	saveLastFrames(out)
	if len(out.LastFrames) != 1 || out.LastFrames[0] != "provider-frame.png" {
		t.Errorf("LastFrames = %v, want the provider frame untouched", out.LastFrames)
	}
}

func TestSaveLastFramesNil(t *testing.T) {
	saveLastFrames(nil) // must not panic
}
