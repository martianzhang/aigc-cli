package music

import (
	"testing"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// recordingMusicClient captures the request the OpenRouter runner builds.
type recordingMusicClient struct {
	client.APIClient
	baseURL string
	lastReq *types.OpenRouterMusicRequest
}

func (c *recordingMusicClient) BaseURL() string          { return c.baseURL }
func (c *recordingMusicClient) SetTimeout(time.Duration) {}

func (c *recordingMusicClient) OpenRouterMusicGenerate(req *types.OpenRouterMusicRequest) ([]byte, string, error) {
	c.lastReq = req
	return []byte("audio"), "", nil
}

func musicRoutingSharedConfig(defaultProvider, defaultModel string) *options.SharedConfig {
	return &options.SharedConfig{
		OutputDir: "", // set by each test
		Cfg: &types.Config{
			Providers: map[string]*types.NamedProvider{
				"apimart": {
					Type:    types.ProviderOpenAI,
					APIKey:  "apimart-key",
					BaseURL: "https://api.apimart.ai",
				},
				"openrouter": {
					Type:    types.ProviderOpenAI,
					APIKey:  "openrouter-key",
					BaseURL: "https://openrouter.ai/api/v1",
				},
			},
			Defaults: &types.ConfigDefaults{
				Music: &types.MusicDefaults{Provider: defaultProvider, Model: defaultModel},
			},
		},
	}
}

// TestGenerateAndSave_ExplicitProviderSkipsDefaultModel is the music analogue
// of the image provider bug: when the caller selects a provider other than
// defaults.music.provider, its default model (e.g. suno) must not be sent to
// that provider.
func TestGenerateAndSave_ExplicitProviderSkipsDefaultModel(t *testing.T) {
	old := options.Shared
	shared := musicRoutingSharedConfig("apimart", "suno")
	shared.OutputDir = t.TempDir()
	options.Shared = shared
	t.Cleanup(func() { options.Shared = old })

	openrouter := &provider.EffectiveProvider{
		Name:         "openrouter",
		Type:         types.ProviderOpenAI,
		BaseURL:      "https://openrouter.ai/api/v1",
		APIKey:       "openrouter-key",
		ProviderType: provider.OpenRouter,
	}
	rec := &recordingMusicClient{baseURL: openrouter.BaseURL}

	if _, _, err := GenerateAndSave(rec, openrouter, &types.MusicGenerateRequest{
		Model:  "google/lyria-3-pro-preview",
		Prompt: "p",
	}); err != nil {
		t.Fatalf("GenerateAndSave() error = %v", err)
	}
	if rec.lastReq == nil {
		t.Fatal("OpenRouter runner was not called")
	}
	if rec.lastReq.Model != "google/lyria-3-pro-preview" {
		t.Errorf("model = %q, want the caller's model (defaults.music.model must not be forced)", rec.lastReq.Model)
	}
}

// TestGenerateAndSave_DefaultProviderKeepsConfigModel guards the documented
// behavior when no provider switch happens.
func TestGenerateAndSave_DefaultProviderKeepsConfigModel(t *testing.T) {
	old := options.Shared
	shared := musicRoutingSharedConfig("openrouter", "suno")
	shared.OutputDir = t.TempDir()
	options.Shared = shared
	t.Cleanup(func() { options.Shared = old })

	configured := options.Shared.ResolveProvider(options.ProviderNameMusic)
	rec := &recordingMusicClient{baseURL: configured.BaseURL}

	if _, _, err := GenerateAndSave(rec, configured, &types.MusicGenerateRequest{
		Prompt: "p",
	}); err != nil {
		t.Fatalf("GenerateAndSave() error = %v", err)
	}
	if rec.lastReq == nil {
		t.Fatal("OpenRouter runner was not called")
	}
	if rec.lastReq.Model != "suno" {
		t.Errorf("model = %q, want defaults.music.model", rec.lastReq.Model)
	}
}
