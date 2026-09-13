package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/music"

func init() {
	rootCmd.AddCommand(music.Cmd())
}
