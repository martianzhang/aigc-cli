package image

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestImageStamp(t *testing.T) {
	if got := imageStamp(1700000000); got != 1700000000 {
		t.Errorf("imageStamp(created) = %d, want the provider timestamp unchanged", got)
	}
	if got := imageStamp(0); got == 0 {
		t.Error("imageStamp(0) = 0; providers that omit `created` must get a fallback timestamp")
	}
}

// TestPromptTaskID_FallsBackWhenCreatedMissing is the regression for files
// named image_sync_0_0.png: an OpenAI-compatible relay that omits `created`
// must still produce a timestamped prefix instead of a constant zero.
func TestPromptTaskID_FallsBackWhenCreatedMissing(t *testing.T) {
	got := promptTaskID("image_sync", 0)
	if got == "image_sync_0" {
		t.Fatal("promptTaskID(..., 0) still collapses to image_sync_0")
	}
	if !strings.HasPrefix(got, "image_sync_") {
		t.Errorf("promptTaskID() = %q, want the image_sync_ prefix", got)
	}
}

func TestSaveOpenAIImageResponse_UsesTimestampedTaskID(t *testing.T) {
	old := options.Shared
	options.Shared = &options.SharedConfig{OutputDir: t.TempDir()}
	t.Cleanup(func() { options.Shared = old })

	const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

	// Created is deliberately 0, as returned by some OpenAI-compatible relays.
	taskID := promptTaskID("image_sync", 0)
	saved, err := saveOpenAIImageResponse(&types.OpenAIImageResponse{
		Data: []types.OpenAIImageData{{B64JSON: onePixelPNG}},
	}, taskID)
	if err != nil {
		t.Fatalf("saveOpenAIImageResponse() error = %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("saved files = %d, want 1", len(saved))
	}

	base := filepath.Base(saved[0])
	if !strings.HasPrefix(base, taskID+"_0") {
		t.Errorf("filename = %q, want prefix %q (unique timestamp + index)", base, taskID+"_0")
	}
	if strings.HasPrefix(base, "image_sync_0_0") {
		t.Errorf("filename = %q; the provider's missing created field was used verbatim", base)
	}
}
