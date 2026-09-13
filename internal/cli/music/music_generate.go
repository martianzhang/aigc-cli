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
	Short:        "Generate music (suno / flowmusic / Lyria-3)",
	SilenceUsage: true,
	Long: `Submit a music generation request.

APIMart (default) is async: submit -> poll -> download. --model selects the
request shape: suno (default) or flowmusic.

OpenRouter is synchronous streaming (Google Lyria-3): select it with
--provider openrouter and a google/lyria-3-* model.

Examples:
  aigc-cli music generate --prompt "city pop"
  aigc-cli music gen --prompt "rock" --model flowmusic
  aigc-cli music gen --prompt "lofi" --lyrics "..." --instrumental
  aigc-cli music gen --provider openrouter --model google/lyria-3-pro-preview --prompt "cinematic"
  aigc-cli music gen --json '{"model":"suno","prompt":"jazz"}'`,
	RunE: runMusicGenerate,
}

func runMusicGenerate(cmd *cobra.Command, _ []string) error {
	req, err := buildMusicGenerateReq(cmd)
	if err != nil {
		return err
	}
	p := options.Shared.ResolveProvider(options.ProviderNameMusic)
	return runMusic(newMusicClient(), p, req)
}

// runMusic dispatches to the provider-specific runner. OpenRouter Lyria runs
// synchronously over streaming chat completions; everything else uses the
// APIMart async submit/poll path. baseURL/apiKey come from the resolved
// provider so --dry-run renders the request against the actual endpoint.
func runMusic(c client.APIClient, p *provider.EffectiveProvider, req *types.MusicGenerateRequest) error {
	baseURL, apiKey := "", ""
	if p != nil {
		baseURL, apiKey = p.BaseURL, p.APIKey
	}
	if provider.Detect(c.BaseURL()) == provider.OpenRouter {
		return runMusicOpenRouter(c, baseURL, apiKey, req)
	}
	body, err := buildMusicBody(req)
	if err != nil {
		return err
	}
	return runMusicSubmitAndPoll(c, baseURL, apiKey, body)
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

	// --json overlay is merged last in buildMusicBody (its keys win).
	if musicJSONInput != "" {
		data, err := service.ReadInput(musicJSONInput)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		extras := map[string]any{}
		if err := json.Unmarshal(data, &extras); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		req.Extras = extras
	}

	return req, nil
}

// buildMusicBody maps a typed request to the backend-native body.
// Provider-specific shape and last-resort values live here (in code), and the
// --json overlay (Extras) is applied last so its keys always win.
func buildMusicBody(req *types.MusicGenerateRequest) (map[string]any, error) {
	model := req.Model
	if model == "" {
		model = "suno"
	}
	backend := "suno"
	if strings.Contains(strings.ToLower(model), "flowmusic") {
		backend = "flowmusic"
	}

	style := req.Prompt
	if style == "" {
		style = req.Style
	}

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

	// --json overlay last: its keys win over flags, config and code defaults.
	for k, v := range req.Extras {
		body[k] = v
	}

	// Validation against the final merged body.
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
