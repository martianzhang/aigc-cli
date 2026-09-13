package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/chat"

func init() {
	rootCmd.AddCommand(chat.Cmd())
}
