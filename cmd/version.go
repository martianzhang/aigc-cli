package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the current release version, injected via ldflags at build time.
// Defaults to "dev" for local builds.
var Version = "dev"

// versionCmd represents the `version` command.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(Version)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
