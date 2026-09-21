package midjourney

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/service"
)

// mjOverlay collects the flags a user explicitly set into a map keyed by the
// JSON field each flag overrides. Cobra's Changed() distinguishes "the flag was
// passed" from "the flag still holds its default value", so explicit zero values
// (e.g. --index 0) are honored even though they equal the zero value.
type mjOverlay struct {
	cmd *cobra.Command
	set map[string]any
}

func newMJOverlay(cmd *cobra.Command) *mjOverlay {
	return &mjOverlay{cmd: cmd, set: map[string]any{}}
}

// str records a string flag when it was explicitly set.
func (o *mjOverlay) str(flag, key, val string) *mjOverlay {
	if o.cmd.Flags().Changed(flag) {
		o.set[key] = val
	}
	return o
}

// strs records a string-slice flag (e.g. repeated --image-url) when set.
func (o *mjOverlay) strs(flag, key string, val []string) *mjOverlay {
	if o.cmd.Flags().Changed(flag) {
		o.set[key] = val
	}
	return o
}

// intFlag records an int flag when explicitly set, including 0.
func (o *mjOverlay) intFlag(flag, key string, val int) *mjOverlay {
	if o.cmd.Flags().Changed(flag) {
		o.set[key] = val
	}
	return o
}

// floatFlag records a float64 flag when explicitly set, including 0.
func (o *mjOverlay) floatFlag(flag, key string, val float64) *mjOverlay {
	if o.cmd.Flags().Changed(flag) {
		o.set[key] = val
	}
	return o
}

// boolFlag records a bool flag when explicitly set, including false.
func (o *mjOverlay) boolFlag(flag, key string, val bool) *mjOverlay {
	if o.cmd.Flags().Changed(flag) {
		o.set[key] = val
	}
	return o
}

// apply merges the collected flags over raw and refreshes req's typed fields
// from the merged body. When no flag was set it is a no-op and raw is returned
// byte-identical, preserving verbatim --json semantics.
func (o *mjOverlay) apply(raw json.RawMessage, req any) (json.RawMessage, error) {
	merged, err := service.MergeJSONOverlay(raw, o.set)
	if err != nil {
		return nil, fmt.Errorf("failed to merge flags into JSON input: %w", err)
	}
	if err := json.Unmarshal(merged, req); err != nil {
		return nil, fmt.Errorf("failed to parse merged JSON: %w", err)
	}
	return merged, nil
}

// ============================================================================
// Per-request overlays (flag name -> JSON key)
// ============================================================================

// mjImagineOverlay maps imagine/edits flags onto types.MJImagineRequest.
func mjImagineOverlay(cmd *cobra.Command) (*mjOverlay, error) {
	o := newMJOverlay(cmd)
	if cmd.Flags().Changed("prompt") {
		prompt, err := resolveMJPrompt(cmd)
		if err != nil {
			return nil, err
		}
		o.set["prompt"] = prompt
	}
	return o.
		strs("image-url", "image_urls", mjImageURLs).
		str("speed", "speed", mjSpeed).
		str("size", "size", mjSize).
		str("quality", "quality", mjQuality).
		str("style", "style", mjStyle).
		str("version", "version", mjVersion).
		str("negative-prompt", "negative_prompt", mjNegPrompt).
		str("cref", "cref", mjCref).
		str("sref", "sref", mjSref).
		str("dref", "dref", mjDref).
		str("extra", "extra", mjExtra).
		intFlag("seed", "seed", mjSeed).
		intFlag("stylize", "stylize", mjStylize).
		intFlag("chaos", "chaos", mjChaos).
		intFlag("weird", "weird", mjWeird).
		intFlag("cw", "cw", mjCw).
		intFlag("sw", "sw", mjSw).
		intFlag("repeat", "repeat", mjRepeat).
		intFlag("stop", "stop", mjStop).
		floatFlag("iw", "iw", mjIw).
		floatFlag("dw", "dw", mjDw).
		boolFlag("tile", "tile", mjTile).
		boolFlag("niji", "niji", mjNiji).
		boolFlag("raw", "raw", mjRaw).
		boolFlag("draft", "draft", mjDraft).
		boolFlag("hd", "hd", mjHd), nil
}

// mjBlendOverlay maps blend flags onto types.MJBlendRequest.
func mjBlendOverlay(cmd *cobra.Command) *mjOverlay {
	return newMJOverlay(cmd).
		strs("image-url", "image_urls", mjImageURLs).
		str("dimensions", "dimensions", mjDimensions).
		str("size", "size", mjSize).
		str("speed", "speed", mjSpeed)
}

// mjDescribeOverlay maps describe flags onto types.MJDescribeRequest.
func mjDescribeOverlay(cmd *cobra.Command) *mjOverlay {
	return newMJOverlay(cmd).
		strs("image-url", "image_urls", mjImageURLs).
		str("speed", "speed", mjSpeed)
}

// mjTaskActionOverlay maps the shared task-action flags onto types.MJTaskActionRequest.
func mjTaskActionOverlay(cmd *cobra.Command) *mjOverlay {
	return newMJOverlay(cmd).
		str("task-id", "task_id", mjTaskID).
		intFlag("index", "index", mjIndex).
		str("custom-id", "custom_id", mjCustomID).
		str("speed", "speed", mjSpeed)
}

// mjRerollOverlay maps reroll flags onto types.MJRerollRequest.
func mjRerollOverlay(cmd *cobra.Command) *mjOverlay {
	return newMJOverlay(cmd).
		str("task-id", "task_id", mjTaskID).
		str("custom-id", "custom_id", mjCustomID).
		str("speed", "speed", mjSpeed)
}

// mjZoomOverlay maps zoom flags onto types.MJZoomRequest.
func mjZoomOverlay(cmd *cobra.Command) *mjOverlay {
	return newMJOverlay(cmd).
		str("task-id", "task_id", mjTaskID).
		str("custom-id", "custom_id", mjCustomID).
		intFlag("index", "index", mjIndex).
		floatFlag("zoom-ratio", "zoom_ratio", mjZoomRatio).
		str("speed", "speed", mjSpeed)
}

// mjPanOverlay maps pan flags onto types.MJPanRequest.
func mjPanOverlay(cmd *cobra.Command) *mjOverlay {
	return newMJOverlay(cmd).
		str("task-id", "task_id", mjTaskID).
		str("custom-id", "custom_id", mjCustomID).
		intFlag("index", "index", mjIndex).
		str("direction", "direction", mjDirection).
		str("speed", "speed", mjSpeed)
}

// mjModalOverlay maps modal flags onto types.MJModalRequest.
func mjModalOverlay(cmd *cobra.Command) *mjOverlay {
	return newMJOverlay(cmd).
		str("task-id", "task_id", mjTaskID).
		str("prompt", "prompt", mjPrompt).
		str("mask-url", "mask_url", mjMaskURL).
		str("speed", "speed", mjSpeed)
}

// mjVideoOverlay maps video flags onto types.MJVideoRequest.
func mjVideoOverlay(cmd *cobra.Command) *mjOverlay {
	return newMJOverlay(cmd).
		str("prompt", "prompt", mjPrompt).
		strs("image-url", "image_urls", mjImageURLs).
		str("task-id", "task_id", mjTaskID).
		intFlag("index", "index", mjIndex).
		str("video-type", "video_type", mjVideoType).
		str("animate-mode", "animate_mode", mjAnimateMode).
		str("motion", "motion", mjMotion).
		intFlag("batch-size", "batch_size", mjBatchSize).
		str("end-url", "end_url", mjEndURL)
}

// mjRemixOverlay maps remix flags onto types.MJRemixRequest.
func mjRemixOverlay(cmd *cobra.Command) *mjOverlay {
	return newMJOverlay(cmd).
		str("task-id", "task_id", mjTaskID).
		str("prompt", "prompt", mjPrompt).
		intFlag("index", "index", mjIndex).
		str("speed", "speed", mjSpeed)
}
