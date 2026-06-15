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

// TaskData contains the full task information.
type TaskData struct {
	ID         string       `json:"id"`
	Status     string       `json:"status"`
	Progress   int          `json:"progress"`
	ActualTime float64      `json:"actual_time,omitempty"`
	Cost       float64      `json:"cost,omitempty"`
	Result     *TaskResult  `json:"result,omitempty"`
}

// TaskResult contains the generated images.
type TaskResult struct {
	Images []ImageResult `json:"images"`
}

// ImageResult contains URLs for a generated image.
type ImageResult struct {
	URL       []string `json:"url"`
	ExpiresAt int64    `json:"expires_at"`
}

// Config represents the YAML configuration file structure.
type Config struct {
	APIKey    string           `mapstructure:"api_key" yaml:"api_key"`
	BaseURL   string           `mapstructure:"base_url" yaml:"base_url"`
	HTTPProxy string           `mapstructure:"http_proxy" yaml:"http_proxy"`
	Defaults  *ConfigDefaults `mapstructure:"defaults" yaml:"defaults"`
}

// ConfigDefaults holds default values for generation parameters.
type ConfigDefaults struct {
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

// MergeInto applies non-zero default values from d to the request.
func (d *ConfigDefaults) MergeInto(req *GenerateRequest) {
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
