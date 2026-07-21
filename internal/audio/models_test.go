package audio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistry_hasExpectedModels(t *testing.T) {
	r := Registry()
	if len(r) == 0 {
		t.Fatal("registry is empty")
	}
	for _, m := range []string{"kokoro", "kokoro-en", "vits-zh-ll", "vits-zh-hf-eula", "vits-zh-aishell3", "vits-cantonese", "vits-ljs", "vits-vctk", "whisper-tiny", "sense-voice"} {
		if _, ok := r[m]; !ok {
			t.Errorf("expected model %q in registry", m)
		}
	}
}

func TestLookup_found(t *testing.T) {
	m, err := Lookup("vits-vctk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "vits-vctk" {
		t.Fatalf("got id %q, want %q", m.ID, "vits-vctk")
	}
	if m.Type != ModelTTS {
		t.Fatalf("vits-vctk type = %q, want %q", m.Type, ModelTTS)
	}
}

func TestLookup_notFound(t *testing.T) {
	_, err := Lookup("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown model")
	}
}

func TestListByType_tts(t *testing.T) {
	models := ListByType(ModelTTS, "")
	if len(models) == 0 {
		t.Fatal("expected at least one TTS model")
	}
	for _, m := range models {
		if m.Type != ModelTTS {
			t.Errorf("model %q has type %q, want %q", m.ID, m.Type, ModelTTS)
		}
	}
}

func TestListByType_asr(t *testing.T) {
	models := ListByType(ModelASR, "")
	if len(models) == 0 {
		t.Fatal("expected at least one ASR model")
	}
	for _, m := range models {
		if m.Type != ModelASR {
			t.Errorf("model %q has type %q, want %q", m.ID, m.Type, ModelASR)
		}
	}
}

func TestListByType_langFilter(t *testing.T) {
	models := ListByType(ModelASR, "zh")
	if len(models) == 0 {
		t.Fatal("expected ASR models supporting zh")
	}
}

func TestListInstalled_empty(t *testing.T) {
	dir := t.TempDir()
	installed, err := ListInstalled(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(installed) != 0 {
		t.Fatalf("expected 0 installed, got %d", len(installed))
	}
}

func TestListInstalled_some(t *testing.T) {
	dir := t.TempDir()
	// Create a fake installed model directory
	modelDir := filepath.Join(dir, "audio", "vits-vctk")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}

	installed, err := ListInstalled(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(installed) != 1 {
		t.Fatalf("expected 1 installed, got %d", len(installed))
	}
	if installed[0].ID != "vits-vctk" {
		t.Fatalf("got %q, want %q", installed[0].ID, "vits-vctk")
	}
}

func TestModelInfo_fields(t *testing.T) {
	for id, m := range Registry() {
		if m.ID == "" {
			t.Errorf("model %q has empty ID", id)
		}
		if m.Name == "" {
			t.Errorf("model %q has empty Name", id)
		}
		if m.Type != ModelASR && m.Type != ModelTTS {
			t.Errorf("model %q has invalid Type %q", id, m.Type)
		}
		if len(m.Files) == 0 {
			t.Errorf("model %q has no files", id)
		}
		for _, f := range m.Files {
			if f.URL == "" {
				t.Errorf("model %q has a file with empty URL", id)
			}
			if f.Path == "" {
				t.Errorf("model %q has a file with empty Path", id)
			}
		}
	}
}
