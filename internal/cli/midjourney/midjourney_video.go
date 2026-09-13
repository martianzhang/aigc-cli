package midjourney

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
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
			data, err := service.ReadInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJVideoRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if len(req.ImageURLs) > 0 {
				c := NewClient()
				resolved, err := c.ResolveLocalImages(req.ImageURLs)
				if err != nil {
					return fmt.Errorf("failed to resolve image-urls: %w", err)
				}
				req.ImageURLs = resolved
			}
			c := NewClient()
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
			c := NewClient()
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
		}
		c := NewClient()
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
		data, err := service.ReadInput(mjJSONInput)
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
		c := NewClient()
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
	c := NewClient()
	return runMJSubmitAndPoll(c, action, req)
}
