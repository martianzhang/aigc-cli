package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/audio"

func init() {
	rootCmd.AddCommand(audio.Cmd())
}
