package midjourney

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// buildMJImagineReq builds MJImagineRequest from flags or --json.
func buildMJImagineReq(cmd *cobra.Command) (*types.MJImagineRequest, error) {
	if mjJSONInput != "" {
		data, err := service.ReadInput(mjJSONInput)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.MJImagineRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		req.RawJSON = data
		if req.Prompt == "" {
			return nil, fmt.Errorf("prompt is required in JSON input")
		}
		return req, nil
	}

	prompt, err := resolveMJPrompt(cmd)
	if err != nil {
		return nil, err
	}

	req := &types.MJImagineRequest{
		Prompt:         prompt,
		ImageURLs:      mjImageURLs,
		Speed:          mjSpeed,
		Size:           mjSize,
		Quality:        mjQuality,
		Style:          mjStyle,
		Version:        mjVersion,
		NegativePrompt: mjNegPrompt,
		Cref:           mjCref,
		Sref:           mjSref,
		Dref:           mjDref,
		Extra:          mjExtra,
	}

	options.SetIntFlag(cmd, "seed", &req.Seed, mjSeed)
	options.SetIntFlag(cmd, "stylize", &req.Stylize, mjStylize)
	options.SetIntFlag(cmd, "chaos", &req.Chaos, mjChaos)
	options.SetIntFlag(cmd, "weird", &req.Weird, mjWeird)
	options.SetIntFlag(cmd, "cw", &req.Cw, mjCw)
	options.SetIntFlag(cmd, "sw", &req.Sw, mjSw)
	options.SetIntFlag(cmd, "repeat", &req.Repeat, mjRepeat)
	options.SetIntFlag(cmd, "stop", &req.Stop, mjStop)
	options.SetFloatFlag(cmd, "iw", &req.Iw, mjIw)
	options.SetFloatFlag(cmd, "dw", &req.Dw, mjDw)
	options.SetBoolFlag(cmd, "tile", &req.Tile, mjTile)
	options.SetBoolFlag(cmd, "niji", &req.Niji, mjNiji)
	options.SetBoolFlag(cmd, "raw", &req.Raw, mjRaw)
	options.SetBoolFlag(cmd, "draft", &req.Draft, mjDraft)
	options.SetBoolFlag(cmd, "hd", &req.Hd, mjHd)

	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required (use --prompt or --json)")
	}
	return req, nil
}

// buildMJTaskActionReq builds MJTaskActionRequest from flags.
func buildMJTaskActionReq() (*types.MJTaskActionRequest, error) {
	if mjTaskID == "" {
		return nil, fmt.Errorf("--task-id is required")
	}
	req := &types.MJTaskActionRequest{
		TaskID:   mjTaskID,
		CustomID: mjCustomID,
		Speed:    mjSpeed,
	}
	if mjIndex > 0 {
		v := mjIndex
		req.Index = &v
	}
	return req, nil
}

// buildMJTaskActionReqFromJSON builds MJTaskActionRequest from --json or flags.
func buildMJTaskActionReqFromJSON() (*types.MJTaskActionRequest, error) {
	if mjJSONInput != "" {
		data, err := service.ReadInput(mjJSONInput)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.MJTaskActionRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		req.RawJSON = data
		if req.TaskID == "" {
			return nil, fmt.Errorf("task_id is required in JSON input")
		}
		return req, nil
	}
	return buildMJTaskActionReq()
}
