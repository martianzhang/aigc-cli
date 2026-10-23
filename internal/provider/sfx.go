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

	"github.com/martianzhang/aigc-cli/internal/jsonmerge"
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
	Model           string
	Format          string // mp3 or wav
	// Extra carries vendor-native fields from --inject-json. It is merged into
	// the ElevenLabs JSON body or the Stability multipart form, so provider
	// knobs (loop, prompt_influence, style, …) never become normalized flags.
	Extra json.RawMessage
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
			"provider %q does not support sound effects; set providers.<name>.base_url to %s or %s, or use --provider local",
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

	payload, err := ElevenLabsSFXPayload(req, model)
	if err != nil {
		return nil, err
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
	fields, err := StabilitySFXFields(req, model)
	if err != nil {
		return nil, err
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

// ElevenLabsSFXPayload builds the JSON body for /v1/sound-generation, merging
// vendor-native Extra fields. CLI-owned keys (text, model_id, duration_seconds)
// always win.
func ElevenLabsSFXPayload(req *SoundEffectRequest, model string) (map[string]any, error) {
	payload := map[string]any{
		"text":     req.Prompt,
		"model_id": model,
	}
	if req.DurationSeconds > 0 {
		payload["duration_seconds"] = req.DurationSeconds
	}

	merged, err := mergeJSONPayload(payload, req.Extra)
	if err != nil {
		return nil, err
	}
	merged["text"] = req.Prompt
	merged["model_id"] = model
	if req.DurationSeconds > 0 {
		merged["duration_seconds"] = req.DurationSeconds
	} else {
		delete(merged, "duration_seconds")
	}
	return merged, nil
}

// StabilitySFXFields builds the multipart fields for Stable Audio, merging
// vendor-native Extra fields. Multipart values must be scalars; nested objects
// and arrays are rejected instead of being silently stringified.
func StabilitySFXFields(req *SoundEffectRequest, model string) (map[string]string, error) {
	fields := map[string]string{
		"prompt":        req.Prompt,
		"model":         model,
		"output_format": stabilityOutputFormat(req.Format),
	}
	if req.DurationSeconds > 0 {
		fields["duration"] = strconv.FormatFloat(req.DurationSeconds, 'f', -1, 64)
	}
	if len(req.Extra) > 0 {
		var extra map[string]any
		dec := json.NewDecoder(bytes.NewReader(req.Extra))
		dec.UseNumber()
		if err := dec.Decode(&extra); err != nil {
			return nil, fmt.Errorf("inject sfx fields: %w", err)
		}
		for key, value := range extra {
			switch v := value.(type) {
			case string:
				fields[key] = v
			case json.Number:
				fields[key] = v.String()
			case bool:
				fields[key] = strconv.FormatBool(v)
			default:
				return nil, fmt.Errorf("inject sfx field %q: multipart providers only accept scalar values", key)
			}
		}
	}

	fields["prompt"] = req.Prompt
	fields["model"] = model
	fields["output_format"] = stabilityOutputFormat(req.Format)
	if req.DurationSeconds > 0 {
		fields["duration"] = strconv.FormatFloat(req.DurationSeconds, 'f', -1, 64)
	} else {
		delete(fields, "duration")
	}
	return fields, nil
}

func mergeJSONPayload(payload map[string]any, extra json.RawMessage) (map[string]any, error) {
	base, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode sfx payload: %w", err)
	}
	merged, err := jsonmerge.Inject(base, extra)
	if err != nil {
		return nil, fmt.Errorf("inject sfx payload: %w", err)
	}
	var out map[string]any
	dec := json.NewDecoder(bytes.NewReader(merged))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("decode sfx payload: %w", err)
	}
	return out, nil
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
