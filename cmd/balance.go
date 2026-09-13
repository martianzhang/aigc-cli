package cmd

import (
	"github.com/martianzhang/aigc-cli/internal/cli/balance"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// balanceDeps resolves the balance command's dependencies from loaded config.
func balanceDeps() balance.Deps {
	var providers map[string]*types.NamedProvider
	if shared.Cfg != nil {
		providers = shared.Cfg.Providers
	}
	return balance.Deps{
		ProviderSet:     shared.ProviderSet,
		Provider:        shared.Provider,
		ResolveProvider: shared.ResolveProvider,
		Providers:       providers,
	}
}

func init() {
	rootCmd.AddCommand(balance.NewCommand(balanceDeps))
}
