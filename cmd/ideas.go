package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/ideas"

// ideasDeps resolves the ideas command's dependencies.
func ideasDeps() ideas.Deps {
	return ideas.Deps{Cfg: shared.Cfg, OutputDir: shared.OutputDir}
}

func init() {
	rootCmd.AddCommand(ideas.NewCommand(ideasDeps))
}
