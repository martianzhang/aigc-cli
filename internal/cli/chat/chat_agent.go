package chat

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runAgentLoop executes the tool-calling loop: send request -> check tool_calls -> execute -> repeat.
// history is modified in-place (appended with assistant + tool messages).
// Returns the final ChatResponse (text response) or error.
// If ctx is cancelled (Ctrl+C), returns immediately with context.Canceled.
func runAgentLoop(ctx context.Context, c *client.Client, history *[]types.ChatMessage, agentTools []types.ToolDefinition, maxIterations int, cmd *cobra.Command) (*types.ChatResponse, error) {
	// Merge defaults.chat.model into options.Shared.Model if empty
	if options.Shared.Model == "" {
		if cfg := options.ChatDefaults(); cfg != nil && cfg.Model != "" {
			options.Shared.Model = cfg.Model
		}
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

		// Auto-compact before building request if context is getting full
		*history = agent.AutoCompactIfNeeded(c, options.Shared.Model, chatTemperature, chatMaxTokens, chatContextSize, *history, options.Stderr())

		req := &types.ChatRequest{
			Model:        options.Shared.Model,
			Messages:     *history,
			Stream:       !chatNoStream,
			OutputWriter: options.Stdout(),
		}
		if len(agentTools) > 0 {
			req.Tools = agentTools
		}
		options.SetFloatFlag(cmd, "temperature", &req.Temperature, chatTemperature)
		options.SetIntFlag(cmd, "max-output", &req.MaxTokens, chatMaxTokens)

		if turnCount > 1 {
			fmt.Fprint(options.Stderr(), "\r\n---\r\n")
		}

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
				fmt.Fprintf(options.Stderr(), "\r\n[tool] %s:\r\n", tc.Function.Name)
				printToolArgs(tc.Function.Arguments)

				toolStart := time.Now()
				toolResult := executeToolCall(c, tc)
				elapsed := time.Since(toolStart).Round(time.Millisecond)

				resultSummary := summarizeToolResult(tc.Function.Name, toolResult)
				fmt.Fprintf(options.Stderr(), "\r\n[tool] done in %v: %s\r\n", elapsed, resultSummary)

				*history = append(*history, types.ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    toolResult,
				})
			}
			continue
		}

		// Text response: streamed by handleSSE when streaming, printed here otherwise.
		if chatNoStream && choice.Message.Content != "" {
			fmt.Fprintln(options.Stdout(), choice.Message.Content)
		}
		*history = append(*history, choice.Message)

		if options.Shared.Verbose {
			printUsageStats(result, time.Since(agentStart))
		}

		if turnCount >= maxIterations {
			fmt.Fprintf(options.Stderr(), "\r\nReached maximum iterations (%d). Start a new message to continue.\r\n", maxIterations)
		}

		return result, nil
	}

	return nil, nil
}

// buildAgentTools returns the list of tool definitions based on config.
// Applies global tools_enable/tools_disable first, then chat-specific tools/disable_tools.
func buildAgentTools(cfg *types.ChatDefaults) []types.ToolDefinition {
	// Chat-specific disable all (e.g. disable_tools: ["*"])
	if cfg != nil && len(cfg.DisableTools) > 0 {
		for _, pattern := range cfg.DisableTools {
			if matched, _ := path.Match(pattern, "*"); matched {
				return nil
			}
		}
	}

	allTools := agent.ToolDefs

	// Apply global tools_enable/tools_disable (from top-level config)
	if options.Shared.Cfg != nil {
		globalEnable := options.Shared.Cfg.ToolsEnable
		globalDisable := options.Shared.Cfg.ToolsDisable
		if len(globalEnable) > 0 || len(globalDisable) > 0 {
			filtered := make([]types.ToolDefinition, 0)
			for _, t := range allTools {
				if options.IsToolAllowed(t.Function.Name, globalEnable, globalDisable) {
					filtered = append(filtered, t)
				}
			}
			allTools = filtered
		}
	}

	// Apply chat-specific whitelist (tools) — can only further restrict
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

	// Apply chat-specific blacklist (disable_tools) — can only further restrict
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

	// Filter by provider — use the resolved chat provider, not global state.
	isAPIMart := options.IsAPIMartProvider(options.Shared.ResolveProvider(options.ProviderNameChat))

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
