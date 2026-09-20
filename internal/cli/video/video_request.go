package video

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// resolveVideoPrompt resolves the video prompt (shared by normal and remix modes).
func resolveVideoPrompt() (string, error) {
	prompt := vidPrompt
	if prompt == "" {
		prompt = "-"
	}
	if prompt == "-" || service.IsFile(prompt) {
		data, err := service.ReadInput(prompt)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt: %w", err)
		}
		return string(data), nil
	}
	return prompt, nil
}

func buildVideoRequest(cmd *cobra.Command) (*types.VideoGenerateRequest, error) {
	if options.Shared.JSONInput != "" {
		data, err := service.ReadInput(options.Shared.JSONInput)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.VideoGenerateRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		req.RawJSON = data
		return req, nil
	}

	prompt, err := resolveVideoPrompt()
	if err != nil {
		return nil, err
	}

	req := &types.VideoGenerateRequest{
		Model:      options.Shared.Model,
		Prompt:     prompt,
		Size:       vidSize,
		Resolution: vidResolution,
		ImageURLs:  vidImageURLs,
		VideoURLs:  vidVideoURLs,
		AudioURLs:  vidAudioURLs,
	}

	options.SetIntFlag(cmd, "duration", &req.Duration, vidDuration)
	options.SetIntFlag(cmd, "seed", &req.Seed, vidSeed)
	options.SetBoolFlag(cmd, "generate-audio", &req.GenerateAudio, vidGenerateAudio)
	options.SetBoolFlag(cmd, "return-last-frame", &req.ReturnLastFrame, vidReturnLastFrame)

	// --first-frame / --last-frame -> image_with_roles
	if cmd.Flags().Changed("first-frame") || cmd.Flags().Changed("last-frame") {
		var roles []types.ImageWithRole
		if cmd.Flags().Changed("first-frame") {
			roles = append(roles, types.ImageWithRole{URL: vidFirstFrame, Role: "first_frame"})
		}
		if cmd.Flags().Changed("last-frame") {
			roles = append(roles, types.ImageWithRole{URL: vidLastFrame, Role: "last_frame"})
		}
		req.ImageWithRoles = roles
	}

	// --tool
	for _, t := range vidTools {
		req.Tools = append(req.Tools, types.VideoTool{Type: t})
	}

	return req, nil
}

// videoWireRequest resolves the method, absolute URL and body a real video
// request would use for p. Shared by --dry-run and --verbose so the preview
// cannot drift from the client call it describes.
func videoWireRequest(req *types.VideoGenerateRequest, p *provider.EffectiveProvider) (method, rawURL string, body interface{}) {
	if p.ProviderType == provider.Pollinations {
		return http.MethodGet, client.NewFromProvider(p).PollinationsVideoURL(req), nil
	}
	base := client.NormalizeBaseURL(p.BaseURL)
	switch p.ProviderType {
	case provider.OpenRouter:
		return http.MethodPost, base + client.OpenRouterVideosPath, openRouterVideoBody(req)
	case provider.Agnes:
		return http.MethodPost, base + client.AgnesVideoSubmitPath, client.AgnesVideoBody(req)
	case provider.Yunwu:
		return http.MethodPost, base + client.YunwuVideoSubPath, client.YunwuVideoBody(req)
	}
	return http.MethodPost, base + client.VideoSubmitPath, req
}

func buildVideoCurl(req *types.VideoGenerateRequest) string {
	p := options.Shared.ResolveProvider(options.ProviderNameVideo)
	method, rawURL, body := videoWireRequest(req, p)
	if method == http.MethodGet {
		cmd := fmt.Sprintf("curl -X GET %s \\\n", rawURL)
		cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", service.MaskKey(p.APIKey))
		cmd += "  --output video.mp4"
		return cmd
	}

	data, _ := json.Marshal(body)
	cmd := fmt.Sprintf("curl -X POST %s \\\n", rawURL)
	cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", service.MaskKey(p.APIKey))
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(data))
	return cmd
}

func buildVideoRemixCurl(req *types.VideoRemixRequest) string {
	p := options.Shared.ResolveProvider(options.ProviderNameVideo)
	body, _ := json.Marshal(req)
	url := fmt.Sprintf("%s/videos/%s/remix", client.NormalizeBaseURL(p.BaseURL), vidTaskID)

	cmd := fmt.Sprintf("curl -X POST %s \\\n", url)
	cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", service.MaskKey(p.APIKey))
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(body))
	return cmd
}
