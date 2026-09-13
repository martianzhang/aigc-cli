package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/models"

// modelsDeps resolves the models command's dependencies.
func modelsDeps() models.Deps {
	return models.Deps{
		APIBase:         shared.APIBase,
		ResolveProvider: shared.ResolveProvider,
	}
}

func init() {
	rootCmd.AddCommand(models.NewCommand(modelsDeps))
}
