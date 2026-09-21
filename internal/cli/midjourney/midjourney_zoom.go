package midjourney

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// ============================================================================
// Subcommand: zoom
// ============================================================================
var mjZoomCmd = &cobra.Command{
	Use:   "zoom",
	Short: "Zoom out / outpaint",
	Long: `Zoom out on a single image after Upscale. zoom_ratio < 2 uses Outpaint (1.5x),
>= 2 or omitted uses CustomZoom (2x).`,
	Example: `  aigc-cli midjourney zoom --task-id task_xxx --zoom-ratio 1.5
  aigc-cli midjourney zoom --task-id task_xxx --zoom-ratio 2
  aigc-cli midjourney zoom --task-id task_xxx --index 1 --speed fast`,
	RunE: func(cmd *cobra.Command, args []string) error {
		req, err := buildMJZoomReq(cmd)
		if err != nil {
			return err
		}
		c := NewClient()
		return runMJSubmitAndPoll(c, "zoom", req)
	},
}

// buildMJZoomReq builds MJZoomRequest from --json (with flag overlay) or flags.
func buildMJZoomReq(cmd *cobra.Command) (*types.MJZoomRequest, error) {
	if mjJSONInput != "" {
		data, err := service.ReadJSONInput(mjJSONInput)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.MJZoomRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		merged, err := mjZoomOverlay(cmd).apply(data, req)
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
		return nil, fmt.Errorf("--task-id is required for zoom")
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
	return req, nil
}

// ============================================================================
// Subcommand: pan
// ============================================================================
var mjPanCmd = &cobra.Command{
	Use:   "pan",
	Short: "Pan in a direction",
	Long: `Pan out in a direction on a single image after Upscale.
Direction: left, right, up, down.`,
	Example: `  aigc-cli midjourney pan --task-id task_xxx --direction right
  aigc-cli midjourney pan --task-id task_xxx --direction up --index 2
  aigc-cli midjourney pan --task-id task_xxx --direction left --speed fast`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := service.ReadJSONInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJPanRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			merged, err := mjPanOverlay(cmd).apply(data, req)
			if err != nil {
				return err
			}
			req.RawJSON = merged
			if req.TaskID == "" {
				return fmt.Errorf("task_id is required")
			}
			if req.Direction == "" && req.CustomID == "" {
				return fmt.Errorf("direction or custom_id is required")
			}
			c := NewClient()
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
		c := NewClient()
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
MODAL state — then call "midjourney modal" with a mask + prompt.`,
	Example: `  aigc-cli midjourney inpaint --json '{"task_id":"task_xxx"}'
  aigc-cli midjourney modal --task-id task_yyy --prompt "replace with a red sofa" --mask-url mask.png`,
	RunE: func(cmd *cobra.Command, args []string) error {
		req, err := buildMJTaskActionReqFromJSON(cmd)
		if err != nil {
			return err
		}
		c := NewClient()
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
With mask_url → inpaint (local repaint). Without → outpaint (expand).`,
	Example: `  aigc-cli midjourney modal --task-id task_xxx --prompt "replace with a red sofa" --mask-url mask.png
  aigc-cli midjourney modal --task-id task_xxx --prompt "expand the scene" --speed fast
  aigc-cli midjourney modal --json request.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := service.ReadJSONInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJModalRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			merged, err := mjModalOverlay(cmd).apply(data, req)
			if err != nil {
				return err
			}
			req.RawJSON = merged
			if req.TaskID == "" {
				return fmt.Errorf("task_id is required")
			}
			if req.MaskURL != "" {
				c := NewClient()
				resolved, err := c.ResolveLocalImages([]string{req.MaskURL})
				if err != nil {
					return fmt.Errorf("failed to resolve mask-url: %w", err)
				}
				req.MaskURL = resolved[0]
			}
			c := NewClient()
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
			c := NewClient()
			resolved, err := c.ResolveLocalImages([]string{req.MaskURL})
			if err != nil {
				return fmt.Errorf("failed to resolve mask-url: %w", err)
			}
			req.MaskURL = resolved[0]
		}
		c := NewClient()
		return runMJSubmitAndPoll(c, "modal", req)
	},
}
