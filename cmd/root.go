package cmd

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/martianzhang/apimart-cli/internal/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	cfgFile     string
	apiKey      string
	apiBase     string
	httpProxy   string
	model       string // global --model, each subcommand handles its own default
	jsonInput   string
	outputDir   string
	verbose     bool
	savePrompt  bool
	mode        string
	printConfig bool
	timeoutFlag int // global --timeout in seconds; 0 means "not set"
)

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:           "apimart-cli",
	Short:         "Unified CLI for OpenAI-compatible APIs (supports OpenAI, OpenRouter, APIMart)",
	Version:       Version,
	SilenceErrors: true,
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true, // hide "completion" to avoid confusion with LLM completion
	},
	Long: `Unified CLI for OpenAI-compatible APIs. Supports OpenAI, OpenRouter, APIMart and any
OpenAI-compatible third-party relay. Backward-compatible with APIMart.`,
	Run: func(cmd *cobra.Command, args []string) {
		// With no subcommand, show help (or --print-config which exits in PersistentPreRunE)
		cmd.Help()
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// --print-config: dump effective config with diagnostics
		if printConfig {
			runPrintConfig()
			os.Exit(0)
		}

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
			if !cmd.Flags().Changed("output") && cfg.OutputDir != "" {
				outputDir = cfg.OutputDir
			}
			if !cmd.Flags().Changed("timeout") && cfg.Timeout != nil && *cfg.Timeout > 0 {
				timeoutFlag = *cfg.Timeout
			}
		}
		if apiKey == "" {
			return fmt.Errorf("API key is required: set it via --api-key flag, OPENAI_API_KEY env, or config.yaml")
		}
		return nil
	},
}

// runPrintConfig prints the effective configuration with inline annotations.
func runPrintConfig() {
	cfg, cfgErr := config.Load(cfgFile)
	configFound := cfgErr == nil

	displayCfg := &configDisplay{}
	if configFound && cfg != nil {
		displayCfg.Config = *cfg
	}
	// env var / CLI flag takes priority over config file
	effectiveKey := displayCfg.APIKey
	if apiKey != "" {
		effectiveKey = apiKey
		displayCfg.APIKey = apiKey
	}
	if apiBase != "" && displayCfg.BaseURL == "" {
		displayCfg.BaseURL = apiBase
	}
	if httpProxy != "" && displayCfg.HTTPProxy == "" {
		displayCfg.HTTPProxy = httpProxy
	}
	// Mask API key for display
	if displayCfg.APIKey != "" {
		displayCfg.APIKey = displayCfg.APIKey[:8] + "..."
	}
	if displayCfg.Defaults == nil {
		displayCfg.Defaults = &types.ConfigDefaults{}
	}

	// Build annotations
	var configNote, baseURLNote, apiKeyNote, proxyNote string

	// Config file path
	if cfgFile != "" {
		configNote = fmt.Sprintf("# config: %s (explicit)", cfgFile)
	} else if configFound {
		// Try to find the actual config file path
		if cfg != nil {
			// cfg doesn't store the path; reconstruct from defaults
			home, _ := os.UserHomeDir()
			candidates := []string{
				filepath.Join(home, ".config", "openai", "config.yaml"),
				filepath.Join(home, ".config", "apimart", "config.yaml"),
			}
			for _, p := range candidates {
				if _, err := os.Stat(p); err == nil {
					configNote = fmt.Sprintf("# config: %s", p)
					break
				}
			}
		}
		if configNote == "" {
			configNote = "# config: found"
		}
	} else {
		configNote = "# config: not found (env vars / code defaults)"
	}

	// API key
	if effectiveKey == "" {
		apiKeyNote = "# api_key: MISSING"
	} else {
		switch {
		case strings.HasPrefix(effectiveKey, "sk-or-"):
			apiKeyNote = "" // normal, no annotation needed
		case strings.HasPrefix(effectiveKey, "sk-") && len(effectiveKey) > 20:
			apiKeyNote = "" // normal
		default:
			apiKeyNote = "# api_key: unknown format"
		}
	}

	// Base URL
	if displayCfg.BaseURL == "" {
		baseURLNote = "# base_url: not set (will use APIMart default)"
	} else {
		u, err := url.Parse(displayCfg.BaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			baseURLNote = fmt.Sprintf("# base_url: INVALID — %s", displayCfg.BaseURL)
		} else {
			baseURLNote = detectProvider(displayCfg.BaseURL)
		}
	}

	// Proxy
	if displayCfg.HTTPProxy != "" {
		proxyNote = fmt.Sprintf("# http_proxy: %s", displayCfg.HTTPProxy)
	} else if envProxy := os.Getenv("HTTP_PROXY"); envProxy != "" {
		proxyNote = ""
	} else {
		proxyNote = "# http_proxy: not set (direct connection)"
	}

	b, _ := yaml.Marshal(displayCfg)
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")

	// Print YAML with inline annotations where applicable
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		key := strings.SplitN(trimmed, ":", 2)[0]

		var annotation string
		switch key {
		case "base_url":
			annotation = baseURLNote
		case "api_key":
			annotation = apiKeyNote
		}
		if annotation != "" {
			fmt.Printf("%s  # %s\n", line, annotation)
		} else {
			fmt.Println(line)
		}
	}

	// Append standalone annotations
	for _, n := range []string{configNote, proxyNote} {
		if n != "" {
			fmt.Println(n)
		}
	}
	fmt.Println()
}

// configDisplay wraps types.Config to inline fields for clean YAML output.
type configDisplay struct {
	types.Config `yaml:",inline"`
}

// detectProvider returns a human-readable provider name from a base URL.
func detectProvider(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "unknown"
	}
	host := strings.ToLower(u.Host)
	switch {
	case strings.Contains(host, "openrouter"):
		return "OpenRouter"
	case strings.Contains(host, "apimart"), strings.Contains(host, "apib"),
		strings.Contains(host, "aiuxu"), strings.Contains(host, "aishuch"):
		return "APIMart"
	case strings.Contains(host, "openai"):
		return "OpenAI"
	case strings.Contains(host, "yunwu"), strings.Contains(host, "wlai"):
		return "Yunwu (云雾)"
	default:
		return "third-party (" + host + ")"
	}
}

// Execute adds all child commands to the root command and sets flags.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "path to config file (default ~/.config/openai/config.yaml or ~/.config/apimart/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "API key (env: OPENAI_API_KEY or APIMART_API_KEY)")
	rootCmd.PersistentFlags().StringVar(&apiBase, "api-base", "", "API base URL (env: OPENAI_BASE_URL or APIMART_API_BASE)")
	rootCmd.PersistentFlags().StringVar(&httpProxy, "http-proxy", "", "HTTP proxy URL (env: OPENAI_HTTP_PROXY or APIMART_HTTP_PROXY)")
	rootCmd.PersistentFlags().StringVarP(&model, "model", "m", "", "Model name (optional; subcommand applies its own default when omitted)")
	rootCmd.PersistentFlags().StringVar(&outputDir, "output", ".", "output directory for downloaded images")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output: show full result JSON")
	rootCmd.PersistentFlags().IntVar(&timeoutFlag, "timeout", 0, "HTTP request timeout in seconds (overrides config)")
	rootCmd.PersistentFlags().BoolVar(&printConfig, "print-config", false, "show effective configuration and exit")
}
