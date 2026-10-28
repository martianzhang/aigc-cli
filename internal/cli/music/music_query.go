package music

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ============================================================================
// Subcommand: query
// ============================================================================
var musicQueryCmd = &cobra.Command{
	Use:          "query <task-id>",
	Short:        "Get an APIMart music task status and result",
	SilenceUsage: true,
	Long: `Query a music task by its task ID (APIMart only; OpenRouter is synchronous).

Example:
  aigc-cli music query task_xxx`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		taskID := args[0]
		c := newMusicClient()
		task, err := c.MusicGetTask(taskID)
		if err != nil {
			return fmt.Errorf("failed to query task: %w", err)
		}
		displayMusicResult(task)

		if task.Result != nil && len(task.Result.Music) > 0 {
			if _, err := downloadMusics(task.Result.Music, task.ID); err != nil {
				return fmt.Errorf("download music: %w", err)
			}
		}
		return nil
	},
}
