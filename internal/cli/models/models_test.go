package models

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestFormatPrice_perGeneration(t *testing.T) {
	m := types.MarketplaceModel{
		Pricing: types.MarketplacePricing{
			HasPrice:      true,
			StartingPrice: 0.006,
			PriceUnit:     "/次",
			BillingType:   "per_generation",
		},
	}
	if got, want := m.FormatPrice(), "$0.0060/次"; got != want {
		t.Errorf("FormatPrice() = %q, want %q", got, want)
	}
}

func TestFormatPrice_perToken(t *testing.T) {
	m := types.MarketplaceModel{
		Pricing: types.MarketplacePricing{
			HasPrice:      true,
			StartingPrice: 4.0,
			PriceUnit:     "/1K tokens",
			BillingType:   "per_token",
		},
	}
	if got, want := m.FormatPrice(), "$4.0000/1K tokens"; got != want {
		t.Errorf("FormatPrice() = %q, want %q", got, want)
	}
}

func TestFormatPrice_noPrice(t *testing.T) {
	m := types.MarketplaceModel{Pricing: types.MarketplacePricing{HasPrice: false}}
	if got, want := m.FormatPrice(), "—"; got != want {
		t.Errorf("FormatPrice() = %q, want %q", got, want)
	}
}

func TestFormatPrice_emptyUnit(t *testing.T) {
	m := types.MarketplaceModel{
		Pricing: types.MarketplacePricing{
			HasPrice:      true,
			StartingPrice: 0.01,
			PriceUnit:     "",
			BillingType:   "per_generation",
		},
	}
	if got, want := m.FormatPrice(), "$0.0100/次"; got != want {
		t.Errorf("FormatPrice() = %q, want %q", got, want)
	}
}

func TestMainDomain(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "https://apimart.ai"},
		{"https://api.apimart.ai", "https://apimart.ai"},
		{"https://api.apimart.ai/v1", "https://apimart.ai"},
		{"https://custom.api.com", "https://custom.api.com"},
	}
	for _, tc := range tests {
		if got := mainDomain(tc.in); got != tc.want {
			t.Errorf("mainDomain(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsHTML(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want bool
	}{
		{"html", []byte("<html><body>blocked</body></html>"), true},
		{"html leading whitespace", []byte("  \n\t<html>"), true},
		{"json object", []byte(`{"data":[]}`), false},
		{"json array", []byte(`[{"id":"test"}]`), false},
		{"empty", nil, false},
		{"plain text", []byte("OK"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isHTML(tc.in); got != tc.want {
				t.Errorf("isHTML(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestOpenRouterModelsLocalServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"data": [
				{
					"id": "test/model",
					"name": "Test Model",
					"architecture": {
						"input_modalities": ["text"],
						"output_modalities": ["image"]
					},
					"supported_parameters": {
						"n": {"type": "range", "min": 1, "max": 4},
						"seed": {"type": "boolean"}
					},
					"supports_streaming": false
				}
			]
		}`))
	}))
	defer srv.Close()

	orig := d
	d = Deps{APIBase: srv.URL}
	defer func() { d = orig }()

	if err := runModelsOpenRouterDiscovery("image"); err != nil {
		t.Fatalf("runModelsOpenRouterDiscovery() error = %v", err)
	}
}

func TestRequireAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		p       *provider.EffectiveProvider
		wantErr bool
	}{
		{"empty key remote", &provider.EffectiveProvider{BaseURL: "https://api.example.com/v1", Type: types.ProviderOpenAI}, true},
		{"key set", &provider.EffectiveProvider{APIKey: "sk-x", BaseURL: "https://api.example.com/v1", Type: types.ProviderOpenAI}, false},
		{"nil provider", nil, false},
		{"ollama", &provider.EffectiveProvider{Type: types.ProviderOllama, BaseURL: "https://ollama.example.com"}, false},
		{"local onnx", &provider.EffectiveProvider{Type: types.ProviderLocal}, false},
		{"localhost exempt", &provider.EffectiveProvider{BaseURL: "http://localhost:1234/v1", Type: types.ProviderOpenAI}, false},
		{"127.0.0.1 exempt", &provider.EffectiveProvider{BaseURL: "http://127.0.0.1:1234/v1", Type: types.ProviderOpenAI}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := options.RequireAPIKey("models", tc.p, nil)
			if tc.wantErr && err == nil {
				t.Fatalf("RequireAPIKey() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("RequireAPIKey() = %v, want nil", err)
			}
		})
	}
}

func TestRequireAPIKeyErrorListsProviders(t *testing.T) {
	orig := d
	d = Deps{Providers: map[string]*types.NamedProvider{
		"zeta":  {APIKey: "k"},
		"alpha": {APIKey: "k"},
		"blank": {},
	}}
	defer func() { d = orig }()

	err := options.RequireAPIKey("models", &provider.EffectiveProvider{BaseURL: "https://api.example.com/v1", Type: types.ProviderOpenAI}, d.Providers)
	if err == nil {
		t.Fatal("RequireAPIKey() = nil, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "(configured with keys: alpha, zeta)") {
		t.Errorf("error %q should list providers alphabetically and omit blank keys", msg)
	}
	if strings.Contains(msg, "blank") {
		t.Errorf("error %q should omit providers without keys", msg)
	}
}

func TestRunModelsOpenAINoRequestWithoutKey(t *testing.T) {
	orig := d
	d = Deps{}
	defer func() { d = orig }()

	p := &provider.EffectiveProvider{BaseURL: "http://models-no-key.invalid/v1", Type: types.ProviderOpenAI}
	err := runModelsOpenAI(p)
	if err == nil {
		t.Fatal("runModelsOpenAI() = nil, want missing-key error")
	}
	if !strings.Contains(err.Error(), "no API key") {
		t.Errorf("runModelsOpenAI() error = %q, want it to report the missing key", err)
	}
	if strings.Contains(err.Error(), "failed to list models") {
		t.Errorf("runModelsOpenAI() error = %q, want no HTTP attempt before the guard", err)
	}
}

func TestRunModelsDetailNoRequestWithoutKey(t *testing.T) {
	orig := d
	d = Deps{}
	defer func() { d = orig }()

	p := &provider.EffectiveProvider{BaseURL: "http://models-no-key.invalid/v1", Type: types.ProviderOpenAI}
	err := runModelsDetail("m", p)
	if err == nil {
		t.Fatal("runModelsDetail() = nil, want missing-key error")
	}
	if !strings.Contains(err.Error(), "no API key") {
		t.Errorf("runModelsDetail() error = %q, want it to report the missing key", err)
	}
	if strings.Contains(err.Error(), "failed to get model") {
		t.Errorf("runModelsDetail() error = %q, want no HTTP attempt before the guard", err)
	}
}
