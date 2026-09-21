package music

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// musicBackend identifies the backend-native body shape a --json overlay
// targets. Each shape has a different flag->key mapping, so a flat struct-tag
// overlay is not correct here.
type musicBackend int

const (
	backendSuno musicBackend = iota
	backendFlowMusic
	backendOpenRouter
	backendFunMusic
)

// musicBodyFlags lists the only flags permitted to write into a --json body.
// Behavioral/global flags (--json, --dry-run, --provider, --api-key, --api-base,
// --http-proxy, --config, --output, --verbose, --timeout, --print-config and the
// shadowed global --model) are intentionally absent, so they can never leak into
// the backend-native body.
var musicBodyFlags = []string{
	"model", "prompt", "style", "title", "lyrics", "instrumental", "duration", "format",
}

// changedMusicFlags returns the subset of body flags explicitly set on the
// command line. Only these flags may override the --json body.
func changedMusicFlags(cmd *cobra.Command) map[string]bool {
	changed := make(map[string]bool)
	for _, name := range musicBodyFlags {
		if cmd.Flags().Changed(name) {
			changed[name] = true
		}
	}
	return changed
}

// peekMusicRaw reads the parts of the backend-native --json body needed to pick
// the backend shape (model) and to know whether the body already carries custom
// lyrics (suno's custom flag).
func peekMusicRaw(raw json.RawMessage) (model string, custom bool) {
	var probe struct {
		Model  string `json:"model"`
		Custom bool   `json:"custom"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return "", false
	}
	return probe.Model, probe.Custom
}

// resolveMusicOverlayBackend mirrors dispatchMusic's provider detection so the
// overlay uses the same key mapping as the backend that will receive the body.
// For APIMart the model selects the shape (suno vs flowmusic).
func resolveMusicOverlayBackend(p *provider.EffectiveProvider, model string) musicBackend {
	baseURL := ""
	if p != nil {
		baseURL = p.BaseURL
	}
	switch provider.Detect(baseURL) {
	case provider.OpenRouter:
		return backendOpenRouter
	case provider.Bailian:
		return backendFunMusic
	}
	if strings.Contains(strings.ToLower(model), "flowmusic") {
		return backendFlowMusic
	}
	return backendSuno
}

// buildMusicOverlaySet returns the backend-native key/value set contributed by
// the explicitly-set flags, using the same mapping as the active backend's body
// builder. rawCustom reports whether the --json body already requests suno
// custom (lyric) mode, so prompt/style/title land on the same keys the builder
// would choose.
func buildMusicOverlaySet(backend musicBackend, req *types.MusicGenerateRequest, changed map[string]bool, rawCustom bool) map[string]any {
	set := make(map[string]any)
	if req == nil {
		return set
	}
	switch backend {
	case backendFlowMusic:
		flowMusicOverlaySet(set, req, changed)
	case backendOpenRouter:
		openRouterOverlaySet(set, req, changed)
	case backendFunMusic:
		funMusicOverlaySet(set, req, changed)
	default:
		sunoOverlaySet(set, req, changed, rawCustom)
	}
	return set
}

// sunoOverlaySet mirrors buildMusicBody's suno branch: --lyrics switches to
// custom mode (prompt=lyrics, custom=true) and --prompt then becomes style;
// without lyrics --prompt (or --style as fallback) lands in prompt.
func sunoOverlaySet(set map[string]any, req *types.MusicGenerateRequest, changed map[string]bool, rawCustom bool) {
	if changed["model"] {
		model := req.Model
		if model == "" {
			model = "suno"
		}
		set["model"] = model
	}
	if changed["instrumental"] && req.Instrumental != nil {
		set["instrumental"] = *req.Instrumental
	}

	lyricsPresent := req.Lyrics != "" || rawCustom
	if changed["lyrics"] {
		if req.Lyrics != "" {
			set["custom"] = true
			set["prompt"] = req.Lyrics
		} else {
			set["custom"] = false
		}
	}

	// Same derivation as buildMusicBody: an explicit prompt wins over style.
	style := musicPromptStyle(req)
	if changed["prompt"] || changed["style"] {
		if lyricsPresent {
			set["style"] = style
		} else {
			set["prompt"] = style
		}
	}
	// The suno builder only emits title in custom (lyrics) mode.
	if lyricsPresent && changed["title"] {
		set["title"] = req.Title
	}

	if changed["duration"] && req.Duration != nil {
		set["duration"] = *req.Duration
	}
	if changed["format"] {
		set["audio_format"] = req.Format
	}
}

// flowMusicOverlaySet mirrors buildMusicBody's flowmusic branch. --instrumental
// suppresses lyrics (flowmusic has no instrumental field); --format is dropped
// because the builder never emits it.
func flowMusicOverlaySet(set map[string]any, req *types.MusicGenerateRequest, changed map[string]bool) {
	if changed["model"] {
		set["model"] = req.Model
	}
	if changed["prompt"] || changed["style"] {
		set["sound_prompt"] = musicPromptStyle(req)
	}
	instrumental := req.Instrumental != nil && *req.Instrumental
	if changed["lyrics"] && !instrumental {
		set["lyrics"] = req.Lyrics
	}
	if changed["title"] {
		set["title"] = req.Title
	}
	if changed["duration"] && req.Duration != nil {
		set["length"] = *req.Duration
	}
	// Instrumental and format write no flowmusic key (the builder emits neither).
}

// openRouterOverlaySet overlays the flags OpenRouter maps to a stable key.
// prompt/style (and lyrics/instrumental/duration) fold into the composite
// messages[0].content, which cannot be rewritten without discarding whatever
// content the raw body already carries, so they are intentionally not overlaid.
func openRouterOverlaySet(set map[string]any, req *types.MusicGenerateRequest, changed map[string]bool) {
	if changed["model"] {
		model := req.Model
		if model == "" {
			model = openRouterMusicDefaultModel
		}
		set["model"] = model
	}
	if changed["format"] {
		set["audio.format"] = req.Format
	}
}

// funMusicOverlaySet mirrors BuildFunMusicBody: generation tunables live under
// "input"; --duration and --title are dropped because Fun-Music has no such
// fields and the builder ignores them.
func funMusicOverlaySet(set map[string]any, req *types.MusicGenerateRequest, changed map[string]bool) {
	if changed["model"] {
		set["model"] = funMusicModel(req.Model)
	}
	if changed["prompt"] || changed["style"] {
		set["input.prompt"] = funMusicPrompt(req)
	}
	instrumental := req.Instrumental != nil && *req.Instrumental
	if changed["instrumental"] && req.Instrumental != nil {
		set["input.is_instrumental"] = *req.Instrumental
	}
	if changed["lyrics"] && !instrumental {
		set["input.lyrics"] = req.Lyrics
	}
	if changed["format"] {
		set["input.format"] = req.Format
	}
}

// applyMusicJSONOverlay rewrites req.RawJSON so explicitly-set flags override
// the backend-native --json body. It is a no-op when --json was not given or no
// body flag was set, so a pure --json request stays byte-identical.
func applyMusicJSONOverlay(changed map[string]bool, req *types.MusicGenerateRequest, p *provider.EffectiveProvider) error {
	if req == nil || len(req.RawJSON) == 0 || len(changed) == 0 {
		return nil
	}

	// Priority for the shape: an explicit --model wins, then the raw body's own
	// model, then any config default already merged into req.Model.
	rawModel, rawCustom := peekMusicRaw(req.RawJSON)
	model := req.Model
	if !changed["model"] && rawModel != "" {
		model = rawModel
	}
	backend := resolveMusicOverlayBackend(p, model)

	set := buildMusicOverlaySet(backend, req, changed, rawCustom)
	if len(set) == 0 {
		return nil
	}

	merged, err := service.MergeJSONOverlay(req.RawJSON, set)
	if err != nil {
		return fmt.Errorf("failed to apply --json overlay: %w", err)
	}
	req.RawJSON = merged
	return nil
}
