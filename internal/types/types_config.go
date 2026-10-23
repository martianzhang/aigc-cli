package types

// ProviderType represents the API protocol type for a named provider.
type ProviderType string

const (
	ProviderOpenAI                ProviderType = "openai"                  // OpenAI-compatible /chat/completions (default)
	ProviderOpenAIChatCompletions ProviderType = "openai_chat_completions" // Explicit alias of openai: classic /chat/completions
	ProviderOpenAIResponses       ProviderType = "openai_responses"        // OpenAI Responses API (/v1/responses)
	ProviderOllama                ProviderType = "ollama"                  // Ollama local (OpenAI subset, no API key)
	ProviderGoogle                ProviderType = "google"                  // Google Gemini API (reserved)
	ProviderGemini                ProviderType = "gemini"                  // Google Gemini native API
	ProviderAnthropic             ProviderType = "anthropic"               // Anthropic Messages API
	ProviderLocal                 ProviderType = "local"                   // Local ONNX models (ocr/vision/detect/background)
)

// DefaultAPIBaseURL is the built-in fallback base URL (bare host, no version
// suffix) used when no base_url is configured anywhere. Callers that need the
// versioned API root append "/v1".
const DefaultAPIBaseURL = "https://api.apimart.ai"

// DetectProvider returns true if this provider type supports upstream provider
// detection from the base URL (e.g., APIMart, OpenRouter). Types with their own
// wire protocol (google, local) return false — their base URL is not analyzed.
func (pt ProviderType) DetectProvider() bool {
	switch pt {
	case ProviderOpenAI, ProviderOpenAIChatCompletions, ProviderOpenAIResponses,
		ProviderOllama, ProviderAnthropic, ProviderGemini:
		return true
	default:
		return false
	}
}

// DisplayType returns the human-readable type label for display purposes.
// For default/unspecified types (empty or "openai"), falls back to the detected
// provider name (e.g., "OpenRouter", "APIMart").
func (pt ProviderType) DisplayType(detected string) string {
	if pt == "" || pt == ProviderOpenAI {
		return detected
	}
	return string(pt)
}

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
	// ZDR opts this provider into zero-data-retention requests. Nil inherits
	// the global zdr; only OpenRouter chat/music honor it today.
	ZDR *bool `mapstructure:"zdr" yaml:"zdr,omitempty"`
}

type Config struct {
	// — Global fallback values (used when no named provider is referenced) —
	APIKey    string `mapstructure:"api_key" yaml:"api_key,omitempty"`
	BaseURL   string `mapstructure:"base_url" yaml:"base_url,omitempty"`
	HTTPProxy string `mapstructure:"http_proxy" yaml:"http_proxy,omitempty"`
	// — Named provider registry (user-defined, referenced by defaults.{cmd}.provider) —
	Providers map[string]*NamedProvider `mapstructure:"providers" yaml:"providers,omitempty"`

	Verbose      bool                          `mapstructure:"verbose" yaml:"verbose"`
	ZDR          bool                          `mapstructure:"zdr" yaml:"zdr"`
	SavePrompt   bool                          `mapstructure:"save_prompt" yaml:"save_prompt"`
	Mode         string                        `mapstructure:"mode" yaml:"mode,omitempty"`
	OutputDir    string                        `mapstructure:"output_dir" yaml:"output_dir,omitempty"`
	Timeout      *int                          `mapstructure:"timeout" yaml:"timeout,omitempty"`
	Defaults     *ConfigDefaults               `mapstructure:"defaults" yaml:"defaults,omitempty"`
	Detect       *DetectConfig                 `mapstructure:"detect" yaml:"detect,omitempty"`
	Background   *BackgroundConfig             `mapstructure:"background" yaml:"background,omitempty"`
	WebSearch    map[string]*WebSearchProvider `mapstructure:"web_search" yaml:"web_search,omitempty"`
	ToolsEnable  []string                      `mapstructure:"tools_enable" yaml:"tools_enable,omitempty"`
	ToolsDisable []string                      `mapstructure:"tools_disable" yaml:"tools_disable,omitempty"`
	MCP          *MCPConfig                    `mapstructure:"mcp" yaml:"mcp,omitempty"`
}

// MCPConfig holds external MCP (Model Context Protocol) servers that the chat
// agent connects to as a client. There is deliberately no built-in server: to
// use aigc-cli's own tools from chat, add an entry pointing at `aigc-cli mcp`.
type MCPConfig struct {
	Servers []MCPServer `mapstructure:"servers" yaml:"servers,omitempty"`
}

// MCPServer describes one MCP server the chat agent launches over stdio.
type MCPServer struct {
	// Name identifies the server in logs and error messages.
	Name string `mapstructure:"name" yaml:"name,omitempty"`
	// Command is the executable to launch (e.g. "aigc-cli" or "npx").
	Command string `mapstructure:"command" yaml:"command,omitempty"`
	// Args are the command arguments (e.g. ["mcp"]).
	Args []string `mapstructure:"args" yaml:"args,omitempty"`
	// Env adds or overrides environment variables ("KEY=VALUE"); the parent
	// process environment is always inherited.
	Env []string `mapstructure:"env" yaml:"env,omitempty"`
	// ToolsEnable is a glob allowlist of tools exposed to the agent. Empty=all.
	ToolsEnable []string `mapstructure:"tools_enable" yaml:"tools_enable,omitempty"`
	// ToolsDisable is a glob denylist; it wins over ToolsEnable.
	ToolsDisable []string `mapstructure:"tools_disable" yaml:"tools_disable,omitempty"`
	// Enabled defaults to true when nil.
	Enabled *bool `mapstructure:"enabled" yaml:"enabled,omitempty"`
}

// IsEnabled reports whether the server should be connected (default true).
func (s MCPServer) IsEnabled() bool { return s.Enabled == nil || *s.Enabled }

// IdeasConfig controls the ideas prompt data path and embedding backend.
type IdeasConfig struct {
	DataPath          string `mapstructure:"data_path" yaml:"data_path,omitempty"`
	EmbeddingProvider string `mapstructure:"embedding_provider" yaml:"embedding_provider,omitempty"`
	EmbeddingModel    string `mapstructure:"embedding_model" yaml:"embedding_model,omitempty"`
	// EmbeddingTimeout is the per-request timeout in seconds (default 180).
	EmbeddingTimeout *int `mapstructure:"embedding_timeout" yaml:"embedding_timeout,omitempty"`
	// EmbeddingMaxRunes truncates the text sent to the embedder (default 256;
	// <=0 disables truncation). Shorter text builds the cache faster.
	EmbeddingMaxRunes *int `mapstructure:"embedding_max_runes" yaml:"embedding_max_runes,omitempty"`
	// SemanticTopK is how many entries the semantic list contributes to RRF
	// (default 200; <=0 uses all).
	SemanticTopK *int `mapstructure:"semantic_top_k" yaml:"semantic_top_k,omitempty"`
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
	Image         *ImageDefaults      `mapstructure:"image" yaml:"image"`
	Video         *VideoDefaults      `mapstructure:"video" yaml:"video"`
	Midjourney    *MidjourneyDefaults `mapstructure:"midjourney" yaml:"midjourney"`
	Music         *MusicDefaults      `mapstructure:"music" yaml:"music,omitempty"`
	Chat          *ChatDefaults       `mapstructure:"chat" yaml:"chat"`
	Audio         *AudioDefaults      `mapstructure:"audio" yaml:"audio"`
	OCR           *OCRDefaults        `mapstructure:"ocr" yaml:"ocr,omitempty"`
	Upscale       *UpscaleDefaults    `mapstructure:"upscale" yaml:"upscale,omitempty"`
	Vision        *VisionDefaults     `mapstructure:"vision" yaml:"vision,omitempty"`
	Knowledgebase *KBDefaults         `mapstructure:"knowledgebase" yaml:"knowledgebase,omitempty"`
	Search        *SearchDefaults     `mapstructure:"search" yaml:"search,omitempty"`
	Ideas         *IdeasConfig        `mapstructure:"ideas" yaml:"ideas,omitempty"`
	Decision      *DecisionDefaults   `mapstructure:"decision" yaml:"decision,omitempty"`
}

// SearchDefaults holds the shared web-search vendor selection used by
// `aigc-cli search`, `kb search`, chat's web_search/web_fetch primitives and
// the MCP tools.
type SearchDefaults struct {
	Provider string `mapstructure:"provider" yaml:"provider,omitempty"`
}

// KBDefaults holds default values for the knowledge base.
type KBDefaults struct {
	BaseDir           string            `mapstructure:"base_dir" yaml:"base_dir,omitempty"`
	MinScore          *float64          `mapstructure:"min_score" yaml:"min_score,omitempty"`
	AutoSave          *bool             `mapstructure:"auto_save" yaml:"auto_save,omitempty"` // nil=true (save to KB)
	Loaders           map[string]string `mapstructure:"loaders" yaml:"loaders,omitempty"`
	EmbeddingProvider string            `mapstructure:"embedding_provider" yaml:"embedding_provider,omitempty"`
	EmbeddingModel    string            `mapstructure:"embedding_model" yaml:"embedding_model,omitempty"`
	// EmbeddingTimeout is the per-request timeout in seconds (default 180).
	EmbeddingTimeout *int `mapstructure:"embedding_timeout" yaml:"embedding_timeout,omitempty"`
}

// WebSearchProvider defines a web search engine configuration.
type WebSearchProvider struct {
	Type   string `mapstructure:"type" yaml:"type,omitempty"`
	APIKey string `mapstructure:"api_key" yaml:"api_key,omitempty"`
}

// OCRDefaults holds default values for OCR scanning.
type OCRDefaults struct {
	Provider string `mapstructure:"provider" yaml:"provider,omitempty"`
	Model    string `mapstructure:"model" yaml:"model,omitempty"`
}

// UpscaleDefaults holds default values for image upscaling.
type UpscaleDefaults struct {
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
	ContextSize       int      `mapstructure:"context_size" yaml:"context_size,omitempty"`               // 输入上下文窗口大小（token），0=使用模型默认值。超过时自动压缩对话
	MaxIterations     int      `mapstructure:"max_iterations" yaml:"max_iterations,omitempty"`           // 每次用户消息，LLM 最多连续调工具次数（默认 10）
	Tools             []string `mapstructure:"tools" yaml:"tools,omitempty"`                             // 允许的工具白名单（glob 模式），空或["*"]=全部允许
	DisableTools      []string `mapstructure:"disable_tools" yaml:"disable_tools,omitempty"`             // 禁用的工具黑名单（glob 模式），覆盖 tools
	AllowToolOverride bool     `mapstructure:"allow_tool_override" yaml:"allow_tool_override,omitempty"` // true=LLM参数可覆盖配置, false=配置强制覆盖LLM（默认false,省钱）
	ToolsOnDemand     bool     `mapstructure:"tools_on_demand" yaml:"tools_on_demand,omitempty"`         // true=MCP 工具按需加载（search_tools/load_tools），减少上下文占用
	ReadDeny          []string `mapstructure:"read_deny" yaml:"read_deny,omitempty"`                     // read 原语的拒绝正则（匹配文件路径或文件名）；空=全部放行
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

// DecisionDefaults holds default values for the decision command.
type DecisionDefaults struct {
	Provider string `mapstructure:"provider" yaml:"provider,omitempty"`
	Model    string `mapstructure:"model" yaml:"model,omitempty"`
	Bank     string `mapstructure:"bank" yaml:"bank,omitempty"`
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

// AudioDefaults holds default values for audio generation, transcription and
// sound effects. Local inference is controlled by the provider type
// (type=local), not this struct.
//
// The nested speak/transcribe/sfx sections are the only supported layout.
type AudioDefaults struct {
	Timeout    *int                     `mapstructure:"timeout" yaml:"timeout,omitempty"`
	Speak      *AudioSpeakDefaults      `mapstructure:"speak" yaml:"speak,omitempty"`
	Transcribe *AudioTranscribeDefaults `mapstructure:"transcribe" yaml:"transcribe,omitempty"`
	SFX        *AudioSFXDefaults        `mapstructure:"sfx" yaml:"sfx,omitempty"`
}

// AudioSpeakDefaults holds defaults for `audio speak`.
type AudioSpeakDefaults struct {
	Provider string `mapstructure:"provider" yaml:"provider,omitempty"`
	Model    string `mapstructure:"model" yaml:"model,omitempty"`
	Voice    string `mapstructure:"voice" yaml:"voice,omitempty"`
	Format   string `mapstructure:"format" yaml:"format,omitempty"`
}

// AudioTranscribeDefaults holds defaults for `audio transcribe`.
type AudioTranscribeDefaults struct {
	Provider string `mapstructure:"provider" yaml:"provider,omitempty"`
	Model    string `mapstructure:"model" yaml:"model,omitempty"`
	Language string `mapstructure:"language" yaml:"language,omitempty"`
}

// AudioSFXDefaults holds defaults for `audio sfx`.
type AudioSFXDefaults struct {
	Provider string `mapstructure:"provider" yaml:"provider,omitempty"`
	Model    string `mapstructure:"model" yaml:"model,omitempty"`
	Format   string `mapstructure:"format" yaml:"format,omitempty"`
}

// SpeakModel returns the effective speak model.
func (d *AudioDefaults) SpeakModel() string {
	if d == nil {
		return ""
	}
	if d.Speak == nil {
		return ""
	}
	return d.Speak.Model
}

// SpeakProvider returns the configured speak provider (empty = global).
func (d *AudioDefaults) SpeakProvider() string {
	if d == nil || d.Speak == nil {
		return ""
	}
	return d.Speak.Provider
}

// SpeakVoice returns the effective speak voice.
func (d *AudioDefaults) SpeakVoice() string {
	if d == nil {
		return ""
	}
	if d.Speak == nil {
		return ""
	}
	return d.Speak.Voice
}

// SpeakFormat returns the effective speak format.
func (d *AudioDefaults) SpeakFormat() string {
	if d == nil {
		return ""
	}
	if d.Speak == nil {
		return ""
	}
	return d.Speak.Format
}

// TranscribeModel returns the effective transcription model.
func (d *AudioDefaults) TranscribeModel() string {
	if d == nil {
		return ""
	}
	if d.Transcribe == nil {
		return ""
	}
	return d.Transcribe.Model
}

// TranscribeProvider returns the configured transcription provider (empty = global).
func (d *AudioDefaults) TranscribeProvider() string {
	if d == nil || d.Transcribe == nil {
		return ""
	}
	return d.Transcribe.Provider
}

// TranscribeLanguage returns the default transcription language hint.
func (d *AudioDefaults) TranscribeLanguage() string {
	if d == nil || d.Transcribe == nil {
		return ""
	}
	return d.Transcribe.Language
}

// SFXModel returns the effective sound-effect model.
func (d *AudioDefaults) SFXModel() string {
	if d == nil || d.SFX == nil {
		return ""
	}
	return d.SFX.Model
}

// SFXProvider returns the configured sound-effect provider (empty = global).
func (d *AudioDefaults) SFXProvider() string {
	if d == nil || d.SFX == nil {
		return ""
	}
	return d.SFX.Provider
}

// SFXFormat returns the effective sound-effect format.
func (d *AudioDefaults) SFXFormat() string {
	if d == nil || d.SFX == nil {
		return ""
	}
	return d.SFX.Format
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
