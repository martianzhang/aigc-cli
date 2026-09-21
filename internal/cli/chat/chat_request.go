package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// buildChatRequest constructs a ChatRequest from --json or individual flags.
func buildChatRequest(cmd *cobra.Command) (*types.ChatRequest, error) {
	if chatJSONFlag != "" {
		data, err := service.ReadJSONInput(chatJSONFlag)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.ChatRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}

		set := map[string]any{}
		if options.HasFlagChanged(cmd, "model") {
			set["model"] = options.Shared.Model
		}
		if options.HasFlagChanged(cmd, "system") || options.HasFlagChanged(cmd, "message") {
			set["messages"] = flagMessages()
		}
		if options.HasFlagChanged(cmd, "no-stream") {
			set["stream"] = !chatNoStream
		}
		if options.HasFlagChanged(cmd, "temperature") {
			set["temperature"] = chatTemperature
		}
		if options.HasFlagChanged(cmd, "max-output") {
			set["max_tokens"] = chatMaxTokens
		}

		merged, err := service.MergeJSONOverlay(data, set)
		if err != nil {
			return nil, fmt.Errorf("failed to merge flags into JSON input: %w", err)
		}
		req.RawJSON = merged
		if err := json.Unmarshal(merged, req); err != nil {
			return nil, fmt.Errorf("failed to parse merged JSON: %w", err)
		}
		return req, nil
	}

	req := &types.ChatRequest{
		Model:        options.Shared.Model,
		Messages:     flagMessages(),
		Stream:       !chatNoStream,
		OutputWriter: options.Stdout(),
	}
	if cmd.Flags().Changed("temperature") {
		v := chatTemperature
		req.Temperature = &v
	}
	options.SetIntFlag(cmd, "max-output", &req.MaxTokens, chatMaxTokens)

	return req, nil
}

func flagMessages() []types.ChatMessage {
	messages := make([]types.ChatMessage, 0, len(chatMessages)+1)
	if chatSystem != "" {
		messages = append(messages, types.ChatMessage{Role: "system", Content: chatSystem})
	}
	for _, msg := range chatMessages {
		messages = append(messages, types.ChatMessage{Role: "user", Content: msg})
	}
	return messages
}

// sendChatRequest sends a single chat request and prints the response.
func sendChatRequest(cmd *cobra.Command, req *types.ChatRequest) error {
	// Streaming default: on, unless --no-stream was passed
	if !chatNoStream {
		req.Stream = true
	}

	// Merge config defaults (only if not already set via --model flag or JSON)
	if req.Model == "" {
		if cfg := options.ChatDefaults(); cfg != nil && cfg.Model != "" {
			req.Model = cfg.Model
		}
	}

	p := options.Shared.ResolveProvider(options.ProviderNameChat)

	if chatDryRun {
		fmt.Println(buildChatCurl(req, p))
		return nil
	}

	c := client.NewFromProvider(p)
	req.OutputWriter = options.Stdout()

	start := time.Now()
	result, err := c.ChatCompletion(req)
	if err != nil {
		return fmt.Errorf("chat failed: %w", err)
	}
	elapsed := time.Since(start)

	// Non-streaming: print result (streaming already written to OutputWriter)
	if !req.Stream && result != nil && len(result.Choices) > 0 {
		fmt.Println(result.Choices[0].Message.Content)
	}

	// Usage stats (to stderr, only with --verbose)
	if options.Shared.Verbose {
		printUsageStats(result, elapsed)
	}

	return nil
}

// buildChatCurl renders the equivalent curl for a chat request, using the
// resolved provider host and the same body the client sends.
func buildChatCurl(req *types.ChatRequest, p *provider.EffectiveProvider) string {
	base := client.NormalizeBaseURL(p.BaseURL)
	url := base + client.ChatPath
	body, _ := json.Marshal(req)
	auth := fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", service.MaskKey(p.APIKey))

	if p.Type == types.ProviderAnthropic {
		url = base + client.AnthropicChatPath
		body, _ = client.AnthropicChatBody(req)
		auth = fmt.Sprintf("  -H \"x-api-key: %s\" \\\n", service.MaskKey(p.APIKey))
		auth += fmt.Sprintf("  -H \"anthropic-version: %s\" \\\n", client.AnthropicVersion)
	}

	cmd := fmt.Sprintf("curl -X POST %s \\\n", url)
	cmd += auth
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(body))
	return cmd
}

// printUsageStats prints token/cost/timing stats to stderr.
func printUsageStats(result *types.ChatResponse, elapsed time.Duration) {
	if result == nil {
		return
	}
	parts := []string{}
	if result.Model != "" {
		parts = append(parts, fmt.Sprintf("Model: %s", result.Model))
	}
	if result.Usage != nil {
		parts = append(parts, fmt.Sprintf("Tokens: %d↑ + %d↓ = %d",
			result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Usage.TotalTokens))
		if result.Usage.Cost > 0 {
			parts = append(parts, fmt.Sprintf("Cost: $%.6f", result.Usage.Cost))
		}
	}
	parts = append(parts, fmt.Sprintf("Time: %v", elapsed.Round(time.Millisecond)))
	fmt.Fprintln(options.Stderr(), "---  "+strings.Join(parts, "  |  "))
}

// toURLs converts a single URL string to a slice (for MJ API compatibility).
func toURLs(url string) []string {
	if url == "" {
		return nil
	}
	return []string{url}
}
