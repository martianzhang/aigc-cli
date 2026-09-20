package image

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
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

const imageEditsDataURINote = "# note: local image files are embedded as data: URIs (data:image/...;base64,...) before sending"

// imageWireRequest resolves the absolute URL and body a real image request
// would use for p. It is the single source of truth shared by --dry-run and
// --verbose, so a preview cannot drift from the client call it describes.
func imageWireRequest(req *types.GenerateRequest, p *provider.EffectiveProvider) (rawURL string, body interface{}, note string) {
	if p.Type == types.ProviderOllama || provider.IsLocalEndpoint(p.BaseURL) {
		return service.OllamaGenerateURL(p.BaseURL), service.OllamaGenerateBody(req), ""
	}
	base := client.NormalizeBaseURL(p.BaseURL)
	switch p.ProviderType {
	case provider.OpenRouter:
		return base + client.OpenRouterImagesPath, client.OpenRouterImageBody(req), ""
	case provider.Gemini:
		return base + client.GeminiInteractionsPath, client.GeminiImageBody(req), ""
	case provider.Zeekai:
		if usesImageEditsJSON(p, req) {
			return base + client.ImageEditsPath, client.ImageEditsBody(req), imageEditsDataURINote
		}
	}
	return base + client.ImageSubmitPath, req, ""
}

// buildImageCurl generates an equivalent curl command for an image generation
// request, rendering the same endpoint and body the client will actually use.
func buildImageCurl(req *types.GenerateRequest, p *provider.EffectiveProvider) string {
	rawURL, body, note := imageWireRequest(req, p)
	data, _ := json.Marshal(body)
	cmd := fmt.Sprintf("curl -X POST %s \\\n", rawURL)
	cmd += curlHeaderLines(p.APIKey)
	if note != "" {
		cmd += fmt.Sprintf("  -d '%s'\n", string(data))
		cmd += note
		return cmd
	}
	cmd += fmt.Sprintf("  -d '%s'", string(data))
	return cmd
}

// curlHeaderLines renders the Authorization and Content-Type header lines.
func curlHeaderLines(apiKey string) string {
	cmd := ""
	if apiKey != "" {
		cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", service.MaskKey(apiKey))
	}
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	return cmd
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
