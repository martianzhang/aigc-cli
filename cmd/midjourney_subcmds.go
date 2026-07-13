package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/martianzhang/apimart-cli/internal/types"
)

var mjImagineCmd = &cobra.Command{
	Use:   "imagine",
	Short: "Text-to-image / image-guided generation",
	Long: `Generate images from a text prompt, optionally with reference images.

The default MJ entry point. Supports all MJ structured fields and native flags.

Examples:
  aigc-cli midjourney imagine --prompt "a cute cat --ar 16:9"
  aigc-cli midjourney imagine --prompt "a cat" --size "16:9" --version "6.1" --style raw
  aigc-cli midjourney imagine --prompt "luxury product" --image-url ref.png --iw 1.2
  aigc-cli midjourney imagine --json request.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		req, err := buildMJImagineReq(cmd)
		if err != nil {
			return err
		}

		// Merge config defaults
		if cfg, err := config.LoadDefaults(shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
			cfg.Defaults.Midjourney.MergeIntoImagine(req)
		}

		// Resolve local images
		if len(req.ImageURLs) > 0 {
			c := newMJClient()
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
		}

		c := newMJClient()
		return runMJSubmitAndPoll(c, "imagine", req)
	},
}

// ============================================================================
// Subcommand: blend
// ============================================================================
var mjBlendCmd = &cobra.Command{
	Use:   "blend",
	Short: "Multi-image blend (2-4 images)",
	Long: `Blend 2-4 images into a new image. No prompt is used — pure image blend.

Examples:
  aigc-cli midjourney blend --image-url a.png --image-url b.png
  aigc-cli midjourney blend --image-url a.png --image-url b.png --image-url c.png --dimensions PORTRAIT`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJBlendRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if len(req.ImageURLs) < 2 {
				return fmt.Errorf("at least 2 image_urls required")
			}
			c := newMJClient()
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
			return runMJSubmitAndPoll(c, "blend", req)
		}

		if len(mjImageURLs) < 2 {
			return fmt.Errorf("at least 2 --image-url required for blend")
		}

		req := &types.MJBlendRequest{
			ImageURLs:  mjImageURLs,
			Dimensions: mjDimensions,
			Size:       mjSize,
			Speed:      mjSpeed,
		}

		c := newMJClient()
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return fmt.Errorf("failed to resolve image-urls: %w", err)
		}
		req.ImageURLs = resolved
		return runMJSubmitAndPoll(c, "blend", req)
	},
}

// ============================================================================
// Subcommand: describe
// ============================================================================
var mjDescribeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Image to text (reverse prompt)",
	Long: `Reverse-engineer a prompt from an image. Returns 4 prompt suggestions.

Example:
  aigc-cli midjourney describe --image-url input.png`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJDescribeRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if len(req.ImageURLs) == 0 {
				return fmt.Errorf("image_urls is required")
			}
			c := newMJClient()
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
			return runMJSubmitAndPoll(c, "describe", req)
		}

		if len(mjImageURLs) == 0 {
			return fmt.Errorf("--image-url is required for describe")
		}

		req := &types.MJDescribeRequest{
			ImageURLs: mjImageURLs,
			Speed:     mjSpeed,
		}
		c := newMJClient()
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return fmt.Errorf("failed to resolve image-urls: %w", err)
		}
		req.ImageURLs = resolved
		return runMJSubmitAndPoll(c, "describe", req)
	},
}

// ============================================================================
// Subcommand: edits
// ============================================================================
var mjEditsCmd = &cobra.Command{
	Use:   "edits",
	Short: "Image edit (rewrite whole image)",
	Long: `Rewrite an entire image from a prompt + reference image.
Good for background replacement, style transfer, and content changes.

Example:
  aigc-cli midjourney edits --prompt "replace background with a modern kitchen" --image-url product.png`,
	RunE: func(cmd *cobra.Command, args []string) error {
		req, err := buildMJImagineReq(cmd) // Same structure as imagine
		if err != nil {
			return err
		}
		if len(req.ImageURLs) == 0 {
			return fmt.Errorf("--image-url is required for edits")
		}

		if cfg, err := config.LoadDefaults(shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
			cfg.Defaults.Midjourney.MergeIntoImagine(req)
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
		return runMJSubmitAndPoll(c, "edits", req)
	},
}

// ============================================================================
// Task-action subcommands (upscale, variation, high-variation, low-variation, inpaint)
// ============================================================================
var mjUpscaleCmd = registerMJTaskActionSubcommand(
	"upscale",
	"Upscale a tile (U1-U4)",
	`Upscale one tile from the parent grid (U1-U4).

Composed locally from existing images — usually returns instantly.

Examples:
  aigc-cli midjourney upscale --task-id task_xxx --index 1
  aigc-cli midjourney upscale --task-id task_xxx --custom-id "MJ::JOB::upsample::1::abc"`,
	"upscale",
)

var mjVariationCmd = registerMJTaskActionSubcommand(
	"variation",
	"Subtle variation (V1-V4)",
	`Create a subtle variation (varySubtle) from one tile of an Imagine grid.

Examples:
  aigc-cli midjourney variation --task-id task_xxx --index 3`,
	"variation",
)

var mjHighVariationCmd = registerMJTaskActionSubcommand(
	"high-variation",
	"High (strong) variation",
	`Create a strong variation (varyStrong) from one tile of an Imagine grid.

Example:
  aigc-cli midjourney high-variation --task-id task_xxx --index 2`,
	"high-variation",
)

var mjLowVariationCmd = registerMJTaskActionSubcommand(
	"low-variation",
	"Low (subtle) variation",
	`Create a low (subtle) variation from one tile.

Example:
  aigc-cli midjourney low-variation --task-id task_xxx --index 4`,
	"low-variation",
)

// ============================================================================
// Subcommand: reroll
// ============================================================================
var mjRerollCmd = &cobra.Command{
	Use:   "reroll",
	Short: "Regenerate the grid (🔄)",
	Long: `Regenerate 4 images from the source task's prompt. No index needed - whole grid is rerolled.

Example:
  aigc-cli midjourney reroll --task-id task_xxx`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJRerollRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if req.TaskID == "" {
				return fmt.Errorf("task_id is required")
			}
			c := newMJClient()
			return runMJSubmitAndPoll(c, "reroll", req)
		}

		if mjTaskID == "" {
			return fmt.Errorf("--task-id is required for reroll")
		}
		req := &types.MJRerollRequest{
			TaskID:   mjTaskID,
			CustomID: mjCustomID,
			Speed:    mjSpeed,
		}
		c := newMJClient()
		return runMJSubmitAndPoll(c, "reroll", req)
	},
}

// ============================================================================
// Subcommand: zoom
// ============================================================================
var mjZoomCmd = &cobra.Command{
	Use:   "zoom",
	Short: "Zoom out / outpaint",
	Long: `Zoom out on a single image after Upscale. zoom_ratio < 2 uses Outpaint (1.5x),
>= 2 or omitted uses CustomZoom (2x).

Example:
  aigc-cli midjourney zoom --task-id task_xxx --zoom-ratio 1.5`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJZoomRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if req.TaskID == "" {
				return fmt.Errorf("task_id is required")
			}
			c := newMJClient()
			return runMJSubmitAndPoll(c, "zoom", req)
		}

		if mjTaskID == "" {
			return fmt.Errorf("--task-id is required for zoom")
		}
		req := &types.MJZoomRequest{
			TaskID:   mjTaskID,
			CustomID: mjCustomID,
			Speed:    mjSpeed,
		}
		if cmd.Flags().Changed("index") {
			v := mjIndex
			req.Index = &v
		}
		if cmd.Flags().Changed("zoom-ratio") {
			v := mjZoomRatio
			req.ZoomRatio = &v
		}
		c := newMJClient()
		return runMJSubmitAndPoll(c, "zoom", req)
	},
}

// ============================================================================
// Subcommand: pan
// ============================================================================
var mjPanCmd = &cobra.Command{
	Use:   "pan",
	Short: "Pan in a direction",
	Long: `Pan out in a direction on a single image after Upscale.
Direction: left, right, up, down.

Example:
  aigc-cli midjourney pan --task-id task_xxx --direction right`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJPanRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if req.TaskID == "" {
				return fmt.Errorf("task_id is required")
			}
			if req.Direction == "" && req.CustomID == "" {
				return fmt.Errorf("direction or custom_id is required")
			}
			c := newMJClient()
			return runMJSubmitAndPoll(c, "pan", req)
		}

		if mjTaskID == "" {
			return fmt.Errorf("--task-id is required for pan")
		}
		if mjDirection == "" && mjCustomID == "" {
			return fmt.Errorf("--direction (left/right/up/down) or --custom-id is required")
		}
		req := &types.MJPanRequest{
			TaskID:    mjTaskID,
			CustomID:  mjCustomID,
			Direction: mjDirection,
			Speed:     mjSpeed,
		}
		if cmd.Flags().Changed("index") {
			v := mjIndex
			req.Index = &v
		}
		c := newMJClient()
		return runMJSubmitAndPoll(c, "pan", req)
	},
}

// ============================================================================
// Subcommand: inpaint
// ============================================================================
var mjInpaintCmd = &cobra.Command{
	Use:   "inpaint",
	Short: "Region inpaint entry (→ modal)",
	Long: `Entry point for region inpaint (Vary Region). After submission, the task enters
MODAL state — then call "midjourney modal" with a mask + prompt.

Example:
  aigc-cli midjourney inpaint --task-id task_xxx`,
	RunE: func(_ *cobra.Command, args []string) error {
		req, err := buildMJTaskActionReqFromJSON()
		if err != nil {
			return err
		}
		c := newMJClient()
		return runMJSubmitAndPoll(c, "inpaint", req)
	},
}

// ============================================================================
// Subcommand: modal
// ============================================================================
var mjModalCmd = &cobra.Command{
	Use:   "modal",
	Short: "Submit mask + prompt for inpaint",
	Long: `Complete a MODAL-state inpaint task by supplying a mask + prompt.
With mask_url → inpaint (local repaint). Without → outpaint (expand).

Example:
  aigc-cli midjourney modal --task-id task_xxx --prompt "replace with red sofa" --mask-url mask.png`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJModalRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if req.TaskID == "" {
				return fmt.Errorf("task_id is required")
			}
			if req.MaskURL != "" {
				c := newMJClient()
				resolved, err := c.ResolveLocalImages([]string{req.MaskURL})
				if err != nil {
					return fmt.Errorf("failed to resolve mask-url: %w", err)
				}
				req.MaskURL = resolved[0]
			}
			c := newMJClient()
			return runMJSubmitAndPoll(c, "modal", req)
		}

		if mjTaskID == "" {
			return fmt.Errorf("--task-id is required for modal")
		}
		req := &types.MJModalRequest{
			TaskID:  mjTaskID,
			Prompt:  mjPrompt,
			MaskURL: mjMaskURL,
			Speed:   mjSpeed,
		}
		// Resolve local mask
		if req.MaskURL != "" {
			c := newMJClient()
			resolved, err := c.ResolveLocalImages([]string{req.MaskURL})
			if err != nil {
				return fmt.Errorf("failed to resolve mask-url: %w", err)
			}
			req.MaskURL = resolved[0]
		}
		c := newMJClient()
		return runMJSubmitAndPoll(c, "modal", req)
	},
}

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

// ============================================================================
