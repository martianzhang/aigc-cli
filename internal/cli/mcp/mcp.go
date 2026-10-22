package mcp

import (
	"fmt"
	"os"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/mcp"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/spf13/cobra"
)

// mcpCmd represents the `aigc-cli mcp` command.
var mcpCmd = &cobra.Command{
	Use:          "mcp",
	Short:        "Start MCP server for AI agent integration",
	SilenceUsage: true,
	Long: `Start an MCP (Model Context Protocol) server over stdio.

This allows AI agents (Claude Desktop, Cursor, etc.) to call aigc-cli
tools directly: generate images, generate videos, query models, etc.

Configuration is read from config.yaml, environment variables, and --config flag.

Use --list-tools to see the tool registry (all tools with their enable/disable
state, or just the enabled ones), and --list-prompts to see available prompts.

Example MCP host config:
{
  "mcpServers": {
    "aigc-cli": {
      "command": "aigc-cli",
      "args": ["mcp"]
    }
  }
}
`,
	Example: `  aigc-cli mcp --list-tools            # every tool, annotated with enable/disable state
  aigc-cli mcp --list-tools=enabled    # only the tools a client will actually see
  aigc-cli mcp --list-prompts          # list available workflow prompts and exit
  aigc-cli mcp                         # run the stdio server (used by MCP host config)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		listTools, _ := cmd.Flags().GetString("list-tools")
		listPrompts, _ := cmd.Flags().GetBool("list-prompts")
		// `--list-tools=` (explicit empty) must still list rather than silently
		// starting the stdio server.
		if cmd.Flags().Changed("list-tools") && listTools == "" {
			listTools = "all"
		}

		// Load config (optional); non-fatal but logged via verbose if set.
		cfg, loadErr := config.Load(options.Shared.CfgFile)

		// Resolve providers for all commands that MCP tools may use.
		cmdProviders := map[string]*provider.EffectiveProvider{
			options.ProviderNameImage:      options.Shared.ResolveProvider(options.ProviderNameImage),
			options.ProviderNameVideo:      options.Shared.ResolveProvider(options.ProviderNameVideo),
			options.ProviderNameChat:       options.Shared.ResolveProvider(options.ProviderNameChat),
			options.ProviderNameAudio:      options.Shared.ResolveProvider(options.ProviderNameAudio),
			options.ProviderNameMusic:      options.Shared.ResolveProvider(options.ProviderNameMusic),
			options.ProviderNameMidjourney: options.Shared.ResolveProvider(options.ProviderNameMidjourney),
		}

		mcpCfg := &mcp.Config{
			APIKey:       options.Shared.APIKey,
			BaseURL:      options.Shared.APIBase,
			Proxy:        options.Shared.HTTPProxy,
			Output:       options.Shared.OutputDir,
			ListTools:    listTools,
			ListPrompts:  listPrompts,
			CmdProviders: cmdProviders,
		}

		if cfg != nil {
		} else if loadErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: config load error (will use defaults only): %v\n", loadErr)
		}
		if cfg != nil {
			if mcpCfg.APIKey == "" {
				mcpCfg.APIKey = cfg.APIKey
			}
			if mcpCfg.BaseURL == "" {
				mcpCfg.BaseURL = cfg.BaseURL
			}
			if mcpCfg.Proxy == "" {
				mcpCfg.Proxy = cfg.HTTPProxy
			}
			mcpCfg.ToolsEnable = cfg.ToolsEnable
			mcpCfg.ToolsDisable = cfg.ToolsDisable
			mcpCfg.SearchProvider = options.SearchProviderFromConfig(cfg)
			mcpCfg.WebSearchProviders = cfg.WebSearch
			mcpCfg.Defaults = cfg.Defaults
			if mcpCfg.Defaults == nil {
				mcpCfg.Defaults = &types.ConfigDefaults{}
			}
			mcpCfg.Providers = cfg.Providers
		}

		return mcp.Run(mcpCfg)
	},
}

func init() {
	// Override PersistentPreRunE to skip the api key check for mcp command.
	// Some MCP tools (list_models, get_model_pricing) work without API key.
	mcpCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		options.Shared.APIKeySet = options.HasFlagChanged(cmd, "api-key")
		options.Shared.APIBaseSet = options.HasFlagChanged(cmd, "api-base")
		options.Shared.ProviderSet = options.HasFlagChanged(cmd, "provider")
		options.Shared.ZDRSet = options.HasFlagChanged(cmd, "zdr")
		options.Shared.HTTPProxySet = options.HasFlagChanged(cmd, "http-proxy")

		// Load config to populate shared fields if not set via flags
		if c, err := config.Load(options.Shared.CfgFile); err == nil {
			options.Shared.Cfg = c // ResolveProvider 依赖 Cfg 读取 defaults.{cmd}.provider
			if options.Shared.APIKey == "" {
				options.Shared.APIKey = c.APIKey
			}
			if options.Shared.APIBase == "" {
				options.Shared.APIBase = c.BaseURL
			}
			if options.Shared.HTTPProxy == "" && !options.Shared.NoProxy {
				options.Shared.HTTPProxy = c.HTTPProxy
			}
			if !options.HasFlagChanged(cmd, "output") && c.OutputDir != "" {
				options.Shared.OutputDir = c.OutputDir
			}
		}
		// Configure global HTTP client with proxy for all requests.
		// --no-proxy also covers the MCP tools' HTTP calls (kb_fetch etc.).
		provider.SetDirectMode(options.Shared.NoProxy)
		client.ConfigureDefaultClient(options.Shared.HTTPProxy)
		// Don't error on missing API key - tools will handle it gracefully
		return nil
	}

	mcpCmd.Flags().String("list-tools", "", "list MCP tools and exit: --list-tools[=all|enabled] (bare flag = all)")
	// NoOptDefVal lets the bare `--list-tools` form keep working while also
	// accepting an explicit mode.
	mcpCmd.Flags().Lookup("list-tools").NoOptDefVal = "all"
	mcpCmd.Flags().Bool("list-prompts", false, "list available MCP prompts and exit")
}

// Cmd returns the mcp command.
func Cmd() *cobra.Command { return mcpCmd }
