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

func TestBuildVideoCurlPollinations(t *testing.T) {
	const key = "sk_pollinations_test"
	options.Shared.Cfg = &types.Config{
		Providers: map[string]*types.NamedProvider{
			"pollinations": {
				Type:    types.ProviderOpenAI,
				APIKey:  key,
				BaseURL: "https://gen.pollinations.ai",
			},
		},
		Defaults: &types.ConfigDefaults{
			Video: &types.VideoDefaults{Provider: "pollinations"},
		},
	}
	t.Cleanup(func() { options.Shared.Cfg = nil })

	duration := 4
	req := &types.VideoGenerateRequest{
		Model:    "community/NamanSoni78/Seedance-2.5",
		Prompt:   "a cat walking",
		Duration: &duration,
	}

	curl := buildVideoCurl(req)

	if !strings.HasPrefix(curl, "curl -X GET ") {
		t.Errorf("pollinations dry-run should be a GET curl:\n%s", curl)
	}
	if !strings.Contains(curl, "gen.pollinations.ai/video/a%20cat%20walking") {
		t.Errorf("curl should use the root /video/{prompt} endpoint:\n%s", curl)
	}
	if strings.Contains(curl, "/videos/generations") {
		t.Errorf("curl must not use the APIMart async path:\n%s", curl)
	}
	if strings.Contains(curl, key) {
		t.Errorf("curl must mask the API key:\n%s", curl)
	}
}
