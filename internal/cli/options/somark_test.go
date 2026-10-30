package options

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestResolveProvider_SomarkNamedProvider(t *testing.T) {
	defer setSharedForTest(&SharedConfig{
		Cfg: &types.Config{
			Providers: map[string]*types.NamedProvider{
				"somark": {
					Type:    types.ProviderSomark,
					APIKey:  "sk-from-config",
					BaseURL: "https://somark.ai/api/v1",
				},
			},
			Defaults: &types.ConfigDefaults{
				OCR: &types.OCRDefaults{Provider: "somark"},
			},
		},
	})()
	p := Shared.ResolveProvider(ProviderNameOCR)
	if p.Type != types.ProviderSomark {
		t.Fatalf("Type = %q, want somark", p.Type)
	}
	if p.APIKey != "sk-from-config" {
		t.Errorf("APIKey = %q, want the config entry's key", p.APIKey)
	}
	if p.BaseURL != "https://somark.ai/api/v1" {
		t.Errorf("BaseURL = %q", p.BaseURL)
	}
}

func TestResolveProvider_SomarkTypeDefaultsToHostedEndpoint(t *testing.T) {
	defer setSharedForTest(&SharedConfig{
		Provider:    "somark",
		ProviderSet: true,
		Cfg: &types.Config{
			Providers: map[string]*types.NamedProvider{
				"somark": {Type: types.ProviderSomark, APIKey: "sk-from-config"},
			},
		},
	})()
	p := Shared.ResolveProvider(ProviderNameOCR)
	if p.BaseURL != provider.SomarkBaseURL {
		t.Errorf("BaseURL = %q, want the hosted SoMark endpoint", p.BaseURL)
	}
	if p.APIKey != "sk-from-config" {
		t.Errorf("APIKey = %q", p.APIKey)
	}
}

func TestResolveProvider_SomarkBaseURLIsDetected(t *testing.T) {
	defer setSharedForTest(&SharedConfig{
		Cfg: &types.Config{
			Providers: map[string]*types.NamedProvider{
				"oxr-local": {BaseURL: "http://localhost:8000/v1"},
				"free-oxr":  {BaseURL: "https://free-oxr.somark.ai/api/v1"},
			},
			Defaults: &types.ConfigDefaults{
				OCR: &types.OCRDefaults{Provider: "free-oxr"},
			},
		},
	})()
	p := Shared.ResolveProvider(ProviderNameOCR)
	if p.ProviderType != provider.Somark {
		t.Errorf("ProviderType = %v, want SoMark via base URL detection", p.ProviderType)
	}
	if !provider.IsDocumentProvider(p) {
		t.Error("a somark.ai base URL must route to document parsing")
	}
}

func TestResolveProvider_UnknownRefStillFallsBack(t *testing.T) {
	defer setSharedForTest(&SharedConfig{
		Provider:    "not-a-provider",
		ProviderSet: true,
	})()
	p := Shared.ResolveProvider(ProviderNameOCR)
	if p.Type == types.ProviderSomark {
		t.Fatalf("unknown provider must not resolve to somark: %+v", p)
	}
	if p.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty (no built-in provider default)", p.BaseURL)
	}
}
