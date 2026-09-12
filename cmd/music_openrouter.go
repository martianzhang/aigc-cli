package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// openRouterMusicDefaultModel is the code last-resort model for OpenRouter music.
const openRouterMusicDefaultModel = "google/lyria-3-clip-preview"

// buildOpenRouterMusicReq maps a typed request to OpenRouter's sync streaming
// chat-completions music shape. lyrics/instrumental/duration are folded into
// the prompt text because they are not documented request fields.
func buildOpenRouterMusicReq(req *types.MusicGenerateRequest) *types.OpenRouterMusicRequest {
	model := req.Model
	if model == "" {
		model = openRouterMusicDefaultModel
	}

	text := req.Prompt
	if text == "" {
		text = req.Style
	}
	if req.Instrumental != nil && *req.Instrumental {
		text = "[Instrumental] " + text
	}
	if req.Lyrics != "" {
		text += "\n\nLyrics:\n" + req.Lyrics
	}
	if req.Duration != nil {
		text += fmt.Sprintf("\n\nTarget duration: about %d seconds.", *req.Duration)
	}

	format := req.Format
	if format == "" {
		format = "mp3"
	}

	return &types.OpenRouterMusicRequest{
		Model:      model,
		Messages:   []types.OpenRouterMusicMessage{{Role: "user", Content: text}},
		Modalities: []string{"text", "audio"},
		Audio:      &types.OpenRouterAudioConfig{Format: format},
		Stream:     true,
	}
}

// buildOpenRouterMusicCurl renders the equivalent streaming curl command.
func buildOpenRouterMusicCurl(baseURL, apiKey string, req *types.MusicGenerateRequest) string {
	bodyJSON, _ := json.Marshal(buildOpenRouterMusicReq(req))
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	if !client.HasVersionSuffix(base) {
		base += "/v1"
	}
	url := base + "/chat/completions"

	cmd := fmt.Sprintf("curl -N -X POST %s \\\n", url)
	cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", maskKey(apiKey))
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(bodyJSON))
	return cmd
}

// runMusicOpenRouter streams a music generation and saves the decoded audio.
func runMusicOpenRouter(c client.APIClient, baseURL, apiKey string, req *types.MusicGenerateRequest) error {
	if musicDryRun {
		fmt.Println(buildOpenRouterMusicCurl(baseURL, apiKey, req))
		return nil
	}

	orReq := buildOpenRouterMusicReq(req)
	if shared.Verbose {
		pretty, _ := json.MarshalIndent(orReq, "", "  ")
		fmt.Printf("Request:\n%s\n\n", string(pretty))
	}

	audio, transcript, err := c.OpenRouterMusicGenerate(orReq)
	if err != nil {
		return fmt.Errorf("openrouter music generation failed: %w", err)
	}

	format := "mp3"
	if orReq.Audio != nil && orReq.Audio.Format != "" {
		format = orReq.Audio.Format
	}
	saved, err := saveAudioFile(audio, format)
	if err != nil {
		return fmt.Errorf("failed to save music: %w", err)
	}
	fmt.Printf("Saved: %s\n", saved)
	if transcript != "" {
		fmt.Println(transcript)
	}
	return nil
}
