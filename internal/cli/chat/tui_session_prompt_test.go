package chat

import (
	"strings"
	"testing"
)

// TestBuildTUISystemPromptOmitsDate guards that the TUI prompt no longer
// carries a dynamic "today is ..." line. The date changes daily (breaking
// prompt caches) and is frozen in persisted sessions.
func TestBuildTUISystemPromptOmitsDate(t *testing.T) {
	t.Chdir(t.TempDir())

	prev := chatSystem
	chatSystem = "You are terse."
	t.Cleanup(func() { chatSystem = prev })

	got := buildTUISystemPrompt()
	if strings.Contains(got, "今天是") || strings.Contains(strings.ToLower(got), "current date") {
		t.Fatalf("buildTUISystemPrompt() still contains a date: %q", got)
	}
	if !strings.Contains(got, "You are terse.") {
		t.Fatalf("buildTUISystemPrompt() dropped the custom system prompt: %q", got)
	}
}
