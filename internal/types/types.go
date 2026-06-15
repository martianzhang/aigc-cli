// Package types defines request/response data structures for the APIMart API.
package types

// GenerateRequest is the request body for POST /v1/images/generations.
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
	Code int        `json:"code"`
	Data *TaskData  `json:"data"`
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
	Code    int    `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
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

// VideoGenerateResponse is the response from POST /v1/videos/generations.
type VideoGenerateResponse struct {
	Code int                `json:"code"`
	Data []TaskSubmission   `json:"data"`
}

// Config represents the YAML configuration file structure.
type Config struct {
	APIKey    string           `mapstructure:"api_key" yaml:"api_key"`
	BaseURL   string           `mapstructure:"base_url" yaml:"base_url"`
	HTTPProxy string           `mapstructure:"http_proxy" yaml:"http_proxy"`
	Defaults  *ConfigDefaults `mapstructure:"defaults" yaml:"defaults"`
}

// ConfigDefaults holds modality-specific default values.
type ConfigDefaults struct {
	Image *ImageDefaults `mapstructure:"image" yaml:"image"`
	Video *VideoDefaults `mapstructure:"video" yaml:"video"`
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
}

// VideoDefaults holds default values for video generation.
type VideoDefaults struct {
	Model       string   `mapstructure:"model" yaml:"model"`
	Size        string   `mapstructure:"size" yaml:"size"`
	Resolution  string   `mapstructure:"resolution" yaml:"resolution"`
	Duration    *int     `mapstructure:"duration" yaml:"duration"`
	ImageURLs   []string `mapstructure:"image_urls" yaml:"image_urls"`
	VideoURLs   []string `mapstructure:"video_urls" yaml:"video_urls"`
	AudioURLs   []string `mapstructure:"audio_urls" yaml:"audio_urls"`
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
