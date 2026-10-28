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
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/session"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runAgentLoop executes the tool-calling loop without persisting a session.
func runAgentLoop(ctx context.Context, c *client.Client, rt *toolRuntime, history *[]types.ChatMessage, maxIterations int, cmd *cobra.Command, p *provider.EffectiveProvider) error {
	return runAgentLoopWithSession(ctx, c, rt, history, maxIterations, cmd, p, nil, 0)
}

// runAgentLoopWithSession executes the tool-calling loop and appends every
// message (user messages are written by the caller) to sess when it is non-nil.
// history is modified in-place (appended with assistant + tool messages).
// If ctx is cancelled (Ctrl+C), returns immediately with context.Canceled.
func runAgentLoopWithSession(ctx context.Context, c *client.Client, rt *toolRuntime, history *[]types.ChatMessage, maxIterations int, cmd *cobra.Command, p *provider.EffectiveProvider, sess *session.Session, toolOutputMax int) error {
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
			return ctx.Err()
		default:
		}
		turnCount++

		// Auto-compact before building request if context is getting full
		newHistory, compaction := agent.AutoCompactIfNeededResult(c, options.Shared.Model, chatTemperature, chatMaxTokens, chatContextSize, *history, options.Stderr())
		*history = newHistory
		if sess != nil && compaction != nil {
			if err := sess.AppendCompaction(compaction.Summary, compaction.Tokens+compaction.TokenSaved, compaction.Tokens, options.Shared.Model); err != nil {
				return err
			}
		}

		req := &types.ChatRequest{
			Model:        options.Shared.Model,
			Messages:     *history,
			Stream:       !chatNoStream,
			OutputWriter: options.Stdout(),
			InjectJSON:   chatInjectRaw,
		}
		if tools := rt.RequestTools(); len(tools) > 0 {
			req.Tools = tools
		}
		options.SetFloatFlag(cmd, "temperature", &req.Temperature, chatTemperature)
		options.SetIntFlag(cmd, "max-output", &req.MaxTokens, chatMaxTokens)

		if turnCount > 1 {
			// No leading blank line: the streamed reply already ended with a
			// newline (or produced no output at all for a tool-only turn).
			fmt.Fprint(options.Stderr(), "---\r\n")
		}

		result, err := c.ChatCompletion(req)
		if err != nil {
			return err
		}

		if len(result.Choices) == 0 {
			break
		}
		choice := result.Choices[0]

		// Check for tool calls
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) > 0 {
			*history = append(*history, choice.Message)
			if sess != nil {
				if err := sess.AppendMessage(choice.Message, "", toolOutputMax); err != nil {
					return err
				}
			}

			for _, tc := range choice.Message.ToolCalls {
				fmt.Fprintf(options.Stderr(), "[tool] %s:\r\n", tc.Function.Name)
				printToolArgs(tc.Function.Arguments)

				toolStart := time.Now()
				toolResult := rt.Execute(ctx, c, tc)
				elapsed := time.Since(toolStart).Round(time.Millisecond)

				resultSummary := summarizeToolResult(tc.Function.Name, toolResult)
				fmt.Fprintf(options.Stderr(), "[tool] done in %v: %s\r\n", elapsed, resultSummary)

				toolMessage := types.ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    toolResult,
				}
				*history = append(*history, toolMessage)
				if sess != nil {
					if err := sess.AppendMessage(toolMessage, tc.Function.Name, toolOutputMax); err != nil {
						return err
					}
				}
			}
			continue
		}

		// Text response: streamed by handleSSE when streaming, printed here otherwise.
		// A whitespace-only reply (the assistant content of a tool-call turn) is
		// dropped rather than printed as a run of blank lines.
		if chatNoStream && strings.TrimSpace(choice.Message.Content) != "" {
			fmt.Fprintln(options.Stdout(), choice.Message.Content)
		}
		*history = append(*history, choice.Message)
		if sess != nil {
			if err := sess.AppendMessage(choice.Message, "", toolOutputMax); err != nil {
				return err
			}
		}

		// Web-search citations (to stderr, when the provider attached any)
		printSources(result)

		if options.Shared.Verbose {
			printUsageStats(result, time.Since(agentStart), p)
		}

		if turnCount >= maxIterations {
			fmt.Fprintf(options.Stderr(), "Reached maximum iterations (%d). Start a new message to continue.\r\n", maxIterations)
		}

		return nil
	}

	return nil
}

// buildAgentTools returns the built-in tool definitions after filtering.
func buildAgentTools(cfg *types.ChatDefaults) []types.ToolDefinition {
	return filterTools(agent.ToolDefs, cfg)
}

// filterTools applies the primitive gating, the global tools_enable/tools_disable,
// the chat-specific tools/disable_tools, and the provider filter to a merged tool
// list (built-in primitives plus any tools contributed by MCP servers).
//
// Primitive gating:
//   - read-only primitives (read/grep/find/ls) are on by default;
//   - privileged primitives (write/edit/bash/search/fetch/web_search/web_fetch)
//     require an
//     explicit tools_enable entry;
//   - tools_disable always wins.
func filterTools(allTools []types.ToolDefinition, cfg *types.ChatDefaults) []types.ToolDefinition {
	// Chat-specific disable all (e.g. disable_tools: ["*"])
	if hashWildcard(chatDisable(cfg)) {
		return nil
	}

	var globalEnable, globalDisable []string
	if options.Shared.Cfg != nil {
		globalEnable = options.Shared.Cfg.ToolsEnable
		globalDisable = options.Shared.Cfg.ToolsDisable
	}
	chatEnable := chatTools(cfg)
	chatRestrict := len(chatEnable) > 0 && !hashWildcard(chatEnable)

	out := make([]types.ToolDefinition, 0, len(allTools))
	for _, t := range allTools {
		name := t.Function.Name

		if matchTool(name, globalDisable) || matchTool(name, chatDisable(cfg)) {
			continue
		}

		switch {
		case agent.IsReadOnlyPrimitive(name):
			// On by default; only the chat whitelist can exclude it.
			if chatRestrict && !matchTool(name, chatEnable) {
				continue
			}
		case agent.IsPrivilegedPrimitive(name):
			// Requires explicit tools_enable opt-in.
			if !matchTool(name, globalEnable) {
				continue
			}
			if chatRestrict && !matchTool(name, chatEnable) {
				continue
			}
		default:
			// Non-primitive (MCP) tools: global allowlist if set, then chat whitelist.
			if len(globalEnable) > 0 && !matchTool(name, globalEnable) {
				continue
			}
			if chatRestrict && !matchTool(name, chatEnable) {
				continue
			}
		}
		out = append(out, t)
	}

	// Filter by provider — use the resolved chat provider, not global state.
	isAPIMart := options.IsAPIMartProvider(options.Shared.ResolveProvider(options.ProviderNameChat))

	providerFiltered := make([]types.ToolDefinition, 0, len(out))
	for _, t := range out {
		if strings.HasPrefix(t.Function.Name, "midjourney") && !isAPIMart {
			continue
		}
		switch t.Function.Name {
		case "balance", "task", "get_balance", "get_task":
			if !isAPIMart {
				continue
			}
		}
		providerFiltered = append(providerFiltered, t)
	}

	return providerFiltered
}

func chatTools(cfg *types.ChatDefaults) []string {
	if cfg == nil {
		return nil
	}
	return cfg.Tools
}

func chatDisable(cfg *types.ChatDefaults) []string {
	if cfg == nil {
		return nil
	}
	return cfg.DisableTools
}

// matchTool reports whether name matches any glob pattern.
func matchTool(name string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// hashWildcard reports whether any pattern matches every name ("*").
func hashWildcard(patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, "*"); ok {
			return true
		}
	}
	return false
}
