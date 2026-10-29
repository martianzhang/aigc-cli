package cmd

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestOpenRouterProviderName(t *testing.T) {
	t.Run("conventional key wins", func(t *testing.T) {
		cfg := &types.Config{Providers: map[string]*types.NamedProvider{
			"or":         {BaseURL: "https://openrouter.ai/api/v1"},
			"openrouter": {BaseURL: "https://api.ofox.ai/v1"},
		}}
		if got := openRouterProviderName(cfg); got != "openrouter" {
			t.Errorf("openRouterProviderName() = %q, want %q", got, "openrouter")
		}
	})

	t.Run("detects by base url", func(t *testing.T) {
		cfg := &types.Config{Providers: map[string]*types.NamedProvider{
			"apimart": {BaseURL: "https://api.apimart.ai"},
			"or":      {BaseURL: "https://openrouter.ai/api/v1"},
		}}
		if got := openRouterProviderName(cfg); got != "or" {
			t.Errorf("openRouterProviderName() = %q, want %q", got, "or")
		}
	})

	t.Run("none configured", func(t *testing.T) {
		cfg := &types.Config{Providers: map[string]*types.NamedProvider{
			"apimart": {BaseURL: "https://api.apimart.ai"},
		}}
		if got := openRouterProviderName(cfg); got != "" {
			t.Errorf("openRouterProviderName() = %q, want empty", got)
		}
		if got := openRouterProviderName(nil); got != "" {
			t.Errorf("openRouterProviderName(nil) = %q, want empty", got)
		}
	})
}
