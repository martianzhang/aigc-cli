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
			data, err := service.ReadJSONInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJVideoRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			merged, err := mjVideoOverlay(cmd).apply(data, req)
			if err != nil {
				return err
			}
			req.RawJSON = merged
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
	Long:  `Strong reshape of a v8/v8.1 parent image. Large change; composition/style may shift.`,
	Example: `  aigc-cli midjourney remix-strong --task-id task_xxx --index 1
  aigc-cli midjourney remix-strong --task-id task_xxx --index 1 --prompt "neon cyberpunk city"
  aigc-cli midjourney remix-strong --task-id task_xxx --index 1 --speed fast`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMJRmix(cmd, "remix-strong")
	},
}

var mjRemixSubtleCmd = &cobra.Command{
	Use:   "remix-subtle",
	Short: "Subtle reshape (v8/v8.1 only)",
	Long:  `Subtle reshape of a v8/v8.1 parent image. Small change; keeps subject/tone.`,
	Example: `  aigc-cli midjourney remix-subtle --task-id task_xxx --index 1
  aigc-cli midjourney remix-subtle --task-id task_xxx --index 1 --prompt "soft morning light"
  aigc-cli midjourney remix-subtle --task-id task_xxx --index 1 --speed fast`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMJRmix(cmd, "remix-subtle")
	},
}

func runMJRmix(cmd *cobra.Command, action string) error {
	req, err := buildMJRemixReq(cmd)
	if err != nil {
		return err
	}
	c := NewClient()
	return runMJSubmitAndPoll(c, action, req)
}

// buildMJRemixReq builds MJRemixRequest from --json (with flag overlay) or flags.
func buildMJRemixReq(cmd *cobra.Command) (*types.MJRemixRequest, error) {
	if mjJSONInput != "" {
		data, err := service.ReadJSONInput(mjJSONInput)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.MJRemixRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		merged, err := mjRemixOverlay(cmd).apply(data, req)
		if err != nil {
			return nil, err
		}
		req.RawJSON = merged
		if req.TaskID == "" {
			return nil, fmt.Errorf("task_id is required")
		}
		return req, nil
	}

	if mjTaskID == "" {
		return nil, fmt.Errorf("--task-id is required for remix")
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
	return req, nil
}
