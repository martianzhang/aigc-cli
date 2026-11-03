package cmd

import "testing"

// TestRootCommandHasChatFlags guards the default-chat contract: chat flags must
// be accepted without the "chat" subcommand.
func TestRootCommandHasChatFlags(t *testing.T) {
	names := []string{
		"system", "message", "prompt", "temperature", "max-output",
		"context-size", "no-stream", "json", "interactive", "dry-run",
		"search-provider", "inject-json", "save", "session-dir", "session", "name",
	}
	for _, name := range names {
		if rootCmd.Flags().Lookup(name) == nil {
			t.Errorf("root command is missing chat flag --%s", name)
		}
	}
}

// TestRootCommandAcceptsPositionalArgs guards that unknown positional text
// reaches RunDefault instead of being rejected as an unknown subcommand.
func TestRootCommandAcceptsPositionalArgs(t *testing.T) {
	if rootCmd.Args == nil {
		t.Fatal("root command Args is nil; positional text would be rejected as an unknown subcommand")
	}
	if err := rootCmd.Args(rootCmd, []string{"hello", "world"}); err != nil {
		t.Fatalf("root command Args rejected a positional message: %v", err)
	}
}
