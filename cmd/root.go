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
	"github.com/martianzhang/aigc-cli/internal/secret"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/vault"
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

Generate images, videos and music, run Midjourney, chat with tools, search the
web, fetch pages as Markdown, search prompt ideas and knowledge bases — or
expose all of it to AI agents via MCP.

Getting started (no config file required):
  1. Credentials:  export OPENAI_API_KEY=sk-xxx
                   export OPENAI_BASE_URL=https://api.apimart.ai   # any OpenAI-compatible endpoint
                   (or per-command flags --api-key/--api-base; Ollama/local needs no key)
  2. Inspect:      aigc-cli --print-config                # effective config as YAML
  3. Run:          aigc-cli image --prompt "A cat" --dry-run   # preview request, zero cost

Persistent config lives at ~/.config/aigc-cli/config.yaml and must already exist
 (copy docs/*/examples/config.yaml). Then write values with:
   aigc-cli config set api_key sk-xxx --force
   aigc-cli config set defaults.image.model gpt-image-2

Models: image and video have no built-in model — pass --model or set
 defaults.<cmd>.model in config.yaml. Other commands apply their own default.
 Run 'aigc-cli models' to list what the current endpoint offers.

Offline commands (detect, ocr, background, depth, upscale, vision, audio, ideas,
 knowledgebase) need a one-time model download: run 'aigc-cli <command> init'.

Output & exit: generated/downloaded files go to -o/--output (default ".").
 Results print as text; -v adds the full request/result JSON; --print-config is
 YAML. Exit code 0 = success, 1 = error (message on stderr).

Guides: docs/zh/ (中文) and docs/en/ (English) are authoritative. Run
'aigc-cli <command> --help' for command-specific flags.`,
	Example: `# Setup (env vars need no config file)
  export OPENAI_API_KEY=sk-xxx
  export OPENAI_BASE_URL=https://api.apimart.ai
  aigc-cli --print-config                                 # show effective config

# Common workflows
  aigc-cli image --prompt "A cat under starry sky"
  aigc-cli video --prompt "A kitten yawning at the camera"
  aigc-cli chat --message "Hello, who are you?"
  aigc-cli midjourney imagine --prompt "a cute cat --ar 16:9"
  aigc-cli detect photo.png
  aigc-cli mcp

# Diagnostics — --dry-run/--json are per-command flags, not global
  aigc-cli image --prompt "A cat" --dry-run              # print equivalent curl, zero cost (also video/chat/depth; midjourney/music/audio: on their subcommands)
  aigc-cli image --json '{"prompt":"a red fox","n":4}'   # pass request as JSON (image/video/chat/midjourney/music)
  aigc-cli image --prompt "A cat" -v                     # verbose debug output (full result JSON + token/cost stats)

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
		shared.ZDRSet = hasFlagChanged(cmd, "zdr")
		shared.HTTPProxySet = hasFlagChanged(cmd, "http-proxy")

		// --no-proxy is a hard bypass: mixing it with an explicit proxy URL is
		// ambiguous, so reject the combination instead of silently picking one.
		if shared.NoProxy && shared.HTTPProxy != "" {
			return fmt.Errorf("--no-proxy cannot be combined with --http-proxy %q", shared.HTTPProxy)
		}
		provider.SetDirectMode(shared.NoProxy)

		// An explicitly passed --config that is absent is user error: fail fast
		// instead of silently running with env/defaults only.
		if err := requireExplicitConfigPath(shared.CfgFile); err != nil {
			return err
		}

		// Load config (optional) to resolve defaults not set via flags. Any
		// remaining error (decryption failure, malformed YAML, permission) is
		// fatal: never run with a silently ignored config error.
		cfg, cfgErr := config.Load(shared.CfgFile)
		if cfgErr != nil {
			return cfgErr
		}
		if cfg != nil {
			shared.Cfg = cfg
			if shared.APIKey == "" {
				shared.APIKey = cfg.APIKey
			}
			if shared.APIBase == "" {
				shared.APIBase = cfg.BaseURL
			}
			if shared.HTTPProxy == "" && !shared.NoProxy {
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

		// Make sure a local encryption master secret exists, then encrypt any
		// plaintext secrets still on disk.
		ensureMasterSecret()
		encryptPlaintextConfigSecrets()
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

	// Append agent-oriented conventions to every subcommand's --help output.
	installAgentHelp(rootCmd)

	// Run parent PersistentPreRunE hooks as well, so the shared startup
	// (master secret + plaintext auto-encryption) also runs for commands that
	// define their own hook (mcp, knowledgebase).
	cobra.EnableTraverseRunHooks = true

	rootCmd.PersistentFlags().StringVar(&shared.CfgFile, "config", "", "path to config file (must exist; default ~/.config/aigc-cli/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&shared.APIKey, "api-key", "", "API key (env: OPENAI_API_KEY)")
	rootCmd.PersistentFlags().StringVar(&shared.APIBase, "api-base", "", "API base URL (env: OPENAI_BASE_URL)")
	rootCmd.PersistentFlags().StringVar(&shared.HTTPProxy, "http-proxy", "", "HTTP proxy URL (env: HTTP_PROXY / HTTPS_PROXY / NO_PROXY)")
	rootCmd.PersistentFlags().BoolVar(&shared.NoProxy, "no-proxy", false, "ignore http_proxy config and HTTP_PROXY / HTTPS_PROXY for this run (direct connection)")
	rootCmd.PersistentFlags().StringVarP(&shared.Model, "model", "m", "", "Model name (overrides defaults.{cmd}.model; required by image/video when unset)")
	rootCmd.PersistentFlags().StringVarP(&shared.Provider, "provider", "P", "", "Named provider from config.providers (overrides defaults.{cmd}.provider)")
	rootCmd.PersistentFlags().StringVarP(&shared.OutputDir, "output", "o", ".", "output directory for downloaded/generated files")
	rootCmd.PersistentFlags().BoolVarP(&shared.Verbose, "verbose", "v", false, "verbose debug output")
	rootCmd.PersistentFlags().IntVar(&shared.TimeoutFlag, "timeout", 0, "HTTP request timeout in seconds; also caps async task polling (overrides config timeout / defaults.<cmd>.timeout)")
	rootCmd.PersistentFlags().BoolVar(&shared.PrintConfig, "print-config", false, "show effective configuration and exit")
	rootCmd.PersistentFlags().BoolVar(&shared.ZDR, "zdr", false, "request zero data retention where supported (OpenRouter chat/music)")
}

func hasFlagChanged(cmd *cobra.Command, name string) bool {
	return options.HasFlagChanged(cmd, name)
}

// requireExplicitConfigPath fails when a user-supplied --config file is absent,
// so the CLI reports the mistake instead of silently ignoring the path (and the
// config-derived env fallbacks along with it). An empty path (default lookup)
// is always allowed.
func requireExplicitConfigPath(path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("config file not found: %s (create it first or omit --config; see docs/zh/examples/config.yaml)", path)
	}
	return nil
}

// ensureMasterSecret resolves (and on first use generates) the local
// encryption master secret. It never fails the command: when the keyring is
// unavailable it prints a one-line hint to set AIGC_CLI_MASTER_KEY.
func ensureMasterSecret() {
	// Env override or explicit opt-out: nothing to generate.
	if os.Getenv(secret.EnvVar) != "" || os.Getenv(secret.DisableEnvVar) != "" {
		return
	}
	_, created, err := vault.EnsureIdentity()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		return
	}
	if created {
		fmt.Fprintln(os.Stderr, "Generated local encryption key (stored in the system keyring).")
	}
}

// encryptPlaintextConfigSecrets rewrites the config file so plaintext secrets
// are stored encrypted. Best-effort: a failure only prints a warning, since
// the command itself can still run with the in-memory plaintext.
func encryptPlaintextConfigSecrets() {
	path := shared.CfgFile
	if path == "" {
		var err error
		path, err = config.DefaultPath()
		if err != nil {
			return
		}
	}
	count, err := config.EncryptSecretsInFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not encrypt config secrets: %v\n", err)
		return
	}
	if count > 0 {
		fmt.Fprintf(os.Stderr, "Encrypted %d secret(s) in %s\n", count, path)
	}
}

// configDisplay wraps types.Config to inline fields for clean YAML output.
type configDisplay struct {
	types.Config `yaml:",inline"`
}
