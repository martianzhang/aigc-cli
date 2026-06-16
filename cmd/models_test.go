package cmd

import (
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
