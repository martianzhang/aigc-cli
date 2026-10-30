package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_customFile(t *testing.T) {
	// Load binds http_proxy to HTTP_PROXY/HTTPS_PROXY, and viper env wins over
	// the config file. Clear them so the file value is observed regardless of
	// the environment the test runs in (viper ignores empty env vars).
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test.yaml")
	content := []byte(`
api_key: "sk-test"
base_url: "https://openrouter.ai/api/v1"
http_proxy: "http://127.0.0.1:7890"
verbose: true
output_dir: "./downloads"
defaults:
  image:
    model: "openai/gpt-image-2"
    size: "1024x1024"
    quality: "low"
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.APIKey != "sk-test" {
		t.Errorf("APIKey = %q", cfg.APIKey)
	}
	if cfg.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.HTTPProxy != "http://127.0.0.1:7890" {
		t.Errorf("HTTPProxy = %q", cfg.HTTPProxy)
	}
	if !cfg.Verbose {
		t.Error("Verbose should be true")
	}
	if cfg.OutputDir != "./downloads" {
		t.Errorf("OutputDir = %q", cfg.OutputDir)
	}
	if cfg.Defaults == nil {
		t.Fatal("Defaults should not be nil")
	}
	if cfg.Defaults.Image == nil {
		t.Fatal("Defaults.Image should not be nil")
	}
	if cfg.Defaults.Image.Model != "openai/gpt-image-2" {
		t.Errorf("Image.Model = %q", cfg.Defaults.Image.Model)
	}
	if cfg.Defaults.Image.Quality != "low" {
		t.Errorf("Image.Quality = %q", cfg.Defaults.Image.Quality)
	}
}

func TestLoad_zdr(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "zdr.yaml")
	if err := os.WriteFile(cfgPath, []byte("zdr: true\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.ZDR {
		t.Error("ZDR should be true")
	}

	offPath := filepath.Join(dir, "off.yaml")
	if err := os.WriteFile(offPath, []byte("zdr: false\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	off, err := Load(offPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if off.ZDR {
		t.Error("ZDR should be false")
	}
}

func TestLoad_providerZDR(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "providers.yaml")
	content := []byte(`
providers:
  openrouter:
    base_url: "https://openrouter.ai/api/v1"
    zdr: true
  plain:
    base_url: "https://api.openai.com/v1"
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	or := cfg.Providers["openrouter"]
	if or == nil || or.ZDR == nil || !*or.ZDR {
		t.Errorf("openrouter zdr = %v, want true", or)
	}
	plain := cfg.Providers["plain"]
	if plain == nil || plain.ZDR != nil {
		t.Errorf("plain zdr = %v, want unset", plain)
	}
}

func TestLoad_missingFile(t *testing.T) {
	// A missing config file must not be an error: Load falls back to env vars and
	// code defaults, so OPENAI_API_KEY / OPENAI_BASE_URL are never dropped when an
	// explicitly passed --config path does not exist.
	t.Setenv("OPENAI_API_KEY", "sk-from-env")
	t.Setenv("OPENAI_BASE_URL", "https://env.example/v1")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")

	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("Load(missing) error = %v, want nil", err)
	}
	if cfg == nil {
		t.Fatal("cfg should not be nil")
	}
	if cfg.APIKey != "sk-from-env" {
		t.Errorf("APIKey = %q, want env value", cfg.APIKey)
	}
	if cfg.BaseURL != "https://env.example/v1" {
		t.Errorf("BaseURL = %q, want env value", cfg.BaseURL)
	}
}

func TestLoad_missingDefaultFileUsesDefaults(t *testing.T) {
	// Point HOME at an empty dir so the default config lookup finds nothing.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error = %v, want nil", err)
	}
	if cfg == nil {
		t.Fatal("cfg should not be nil")
	}
	if cfg.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty (no built-in provider default)", cfg.BaseURL)
	}
}

func TestLoad_emptyFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "empty.yaml")
	os.WriteFile(cfgPath, []byte{}, 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("cfg should not be nil")
	}
}

func TestLoad_minimalConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "minimal.yaml")
	os.WriteFile(cfgPath, []byte(`api_key: "sk-minimal"`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.APIKey != "sk-minimal" {
		t.Errorf("APIKey = %q", cfg.APIKey)
	}
	// No built-in base URL: an unset value stays empty.
	if cfg.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty", cfg.BaseURL)
	}
}

func TestLoad_videoDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "video.yaml")
	os.WriteFile(cfgPath, []byte(`
api_key: "sk-test"
defaults:
  video:
    model: "google/veo-3.1"
    size: "16:9"
    resolution: "720p"
    duration: 8
`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Defaults.Video.Model != "google/veo-3.1" {
		t.Errorf("Video.Model = %q", cfg.Defaults.Video.Model)
	}
	if cfg.Defaults.Video.Size != "16:9" {
		t.Errorf("Video.Size = %q", cfg.Defaults.Video.Size)
	}
	if cfg.Defaults.Video.Resolution != "720p" {
		t.Errorf("Video.Resolution = %q", cfg.Defaults.Video.Resolution)
	}
	if cfg.Defaults.Video.Duration == nil || *cfg.Defaults.Video.Duration != 8 {
		t.Errorf("Video.Duration = %v", cfg.Defaults.Video.Duration)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := LoadDefaults("")
	if err != nil {
		t.Fatalf("LoadDefaults() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("cfg should not be nil")
	}
}

func TestLoad_audioDefaults(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")

	path := filepath.Join(t.TempDir(), "audio.yaml")
	content := []byte(`
defaults:
  audio:
    speak:
      provider: "nested-tts"
      model: "nested-model"
      voice: "alloy"
      format: "mp3"
    transcribe:
      model: "whisper-large-v3"
      language: "zh"
    sfx:
      provider: "elevenlabs"
      model: "eleven_text_to_sound_v2"
      format: "wav"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Defaults == nil || cfg.Defaults.Audio == nil {
		t.Fatal("defaults.audio was not parsed")
	}
	audio := cfg.Defaults.Audio
	if got := audio.SpeakModel(); got != "nested-model" {
		t.Errorf("SpeakModel() = %q, want nested-model", got)
	}
	if got := audio.SpeakProvider(); got != "nested-tts" {
		t.Errorf("SpeakProvider() = %q, want nested-tts", got)
	}
	if got := audio.SpeakVoice(); got != "alloy" {
		t.Errorf("SpeakVoice() = %q, want alloy", got)
	}
	if got := audio.SpeakFormat(); got != "mp3" {
		t.Errorf("SpeakFormat() = %q, want mp3", got)
	}
	if got := audio.TranscribeModel(); got != "whisper-large-v3" {
		t.Errorf("TranscribeModel() = %q, want whisper-large-v3", got)
	}
	if got := audio.TranscribeProvider(); got != "" {
		t.Errorf("TranscribeProvider() = %q, want empty (global fallback)", got)
	}
	if got := audio.TranscribeLanguage(); got != "zh" {
		t.Errorf("TranscribeLanguage() = %q, want zh", got)
	}
	if got := audio.SFXProvider(); got != "elevenlabs" {
		t.Errorf("SFXProvider() = %q, want elevenlabs", got)
	}
	if got := audio.SFXFormat(); got != "wav" {
		t.Errorf("SFXFormat() = %q, want wav", got)
	}
}
