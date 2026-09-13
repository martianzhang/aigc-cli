package image

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestBuildImageCurl(t *testing.T) {
	req := &types.GenerateRequest{
		Model:  "gpt-image-2-official",
		Prompt: "test",
	}
	curl := buildImageCurl(req, "https://api.apimart.ai/v1", "test-key")
	if curl == "" {
		t.Fatal("buildImageCurl() returned empty string")
	}
	if !strings.Contains(curl, "...-key") {
		t.Error("curl should contain masked API key")
	}
	if !strings.Contains(curl, "gpt-image-2-official") {
		t.Error("curl should contain model name")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1023, "1023B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1024 * 1024, "1.0MB"},
		{1024 * 1024 * 3 / 2, "1.5MB"},
	}
	for _, tc := range tests {
		if got := formatBytes(tc.in); got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatParams(t *testing.T) {
	tests := []struct {
		format  string
		quality int
		want    string
	}{
		{"", 0, ""},
		{"jpg", 0, " [jpg]"},
		{"jpg", 85, " [jpg q85]"},
		{"webp", -1, " [webp]"},
	}
	for _, tc := range tests {
		if got := formatParams(tc.format, tc.quality); got != tc.want {
			t.Errorf("formatParams(%q,%d) = %q, want %q", tc.format, tc.quality, got, tc.want)
		}
	}
}
