package chat

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/session"
)

// validateListSessions rejects combinations that imply a different action. It
// checks FlagSet.Changed rather than the variables so config defaults such as
// defaults.chat.save do not turn a read-only listing into an error.
func validateListSessions(cmd *cobra.Command) error {
	if chatSessionLimit < 0 {
		return fmt.Errorf("--session-limit must be >= 0")
	}
	conflicts := []string{"save", "session", "name", "json", "dry-run", "interactive"}
	for _, name := range conflicts {
		if cmd != nil && cmd.Flags().Changed(name) {
			return fmt.Errorf("--list-sessions cannot be combined with --%s", name)
		}
	}
	if len(chatMessages) > 0 {
		return fmt.Errorf("--list-sessions cannot be combined with a message")
	}
	return nil
}

// runListSessions prints the saved sessions and exits. It never resolves a
// provider or calls the API, so it works without credentials.
func runListSessions(cmd *cobra.Command) error {
	cfg := options.ChatDefaults()
	return writeSessionList(cmd.OutOrStdout(), sessionDir(cfg), chatSessionLimit)
}

// writeSessionList renders summaries as text. limit <= 0 lists every session;
// the interactive /session view passes its own smaller limit.
func writeSessionList(out io.Writer, dir string, limit int) error {
	list, err := session.List(dir, limit)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Fprintf(out, "No saved sessions in %s\n", dir)
		return nil
	}

	fmt.Fprintf(out, "Sessions in %s\n", dir)
	for i, s := range list {
		parts := []string{s.DisplayName()}
		if s.Name != "" && s.Name != s.ID {
			parts = append(parts, s.ID)
		}
		model := s.Model
		if model == "" {
			model = "<API default>"
		}
		parts = append(parts,
			formatSessionAge(s.UpdatedAt),
			fmt.Sprintf("%d msgs", s.Messages),
			model,
		)
		fmt.Fprintf(out, "%2d. %s\n", i+1, strings.Join(parts, "  ·  "))
		if s.FirstUser != "" {
			fmt.Fprintf(out, "    %s\n", truncateTitle(s.FirstUser, 72))
		}
	}
	if limit > 0 {
		if total, err := session.Count(dir); err == nil && total > len(list) {
			fmt.Fprintf(out, "\nShowing the %d most recent of %d sessions; use --session-limit 0 to list all.\n", len(list), total)
		}
	}
	fmt.Fprintln(out, "\nResume: aigc-cli chat --session <id>")
	return nil
}
