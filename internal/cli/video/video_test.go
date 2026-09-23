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
	options.Shared.APIKeySet = true
	options.Shared.APIBaseSet = true
	t.Cleanup(func() {
		options.Shared.APIKeySet = false
		options.Shared.APIBaseSet = false
	})

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
	if !strings.Contains(curl, "https://api.apimart.ai/v1/videos/generations") {
		t.Errorf("curl should target the resolved provider host, got:\n%s", curl)
	}
}

func withVideoNamedProvider(t *testing.T, ref string, np *types.NamedProvider) {
	t.Helper()
	options.Shared.APIKeySet = false
	options.Shared.APIBaseSet = false
	options.Shared.ProviderSet = false
	options.Shared.Cfg = &types.Config{
		Providers: map[string]*types.NamedProvider{ref: np},
		Defaults:  &types.ConfigDefaults{Video: &types.VideoDefaults{Provider: ref}},
	}
	t.Cleanup(func() { options.Shared.Cfg = nil })
}

func TestBuildVideoCurlVerbatimPerProvider(t *testing.T) {
	const raw = `{"model":"m","prompt":"p","aspect_ratio":"16:9","custom_x":1}`
	req := &types.VideoGenerateRequest{RawJSON: []byte(raw)}

	tests := []struct {
		name    string
		ref     string
		np      *types.NamedProvider
		wantURL string
	}{
		{
			"openrouter uses /videos",
			"openrouter",
			&types.NamedProvider{Type: types.ProviderOpenAI, APIKey: "k", BaseURL: "https://openrouter.ai/api/v1"},
			"https://openrouter.ai/api/v1/videos",
		},
		{
			"agnes uses /videos",
			"agnes",
			&types.NamedProvider{Type: types.ProviderOpenAI, APIKey: "k", BaseURL: "https://apihub.agnes-ai.com/v1"},
			"https://apihub.agnes-ai.com/v1/videos",
		},
		{
			"openlux uses /video/create",
			"openlux",
			&types.NamedProvider{Type: types.ProviderOpenAI, APIKey: "k", BaseURL: "https://api.openlux.ai"},
			"https://api.openlux.ai/v1/video/create",
		},
		{
			"apimart uses /videos/generations",
			"apimart",
			&types.NamedProvider{Type: types.ProviderOpenAI, APIKey: "k", BaseURL: "https://api.apimart.ai"},
			"https://api.apimart.ai/v1/videos/generations",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withVideoNamedProvider(t, tc.ref, tc.np)
			curl := buildVideoCurl(req)
			if !strings.Contains(curl, tc.wantURL) {
				t.Errorf("curl should target %s, got:\n%s", tc.wantURL, curl)
			}
			if !strings.Contains(curl, raw) {
				t.Errorf("curl should forward the --json body verbatim, got:\n%s", curl)
			}
		})
	}
}

func TestBuildVideoCurlRendersLocalUploads(t *testing.T) {
	withVideoNamedProvider(t, "apimart", &types.NamedProvider{
		Type:    types.ProviderOpenAI,
		APIKey:  "k",
		BaseURL: "https://api.apimart.ai",
	})

	local := testLocalImage(t)

	req := &types.VideoGenerateRequest{
		Model:          "doubao-seedance-2.0",
		Prompt:         "test",
		ImageURLs:      []string{local},
		ImageWithRoles: []types.ImageWithRole{{URL: local, Role: "first_frame"}},
	}
	curl := buildVideoCurl(req)

	if !strings.Contains(curl, "https://api.apimart.ai/v1/uploads/images") {
		t.Errorf("curl should render the upload endpoint:\n%s", curl)
	}
	if n := strings.Count(curl, `-F "file=@`); n != 2 {
		t.Errorf("multipart upload count = %d, want 2:\n%s", n, curl)
	}
	for _, token := range []string{"<UPLOAD_URL_0>", "<UPLOAD_URL_1>"} {
		if !strings.Contains(curl, token) {
			t.Errorf("curl should carry literal placeholder %s:\n%s", token, curl)
		}
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
