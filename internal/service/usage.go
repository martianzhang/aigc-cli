package service

import (
	"fmt"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// PrintUsage prints token usage and cost information.
func PrintUsage(usage *types.OpenAIImageUsage) {
	if usage == nil {
		return
	}
	var parts []string
	if usage.PromptTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d in", usage.PromptTokens))
	}
	if usage.CompletionTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d out", usage.CompletionTokens))
	}
	if usage.TotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d total", usage.TotalTokens))
	}
	tokenStr := ""
	if len(parts) > 0 {
		tokenStr = strings.Join(parts, " / ")
	}
	if tokenStr != "" || usage.Cost > 0 {
		if tokenStr != "" {
			fmt.Printf("Tokens: %s", tokenStr)
		}
		if usage.Cost > 0 {
			if tokenStr != "" {
				fmt.Printf(" | ")
			}
			fmt.Printf("Cost: $%.5f", usage.Cost)
		}
		fmt.Println()
	}
}
