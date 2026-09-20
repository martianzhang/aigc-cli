package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func namedProviderConfig() *Config {
	return &Config{
		APIKey:  "sk-global",
		BaseURL: "https://global.example.com/v1",
		Providers: map[string]*types.NamedProvider{
			"my-openrouter": {
				Type:    types.ProviderOpenAI,
				APIKey:  "sk-named",
				BaseURL: "https://openrouter.ai/api/v1",
			},
		},
	}
}

func TestResolveProviderRef_EmptyUsesCommandDefault(t *testing.T) {
	want := &provider.EffectiveProvider{
		Name:    "image-default",
		APIKey:  "sk-default",
		BaseURL: "https://default.example.com/v1",
	}
	cfg := namedProviderConfig()
	cfg.CmdProviders = map[string]*provider.EffectiveProvider{"image": want}

	got, errMsg := cfg.resolveProviderRef("image", "")
	if errMsg != "" {
		t.Fatalf("empty provider ref must not be rejected: %s", errMsg)
	}
	if got != want {
		t.Fatalf("got %+v, want the pre-resolved command default %+v", got, want)
	}
}

func TestResolveProviderRef_EmptyFallsBackToGlobal(t *testing.T) {
	cfg := namedProviderConfig()

	got, errMsg := cfg.resolveProviderRef("image", "")
	if errMsg != "" {
		t.Fatalf("empty provider ref must not be rejected: %s", errMsg)
	}
	if got == nil {
		t.Fatal("expected non-nil provider")
	}
	if got.APIKey != "sk-global" || got.BaseURL != "https://global.example.com/v1" {
		t.Errorf("got APIKey=%q BaseURL=%q, want global fallback", got.APIKey, got.BaseURL)
	}
}

func TestResolveProviderRef_NamedProviderResolvesBaseURLAndKey(t *testing.T) {
	cfg := namedProviderConfig()

	got, errMsg := cfg.resolveProviderRef("image", "my-openrouter")
	if errMsg != "" {
		t.Fatalf("valid named provider rejected: %s", errMsg)
	}
	if got == nil {
		t.Fatal("expected non-nil provider")
	}
	if got.Name != "my-openrouter" {
		t.Errorf("Name = %q, want my-openrouter", got.Name)
	}
	if got.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("BaseURL = %q, want the named provider base_url", got.BaseURL)
	}
	if got.APIKey != "sk-named" {
		t.Errorf("APIKey = %q, want the named provider api_key", got.APIKey)
	}
}

func TestResolveProviderRef_UnknownNameRejected(t *testing.T) {
	cfg := namedProviderConfig()

	got, errMsg := cfg.resolveProviderRef("image", "not-configured")
	if errMsg == "" {
		t.Fatal("expected unknown provider name to be rejected")
	}
	if got != nil {
		t.Errorf("got provider %+v, want nil on rejection", got)
	}
	if !strings.Contains(errMsg, "not found") {
		t.Errorf("message = %q, want it to mention not found", errMsg)
	}
}

func TestResolveProviderRef_NoProvidersConfiguredRejected(t *testing.T) {
	cfg := &Config{APIKey: "sk-global", BaseURL: "https://global.example.com/v1"}

	got, errMsg := cfg.resolveProviderRef("image", "anything")
	if errMsg == "" {
		t.Fatal("expected rejection when no providers are configured")
	}
	if got != nil {
		t.Errorf("got provider %+v, want nil on rejection", got)
	}
}

func TestResolveProviderRef_URLRejectedEvenIfWhitelisted(t *testing.T) {
	raw := "https://evil.example.com/v1"
	cfg := &Config{
		APIKey:  "sk-global",
		BaseURL: "https://global.example.com/v1",
		Providers: map[string]*types.NamedProvider{
			raw: {BaseURL: raw, APIKey: "sk-evil"},
		},
	}

	got, errMsg := cfg.resolveProviderRef("image", raw)
	if errMsg == "" {
		t.Fatal("a raw URL must be rejected even when present as a provider key")
	}
	if got != nil {
		t.Errorf("got provider %+v, want nil on rejection", got)
	}
	if !strings.Contains(errMsg, "not accepted") {
		t.Errorf("message = %q, want URL rejection", errMsg)
	}
}

func TestResolveProviderRef_ShapeRejected(t *testing.T) {
	cases := []struct {
		name string
		ref  string
	}{
		{"http scheme", "http://127.0.0.1:8000"},
		{"absolute path", "/etc/aigc-cli/config.yaml"},
		{"relative path", "providers/local"},
		{"windows path", `..\secrets\provider`},
		{"at sign", "key@host"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := namedProviderConfig()

			got, errMsg := cfg.resolveProviderRef("image", tc.ref)
			if errMsg == "" {
				t.Errorf("provider %q must be rejected", tc.ref)
			}
			if got != nil {
				t.Errorf("got provider %+v, want nil on rejection", got)
			}
		})
	}
}

func TestGenerationTools_providerParamWhitelist(t *testing.T) {
	tools := []mcp.Tool{
		newGenerateImageTool("d"),
		newGenerateMusicTool("d"),
		newGenerateSpeechTool("d"),
		newTranscribeAudioTool("d"),
		newMidjourneyImagineTool("d"),
		newMidjourneyDescribeTool("d"),
		newMidjourneyRerollTool("d"),
		newMidjourneyVideoTool("d"),
	}
	for _, tool := range tools {
		props := tool.InputSchema.Properties
		raw, ok := props["provider"]
		if !ok {
			t.Errorf("%s: missing provider param", tool.Name)
			continue
		}
		schema, ok := raw.(map[string]any)
		if !ok {
			t.Errorf("%s: provider schema type = %T, want map[string]any", tool.Name, raw)
			continue
		}
		if schema["description"] != providerArgDesc {
			t.Errorf("%s: provider description = %v, want %q", tool.Name, schema["description"], providerArgDesc)
		}
	}

	for _, tool := range append(tools, newGenerateVideoTool("d")) {
		for _, forbidden := range []string{"api_key", "base_url", "url", "set_config"} {
			if _, ok := tool.InputSchema.Properties[forbidden]; ok {
				t.Errorf("%s: forbidden param %q exposed", tool.Name, forbidden)
			}
		}
	}
}

func TestGenerateVideoTool_hasNoProviderParam(t *testing.T) {
	tool := newGenerateVideoTool("d")
	if _, ok := tool.InputSchema.Properties["provider"]; ok {
		t.Error("generate_video must not expose a provider param: generateVideoHandler delegates to video.GenerateAndSave, which re-resolves the provider from config")
	}
}

func TestGenerationHandlers_rejectURLProvider(t *testing.T) {
	cfg := &Config{APIKey: "sk-test", BaseURL: "https://api.example.com/v1"}
	handlers := []struct {
		name string
		fn   func(*Config) server.ToolHandlerFunc
	}{
		{"generate_image", generateImageHandler},
		{"generate_music", generateMusicHandler},
		{"generate_speech", generateSpeechHandler},
		{"transcribe_audio", transcribeAudioHandler},
		{"midjourney_imagine", midjourneyImagineHandler},
		{"midjourney_describe", midjourneyDescribeHandler},
		{"midjourney_reroll", midjourneyRerollHandler},
		{"midjourney_video", midjourneyVideoHandler},
	}
	for _, h := range handlers {
		t.Run(h.name, func(t *testing.T) {
			req := mcp.CallToolRequest{}
			req.Params.Name = h.name
			req.Params.Arguments = map[string]any{"provider": "https://evil.example.com/v1"}

			res, err := h.fn(cfg)(context.Background(), req)
			if err != nil {
				t.Fatalf("handler returned transport error: %v", err)
			}
			if res == nil || !res.IsError {
				t.Fatal("expected IsError=true for a URL provider")
			}
			if got := toolResultText(t, res); !strings.Contains(got, "not accepted") {
				t.Errorf("message = %q, want URL rejection", got)
			}
		})
	}
}
