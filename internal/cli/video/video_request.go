package video

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
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

// buildVideoCurl renders the equivalent curl commands for a video generation
// request: one multipart upload per local image, then the generation call. It
// renders the same plan the client executes, so the preview cannot drift.
func buildVideoCurl(req *types.VideoGenerateRequest) string {
	p := options.Shared.ResolveProvider(options.ProviderNameVideo)
	plan, err := buildVideoPlan(req, p)
	if err != nil {
		return fmt.Sprintf("# failed to build preview: %v", err)
	}
	return plan.RenderCurls(p.APIKey)
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
