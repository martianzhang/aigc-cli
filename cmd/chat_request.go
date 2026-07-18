package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// buildChatRequest constructs a ChatRequest from --json or individual flags.
func buildChatRequest(cmd *cobra.Command) (*types.ChatRequest, error) {
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

	messages := make([]types.ChatMessage, 0, len(chatMessages)+1)
	if chatSystem != "" {
		messages = append(messages, types.ChatMessage{Role: "system", Content: chatSystem})
	}
	for _, msg := range chatMessages {
		messages = append(messages, types.ChatMessage{Role: "user", Content: msg})
	}

	req := &types.ChatRequest{
		Model:        shared.Model,
		Messages:     messages,
		Stream:       !chatNoStream,
		OutputWriter: chatStdout,
	}
	if cmd.Flags().Changed("temperature") {
		v := chatTemperature
		req.Temperature = &v
	}
	setIntFlag(cmd, "max-tokens", &req.MaxTokens, chatMaxTokens)

	return req, nil
}

// sendChatRequest sends a single chat request and prints the response.
func sendChatRequest(cmd *cobra.Command, req *types.ChatRequest) error {
	// Apply defaults
	if !cmd.Flags().Changed("stream") {
		req.Stream = true
	}

	// Merge config defaults
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Chat != nil {
		if shared.Cfg.Defaults.Chat.Model != "" {
			req.Model = shared.Cfg.Defaults.Chat.Model
		}
	}

	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
	req.OutputWriter = chatStdout

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
	if shared.Verbose {
		printUsageStats(result, elapsed)
	}

	return nil
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
	fmt.Fprintln(chatStderr, "---  "+strings.Join(parts, "  |  "))
}

// toURLs converts a single URL string to a slice (for MJ API compatibility).
func toURLs(url string) []string {
	if url == "" {
		return nil
	}
	return []string{url}
}
