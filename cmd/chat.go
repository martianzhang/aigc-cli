package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// chatStdout and chatStderr are the output writers used by the chat REPL.
// They default to chatStdout/chatStderr but can be overridden in tests to
// capture output without touching real file descriptors.
var chatStdout io.Writer = os.Stdout
var chatStderr io.Writer = os.Stderr

// chat flag variables
var (
	chatSystem      string
	chatMessages    []string
	chatTemperature float64
	chatMaxTokens   int
	chatNoStream    bool
	chatJSONFlag    string
	chatInteractive bool
)

// chatCmd represents the `aigc-cli chat` command.
var chatCmd = &cobra.Command{
	Use:          "chat",
	Short:        "Chat with AI models (streaming by default)",
	SilenceUsage: true,
	Long: `Start a chat conversation with AI models via the APIMart API.

Supports all major models: GPT, Claude, Gemini, DeepSeek, and more.
Streaming output is enabled by default.

Agentic Chat:
  Chat supports tool calling by default.

Modes:
  - Interactive multi-turn (default without --message):
      aigc-cli chat
  - Single-turn with --message:
      aigc-cli chat --message "Hello"

Examples:
  aigc-cli chat --message "Hello, who are you?"
  aigc-cli chat --json '{"model":"gpt-5","messages":[{"role":"user","content":"Hi"}]}'`,
	RunE: runChat,
}

func runChat(cmd *cobra.Command, args []string) error {
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

	// Interactive REPL (TUI) when no --message and not --json
	if !cmd.Flags().Changed("message") || chatInteractive {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			// Piped input without messages -- non-interactive single turn
			return sendChatRequest(cmd, req)
		}

		return runChatTUI(cmd)
	}

	// Non-interactive with --message(s)
	if err := sendChatRequest(cmd, req); err != nil {
		return err
	}

	// Check if agent loop should be used (has tools configured)
	var chatCfg *types.ChatDefaults
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Chat != nil {
		chatCfg = shared.Cfg.Defaults.Chat
	}
	maxIterations := 10
	if chatCfg != nil && chatCfg.MaxIterations > 0 {
		maxIterations = chatCfg.MaxIterations
	}
	agentTools := buildAgentTools(chatCfg)

	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
	history := req.Messages
	_, err = runAgentLoop(context.Background(), c, &history, agentTools, maxIterations, cmd)
	if err != nil {
		return err
	}
	if shared.Verbose && len(history) > len(req.Messages) {
		added := len(history) - len(req.Messages)
		fmt.Fprintf(chatStderr, "Agent loop completed: %d additional messages accumulated (tool calls + responses)\n", added)
	}
	return nil
}

// generateImageArgs is the JSON structure for generate_image tool arguments.
type generateImageArgs struct {
	Prompt  string `json:"prompt"`
	Size    string `json:"size,omitempty"`
	N       int    `json:"n,omitempty"`
	Quality string `json:"quality,omitempty"`
}

// generateVideoArgs is the JSON structure for generate_video tool arguments.
type generateVideoArgs struct {
	Prompt     string `json:"prompt"`
	Duration   int    `json:"duration,omitempty"`
	Resolution string `json:"resolution,omitempty"`
}

// watermarkArgs is the JSON structure for watermark tools.
type watermarkArgs struct {
	FilePath   string `json:"file_path"`
	OutputPath string `json:"output_path"`
	Producer   string `json:"producer"`
}

func init() {
	f := chatCmd.Flags()
	f.StringVarP(&chatSystem, "system", "s", "", "System prompt to set AI behavior")
	f.StringArrayVar(&chatMessages, "message", nil, "User message (repeatable for multi-turn)")
	f.Float64VarP(&chatTemperature, "temperature", "t", 0, "Sampling temperature (0-2)")
	f.IntVar(&chatMaxTokens, "max-tokens", 0, "Maximum tokens in response")
	f.BoolVar(&chatNoStream, "no-stream", false, "Disable streaming, wait for full response")
	f.StringVar(&chatJSONFlag, "json", "", "JSON file, string, or \"-\" for stdin")
	f.BoolVarP(&chatInteractive, "interactive", "i", false, "Enter interactive multi-turn chat mode")

	rootCmd.AddCommand(chatCmd)
}
