package cmd

import (
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/mcp"
	"github.com/spf13/cobra"
)

// mcpCmd represents the `aigc-cli mcp` command.
var mcpCmd = &cobra.Command{
	Use:          "mcp",
	Short:        "Start MCP server for AI agent integration",
	SilenceUsage: true,
	Long: `Start an MCP (Model Context Protocol) server over stdio.

This allows AI agents (Claude Desktop, Cursor, etc.) to call APIMart
tools directly: generate images, generate videos, query models, etc.

Configuration is read from config.yaml, environment variables, and --config flag.

Example MCP host config:
{
  "mcpServers": {
    "apimart": {
      "command": "aigc-cli",
      "args": ["mcp"]
    }
  }
}
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load config (optional) to get defaults
		cfg, _ := config.Load(shared.CfgFile)

		mcpCfg := &mcp.Config{
			APIKey:  shared.APIKey,
			BaseURL: shared.APIBase,
			Proxy:   shared.HTTPProxy,
			Output:  shared.OutputDir,
		}

		// Copy defaults from config if present
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
			mcpCfg.Defaults = cfg.Defaults
		}

		return mcp.Run(mcpCfg)
	},
}

func init() {
	// Override PersistentPreRunE to skip the api key check for mcp command.
	// Some MCP tools (list_models, get_model_pricing) work without API key.
	mcpCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Load config to populate shared fields if not set via flags
		if c, err := config.Load(shared.CfgFile); err == nil {
			if shared.APIKey == "" {
				shared.APIKey = c.APIKey
			}
			if shared.APIBase == "" {
				shared.APIBase = c.BaseURL
			}
			if shared.HTTPProxy == "" {
				shared.HTTPProxy = c.HTTPProxy
			}
			if !hasFlagChanged(cmd, "output") && c.OutputDir != "" {
				shared.OutputDir = c.OutputDir
			}
		}
		// Configure global HTTP client with proxy for all requests
		client.ConfigureDefaultClient(shared.HTTPProxy)
		// Don't error on missing API key - tools will handle it gracefully
		return nil
	}

	rootCmd.AddCommand(mcpCmd)
}
