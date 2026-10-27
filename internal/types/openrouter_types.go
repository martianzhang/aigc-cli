// Package types — OpenRouter-specific API types for image and video generation.
package types

import (
	"encoding/json"
	"strconv"
	"strings"
)

// OpenRouterUsage holds token and cost information.
type OpenRouterUsage struct {
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	TotalCost    float64 `json:"total_cost,omitempty"`
}

// ---------------------------------------------------------------------------
// OpenRouter Video — dedicated async API (POST /api/v1/videos)
// ---------------------------------------------------------------------------

// OpenRouterVideoRequest is the request body for OpenRouter video generation.
type OpenRouterVideoRequest struct {
	Model         string                 `json:"model"`
	Prompt        string                 `json:"prompt"`
	Duration      *int                   `json:"duration,omitempty"`
	Resolution    string                 `json:"resolution,omitempty"`
	AspectRatio   string                 `json:"aspect_ratio,omitempty"`
	Size          string                 `json:"size,omitempty"`
	GenerateAudio *bool                  `json:"generate_audio,omitempty"`
	Seed          *int                   `json:"seed,omitempty"`
	FrameImages   []OpenRouterFrameImage `json:"frame_images,omitempty"`
	// RawJSON is a verbatim --json body carried over from the video command.
	RawJSON json.RawMessage `json:"-" yaml:"-"`
	// InjectJSON is a --inject-json snippet carried over from the video command.
	InjectJSON json.RawMessage `json:"-" yaml:"-"`
}

// MarshalJSON emits RawJSON unchanged when set, otherwise the typed fields,
// then layers InjectJSON on top.
func (r OpenRouterVideoRequest) MarshalJSON() ([]byte, error) {
	type typed OpenRouterVideoRequest
	return marshalVerbatim(r.RawJSON, r.InjectJSON, typed(r))
}

// OpenRouterFrameImage represents an image used as first/last frame or reference.
type OpenRouterFrameImage struct {
	Type     string `json:"type"` // "image_url"
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url"`
	FrameType string `json:"frame_type,omitempty"` // "first_frame" or "last_frame"
}

// OpenRouterVideoSubmitResponse is returned immediately after submitting a video job.
type OpenRouterVideoSubmitResponse struct {
	ID         string `json:"id"`
	PollingURL string `json:"polling_url"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

// OpenRouterVideoStatusResponse is returned when polling a video job.
type OpenRouterVideoStatusResponse struct {
	ID           string           `json:"id"`
	Status       string           `json:"status"` // pending | running | completed | failed | cancelled | expired
	UnsignedURLs []string         `json:"unsigned_urls,omitempty"`
	Error        string           `json:"error,omitempty"`
	Usage        *OpenRouterUsage `json:"usage,omitempty"`
}

// ---------------------------------------------------------------------------
// OpenRouter Model Discovery — GET /api/v1/images/models, /api/v1/videos/models
// ---------------------------------------------------------------------------

// OpenRouterMediaModelList is the response from OpenRouter's media model discovery endpoints.
type OpenRouterMediaModelList struct {
	Data []OpenRouterMediaModel `json:"data"`
}

// OpenRouterMediaModel is a single model entry from OpenRouter's model discovery.
type OpenRouterMediaModel struct {
	ID                  string                               `json:"id"`
	Name                string                               `json:"name"`
	Description         string                               `json:"description,omitempty"`
	Created             int64                                `json:"created,omitempty"`
	Architecture        *OpenRouterModelArchitecture         `json:"architecture,omitempty"`
	SupportedParameters map[string]OpenRouterParamDescriptor `json:"supported_parameters,omitempty"`
	SupportsStreaming   bool                                 `json:"supports_streaming"`
	Endpoints           string                               `json:"endpoints,omitempty"`
}

// OpenRouterModelArchitecture describes the input/output modalities.
type OpenRouterModelArchitecture struct {
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
}

// OpenRouterParamDescriptor describes a single supported parameter.
// It can be one of: enum, range, boolean.
type OpenRouterParamDescriptor struct {
	Type   string   `json:"type"`             // "enum", "range", "boolean"
	Values []string `json:"values,omitempty"` // enum values
	Min    *int     `json:"min,omitempty"`    // range min
	Max    *int     `json:"max,omitempty"`    // range max
}

// ---------------------------------------------------------------------------
// OpenRouter Standard Models API — GET /api/v1/models?output_modalities=...
// ---------------------------------------------------------------------------

// OpenRouterModelList is the standard Models API response. It covers every
// output modality: text, image, video, audio, speech, transcription,
// embeddings, rerank and decisions.
type OpenRouterModelList struct {
	Data       []OpenRouterModel `json:"data"`
	TotalCount int               `json:"total_count,omitempty"`
}

// OpenRouterModel is one entry from the standard Models API. Unlike
// OpenRouterMediaModel (the dedicated image/video endpoints), this shape is
// shared by every modality and carries pricing, voices and expiry.
type OpenRouterModel struct {
	ID                  string                       `json:"id"`
	CanonicalSlug       string                       `json:"canonical_slug,omitempty"`
	Name                string                       `json:"name"`
	Description         string                       `json:"description,omitempty"`
	Created             int64                        `json:"created,omitempty"`
	ContextLength       int                          `json:"context_length,omitempty"`
	Architecture        *OpenRouterModelArchitecture `json:"architecture,omitempty"`
	SupportedParameters []string                     `json:"supported_parameters,omitempty"`
	DefaultParameters   map[string]any               `json:"default_parameters,omitempty"`
	SupportedVoices     []string                     `json:"supported_voices,omitempty"`
	Pricing             OpenRouterPricing            `json:"pricing,omitempty"`
	ExpirationDate      *string                      `json:"expiration_date,omitempty"`
}

// IsFree reports whether the entry is an explicit ":free" catalog variant.
// Pricing that merely lists zeros is not treated as free here (some media
// models, e.g. music, report zeros per token but charge per generation).
func (m OpenRouterModel) IsFree() bool {
	return strings.HasSuffix(m.ID, ":free")
}

// OpenRouterPricing holds OpenRouter's per-unit prices. Values are JSON
// strings for regular fields (and arrays/objects for special ones such as
// "overrides"), so the map stays loosely typed and is read through Price.
type OpenRouterPricing map[string]any

// Price returns the numeric value of one pricing field. JSON strings and
// numbers are accepted; missing, array or object fields report false.
func (p OpenRouterPricing) Price(key string) (float64, bool) {
	raw, ok := p[key]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case string:
		f, err := strconv.ParseFloat(v, 64)
		return f, err == nil
	case float64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	}
	return 0, false
}

// IsFree reports whether every numeric price is zero. A missing or
// non-numeric pricing map is not treated as free.
func (p OpenRouterPricing) IsFree() bool {
	priced := false
	for key := range p {
		f, ok := p.Price(key)
		if !ok {
			continue
		}
		if f > 0 {
			return false
		}
		priced = true
	}
	return priced
}
