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
