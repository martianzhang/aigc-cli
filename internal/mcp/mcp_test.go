package mcp

import (
	"testing"

	"github.com/martianzhang/apimart-cli/internal/types"
)

func TestParseImageURLs_empty(t *testing.T) {
	got := parseImageURLs("")
	if got != nil {
		t.Errorf("parseImageURLs('') = %v, want nil", got)
	}
}

func TestParseImageURLs_single(t *testing.T) {
	got := parseImageURLs("https://example.com/img.png")
	if len(got) != 1 || got[0] != "https://example.com/img.png" {
		t.Errorf("parseImageURLs() = %v", got)
	}
}

func TestParseImageURLs_multiple(t *testing.T) {
	got := parseImageURLs("https://a.com/1.png, https://b.com/2.png")
	if len(got) != 2 {
		t.Fatalf("expected 2 URLs, got %d", len(got))
	}
	if got[0] != "https://a.com/1.png" {
		t.Errorf("got[0] = %q", got[0])
	}
	if got[1] != "https://b.com/2.png" {
		t.Errorf("got[1] = %q", got[1])
	}
}

func TestParseImageURLs_trailingComma(t *testing.T) {
	got := parseImageURLs("https://a.com/img.png,")
	if len(got) != 1 {
		t.Errorf("expected 1 URL, got %d", len(got))
	}
}

func TestBuildImageDesc_defaults(t *testing.T) {
	d := &types.ImageDefaults{
		Model:   "gpt-image-2-official",
		Size:    "1:1",
		Quality: "low",
	}
	desc := buildImageDesc(d, "https://api.apimart.ai")
	if desc == "" {
		t.Fatal("buildImageDesc returned empty")
	}
	if !contains(desc, "APIMart") {
		t.Error("description should mention APIMart")
	}
	if !contains(desc, "gpt-image-2-official") {
		t.Error("description should include model name")
	}
}

func TestBuildImageDesc_openRouter(t *testing.T) {
	d := &types.ImageDefaults{
		Model: "openai/gpt-image-2",
	}
	desc := buildImageDesc(d, "https://openrouter.ai/api/v1")
	if desc == "" {
		t.Fatal("buildImageDesc returned empty")
	}
	if !contains(desc, "OpenRouter") {
		t.Error("description should mention OpenRouter")
	}
}

func TestBuildImageDesc_nilDefaults(t *testing.T) {
	desc := buildImageDesc(nil, "https://openrouter.ai/api/v1")
	if desc == "" {
		t.Fatal("buildImageDesc returned empty")
	}
}

func TestBuildVideoDesc_defaults(t *testing.T) {
	d := &types.VideoDefaults{
		Model:      "doubao-seedance-2.0",
		Size:       "16:9",
		Resolution: "480p",
	}
	desc := buildVideoDesc(d, "https://api.apimart.ai")
	if desc == "" {
		t.Fatal("buildVideoDesc returned empty")
	}
	if !contains(desc, "APIMart") {
		t.Error("description should mention APIMart")
	}
}

func TestBuildVideoDesc_openRouter(t *testing.T) {
	d := &types.VideoDefaults{
		Model: "google/veo-3.1",
	}
	desc := buildVideoDesc(d, "https://openrouter.ai/api/v1")
	if desc == "" {
		t.Fatal("buildVideoDesc returned empty")
	}
	if !contains(desc, "OpenRouter") {
		t.Error("description should mention OpenRouter")
	}
	if !contains(desc, "polling_url") {
		t.Error("OpenRouter description should mention polling_url")
	}
}

func TestBuildVideoDesc_nilDefaults(t *testing.T) {
	desc := buildVideoDesc(nil, "https://openrouter.ai/api/v1")
	if desc == "" {
		t.Fatal("buildVideoDesc returned empty")
	}
}

func TestNewGetTaskTool(t *testing.T) {
	tool := newGetTaskTool()
	if tool.Name != "get_task" {
		t.Errorf("tool name = %q", tool.Name)
	}
}

func TestNewListModelsTool(t *testing.T) {
	tool := newListModelsTool()
	if tool.Name != "list_models" {
		t.Errorf("tool name = %q", tool.Name)
	}
}

func TestNewGetBalanceTool(t *testing.T) {
	tool := newGetBalanceTool()
	if tool.Name != "get_balance" {
		t.Errorf("tool name = %q", tool.Name)
	}
}

func TestNewGetModelPricingTool(t *testing.T) {
	tool := newGetModelPricingTool()
	if tool.Name != "get_model_pricing" {
		t.Errorf("tool name = %q", tool.Name)
	}
}

// contains reports whether substr is in s.
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && searchString(s, substr)
}

// searchString finds substr in s using simple substring match.
func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
