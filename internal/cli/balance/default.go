package balance

import (
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// GetTextDefault queries balances using the process-global shared config.
func GetTextDefault(scope string) (string, error) {
	var providers map[string]*types.NamedProvider
	if options.Shared.Cfg != nil {
		providers = options.Shared.Cfg.Providers
	}
	return GetText(Deps{
		ProviderSet:     options.Shared.ProviderSet,
		Provider:        options.Shared.Provider,
		ResolveProvider: options.Shared.ResolveProvider,
		Providers:       providers,
	}, scope)
}
