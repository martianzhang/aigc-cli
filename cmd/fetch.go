package cmd

import "github.com/martianzhang/aigc-cli/internal/cli/web"

func init() {
	rootCmd.AddCommand(web.FetchCmd())
}
