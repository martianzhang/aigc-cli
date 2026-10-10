package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SystemOneEndpoint builds the /v1/systemone URL from a provider base_url.
// Endpoint paths differ per provider:
//   - a trailing "/" then a trailing "/v1" are trimmed
//   - OpenRouter serves from .../api (not /api/v1) — the path is ensured to
//     end in "/api"
//   - "/v1/systemone" is always appended
//
// Verified: api.typesafe.ai, openrouter.ai/api[/v1], localhost:11434[/v1],
// api.llmgateway.io/v1 and {proxy}/typesafe all resolve correctly.
func SystemOneEndpoint(baseURL string) string {
	u := strings.TrimRight(baseURL, "/")
	u = strings.TrimSuffix(u, "/v1")
	if IsOpenRouter(u) && !strings.HasSuffix(u, "/api") {
		u += "/api"
	}
	return u + "/v1/systemone"
}

// DecisionRequest is the wire body for POST /v1/systemone (TypeSafe Jev
// protocol): a state to evaluate plus named typed questions.
type DecisionRequest struct {
	Model string          `json:"model,omitempty"`
	State json.RawMessage `json:"state"`
	// Images are raw base64 images (no data: prefix), shared by all
	// questions and scored jointly with the text state. PNG/JPEG/WebP;
	// Clef / Clef-Flash only (Ollama >= 0.35.1).
	Images []string `json:"images,omitempty"`
	// Audio and Videos are base64 data URLs (e.g.
	// data:audio/wav;base64,...), shared by all questions and scored jointly
	// with the text state. Footage is sampled at 2 fps server-side, with its
	// soundtrack when present. Clef-Omni / Clef family only.
	Audio     []string                   `json:"audio,omitempty"`
	Videos    []string                   `json:"videos,omitempty"`
	Questions map[string]json.RawMessage `json:"questions"`
	KeepAlive string                     `json:"keep_alive,omitempty"`
}

// DecisionUsage reports token usage for a decision call.
type DecisionUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// DecisionResponse is the wire answer from POST /v1/systemone: one answer per
// question, keyed by the same ids the request used.
type DecisionResponse struct {
	Model   string                     `json:"model"`
	Answers map[string]json.RawMessage `json:"answers"`
	Usage   DecisionUsage              `json:"usage"`
}

// Decide POSTs a decision request to the provider's /v1/systemone endpoint.
// The Authorization header is only set with a non-empty API key (Ollama
// serves decision models without one).
func Decide(p *EffectiveProvider, req *DecisionRequest, timeout time.Duration) (*DecisionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", SystemOneEndpoint(p.BaseURL), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	SetAttribution(httpReq, p.BaseURL)

	resp, err := httpClient(p, timeout).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result DecisionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}
