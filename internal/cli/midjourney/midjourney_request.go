package midjourney

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cliutil"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// buildMJImagineReq builds MJImagineRequest from flags or --json.
func buildMJImagineReq(cmd *cobra.Command) (*types.MJImagineRequest, error) {
	if mjJSONInput != "" {
		data, err := service.ReadJSONInput(mjJSONInput)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.MJImagineRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		overlay, err := mjImagineOverlay(cmd)
		if err != nil {
			return nil, err
		}
		merged, err := overlay.apply(data, req)
		if err != nil {
			return nil, err
		}
		req.RawJSON = merged
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

	cliutil.SetIntFlag(cmd, "seed", &req.Seed, mjSeed)
	cliutil.SetIntFlag(cmd, "stylize", &req.Stylize, mjStylize)
	cliutil.SetIntFlag(cmd, "chaos", &req.Chaos, mjChaos)
	cliutil.SetIntFlag(cmd, "weird", &req.Weird, mjWeird)
	cliutil.SetIntFlag(cmd, "cw", &req.Cw, mjCw)
	cliutil.SetIntFlag(cmd, "sw", &req.Sw, mjSw)
	cliutil.SetIntFlag(cmd, "repeat", &req.Repeat, mjRepeat)
	cliutil.SetIntFlag(cmd, "stop", &req.Stop, mjStop)
	cliutil.SetFloatFlag(cmd, "iw", &req.Iw, mjIw)
	cliutil.SetFloatFlag(cmd, "dw", &req.Dw, mjDw)
	cliutil.SetBoolFlag(cmd, "tile", &req.Tile, mjTile)
	cliutil.SetBoolFlag(cmd, "niji", &req.Niji, mjNiji)
	cliutil.SetBoolFlag(cmd, "raw", &req.Raw, mjRaw)
	cliutil.SetBoolFlag(cmd, "draft", &req.Draft, mjDraft)
	cliutil.SetBoolFlag(cmd, "hd", &req.Hd, mjHd)

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
func buildMJTaskActionReqFromJSON(cmd *cobra.Command) (*types.MJTaskActionRequest, error) {
	if mjJSONInput != "" {
		data, err := service.ReadJSONInput(mjJSONInput)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.MJTaskActionRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		merged, err := mjTaskActionOverlay(cmd).apply(data, req)
		if err != nil {
			return nil, err
		}
		req.RawJSON = merged
		if req.TaskID == "" {
			return nil, fmt.Errorf("task_id is required in JSON input")
		}
		return req, nil
	}
	return buildMJTaskActionReq()
}
