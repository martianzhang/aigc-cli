package cmd

import (
	"github.com/martianzhang/aigc-cli/internal/cli/models"
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
		ResolveProvider: shared.ResolveProvider,
		Providers:       providers,
	}
}

func init() {
	rootCmd.AddCommand(models.NewCommand(modelsDeps))
}
