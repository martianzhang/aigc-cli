package agent

import (
	"encoding/json"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// Token estimation is intentionally conservative but must never be off by an
// order of magnitude. ASCII text averages ~4 chars/token; CJK and other wide
// runes average closer to 1 token/rune. Images are charged a flat estimate
// because their wire cost is driven by dimensions, not by the base64 byte size.
const (
	defaultImageTokens  = 1200
	perMessageOverhead  = 4
	defaultToolsReserve = 0
)

// EstimateTokens estimates the token count of a string. It counts ASCII bytes
// and non-ASCII runes separately so English is not overestimated by 2x (the old
// len(s)/2) and Chinese is not mistaken for its UTF-8 byte length.
func EstimateTokens(s string) int {
	ascii, wide := 0, 0
	for _, r := range s {
		if r < 0x80 {
			ascii++
		} else {
			wide++
		}
	}
	return (ascii+3)/4 + wide
}

// EstimateImageTokens returns a fixed estimate for one attached image. Vision
// tokenizers charge by tiles, not by the base64 payload; a flat value avoids
// treating a 1 MB screenshot as hundreds of thousands of text tokens.
func EstimateImageTokens(_ types.ChatImage) int { return defaultImageTokens }

// EstimateMessageTokens estimates one message, including tool call arguments.
// Tool call arguments often contain entire file bodies (write/edit) and used to
// be invisible to the estimator.
func EstimateMessageTokens(m types.ChatMessage) int {
	n := perMessageOverhead
	n += EstimateTokens(m.Content)
	for _, tc := range m.ToolCalls {
		n += EstimateTokens(tc.Function.Name)
		n += EstimateTokens(tc.Function.Arguments)
	}
	n += EstimateTokens(m.ToolCallID)
	for _, a := range m.Annotations {
		n += EstimateTokens(a.Title) + EstimateTokens(a.URL)
	}
	for _, img := range m.Images {
		n += EstimateImageTokens(img)
	}
	return n
}

// EstimateHistoryTokens estimates the token count of a full chat history.
func EstimateHistoryTokens(history []types.ChatMessage) int {
	total := 0
	for _, msg := range history {
		total += EstimateMessageTokens(msg)
	}
	return total
}

// EstimateToolDefinitions estimates the request overhead of tool schemas.
func EstimateToolDefinitions(tools []types.ToolDefinition) int {
	if len(tools) == 0 {
		return defaultToolsReserve
	}
	data, err := json.Marshal(tools)
	if err != nil {
		return len(tools) * 200
	}
	return EstimateTokens(string(data))
}

// EstimateRequestExtras estimates non-message request fields that still consume
// input context (injected JSON, response format hints, etc.).
func EstimateRequestExtras(extras []byte) int {
	if len(extras) == 0 {
		return 0
	}
	return EstimateTokens(string(extras))
}

// UsageAnchor records the provider-reported prompt size for a request and the
// history length it corresponded to. Estimates for messages appended after the
// anchor are added on top, which keeps context accounting accurate even when
// the local tokenizer estimate is imperfect.
type UsageAnchor struct {
	MessageIndex int
	PromptTokens int
}

// EstimateContext returns the best available context size. Provider usage wins
// when an anchor is available; otherwise the full history is estimated.
func EstimateContext(history []types.ChatMessage, anchor *UsageAnchor, toolsTokens int) int {
	if anchor != nil && anchor.PromptTokens > 0 && anchor.MessageIndex >= 0 && anchor.MessageIndex <= len(history) {
		// PromptTokens already includes the system prompt, tool schemas and
		// every message in the request. Only messages appended after the
		// anchor need to be added; do not add toolsTokens again.
		total := anchor.PromptTokens
		for _, msg := range history[anchor.MessageIndex:] {
			total += EstimateMessageTokens(msg)
		}
		return total
	}
	return EstimateHistoryTokens(history) + toolsTokens
}

// TokenBreakdown is a diagnostic split of the estimated context.
type TokenBreakdown struct {
	Total       int
	System      int
	Tools       int
	User        int
	Assistant   int
	ToolResults int
	Images      int
}

// ContextUsage combines the best context estimate with a diagnostic breakdown.
type ContextUsage struct {
	Tokens     int
	Breakdown  TokenBreakdown
	AnchorUsed bool
	UsagePart  int
	TailPart   int
}

// AnalyzeContext estimates a history and reports where the tokens go.
func AnalyzeContext(history []types.ChatMessage, anchor *UsageAnchor, toolsTokens int) ContextUsage {
	var b TokenBreakdown
	b.Tools = toolsTokens
	for _, msg := range history {
		n := EstimateMessageTokens(msg)
		switch msg.Role {
		case "system":
			b.System += n
		case "user":
			b.User += n
		case "assistant":
			b.Assistant += n
		case "tool":
			b.ToolResults += n
		default:
			b.User += n
		}
		for _, img := range msg.Images {
			b.Images += EstimateImageTokens(img)
		}
	}
	b.Total = b.System + b.Tools + b.User + b.Assistant + b.ToolResults
	usage := ContextUsage{Tokens: EstimateContext(history, anchor, toolsTokens), Breakdown: b}
	if anchor != nil && anchor.PromptTokens > 0 && anchor.MessageIndex >= 0 && anchor.MessageIndex <= len(history) {
		usage.AnchorUsed = true
		usage.UsagePart = anchor.PromptTokens
		usage.TailPart = usage.Tokens - anchor.PromptTokens
	}
	return usage
}
