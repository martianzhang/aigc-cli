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
	confirm, err := newConfirmPolicy(options.ChatDefaults())
	if err != nil {
		return err
	}
	return runAgentLoopWithSession(ctx, c, rt, history, maxIterations, cmd, p, nil, 0, agent.Settings{}, confirm)
}

// runAgentLoopWithSession executes the tool-calling loop and appends every
// message (user messages are written by the caller) to sess when it is non-nil.
// history is modified in-place (appended with assistant + tool messages).
// If ctx is cancelled (Ctrl+C), returns immediately with context.Canceled.
//
// This is the non-interactive path, so confirm-gated tools fail closed unless
// --yes was passed: the model receives a blocked tool result instead of the
// command output and can explain how to proceed.
func runAgentLoopWithSession(ctx context.Context, c *client.Client, rt *toolRuntime, history *[]types.ChatMessage, maxIterations int, cmd *cobra.Command, p *provider.EffectiveProvider, sess *session.Session, toolOutputMax int, settings agent.Settings, confirm *confirmPolicy) error {
	// Merge defaults.chat.model into options.Shared.Model if empty
	if options.Shared.Model == "" {
		if cfg := options.ChatDefaults(); cfg != nil && cfg.Model != "" {
			options.Shared.Model = cfg.Model
		}
	}

	turnCount := 0
	agentStart := time.Now()
	var anchor *agent.UsageAnchor
	cooldown := 0

	for turnCount < maxIterations {
		// Check for Ctrl+C between turns
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		turnCount++

		tools := rt.RequestTools()
		toolsTokens := agent.EstimateToolDefinitions(tools) + agent.EstimateRequestExtras(chatInjectRaw)

		// Auto-compact before building the request if the context is getting
		// full. This also covers a resumed session whose stored history is
		// already close to the limit.
		if cooldown > 0 {
			cooldown--
		} else if settings.Enabled && settings.Window > 0 {
			opts := compactionOptions(settings, anchor, toolsTokens, sess, options.Shared.Model, "threshold", false, options.Stderr())
			res, cerr := agent.AutoCompact(ctx, c, *history, opts)
			if cerr != nil {
				cooldown = 3
				fmt.Fprintf(options.Stderr(), "[auto-compact] failed: %v\r\n", cerr)
			} else if res != nil {
				*history = res.History
				anchor = nil
				if err := persistCompaction(sess, res, options.Shared.Model); err != nil {
					return err
				}
			}
		}

		if turnCount > 1 {
			// No leading blank line: the streamed reply already ended with a
			// newline (or produced no output at all for a tool-only turn).
			fmt.Fprint(options.Stderr(), "---\r\n")
		}

		var result *types.ChatResponse
		var err error
		recovered := false
		for attempt := 0; attempt < 2; attempt++ {
			req := &types.ChatRequest{
				Model:        options.Shared.Model,
				Messages:     *history,
				Stream:       !chatNoStream,
				OutputWriter: options.Stdout(),
				InjectJSON:   chatInjectRaw,
			}
			if len(tools) > 0 {
				req.Tools = tools
			}
			options.SetFloatFlag(cmd, "temperature", &req.Temperature, chatTemperature)
			options.SetIntFlag(cmd, "max-output", &req.MaxTokens, chatMaxTokens)

			result, err = c.ChatCompletion(req)
			if err == nil {
				break
			}
			if recovered || !agent.IsContextOverflow(err) {
				break
			}
			recovered = true
			fmt.Fprintf(options.Stderr(), "[auto-compact] context overflow; compacting and retrying once\r\n")
			opts := compactionOptions(settings, anchor, toolsTokens, sess, options.Shared.Model, "overflow", true, options.Stderr())
			res, cerr := agent.AutoCompact(ctx, c, *history, opts)
			if cerr != nil || res == nil {
				trimmed, keepLast, dropped, ok := agent.EmergencyTrim(*history, settings, toolsTokens)
				if !ok {
					break
				}
				before := agent.EstimateContext(*history, nil, toolsTokens)
				summary := agent.PreviousSummary(trimmed)
				if summary == "" {
					summary = "Context was trimmed locally after compaction failed."
				}
				if dropped > 0 {
					if perr := persistCompaction(sess, &agent.CompactResult{
						Summary: summary, KeepLast: keepLast, TokensBefore: before,
						Tokens: agent.EstimateContext(trimmed, nil, toolsTokens),
						Reason: "emergency", SummaryModel: options.Shared.Model,
					}, options.Shared.Model); perr != nil {
						return perr
					}
				}
				*history = trimmed
				anchor = nil
				fmt.Fprintf(options.Stderr(), "[auto-compact] dropped %d oldest messages locally\r\n", dropped)
				continue
			}
			*history = res.History
			anchor = nil
			if err := persistCompaction(sess, res, options.Shared.Model); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
		if result == nil {
			return fmt.Errorf("chat API returned no response")
		}

		// The provider-reported prompt size is the best anchor for the next
		// turn; messages appended after it are added by estimate.
		if result.Usage != nil && result.Usage.PromptTokens > 0 {
			anchor = &agent.UsageAnchor{MessageIndex: len(*history), PromptTokens: result.Usage.PromptTokens}
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

				var toolResult string
				if confirm.needs(tc.Function.Name, tc.Function.Arguments) && !chatYes {
					toolResult = blockedToolResult(tc.Function.Name)
					fmt.Fprintf(options.Stderr(), "[tool] blocked: requires confirmation (rerun with --yes)\r\n")
				} else {
					toolStart := time.Now()
					toolResult = rt.Execute(ctx, c, tc)
					elapsed := time.Since(toolStart).Round(time.Millisecond)

					resultSummary := summarizeToolResult(tc.Function.Name, toolResult)
					fmt.Fprintf(options.Stderr(), "[tool] done in %v: %s\r\n", elapsed, resultSummary)
				}

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
			fmt.Fprintf(options.Stderr(), "%s\r\n", maxIterationsNotice(maxIterations))
		}

		return nil
	}

	// The budget ran out on a tool-call turn (the tools ran, the next round-trip
	// was refused). Say so instead of returning silently with the task half-done.
	if turnCount >= maxIterations {
		fmt.Fprintf(options.Stderr(), "%s\r\n", maxIterationsNotice(maxIterations))
	}

	return nil
}

// maxIterationsNotice is the user-facing message shown when one user message
// has consumed the whole tool-calling budget.
func maxIterationsNotice(maxIterations int) string {
	return fmt.Sprintf("Reached maximum iterations (%d). Start a new message to continue.", maxIterations)
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
