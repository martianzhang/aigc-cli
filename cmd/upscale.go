package cmd

import (
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/cli/upscale"
	"github.com/martianzhang/aigc-cli/internal/options"
)

// upscaleDeps resolves the upscale command's dependencies.
func upscaleDeps() upscale.Deps {
	return upscale.Deps{
		OutputDir: shared.OutputDir,
		Verbose:   shared.Verbose,
		ModelsDir: filepath.Join(options.ConfigDir(), "models"),
	}
}

func init() {
	rootCmd.AddCommand(upscale.NewCommand(upscaleDeps))
}
