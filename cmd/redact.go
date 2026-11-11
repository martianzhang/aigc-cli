package cmd

import (
	"github.com/martianzhang/aigc-cli/internal/cli/redact"
)

// redactDeps resolves the redact command's dependencies at run time, after
// PersistentPreRunE has loaded config and flags.
func redactDeps() redact.Deps {
	return redact.Deps{
		OutputDir:       shared.OutputDir,
		Verbose:         shared.Verbose,
		Cfg:             shared.Cfg,
		ResolveProvider: shared.ResolveProvider,
	}
}

func init() {
	rootCmd.AddCommand(redact.NewCommand(redactDeps))
}
