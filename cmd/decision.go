package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/decision"

func init() {
	rootCmd.AddCommand(decision.Cmd())
}
