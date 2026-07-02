// Package types — OpenRouter-specific API types for image and video generation.
package types

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
