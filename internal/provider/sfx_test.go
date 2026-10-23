package provider

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateSoundEffectUnsupportedProvider(t *testing.T) {
	p := &EffectiveProvider{Name: "openai", BaseURL: "https://api.example.com", ProviderType: OpenAI}
	if _, err := GenerateSoundEffect(p, &SoundEffectRequest{Prompt: "whoosh"}); err == nil {
		t.Fatal("GenerateSoundEffect() error = nil, want unsupported provider error")
	}
}

func TestGenerateSoundEffectElevenLabs(t *testing.T) {
	var gotPath, gotKey string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotKey = r.Header.Get("xi-api-key")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("fake-mp3"))
	}))
	defer srv.Close()

	p := &EffectiveProvider{
		Name:         "elevenlabs",
		BaseURL:      srv.URL,
		APIKey:       "xi-key",
		ProviderType: ElevenLabs,
	}
	res, err := GenerateSoundEffect(p, &SoundEffectRequest{
		Prompt:          "door slam",
		DurationSeconds: 1.5,
		Format:          "wav",
		Extra:           json.RawMessage(`{"loop":true,"prompt_influence":0.8}`),
	})
	if err != nil {
		t.Fatalf("GenerateSoundEffect() error = %v", err)
	}

	if gotPath != "/v1/sound-generation?output_format=wav_44100" {
		t.Errorf("path = %q, want /v1/sound-generation with wav format", gotPath)
	}
	if gotKey != "xi-key" {
		t.Errorf("xi-api-key = %q, want xi-key", gotKey)
	}
	if gotBody["text"] != "door slam" {
		t.Errorf("text = %v, want door slam", gotBody["text"])
	}
	if gotBody["model_id"] != DefaultSFXModelElevenLabs {
		t.Errorf("model_id = %v, want %s", gotBody["model_id"], DefaultSFXModelElevenLabs)
	}
	if gotBody["loop"] != true {
		t.Errorf("loop = %v, want true", gotBody["loop"])
	}
	if gotBody["duration_seconds"] != 1.5 {
		t.Errorf("duration_seconds = %v, want 1.5", gotBody["duration_seconds"])
	}
	if gotBody["prompt_influence"] != 0.8 {
		t.Errorf("prompt_influence = %v, want 0.8", gotBody["prompt_influence"])
	}
	if string(res.Data) != "fake-mp3" {
		t.Errorf("data = %q, want fake-mp3", res.Data)
	}
	if res.ContentType != "audio/mpeg" {
		t.Errorf("content type = %q, want audio/mpeg", res.ContentType)
	}
}

func TestGenerateSoundEffectElevenLabsOmitsAutoDuration(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = w.Write([]byte("audio"))
	}))
	defer srv.Close()

	p := &EffectiveProvider{BaseURL: srv.URL, APIKey: "k", ProviderType: ElevenLabs}
	if _, err := GenerateSoundEffect(p, &SoundEffectRequest{Prompt: "rain"}); err != nil {
		t.Fatalf("GenerateSoundEffect() error = %v", err)
	}
	if _, ok := gotBody["duration_seconds"]; ok {
		t.Errorf("duration_seconds should be omitted when 0, body = %v", gotBody)
	}
	if _, ok := gotBody["prompt_influence"]; ok {
		t.Errorf("prompt_influence must only come from --inject-json, body = %v", gotBody)
	}
	if _, ok := gotBody["loop"]; ok {
		t.Errorf("loop must only come from --inject-json, body = %v", gotBody)
	}
}

func TestGenerateSoundEffectElevenLabsRequiresKey(t *testing.T) {
	p := &EffectiveProvider{BaseURL: "https://api.elevenlabs.io", ProviderType: ElevenLabs}
	if _, err := GenerateSoundEffect(p, &SoundEffectRequest{Prompt: "rain"}); err == nil {
		t.Fatal("GenerateSoundEffect() error = nil, want missing key error")
	}
}

func TestGenerateSoundEffectStability(t *testing.T) {
	var gotPath, gotAuth string
	var gotForm map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		mediaType, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if !strings.HasPrefix(mediaType, "multipart/") {
			t.Errorf("content type = %q, want multipart", mediaType)
		}
		reader := multipart.NewReader(r.Body, params["boundary"])
		gotForm = map[string]string{}
		for {
			part, err := reader.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(part)
			gotForm[part.FormName()] = string(data)
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("fake-wav"))
	}))
	defer srv.Close()

	p := &EffectiveProvider{BaseURL: srv.URL, APIKey: "sk-key", ProviderType: Stability}
	res, err := GenerateSoundEffect(p, &SoundEffectRequest{
		Prompt:          "rain on roof",
		DurationSeconds: 4,
		Format:          "wav",
	})
	if err != nil {
		t.Fatalf("GenerateSoundEffect() error = %v", err)
	}

	if gotPath != "/v2beta/audio/stable-audio-2/text-to-audio" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer sk-key" {
		t.Errorf("authorization = %q, want Bearer sk-key", gotAuth)
	}
	want := map[string]string{
		"prompt":        "rain on roof",
		"model":         DefaultSFXModelStability,
		"duration":      "4",
		"output_format": "wav",
	}
	for key, value := range want {
		if gotForm[key] != value {
			t.Errorf("form[%s] = %q, want %q", key, gotForm[key], value)
		}
	}
	if string(res.Data) != "fake-wav" {
		t.Errorf("data = %q, want fake-wav", res.Data)
	}
}

func TestGenerateSoundEffectStabilityRequiresKey(t *testing.T) {
	p := &EffectiveProvider{BaseURL: "https://api.stability.ai", ProviderType: Stability}
	if _, err := GenerateSoundEffect(p, &SoundEffectRequest{Prompt: "rain"}); err == nil {
		t.Fatal("GenerateSoundEffect() error = nil, want missing key error")
	}
}

func TestGenerateSoundEffectVendorError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"quota exceeded"}`, http.StatusPaymentRequired)
	}))
	defer srv.Close()

	p := &EffectiveProvider{BaseURL: srv.URL, APIKey: "k", ProviderType: ElevenLabs}
	_, err := GenerateSoundEffect(p, &SoundEffectRequest{Prompt: "rain"})
	if err == nil {
		t.Fatal("GenerateSoundEffect() error = nil, want API error")
	}
	if !strings.Contains(err.Error(), "402") || !strings.Contains(err.Error(), "quota exceeded") {
		t.Errorf("error = %v, want status and vendor message", err)
	}
}

func TestFormatMapping(t *testing.T) {
	if got := elevenLabsOutputFormat("wav"); got != "wav_44100" {
		t.Errorf("elevenLabsOutputFormat(wav) = %q", got)
	}
	if got := elevenLabsOutputFormat("unknown"); got != "mp3_44100_128" {
		t.Errorf("elevenLabsOutputFormat(unknown) = %q", got)
	}
	if got := stabilityOutputFormat(""); got != "mp3" {
		t.Errorf("stabilityOutputFormat() = %q", got)
	}
	if got := stabilityOutputFormat("WAV"); got != "wav" {
		t.Errorf("stabilityOutputFormat(WAV) = %q", got)
	}
}

func TestElevenLabsInjectKeepsReservedFields(t *testing.T) {
	req := &SoundEffectRequest{
		Prompt:          "door slam",
		DurationSeconds: 1.5,
		Extra:           json.RawMessage(`{"text":"hacked","model_id":"hacked","duration_seconds":99,"style":"cinematic"}`),
	}
	payload, err := ElevenLabsSFXPayload(req, "eleven_text_to_sound_v2")
	if err != nil {
		t.Fatalf("ElevenLabsSFXPayload() error = %v", err)
	}
	if payload["text"] != "door slam" {
		t.Errorf("text = %v, want the CLI prompt", payload["text"])
	}
	if payload["model_id"] != "eleven_text_to_sound_v2" {
		t.Errorf("model_id = %v, want the CLI model", payload["model_id"])
	}
	if payload["duration_seconds"] != 1.5 {
		t.Errorf("duration_seconds = %v, want 1.5", payload["duration_seconds"])
	}
	if payload["style"] != "cinematic" {
		t.Errorf("style = %v, want the injected value", payload["style"])
	}
}

func TestStabilityInjectFields(t *testing.T) {
	req := &SoundEffectRequest{
		Prompt:          "rain",
		DurationSeconds: 5,
		Format:          "wav",
		Extra:           json.RawMessage(`{"cfg_scale":7,"seed":12}`),
	}
	fields, err := StabilitySFXFields(req, "stable-audio-2.5")
	if err != nil {
		t.Fatalf("StabilitySFXFields() error = %v", err)
	}
	if fields["cfg_scale"] != "7" || fields["seed"] != "12" {
		t.Errorf("injected fields = %v", fields)
	}
	if fields["prompt"] != "rain" || fields["output_format"] != "wav" {
		t.Errorf("reserved fields = %v", fields)
	}

	req.Extra = json.RawMessage(`{"nested":{"a":1}}`)
	if _, err := StabilitySFXFields(req, "stable-audio-2.5"); err == nil {
		t.Fatal("nested inject value must be rejected for multipart")
	}
}
