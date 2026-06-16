package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// chat flag variables
var (
	chatModel       string
	chatSystem      string
	chatMessages    []string
	chatTemperature float64
	chatMaxTokens   int
	chatNoStream    bool
	chatJSONFlag    string
)

// chatCmd represents the `apimart-cli chat` command.
var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Chat with AI models (streaming by default)",
	Long: `Start a chat conversation with AI models via the APIMart API.

Supports all major models: GPT-5, Claude, Gemini, DeepSeek, and more.
Default model is deepseek-v4-flash. Streaming output is enabled by default.

Examples:
  apimart-cli chat --message "Hello, who are you?"
  apimart-cli chat --system "You are a poet" --message "Write a poem about AI"
  apimart-cli chat --message "What is Go?" --message "Can you give an example?" --no-stream
  apimart-cli chat --json '{"model":"gpt-5","messages":[{"role":"user","content":"Hi"}]}'`,
	RunE: runChat,
}

func runChat(cmd *cobra.Command, args []string) error {
	req, err := buildChatRequest(cmd)
	if err != nil {
		return err
	}

	// Apply defaults
	if req.Model == "" {
		req.Model = "deepseek-v4-flash"
	}
	if !cmd.Flags().Changed("stream") {
		req.Stream = true
	}

	// Merge config defaults
	if cfg, err := config.LoadDefaults(cfgFile); err == nil && cfg != nil && cfg.Defaults != nil && cfg.Defaults.Chat != nil {
		d := cfg.Defaults.Chat
		if d.Model != "" {
			req.Model = d.Model
		}
	}

	c := client.New(apiKey, apiBase, httpProxy)
	result, err := c.ChatCompletion(req)
	if err != nil {
		return fmt.Errorf("chat failed: %w", err)
	}

	// Non-streaming: print result
	if !req.Stream && result != nil && len(result.Choices) > 0 {
		fmt.Println(result.Choices[0].Message.Content)
	}

	// Usage stats
	if result != nil && result.Usage != nil {
		fmt.Fprintf(os.Stderr, "\n---\nTokens: %d prompt + %d completion = %d total\n",
			result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Usage.TotalTokens)
	}

	return nil
}

func buildChatRequest(cmd *cobra.Command) (*types.ChatRequest, error) {
	// JSON input
	if chatJSONFlag != "" {
		data, err := readInput(chatJSONFlag)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.ChatRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		return req, nil
	}

	var messages []types.ChatMessage

	if chatSystem != "" {
		messages = append(messages, types.ChatMessage{Role: "system", Content: chatSystem})
	}

	for _, msg := range chatMessages {
		messages = append(messages, types.ChatMessage{Role: "user", Content: msg})
	}

	// If no --message, read from stdin
	if len(messages) == 0 {
		data, err := readInput("-")
		if err != nil {
			return nil, fmt.Errorf("failed to read prompt from stdin: %w", err)
		}
		prompt := strings.TrimSpace(string(data))
		if prompt != "" {
			messages = append(messages, types.ChatMessage{Role: "user", Content: prompt})
		}
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("message is required (use --message or pipe to stdin)")
	}

	req := &types.ChatRequest{
		Model:    chatModel,
		Messages: messages,
		Stream:   !chatNoStream,
	}

	setFloatFlag(cmd, "temperature", &req.Temperature, chatTemperature)
	setIntFlag(cmd, "max-tokens", &req.MaxTokens, chatMaxTokens)

	return req, nil
}

func setFloatFlag(cmd *cobra.Command, name string, target **float64, val float64) {
	if cmd.Flags().Changed(name) {
		v := val
		*target = &v
	}
}

func init() {
	f := chatCmd.Flags()
	f.StringVar(&chatModel, "model", "", `Model name (default "deepseek-v4-flash")`)
	f.StringVar(&chatSystem, "system", "", "System prompt to set AI behavior")
	f.StringArrayVar(&chatMessages, "message", nil, "User message (repeatable for multi-turn)")
	f.Float64Var(&chatTemperature, "temperature", 0, "Sampling temperature (0-2)")
	f.IntVar(&chatMaxTokens, "max-tokens", 0, "Maximum tokens in response")
	f.BoolVar(&chatNoStream, "no-stream", false, "Disable streaming, wait for full response")
	f.StringVar(&chatJSONFlag, "json", "", "JSON file, string, or \"-\" for stdin")

	rootCmd.AddCommand(chatCmd)
}
