package options

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestRequireAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		cmdName string
		p       *provider.EffectiveProvider
		wantErr bool
	}{
		{"nil provider", "models", nil, false},
		{"key set", "models", &provider.EffectiveProvider{APIKey: "sk-x", BaseURL: "https://api.example.com/v1", Type: types.ProviderOpenAI}, false},
		{"keyless remote", "models", &provider.EffectiveProvider{BaseURL: "https://api.example.com/v1", Type: types.ProviderOpenAI}, true},
		{"ollama", "models", &provider.EffectiveProvider{Type: types.ProviderOllama, BaseURL: "https://ollama.example.com"}, false},
		{"local onnx", "models", &provider.EffectiveProvider{Type: types.ProviderLocal}, false},
		{"localhost", "models", &provider.EffectiveProvider{BaseURL: "http://localhost:1234/v1", Type: types.ProviderOpenAI}, false},
		{"127.0.0.1", "models", &provider.EffectiveProvider{BaseURL: "http://127.0.0.1:1234/v1", Type: types.ProviderOpenAI}, false},
		{"balance keyless remote", "balance", &provider.EffectiveProvider{BaseURL: "https://api.example.com/v1", Type: types.ProviderOpenAI}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := RequireAPIKey(tc.cmdName, tc.p, nil)
			if tc.wantErr && err == nil {
				t.Fatalf("RequireAPIKey() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("RequireAPIKey() = %v, want nil", err)
			}
		})
	}
}

func TestRequireAPIKeyMessage(t *testing.T) {
	t.Run("command name label", func(t *testing.T) {
		err := RequireAPIKey("balance", &provider.EffectiveProvider{BaseURL: "https://api.example.com/v1", Type: types.ProviderOpenAI}, nil)
		if err == nil || !strings.Contains(err.Error(), "no API key for balance") {
			t.Fatalf("RequireAPIKey() = %v, want command name in message", err)
		}
		if !strings.Contains(err.Error(), "configured provider that has an API key") {
			t.Errorf("RequireAPIKey() = %q, want balance-specific note", err)
		}
	})

	t.Run("named provider label", func(t *testing.T) {
		err := RequireAPIKey("balance", &provider.EffectiveProvider{Name: "nokey", BaseURL: "https://api.example.com/v1", Type: types.ProviderOpenAI}, nil)
		if err == nil || !strings.Contains(err.Error(), `no API key for provider "nokey"`) {
			t.Fatalf("RequireAPIKey() = %v, want named provider in message", err)
		}
	})
}

func TestRequireAPIKeyProviderClause(t *testing.T) {
	t.Run("keyed providers sorted, keyless omitted", func(t *testing.T) {
		providers := map[string]*types.NamedProvider{
			"zeta":  {APIKey: "k"},
			"alpha": {APIKey: "k"},
			"blank": {},
		}
		err := RequireAPIKey("models", &provider.EffectiveProvider{BaseURL: "https://api.example.com/v1", Type: types.ProviderOpenAI}, providers)
		if err == nil {
			t.Fatal("RequireAPIKey() = nil, want error")
		}
		msg := err.Error()
		if !strings.Contains(msg, "(configured with keys: alpha, zeta)") {
			t.Errorf("RequireAPIKey() = %q, want sorted keyed clause", msg)
		}
		if strings.Contains(msg, "blank") {
			t.Errorf("RequireAPIKey() = %q, want keyless providers omitted", msg)
		}
	})

	t.Run("fallback to all configured names", func(t *testing.T) {
		providers := map[string]*types.NamedProvider{
			"beta":  {},
			"alpha": {APIKey: ""},
		}
		err := RequireAPIKey("balance", &provider.EffectiveProvider{BaseURL: "https://api.example.com/v1", Type: types.ProviderOpenAI}, providers)
		if err == nil {
			t.Fatal("RequireAPIKey() = nil, want error")
		}
		msg := err.Error()
		if !strings.Contains(msg, "(configured providers: alpha, beta)") {
			t.Errorf("RequireAPIKey() = %q, want all-configured fallback", msg)
		}
		if strings.Contains(msg, "configured with keys") {
			t.Errorf("RequireAPIKey() = %q, want no keyed clause when none has a key", msg)
		}
	})

	t.Run("clause omitted when no providers", func(t *testing.T) {
		err := RequireAPIKey("models", &provider.EffectiveProvider{BaseURL: "https://api.example.com/v1", Type: types.ProviderOpenAI}, nil)
		if err == nil {
			t.Fatal("RequireAPIKey() = nil, want error")
		}
		if strings.Contains(err.Error(), "configured") {
			t.Errorf("RequireAPIKey() = %q, want no provider clause", err)
		}
	})
}
