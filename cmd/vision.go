package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/vision"

func init() {
	rootCmd.AddCommand(vision.Cmd())
}
