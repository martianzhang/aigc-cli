package sfx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckBundleMissing(t *testing.T) {
	if err := checkBundle(t.TempDir()); err == nil {
		t.Fatal("checkBundle() = nil, want error for an empty directory")
	}
}

func TestCheckBundleComplete(t *testing.T) {
	dir := t.TempDir()
	for _, name := range RequiredFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkBundle(dir); err != nil {
		t.Fatalf("checkBundle() error = %v", err)
	}
}

// TestEngineGenerate is an integration test. It needs the ONNX bundle and the
// ONNX Runtime shared library:
//
//	AIGC_SFX_MODEL_DIR=/path/to/stable-audio-open-small go test ./internal/audio/sfx -run TestEngineGenerate
func TestEngineGenerate(t *testing.T) {
	dir := os.Getenv("AIGC_SFX_MODEL_DIR")
	if dir == "" {
		t.Skip("set AIGC_SFX_MODEL_DIR to run the local inference test")
	}
	lib := os.Getenv("AIGC_SFX_ORT_LIB")
	if lib == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		lib = filepath.Join(home, ".config", "aigc-cli", "models", "libonnxruntime.dylib")
	}

	engine, err := NewEngine(dir, lib)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	defer engine.Close()

	audio, err := engine.Generate(Request{
		Prompt:    "short sci-fi UI confirmation beep",
		Seconds:   1,
		Seed:      42,
		Normalize: true,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if audio.Channels != 2 || audio.SampleRate != 44100 {
		t.Fatalf("format = %dch/%dHz", audio.Channels, audio.SampleRate)
	}
	if want := 44100 * 2; len(audio.Samples) != want {
		t.Fatalf("sample count = %d, want %d", len(audio.Samples), want)
	}
	if audio.Duration() != 1 {
		t.Errorf("duration = %v, want 1", audio.Duration())
	}
	if peak := maxAbs(audio.Samples); peak == 0 {
		t.Error("generated audio is silent")
	} else if peak > TargetPeak+1e-5 {
		t.Errorf("peak = %v, want <= %v", peak, TargetPeak)
	}
	if audio.Seed != 42 {
		t.Errorf("seed = %d, want 42", audio.Seed)
	}
}
