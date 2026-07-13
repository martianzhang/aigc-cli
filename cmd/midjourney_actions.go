package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/types"
)

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
