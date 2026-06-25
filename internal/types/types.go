// Package types defines request/response data structures for the APIMart API.
package types

import (
	"encoding/json"
	"strconv"
)

// ErrCode handles API error codes that can be int or string in the response.
type ErrCode int

func (c *ErrCode) UnmarshalJSON(data []byte) error {
	// Try int first
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*c = ErrCode(i)
		return nil
	}
	// Try string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if n, err := strconv.Atoi(s); err == nil {
			*c = ErrCode(n)
			return nil
		}
	}
	// Fallback: don't block the whole parse for a code format issue
	return nil
}

// GenerateRequest is the request body for POST /v1/images/generations.
// Contains both OpenAI and APIMart fields; unused fields are omitted when marshalling.
type GenerateRequest struct {
	Model             string   `json:"model" yaml:"model"`
	Prompt            string   `json:"prompt" yaml:"prompt"`
	Size              string   `json:"size,omitempty" yaml:"size,omitempty"`
	Resolution        string   `json:"resolution,omitempty" yaml:"resolution,omitempty"`
	Quality           string   `json:"quality,omitempty" yaml:"quality,omitempty"`
	Background        string   `json:"background,omitempty" yaml:"background,omitempty"`
	Moderation        string   `json:"moderation,omitempty" yaml:"moderation,omitempty"`
	OutputFormat      string   `json:"output_format,omitempty" yaml:"output_format,omitempty"`
	OutputCompression *int     `json:"output_compression,omitempty" yaml:"output_compression,omitempty"`
	N                 *int     `json:"n,omitempty" yaml:"n,omitempty"`
	ImageURLs         []string `json:"image_urls,omitempty" yaml:"image_urls,omitempty"`
	MaskURL           string   `json:"mask_url,omitempty" yaml:"mask_url,omitempty"`
	// OpenAI-specific fields
	Style          string `json:"style,omitempty" yaml:"style,omitempty"`
	ResponseFormat string `json:"response_format,omitempty" yaml:"response_format,omitempty"`
}

// OpenAIImageResponse is the synchronous response from OpenAI/OpenRouter-compatible
// image generation endpoints.
type OpenAIImageResponse struct {
	Created int64             `json:"created"`
	Data    []OpenAIImageData `json:"data"`
}

// OpenAIImageData represents a single generated image in sync mode.
type OpenAIImageData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// GenerateResponse is the response from POST /v1/images/generations.
type GenerateResponse struct {
	Code int              `json:"code"`
	Data []TaskSubmission `json:"data"`
}

// TaskSubmission represents a submitted generation task.
type TaskSubmission struct {
	Status string `json:"status"`
	TaskID string `json:"task_id"`
}

// TaskResponse is the response from GET /v1/tasks/{task_id}.
type TaskResponse struct {
	Code int       `json:"code"`
	Data *TaskData `json:"data"`
}

// TaskData contains the full task information from GET /v1/tasks/{task_id}.
type TaskData struct {
	ID            string      `json:"id"`
	Status        string      `json:"status"`
	Progress      int         `json:"progress"`
	Cost          float64     `json:"cost,omitempty"`
	CreditsCost   float64     `json:"credits_cost,omitempty"`
	ActualTime    int         `json:"actual_time,omitempty"`
	EstimatedTime int         `json:"estimated_time,omitempty"`
	Created       int64       `json:"created,omitempty"`
	Completed     int64       `json:"completed,omitempty"`
	Result        *TaskResult `json:"result,omitempty"`
	Error         *TaskError  `json:"error,omitempty"`
}

// TaskResult contains the generated images or videos.
type TaskResult struct {
	Images []ImageResult `json:"images,omitempty"`
	Videos []VideoResult `json:"videos,omitempty"`
}

// ImageResult contains URLs for a generated image.
type ImageResult struct {
	URL       []string `json:"url"`
	ExpiresAt int64    `json:"expires_at"`
}

// VideoResult contains URLs for a generated video.
type VideoResult struct {
	URL       []string `json:"url"`
	ExpiresAt int64    `json:"expires_at"`
}

// TaskError contains error details for a failed task.
type TaskError struct {
	Code    ErrCode `json:"code"`
	Message string  `json:"message"`
	Type    string  `json:"type"`
}

// TokenBalanceResponse is the response from GET /v1/balance.
type TokenBalanceResponse struct {
	Success        bool    `json:"success"`
	Message        string  `json:"message,omitempty"`
	RemainBalance  float64 `json:"remain_balance"`
	RemainCredits  float64 `json:"remain_credits"`
	UsedBalance    float64 `json:"used_balance"`
	UsedCredits    float64 `json:"used_credits"`
	UnlimitedQuota bool    `json:"unlimited_quota"`
}

// UserBalanceResponse is the response from GET /v1/user/balance.
type UserBalanceResponse struct {
	Success       bool    `json:"success"`
	Message       string  `json:"message,omitempty"`
	RemainBalance float64 `json:"remain_balance"`
	RemainCredits float64 `json:"remain_credits"`
	UsedBalance   float64 `json:"used_balance"`
	UsedCredits   float64 `json:"used_credits"`
}

// UploadResponse is the response from POST /v1/uploads/images.
type UploadResponse struct {
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Bytes       int    `json:"bytes"`
	CreatedAt   int64  `json:"created_at"`
}

// VideoGenerateRequest is the request body for POST /v1/videos/generations.
type VideoGenerateRequest struct {
	Model           string          `json:"model"`
	Prompt          string          `json:"prompt,omitempty"`
	Duration        *int            `json:"duration,omitempty"`
	Size            string          `json:"size,omitempty"`
	Resolution      string          `json:"resolution,omitempty"`
	Seed            *int            `json:"seed,omitempty"`
	GenerateAudio   *bool           `json:"generate_audio,omitempty"`
	ReturnLastFrame *bool           `json:"return_last_frame,omitempty"`
	Tools           []VideoTool     `json:"tools,omitempty"`
	ImageURLs       []string        `json:"image_urls,omitempty"`
	ImageWithRoles  []ImageWithRole `json:"image_with_roles,omitempty"`
	VideoURLs       []string        `json:"video_urls,omitempty"`
	AudioURLs       []string        `json:"audio_urls,omitempty"`
}

// VideoTool represents a tool for video generation (e.g. web_search).
type VideoTool struct {
	Type string `json:"type"`
}

// ImageWithRole represents an image with a specific role (first_frame, last_frame, reference_image).
type ImageWithRole struct {
	URL  string `json:"url"`
	Role string `json:"role"`
}

// VideoRemixRequest is the request body for POST /v1/videos/{task_id}/remix (VEO3 Remix).
type VideoRemixRequest struct {
	Model       string `json:"model"`
	Prompt      string `json:"prompt"`
	Raw         *bool  `json:"raw,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Resolution  string `json:"resolution,omitempty"`
}

// VideoRemixResponse is the response from POST /v1/videos/{task_id}/remix.
type VideoRemixResponse struct {
	Code int              `json:"code"`
	Data []TaskSubmission `json:"data"`
}

// VideoGenerateResponse is the response from POST /v1/videos/generations.
type VideoGenerateResponse struct {
	Code int              `json:"code"`
	Data []TaskSubmission `json:"data"`
}

// ChatMessage represents a single message in a chat conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the request body for chat completion.
type ChatRequest struct {
	Model            string        `json:"model"`
	Messages         []ChatMessage `json:"messages"`
	Stream           bool          `json:"stream,omitempty"`
	Temperature      *float64      `json:"temperature,omitempty"`
	MaxTokens        *int          `json:"max_tokens,omitempty"`
	TopP             *float64      `json:"top_p,omitempty"`
	FrequencyPenalty *float64      `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64      `json:"presence_penalty,omitempty"`
	Stop             []string      `json:"stop,omitempty"`
	N                *int          `json:"n,omitempty"`
}

// ChatResponse is the non-streaming response from chat completion.
type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   *ChatUsage   `json:"usage,omitempty"`
}

// ChatChoice represents a single choice in a chat response.
type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// ChatStreamChunk represents a single SSE chunk in streaming response.
type ChatStreamChunk struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []ChatStreamChoice `json:"choices"`
}

// ChatStreamChoice represents a choice in a streaming chunk.
type ChatStreamChoice struct {
	Index        int             `json:"index"`
	Delta        ChatStreamDelta `json:"delta"`
	FinishReason string          `json:"finish_reason,omitempty"`
}

// ChatStreamDelta represents the delta content in a streaming chunk.
type ChatStreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// ChatUsage represents token usage statistics.
// Compatible with OpenAI and OpenRouter (which adds cost).
type ChatUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Cost             float64 `json:"cost,omitempty"` // OpenRouter-specific
}

// MarketplaceResponse is the response from the public marketplace API.
type MarketplaceResponse struct {
	Success bool            `json:"success"`
	Data    MarketplaceData `json:"data"`
}

type MarketplaceData struct {
	Total    int                `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Models   []MarketplaceModel `json:"models"`
}

type MarketplaceModel struct {
	ID          int                `json:"id"`
	ModelName   string             `json:"model_name"`
	DisplayName string             `json:"display_name"`
	Description string             `json:"description"`
	MediaType   string             `json:"media_type"`
	DetailURL   string             `json:"detail_url"`
	Tags        []string           `json:"tags"`
	Vendor      *MarketplaceVendor `json:"vendor"`
	Pricing     MarketplacePricing `json:"pricing"`
	CallCount   int64              `json:"call_count"`
	DiscountPct int                `json:"discount_percent"`
}

type MarketplaceVendor struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

type MarketplacePricing struct {
	StartingPrice float64 `json:"starting_price"`
	DiscountRate  float64 `json:"discount_rate"`
	CreditsPerGen int     `json:"credits_per_generation"`
	BillingType   string  `json:"billing_type"`
	PriceUnit     string  `json:"price_unit"`
	HasPrice      bool    `json:"has_price"`
	InputPrice    float64 `json:"input_price,omitempty"`
	OutputPrice   float64 `json:"output_price,omitempty"`
}

// ModelPricingResponse is the response from /api/pricing/model?model=xxx.
type ModelPricingResponse struct {
	Success bool             `json:"success"`
	Data    ModelPricingData `json:"data"`
}

type ModelPricingData struct {
	ModelName          string                        `json:"model_name"`
	BillingType        string                        `json:"billing_type"`
	ModelPrice         float64                       `json:"model_price"`
	DiscountRate       float64                       `json:"discount_rate"`
	ResolutionEnabled  bool                          `json:"resolution_enabled"`
	SupportedSizes     []string                      `json:"supported_sizes"`
	SupportedQualities []string                      `json:"supported_qualities"`
	SizeQualityPrices  map[string]map[string]float64 `json:"size_quality_prices"`
	ResolutionPrices   map[string]float64            `json:"resolution_prices"`
}

// Config represents the YAML configuration file structure.
type Config struct {
	APIKey     string          `mapstructure:"api_key" yaml:"api_key"`
	BaseURL    string          `mapstructure:"base_url" yaml:"base_url"`
	HTTPProxy  string          `mapstructure:"http_proxy" yaml:"http_proxy"`
	Verbose    bool            `mapstructure:"verbose" yaml:"verbose"`
	SavePrompt bool            `mapstructure:"save_prompt" yaml:"save_prompt"`
	Mode       string          `mapstructure:"mode" yaml:"mode"` // sync, async, or "" for auto
	OutputDir  string          `mapstructure:"output_dir" yaml:"output_dir"`
	Defaults   *ConfigDefaults `mapstructure:"defaults" yaml:"defaults"`
}

// ConfigDefaults holds modality-specific default values.
type ConfigDefaults struct {
	Image      *ImageDefaults      `mapstructure:"image" yaml:"image"`
	Video      *VideoDefaults      `mapstructure:"video" yaml:"video"`
	Midjourney *MidjourneyDefaults `mapstructure:"midjourney" yaml:"midjourney"`
	Chat       *ChatDefaults       `mapstructure:"chat" yaml:"chat"`
}

// ChatDefaults holds default values for chat completion.
type ChatDefaults struct {
	Model       string  `mapstructure:"model" yaml:"model"`
	Temperature float64 `mapstructure:"temperature" yaml:"temperature"`
	MaxTokens   int     `mapstructure:"max_tokens" yaml:"max_tokens"`
}

// ImageDefaults holds default values for image generation.
type ImageDefaults struct {
	Model             string   `mapstructure:"model" yaml:"model"`
	Size              string   `mapstructure:"size" yaml:"size"`
	Resolution        string   `mapstructure:"resolution" yaml:"resolution"`
	Quality           string   `mapstructure:"quality" yaml:"quality"`
	Background        string   `mapstructure:"background" yaml:"background"`
	Moderation        string   `mapstructure:"moderation" yaml:"moderation"`
	OutputFormat      string   `mapstructure:"output_format" yaml:"output_format"`
	OutputCompression *int     `mapstructure:"output_compression" yaml:"output_compression"`
	N                 *int     `mapstructure:"n" yaml:"n"`
	ImageURLs         []string `mapstructure:"image_urls" yaml:"image_urls"`
	MaskURL           string   `mapstructure:"mask_url" yaml:"mask_url"`
	Style             string   `mapstructure:"style" yaml:"style"`
	ResponseFormat    string   `mapstructure:"response_format" yaml:"response_format"`
}

// MergeIntoImage applies non-zero default values to an image generation request.
func (d *ImageDefaults) MergeIntoImage(req *GenerateRequest) {
	if d == nil {
		return
	}
	if req.Model == "" && d.Model != "" {
		req.Model = d.Model
	}
	if req.Size == "" && d.Size != "" {
		req.Size = d.Size
	}
	if req.Resolution == "" && d.Resolution != "" {
		req.Resolution = d.Resolution
	}
	if req.Quality == "" && d.Quality != "" {
		req.Quality = d.Quality
	}
	if req.Background == "" && d.Background != "" {
		req.Background = d.Background
	}
	if req.Moderation == "" && d.Moderation != "" {
		req.Moderation = d.Moderation
	}
	if req.OutputFormat == "" && d.OutputFormat != "" {
		req.OutputFormat = d.OutputFormat
	}
	if req.OutputCompression == nil && d.OutputCompression != nil {
		req.OutputCompression = d.OutputCompression
	}
	if req.N == nil && d.N != nil {
		req.N = d.N
	}
	if len(req.ImageURLs) == 0 && len(d.ImageURLs) > 0 {
		req.ImageURLs = d.ImageURLs
	}
	if req.MaskURL == "" && d.MaskURL != "" {
		req.MaskURL = d.MaskURL
	}
	if req.Style == "" && d.Style != "" {
		req.Style = d.Style
	}
	if req.ResponseFormat == "" && d.ResponseFormat != "" {
		req.ResponseFormat = d.ResponseFormat
	}
}

// VideoDefaults holds default values for video generation.
type VideoDefaults struct {
	Model      string   `mapstructure:"model" yaml:"model"`
	Size       string   `mapstructure:"size" yaml:"size"`
	Resolution string   `mapstructure:"resolution" yaml:"resolution"`
	Duration   *int     `mapstructure:"duration" yaml:"duration"`
	ImageURLs  []string `mapstructure:"image_urls" yaml:"image_urls"`
	VideoURLs  []string `mapstructure:"video_urls" yaml:"video_urls"`
	AudioURLs  []string `mapstructure:"audio_urls" yaml:"audio_urls"`
}

// MidjourneyDefaults holds default values for Midjourney generation.
type MidjourneyDefaults struct {
	Speed   string `mapstructure:"speed" yaml:"speed"`
	Version string `mapstructure:"version" yaml:"version"`
	Style   string `mapstructure:"style" yaml:"style"`
	Size    string `mapstructure:"size" yaml:"size"`
	Quality string `mapstructure:"quality" yaml:"quality"`
	Niji    *bool  `mapstructure:"niji" yaml:"niji"`
}

// MergeIntoImagine applies non-zero default values to an MJ imagine request.
func (d *MidjourneyDefaults) MergeIntoImagine(req *MJImagineRequest) {
	if d == nil {
		return
	}
	if req.Speed == "" && d.Speed != "" {
		req.Speed = d.Speed
	}
	if req.Version == "" && d.Version != "" {
		req.Version = d.Version
	}
	if req.Style == "" && d.Style != "" {
		req.Style = d.Style
	}
	if req.Size == "" && d.Size != "" {
		req.Size = d.Size
	}
	if req.Quality == "" && d.Quality != "" {
		req.Quality = d.Quality
	}
	if req.Niji == nil && d.Niji != nil {
		req.Niji = d.Niji
	}
}

// MergeIntoVideo applies non-zero default values to a video generation request.
func (d *VideoDefaults) MergeIntoVideo(req *VideoGenerateRequest) {
	if d == nil {
		return
	}
	if req.Model == "" && d.Model != "" {
		req.Model = d.Model
	}
	if req.Size == "" && d.Size != "" {
		req.Size = d.Size
	}
	if req.Resolution == "" && d.Resolution != "" {
		req.Resolution = d.Resolution
	}
	if req.Duration == nil && d.Duration != nil {
		req.Duration = d.Duration
	}
	if len(req.ImageURLs) == 0 && len(d.ImageURLs) > 0 {
		req.ImageURLs = d.ImageURLs
	}
	if len(req.VideoURLs) == 0 && len(d.VideoURLs) > 0 {
		req.VideoURLs = d.VideoURLs
	}
	if len(req.AudioURLs) == 0 && len(d.AudioURLs) > 0 {
		req.AudioURLs = d.AudioURLs
	}
}
