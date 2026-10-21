package web

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/search"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// registerProviderCompletion wires shell completion for --provider.
//
// The completion callback runs outside the root PersistentPreRunE, so
// options.Shared.Cfg may still be nil; the config is loaded here on demand so
// the list matches what the command would actually use. fetchOnly restricts the
// suggestions to providers implementing search.Fetcher.
func registerProviderCompletion(cmd *cobra.Command, fetchOnly bool) {
	_ = cmd.RegisterFlagCompletionFunc("provider",
		func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			providers := completionWebSearchConfig()
			return providerCompletionNames(providers, fetchOnly, toComplete), cobra.ShellCompDirectiveNoFileComp
		})
}

// completionWebSearchConfig returns web_search.* from the already-loaded config
// or loads it from the default path. A broken/missing config degrades to nil
// (only the built-in duckduckgo) instead of failing completion.
func completionWebSearchConfig() map[string]*types.WebSearchProvider {
	if options.Shared.Cfg != nil {
		return options.Shared.Cfg.WebSearch
	}
	cfg, err := config.Load(options.Shared.CfgFile)
	if err != nil || cfg == nil {
		return nil
	}
	return cfg.WebSearch
}

// providerCompletionNames builds the registry and filters by the typed prefix.
func providerCompletionNames(providers map[string]*types.WebSearchProvider, fetchOnly bool, toComplete string) []string {
	router := search.NewRouterAll(providers)

	var names []string
	if fetchOnly {
		names = router.FetcherNames()
	} else {
		names = router.Names()
	}

	prefix := strings.ToLower(toComplete)
	out := make([]string, 0, len(names))
	for _, name := range names {
		if strings.HasPrefix(strings.ToLower(name), prefix) {
			out = append(out, name)
		}
	}
	return out
}
