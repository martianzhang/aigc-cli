package agent

import "github.com/martianzhang/aigc-cli/internal/types"

// cutPoint describes where a compaction checkpoint starts keeping raw messages.
// firstKept is an index into the non-system conversation slice; keepLast is the
// number of trailing messages that must survive verbatim.
type cutPoint struct {
	firstKept int
	keepLast  int
	isSplit   bool
}

// findCutPoint walks backwards from the newest message until roughly
// keepRecent tokens have been accumulated, then snaps to a safe message
// boundary. Tool results are never valid cut points: cutting there would leave
// an orphan tool message without its assistant tool_calls.
//
// The returned firstKept is always > 0 when there is at least one message that
// can be summarized. Callers treat firstKept == 0 as "nothing to compact".
func findCutPoint(messages []types.ChatMessage, keepRecent int) cutPoint {
	if len(messages) == 0 {
		return cutPoint{}
	}
	if keepRecent < 0 {
		keepRecent = 0
	}

	target := -1
	accumulated := 0
	for i := len(messages) - 1; i >= 0; i-- {
		n := EstimateMessageTokens(messages[i])
		if n == 0 {
			continue
		}
		accumulated += n
		if accumulated >= keepRecent {
			target = i
			break
		}
	}
	if target < 0 {
		// The whole conversation fits in the recent budget: keep everything.
		return cutPoint{firstKept: 0, keepLast: len(messages)}
	}

	first := -1
	for i := target; i < len(messages); i++ {
		role := messages[i].Role
		if role != "tool" && role != "system" {
			first = i
			break
		}
	}
	if first <= 0 {
		// No safe point inside the budget. Fall back to the start of the most
		// recent user turn so the active request stays verbatim. If even that
		// is index 0, there is nothing older to summarize.
		first = lastUserIndex(messages)
		if first <= 0 {
			return cutPoint{firstKept: 0, keepLast: len(messages)}
		}
	}

	// An assistant tool_call with no matching tool result cannot start the
	// retained tail. Advance to the next safe message (normally its tool
	// result), keeping pairing intact.
	for first < len(messages) && !validCutTail(messages[first:]) {
		first++
	}
	if first <= 0 {
		return cutPoint{firstKept: 0, keepLast: len(messages)}
	}
	if first >= len(messages) {
		return cutPoint{firstKept: len(messages), keepLast: 0}
	}

	return cutPoint{
		firstKept: first,
		keepLast:  len(messages) - first,
		isSplit:   messages[first].Role != "user",
	}
}

func lastUserIndex(messages []types.ChatMessage) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return i
		}
	}
	return -1
}

// validCutTail reports whether tail can be sent as-is after the cut. It is used
// by tests and by the emergency trimmer; production callers use findCutPoint.
func validCutTail(tail []types.ChatMessage) bool {
	open := map[string]bool{}
	for _, m := range tail {
		if m.Role == "assistant" {
			for _, tc := range m.ToolCalls {
				open[tc.ID] = true
			}
		}
		if m.Role == "tool" && m.ToolCallID != "" {
			if !open[m.ToolCallID] {
				return false
			}
			delete(open, m.ToolCallID)
		}
	}
	return len(open) == 0
}
