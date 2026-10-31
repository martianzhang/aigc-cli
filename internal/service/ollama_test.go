package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

	rawBody, err := OllamaGenerateBody(&types.GenerateRequest{RawJSON: []byte(raw)})
	if err != nil {
		t.Fatalf("build raw body: %v", err)
	}
	got, err := json.Marshal(rawBody)
	if err != nil {
		t.Fatalf("marshal raw body: %v", err)
	}
	if string(got) != raw {
		t.Errorf("raw --json body = %s, want %s", got, raw)
	}

	typedBody, err := OllamaGenerateBody(&types.GenerateRequest{Model: "m", Prompt: "p"})
	if err != nil {
		t.Fatalf("build typed body: %v", err)
	}
	typed, err := json.Marshal(typedBody)
	if err != nil {
		t.Fatalf("marshal typed body: %v", err)
	}
	for _, want := range []string{`"model":"m"`, `"prompt":"p"`, `"stream":false`} {
		if !strings.Contains(string(typed), want) {
			t.Errorf("typed body missing %s: %s", want, typed)
		}
	}
}

func TestOllamaGenerateImagesHonorsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"images":["aGk="]}`))
	}))
	defer srv.Close()

	_, err := OllamaGenerateImages(srv.URL, &types.GenerateRequest{Model: "m", Prompt: "p"}, t.TempDir(), 50*time.Millisecond)
	if err == nil {
		t.Fatal("OllamaGenerateImages() error = nil, want timeout error")
	}
}
