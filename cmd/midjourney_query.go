package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// ============================================================================
// Subcommand: query
// ============================================================================
var mjQueryCmd = &cobra.Command{
	Use:   "query <task-id>",
	Short: "Get MJ task status and result",
	Long: `Query a Midjourney task by its task ID.

Example:
  aigc-cli midjourney query task_xxx`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := args[0]
		c := newMJClient()
		task, err := c.MidjourneyGetTask(taskID)
		if err != nil {
			return fmt.Errorf("failed to query task: %w", err)
		}
		displayMJResult(task)

		// Download images if available
		if task.Status == "SUCCESS" && len(task.ImageURLs) > 0 {
			// Convert to ImageResult slices for downloadImages helper
			images := make([]types.ImageResult, len(task.ImageURLs))
			for i, u := range task.ImageURLs {
				images[i] = types.ImageResult{URL: []string{u}}
			}
			if _, err := downloadImages(images, task.ID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
			}
		}
		// Download videos if available
		if task.Status == "SUCCESS" && len(task.VideoURLs) > 0 {
			videos := make([]types.VideoResult, len(task.VideoURLs))
			for i, u := range task.VideoURLs {
				videos[i] = types.VideoResult{URL: []string{u}}
			}
			if _, err := downloadVideos(videos, task.ID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
			}
		}
		if task.Status == "SUCCESS" && task.VideoURL != "" {
			// Also download single video_url if present
			videos := []types.VideoResult{{URL: []string{task.VideoURL}}}
			if _, err := downloadVideos(videos, task.ID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
			}
		}
		return nil
	},
}
