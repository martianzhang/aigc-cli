package knowledge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// EmbedConfig configures an HTTP embedding backend. The endpoint is the
// OpenAI-compatible /v1/embeddings, which both local Ollama and online vendors
// serve, so one backend covers every remote provider.
type EmbedConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	// Timeout is the per-request timeout; 0 uses defaultEmbedTimeout.
	Timeout time.Duration
}

// defaultEmbedTimeout bounds one embedding request. A large batch can take a
// while on a slow local model, so the default is generous.
const defaultEmbedTimeout = 180 * time.Second

// APIEmbedder calls an OpenAI-compatible /v1/embeddings endpoint.
type APIEmbedder struct {
	cfg    EmbedConfig
	url    string
	client *http.Client
	dim    int
}

// NewAPIEmbedder builds an HTTP embedder for cfg.BaseURL + cfg.Model.
func NewAPIEmbedder(cfg EmbedConfig) *APIEmbedder {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultEmbedTimeout
	}
	client := &http.Client{Timeout: timeout}
	if t := http.DefaultClient.Transport; t != nil {
		client.Transport = t
	}
	return &APIEmbedder{cfg: cfg, url: embeddingsURL(cfg.BaseURL), client: client}
}

// embeddingsURL normalizes a base URL to its /v1/embeddings endpoint, accepting
// a bare host, a trailing slash, or an existing /v1 suffix.
func embeddingsURL(baseURL string) string {
	u := strings.TrimRight(baseURL, "/")
	u = strings.TrimSuffix(u, "/v1")
	return u + "/v1/embeddings"
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Embed embeds a single text.
func (e *APIEmbedder) Embed(text string) (Embedding, error) {
	out, err := e.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("embedding API returned no vectors")
	}
	return out[0], nil
}

// EmbedBatch embeds all texts in one round trip.
func (e *APIEmbedder) EmbedBatch(texts []string) ([]Embedding, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(embedRequest{Model: e.cfg.Model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, e.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var parsed embedResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("embedding API error: %s", parsed.Error.Message)
	}
	out := make([]Embedding, len(parsed.Data))
	for i, d := range parsed.Data {
		out[i] = Embedding(d.Embedding)
	}
	if len(out) > 0 {
		e.dim = len(out[0])
	}
	return out, nil
}

// Dim returns the embedding dimension observed from the last response (0 before
// the first call).
func (e *APIEmbedder) Dim() int { return e.dim }

// Name identifies this backend for the stored embedder fingerprint.
func (e *APIEmbedder) Name() string { return "api:" + e.cfg.Model }
