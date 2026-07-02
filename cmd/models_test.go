package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/martianzhang/apimart-cli/internal/types"
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
	got := formatPrice(m)
	want := "$0.0060/次"
	if got != want {
		t.Errorf("formatPrice() = %q, want %q", got, want)
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
	got := formatPrice(m)
	want := "$4.0000/1K tokens"
	if got != want {
		t.Errorf("formatPrice() = %q, want %q", got, want)
	}
}

func TestFormatPrice_noPrice(t *testing.T) {
	m := types.MarketplaceModel{
		Pricing: types.MarketplacePricing{
			HasPrice: false,
		},
	}
	got := formatPrice(m)
	want := "—"
	if got != want {
		t.Errorf("formatPrice() = %q, want %q", got, want)
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
	got := formatPrice(m)
	want := "$0.0100/次"
	if got != want {
		t.Errorf("formatPrice() = %q, want %q", got, want)
	}
}

func TestMainDomain_default(t *testing.T) {
	got := mainDomain("")
	want := "https://apimart.ai"
	if got != want {
		t.Errorf("mainDomain(%q) = %q, want %q", "", got, want)
	}
}

func TestMainDomain_apiSubdomain(t *testing.T) {
	got := mainDomain("https://api.apimart.ai")
	want := "https://apimart.ai"
	if got != want {
		t.Errorf("mainDomain(%q) = %q, want %q", "https://api.apimart.ai", got, want)
	}
}

func TestMainDomain_withV1(t *testing.T) {
	got := mainDomain("https://api.apimart.ai/v1")
	want := "https://apimart.ai"
	if got != want {
		t.Errorf("mainDomain(%q) = %q, want %q", "https://api.apimart.ai/v1", got, want)
	}
}

func TestMainDomain_customDomain(t *testing.T) {
	got := mainDomain("https://custom.api.com")
	want := "https://custom.api.com"
	if got != want {
		t.Errorf("mainDomain(%q) = %q, want %q", "https://custom.api.com", got, want)
	}
}

// ---------------------------------------------------------------------------
// isHTML tests
// ---------------------------------------------------------------------------

func TestIsHTML_html(t *testing.T) {
	if !isHTML([]byte("<html><body>blocked</body></html>")) {
		t.Error("isHTML should detect HTML")
	}
}

func TestIsHTML_htmlLeadingWhitespace(t *testing.T) {
	if !isHTML([]byte("  \n\t<html>")) {
		t.Error("isHTML should detect HTML with leading whitespace")
	}
}

func TestIsHTML_jsonObject(t *testing.T) {
	if isHTML([]byte(`{"data":[]}`)) {
		t.Error("isHTML should NOT detect JSON as HTML")
	}
}

func TestIsHTML_jsonArray(t *testing.T) {
	if isHTML([]byte(`[{"id":"test"}]`)) {
		t.Error("isHTML should NOT detect JSON array as HTML")
	}
}

func TestIsHTML_empty(t *testing.T) {
	if isHTML(nil) || isHTML([]byte{}) {
		t.Error("isHTML should return false for empty input")
	}
}

func TestIsHTML_plainText(t *testing.T) {
	if isHTML([]byte("OK")) {
		t.Error("isHTML should return false for plain text")
	}
}

// ---------------------------------------------------------------------------
// OpenRouter model discovery integration test (no API key required)
// ---------------------------------------------------------------------------

func TestOpenRouterModelsImageDiscovery(t *testing.T) {
	// This test calls the real OpenRouter API (free, no auth required)
	resp, err := http.Get("https://openrouter.ai/api/v1/images/models")
	if err != nil {
		t.Fatalf("failed to call OpenRouter image models API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("API returned status %d", resp.StatusCode)
	}

	var list types.OpenRouterMediaModelList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(list.Data) == 0 {
		t.Fatal("expected at least 1 image model from OpenRouter")
	}

	// Check basic structure of first model
	m := list.Data[0]
	if m.ID == "" {
		t.Error("first model should have an ID")
	}
	if m.Architecture == nil {
		t.Error("first model should have architecture info")
	} else {
		if len(m.Architecture.OutputModalities) == 0 {
			t.Error("model should specify output modalities")
		}
	}
}

func TestOpenRouterModelsVideoDiscovery(t *testing.T) {
	// This test calls the real OpenRouter API (free, no auth required)
	resp, err := http.Get("https://openrouter.ai/api/v1/videos/models")
	if err != nil {
		t.Fatalf("failed to call OpenRouter video models API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("API returned status %d", resp.StatusCode)
	}

	var list types.OpenRouterMediaModelList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Video models might be fewer, but the API should return a valid response
	if len(list.Data) > 0 {
		m := list.Data[0]
		if m.ID == "" {
			t.Error("first video model should have an ID")
		}
	}
}

// TestOpenRouterModelsImageDiscovery_format validates that the real API response
// can be correctly processed by the display logic (parameter descriptors, etc.)
func TestOpenRouterModelsImageDiscovery_format(t *testing.T) {
	resp, err := http.Get("https://openrouter.ai/api/v1/images/models")
	if err != nil {
		t.Fatalf("API call failed: %v", err)
	}
	defer resp.Body.Close()

	var list types.OpenRouterMediaModelList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	// Verify parameter descriptor types for every model
	for _, m := range list.Data {
		for k, p := range m.SupportedParameters {
			if p.Type != "enum" && p.Type != "range" && p.Type != "boolean" {
				t.Errorf("model %s param %q has unknown type %q", m.ID, k, p.Type)
			}
			if p.Type == "enum" && len(p.Values) == 0 {
				t.Errorf("model %s param %q is enum but has no values", m.ID, k)
			}
			if p.Type == "range" && (p.Min == nil || p.Max == nil) {
				t.Errorf("model %s param %q is range but missing min/max", m.ID, k)
			}
		}
	}
}

// TestOpenRouterModelsLocalServer verifies the display logic with a mock server.
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
					"supports_streaming": false,
					"endpoints": "/api/v1/images/models/test/model/endpoints"
				}
			]
		}`))
	}))
	defer srv.Close()

	// Override apiBase and test the discovery function
	origBase := shared.APIBase
	shared.APIBase = srv.URL
	defer func() { shared.APIBase = origBase }()

	// Just verify no panic and clean output
	err := runModelsOpenRouterDiscovery("image")
	if err != nil {
		t.Fatalf("runModelsOpenRouterDiscovery() error = %v", err)
	}
}
