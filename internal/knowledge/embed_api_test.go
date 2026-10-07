package knowledge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbeddingsURL(t *testing.T) {
	tests := map[string]string{
		"http://localhost:11434":        "http://localhost:11434/v1/embeddings",
		"http://localhost:11434/":       "http://localhost:11434/v1/embeddings",
		"http://localhost:11434/v1":     "http://localhost:11434/v1/embeddings",
		"https://api.openai.com/v1/":    "https://api.openai.com/v1/embeddings",
		"https://vendor.example/api/v1": "https://vendor.example/api/v1/embeddings",
	}
	for in, want := range tests {
		if got := embeddingsURL(in); got != want {
			t.Errorf("embeddingsURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAPIEmbedderBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("path = %s, want /v1/embeddings", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("Authorization = %q, want Bearer k", got)
		}
		var req embedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		resp := embedResponse{}
		for range req.Input {
			resp.Data = append(resp.Data, struct {
				Embedding []float32 `json:"embedding"`
			}{Embedding: []float32{0.1, 0.2, 0.3}})
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewAPIEmbedder(EmbedConfig{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	if e.Name() != "api:m" {
		t.Errorf("Name() = %q, want api:m", e.Name())
	}
	out, err := e.EmbedBatch([]string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(out) != 2 || len(out[0]) != 3 {
		t.Fatalf("EmbedBatch returned %d vectors, first dim %d; want 2 of dim 3", len(out), len(out[0]))
	}
	if e.Dim() != 3 {
		t.Errorf("Dim() = %d, want 3", e.Dim())
	}
	single, err := e.Embed("a")
	if err != nil || len(single) != 3 {
		t.Errorf("Embed() = (%v, %v), want one dim-3 vector", single, err)
	}
}

func TestAPIEmbedderHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()

	e := NewAPIEmbedder(EmbedConfig{BaseURL: srv.URL, Model: "m"})
	if _, err := e.Embed("x"); err == nil {
		t.Error("Embed() on HTTP 401 = nil error, want error")
	}
}

func TestEmbedderFingerprint(t *testing.T) {
	if got := embedderFingerprint(nil, 384); got != "hash:384" {
		t.Errorf("nil fingerprint = %q, want hash:384", got)
	}
	if got := embedderFingerprint(NewHashEmbedder(768), 768); got != "hash:768" {
		t.Errorf("hash fingerprint = %q, want hash:768", got)
	}
	api := NewAPIEmbedder(EmbedConfig{Model: "embeddinggemma-2"})
	if got := embedderFingerprint(api, 768); got != "api:embeddinggemma-2" {
		t.Errorf("api fingerprint = %q, want api:embeddinggemma-2", got)
	}
}

func TestNewEmbedderSelection(t *testing.T) {
	fallback, err := NewEmbedder(nil, t.TempDir(), "")
	if err != nil {
		t.Fatalf("NewEmbedder(nil) error = %v", err)
	}
	if fallback.Dim() != 384 {
		t.Errorf("fallback Dim() = %d, want 384", fallback.Dim())
	}

	api, err := NewEmbedder(&EmbedConfig{BaseURL: "http://localhost:11434", Model: "embeddinggemma-2"}, "", "")
	if err != nil {
		t.Fatalf("NewEmbedder(cfg) error = %v", err)
	}
	if _, ok := api.(*APIEmbedder); !ok {
		t.Errorf("NewEmbedder(cfg) = %T, want *APIEmbedder", api)
	}
}
