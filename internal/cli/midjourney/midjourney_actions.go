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
var mjUpscaleCmd = registerMJTaskActionSubcommand(mjTaskActionSpec{
	name:  "upscale",
	short: "Upscale a tile (U1-U4)",
	long: `Upscale one tile from the parent grid (U1-U4).

Composed locally from existing images — usually returns instantly.

Examples:
  aigc-cli midjourney upscale --task-id task_xxx --index 1
  aigc-cli midjourney upscale --task-id task_xxx --custom-id "MJ::JOB::upsample::1::abc"`,
	action: "upscale",
})

var mjVariationCmd = registerMJTaskActionSubcommand(mjTaskActionSpec{
	name:  "variation",
	short: "Subtle variation (V1-V4)",
	long: `Create a subtle variation (varySubtle) from one tile of an Imagine grid.

Examples:
  aigc-cli midjourney variation --task-id task_xxx --index 3`,
	action: "variation",
})

var mjHighVariationCmd = registerMJTaskActionSubcommand(mjTaskActionSpec{
	name:  "high-variation",
	short: "High (strong) variation",
	long:  `Create a strong variation (varyStrong) from one tile of an Imagine grid.`,
	example: `  aigc-cli midjourney high-variation --task-id task_xxx --index 2
  aigc-cli midjourney high-variation --task-id task_xxx --index 2 --speed fast
  aigc-cli midjourney high-variation --json request.json`,
	action: "high-variation",
})

var mjLowVariationCmd = registerMJTaskActionSubcommand(mjTaskActionSpec{
	name:  "low-variation",
	short: "Low (subtle) variation",
	long:  `Create a low (subtle) variation from one tile.`,
	example: `  aigc-cli midjourney low-variation --task-id task_xxx --index 4
  aigc-cli midjourney low-variation --task-id task_xxx --index 4 --speed fast`,
	action: "low-variation",
})

// ============================================================================
// Subcommand: reroll
// ============================================================================
var mjRerollCmd = &cobra.Command{
	Use:   "reroll",
	Short: "Regenerate the grid (🔄)",
	Long:  `Regenerate 4 images from the source task's prompt. No index needed - whole grid is rerolled.`,
	Example: `  aigc-cli midjourney reroll --task-id task_xxx
  aigc-cli midjourney reroll --task-id task_xxx --speed fast
  aigc-cli midjourney reroll --json request.json`,
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
