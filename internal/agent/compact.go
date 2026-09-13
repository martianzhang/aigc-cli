package agent

import (
	"fmt"
	"io"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// EstimateTokens provides a rough token count estimate for a string.
func EstimateTokens(s string) int {
	return len(s) / 2
}

// EstimateHistoryTokens estimates total tokens for a chat history.
func EstimateHistoryTokens(history []types.ChatMessage) int {
	total := 0
	for _, msg := range history {
		total += EstimateTokens(msg.Content)
	}
	return total
}

// CompactResult holds the result of a compaction operation.
type CompactResult struct {
	History    []types.ChatMessage
	Tokens     int
	Summary    string
	TokenSaved int
}

// CompactConversation summarizes the full history and returns a condensed
// history. Returns nil when compaction is unnecessary or fails. stderr receives
// progress/error lines.
func CompactConversation(c *client.Client, model string, temperature float64, maxTokens int, history []types.ChatMessage, stderr io.Writer) *CompactResult {
	if len(history) < 4 {
		return nil // too few messages to compact
	}

	summarizePrompt := `Please provide a detailed summary of our conversation above. Capture: 1) the user's goals and requirements, 2) key decisions made, 3) any files created or modified, 4) current status of any ongoing work. Be thorough — this summary will replace the conversation history so nothing important should be lost.`

	req := &types.ChatRequest{
		Model:    model,
		Messages: append(history, types.ChatMessage{Role: "user", Content: summarizePrompt}),
		Stream:   false,
	}
	if temperature > 0 {
		t := temperature
		req.Temperature = &t
	}
	if maxTokens > 0 {
		t := maxTokens
		req.MaxTokens = &t
	}

	result, err := c.ChatCompletion(req)
	if err != nil {
		fmt.Fprintf(stderr, "\r\n[compact] failed: %v\r\n", err)
		return nil
	}
	if len(result.Choices) == 0 {
		fmt.Fprintln(stderr, "\r\n[compact] API returned no choices")
		return nil
	}

	summary := result.Choices[0].Message.Content
	oldTokens := EstimateHistoryTokens(history)

	var newHistory []types.ChatMessage
	if len(history) > 0 && history[0].Role == "system" {
		newHistory = append(newHistory, history[0])
	}
	newHistory = append(newHistory, types.ChatMessage{
		Role:    "system",
		Content: "Previous conversation summary:\n\n" + summary,
	})

	newTokens := EstimateHistoryTokens(newHistory)
	return &CompactResult{
		History:    newHistory,
		Tokens:     newTokens,
		Summary:    summary,
		TokenSaved: oldTokens - newTokens,
	}
}

// AutoCompactIfNeeded compacts the history when it exceeds 80% of contextSize.
func AutoCompactIfNeeded(c *client.Client, model string, temperature float64, maxTokens int, contextSize int, history []types.ChatMessage, stderr io.Writer) []types.ChatMessage {
	if contextSize <= 0 {
		return history
	}

	currentTokens := EstimateHistoryTokens(history)
	threshold := int(float64(contextSize) * 0.8)
	if currentTokens <= threshold {
		return history
	}

	fmt.Fprintf(stderr, "\r\n[auto-compact] context %d tokens > %d threshold (limit %d), compacting...\r\n",
		currentTokens, threshold, contextSize)

	result := CompactConversation(c, model, temperature, maxTokens, history, stderr)
	if result == nil {
		fmt.Fprintln(stderr, "\r\n[auto-compact] compaction failed, continuing with full history")
		return history
	}

	fmt.Fprintf(stderr, "\r\n[auto-compact] compacted: %d → %d tokens (saved %d)\r\n",
		currentTokens, result.Tokens, result.TokenSaved)
	return result.History
}
