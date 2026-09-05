package cmd

import (
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// estimateTokens provides a rough token count estimate for a string.
// Uses a simple heuristic: ~2.5 chars per token (works for mixed CJK/English).
func estimateTokens(s string) int {
	return len(s) / 2
}

// estimateHistoryTokens estimates total tokens for a chat history.
func estimateHistoryTokens(history []types.ChatMessage) int {
	total := 0
	for _, msg := range history {
		total += estimateTokens(msg.Content)
	}
	return total
}

// compactResult holds the result of a compaction operation.
type compactResult struct {
	History    []types.ChatMessage
	Tokens     int // estimated tokens after compaction
	Summary    string
	TokenSaved int // estimated tokens saved
}

// compactConversation sends the full history to the API for summarization
// and returns a condensed history. The caller must provide a client and model.
// Returns nil if compaction fails or is unnecessary.
func compactConversation(c *client.Client, model string, temperature float64, maxTokens int, history []types.ChatMessage) *compactResult {
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
		fmt.Fprintf(chatStderr, "\r\n[compact] failed: %v\r\n", err)
		return nil
	}
	if len(result.Choices) == 0 {
		fmt.Fprintln(chatStderr, "\r\n[compact] API returned no choices")
		return nil
	}

	summary := result.Choices[0].Message.Content
	oldTokens := estimateHistoryTokens(history)

	// Build compacted history: keep system message + summary
	var newHistory []types.ChatMessage
	if len(history) > 0 && history[0].Role == "system" {
		newHistory = append(newHistory, history[0])
	}
	newHistory = append(newHistory, types.ChatMessage{
		Role:    "system",
		Content: "Previous conversation summary:\n\n" + summary,
	})

	newTokens := estimateHistoryTokens(newHistory)
	return &compactResult{
		History:    newHistory,
		Tokens:     newTokens,
		Summary:    summary,
		TokenSaved: oldTokens - newTokens,
	}
}

// autoCompactIfNeeded checks if the history exceeds the context limit and
// compacts if necessary. Returns the (possibly compacted) history.
// Trigger threshold: 80% of contextSize.
func autoCompactIfNeeded(c *client.Client, model string, temperature float64, maxTokens int, contextSize int, history []types.ChatMessage) []types.ChatMessage {
	if contextSize <= 0 {
		return history
	}

	currentTokens := estimateHistoryTokens(history)
	threshold := int(float64(contextSize) * 0.8)
	if currentTokens <= threshold {
		return history
	}

	fmt.Fprintf(chatStderr, "\r\n[auto-compact] context %d tokens > %d threshold (limit %d), compacting...\r\n",
		currentTokens, threshold, contextSize)

	result := compactConversation(c, model, temperature, maxTokens, history)
	if result == nil {
		fmt.Fprintln(chatStderr, "\r\n[auto-compact] compaction failed, continuing with full history")
		return history
	}

	fmt.Fprintf(chatStderr, "\r\n[auto-compact] compacted: %d → %d tokens (saved %d)\r\n",
		currentTokens, result.Tokens, result.TokenSaved)
	return result.History
}
