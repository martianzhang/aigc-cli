package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/background"

func init() {
	rootCmd.AddCommand(background.Cmd())
}
