package image

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestPrepareAgnesImageRequest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.png")
	if err := os.WriteFile(path, []byte("\x89PNG\r\n\x1a\nfake-body"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	req := &types.GenerateRequest{
		ImageURLs:      []string{path},
		Ratio:          "16:9",
		ResponseFormat: "url",
		Quality:        "high",
		OutputFormat:   "png",
	}

	if err := prepareAgnesImageRequest(req); err != nil {
		t.Fatalf("prepareAgnesImageRequest() error = %v", err)
	}

	if req.ImageURLs != nil {
		t.Errorf("top-level ImageURLs should be cleared, got %v", req.ImageURLs)
	}

	images, ok := req.ExtraBody["image"].([]string)
	if !ok {
		t.Fatalf("extra_body.image type = %T, want []string", req.ExtraBody["image"])
	}
	if len(images) != 1 {
		t.Fatalf("extra_body.image length = %d, want 1", len(images))
	}
	if !strings.HasPrefix(images[0], "data:image/png;base64,") {
		t.Errorf("extra_body.image[0] = %q, want data:image/png;base64,... prefix", images[0])
	}

	if req.ExtraBody["ratio"] != "16:9" {
		t.Errorf("extra_body.ratio = %v, want 16:9", req.ExtraBody["ratio"])
	}
	if req.ExtraBody["response_format"] != "url" {
		t.Errorf("extra_body.response_format = %v, want url", req.ExtraBody["response_format"])
	}
	if req.ResponseFormat != "" {
		t.Errorf("top-level ResponseFormat should be cleared, got %q", req.ResponseFormat)
	}
	if req.Quality != "" {
		t.Errorf("Quality should be cleared, got %q", req.Quality)
	}
	if req.OutputFormat != "" {
		t.Errorf("OutputFormat should be cleared, got %q", req.OutputFormat)
	}
}
