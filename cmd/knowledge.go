package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/knowledge"

func init() {
	rootCmd.AddCommand(knowledge.Cmd())
}
