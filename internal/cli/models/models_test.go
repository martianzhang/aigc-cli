package models

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
