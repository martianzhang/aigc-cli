package music

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// funMusicDefaultModel is the code last-resort model for Alibaba Fun-Music.
const funMusicDefaultModel = "fun-music-v1"

// BuildFunMusicBody maps a typed request to the DashScope-native Fun-Music body.
// Unlike the flat APIMart shapes all generation tunables live under "input".
// Fun-Music exposes no style or duration field: style folds into prompt and the
// duration is derived by the model from the lyric length, so an explicit
// --duration/--title is reported and dropped.
func BuildFunMusicBody(req *types.MusicGenerateRequest) (any, error) {
	if len(req.RawJSON) > 0 {
		return req.RawJSON, nil
	}
	if req.Duration != nil {
		fmt.Fprintln(os.Stderr, "Warning: fun-music ignores --duration (length is derived from the lyrics)")
	}
	if req.Title != "" {
		fmt.Fprintln(os.Stderr, "Warning: fun-music ignores --title")
	}

	model := req.Model
	if !strings.HasPrefix(model, "fun-music") {
		model = funMusicDefaultModel
	}

	prompt := req.Prompt
	if req.Style != "" {
		if prompt == "" {
			prompt = req.Style
		} else {
			prompt = prompt + "，" + req.Style
		}
	}

	input := map[string]any{}
	if prompt != "" {
		input["prompt"] = prompt
	}
	if req.Lyrics != "" {
		input["lyrics"] = req.Lyrics
	}
	if req.Format != "" {
		input["format"] = req.Format
	}
	if req.Instrumental != nil {
		input["is_instrumental"] = *req.Instrumental
	}

	body := map[string]any{"model": model, "input": input}

	// is_instrumental=true makes lyrics and gender invalid per the API contract.
	if inst, _ := input["is_instrumental"].(bool); inst {
		delete(input, "lyrics")
		delete(input, "gender")
	}

	lyrics, _ := input["lyrics"].(string)
	promptText, _ := input["prompt"].(string)
	if promptText == "" && lyrics == "" {
		return nil, fmt.Errorf("fun-music requires a prompt or lyrics")
	}
	if len([]rune(promptText)) > 2000 {
		return nil, fmt.Errorf("fun-music prompt exceeds the 2000 character limit")
	}

	return body, nil
}

// buildFunMusicCurl renders the equivalent curl command for the native endpoint.
func buildFunMusicCurl(baseURL, apiKey string, body any) string {
	bodyJSON, _ := json.Marshal(body)
	url := client.FunMusicEndpoint(baseURL)

	cmd := fmt.Sprintf("curl -X POST '%s' \\\n", url)
	cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", service.MaskKey(apiKey))
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(bodyJSON))
	return cmd
}

// runFunMusicMusic runs the DashScope-native synchronous Fun-Music path.
func runFunMusicMusic(c client.APIClient, req *types.MusicGenerateRequest, ctx *musicDispatchCtx) ([]string, error) {
	body, err := BuildFunMusicBody(req)
	if err != nil {
		return nil, err
	}

	if musicDryRun {
		fmt.Println(buildFunMusicCurl(ctx.baseURL, ctx.apiKey, body))
		return nil, nil
	}

	if options.Shared.Verbose {
		pretty, _ := json.MarshalIndent(body, "", "  ")
		fmt.Printf("Request:\n%s\n\n", string(pretty))
	}

	resp, err := c.FunMusicGenerate(body)
	if err != nil {
		return nil, fmt.Errorf("fun-music generation failed: %w", err)
	}

	track := resp.Track()
	if built, ok := body.(map[string]any); ok {
		fmt.Printf("Model: %v\n", built["model"])
	}
	fmt.Printf("Duration: %ds\n", resp.Usage.Duration)
	if track.AudioURL != "" {
		fmt.Printf("Audio: %s\n", track.AudioURL)
	}
	if options.Shared.Verbose && track.Lyrics != "" {
		fmt.Printf("Lyrics:\n%s\n", track.Lyrics)
	}

	label := resp.Output.Audio.ID
	if label == "" {
		label = resp.RequestID
	}
	return downloadMusics([]types.MusicTrack{track}, label)
}
