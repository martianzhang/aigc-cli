package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// mustFind resolves a command from the root tree by name.
func mustFind(t *testing.T, name string) *cobra.Command {
	t.Helper()
	cmd, _, err := rootCmd.Find([]string{name})
	if err != nil {
		t.Fatalf("find %q: %v", name, err)
	}
	if cmd == nil {
		t.Fatalf("find %q: no command", name)
	}
	return cmd
}

// TestOwnsTerminal guards that the commands which take over the terminal are
// excluded from the shared title/Ctrl-C cleanup — including the bare `aigc-cli`
// invocation, whose RunE hands off to the chat TUI while still reporting the
// root command name.
func TestOwnsTerminal(t *testing.T) {
	// cobra registers the completion command lazily.
	rootCmd.InitDefaultCompletionCmd()

	cases := []struct {
		name string
		cmd  *cobra.Command
		want bool
	}{
		{name: "bare root runs the chat TUI", cmd: rootCmd, want: true},
		{name: "chat", cmd: mustFind(t, "chat"), want: true},
		{name: "mcp", cmd: mustFind(t, "mcp"), want: true},
		{name: "completion", cmd: mustFind(t, "completion"), want: true},
		{name: "image", cmd: mustFind(t, "image"), want: false},
		{name: "video", cmd: mustFind(t, "video"), want: false},
		{name: "models", cmd: mustFind(t, "models"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ownsTerminal(tc.cmd); got != tc.want {
				t.Errorf("ownsTerminal(%q) = %v, want %v", tc.cmd.Name(), got, tc.want)
			}
		})
	}
}
