package video

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/cliutil"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
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
	inject, err := service.ReadInjectJSON(options.Shared.InjectJSON)
	if err != nil {
		return nil, err
	}
	if options.Shared.JSONInput != "" {
		req, err := buildVideoRequestFromJSON(cmd)
		if err != nil {
			return nil, err
		}
		req.InjectJSON = inject
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

	cliutil.SetIntFlag(cmd, "duration", &req.Duration, vidDuration)
	cliutil.SetIntFlag(cmd, "seed", &req.Seed, vidSeed)
	cliutil.SetBoolFlag(cmd, "generate-audio", &req.GenerateAudio, vidGenerateAudio)
	cliutil.SetBoolFlag(cmd, "return-last-frame", &req.ReturnLastFrame, vidReturnLastFrame)

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

	req.InjectJSON = inject
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
	curl, err := plan.RenderCurlsErr(p.APIKey)
	if err != nil {
		return fmt.Sprintf("# failed to render preview: %v", err)
	}
	return curl
}

func buildVideoRemixCurl(req *types.VideoRemixRequest) string {
	p := options.Shared.ResolveProvider(options.ProviderNameVideo)
	url := fmt.Sprintf("%s/videos/%s/remix", client.NormalizeBaseURL(p.BaseURL), vidTaskID)
	plan := &reqbuild.Plan{Method: http.MethodPost, URL: url, Body: req}
	curl, err := plan.RenderCurlsErr(p.APIKey)
	if err != nil {
		return fmt.Sprintf("# failed to render preview: %v", err)
	}
	return curl
}
