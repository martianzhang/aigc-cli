package types

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

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
	Usage   *OpenAIImageUsage `json:"usage,omitempty"`
}

// OpenAIImageData represents a single generated image in sync mode.
type OpenAIImageData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// OpenAIImageUsage holds token and cost information for image generation.
type OpenAIImageUsage struct {
	PromptTokens     int     `json:"prompt_tokens,omitempty"`
	CompletionTokens int     `json:"completion_tokens,omitempty"`
	TotalTokens      int     `json:"total_tokens,omitempty"`
	Cost             float64 `json:"cost,omitempty"`
}

// OpenAIModel is a single model from GET /v1/models.
type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
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

// YunwuVideoCreateResponse is returned by yunwu.ai's POST /v1/video/create.
type YunwuVideoCreateResponse struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	StatusUpdateTime int64  `json:"status_update_time,omitempty"`
}

// YunwuVideoQueryResponse is returned when polling yunwu.ai's video task.
type YunwuVideoQueryResponse struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	VideoURL         string `json:"video_url,omitempty"`
	EnhancedPrompt   string `json:"enhanced_prompt,omitempty"`
	StatusUpdateTime int64  `json:"status_update_time,omitempty"`
}

// ToolDefinition defines a tool that the LLM can call.
type ToolDefinition struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction defines the schema of a callable function.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// ToolCall is a tool call returned by the LLM.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction contains the name and arguments of a tool call.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ChatStreamDeltaToolCall represents a tool call delta in streaming response.
type ChatStreamDeltaToolCall struct {
	Index    int                      `json:"index"`
	ID       string                   `json:"id,omitempty"`
	Type     string                   `json:"type,omitempty"`
	Function *ChatStreamDeltaFunction `json:"function,omitempty"`
}

// ChatStreamDeltaFunction contains partial name/arguments in a streaming delta.
type ChatStreamDeltaFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ChatMessage represents a single message in a chat conversation.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ChatRequest is the request body for chat completion.
type ChatRequest struct {
	Model            string           `json:"model"`
	Messages         []ChatMessage    `json:"messages"`
	Stream           bool             `json:"stream,omitempty"`
	Temperature      *float64         `json:"temperature,omitempty"`
	MaxTokens        *int             `json:"max_tokens,omitempty"`
	TopP             *float64         `json:"top_p,omitempty"`
	FrequencyPenalty *float64         `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64         `json:"presence_penalty,omitempty"`
	Stop             []string         `json:"stop,omitempty"`
	N                *int             `json:"n,omitempty"`
	Tools            []ToolDefinition `json:"tools,omitempty"`
	// OutputWriter directs streaming output. When nil, streamed tokens are
	// accumulated silently and returned in the full response (no terminal output).
	// CLI sets this to os.Stdout; MCP and other callers leave it nil.
	OutputWriter io.Writer `json:"-" yaml:"-"`
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
	Role      string                    `json:"role,omitempty"`
	Content   string                    `json:"content,omitempty"`
	ToolCalls []ChatStreamDeltaToolCall `json:"tool_calls,omitempty"`
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

// FormatPrice formats the model's starting price for display.
func (m MarketplaceModel) FormatPrice() string {
	if !m.Pricing.HasPrice {
		return "—"
	}
	unit := m.Pricing.PriceUnit
	if unit == "" {
		unit = "/次"
	}
	return fmt.Sprintf("$%.4f%s", m.Pricing.StartingPrice, unit)
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
