package client

import (
	"encoding/json"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestGeminiImageFormat(t *testing.T) {
	tests := []struct {
		name       string
		size       string
		ratio      string
		wantAspect string
		wantSize   string
	}{
		{name: "tier and ratio", size: "2K", ratio: "16:9", wantAspect: "16:9", wantSize: "2K"},
		{name: "lowercase tier", size: "2k", wantSize: "2K"},
		{name: "tier only", size: "4K", wantSize: "4K"},
		{name: "half K tier", size: "0.5K", wantSize: "0.5K"},
		{name: "512 tier", size: "512", wantSize: "512"},
		{name: "512px tier", size: "512px", wantSize: "512"},
		{name: "unknown tier passes through", size: "3K", wantSize: "3K"},
		{name: "ratio size", size: "16:9", wantAspect: "16:9"},
		{name: "ratio flag", ratio: "21:9", wantAspect: "21:9"},
		{name: "ratio flag wins over pixels", size: "1024x1024", ratio: "16:9", wantAspect: "16:9"},
		{name: "square pixels", size: "1024x1024", wantAspect: "1:1"},
		{name: "widescreen pixels", size: "2048x1152", wantAspect: "16:9"},
		{name: "uppercase separator", size: "2048X1152", wantAspect: "16:9"},
		{name: "dashscope separator", size: "1024*1024", wantAspect: "1:1"},
		{name: "portrait 3:4", size: "1024x1365", wantAspect: "3:4"},
		{name: "landscape 3:2", size: "1536x1024", wantAspect: "3:2"},
		{name: "ultrawide 21:9", size: "2401x1024", wantAspect: "21:9"},
		{name: "portrait 4:5", size: "1024x1280", wantAspect: "4:5"},
		{name: "portrait 9:16", size: "1080x1920", wantAspect: "9:16"},
		{name: "non standard square", size: "1000x1000", wantAspect: "1:1"},
		{name: "auto size", size: "auto"},
		{name: "auto ratio", size: "1024x1024", ratio: "auto", wantAspect: "1:1"},
		{name: "unrecognized size", size: "garbage"},
		{name: "empty", size: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			aspect, size := geminiImageFormat(tc.size, tc.ratio)
			if aspect != tc.wantAspect || size != tc.wantSize {
				t.Errorf("geminiImageFormat(%q, %q) = (%q, %q), want (%q, %q)",
					tc.size, tc.ratio, aspect, size, tc.wantAspect, tc.wantSize)
			}
		})
	}
}

func TestGeminiNearestRatio(t *testing.T) {
	tests := []struct {
		w, h int
		want string
	}{
		{1024, 1024, "1:1"},
		{2048, 1152, "16:9"},
		{1024, 1365, "3:4"},
		{1365, 1024, "4:3"},
		{1820, 1024, "16:9"},
		{1024, 1820, "9:16"},
		{2401, 1024, "21:9"},
		{1536, 864, "16:9"},
		{0, 1024, ""},
	}
	for _, tc := range tests {
		if got := geminiNearestRatio(tc.w, tc.h); got != tc.want {
			t.Errorf("geminiNearestRatio(%d, %d) = %q, want %q", tc.w, tc.h, got, tc.want)
		}
	}
}

// geminiFormatOf builds the Gemini body and decodes its response_format.
func geminiFormatOf(t *testing.T, req *types.GenerateRequest) geminiResponseFormat {
	t.Helper()
	raw := bodyOf(t, GeminiImageBody, req)
	var probe struct {
		ResponseFormat geminiResponseFormat `json:"response_format"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		t.Fatalf("unmarshal gemini body: %v", err)
	}
	return probe.ResponseFormat
}

func TestGeminiImageBodyMapsSize(t *testing.T) {
	t.Run("tier and ratio", func(t *testing.T) {
		got := geminiFormatOf(t, &types.GenerateRequest{
			Model: "gemini-3-pro-image", Prompt: "p", Size: "2K", Ratio: "16:9",
		})
		if got.AspectRatio != "16:9" || got.ImageSize != "2K" || got.Type != "image" {
			t.Errorf("response_format = %+v, want type=image aspect_ratio=16:9 image_size=2K", got)
		}
	})

	t.Run("tier only", func(t *testing.T) {
		got := geminiFormatOf(t, &types.GenerateRequest{Prompt: "p", Size: "2K"})
		if got.AspectRatio != "" || got.ImageSize != "2K" {
			t.Errorf("response_format = %+v, want image_size=2K and no aspect_ratio", got)
		}
	})

	t.Run("pixels derive aspect ratio", func(t *testing.T) {
		got := geminiFormatOf(t, &types.GenerateRequest{Prompt: "p", Size: "2048x1152"})
		if got.AspectRatio != "16:9" || got.ImageSize != "" {
			t.Errorf("response_format = %+v, want aspect_ratio=16:9 and no image_size", got)
		}
	})

	t.Run("ratio only", func(t *testing.T) {
		got := geminiFormatOf(t, &types.GenerateRequest{Prompt: "p", Ratio: "9:16"})
		if got.AspectRatio != "9:16" || got.ImageSize != "" {
			t.Errorf("response_format = %+v, want aspect_ratio=9:16", got)
		}
	})

	t.Run("no size keeps model default", func(t *testing.T) {
		got := geminiFormatOf(t, &types.GenerateRequest{Prompt: "p"})
		if got.AspectRatio != "" || got.ImageSize != "" || got.Type != "image" {
			t.Errorf("response_format = %+v, want type=image only", got)
		}
	})
}
