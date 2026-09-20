package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestOllamaGenerateURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"http://localhost:11434/v1", "http://localhost:11434/api/generate"},
		{"http://localhost:11434", "http://localhost:11434/api/generate"},
		{"https://ollama.example.com/v1/", "https://ollama.example.com/api/generate"},
	}
	for _, tc := range tests {
		if got := OllamaGenerateURL(tc.in); got != tc.want {
			t.Errorf("OllamaGenerateURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestOllamaGenerateBody(t *testing.T) {
	const raw = `{"model":"m","prompt":"p","loras":"u/r","seed":1}`

	got, err := json.Marshal(OllamaGenerateBody(&types.GenerateRequest{RawJSON: []byte(raw)}))
	if err != nil {
		t.Fatalf("marshal raw body: %v", err)
	}
	if string(got) != raw {
		t.Errorf("raw --json body = %s, want %s", got, raw)
	}

	typed, err := json.Marshal(OllamaGenerateBody(&types.GenerateRequest{Model: "m", Prompt: "p"}))
	if err != nil {
		t.Fatalf("marshal typed body: %v", err)
	}
	for _, want := range []string{`"model":"m"`, `"prompt":"p"`, `"stream":false`} {
		if !strings.Contains(string(typed), want) {
			t.Errorf("typed body missing %s: %s", want, typed)
		}
	}
}
