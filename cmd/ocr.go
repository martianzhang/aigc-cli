package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/ocr"

func init() {
	rootCmd.AddCommand(ocr.Cmd())
}
