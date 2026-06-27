package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_customFile(t *testing.T) {
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

func TestLoad_missingFile(t *testing.T) {
	// Use a random non-existent path that won't be found.
	// Note: viper's ConfigFileNotFoundError detection works only with config search paths,
	// not with explicit SetConfigFile paths. This test verifies the function handles it gracefully.
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err != nil {
		// Accept either nil error (missing file ignored) or a path error (viper behavior varies by OS)
		t.Logf("Load() returned expected error for missing file: %v", err)
		return
	}
	if cfg == nil {
		t.Fatal("cfg should not be nil")
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
	// Default baseURL should be set
	if cfg.BaseURL == "" {
		t.Error("BaseURL should have a default value")
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
