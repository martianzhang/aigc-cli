package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/mcp"

func init() {
	rootCmd.AddCommand(mcp.Cmd())
}
