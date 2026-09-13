package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// shared holds all shared configuration values, initialized in PersistentPreRunE.
// Replaces the previous 12 individual global variables.
var shared = &SharedConfig{}

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:           "aigc-cli",
	Short:         "Unified CLI for OpenAI-compatible APIs (supports OpenAI, OpenRouter, APIMart)",
	Version:       Version,
	SilenceErrors: true,
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: false,
	},
	Long: `Unified CLI for OpenAI-compatible APIs. Supports OpenAI, OpenRouter, APIMart and any
OpenAI-compatible third-party relay. Backward-compatible with APIMart.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return chatCmd.RunE(chatCmd, args)
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// --print-config: dump effective config with diagnostics
		if shared.PrintConfig {
			runPrintConfig(cmd)
			os.Exit(0)
		}

		// Track which persistent flags were explicitly set by the user
		// (vs inherited from config file). Used by ResolveProvider to decide
		// whether to treat these as CLI overrides.
		shared.APIKeySet = hasFlagChanged(cmd, "api-key")
		shared.APIBaseSet = hasFlagChanged(cmd, "api-base")
		shared.ProviderSet = hasFlagChanged(cmd, "provider")

		// Load config (optional) to resolve defaults not set via flags
		if cfg, err := config.Load(shared.CfgFile); err == nil {
			shared.Cfg = cfg
			if shared.APIKey == "" {
				shared.APIKey = cfg.APIKey
			}
			if shared.APIBase == "" {
				shared.APIBase = cfg.BaseURL
			}
			if shared.HTTPProxy == "" {
				shared.HTTPProxy = cfg.HTTPProxy
			}
			// Check both local and persistent flags (persistent flags are on inherited set)
			if !hasFlagChanged(cmd, "verbose") {
				shared.Verbose = cfg.Verbose
			}
			if !hasFlagChanged(cmd, "output") && cfg.OutputDir != "" {
				shared.OutputDir = cfg.OutputDir
			}
			if !hasFlagChanged(cmd, "timeout") && cfg.Timeout != nil && *cfg.Timeout > 0 {
				shared.TimeoutFlag = *cfg.Timeout
			}
			// Validate provider references in defaults
			if cfg.Defaults != nil {
				validateCmdProviders(cfg)
			}
		}

		// Configure global HTTP client with proxy for all requests
		client.ConfigureDefaultClient(shared.HTTPProxy)
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Propagate version to client package so User-Agent reflects the real version.
	client.Version = Version

	rootCmd.PersistentFlags().StringVar(&shared.CfgFile, "config", "", "path to config file (default ~/.config/aigc-cli/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&shared.APIKey, "api-key", "", "API key (env: OPENAI_API_KEY)")
	rootCmd.PersistentFlags().StringVar(&shared.APIBase, "api-base", "", "API base URL (env: OPENAI_BASE_URL)")
	rootCmd.PersistentFlags().StringVar(&shared.HTTPProxy, "http-proxy", "", "HTTP proxy URL (env: HTTP_PROXY / HTTPS_PROXY / NO_PROXY)")
	rootCmd.PersistentFlags().StringVarP(&shared.Model, "model", "m", "", "Model name (optional; subcommand applies its own default when omitted)")
	rootCmd.PersistentFlags().StringVar(&shared.Provider, "provider", "", "Named provider from config.providers (overrides defaults.{cmd}.provider)")
	rootCmd.PersistentFlags().StringVar(&shared.OutputDir, "output", ".", "output directory for downloaded images")
	rootCmd.PersistentFlags().BoolVarP(&shared.Verbose, "verbose", "v", false, "verbose output: show full result JSON")
	rootCmd.PersistentFlags().IntVar(&shared.TimeoutFlag, "timeout", 0, "HTTP request timeout in seconds (overrides config)")
	rootCmd.PersistentFlags().BoolVar(&shared.PrintConfig, "print-config", false, "show effective configuration and exit")
}

func hasFlagChanged(cmd *cobra.Command, name string) bool {
	return cmd.Flags().Changed(name) || cmd.PersistentFlags().Changed(name) || cmd.InheritedFlags().Changed(name)
}

// configDisplay wraps types.Config to inline fields for clean YAML output.
type configDisplay struct {
	types.Config `yaml:",inline"`
}
