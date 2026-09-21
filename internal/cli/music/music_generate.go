package music

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// ============================================================================
// Subcommand: generate (alias gen)
// ============================================================================
var musicGenerateCmd = &cobra.Command{
	Use:          "generate",
	Aliases:      []string{"gen"},
	Short:        "Generate music (suno / flowmusic / Lyria-3 / Fun-Music)",
	SilenceUsage: true,
	Long: `Submit a music generation request.

APIMart (default) is async: submit -> poll -> download. --model selects the
request shape: suno (default) or flowmusic.

OpenRouter is synchronous streaming (Google Lyria-3): select it with
--provider openrouter and a google/lyria-3-* model.

Bailian (阿里云百炼) is synchronous (Fun-Music): select it with
--provider dashscope and a fun-music-* model.

Examples:
  aigc-cli music generate --prompt "city pop"
  aigc-cli music gen --prompt "rock" --model flowmusic
  aigc-cli music gen --prompt "lofi" --lyrics "..." --instrumental
  aigc-cli music gen --provider openrouter --model google/lyria-3-pro-preview --prompt "cinematic"
  aigc-cli music gen --provider dashscope --model fun-music-v1 --prompt "夏日清新民谣"
  aigc-cli music gen --json '{"model":"suno","prompt":"jazz"}'`,
	RunE: runMusicGenerate,
}

func runMusicGenerate(cmd *cobra.Command, _ []string) error {
	req, err := buildMusicGenerateReq(cmd)
	if err != nil {
		return err
	}
	p := options.Shared.ResolveProvider(options.ProviderNameMusic)
	// Explicitly-set flags override the backend-native --json body; with no
	// changed body flags the verbatim body is left untouched.
	if err := applyMusicJSONOverlay(changedMusicFlags(cmd), req, p); err != nil {
		return err
	}
	return runMusic(newMusicClient(), p, req)
}

// musicPromptStyle is the style value shared by the suno and flowmusic shapes:
// an explicit prompt wins, falling back to the style field.
func musicPromptStyle(req *types.MusicGenerateRequest) string {
	if req.Prompt != "" {
		return req.Prompt
	}
	return req.Style
}

// runMusic dispatches to the provider-specific runner via the strategy table.
// baseURL/apiKey come from the resolved provider so --dry-run renders the
// request against the actual endpoint.
func runMusic(c client.APIClient, p *provider.EffectiveProvider, req *types.MusicGenerateRequest) error {
	_, err := dispatchMusic(c, req, newMusicDispatchCtx(c, p))
	return err
}

// buildMusicGenerateReq builds a typed music request from flags, config
// defaults, and the --json overlay.
func buildMusicGenerateReq(cmd *cobra.Command) (*types.MusicGenerateRequest, error) {
	req := &types.MusicGenerateRequest{
		Model:  musicModel,
		Prompt: musicPrompt,
		Title:  musicTitle,
		Style:  musicStyle,
		Lyrics: musicLyrics,
		Format: musicFormat,
	}
	options.SetBoolFlag(cmd, "instrumental", &req.Instrumental, musicInstrumental)
	options.SetIntFlag(cmd, "duration", &req.Duration, musicDuration)

	// Config defaults fill only empty/nil fields — flags win.
	if cfg := options.MusicDefaults(); cfg != nil {
		cfg.MergeIntoMusic(req)
	}

	// --json is forwarded verbatim; it replaces the mapped body entirely.
	if musicJSONInput != "" {
		data, err := service.ReadJSONInput(musicJSONInput)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		var probe any
		if err := json.Unmarshal(data, &probe); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		req.RawJSON = data
	}

	return req, nil
}

// buildMusicBody maps a typed request to the backend-native body. A verbatim
// --json body is returned as-is, skipping the mapping and its validation.
func buildMusicBody(req *types.MusicGenerateRequest) (any, error) {
	if len(req.RawJSON) > 0 {
		return req.RawJSON, nil
	}

	model := req.Model
	if model == "" {
		model = "suno"
	}
	backend := "suno"
	if strings.Contains(strings.ToLower(model), "flowmusic") {
		backend = "flowmusic"
	}

	style := musicPromptStyle(req)

	body := map[string]any{"model": model}

	if backend == "flowmusic" {
		body["sound_prompt"] = style
		if req.Lyrics != "" && (req.Instrumental == nil || !*req.Instrumental) {
			body["lyrics"] = req.Lyrics
		}
		if req.Title != "" {
			body["title"] = req.Title
		}
		length := 120
		if req.Duration != nil {
			length = *req.Duration
		}
		body["length"] = length
	} else {
		instrumental := false
		if req.Instrumental != nil {
			instrumental = *req.Instrumental
		}
		body["instrumental"] = instrumental
		if req.Lyrics != "" {
			body["custom"] = true
			body["prompt"] = req.Lyrics
			if style != "" {
				body["style"] = style
			}
			if req.Title != "" {
				body["title"] = req.Title
			}
		} else {
			body["custom"] = false
			if style != "" {
				body["prompt"] = style
			}
		}
		body["version"] = "v6"
		if req.Duration != nil {
			body["duration"] = *req.Duration
		}
		if req.Format != "" {
			body["audio_format"] = req.Format
		}
	}

	// Validation against the built body.
	if backend == "flowmusic" {
		sound, _ := body["sound_prompt"].(string)
		lyrics, _ := body["lyrics"].(string)
		if sound == "" && lyrics == "" {
			return nil, fmt.Errorf("flowmusic requires sound_prompt or lyrics")
		}
	} else {
		prompt, _ := body["prompt"].(string)
		if prompt == "" {
			return nil, fmt.Errorf("suno requires a prompt")
		}
	}

	return body, nil
}
