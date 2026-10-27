package cmd

import (
	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/terminal"
)

// titleForCommand shows the running command in the terminal tab from the moment
// it starts, so long silent waits are never invisible:
//
//	aigc-cli · image … 42s
//
// Providers that expose a percentage refine the title later; ones that just
// queue a job (ModelScope, OpenRouter video) at least show elapsed time.
// Progress is cleared on exit by Execute.
func titleForCommand(cmd *cobra.Command) {
	prog := terminal.Default()
	if !prog.Enabled() || cmd.Name() == "help" {
		return
	}
	prog.BusyStage(cmd.Name())
}

// ownsTerminal reports whether the command being executed manages its own
// terminal (alt screen, raw mode, signal handling), so the shared tab indicator
// and Ctrl-C cleanup must stay out of its way.
//
// A bare `aigc-cli` has no subcommand: root's RunE hands off to the chat TUI, so
// it owns the terminal exactly like `aigc-cli chat` does even though the
// executed command is the root one.
func ownsTerminal(cmd *cobra.Command) bool {
	if cmd.Root() == cmd {
		return true
	}
	switch cmd.Name() {
	case "chat", "mcp", "completion":
		return true
	}
	return false
}
