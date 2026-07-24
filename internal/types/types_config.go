package types

// ProviderType represents the API protocol type for a named provider.
type ProviderType string

const (
	ProviderOpenAI    ProviderType = "openai"    // OpenAI-compatible API (default)
	ProviderOllama    ProviderType = "ollama"    // Ollama local (OpenAI subset, no API key)
	ProviderGoogle    ProviderType = "google"    // Google Gemini API (reserved)
	ProviderAnthropic ProviderType = "anthropic" // Anthropic Messages API (reserved)
	ProviderLocal     ProviderType = "local"     // Local ONNX models (ocr/vision/detect/background)
)

// NamedProvider defines a reusable provider configuration.
// Users define these in the `providers` section of config.yaml and reference
// them by name from defaults.{cmd}.provider.
type NamedProvider struct {
	// Type is the API protocol. Defaults to "openai" when empty.
	Type ProviderType `mapstructure:"type" yaml:"type,omitempty"`
	// API credentials (not needed for local/Ollama providers).
	APIKey    string `mapstructure:"api_key" yaml:"api_key,omitempty"`
	BaseURL   string `mapstructure:"base_url" yaml:"base_url,omitempty"`
	HTTPProxy string `mapstructure:"http_proxy" yaml:"http_proxy,omitempty"`
	// Local model settings (used when type=local).
	ModelsDir string `mapstructure:"models_dir" yaml:"models_dir,omitempty"`
	Model     string `mapstructure:"model" yaml:"model,omitempty"`
}

type Config struct {
	// — Global fallback values (used when no named provider is referenced) —
	APIKey    string `mapstructure:"api_key" yaml:"api_key,omitempty"`
	BaseURL   string `mapstructure:"base_url" yaml:"base_url,omitempty"`
	HTTPProxy string `mapstructure:"http_proxy" yaml:"http_proxy,omitempty"`
	// — Named provider registry (user-defined, referenced by defaults.{cmd}.provider) —
	Providers map[string]*NamedProvider `mapstructure:"providers" yaml:"providers,omitempty"`

	Verbose      bool              `mapstructure:"verbose" yaml:"verbose"`
	SavePrompt   bool              `mapstructure:"save_prompt" yaml:"save_prompt"`
	Mode         string            `mapstructure:"mode" yaml:"mode,omitempty"`
	OutputDir    string            `mapstructure:"output_dir" yaml:"output_dir,omitempty"`
	Timeout      *int              `mapstructure:"timeout" yaml:"timeout,omitempty"`
	Defaults     *ConfigDefaults   `mapstructure:"defaults" yaml:"defaults,omitempty"`
	Ideas        *IdeasConfig      `mapstructure:"ideas" yaml:"ideas,omitempty"`
	Detect       *DetectConfig     `mapstructure:"detect" yaml:"detect,omitempty"`
	Background   *BackgroundConfig `mapstructure:"background" yaml:"background,omitempty"`
	ToolsEnable  []string          `mapstructure:"tools_enable" yaml:"tools_enable,omitempty"`
	ToolsDisable []string          `mapstructure:"tools_disable" yaml:"tools_disable,omitempty"`
}

// IdeasConfig controls the ideas prompt data path.
type IdeasConfig struct {
	DataPath string `mapstructure:"data_path" yaml:"data_path,omitempty"`
}

// DetectConfig controls the AIGC detection behavior.
type DetectConfig struct {
	// ModelsDir is the directory where ONNX Runtime and model files are stored.
	// Default: ~/.config/aigc-cli/models
	ModelsDir string `mapstructure:"models_dir" yaml:"models_dir,omitempty"`
	// Model selects which ONNX model to use.
	// Supported: "vit-base" (86M params, default), "distilled-vit" (11.8M params)
	// Add new models here as the project grows.
	Model string `mapstructure:"model" yaml:"model,omitempty"`
}

// BackgroundConfig controls the RMBG-based background removal behavior.
type BackgroundConfig struct {
	// ModelsDir is the directory where ONNX Runtime and RMBG model files are stored.
	// Default: ~/.config/aigc-cli/models (same as detect.models_dir).
	ModelsDir string `mapstructure:"models_dir" yaml:"models_dir,omitempty"`
	// Model selects which RMBG ONNX model to use.
	// Supported: "rmbg-2.0" (default)
	Model string `mapstructure:"model" yaml:"model,omitempty"`
}

// AIGCDetectionConfig holds optional online API settings for pixel-level AI image detection.
type AIGCDetectionConfig struct {
	Provider    string             `mapstructure:"provider" yaml:"provider,omitempty"`
	Sightengine *SightengineConfig `mapstructure:"sightengine" yaml:"sightengine,omitempty"`
}

// SightengineConfig holds API credentials for Sightengine's genai detection.
// Get free credentials at https://dashboard.sightengine.com/signup (2000 ops/month).
type SightengineConfig struct {
	APIUser   string `mapstructure:"api_user" yaml:"api_user,omitempty"`
	APISecret string `mapstructure:"api_secret" yaml:"api_secret,omitempty"`
}

// ConfigDefaults holds modality-specific default values.
type ConfigDefaults struct {
	Image      *ImageDefaults      `mapstructure:"image" yaml:"image"`
	Video      *VideoDefaults      `mapstructure:"video" yaml:"video"`
	Midjourney *MidjourneyDefaults `mapstructure:"midjourney" yaml:"midjourney"`
	Chat       *ChatDefaults       `mapstructure:"chat" yaml:"chat"`
	Audio      *AudioDefaults      `mapstructure:"audio" yaml:"audio"`
	OCR        *OCRDefaults        `mapstructure:"ocr" yaml:"ocr,omitempty"`
	Vision     *VisionDefaults     `mapstructure:"vision" yaml:"vision,omitempty"`
}

// OCRDefaults holds default values for OCR scanning.
type OCRDefaults struct {
	Provider string `mapstructure:"provider" yaml:"provider,omitempty"`
	Model    string `mapstructure:"model" yaml:"model,omitempty"`
}

// VisionDefaults holds default values for vision/describe.
type VisionDefaults struct {
	Provider  string `mapstructure:"provider" yaml:"provider,omitempty"`
	Model     string `mapstructure:"model" yaml:"model,omitempty"`
	MaxTokens int    `mapstructure:"max_tokens" yaml:"max_tokens,omitempty"`
}

// ChatDefaults holds default values for chat completion.
type ChatDefaults struct {
	Provider          string   `mapstructure:"provider" yaml:"provider,omitempty"`
	Model             string   `mapstructure:"model" yaml:"model,omitempty"`
	Temperature       float64  `mapstructure:"temperature" yaml:"temperature,omitempty"`
	MaxTokens         int      `mapstructure:"max_tokens" yaml:"max_tokens,omitempty"`
	MaxIterations     int      `mapstructure:"max_iterations" yaml:"max_iterations,omitempty"`           // 每次用户消息，LLM 最多连续调工具次数（默认 10）
	Tools             []string `mapstructure:"tools" yaml:"tools,omitempty"`                             // 允许的工具白名单（glob 模式），空或["*"]=全部允许
	DisableTools      []string `mapstructure:"disable_tools" yaml:"disable_tools,omitempty"`             // 禁用的工具黑名单（glob 模式），覆盖 tools
	AllowToolOverride bool     `mapstructure:"allow_tool_override" yaml:"allow_tool_override,omitempty"` // true=LLM参数可覆盖配置, false=配置强制覆盖LLM（默认false,省钱）
}

// ImageDefaults holds default values for image generation.
type ImageDefaults struct {
	Provider          string   `mapstructure:"provider" yaml:"provider,omitempty"`
	Model             string   `mapstructure:"model" yaml:"model,omitempty"`
	Size              string   `mapstructure:"size" yaml:"size,omitempty"`
	Resolution        string   `mapstructure:"resolution" yaml:"resolution,omitempty"`
	Quality           string   `mapstructure:"quality" yaml:"quality,omitempty"`
	Background        string   `mapstructure:"background" yaml:"background,omitempty"`
	Moderation        string   `mapstructure:"moderation" yaml:"moderation,omitempty"`
	OutputFormat      string   `mapstructure:"output_format" yaml:"output_format,omitempty"`
	OutputCompression *int     `mapstructure:"output_compression" yaml:"output_compression,omitempty"`
	Compress          string   `mapstructure:"compress" yaml:"compress,omitempty"`
	N                 *int     `mapstructure:"n" yaml:"n,omitempty"`
	ImageURLs         []string `mapstructure:"image_urls" yaml:"image_urls,omitempty"`
	MaskURL           string   `mapstructure:"mask_url" yaml:"mask_url,omitempty"`
	Style             string   `mapstructure:"style" yaml:"style,omitempty"`
	ResponseFormat    string   `mapstructure:"response_format" yaml:"response_format,omitempty"`
	Timeout           *int     `mapstructure:"timeout" yaml:"timeout,omitempty"`
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
	Provider   string   `mapstructure:"provider" yaml:"provider,omitempty"`
	Model      string   `mapstructure:"model" yaml:"model,omitempty"`
	Size       string   `mapstructure:"size" yaml:"size,omitempty"`
	Resolution string   `mapstructure:"resolution" yaml:"resolution,omitempty"`
	Duration   *int     `mapstructure:"duration" yaml:"duration,omitempty"`
	ImageURLs  []string `mapstructure:"image_urls" yaml:"image_urls,omitempty"`
	VideoURLs  []string `mapstructure:"video_urls" yaml:"video_urls,omitempty"`
	AudioURLs  []string `mapstructure:"audio_urls" yaml:"audio_urls,omitempty"`
	Timeout    *int     `mapstructure:"timeout" yaml:"timeout,omitempty"`
}

// MidjourneyDefaults holds default values for Midjourney generation.
type MidjourneyDefaults struct {
	Provider string `mapstructure:"provider" yaml:"provider,omitempty"`
	Speed    string `mapstructure:"speed" yaml:"speed,omitempty"`
	Version  string `mapstructure:"version" yaml:"version,omitempty"`
	Style    string `mapstructure:"style" yaml:"style,omitempty"`
	Size     string `mapstructure:"size" yaml:"size,omitempty"`
	Quality  string `mapstructure:"quality" yaml:"quality,omitempty"`
	Niji     *bool  `mapstructure:"niji" yaml:"niji,omitempty"`
	Timeout  *int   `mapstructure:"timeout" yaml:"timeout,omitempty"`
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

// AudioDefaults holds default values for audio generation and transcription.
// Local inference is controlled by the provider type (type=local), not this struct.
type AudioDefaults struct {
	Provider        string `mapstructure:"provider" yaml:"provider,omitempty"`
	SpeakModel      string `mapstructure:"speak_model" yaml:"speak_model,omitempty"`
	TranscribeModel string `mapstructure:"transcribe_model" yaml:"transcribe_model,omitempty"`
	Voice           string `mapstructure:"voice" yaml:"voice,omitempty"`
	Format          string `mapstructure:"format" yaml:"format,omitempty"`
	Timeout         *int   `mapstructure:"timeout" yaml:"timeout,omitempty"`
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
