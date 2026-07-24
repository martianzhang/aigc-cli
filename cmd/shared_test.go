package cmd

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestResolveProvider_CLIOverride(t *testing.T) {
	defer SetSharedForTest(&SharedConfig{
		APIKey:     "cli-key",
		APIBase:    "https://cli.com/v1",
		HTTPProxy:  "http://proxy:8080",
		APIKeySet:  true,
		APIBaseSet: true,
		Cfg: &types.Config{
			APIKey:  "cfg-key",
			BaseURL: "https://cfg.com/v1",
		},
	})()

	ep := shared.ResolveProvider("image")
	if ep.APIKey != "cli-key" {
		t.Errorf("APIKey = %q, want %q", ep.APIKey, "cli-key")
	}
	if ep.BaseURL != "https://cli.com/v1" {
		t.Errorf("BaseURL = %q, want %q", ep.BaseURL, "https://cli.com/v1")
	}
}

func TestResolveProvider_ProviderFlag(t *testing.T) {
	defer SetSharedForTest(&SharedConfig{
		Provider:    "my-provider",
		ProviderSet: true,
		Cfg: &types.Config{
			APIKey:  "cfg-key",
			BaseURL: "https://cfg.com/v1",
			Providers: map[string]*types.NamedProvider{
				"my-provider": {
					BaseURL: "https://my-provider.com/v1",
					APIKey:  "my-key",
				},
			},
		},
	})()

	ep := shared.ResolveProvider("image")
	if ep.Name != "my-provider" {
		t.Errorf("Name = %q, want %q", ep.Name, "my-provider")
	}
	if ep.APIKey != "my-key" {
		t.Errorf("APIKey = %q, want %q", ep.APIKey, "my-key")
	}
	if ep.BaseURL != "https://my-provider.com/v1" {
		t.Errorf("BaseURL = %q, want %q", ep.BaseURL, "https://my-provider.com/v1")
	}
}

func TestResolveProvider_DefaultsRef(t *testing.T) {
	defer SetSharedForTest(&SharedConfig{
		Cfg: &types.Config{
			APIKey:  "cfg-key",
			BaseURL: "https://cfg.com/v1",
			Defaults: &types.ConfigDefaults{
				Image: &types.ImageDefaults{
					Provider: "img-provider",
				},
			},
			Providers: map[string]*types.NamedProvider{
				"img-provider": {
					BaseURL: "https://img.com/v1",
					APIKey:  "img-key",
				},
			},
		},
	})()

	ep := shared.ResolveProvider("image")
	if ep.Name != "img-provider" {
		t.Errorf("Name = %q, want %q", ep.Name, "img-provider")
	}
	if ep.APIKey != "img-key" {
		t.Errorf("APIKey = %q, want %q", ep.APIKey, "img-key")
	}
}

func TestResolveProvider_DefaultsWithoutRef(t *testing.T) {
	defer SetSharedForTest(&SharedConfig{
		Cfg: &types.Config{
			APIKey:  "cfg-key",
			BaseURL: "https://cfg.com/v1",
			Defaults: &types.ConfigDefaults{
				Image: &types.ImageDefaults{
					Model: "dall-e-3",
				},
			},
		},
	})()

	ep := shared.ResolveProvider("image")
	if ep.Name != "" {
		t.Errorf("Name = %q, want empty (global)", ep.Name)
	}
	if ep.APIKey != "cfg-key" {
		t.Errorf("APIKey = %q, want %q", ep.APIKey, "cfg-key")
	}
	if ep.BaseURL != "https://cfg.com/v1" {
		t.Errorf("BaseURL = %q, want %q", ep.BaseURL, "https://cfg.com/v1")
	}
}

func TestResolveProvider_GlobalFallback(t *testing.T) {
	defer SetSharedForTest(&SharedConfig{
		Cfg: &types.Config{
			BaseURL: "https://global.com/v1",
		},
	})()

	ep := shared.ResolveProvider("ocr")
	if ep.BaseURL != "https://global.com/v1" {
		t.Errorf("BaseURL = %q, want %q", ep.BaseURL, "https://global.com/v1")
	}
	if ep.Type != types.ProviderOpenAI {
		t.Errorf("Type = %q, want %q", ep.Type, types.ProviderOpenAI)
	}
}

func TestResolveProvider_DefaultsModelMerge(t *testing.T) {
	defer SetSharedForTest(&SharedConfig{
		Cfg: &types.Config{
			BaseURL: "https://api.com/v1",
			Providers: map[string]*types.NamedProvider{
				"my-prov": {
					BaseURL: "https://prov.com/v1",
				},
			},
			Defaults: &types.ConfigDefaults{
				Image: &types.ImageDefaults{
					Provider: "my-prov",
					Model:    "default-model",
				},
			},
		},
	})()

	ep := shared.ResolveProvider("image")
	// Named provider has no model, defaults model should merge
	if ep.Model != "default-model" {
		t.Errorf("Model = %q, want %q", ep.Model, "default-model")
	}
}

func TestResolveProvider_ProviderFlagOverridesDefaults(t *testing.T) {
	defer SetSharedForTest(&SharedConfig{
		Provider:    "override-prov",
		ProviderSet: true,
		Cfg: &types.Config{
			BaseURL: "https://cfg.com/v1",
			Providers: map[string]*types.NamedProvider{
				"override-prov": {BaseURL: "https://override.com/v1", APIKey: "override-key"},
				"defaults-prov": {BaseURL: "https://defaults.com/v1", APIKey: "defaults-key"},
			},
			Defaults: &types.ConfigDefaults{
				Image: &types.ImageDefaults{
					Provider: "defaults-prov",
				},
			},
		},
	})()

	ep := shared.ResolveProvider("image")
	if ep.Name != "override-prov" {
		t.Errorf("Name = %q, want %q (--provider should win)", ep.Name, "override-prov")
	}
}

func TestResolveProvider_LocalBuiltin(t *testing.T) {
	defer SetSharedForTest(&SharedConfig{
		Provider:    ProviderNameLocal,
		ProviderSet: true,
	})()

	ep := shared.ResolveProvider("ocr")
	if ep.Type != types.ProviderLocal {
		t.Errorf("Type = %q, want %q", ep.Type, types.ProviderLocal)
	}
	if ep.Name != ProviderNameLocal {
		t.Errorf("Name = %q, want %q", ep.Name, ProviderNameLocal)
	}
}

// lookupCmdProviderAndModel tests
func TestLookupCmdProviderAndModel(t *testing.T) {
	defaults := &types.ConfigDefaults{
		Image:      &types.ImageDefaults{Provider: "img-prov", Model: "img-model"},
		Video:      &types.VideoDefaults{Provider: "vid-prov"},
		Chat:       &types.ChatDefaults{Provider: "chat-prov"},
		Audio:      &types.AudioDefaults{Provider: "audio-prov"},
		Midjourney: &types.MidjourneyDefaults{Provider: "mj-prov"},
		OCR:        &types.OCRDefaults{Provider: "ocr-prov", Model: "ocr-model"},
		Vision:     &types.VisionDefaults{Provider: "vis-prov"},
	}

	tests := []struct {
		cmd          string
		wantProvider string
		wantModel    string
	}{
		{"image", "img-prov", "img-model"},
		{"video", "vid-prov", ""},
		{"chat", "chat-prov", ""},
		{"audio", "audio-prov", ""},
		{"midjourney", "mj-prov", ""},
		{"ocr", "ocr-prov", "ocr-model"},
		{"vision", "vis-prov", ""},
		{"unknown", "", ""},
	}

	for _, tt := range tests {
		p, m := lookupCmdProviderAndModel(tt.cmd, defaults)
		if p != tt.wantProvider {
			t.Errorf("lookupCmdProviderAndModel(%q) provider = %q, want %q", tt.cmd, p, tt.wantProvider)
		}
		if m != tt.wantModel {
			t.Errorf("lookupCmdProviderAndModel(%q) model = %q, want %q", tt.cmd, m, tt.wantModel)
		}
	}
}

func TestLookupCmdProviderAndModel_nil(t *testing.T) {
	p, m := lookupCmdProviderAndModel("image", nil)
	if p != "" || m != "" {
		t.Errorf("with nil defaults: got (%q, %q), want empty", p, m)
	}
}

// firstNonEmpty test (from shared.go helpers)
func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		vals []string
		want string
	}{
		{[]string{"a", "b"}, "a"},
		{[]string{"", "b"}, "b"},
		{[]string{"", ""}, ""},
		{nil, ""},
	}
	for _, tt := range tests {
		got := firstNonEmpty(tt.vals...)
		if got != tt.want {
			t.Errorf("firstNonEmpty(%v) = %q, want %q", tt.vals, got, tt.want)
		}
	}
}

// firstNonEmpty helper (same as internal/provider/config.go's version)
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
