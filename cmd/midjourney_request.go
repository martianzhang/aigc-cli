package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/types"
)

// buildMJImagineReq builds MJImagineRequest from flags or --json.
func buildMJImagineReq(cmd *cobra.Command) (*types.MJImagineRequest, error) {
	if mjJSONInput != "" {
		data, err := readInput(mjJSONInput)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.MJImagineRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
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

	setMJIntFlag(cmd, "seed", &req.Seed, mjSeed)
	setMJIntFlag(cmd, "stylize", &req.Stylize, mjStylize)
	setMJIntFlag(cmd, "chaos", &req.Chaos, mjChaos)
	setMJIntFlag(cmd, "weird", &req.Weird, mjWeird)
	setMJIntFlag(cmd, "cw", &req.Cw, mjCw)
	setMJIntFlag(cmd, "sw", &req.Sw, mjSw)
	setMJIntFlag(cmd, "repeat", &req.Repeat, mjRepeat)
	setMJIntFlag(cmd, "stop", &req.Stop, mjStop)
	setMJFloatFlag(cmd, "iw", &req.Iw, mjIw)
	setMJFloatFlag(cmd, "dw", &req.Dw, mjDw)
	setMJBoolFlag(cmd, "tile", &req.Tile, mjTile)
	setMJBoolFlag(cmd, "niji", &req.Niji, mjNiji)
	setMJBoolFlag(cmd, "raw", &req.Raw, mjRaw)
	setMJBoolFlag(cmd, "draft", &req.Draft, mjDraft)
	setMJBoolFlag(cmd, "hd", &req.Hd, mjHd)

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
		data, err := readInput(mjJSONInput)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.MJTaskActionRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		if req.TaskID == "" {
			return nil, fmt.Errorf("task_id is required in JSON input")
		}
		return req, nil
	}
	return buildMJTaskActionReq()
}
