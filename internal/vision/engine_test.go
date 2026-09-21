package vision

import (
	"os"
	"path/filepath"
	"testing"
)

// ── DefaultModelsDir ──

func TestDefaultModelsDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("UserHomeDir unavailable: %v", err)
	}

	want := filepath.Join(home, ".config", "aigc-cli", "models")
	got := DefaultModelsDir()

	if got != want {
		t.Errorf("DefaultModelsDir() = %q, want %q", got, want)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("DefaultModelsDir() = %q, want absolute path", got)
	}
}

func TestDefaultModelsDirUsesUserHomeDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("UserHomeDir unavailable: %v", err)
	}

	got := DefaultModelsDir()
	rel, err := filepath.Rel(home, got)
	if err != nil {
		t.Fatalf("Rel(%q, %q) error: %v", home, got, err)
	}
	want := filepath.Join(".config", "aigc-cli", "models")
	if rel != want {
		t.Errorf("DefaultModelsDir() relative to home = %q, want %q", rel, want)
	}
}

// ── VariantDir ──

func TestVariantDir(t *testing.T) {
	tests := []struct {
		name      string
		modelsDir string
		variant   string
		want      string
	}{
		{"absolute", "/models", "base-int8", "/models/vision/base-int8"},
		{"relative", "models", "base-int8", "models/vision/base-int8"},
		{"trailing slash", "/models/", "base-int8", "/models/vision/base-int8"},
		{"empty variant", "/models", "", "/models/vision"},
		{"empty modelsDir", "", "base-int8", "vision/base-int8"},
		{"nested variant", "/m", "a/b", "/m/vision/a/b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VariantDir(tt.modelsDir, tt.variant)
			if got != tt.want {
				t.Errorf("VariantDir(%q, %q) = %q, want %q", tt.modelsDir, tt.variant, got, tt.want)
			}
		})
	}
}

// ── modelPath ──

func TestModelPath(t *testing.T) {
	tests := []struct {
		name      string
		modelsDir string
		variant   string
		filename  string
		want      string
	}{
		{"absolute", "/models", "base-int8", "vision_encoder.onnx", "/models/vision/base-int8/vision_encoder.onnx"},
		{"relative", "models", "base-int8", "embed_tokens.onnx", "models/vision/base-int8/embed_tokens.onnx"},
		{"trailing slash dir", "/models/", "base-int8", "encoder_model.onnx", "/models/vision/base-int8/encoder_model.onnx"},
		{"empty filename", "/models", "base-int8", "", "/models/vision/base-int8"},
		{"empty variant", "/models", "", "decoder_model.onnx", "/models/vision/decoder_model.onnx"},
		{"empty all", "", "", "", "vision"},
		{"nested filename", "/m", "v", "sub/file.onnx", "/m/vision/v/sub/file.onnx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := modelPath(tt.modelsDir, tt.variant, tt.filename)
			if got != tt.want {
				t.Errorf("modelPath(%q, %q, %q) = %q, want %q",
					tt.modelsDir, tt.variant, tt.filename, got, tt.want)
			}
		})
	}
}

func TestModelPathIsWithinVariantDir(t *testing.T) {
	modelsDir := t.TempDir()
	got := modelPath(modelsDir, DefaultModelVariant, "decoder_model.onnx")

	variantDir := VariantDir(modelsDir, DefaultModelVariant)
	rel, err := filepath.Rel(variantDir, got)
	if err != nil {
		t.Fatalf("Rel error: %v", err)
	}
	if rel != "decoder_model.onnx" {
		t.Errorf("modelPath not directly inside VariantDir: rel = %q", rel)
	}

	if dir := filepath.Dir(got); dir != variantDir {
		t.Errorf("filepath.Dir(modelPath(...)) = %q, want VariantDir %q", dir, variantDir)
	}
}

// ── DefaultModelVariant ──

func TestDefaultModelVariant(t *testing.T) {
	if DefaultModelVariant != "base-int8" {
		t.Errorf("DefaultModelVariant = %q, want %q", DefaultModelVariant, "base-int8")
	}
}

func TestDefaultModelVariantIsResolvable(t *testing.T) {
	v, err := ResolveModelVariant(DefaultModelVariant)
	if err != nil {
		t.Fatalf("ResolveModelVariant(%q) error: %v", DefaultModelVariant, err)
	}
	if v.ID != DefaultModelVariant {
		t.Errorf("resolved variant ID = %q, want %q", v.ID, DefaultModelVariant)
	}
}

func TestDefaultModelVariantHasModelFiles(t *testing.T) {
	files, err := ModelFiles(DefaultModelVariant)
	if err != nil {
		t.Fatalf("ModelFiles(%q) error: %v", DefaultModelVariant, err)
	}
	if len(files) == 0 {
		t.Fatalf("ModelFiles(%q) returned no files", DefaultModelVariant)
	}
}

// ── EngineConfig ──

func TestEngineConfigFields(t *testing.T) {
	cfg := EngineConfig{
		ModelsDir:         "/models",
		Variant:           "base-int8",
		LibPath:           "/lib/onnxruntime.so",
		Tokenizer:         &Tokenizer{},
		MaxTokens:         128,
		Temperature:       0.7,
		TopK:              50,
		RepetitionPenalty: 1.2,
	}

	if cfg.ModelsDir != "/models" {
		t.Errorf("ModelsDir = %q", cfg.ModelsDir)
	}
	if cfg.Variant != "base-int8" {
		t.Errorf("Variant = %q", cfg.Variant)
	}
	if cfg.LibPath != "/lib/onnxruntime.so" {
		t.Errorf("LibPath = %q", cfg.LibPath)
	}
	if cfg.Tokenizer == nil {
		t.Error("Tokenizer should be assignable")
	}
	if cfg.MaxTokens != 128 {
		t.Errorf("MaxTokens = %d", cfg.MaxTokens)
	}
	if cfg.Temperature != 0.7 {
		t.Errorf("Temperature = %v", cfg.Temperature)
	}
	if cfg.TopK != 50 {
		t.Errorf("TopK = %d", cfg.TopK)
	}
	if cfg.RepetitionPenalty != 1.2 {
		t.Errorf("RepetitionPenalty = %v", cfg.RepetitionPenalty)
	}
}

func TestEngineConfigZeroValue(t *testing.T) {
	var cfg EngineConfig
	if cfg.ModelsDir != "" || cfg.Variant != "" || cfg.LibPath != "" {
		t.Error("zero value string fields should be empty")
	}
	if cfg.Tokenizer != nil {
		t.Error("zero value Tokenizer should be nil")
	}
	if cfg.MaxTokens != 0 || cfg.TopK != 0 {
		t.Error("zero value int fields should be 0")
	}
	if cfg.Temperature != 0 || cfg.RepetitionPenalty != 0 {
		t.Error("zero value float fields should be 0")
	}
}

// ── ModelFiles ──

func TestModelFiles(t *testing.T) {
	files, err := ModelFiles(DefaultModelVariant)
	if err != nil {
		t.Fatalf("ModelFiles error: %v", err)
	}

	wantNames := map[string]bool{
		"vision_encoder.onnx": false,
		"encoder_model.onnx":  false,
		"decoder_model.onnx":  false,
		"embed_tokens.onnx":   false,
		"vocab.json":          false,
		"merges.txt":          false,
	}

	for _, f := range files {
		if _, ok := wantNames[f.Filename]; !ok {
			t.Errorf("unexpected model file %q", f.Filename)
		} else {
			wantNames[f.Filename] = true
		}
		if f.URL == "" {
			t.Errorf("model file %q has empty URL", f.Filename)
		}
	}
	for name, found := range wantNames {
		if !found {
			t.Errorf("missing expected model file %q", name)
		}
	}
}

func TestModelFilesUnknownVariant(t *testing.T) {
	_, err := ModelFiles("does-not-exist")
	if err == nil {
		t.Error("ModelFiles(unknown) = nil error, want error")
	}
}

// ── ResolveModelVariant ──

func TestResolveModelVariant(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{"base-int8", false},
		{"", true},
		{"unknown", true},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			v, err := ResolveModelVariant(tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveModelVariant(%q) = nil error, want error", tt.id)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveModelVariant(%q) error: %v", tt.id, err)
			}
			if v.ID != tt.id {
				t.Errorf("resolved ID = %q, want %q", v.ID, tt.id)
			}
		})
	}
}
