package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// ElevenLabsBaseURL is the default ElevenLabs API root.
	ElevenLabsBaseURL = "https://api.elevenlabs.io"
	// StabilityBaseURL is the default Stability AI API root.
	StabilityBaseURL = "https://api.stability.ai"

	// DefaultSFXModelElevenLabs is the default sound-effects model.
	DefaultSFXModelElevenLabs = "eleven_text_to_sound_v2"
	// DefaultSFXModelStability is the default Stable Audio model.
	DefaultSFXModelStability = "stable-audio-2.5"

	sfxTimeout = 5 * time.Minute
)

// SoundEffectRequest describes one text-to-SFX generation. Zero values mean
// "let the vendor decide" (except Duration, which the CLI normalizes).
type SoundEffectRequest struct {
	Prompt          string
	DurationSeconds float64
	Loop            bool
	PromptInfluence *float64
	Model           string
	Format          string // mp3 or wav
}

// SoundEffectResult is the generated audio plus the vendor's content type.
type SoundEffectResult struct {
	Data        []byte
	ContentType string
}

// GenerateSoundEffect dispatches a text-to-SFX request to the provider that
// the base URL identifies. Only ElevenLabs and Stability are supported today;
// other providers get an actionable error instead of a confusing 404.
func GenerateSoundEffect(p *EffectiveProvider, req *SoundEffectRequest) (*SoundEffectResult, error) {
	if p == nil {
		return nil, fmt.Errorf("no provider configured for sound effects")
	}
	switch p.ProviderType {
	case ElevenLabs:
		return generateElevenLabsSFX(p, req)
	case Stability:
		return generateStabilitySFX(p, req)
	default:
		return nil, fmt.Errorf(
			"provider %q does not support sound effects; set providers.<name>.base_url to %s or %s, or use --local",
			firstNonEmpty(p.Name, p.BaseURL), ElevenLabsBaseURL, StabilityBaseURL)
	}
}

// generateElevenLabsSFX calls POST /v1/sound-generation.
func generateElevenLabsSFX(p *EffectiveProvider, req *SoundEffectRequest) (*SoundEffectResult, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("elevenlabs requires an api key (--api-key, providers.<name>.api_key, or ELEVENLABS_API_KEY)")
	}
	model := req.Model
	if model == "" {
		model = DefaultSFXModelElevenLabs
	}

	payload := map[string]any{
		"text":             req.Prompt,
		"model_id":         model,
		"loop":             req.Loop,
		"prompt_influence": defaultPromptInfluence(req.PromptInfluence),
	}
	if req.DurationSeconds > 0 {
		payload["duration_seconds"] = req.DurationSeconds
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode elevenlabs payload: %w", err)
	}

	endpoint := strings.TrimRight(p.BaseURL, "/") + "/v1/sound-generation"
	if format := elevenLabsOutputFormat(req.Format); format != "" {
		endpoint += "?output_format=" + url.QueryEscape(format)
	}

	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "audio/*")
	httpReq.Header.Set("xi-api-key", p.APIKey)

	return doSFXRequest(httpReq, "elevenlabs")
}

// generateStabilitySFX calls POST /v2beta/audio/stable-audio-2/text-to-audio.
func generateStabilitySFX(p *EffectiveProvider, req *SoundEffectRequest) (*SoundEffectResult, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("stability requires an api key (--api-key, providers.<name>.api_key, or STABILITY_API_KEY)")
	}
	model := req.Model
	if model == "" {
		model = DefaultSFXModelStability
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	fields := map[string]string{
		"prompt":        req.Prompt,
		"model":         model,
		"output_format": stabilityOutputFormat(req.Format),
	}
	if req.DurationSeconds > 0 {
		fields["duration"] = strconv.FormatFloat(req.DurationSeconds, 'f', -1, 64)
	}
	for name, value := range fields {
		if err := form.WriteField(name, value); err != nil {
			return nil, fmt.Errorf("encode stability form: %w", err)
		}
	}
	if err := form.Close(); err != nil {
		return nil, fmt.Errorf("close stability form: %w", err)
	}

	endpoint := strings.TrimRight(p.BaseURL, "/") + "/v2beta/audio/stable-audio-2/text-to-audio"
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, fmt.Errorf("stability request: %w", err)
	}
	httpReq.Header.Set("Content-Type", form.FormDataContentType())
	httpReq.Header.Set("Accept", "audio/*")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	return doSFXRequest(httpReq, "stability")
}

// doSFXRequest sends the request and returns the audio bytes. Vendor errors are
// truncated so a JSON error body stays readable in the terminal.
func doSFXRequest(req *http.Request, vendor string) (*SoundEffectResult, error) {
	client := &http.Client{Timeout: sfxTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s sound effect: %w", vendor, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("%s read: %w", vendor, err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s API error %d: %s", vendor, resp.StatusCode, truncateError(string(data)))
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s returned an empty audio response", vendor)
	}
	return &SoundEffectResult{Data: data, ContentType: resp.Header.Get("Content-Type")}, nil
}

func defaultPromptInfluence(v *float64) float64 {
	if v == nil {
		return 0.3
	}
	return *v
}

// elevenLabsOutputFormat maps a user-facing format to ElevenLabs' output_format
// values. Empty means the vendor default (mp3 44.1kHz 128kbps).
func elevenLabsOutputFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "mp3":
		return "mp3_44100_128"
	case "wav":
		return "wav_44100"
	case "opus":
		return "opus_48000_128"
	case "pcm":
		return "pcm_44100"
	default:
		return "mp3_44100_128"
	}
}

// stabilityOutputFormat maps a user-facing format to Stable Audio's field.
func stabilityOutputFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav":
		return "wav"
	default:
		return "mp3"
	}
}

// truncateError keeps vendor error bodies to a readable length.
func truncateError(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500] + "…"
	}
	return s
}
