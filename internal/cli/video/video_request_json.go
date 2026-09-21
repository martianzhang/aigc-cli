package video

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// buildVideoRequestFromJSON builds a request from --json and overlays every
// explicitly set flag. An explicitly set CLI flag wins over the JSON key of the
// same name (documented precedence: CLI flags > JSON input). Keys no flag
// addresses stay byte-identical, including vendor-specific keys the typed
// request does not model.
func buildVideoRequestFromJSON(cmd *cobra.Command) (*types.VideoGenerateRequest, error) {
	data, err := service.ReadJSONInput(options.Shared.JSONInput)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON input: %w", err)
	}

	req := &types.VideoGenerateRequest{}
	if err := json.Unmarshal(data, req); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	set, err := videoFlagOverlay(cmd, data)
	if err != nil {
		return nil, err
	}

	merged, err := service.MergeJSONOverlay(data, set)
	if err != nil {
		return nil, fmt.Errorf("failed to merge flags into JSON input: %w", err)
	}

	req.RawJSON = merged
	if err := json.Unmarshal(merged, req); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return req, nil
}

// videoFlagOverlay maps explicitly set video flags to the JSON keys they
// override. Unset flags contribute nothing, so a JSON-only request is never
// rewritten. Behavioral switches (--remix, --gif, --dry-run, …) are absent by
// design: they configure the CLI, not the API body.
func videoFlagOverlay(cmd *cobra.Command, raw []byte) (map[string]any, error) {
	set := map[string]any{}

	if options.HasFlagChanged(cmd, "model") {
		set["model"] = options.Shared.Model
	}
	if cmd.Flags().Changed("prompt") {
		prompt, err := resolveVideoPrompt()
		if err != nil {
			return nil, err
		}
		set["prompt"] = prompt
	}
	if cmd.Flags().Changed("duration") {
		set["duration"] = vidDuration
	}
	if cmd.Flags().Changed("size") {
		set["size"] = vidSize
	}
	if cmd.Flags().Changed("resolution") {
		set["resolution"] = vidResolution
	}
	if cmd.Flags().Changed("seed") {
		set["seed"] = vidSeed
	}
	if cmd.Flags().Changed("generate-audio") {
		set["generate_audio"] = vidGenerateAudio
	}
	if cmd.Flags().Changed("return-last-frame") {
		set["return_last_frame"] = vidReturnLastFrame
	}

	for _, pair := range [][2]string{
		{"image-url", "image_urls"},
		{"video-url", "video_urls"},
		{"audio-url", "audio_urls"},
	} {
		if err := overlayVideoURLArray(cmd, set, pair[0], pair[1]); err != nil {
			return nil, err
		}
	}

	if cmd.Flags().Changed("tool") {
		tools := make([]map[string]any, 0, len(vidTools))
		for _, t := range vidTools {
			tools = append(tools, map[string]any{"type": t})
		}
		set["tools"] = tools
	}

	if firstSet, lastSet := cmd.Flags().Changed("first-frame"), cmd.Flags().Changed("last-frame"); firstSet || lastSet {
		set["image_with_roles"] = overlayVideoImageWithRoles(raw, vidFirstFrame, vidLastFrame, firstSet, lastSet)
	}

	return set, nil
}

// overlayVideoURLArray copies a repeatable URL flag into set under key.
func overlayVideoURLArray(cmd *cobra.Command, set map[string]any, flag, key string) error {
	if !cmd.Flags().Changed(flag) {
		return nil
	}
	urls, err := cmd.Flags().GetStringArray(flag)
	if err != nil {
		return fmt.Errorf("failed to read --%s: %w", flag, err)
	}
	set[key] = urls
	return nil
}

// overlayVideoImageWithRoles rebuilds image_with_roles from the JSON body plus
// the --first-frame/--last-frame flags. Entries of a role whose flag is set are
// overridden in place (dropped when the flag value is empty); every other entry
// the body carries is preserved.
func overlayVideoImageWithRoles(raw []byte, first, last string, firstSet, lastSet bool) []any {
	entries := rawImageWithRoles(raw)
	out := make([]any, 0, len(entries)+2)
	seenFirst, seenLast := false, false

	for _, entry := range entries {
		role, isEntry := rawImageRole(entry)
		switch {
		case isEntry && role == "first_frame" && firstSet:
			seenFirst = true
			if first != "" {
				out = append(out, types.ImageWithRole{URL: first, Role: "first_frame"})
			}
		case isEntry && role == "last_frame" && lastSet:
			seenLast = true
			if last != "" {
				out = append(out, types.ImageWithRole{URL: last, Role: "last_frame"})
			}
		default:
			out = append(out, entry)
		}
	}

	if firstSet && !seenFirst && first != "" {
		out = append(out, types.ImageWithRole{URL: first, Role: "first_frame"})
	}
	if lastSet && !seenLast && last != "" {
		out = append(out, types.ImageWithRole{URL: last, Role: "last_frame"})
	}
	return out
}

// rawImageWithRoles decodes the image_with_roles array of a raw JSON body.
// A missing array (or a body that is not an object) yields nil.
func rawImageWithRoles(raw []byte) []any {
	var body struct {
		ImageWithRoles []any `json:"image_with_roles"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &body) != nil {
		return nil
	}
	return body.ImageWithRoles
}

// rawImageRole returns the role of one image_with_roles entry.
func rawImageRole(entry any) (string, bool) {
	m, ok := entry.(map[string]any)
	if !ok {
		return "", false
	}
	role, ok := m["role"].(string)
	return role, ok
}
