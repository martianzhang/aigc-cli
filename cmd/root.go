package cmd

import (
	"fmt"
	"os"

	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile    string
	apiKey     string
	apiBase    string
	httpProxy  string
	jsonInput  string
	outputDir  string
	verbose    bool
	savePrompt bool
	mode       string
)

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:     "apimart-cli",
	Short:   "Unified CLI for OpenAI-compatible APIs (supports OpenAI, OpenRouter, APIMart)",
	Version: Version,
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true, // hide "completion" to avoid confusion with LLM completion
	},
	Long: `Unified CLI for OpenAI-compatible APIs. Supports OpenAI, OpenRouter, APIMart and any
OpenAI-compatible third-party relay. Backward-compatible with APIMart.`,
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
			if !cmd.Flags().Changed("verbose") {
				verbose = cfg.Verbose
			}
			if !cmd.Flags().Changed("save-prompt") {
				savePrompt = cfg.SavePrompt
			}
			if mode == "" && cfg.Mode != "" {
				mode = cfg.Mode
			}
		}
		if apiKey == "" {
			return fmt.Errorf("API key is required: set it via --api-key flag, OPENAI_API_KEY env, or config.yaml")
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
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "path to config file (default ~/.config/openai/config.yaml or ~/.config/apimart/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "API key (env: OPENAI_API_KEY or APIMART_API_KEY)")
	rootCmd.PersistentFlags().StringVar(&apiBase, "api-base", "", "API base URL (env: OPENAI_BASE_URL or APIMART_API_BASE)")
	rootCmd.PersistentFlags().StringVar(&httpProxy, "http-proxy", "", "HTTP proxy URL (env: OPENAI_HTTP_PROXY or APIMART_HTTP_PROXY)")
	rootCmd.PersistentFlags().StringVar(&jsonInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	rootCmd.PersistentFlags().StringVar(&mode, "mode", "", "Generation mode: auto (detect), sync, async (default: auto)")

	rootCmd.PersistentFlags().StringVar(&outputDir, "output", ".", "output directory for downloaded images")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output: show full result JSON")
	rootCmd.PersistentFlags().BoolVar(&savePrompt, "save-prompt", false, "save prompt to .md file alongside results")
}
