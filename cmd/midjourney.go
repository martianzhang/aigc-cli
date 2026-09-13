package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/midjourney"

func init() {
	rootCmd.AddCommand(midjourney.Cmd())
}
