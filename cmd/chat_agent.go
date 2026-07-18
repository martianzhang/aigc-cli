package cmd

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runAgentLoop executes the tool-calling loop: send request -> check tool_calls -> execute -> repeat.
// history is modified in-place (appended with assistant + tool messages).
// Returns the final ChatResponse (text response) or error.
// If ctx is cancelled (Ctrl+C), returns immediately with context.Canceled.
func runAgentLoop(ctx context.Context, c *client.Client, history *[]types.ChatMessage, agentTools []types.ToolDefinition, maxIterations int, cmd *cobra.Command) (*types.ChatResponse, error) {
	// Merge defaults.chat.model into shared.Model if empty
	if shared.Model == "" && shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Chat != nil {
		shared.Model = shared.Cfg.Defaults.Chat.Model
	}

	turnCount := 0
	agentStart := time.Now()
	for turnCount < maxIterations {
		// Check for Ctrl+C between turns
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		turnCount++

		req := &types.ChatRequest{
			Model:        shared.Model,
			Messages:     *history,
			Stream:       true,
			OutputWriter: chatStdout,
		}
		if len(agentTools) > 0 {
			req.Tools = agentTools
		}
		setFloatFlag(cmd, "temperature", &req.Temperature, chatTemperature)
		setIntFlag(cmd, "max-tokens", &req.MaxTokens, chatMaxTokens)

		if turnCount > 1 {
			fmt.Fprint(chatStderr, "\r\n---\r\n")
		}
		fmt.Fprint(chatStderr, "\r\n")

		result, err := c.ChatCompletion(req)
		if err != nil {
			return nil, err
		}

		if len(result.Choices) == 0 {
			break
		}
		choice := result.Choices[0]

		// Check for tool calls
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) > 0 {
			*history = append(*history, choice.Message)

			for _, tc := range choice.Message.ToolCalls {
				fmt.Fprintf(chatStderr, "\r\n[tool] %s:\r\n", tc.Function.Name)
				printToolArgs(tc.Function.Arguments)

				toolStart := time.Now()
				toolResult := executeToolCall(c, tc)
				elapsed := time.Since(toolStart).Round(time.Millisecond)

				resultSummary := summarizeToolResult(tc.Function.Name, toolResult)
				fmt.Fprintf(chatStderr, "\r\n[tool] done in %v: %s\r\n", elapsed, resultSummary)

				*history = append(*history, types.ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    toolResult,
				})
			}
			continue
		}

		// Text response -- already streamed to stdout by handleSSE
		*history = append(*history, choice.Message)

		if shared.Verbose {
			printUsageStats(result, time.Since(agentStart))
		}

		if turnCount >= maxIterations {
			fmt.Fprintf(chatStderr, "\r\nReached maximum iterations (%d). Start a new message to continue.\r\n", maxIterations)
		}

		return result, nil
	}

	return nil, nil
}

// buildAgentTools returns the list of tool definitions based on config.
// Applies tools (whitelist) and disable_tools (blacklist) glob patterns.
func buildAgentTools(cfg *types.ChatDefaults) []types.ToolDefinition {
	if cfg != nil && len(cfg.DisableTools) > 0 {
		for _, pattern := range cfg.DisableTools {
			if matched, _ := path.Match(pattern, "*"); matched {
				return nil
			}
		}
	}

	allTools := agentToolDefs

	// Apply whitelist (tools)
	if cfg != nil && len(cfg.Tools) > 0 {
		hasWildcard := false
		for _, pattern := range cfg.Tools {
			if matched, _ := path.Match(pattern, "*"); matched {
				hasWildcard = true
				break
			}
		}
		if !hasWildcard {
			filtered := make([]types.ToolDefinition, 0)
			for _, t := range allTools {
				for _, pattern := range cfg.Tools {
					if matched, _ := path.Match(pattern, t.Function.Name); matched {
						filtered = append(filtered, t)
						break
					}
				}
			}
			allTools = filtered
		}
	}

	// Apply blacklist (disable_tools)
	if cfg != nil && len(cfg.DisableTools) > 0 {
		filtered := make([]types.ToolDefinition, 0)
		for _, t := range allTools {
			disabled := false
			for _, pattern := range cfg.DisableTools {
				if matched, _ := path.Match(pattern, t.Function.Name); matched {
					disabled = true
					break
				}
			}
			if !disabled {
				filtered = append(filtered, t)
			}
		}
		allTools = filtered
	}

	// Filter by provider
	isAPIMart := isAPIMartProvider()

	providerFiltered := make([]types.ToolDefinition, 0)
	for _, t := range allTools {
		if strings.HasPrefix(t.Function.Name, "midjourney") && !isAPIMart {
			continue
		}
		if (t.Function.Name == "balance" || t.Function.Name == "task") && !isAPIMart {
			continue
		}
		providerFiltered = append(providerFiltered, t)
	}

	return providerFiltered
}
