package image

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// routingClient records which strategy runner touched it. Only the methods the
// runners actually call are implemented; the embedded nil interface panics on
// anything else, which surfaces an unexpected dispatch as a test failure.
type routingClient struct {
	client.APIClient
	baseURL     string
	syncCalls   int
	submitCalls int
	lastModel   string
}

func (c *routingClient) BaseURL() string          { return c.baseURL }
func (c *routingClient) SetTimeout(time.Duration) {}

func (c *routingClient) ImageGenerateSync(req *types.GenerateRequest) (*types.OpenAIImageResponse, error) {
	c.syncCalls++
	c.lastModel = req.Model
	return &types.OpenAIImageResponse{
		Created: 1,
		Data: []types.OpenAIImageData{{
			B64JSON: base64.StdEncoding.EncodeToString([]byte("png")),
		}},
	}, nil
}

func (c *routingClient) Submit(req *types.GenerateRequest) (*types.GenerateResponse, error) {
	c.submitCalls++
	return nil, fmt.Errorf("ModelScope async path used with model %q", req.Model)
}

// TestNewImageDispatchCtx_UsesPerCallProvider locks the root cause of the MCP
// 401: the strategy flags and the ModelScope API key must come from the
// provider that was resolved for this call, not from the configured default.
func TestNewImageDispatchCtx_UsesPerCallProvider(t *testing.T) {
	ofox := &provider.EffectiveProvider{
		Name:         "ofox",
		Type:         types.ProviderOpenAI,
		BaseURL:      "https://api.ofox.io/v1",
		APIKey:       "ofox-key",
		ProviderType: provider.OpenAI,
	}
	ctx := newImageDispatchCtx(ofox, &types.GenerateRequest{Model: "m", Prompt: "p"}, false)
	if ctx.isModelScope {
		t.Error("isModelScope = true for an OpenAI-compatible provider")
	}
	if ctx.isAPIMart || ctx.isOpenRouter || ctx.isAgnes || ctx.isGemini || ctx.isZeekai {
		t.Errorf("strategy flags leaked from another provider: %+v", ctx)
	}
	if ctx.modelScopeKey != "ofox-key" {
		t.Errorf("modelScopeKey = %q, want the per-call provider key", ctx.modelScopeKey)
	}
	if ctx.providerFor() != ofox {
		t.Error("providerFor did not return the per-call provider")
	}

	ms := &provider.EffectiveProvider{
		Name:         "modelscope",
		Type:         types.ProviderOpenAI,
		BaseURL:      "https://api-inference.modelscope.cn/v1",
		APIKey:       "ms-key",
		ProviderType: provider.ModelScope,
	}
	mctx := newImageDispatchCtx(ms, &types.GenerateRequest{Model: "m", Prompt: "p"}, false)
	if !mctx.isModelScope {
		t.Error("isModelScope = false for a ModelScope provider")
	}
	if mctx.modelScopeKey != "ms-key" {
		t.Errorf("modelScopeKey = %q, want the ModelScope key", mctx.modelScopeKey)
	}
}

func TestProviderLabel(t *testing.T) {
	tests := []struct {
		name string
		p    *provider.EffectiveProvider
		want string
	}{
		{"nil", nil, "image"},
		{"named provider keeps its name", &provider.EffectiveProvider{Name: "ofox", ProviderType: provider.OpenAI}, "ofox"},
		{"unnamed modelscope uses its detected type", &provider.EffectiveProvider{ProviderType: provider.ModelScope}, "modelscope"},
		{"unnamed openai uses its detected type", &provider.EffectiveProvider{ProviderType: provider.OpenAI}, "openai"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := providerLabel(tt.p); got != tt.want {
				t.Errorf("providerLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGenerateAndSave_UsesPassedProvider reproduces the reported MCP bug:
// defaults.image.provider is ModelScope while the call selected an
// OpenAI-compatible provider. The request must use the passed provider's
// strategy and model, not ModelScope's async submit path or its model.
func TestGenerateAndSave_UsesPassedProvider(t *testing.T) {
	old := options.Shared
	options.Shared = &options.SharedConfig{
		OutputDir: t.TempDir(),
		Cfg: &types.Config{
			Providers: map[string]*types.NamedProvider{
				"modelscope": {
					Type:    types.ProviderOpenAI,
					APIKey:  "ms-key",
					BaseURL: "https://api-inference.modelscope.cn/v1",
				},
			},
			Defaults: &types.ConfigDefaults{
				Image: &types.ImageDefaults{
					Provider: "modelscope",
					Model:    "Qwen/Qwen-Image-2.1-Turbo",
				},
			},
		},
	}
	t.Cleanup(func() { options.Shared = old })

	ofox := &provider.EffectiveProvider{
		Name:         "ofox",
		Type:         types.ProviderOpenAI,
		BaseURL:      "https://api.ofox.io/v1",
		APIKey:       "ofox-key",
		ProviderType: provider.OpenAI,
	}
	rec := &routingClient{baseURL: ofox.BaseURL}

	saved, err := GenerateAndSave(rec, ofox, &types.GenerateRequest{
		Model:        "google/gemini-nano-banana-2.1",
		Prompt:       "p",
		Size:         "1:1",
		Quality:      "auto",
		OutputFormat: "png",
	})
	if err != nil {
		t.Fatalf("GenerateAndSave() error = %v", err)
	}
	if rec.submitCalls != 0 {
		t.Fatalf("ModelScope async submit was used %d time(s); the per-call provider must drive dispatch", rec.submitCalls)
	}
	if rec.syncCalls != 1 {
		t.Fatalf("sync runner calls = %d, want 1", rec.syncCalls)
	}
	if rec.lastModel != "google/gemini-nano-banana-2.1" {
		t.Errorf("model sent = %q, want the per-call provider model (defaults.image.model must not be forced)", rec.lastModel)
	}
	if len(saved) == 0 {
		t.Error("no images were saved")
	}
}

// TestGenerateAndSave_DefaultProviderKeepsConfigModel guards the documented
// behavior for calls that do not switch providers: defaults.image.model still
// wins over the model supplied by the agent.
func TestGenerateAndSave_DefaultProviderKeepsConfigModel(t *testing.T) {
	old := options.Shared
	options.Shared = &options.SharedConfig{
		OutputDir: t.TempDir(),
		Cfg: &types.Config{
			Providers: map[string]*types.NamedProvider{
				"ofox": {
					Type:    types.ProviderOpenAI,
					APIKey:  "ofox-key",
					BaseURL: "https://api.ofox.io/v1",
				},
			},
			Defaults: &types.ConfigDefaults{
				Image: &types.ImageDefaults{Provider: "ofox", Model: "ofox-default-model"},
			},
		},
	}
	t.Cleanup(func() { options.Shared = old })

	configured := options.Shared.ResolveProvider(options.ProviderNameImage)
	rec := &routingClient{baseURL: configured.BaseURL}

	if _, err := GenerateAndSave(rec, configured, &types.GenerateRequest{
		Model:        "agent-model",
		Prompt:       "p",
		Size:         "1:1",
		Quality:      "auto",
		OutputFormat: "png",
	}); err != nil {
		t.Fatalf("GenerateAndSave() error = %v", err)
	}
	if rec.lastModel != "ofox-default-model" {
		t.Errorf("model sent = %q, want defaults.image.model", rec.lastModel)
	}
}
