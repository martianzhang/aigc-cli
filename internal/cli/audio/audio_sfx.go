package audio

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

var sfxCmd = &cobra.Command{
	Use:          "sfx",
	Short:        "Generate a short sound effect from text",
	SilenceUsage: true,
	Long: `Generate a short sound effect (SFX) from a text description.

Unlike the music command, this targets one-shots: whooshes, impacts, UI
sounds, footsteps and ambience. Duration is explicit, a seamless loop can be
requested, and prompt adherence is tunable.

Supported providers (detected from the provider base URL):
  elevenlabs   POST /v1/sound-generation, 0.5-30s, loop + prompt influence
  stability    POST /v2beta/audio/stable-audio-2/text-to-audio, 1-190s
  local        Stable Audio Open Small (ONNX, CPU, up to 11.9s, wav only) via -P local

Configure a named provider and select it with --provider, or run
'aigc-cli audio sfx ... --provider elevenlabs' with ELEVENLABS_API_KEY set:

  providers:
    elevenlabs:
      base_url: https://api.elevenlabs.io
      api_key: "xi-xxx"
  defaults:
    audio:
      sfx:
        provider: elevenlabs

Examples:
  aigc-cli audio sfx --input "heavy metal door slam with echo" --duration 1.5
  aigc-cli audio sfx --input "rain on a car roof" --duration 10 --loop
  aigc-cli audio sfx --input "sci-fi UI confirmation beep" --duration 0.8 \
    --influence 0.8 --format wav
  aigc-cli audio sfx --input "cinematic whoosh" --dry-run
  aigc-cli audio sfx -P local -m stable-audio-open-small \
    --input "heavy metal door slam" -d 2 --seed 42`,
	RunE: runAudioSFX,
}

var (
	sfxInput       string
	sfxDuration    float64
	sfxLoop        bool
	sfxInfluence   float64
	sfxModel       string
	sfxFormat      string
	sfxPlay        bool
	sfxSeed        int64
	sfxNoNormalize bool
)

func registerAudioSFXFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&sfxInput, "input", "i", "", "Sound description (file path, raw text, or omit to auto-detect piped stdin)")
	f.StringVarP(&sfxModel, "model", "m", "", "SFX model (online default: eleven_text_to_sound_v2 / stable-audio-2.5; local default: stable-audio-open-small)")
	f.Float64VarP(&sfxDuration, "duration", "d", 0, "Duration in seconds (0 = let the provider decide)")
	f.BoolVar(&sfxLoop, "loop", false, "Generate a seamlessly looping sound (ElevenLabs only)")
	f.Float64Var(&sfxInfluence, "influence", 0.3, "Prompt adherence 0-1 (ElevenLabs only, default 0.3)")
	f.StringVarP(&sfxFormat, "format", "f", "", "Audio format: mp3 or wav (default: mp3)")
	f.BoolVar(&sfxPlay, "play", false, "Play the generated sound after saving")
	f.Int64Var(&sfxSeed, "seed", 0, "Random seed for local generation (0 = time-based)")
	f.BoolVar(&sfxNoNormalize, "no-normalize", false, "Keep raw local output levels instead of peak-normalizing to -1 dBFS")
}

func runAudioSFX(cmd *cobra.Command, _ []string) error {
	prompt, err := readSFXInput()
	if err != nil {
		return err
	}

	p := resolveSFXProvider()
	if isLocalSFXProvider(p) {
		if err := checkLocalSFXFlags(cmd); err != nil {
			return err
		}
		return runLocalSFX(prompt, p)
	}
	if err := checkOnlineSFXFlags(cmd); err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("no provider configured for sound effects")
	}

	req, err := buildSFXRequest(cmd, prompt, p)
	if err != nil {
		return err
	}

	if audioDryRun {
		fmt.Println(buildSFXCurl(p, req))
		return nil
	}

	if options.Shared.Verbose {
		fmt.Printf("Provider: %s (%s)\n", firstNonEmpty(p.Name, p.ProviderType.String()), p.BaseURL)
		fmt.Printf("Model: %s\n", req.Model)
		fmt.Printf("Prompt: %d chars, duration=%.1fs loop=%v\n", len(req.Prompt), req.DurationSeconds, req.Loop)
	}

	start := time.Now()
	result, err := provider.GenerateSoundEffect(p, req)
	if err != nil {
		return fmt.Errorf("sound effect failed: %w", err)
	}
	elapsed := time.Since(start)

	format := req.Format
	if format == "" {
		format = "mp3"
	}
	if result.ContentType != "" {
		format = audioFormatFromContentType(result.ContentType)
	}

	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Format: %s\n", format)
	fmt.Printf("Size: %d bytes\n", len(result.Data))
	fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())

	filename, err := saveAudioFile(result.Data, format)
	if err != nil {
		return fmt.Errorf("failed to save audio: %w", err)
	}
	fmt.Printf("Saved: %s\n", filename)

	if sfxPlay {
		if err := service.PreviewFile(filename); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: playback failed: %v\n", err)
		}
	}
	return nil
}

// readSFXInput mirrors the TTS input handling: an explicit --input value is a
// path or raw text, and an empty value reads a pipe/file stdin.
func readSFXInput() (string, error) {
	src := sfxInput
	if src == "" {
		stat, err := os.Stdin.Stat()
		if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return "", fmt.Errorf("failed to read stdin: %w", err)
			}
			src = string(data)
		}
	} else {
		data, err := service.ReadInput(src)
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
		src = string(data)
	}
	src = strings.TrimSpace(src)
	if src == "" {
		return "", fmt.Errorf("sound description is required: set via --input, a file path, or stdin")
	}
	return src, nil
}

// isLocalSFXProvider reports whether provider selection points at local ONNX
// inference, either through -P/--provider local or defaults.audio.sfx.provider.
func isLocalSFXProvider(p *provider.EffectiveProvider) bool {
	return p != nil && p.Type == types.ProviderLocal
}

// resolveSFXProvider resolves defaults.sfx.provider / --provider, then falls
// back to the built-in ElevenLabs and Stability aliases so the command works
// with just an environment variable.
func resolveSFXProvider() *provider.EffectiveProvider {
	p := options.Shared.ResolveProvider(options.ProviderNameAudioSFX)
	if p != nil && (p.ProviderType == provider.ElevenLabs || p.ProviderType == provider.Stability) {
		return p
	}

	name := strings.ToLower(strings.TrimSpace(sfxProviderName()))
	switch name {
	case "elevenlabs":
		key := firstNonEmpty(providerKey(p), os.Getenv("ELEVENLABS_API_KEY"))
		return &provider.EffectiveProvider{
			Name:         "elevenlabs",
			Type:         types.ProviderOpenAI,
			BaseURL:      sfxBaseURL(p, provider.ElevenLabsBaseURL),
			APIKey:       key,
			ProviderType: provider.ElevenLabs,
		}
	case "stability":
		key := firstNonEmpty(providerKey(p), os.Getenv("STABILITY_API_KEY"))
		return &provider.EffectiveProvider{
			Name:         "stability",
			Type:         types.ProviderOpenAI,
			BaseURL:      sfxBaseURL(p, provider.StabilityBaseURL),
			APIKey:       key,
			ProviderType: provider.Stability,
		}
	}
	return p
}

// sfxBaseURL keeps an explicitly configured base URL (proxy, mock server) and
// only falls back to the vendor default when the resolver returned the global
// APIMart default.
func sfxBaseURL(p *provider.EffectiveProvider, fallback string) string {
	if p != nil {
		base := strings.TrimSpace(p.BaseURL)
		if base != "" && base != types.DefaultAPIBaseURL {
			return base
		}
	}
	return fallback
}

// sfxProviderName returns the explicitly selected provider name, if any.
func sfxProviderName() string {
	if options.Shared.ProviderSet {
		return options.Shared.Provider
	}
	if cfg := options.AudioDefaults(); cfg != nil {
		return cfg.SFXProvider()
	}
	return ""
}

func providerKey(p *provider.EffectiveProvider) string {
	if p == nil {
		return ""
	}
	return p.APIKey
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// buildSFXRequest applies config defaults and validates vendor limits.
func buildSFXRequest(cmd *cobra.Command, prompt string, p *provider.EffectiveProvider) (*provider.SoundEffectRequest, error) {
	req := &provider.SoundEffectRequest{
		Prompt:          prompt,
		DurationSeconds: sfxDuration,
		Loop:            sfxLoop,
		Model:           sfxModel,
		Format:          strings.ToLower(strings.TrimSpace(sfxFormat)),
	}
	if cmd.Flags().Changed("influence") {
		v := sfxInfluence
		req.PromptInfluence = &v
	}

	if cfg := options.AudioDefaults(); cfg != nil {
		if req.Model == "" {
			req.Model = cfg.SFXModel()
		}
		if req.Format == "" {
			req.Format = strings.ToLower(strings.TrimSpace(cfg.SFXFormat()))
		}
	}

	if req.DurationSeconds < 0 {
		return nil, fmt.Errorf("duration must be zero (auto) or positive")
	}
	if req.PromptInfluence != nil && (*req.PromptInfluence < 0 || *req.PromptInfluence > 1) {
		return nil, fmt.Errorf("influence must be between 0 and 1")
	}
	if req.Format != "" && req.Format != "mp3" && req.Format != "wav" {
		return nil, fmt.Errorf("format must be mp3 or wav, got %q", req.Format)
	}

	switch p.ProviderType {
	case provider.ElevenLabs:
		if req.DurationSeconds > 0 && (req.DurationSeconds < 0.5 || req.DurationSeconds > 30) {
			return nil, fmt.Errorf("elevenlabs duration must be 0.5-30s, got %.1fs", req.DurationSeconds)
		}
		if req.Loop && req.DurationSeconds > 22 {
			return nil, fmt.Errorf("elevenlabs looping sounds are limited to 22s")
		}
		if req.Model == "" {
			req.Model = provider.DefaultSFXModelElevenLabs
		}
	case provider.Stability:
		if req.Loop {
			return nil, fmt.Errorf("stability does not support --loop")
		}
		if req.DurationSeconds == 0 {
			req.DurationSeconds = 5
		}
		if req.DurationSeconds < 1 || req.DurationSeconds > 190 {
			return nil, fmt.Errorf("stability duration must be 1-190s, got %.1fs", req.DurationSeconds)
		}
		if req.PromptInfluence != nil {
			return nil, fmt.Errorf("stability does not support --influence")
		}
		if req.Model == "" {
			req.Model = provider.DefaultSFXModelStability
		}
	}
	return req, nil
}

// buildSFXCurl renders the equivalent curl for --dry-run.
func buildSFXCurl(p *provider.EffectiveProvider, req *provider.SoundEffectRequest) string {
	base := strings.TrimRight(p.BaseURL, "/")
	masked := service.MaskKey(p.APIKey)
	format := req.Format
	if format == "" {
		format = "mp3"
	}

	var b strings.Builder
	switch p.ProviderType {
	case provider.ElevenLabs:
		outputFormat := "mp3_44100_128"
		if format == "wav" {
			outputFormat = "wav_44100"
		}
		payload := map[string]any{
			"text":             req.Prompt,
			"model_id":         firstNonEmpty(req.Model, provider.DefaultSFXModelElevenLabs),
			"loop":             req.Loop,
			"prompt_influence": defaultInfluence(req.PromptInfluence),
		}
		if req.DurationSeconds > 0 {
			payload["duration_seconds"] = req.DurationSeconds
		}
		data, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Fprintf(&b, "curl -X POST %s/v1/sound-generation?output_format=%s \\\n", base, outputFormat)
		fmt.Fprintf(&b, "  -H \"xi-api-key: %s\" \\\n", masked)
		b.WriteString("  -H \"Content-Type: application/json\" \\\n")
		fmt.Fprintf(&b, "  -d '%s' \\\n", string(data))
		fmt.Fprintf(&b, "  --output sfx.%s\n", format)
	case provider.Stability:
		fmt.Fprintf(&b, "curl -X POST %s/v2beta/audio/stable-audio-2/text-to-audio \\\n", base)
		fmt.Fprintf(&b, "  -H \"Authorization: Bearer %s\" \\\n", masked)
		b.WriteString("  -H \"Accept: audio/*\" \\\n")
		fmt.Fprintf(&b, "  -F prompt=%q \\\n", req.Prompt)
		fmt.Fprintf(&b, "  -F model=%q \\\n", firstNonEmpty(req.Model, provider.DefaultSFXModelStability))
		fmt.Fprintf(&b, "  -F duration=%.1f \\\n", req.DurationSeconds)
		fmt.Fprintf(&b, "  -F output_format=%s \\\n", formatForStability(format))
		fmt.Fprintf(&b, "  --output sfx.%s\n", format)
	default:
		fmt.Fprintf(&b, "# unsupported provider type: %s\n", p.ProviderType)
	}
	return b.String()
}

func defaultInfluence(v *float64) float64 {
	if v == nil {
		return 0.3
	}
	return *v
}

func formatForStability(format string) string {
	if format == "wav" {
		return "wav"
	}
	return "mp3"
}
