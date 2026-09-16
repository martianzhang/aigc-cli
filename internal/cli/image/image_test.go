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
	curl := buildImageCurl(req, "https://api.apimart.ai/v1", "test-key", false)
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

func TestBuildImageCurlEdits(t *testing.T) {
	req := &types.GenerateRequest{
		Model:     "my-model",
		Prompt:    "make it blue",
		Size:      "1024x768",
		ImageURLs: []string{"photo.png"},
	}

	t.Run("bare host gains v1", func(t *testing.T) {
		curl := buildImageCurl(req, "https://api.zeekai.cc", "test-key", true)
		if !strings.Contains(curl, "https://api.zeekai.cc/v1/images/edits") {
			t.Errorf("url should end with /v1/images/edits, got:\n%s", curl)
		}
		if !strings.Contains(curl, `"images":[{"image_url":"photo.png"}]`) {
			t.Errorf("body should embed images[].image_url, got:\n%s", curl)
		}
		if !strings.Contains(curl, "# note: local image files are embedded as data: URIs") {
			t.Errorf("edits dry-run should append the data-URI note, got:\n%s", curl)
		}
		for _, absent := range []string{`"n":`, `"quality":`, `"output_format":`} {
			if strings.Contains(curl, absent) {
				t.Errorf("unset optional field %s should be absent, got:\n%s", absent, curl)
			}
		}
	})

	t.Run("versioned base not doubled", func(t *testing.T) {
		curl := buildImageCurl(req, "https://api.zeekai.cc/v1", "test-key", true)
		if strings.Contains(curl, "/v1/v1/") {
			t.Errorf("version segment should not be doubled, got:\n%s", curl)
		}
		if !strings.Contains(curl, "https://api.zeekai.cc/v1/images/edits") {
			t.Errorf("url should be /v1/images/edits, got:\n%s", curl)
		}
	})
}

func TestBuildImageCurlDefaultMode(t *testing.T) {
	req := &types.GenerateRequest{
		Model:     "gpt-image-2",
		Prompt:    "a cat",
		ImageURLs: []string{"photo.png"},
	}
	curl := buildImageCurl(req, "https://api.zeekai.cc", "test-key", false)
	if !strings.Contains(curl, "https://api.zeekai.cc/v1/images/generations") {
		t.Errorf("bare host should be normalized to /v1/images/generations, got:\n%s", curl)
	}
	if strings.Contains(curl, "/images/edits") {
		t.Errorf("default mode should not use /images/edits, got:\n%s", curl)
	}
	if !strings.Contains(curl, `"image_urls":["photo.png"]`) {
		t.Errorf("default body should use top-level image_urls, got:\n%s", curl)
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
