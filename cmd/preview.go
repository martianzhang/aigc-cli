package cmd

import (
	"github.com/martianzhang/aigc-cli/internal/cli/preview"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// previewDeps resolves the preview command's dependencies.
func previewDeps() preview.Deps {
	return preview.Deps{ReadInput: service.ReadInput}
}

func init() {
	rootCmd.AddCommand(preview.NewCommand(previewDeps))
}
