package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/image"

func init() {
	rootCmd.AddCommand(image.Cmd())
}
