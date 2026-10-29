package types

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/jsonmerge"
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
	// Ratio is the aspect ratio for tiered sizing (Agnes 2.1+), e.g. "16:9", "1:1".
	Ratio string `json:"ratio,omitempty" yaml:"ratio,omitempty"`
	// OpenAI-specific fields
	Style          string `json:"style,omitempty" yaml:"style,omitempty"`
	ResponseFormat string `json:"response_format,omitempty" yaml:"response_format,omitempty"`
	// ExtraBody holds provider-specific fields that must be nested inside
	// "extra_body" (e.g., Agnes API requires response_format in extra_body).
	ExtraBody map[string]any `json:"extra_body,omitempty" yaml:"extra_body,omitempty"`
	// RawJSON is a verbatim --json body; when set it overrides MarshalJSON so
	// provider-specific parameters reach the API unmodelled.
	RawJSON json.RawMessage `json:"-" yaml:"-"`
	// InjectJSON is a --inject-json snippet merged additively on top of the
	// final body (objects recurse, arrays concatenate, scalars overwrite).
	InjectJSON json.RawMessage `json:"-" yaml:"-"`
}

// MarshalJSON emits RawJSON unchanged when set, otherwise the typed fields,
// then layers InjectJSON on top.
func (r GenerateRequest) MarshalJSON() ([]byte, error) {
	type typed GenerateRequest
	return marshalVerbatim(r.RawJSON, r.InjectJSON, typed(r))
}

// BodyOrRaw returns the verbatim --json body when set, otherwise the
// caller-built body. Providers whose endpoint needs a different wire shape
// (OpenRouter, Gemini, Ollama, /images/edits) build that shape and pass it
// here, so --json passthrough cannot be silently dropped on those paths.
func (r *GenerateRequest) BodyOrRaw(built any) (any, error) {
	return RawOr(r.RawJSON, r.InjectJSON, built)
}

// BodyOrRaw is the video twin of GenerateRequest.BodyOrRaw.
func (r *VideoGenerateRequest) BodyOrRaw(built any) (any, error) {
	return RawOr(r.RawJSON, r.InjectJSON, built)
}

// RawOr returns raw verbatim when set, otherwise built, then merges inject on
// top. It returns an error instead of swallowing one, so a malformed
// --inject-json surfaces rather than silently sending a wrong body.
func RawOr(raw json.RawMessage, inject json.RawMessage, built any) (any, error) {
	if len(inject) == 0 {
		if len(raw) > 0 {
			return raw, nil
		}
		return built, nil
	}
	base := raw
	if len(base) == 0 {
		b, err := json.Marshal(built)
		if err != nil {
			return nil, err
		}
		base = b
	}
	return jsonmerge.Inject(base, inject)
}

// MaybeInject layers inject on top of an already-built body. It returns body
// unchanged when inject is empty, so providers that build provider-native maps
// (rather than a request type with MarshalJSON) still honor --inject-json.
func MaybeInject(body any, inject json.RawMessage) (any, error) {
	if len(inject) == 0 {
		return body, nil
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return jsonmerge.Inject(data, inject)
}

// marshalVerbatim returns raw unchanged when set, otherwise the marshalled
// typed view, then layers inject on top. Request types route MarshalJSON
// through it so a --json body reaches the wire verbatim and --inject-json is
// added to the final body instead of being rebuilt from modeled fields.
func marshalVerbatim(raw json.RawMessage, inject json.RawMessage, typed any) ([]byte, error) {
	base := raw
	if len(base) == 0 {
		b, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		base = b
	}
	merged, err := jsonmerge.Inject(base, inject)
	if err != nil {
		return nil, err
	}
	return merged, nil
}

// ValidateBackground checks background/output_format compatibility.
// OpenAI's Images API requires png or webp when background=transparent.
func (r *GenerateRequest) ValidateBackground() error {
	if !strings.EqualFold(r.Background, "transparent") {
		return nil
	}
	f := strings.ToLower(r.OutputFormat)
	if f != "" && f != "png" && f != "webp" {
		return fmt.Errorf("background=transparent requires output_format=png or webp (got %q)", r.OutputFormat)
	}
	return nil
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
	// SupportedEndpointTypes is a non-OpenAI extension some relays return
	// (e.g. Agnes: ["openai"]). It is displayed when present and ignored
	// otherwise.
	SupportedEndpointTypes []string `json:"supported_endpoint_types,omitempty"`
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

// OpenRouterCredits is the response from GET /api/v1/credits. Account-level:
// remaining credits = TotalCredits - TotalUsage. Requires a management key.
type OpenRouterCredits struct {
	Data struct {
		TotalCredits float64 `json:"total_credits"`
		TotalUsage   float64 `json:"total_usage"`
	} `json:"data"`
}

// OpenRouterKeyInfo is the response from GET /api/v1/key. Limit and
// LimitRemaining are nil when the key has no spending cap.
type OpenRouterKeyInfo struct {
	Data struct {
		Label          string   `json:"label"`
		Limit          *float64 `json:"limit"`
		LimitRemaining *float64 `json:"limit_remaining"`
		Usage          float64  `json:"usage"`
		IsFreeTier     bool     `json:"is_free_tier"`
	} `json:"data"`
}

// DeepSeekBalance is the response from GET /user/balance. Amounts are decimal
// strings, one entry per currency (CNY, USD).
type DeepSeekBalance struct {
	IsAvailable  bool                  `json:"is_available"`
	BalanceInfos []DeepSeekBalanceItem `json:"balance_infos"`
}

// DeepSeekBalanceItem is one currency entry of DeepSeekBalance.
type DeepSeekBalanceItem struct {
	Currency        string `json:"currency"`
	TotalBalance    string `json:"total_balance"`
	GrantedBalance  string `json:"granted_balance"`
	ToppedUpBalance string `json:"topped_up_balance"`
}

// MoonshotBalance is the response from GET /v1/users/me/balance. Amounts are in
// CNY.
type MoonshotBalance struct {
	Code   int  `json:"code"`
	Status bool `json:"status"`
	Data   struct {
		AvailableBalance float64 `json:"available_balance"`
		VoucherBalance   float64 `json:"voucher_balance"`
		CashBalance      float64 `json:"cash_balance"`
	} `json:"data"`
}

// OfoxBalance is the response from GET /v1/user/balance (OfoxAI). It is
// deliberately compatibly shaped for third-party balance widgets.
type OfoxBalance struct {
	IsActive bool    `json:"is_active"`
	Balance  float64 `json:"balance"`
	Total    float64 `json:"total"`
	Used     float64 `json:"used"`
	Currency string  `json:"currency"`
}

// OpenAIBillingSubscription is GET /dashboard/billing/subscription. Relays
// built on new-api/one-api answer it with placeholder limits (1e6 = "unlimited")
// instead of a real cap.
type OpenAIBillingSubscription struct {
	Object           string  `json:"object"`
	HasPaymentMethod bool    `json:"has_payment_method"`
	SoftLimitUSD     float64 `json:"soft_limit_usd"`
	HardLimitUSD     float64 `json:"hard_limit_usd"`
	SystemHardLimit  float64 `json:"system_hard_limit_usd"`
}

// OpenAIBillingUsage is GET /dashboard/billing/usage. TotalUsage is in cents,
// matching OpenAI's own billing API.
type OpenAIBillingUsage struct {
	Object     string  `json:"object"`
	TotalUsage float64 `json:"total_usage"`
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
	// RawJSON is a verbatim --json body; when set it overrides MarshalJSON so
	// provider-specific parameters reach the API unmodelled.
	RawJSON json.RawMessage `json:"-" yaml:"-"`
	// InjectJSON is a --inject-json snippet merged additively on top of the
	// final body (objects recurse, arrays concatenate, scalars overwrite).
	InjectJSON json.RawMessage `json:"-" yaml:"-"`
}

// MarshalJSON emits RawJSON unchanged when set, otherwise the typed fields,
// then layers InjectJSON on top.
func (r VideoGenerateRequest) MarshalJSON() ([]byte, error) {
	type typed VideoGenerateRequest
	return marshalVerbatim(r.RawJSON, r.InjectJSON, typed(r))
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

// OpenLuxVideoCreateResponse is returned by api.openlux.ai's POST /v1/video/create.
type OpenLuxVideoCreateResponse struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	StatusUpdateTime int64  `json:"status_update_time,omitempty"`
}

// OpenLuxVideoQueryResponse is returned when polling api.openlux.ai's video task.
type OpenLuxVideoQueryResponse struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	VideoURL         string `json:"video_url,omitempty"`
	EnhancedPrompt   string `json:"enhanced_prompt,omitempty"`
	StatusUpdateTime int64  `json:"status_update_time,omitempty"`
}

// AgnesVideoCreateResponse is returned by agnes.ai's POST /v1/videos (async task).
type AgnesVideoCreateResponse struct {
	ID        string `json:"id"`
	VideoID   string `json:"video_id"`
	TaskID    string `json:"task_id"`
	Object    string `json:"object"`
	Model     string `json:"model"`
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
	CreatedAt int64  `json:"created_at"`
	Seconds   string `json:"seconds"`
	Size      string `json:"size"`
}

// AgnesVideoQueryResponse is returned when polling agnes.ai's video task via GET /agnesapi.
type AgnesVideoQueryResponse struct {
	ID          string          `json:"id"`
	VideoID     string          `json:"video_id"`
	TaskID      string          `json:"task_id"`
	Model       string          `json:"model"`
	Status      string          `json:"status"`
	Progress    int             `json:"progress"`
	CreatedAt   int64           `json:"created_at"`
	CompletedAt int64           `json:"completed_at,omitempty"`
	Seconds     string          `json:"seconds"`
	Size        string          `json:"size"`
	URL         string          `json:"url"` // final video URL when completed
	Metadata    *AgnesVideoMeta `json:"metadata,omitempty"`
	Error       json.RawMessage `json:"error,omitempty"`
}

// AgnesVideoMeta holds the final result metadata (url lives here per API docs,
// but is also mirrored at the top-level `url` field).
type AgnesVideoMeta struct {
	URL string `json:"url"`
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

// ChatImage is an image attached to a chat message. It travels through the
// pipeline as raw bytes and is expanded to a provider-specific wire shape at
// marshal time (OpenAI image_url data URI, Anthropic base64 source, Responses
// input_image).
type ChatImage struct {
	Name     string `json:"-"` // display name (informational)
	MIMEType string `json:"-"` // e.g. "image/png"
	Data     []byte `json:"-"`
}

// DataURI renders the image as a base64 data URI (used by OpenAI-style and
// Responses-style wire formats).
func (i ChatImage) DataURI() string {
	return "data:" + i.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(i.Data)
}

// ChatMessage represents a single message in a chat conversation.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	// Annotations are citations returned by providers that run web search
	// server-side (OpenAI url_citation, OpenRouter web plugin, MiMo web_search).
	// They are response-only: MarshalJSON never emits them on a request.
	Annotations []ChatAnnotation `json:"annotations,omitempty"`
	// Images are multimodal attachments. They are never serialized as a field
	// (json:"-"): MarshalJSON folds them into Content as OpenAI-style
	// content parts when any are present.
	Images []ChatImage `json:"-"`
}

// ChatAnnotation is one citation attached to an assistant message by a
// provider-side web search tool. The common fields follow OpenAI's
// url_citation shape; MiMo also fills site_name, publish_time, summary and
// logo_url, all of which are preserved for callers that want them.
type ChatAnnotation struct {
	Type        string `json:"type,omitempty"`
	URL         string `json:"url,omitempty"`
	Title       string `json:"title,omitempty"`
	SiteName    string `json:"site_name,omitempty"`
	PublishTime string `json:"publish_time,omitempty"`
	Summary     string `json:"summary,omitempty"`
	LogoURL     string `json:"logo_url,omitempty"`
}

// MarshalJSON emits the OpenAI-compatible wire shape. With no images the output
// is identical to the previous string-content form (empty content omitted).
// With images, content becomes a parts array: a text part (when non-empty)
// followed by one image_url part per attachment.
func (m ChatMessage) MarshalJSON() ([]byte, error) {
	type wire struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content,omitempty"`
		ToolCallID string          `json:"tool_call_id,omitempty"`
		ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	}

	out := wire{Role: m.Role, ToolCallID: m.ToolCallID, ToolCalls: m.ToolCalls}
	switch {
	case len(m.Images) == 0:
		if m.Content != "" {
			raw, err := json.Marshal(m.Content)
			if err != nil {
				return nil, err
			}
			out.Content = raw
		}
	default:
		parts := make([]map[string]any, 0, len(m.Images)+1)
		if m.Content != "" {
			parts = append(parts, map[string]any{"type": "text", "text": m.Content})
		}
		for _, img := range m.Images {
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": img.DataURI()},
			})
		}
		raw, err := json.Marshal(parts)
		if err != nil {
			return nil, err
		}
		out.Content = raw
	}
	return json.Marshal(out)
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
	// RawJSON is a verbatim --json body; when set it overrides MarshalJSON so
	// provider-specific parameters reach the API unmodelled.
	RawJSON json.RawMessage `json:"-" yaml:"-"`
	// InjectJSON is a --inject-json snippet merged additively on top of the
	// final body (objects recurse, arrays concatenate, scalars overwrite).
	InjectJSON json.RawMessage `json:"-" yaml:"-"`
}

// MarshalJSON emits RawJSON unchanged when set, otherwise the typed fields,
// then layers InjectJSON on top.
func (r ChatRequest) MarshalJSON() ([]byte, error) {
	type typed ChatRequest
	return marshalVerbatim(r.RawJSON, r.InjectJSON, typed(r))
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
	// Usage is emitted by providers that report usage in the final chunk.
	Usage *ChatUsage `json:"usage,omitempty"`
}

// ChatStreamChoice represents a choice in a streaming chunk.
type ChatStreamChoice struct {
	Index        int             `json:"index"`
	Delta        ChatStreamDelta `json:"delta"`
	FinishReason string          `json:"finish_reason,omitempty"`
}

// ChatStreamDelta represents the delta content in a streaming chunk.
type ChatStreamDelta struct {
	Role        string                    `json:"role,omitempty"`
	Content     string                    `json:"content,omitempty"`
	ToolCalls   []ChatStreamDeltaToolCall `json:"tool_calls,omitempty"`
	Annotations []ChatAnnotation          `json:"annotations,omitempty"`
}

// ChatUsage represents token usage statistics.
// Compatible with OpenAI and OpenRouter (which adds cost).
type ChatUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Cost             float64 `json:"cost,omitempty"` // OpenRouter-specific
	// WebSearchUsage is MiMo's server-side search breakdown
	// (usage.web_search_usage). Other providers leave it nil.
	WebSearchUsage *WebSearchUsage `json:"web_search_usage,omitempty"`
}

// WebSearchUsage reports MiMo web_search consumption: how many search calls
// were made and how many result pages were returned.
type WebSearchUsage struct {
	ToolUsage int `json:"tool_usage"`
	PageUsage int `json:"page_usage"`
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
