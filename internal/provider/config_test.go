package provider

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestResolveCmdProvider_CLIOverride(t *testing.T) {
	named := map[string]*types.NamedProvider{
		"my-provider": {BaseURL: "https://my-provider.com/v1", APIKey: "key-named"},
	}
	global := &GlobalConfig{APIKey: "key-global", BaseURL: "https://global.com/v1"}

	t.Run("api key override uses CLI values, base falls back to global", func(t *testing.T) {
		cli := &CLIOverride{APIKey: "key-cli"}
		ep := ResolveCmdProvider(cli, "my-provider", named, global)
		if ep.APIKey != "key-cli" {
			t.Errorf("APIKey = %q, want %q", ep.APIKey, "key-cli")
		}
		if ep.BaseURL != "https://global.com/v1" {
			t.Errorf("BaseURL = %q, want %q", ep.BaseURL, "https://global.com/v1")
		}
	})

	t.Run("base url override uses CLI values", func(t *testing.T) {
		cli := &CLIOverride{BaseURL: "https://cli.com/v1"}
		ep := ResolveCmdProvider(cli, "my-provider", named, global)
		if ep.BaseURL != "https://cli.com/v1" {
			t.Errorf("BaseURL = %q, want %q", ep.BaseURL, "https://cli.com/v1")
		}
	})
}

func TestResolveCmdProvider_NamedProvider(t *testing.T) {
	global := &GlobalConfig{APIKey: "key-global", BaseURL: "https://global.com/v1"}

	t.Run("found named provider", func(t *testing.T) {
		named := map[string]*types.NamedProvider{
			"my-ai": {BaseURL: "https://my-ai.com/v1", APIKey: "key-my"},
		}
		ep := ResolveCmdProvider(nil, "my-ai", named, global)
		if ep.Name != "my-ai" {
			t.Errorf("Name = %q, want %q", ep.Name, "my-ai")
		}
		if ep.APIKey != "key-my" {
			t.Errorf("APIKey = %q, want %q", ep.APIKey, "key-my")
		}
		if ep.BaseURL != "https://my-ai.com/v1" {
			t.Errorf("BaseURL = %q, want %q", ep.BaseURL, "https://my-ai.com/v1")
		}
	})

	t.Run("named provider empty fields inherit global", func(t *testing.T) {
		named := map[string]*types.NamedProvider{
			"partial": {BaseURL: "https://partial.com/v1"}, // no APIKey
		}
		ep := ResolveCmdProvider(nil, "partial", named, global)
		if ep.APIKey != "key-global" {
			t.Errorf("APIKey = %q, want %q", ep.APIKey, "key-global")
		}
	})

	t.Run("named provider explicit type", func(t *testing.T) {
		named := map[string]*types.NamedProvider{
			"ollama-prov": {Type: types.ProviderOllama, BaseURL: "http://localhost:11434"},
		}
		ep := ResolveCmdProvider(nil, "ollama-prov", named, global)
		if ep.Type != types.ProviderOllama {
			t.Errorf("Type = %q, want %q", ep.Type, types.ProviderOllama)
		}
	})

	t.Run("named provider not found fallback to global", func(t *testing.T) {
		named := map[string]*types.NamedProvider{}
		ep := ResolveCmdProvider(nil, "nonexistent", named, global)
		if ep.Name != "" {
			t.Errorf("Name = %q, want empty", ep.Name)
		}
		if ep.APIKey != "key-global" {
			t.Errorf("APIKey = %q, want %q", ep.APIKey, "key-global")
		}
		if ep.BaseURL != "https://global.com/v1" {
			t.Errorf("BaseURL = %q, want %q", ep.BaseURL, "https://global.com/v1")
		}
	})
}

func TestResolveCmdProvider_GlobalFallback(t *testing.T) {
	t.Run("empty ref uses global", func(t *testing.T) {
		global := &GlobalConfig{APIKey: "key-g", BaseURL: "https://g.com/v1"}
		ep := ResolveCmdProvider(nil, "", nil, global)
		if ep.APIKey != "key-g" {
			t.Errorf("APIKey = %q, want %q", ep.APIKey, "key-g")
		}
		if ep.BaseURL != "https://g.com/v1" {
			t.Errorf("BaseURL = %q, want %q", ep.BaseURL, "https://g.com/v1")
		}
	})

	t.Run("empty global base url uses default", func(t *testing.T) {
		global := &GlobalConfig{}
		ep := ResolveCmdProvider(nil, "", nil, global)
		if ep.BaseURL != defaultBaseURL {
			t.Errorf("BaseURL = %q, want %q", ep.BaseURL, defaultBaseURL)
		}
	})

	t.Run("nil named providers with ref falls back", func(t *testing.T) {
		global := &GlobalConfig{APIKey: "key-g", BaseURL: "https://g.com/v1"}
		ep := ResolveCmdProvider(nil, "anything", nil, global)
		if ep.Name != "" {
			t.Errorf("Name = %q, want empty (global fallback)", ep.Name)
		}
		if ep.APIKey != "key-g" {
			t.Errorf("APIKey = %q, want %q", ep.APIKey, "key-g")
		}
	})

	t.Run("default type is openai", func(t *testing.T) {
		global := &GlobalConfig{BaseURL: "https://api.openai.com/v1"}
		ep := ResolveCmdProvider(nil, "", nil, global)
		if ep.Type != types.ProviderOpenAI {
			t.Errorf("Type = %q, want %q", ep.Type, types.ProviderOpenAI)
		}
	})
}

func TestResolveCmdProvider_ProviderTypeDetection(t *testing.T) {
	t.Run("openai url detects openai type", func(t *testing.T) {
		global := &GlobalConfig{BaseURL: "https://api.openai.com/v1"}
		ep := ResolveCmdProvider(nil, "", nil, global)
		if ep.ProviderType != OpenAI {
			t.Errorf("ProviderType = %v, want OpenAI", ep.ProviderType)
		}
	})

	t.Run("apimart url detects apimart type", func(t *testing.T) {
		global := &GlobalConfig{BaseURL: "https://api.apimart.ai"}
		ep := ResolveCmdProvider(nil, "", nil, global)
		if ep.ProviderType != APIMart {
			t.Errorf("ProviderType = %v, want APIMart", ep.ProviderType)
		}
	})

	t.Run("openrouter url detects openrouter type", func(t *testing.T) {
		global := &GlobalConfig{BaseURL: "https://openrouter.ai/api/v1"}
		ep := ResolveCmdProvider(nil, "", nil, global)
		if ep.ProviderType != OpenRouter {
			t.Errorf("ProviderType = %v, want OpenRouter", ep.ProviderType)
		}
	})
}

func TestValidateProviderRef(t *testing.T) {
	providers := map[string]*types.NamedProvider{
		"valid": {BaseURL: "https://valid.com"},
	}

	t.Run("empty ref returns nil", func(t *testing.T) {
		if err := ValidateProviderRef("", providers); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("existing ref returns nil", func(t *testing.T) {
		if err := ValidateProviderRef("valid", providers); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing ref returns error", func(t *testing.T) {
		if err := ValidateProviderRef("invalid", providers); err == nil {
			t.Error("expected error for missing ref")
		}
	})

	t.Run("nil map with non-empty ref returns error", func(t *testing.T) {
		if err := ValidateProviderRef("anything", nil); err == nil {
			t.Error("expected error for nil map")
		}
	})
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{[]string{"a", "b", "c"}, "a"},
		{[]string{"", "b", "c"}, "b"},
		{[]string{"", "", "c"}, "c"},
		{[]string{"", "", ""}, ""},
		{[]string{}, ""},
	}
	for _, tt := range tests {
		got := firstNonEmpty(tt.input...)
		if got != tt.want {
			t.Errorf("firstNonEmpty(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
