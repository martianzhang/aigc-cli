package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/detect"

func init() {
	rootCmd.AddCommand(detect.Cmd())
}
