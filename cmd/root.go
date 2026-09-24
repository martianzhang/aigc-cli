package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/chat"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// shared aliases the options package's shared config (same pointer), so existing
// call sites keep working while subpackages read options.Shared.
var shared = options.Shared

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:           "aigc-cli",
	Short:         "Unified CLI for OpenAI-compatible APIs (supports OpenAI, OpenRouter, APIMart)",
	Version:       Version,
	SilenceErrors: true,
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: false,
	},
	Long: `Unified CLI for OpenAI-compatible APIs (OpenAI, OpenRouter, APIMart and any
compatible relay), plus local offline models for AIGC detection, OCR, background
removal, depth maps, TTS/ASR, and image understanding.

Generate images, videos and music, run Midjourney, chat with tools, search prompt
ideas and knowledge bases — or expose all of it to AI agents via MCP.

Guides: docs/zh/ (中文) and docs/en/ (English) are authoritative. Run
'aigc-cli <command> --help' for command-specific flags.`,
	Example: `# Common workflows
  aigc-cli image --prompt "A cat under starry sky"
  aigc-cli video --prompt "A kitten yawning at the camera"
  aigc-cli chat --message "Hello, who are you?"
  aigc-cli midjourney imagine --prompt "a cute cat --ar 16:9"
  aigc-cli detect photo.png
  aigc-cli mcp

# Diagnostics — --dry-run/--json are per-command flags, not global
  aigc-cli image --prompt "A cat" --dry-run              # print equivalent curl, zero cost (also video/chat/depth; midjourney/music/audio: on their subcommands)
  aigc-cli image --json '{"prompt":"a red fox","n":4}'   # pass request as JSON (image/video/chat/midjourney/music)
  aigc-cli detect photo.png --json                       # output as JSON (detect/ideas/ocr scan/background)
  aigc-cli image --prompt "A cat" -v                     # full result JSON + token/cost stats (global flag)

# Agent integration and guides
  aigc-cli mcp --list-tools                              # list MCP tools (--list-prompts for workflow prompts)
  # docs/zh/ and docs/en/ guide-*.md files are the authoritative reference`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c := chat.Cmd()
		return c.RunE(c, args)
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
	provider.Version = Version

	rootCmd.PersistentFlags().StringVar(&shared.CfgFile, "config", "", "path to config file (default ~/.config/aigc-cli/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&shared.APIKey, "api-key", "", "API key (env: OPENAI_API_KEY)")
	rootCmd.PersistentFlags().StringVar(&shared.APIBase, "api-base", "", "API base URL (env: OPENAI_BASE_URL)")
	rootCmd.PersistentFlags().StringVar(&shared.HTTPProxy, "http-proxy", "", "HTTP proxy URL (env: HTTP_PROXY / HTTPS_PROXY / NO_PROXY)")
	rootCmd.PersistentFlags().StringVarP(&shared.Model, "model", "m", "", "Model name (optional; subcommand applies its own default when omitted)")
	rootCmd.PersistentFlags().StringVarP(&shared.Provider, "provider", "P", "", "Named provider from config.providers (overrides defaults.{cmd}.provider)")
	rootCmd.PersistentFlags().StringVarP(&shared.OutputDir, "output", "o", ".", "output directory for downloaded/generated files")
	rootCmd.PersistentFlags().BoolVarP(&shared.Verbose, "verbose", "v", false, "verbose output: show full result JSON")
	rootCmd.PersistentFlags().IntVar(&shared.TimeoutFlag, "timeout", 0, "HTTP request timeout in seconds (overrides config)")
	rootCmd.PersistentFlags().BoolVar(&shared.PrintConfig, "print-config", false, "show effective configuration and exit")
}

func hasFlagChanged(cmd *cobra.Command, name string) bool {
	return options.HasFlagChanged(cmd, name)
}

// configDisplay wraps types.Config to inline fields for clean YAML output.
type configDisplay struct {
	types.Config `yaml:",inline"`
}
