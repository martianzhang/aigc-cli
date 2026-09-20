package midjourney

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
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
			data, err := service.ReadInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJRerollRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			req.RawJSON = data
			if req.TaskID == "" {
				return fmt.Errorf("task_id is required")
			}
			c := NewClient()
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
		c := NewClient()
		return runMJSubmitAndPoll(c, "reroll", req)
	},
}
