package image

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// buildImageRequest constructs a GenerateRequest from --json or individual flags.
func buildImageRequest(cmd *cobra.Command) (*types.GenerateRequest, error) {
	if options.Shared.JSONInput != "" {
		return parseJSONInput()
	}

	prompt, err := resolvePrompt()
	if err != nil {
		return nil, err
	}

	req := &types.GenerateRequest{
		Model:          options.Shared.Model,
		Prompt:         prompt,
		Size:           genSize,
		Ratio:          genRatio,
		Resolution:     genResolution,
		Quality:        genQuality,
		Background:     genBackground,
		Moderation:     genModeration,
		OutputFormat:   genOutputFormat,
		ImageURLs:      genImageURLs,
		MaskURL:        genMaskURL,
		Style:          genStyle,
		ResponseFormat: genResponseFmt,
	}

	if cmd.Flags().Changed("output-compression") {
		v := genCompression
		req.OutputCompression = &v
	}
	if cmd.Flags().Changed("n") {
		v := genN
		req.N = &v
	}

	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required (use --prompt or --json)")
	}

	return req, nil
}

// imageEditsDataURINote is the plain-text note reqbuild renders as
// "# note: <text>" for the Zeekai edits plan. It must not carry the prefix.
const imageEditsDataURINote = "local image files are embedded as data: URIs (data:image/...;base64,...) before sending"

// buildImageCurl renders the equivalent curl commands for an image generation
// request from the same plan execution uses. It returns an empty string when
// the plan cannot be built.
func buildImageCurl(req *types.GenerateRequest, p *provider.EffectiveProvider) string {
	pl, err := buildImagePlan(req, p)
	if err != nil {
		return ""
	}
	return pl.RenderCurls(p.APIKey)
}

// parseJSONInput reads JSON from file path, string literal, or stdin.
func parseJSONInput() (*types.GenerateRequest, error) {
	data, err := service.ReadInput(options.Shared.JSONInput)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON input: %w", err)
	}

	req := &types.GenerateRequest{}
	if err := json.Unmarshal(data, req); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	req.RawJSON = data

	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required in JSON input")
	}

	return req, nil
}

// resolvePrompt resolves the prompt text from --prompt flag.
// Defaults to stdin when --prompt is not specified.
func resolvePrompt() (string, error) {
	input := genPrompt
	if input == "" {
		input = "-"
	}
	if input == "-" || service.IsFile(input) {
		data, err := service.ReadInput(input)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt: %w", err)
		}
		return string(data), nil
	}
	return input, nil
}
