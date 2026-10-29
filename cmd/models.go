package cmd

import (
	"sort"

	"github.com/martianzhang/aigc-cli/internal/cli/models"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// modelsDeps resolves the models command's dependencies.
func modelsDeps() models.Deps {
	var providers map[string]*types.NamedProvider
	if shared.Cfg != nil {
		providers = shared.Cfg.Providers
	}
	return models.Deps{
		APIBase:         shared.APIBase,
		Model:           shared.Model,
		ResolveProvider: resolveModelsProvider,
		Providers:       providers,
	}
}

// resolveModelsProvider defaults `models` to OpenRouter when the user did not
// pick a provider: its catalogs expose sizing metadata (aspect ratios,
// resolution tiers, per-model parameters) that the other providers omit, and
// its model ids are owner/slug, so lookups work without a provider hint.
func resolveModelsProvider(cmdName string) *provider.EffectiveProvider {
	ep := shared.ResolveProvider(cmdName)
	if cmdName != "models" {
		return ep
	}
	// Explicit flags or a named resolution win.
	if shared.ProviderSet || shared.APIBaseSet || shared.APIKeySet || (ep != nil && ep.Name != "") {
		return ep
	}
	if name := openRouterProviderName(shared.Cfg); name != "" {
		if fallback := shared.ResolveNamedProvider(name); fallback != nil {
			return fallback
		}
	}
	return ep
}

// openRouterProviderName finds the configured provider that serves OpenRouter,
// preferring the conventional "openrouter" key so the choice is deterministic.
func openRouterProviderName(cfg *types.Config) string {
	if cfg == nil || len(cfg.Providers) == 0 {
		return ""
	}
	if _, ok := cfg.Providers["openrouter"]; ok {
		return "openrouter"
	}
	names := make([]string, 0, len(cfg.Providers))
	for name := range cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		np := cfg.Providers[name]
		if np != nil && provider.Detect(np.BaseURL) == provider.OpenRouter {
			return name
		}
	}
	return ""
}

func init() {
	rootCmd.AddCommand(models.NewCommand(modelsDeps))
}
