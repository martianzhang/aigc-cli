package video

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestBuildVideoCurl(t *testing.T) {
	options.Shared.APIKey = "test-key"
	options.Shared.APIBase = "https://api.apimart.ai"
	req := &types.VideoGenerateRequest{
		Model:  "doubao-seedance-2.0",
		Prompt: "test video",
	}
	curl := buildVideoCurl(req)
	if curl == "" {
		t.Fatal("buildVideoCurl() returned empty string")
	}
	if !strings.Contains(curl, "...-key") {
		t.Error("curl should contain masked API key")
	}
	if !strings.Contains(curl, "doubao-seedance-2.0") {
		t.Error("curl should contain model name")
	}
}
