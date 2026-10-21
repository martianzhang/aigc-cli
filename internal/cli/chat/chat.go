package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/mcpcli"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// chat flag variables
var (
	chatSystem      string
	chatMessages    []string
	chatTemperature float64
	chatMaxTokens   int
	chatContextSize int
	chatNoStream    bool
	chatJSONFlag    string
	chatInteractive bool
	chatDryRun      bool
	// chatSearchProvider overrides defaults.search.provider for the
	// web_search/web_fetch primitives when set on the CLI.
	chatSearchProvider string
	// chatInjectJSON is a --inject-json snippet merged additively into the body.
	chatInjectJSON string
	// chatInjectRaw is chatInjectJSON parsed once by buildChatRequest, so the
	// agent loop and TUI (which rebuild the request each turn) can re-apply it.
	chatInjectRaw json.RawMessage
)

// chatCmd represents the `aigc-cli chat` command.
var chatCmd = &cobra.Command{
	Use:          "chat",
	Short:        "Chat with AI models (streaming by default)",
	SilenceUsage: true,
	Long: `Start a chat conversation with AI models via any OpenAI-compatible API.

Supports all major models: GPT, Claude, Gemini, DeepSeek, and more.
Streaming output is enabled by default.

Agentic Chat:
  Chat supports tool calling by default.

Context Management:
  Use --context-size to limit input context. When the conversation exceeds
  80% of the limit, older messages are automatically summarized to stay
  within bounds. Use /compact in interactive mode for manual compaction.

Modes:
  - Interactive multi-turn (default without --message):
      aigc-cli chat
  - Single-turn with --message:
      aigc-cli chat --message "Hello"

Examples:
  aigc-cli chat --message "Hello, who are you?"
  aigc-cli chat --context-size 8192 --max-output 2048
  aigc-cli chat --json '{"model":"gpt-5","messages":[{"role":"user","content":"Hi"}]}'`,
	RunE: runChat,
}

func runChat(cmd *cobra.Command, args []string) error {
	chatCfg := options.ChatDefaults()
	applyChatConfigDefaults(cmd, chatCfg)

	// --json mode is always single-turn
	if chatJSONFlag != "" {
		req, err := buildChatRequest(cmd)
		if err != nil {
			return err
		}
		return sendChatRequest(cmd, req)
	}

	req, err := buildChatRequest(cmd)
	if err != nil {
		return err
	}

	// --dry-run is single-turn: a preview must never open the interactive TUI.
	if chatDryRun {
		return sendChatRequest(cmd, req)
	}

	// Interactive REPL (TUI) when no --message and not --json
	if !cmd.Flags().Changed("message") || chatInteractive {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			// Piped input without messages -- non-interactive single turn
			return sendChatRequest(cmd, req)
		}

		return runChatTUI(cmd)
	}

	// Non-interactive with --message(s): the agent loop below performs the
	// single send (with tools) and prints the answer once.
	maxIterations := 10
	if chatCfg != nil && chatCfg.MaxIterations > 0 {
		maxIterations = chatCfg.MaxIterations
	}
	ctx := context.Background()
	mcpMgr, mcpErr := mcpcli.NewManager(ctx, options.MCPConfig())
	if mcpErr != nil {
		fmt.Fprintf(options.Stderr(), "mcp: %v\n", mcpErr)
	}
	defer mcpMgr.Close()
	rt := newToolRuntime(chatCfg, mcpMgr)

	p := options.Shared.ResolveProvider(options.ProviderNameChat)
	c := client.NewFromProvider(p)
	history := req.Messages
	if ctx := loadAgentsContext(); ctx != "" && (len(history) == 0 || history[0].Role != "system") {
		history = append([]types.ChatMessage{{Role: "system", Content: ctx}}, history...)
	}
	err = runAgentLoop(ctx, c, rt, &history, maxIterations, cmd, p)
	if err != nil {
		return err
	}
	if options.Shared.Verbose && len(history) > len(req.Messages) {
		added := len(history) - len(req.Messages)
		fmt.Fprintf(options.Stderr(), "Agent loop completed: %d additional messages accumulated (tool calls + responses)\n", added)
	}
	return nil
}

// applyChatConfigDefaults fills flag variables from config when the flag
// was not set on the CLI. Config priority: CLI flags > config defaults.
func applyChatConfigDefaults(cmd *cobra.Command, cfg *types.ChatDefaults) {
	if cfg == nil {
		return
	}
	if !cmd.Flags().Changed("context-size") && cfg.ContextSize > 0 {
		chatContextSize = cfg.ContextSize
	}
	if !cmd.Flags().Changed("max-output") && cfg.MaxTokens > 0 {
		chatMaxTokens = cfg.MaxTokens
	}
}

func init() {
	f := chatCmd.Flags()
	f.StringVarP(&chatSystem, "system", "s", "", "System prompt to set AI behavior")
	f.StringArrayVar(&chatMessages, "message", nil, "User message (repeatable for multi-turn)")
	f.Float64VarP(&chatTemperature, "temperature", "t", 0, "Sampling temperature (0-2)")
	f.IntVar(&chatMaxTokens, "max-output", 0, "Maximum tokens in response")
	f.IntVar(&chatContextSize, "context-size", 0, "Maximum input context size in tokens (0 = model default). Auto-compacts at 80% by summarizing older messages.")
	f.BoolVar(&chatNoStream, "no-stream", false, "Disable streaming, wait for full response")
	f.StringVar(&chatJSONFlag, "json", "", "JSON file, string, or \"-\" for stdin")
	f.BoolVarP(&chatInteractive, "interactive", "i", false, "Enter interactive multi-turn chat mode")
	f.BoolVar(&chatDryRun, "dry-run", false, "Print the equivalent curl without calling the API")
	f.StringVar(&chatSearchProvider, "search-provider", "", "Third-party vendor for the web_search/web_fetch tools: duckduckgo/firecrawl/brave/doubao/deepseek/exa, or auto/free/quality (overrides defaults.search.provider)")
	f.StringVar(&chatInjectJSON, "inject-json", "", "JSON object merged additively into the request body (arrays append, scalars overwrite); JSON string or file path")
}

// Cmd returns the chat command.
func Cmd() *cobra.Command { return chatCmd }
