package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/video"

func init() {
	rootCmd.AddCommand(video.Cmd())
}
