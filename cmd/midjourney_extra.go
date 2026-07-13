package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/types"
)

// ============================================================================
// Subcommand: video
// ============================================================================
var mjVideoCmd = &cobra.Command{
	Use:   "video",
	Short: "Image-to-video",
	Long: `Generate a video from an image using MJ's image-to-video (i2v).
Text-to-video is NOT supported — a first frame is required.

Examples:
  aigc-cli midjourney video --image-url cat.png --batch-size 4
  aigc-cli midjourney video --task-id task_xxx --index 0 --animate-mode auto`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJVideoRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if len(req.ImageURLs) > 0 {
				c := newMJClient()
				resolved, err := c.ResolveLocalImages(req.ImageURLs)
				if err != nil {
					return fmt.Errorf("failed to resolve image-urls: %w", err)
				}
				req.ImageURLs = resolved
			}
			c := newMJClient()
			return runMJSubmitAndPoll(c, "video", req)
		}

		if len(mjImageURLs) == 0 && mjTaskID == "" {
			return fmt.Errorf("either --image-url or --task-id is required for video")
		}

		req := &types.MJVideoRequest{
			Prompt:      mjPrompt,
			ImageURLs:   mjImageURLs,
			TaskID:      mjTaskID,
			VideoType:   mjVideoType,
			AnimateMode: mjAnimateMode,
			Motion:      mjMotion,
			EndURL:      mjEndURL,
		}
		if cmd.Flags().Changed("index") {
			v := mjIndex
			req.Index = &v
		}
		if cmd.Flags().Changed("batch-size") {
			v := mjBatchSize
			req.BatchSize = &v
		}

		if len(req.ImageURLs) > 0 {
			c := newMJClient()
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
		}
		c := newMJClient()
		return runMJSubmitAndPoll(c, "video", req)
	},
}

// ============================================================================
// Subcommand: remix-strong / remix-subtle
// ============================================================================
var mjRemixStrongCmd = &cobra.Command{
	Use:   "remix-strong",
	Short: "Strong reshape (v8/v8.1 only)",
	Long: `Strong reshape of a v8/v8.1 parent image. Large change; composition/style may shift.

Example:
  aigc-cli midjourney remix-strong --task-id task_xxx --index 1`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMJRmix(cmd, "remix-strong")
	},
}

var mjRemixSubtleCmd = &cobra.Command{
	Use:   "remix-subtle",
	Short: "Subtle reshape (v8/v8.1 only)",
	Long: `Subtle reshape of a v8/v8.1 parent image. Small change; keeps subject/tone.

Example:
  aigc-cli midjourney remix-subtle --task-id task_xxx --index 1 --prompt "new style"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMJRmix(cmd, "remix-subtle")
	},
}

func runMJRmix(cmd *cobra.Command, action string) error {
	if mjJSONInput != "" {
		data, err := readInput(mjJSONInput)
		if err != nil {
			return fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.MJRemixRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return fmt.Errorf("failed to parse JSON: %w", err)
		}
		if req.TaskID == "" {
			return fmt.Errorf("task_id is required")
		}
		c := newMJClient()
		return runMJSubmitAndPoll(c, action, req)
	}

	if mjTaskID == "" {
		return fmt.Errorf("--task-id is required for remix")
	}

	req := &types.MJRemixRequest{
		TaskID: mjTaskID,
		Prompt: mjPrompt,
		Speed:  mjSpeed,
	}
	if cmd.Flags().Changed("index") {
		v := mjIndex
		req.Index = &v
	}
	c := newMJClient()
	return runMJSubmitAndPoll(c, action, req)
}

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
