package mcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// imageRoutingClient records the strategy the shared image runner chose.
// Only the methods it calls are implemented; any other call panics through the
// embedded nil interface, making an unexpected dispatch obvious.
type imageRoutingClient struct {
	client.APIClient
	baseURL     string
	syncCalls   int
	submitCalls int
	lastModel   string
}

func (c *imageRoutingClient) BaseURL() string          { return c.baseURL }
func (c *imageRoutingClient) SetTimeout(time.Duration) {}

func (c *imageRoutingClient) ImageGenerateSync(req *types.GenerateRequest) (*types.OpenAIImageResponse, error) {
	c.syncCalls++
	c.lastModel = req.Model
	return &types.OpenAIImageResponse{
		Created: 1,
		Data: []types.OpenAIImageData{{
			B64JSON: base64.StdEncoding.EncodeToString([]byte("png")),
		}},
	}, nil
}

func (c *imageRoutingClient) Submit(req *types.GenerateRequest) (*types.GenerateResponse, error) {
	c.submitCalls++
	return nil, fmt.Errorf("ModelScope async path used with model %q", req.Model)
}

// TestGenerateImageHandler_PerCallProviderRouting is the end-to-end regression
// for the reported MCP 401: defaults.image.provider is ModelScope, but the call
// passes provider=ofox. The handler must pass that provider into the shared
// runner so the OpenAI sync strategy is used with the caller's model, instead
// of ModelScope's async submit path (which previously sent the ModelScope key
// to Ofox and produced "Invalid or expired API key").
func TestGenerateImageHandler_PerCallProviderRouting(t *testing.T) {
	oldShared := options.Shared
	sharedCfg := &types.Config{
		Providers: map[string]*types.NamedProvider{
			"ofox": {
				Type:    types.ProviderOpenAI,
				APIKey:  "ofox-key",
				BaseURL: "https://api.ofox.io/v1",
			},
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
	}
	options.Shared = &options.SharedConfig{OutputDir: t.TempDir(), Cfg: sharedCfg}
	t.Cleanup(func() { options.Shared = oldShared })

	oldClient := imageAPIClient
	rec := &imageRoutingClient{baseURL: "https://api.ofox.io/v1"}
	imageAPIClient = func(*provider.EffectiveProvider) client.APIClient { return rec }
	t.Cleanup(func() { imageAPIClient = oldClient })

	cfg := &Config{
		Output:   options.Shared.OutputDir,
		Defaults: sharedCfg.Defaults,
		Providers: map[string]*types.NamedProvider{
			"ofox": {
				Type:    types.ProviderOpenAI,
				APIKey:  "ofox-key",
				BaseURL: "https://api.ofox.io/v1",
			},
			"modelscope": {
				Type:    types.ProviderOpenAI,
				APIKey:  "ms-key",
				BaseURL: "https://api-inference.modelscope.cn/v1",
			},
		},
		CmdProviders: map[string]*provider.EffectiveProvider{
			"image": {
				Name:         "modelscope",
				Type:         types.ProviderOpenAI,
				BaseURL:      "https://api-inference.modelscope.cn/v1",
				APIKey:       "ms-key",
				ProviderType: provider.ModelScope,
			},
		},
		Generate: GenerateDeps{
			Image: func(c client.APIClient, p *provider.EffectiveProvider, req *types.GenerateRequest) ([]string, error) {
				if p.Name != "ofox" {
					t.Errorf("shared runner received provider %q, want ofox", p.Name)
				}
				if _, err := c.ImageGenerateSync(req); err != nil {
					return nil, err
				}
				return []string{}, nil
			},
		},
	}

	res, err := generateImageHandler(cfg)(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{
			"prompt":   "p",
			"provider": "ofox",
			"model":    "google/gemini-nano-banana-2.1",
		}},
	})
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if res.IsError {
		t.Fatalf("handler error: %s", toolResultText(t, res))
	}

	if rec.submitCalls != 0 {
		t.Fatalf("ModelScope async submit was used %d time(s); provider=ofox must select the sync strategy", rec.submitCalls)
	}
	if rec.syncCalls != 1 {
		t.Fatalf("sync runner calls = %d, want 1", rec.syncCalls)
	}
	if rec.lastModel != "google/gemini-nano-banana-2.1" {
		t.Errorf("model sent = %q, want the per-call provider model", rec.lastModel)
	}
}
