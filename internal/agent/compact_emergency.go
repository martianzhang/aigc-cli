package agent

import (
	"encoding/json"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// EmergencyTrim is the last-resort local fallback used when compaction fails
// and the request no longer fits the model window. It drops the oldest complete
// turns and, if a single turn is still too large, truncates message text. It
// never calls an API and never leaves an orphan tool message.
//
// Returns the projected history, how many trailing messages were kept, how many
// messages were dropped, and whether anything changed.
func EmergencyTrim(history []types.ChatMessage, s Settings, toolsTokens int) ([]types.ChatMessage, int, int, bool) {
	s = s.withDefaults()
	if s.Window <= 0 {
		return history, 0, 0, false
	}
	budget := s.Threshold()
	if EstimateContext(history, nil, toolsTokens) <= budget {
		return history, 0, 0, false
	}

	system, previousSummary, messages := splitHistory(history)
	if len(messages) == 0 {
		return history, 0, 0, false
	}

	// Prefer the oldest cut that still fits. Starting at 0 drops everything but
	// the previous summary; user boundaries keep complete turns.
	cuts := []int{0}
	for i, m := range messages {
		if i > 0 && m.Role == "user" {
			cuts = append(cuts, i)
		}
	}
	chosen := -1
	for _, cut := range cuts {
		tail := messages[cut:]
		if !validCutTail(tail) {
			continue
		}
		cand := buildProjectedHistory(system, previousSummary, tail)
		if EstimateContext(cand, nil, toolsTokens) <= budget {
			chosen = cut
			break
		}
	}
	if chosen < 0 {
		chosen = lastUserIndex(messages)
		if chosen < 0 {
			chosen = 0
		}
	}

	tail := append([]types.ChatMessage(nil), messages[chosen:]...)
	if !validCutTail(tail) {
		// Advancing to the next user boundary restores tool pairing.
		if next := nextUserIndex(tail, 1); next > 0 {
			tail = tail[next:]
		}
	}

	changed := chosen > 0

	// Drop whole turns until the estimate fits.
	for len(tail) > 1 && EstimateContext(buildProjectedHistory(system, previousSummary, tail), nil, toolsTokens) > budget {
		next := nextUserIndex(tail, 1)
		if next <= 0 {
			break
		}
		nextTail := tail[next:]
		if !validCutTail(nextTail) {
			break
		}
		tail = nextTail
		changed = true
	}

	// A single oversized turn remains: shrink its largest message text.
	for i := 0; i < 8; i++ {
		cur := buildProjectedHistory(system, previousSummary, tail)
		est := EstimateContext(cur, nil, toolsTokens)
		if est <= budget {
			break
		}
		idx := largestMessageIndex(tail)
		if idx < 0 {
			break
		}
		over := est - budget
		target := EstimateMessageTokens(tail[idx]) - over
		if target < 128 {
			target = 128
		}
		beforeEst := EstimateMessageTokens(tail[idx])
		shrunk := shrinkMessage(tail[idx], target)
		if EstimateMessageTokens(shrunk) >= beforeEst {
			break
		}
		tail[idx] = shrunk
		changed = true
	}

	if !changed {
		return history, 0, 0, false
	}
	out := buildProjectedHistory(system, previousSummary, tail)
	return out, len(tail), len(messages) - len(tail), true
}

func nextUserIndex(messages []types.ChatMessage, from int) int {
	for i := from; i < len(messages); i++ {
		if messages[i].Role == "user" {
			return i
		}
	}
	return -1
}

func largestMessageIndex(messages []types.ChatMessage) int {
	best, bestN := -1, 0
	for i, m := range messages {
		n := EstimateMessageTokens(m)
		if n > bestN {
			best, bestN = i, n
		}
	}
	return best
}

func shrinkMessage(m types.ChatMessage, target int) types.ChatMessage {
	// Prefer shrinking visible text and tool output; tool-call arguments are
	// kept valid JSON via shrinkToolArgs.
	if m.Content != "" {
		maxChars := target * 3
		if len(m.Content) > maxChars {
			m.Content = truncateForSummary(m.Content, maxChars)
			return m
		}
	}
	if len(m.Images) > 0 {
		m.Images = nil
		if m.Content != "" {
			m.Content += "\n\n[image removed to fit the context window]"
		}
		return m
	}
	for i := range m.ToolCalls {
		args := shrinkToolArgs(m.ToolCalls[i].Function.Arguments, target*2)
		m.ToolCalls[i].Function.Arguments = args
	}
	return m
}

// shrinkToolArgs truncates the large string fields inside a tool call's JSON
// arguments while keeping the JSON valid.
func shrinkToolArgs(args string, maxChars int) string {
	if maxChars <= 0 || len(args) <= maxChars {
		return args
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(args), &obj); err != nil {
		// Keep historical tool calls syntactically valid for providers that
		// validate the transcript; the real arguments are already gone.
		return `{"truncated":true}`
	}
	for _, key := range []string{"content", "old_string", "new_string", "text", "body"} {
		if s, ok := obj[key].(string); ok && len(s) > maxChars {
			obj[key] = truncateForSummary(s, maxChars)
		}
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return truncateForSummary(args, maxChars)
	}
	return string(out)
}
