package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/martianzhang/apimart-cli/internal/config"
)

var (
	cfgFile    string
	apiKey     string
	apiBase    string
	httpProxy  string
	jsonInput  string
	waitFlag   bool
	outputDir  string
)

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:   "apimart-cli",
	Short: "CLI tool for APIMart image generation",
	Long: `A command-line tool to generate images via the APIMart API.

Supports all GPT-Image-2 parameters via flags or raw JSON input.
Default values can be set in ~/.config/apimart/config.yaml`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Load config (optional) to resolve defaults not set via flags
		if cfg, err := config.Load(cfgFile); err == nil {
			if apiKey == "" {
				apiKey = cfg.APIKey
			}
			if apiBase == "" {
				apiBase = cfg.BaseURL
			}
			if httpProxy == "" {
				httpProxy = cfg.HTTPProxy
			}
		}
		if apiKey == "" {
			return fmt.Errorf("API key is required: set it via --api-key flag, APIMART_API_KEY env, or config.yaml")
		}
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "path to config file (default ~/.config/apimart/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "APIMart API key (env: APIMART_API_KEY)")
	rootCmd.PersistentFlags().StringVar(&apiBase, "api-base", "", "API base URL (env: APIMART_API_BASE, default https://api.apimart.ai)")
	rootCmd.PersistentFlags().StringVar(&httpProxy, "http-proxy", "", "HTTP proxy URL (env: APIMART_HTTP_PROXY, e.g. http://127.0.0.1:7890)")
	rootCmd.PersistentFlags().StringVar(&jsonInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	rootCmd.PersistentFlags().BoolVar(&waitFlag, "wait", false, "wait for task completion and download results")
	rootCmd.PersistentFlags().StringVar(&outputDir, "output", ".", "output directory for downloaded images")
}
