package cmd

import (
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/cli/depth"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
)

// depthDeps resolves the depth command's dependencies.
func depthDeps() depth.Deps {
	return depth.Deps{
		OutputDir: shared.OutputDir,
		Verbose:   shared.Verbose,
		ModelsDir: filepath.Join(options.ConfigDir(), "models"),
	}
}

func init() {
	rootCmd.AddCommand(depth.NewCommand(depthDeps))
}
