// Package mcp implements an MCP (Model Context Protocol) server.
// Supports APIMart and OpenRouter providers (auto-detected from base_url).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/aigc-cli/internal/ideas"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// Config holds the configuration for the MCP server.
// It's a subset of the full CLI config, focused on what MCP tools need.
type Config struct {
	APIKey       string
	BaseURL      string
	Proxy        string
	Output       string
	Defaults     *types.ConfigDefaults
	ToolsEnable  []string
	ToolsDisable []string
	ListTools    bool
	// CmdProviders holds pre-resolved providers for each command,
	// set by cmd/mcp.go at startup.
	CmdProviders map[string]*provider.EffectiveProvider
}

// cmdProvider returns the effective provider for a command.
// Falls back to global cfg fields if not pre-resolved.
func (cfg *Config) cmdProvider(name string) *provider.EffectiveProvider {
	if cfg.CmdProviders != nil {
		if p, ok := cfg.CmdProviders[name]; ok {
			return p
		}
	}
	return &provider.EffectiveProvider{
		APIKey:       cfg.APIKey,
		BaseURL:      cfg.BaseURL,
		HTTPProxy:    cfg.Proxy,
		ProviderType: provider.Detect(cfg.BaseURL),
	}
}

// buildImageDesc builds the generate_image tool description with config defaults injected.
func buildImageDesc(d *types.ImageDefaults, baseURL string) string {
	b := new(strings.Builder)
	p := provider.Detect(baseURL)
	fmt.Fprintf(b, "Generate images via %s.\n\n当前配置（在 ~/.config/aigc-cli/config.yaml 中修改）:\n", p)
	if d != nil {
		fmt.Fprintf(b, "  model = %s | size = %s | resolution = %s\n", d.Model, d.Size, d.Resolution)
		fmt.Fprintf(b, "  quality = %s | output_format = %s", d.Quality, d.OutputFormat)
		if d.N != nil {
			fmt.Fprintf(b, " | n = %d", *d.N)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n策略: 参数已设好默认值，不要主动填写。只有在用户提示词中明确指定了某个参数时（如 \"用 4k 分辨率\"），才传入对应参数覆盖。")
	return b.String()
}

// buildVideoDesc builds the generate_video tool description with config defaults injected.
func buildVideoDesc(d *types.VideoDefaults, baseURL string) string {
	b := new(strings.Builder)
	p := provider.Detect(baseURL)
	fmt.Fprintf(b, "Generate videos via %s (async submit → poll).\n\n当前配置（在 ~/.config/aigc-cli/config.yaml 中修改）:\n", p)
	if d != nil {
		fmt.Fprintf(b, "  model = %s", d.Model)
		if d.Size != "" {
			fmt.Fprintf(b, " | size = %s", d.Size)
		}
		if d.Resolution != "" {
			fmt.Fprintf(b, " | resolution = %s", d.Resolution)
		}
		if d.Duration != nil {
			fmt.Fprintf(b, " | duration = %ds", *d.Duration)
		}
		b.WriteString("\n")
	}
	if provider.Detect(baseURL) == provider.OpenRouter {
		b.WriteString("\n注意: OpenRouter 视频生成是异步的，提交后返回 Job ID + polling_url。稍后使用 get_task 工具传入 Job ID 查询。")
	} else {
		b.WriteString("\n策略: 参数已设好默认值，不要主动填写。只有在用户提示词中明确指定了某个参数时，才传入对应参数覆盖。\n注意: 视频生成是异步的，提交后立即返回 task_id，请使用 get_task 工具查询结果。")
	}
	return b.String()
}

// buildAudioDesc builds the generate_speech tool description with config defaults injected.
func buildAudioDesc(d *types.AudioDefaults, baseURL string) string {
	b := new(strings.Builder)
	p := provider.Detect(baseURL)
	fmt.Fprintf(b, "Generate speech audio from text via %s.\n\n当前配置（在 ~/.config/aigc-cli/config.yaml 中修改）:\n", p)
	if d != nil {
		fmt.Fprintf(b, "  speak_model = %s | transcribe_model = %s | voice = %s | format = %s\n", d.SpeakModel, d.TranscribeModel, d.Voice, d.Format)
	}
	b.WriteString("\n策略: 参数已设好默认值，不要主动填写。只有在用户提示词中明确指定了某个参数时，才传入对应参数覆盖。")
	return b.String()
}

type toolInfo struct {
	name    string
	desc    string
	newTool func(desc string) mcp.Tool
	handler func(cfg *Config) server.ToolHandlerFunc
}

var toolRegistry = []toolInfo{
	{"generate_image", "Generate images via AI", newGenerateImageTool, func(cfg *Config) server.ToolHandlerFunc { return generateImageHandler(cfg) }},
	{"generate_video", "Generate videos via AI (async submit → poll)", newGenerateVideoTool, func(cfg *Config) server.ToolHandlerFunc { return generateVideoHandler(cfg) }},
	{"generate_speech", "Convert text to speech (TTS)", newGenerateSpeechTool, func(cfg *Config) server.ToolHandlerFunc { return generateSpeechHandler(cfg) }},
	{"transcribe_audio", "Transcribe audio to text (STT)", newTranscribeAudioTool, func(cfg *Config) server.ToolHandlerFunc { return transcribeAudioHandler(cfg) }},
	{"list_models", "List available models", func(desc string) mcp.Tool { return newListModelsTool() }, func(cfg *Config) server.ToolHandlerFunc { return listModelsHandler() }},
	{"get_model_pricing", "Query model pricing details", func(desc string) mcp.Tool { return newGetModelPricingTool() }, func(cfg *Config) server.ToolHandlerFunc { return getModelPricingHandler() }},
	{"get_balance", "Query API key or account balance", func(desc string) mcp.Tool { return newGetBalanceTool() }, func(cfg *Config) server.ToolHandlerFunc { return getBalanceHandler(cfg) }},
	{"get_task", "Query async task/job status", func(desc string) mcp.Tool { return newGetTaskTool() }, func(cfg *Config) server.ToolHandlerFunc { return getTaskHandler(cfg) }},
	{"describe_image", "Read or write image caption", func(desc string) mcp.Tool { return newDescribeImageTool() }, func(cfg *Config) server.ToolHandlerFunc { return describeImageHandler() }},
	{"search_ideas", "Search AI prompt ideas", func(desc string) mcp.Tool { return newSearchIdeasTool() }, func(cfg *Config) server.ToolHandlerFunc { return searchIdeasHandler() }},
	{"remove_background", "Remove image background (offline)", func(desc string) mcp.Tool { return newRemoveBackgroundTool() }, func(cfg *Config) server.ToolHandlerFunc { return removeBackgroundHandler() }},
	{"detect_image", "Detect AIGC/watermark in images (offline)", func(desc string) mcp.Tool { return newDetectTool() }, func(cfg *Config) server.ToolHandlerFunc { return detectHandler() }},
	{"remove_watermark", "Remove visible AI watermark", func(desc string) mcp.Tool { return newRemoveWatermarkTool() }, func(cfg *Config) server.ToolHandlerFunc { return removeWatermarkHandler() }},
	{"add_watermark", "Add visible AI watermark (test only)", func(desc string) mcp.Tool { return newAddWatermarkTool() }, func(cfg *Config) server.ToolHandlerFunc { return addWatermarkHandler() }},
	{"ocr_text", "Recognize text in images/PDF using offline OCR", func(desc string) mcp.Tool { return newOCRTextTool() }, func(cfg *Config) server.ToolHandlerFunc { return ocrTextHandler() }},
}

// NewServer creates and configures an MCP server, registering tools based on config.
func NewServer(cfg *Config) *server.MCPServer {
	s := server.NewMCPServer(
		"aigc-cli",
		"0.1.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)

	imgDesc := buildImageDesc(cfg.Defaults.Image, cfg.BaseURL)
	videoDesc := buildVideoDesc(cfg.Defaults.Video, cfg.BaseURL)
	audioDesc := buildAudioDesc(cfg.Defaults.Audio, cfg.BaseURL)
	toolDescriptions := map[string]string{
		"generate_image":   imgDesc,
		"generate_video":   videoDesc,
		"generate_speech":  audioDesc,
		"transcribe_audio": audioDesc,
	}

	enableList := cfg.ToolsEnable
	disableList := cfg.ToolsDisable
	for _, info := range toolRegistry {
		if !isToolAllowed(info.name, enableList, disableList) {
			continue
		}
		desc := toolDescriptions[info.name]
		if desc == "" {
			desc = info.desc
		}
		s.AddTool(info.newTool(desc), info.handler(cfg))
	}

	return s
}

func isToolAllowed(name string, enable, disable []string) bool {
	if len(enable) > 0 && !matchAny(name, enable) {
		return false
	}
	if matchAny(name, disable) {
		return false
	}
	return true
}

func matchAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if matched, _ := path.Match(p, name); matched {
			return true
		}
	}
	return false
}

// Run starts the MCP server with stdio transport.
func Run(cfg *Config) error {
	if cfg.ListTools {
		ListTools(cfg)
		return nil
	}
	s := NewServer(cfg)
	return server.ServeStdio(s)
}

// ListTools prints all registered MCP tools and exits.
func ListTools(cfg *Config) {
	fmt.Println("Available MCP tools:")
	for _, info := range toolRegistry {
		if !isToolAllowed(info.name, cfg.ToolsEnable, cfg.ToolsDisable) {
			continue
		}
		fmt.Printf("  %-24s  %s\n", info.name, info.desc)
	}

	if len(cfg.ToolsEnable) > 0 {
		fmt.Printf("\n  tools_enable:  %v\n", cfg.ToolsEnable)
	}
	if len(cfg.ToolsDisable) > 0 {
		fmt.Printf("  tools_disable: %v\n", cfg.ToolsDisable)
	}
}

// ----- Tool definitions -----

func newGenerateImageTool(desc string) mcp.Tool {
	t := mcp.NewTool("generate_image",
		mcp.WithDescription(desc),
		mcp.WithString("prompt",
			mcp.Required(),
			mcp.Description("Image description / prompt"),
		),
		mcp.WithString("model",
			mcp.Description("Override the config default model"),
		),
		mcp.WithString("size",
			mcp.Description("Override the config default size/aspect ratio"),
		),
		mcp.WithString("resolution",
			mcp.Enum("1k", "2k", "4k"),
			mcp.Description("Override the config default resolution"),
		),
		mcp.WithString("quality",
			mcp.Enum("auto", "low", "medium", "high"),
			mcp.Description("Override the config default quality"),
		),
		mcp.WithString("output_format",
			mcp.Enum("png", "jpeg", "webp"),
			mcp.Description("Override the config default output format"),
		),
		mcp.WithString("image_urls",
			mcp.Description("Reference image URLs (comma-separated) for image-to-image"),
		),
		mcp.WithString("mask_url",
			mcp.Description("Mask image URL for inpainting"),
		),
		mcp.WithString("background",
			mcp.Description("Background mode: auto, opaque, transparent"),
		),
	)
	return t
}

func newGenerateVideoTool(desc string) mcp.Tool {
	t := mcp.NewTool("generate_video",
		mcp.WithDescription(desc),
		mcp.WithString("prompt",
			mcp.Required(),
			mcp.Description("Video content description"),
		),
		mcp.WithString("model",
			mcp.Description("Override the config default model"),
		),
		mcp.WithInteger("duration",
			mcp.Description("Duration in seconds (4-15), override config default"),
		),
		mcp.WithString("size",
			mcp.Description("Override the config default aspect ratio"),
		),
		mcp.WithString("resolution",
			mcp.Enum("480p", "720p", "1080p"),
			mcp.Description("Override the config default resolution"),
		),
		mcp.WithString("image_urls",
			mcp.Description("Reference image URLs (comma-separated)"),
		),
		mcp.WithString("video_urls",
			mcp.Description("Reference video URLs (comma-separated)"),
		),
		mcp.WithBoolean("generate_audio",
			mcp.Description("Generate AI audio for the video"),
		),
	)
	return t
}

func newGenerateSpeechTool(desc string) mcp.Tool {
	t := mcp.NewTool("generate_speech",
		mcp.WithDescription(desc),
		mcp.WithString("input",
			mcp.Required(),
			mcp.Description("Text to convert to speech"),
		),
		mcp.WithString("model",
			mcp.Description("TTS model (e.g. openai/gpt-4o-mini-tts)"),
		),
		mcp.WithString("voice",
			mcp.Required(),
			mcp.Description("Voice name (e.g. alloy, nova, echo, fable)"),
		),
		mcp.WithString("format",
			mcp.Description("Audio format: mp3, wav, opus, aac, flac, pcm (default: mp3)"),
		),
	)
	return t
}

func newTranscribeAudioTool(desc string) mcp.Tool {
	t := mcp.NewTool("transcribe_audio",
		mcp.WithDescription(desc),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path to the audio file to transcribe"),
		),
		mcp.WithString("model",
			mcp.Description("STT model (e.g. openai/whisper-1)"),
		),
		mcp.WithString("language",
			mcp.Description("Language hint (ISO-639-1, e.g. en, ja, zh)"),
		),
	)
	return t
}

func newListModelsTool() mcp.Tool {
	return mcp.NewTool("list_models",
		mcp.WithDescription("列出 APIMart 市场所有可用模型及其类型。无需 API Key。"),
		mcp.WithString("type",
			mcp.Enum("image", "video", "chat"),
			mcp.Description("Filter by model type (optional)"),
		),
	)
}

func newGetModelPricingTool() mcp.Tool {
	return mcp.NewTool("get_model_pricing",
		mcp.WithDescription("查询指定模型的详细定价信息。无需 API Key。"),
		mcp.WithString("model",
			mcp.Required(),
			mcp.Description("Model name, e.g. gpt-image-2-official"),
		),
	)
}

func newGetBalanceTool() mcp.Tool {
	return mcp.NewTool("get_balance",
		mcp.WithDescription("查询余额和用量。同时返回当前 API Key 的余额和用户账号的总余额。"),
	)
}

func newGetTaskTool() mcp.Tool {
	return mcp.NewTool("get_task",
		mcp.WithDescription("查询异步任务的状态和结果。APIMart: 查询 task_id；OpenRouter: 查询 job_id（视频提交后返回的 ID）。轮询直到 status 为 completed。"),
		mcp.WithString("task_id",
			mcp.Required(),
			mcp.Description("APIMart task_id 或 OpenRouter job_id"),
		),
	)
}

func newDetectTool() mcp.Tool {
	return mcp.NewTool("detect_image",
		mcp.WithDescription("检测图片中的 C2PA Content Credentials、SynthID 隐形水印、TC260 AIGC 标签（中国 GB 45438-2025），以及 EXIF 相机元数据。完全离线运行，无需 API Key。支持 PNG、JPEG、WebP、GIF、BMP 格式。"),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("图片文件的本地路径"),
		),
	)
}

// detectHandler handles the detect_image tool call.
func detectHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("file_path")
		if err != nil {
			return mcp.NewToolResultError("file_path is required"), nil
		}

		// Resolve relative paths
		if !filepath.IsAbs(path) {
			abs, err := filepath.Abs(path)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid path: %v", err)), nil
			}
			path = abs
		}

		result, err := service.DetectImage(path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("detection failed: %v", err)), nil
		}

		// Format as JSON for structured output
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("JSON encoding failed: %v", err)), nil
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func newRemoveWatermarkTool() mcp.Tool {
	return mcp.NewTool("remove_watermark",
		mcp.WithDescription("⚠️  检测并移除图片中的可见 AI 水印（内置 gemini，其他需通过 learn-watermark 学习）。\n\n仅用于验证检测算法或合法修复用途（如修复个人旧照片）。\n禁止用于去除他人版权图片的水印。\n\n完全离线运行，无需 API Key。输出为 <原图>_clean<ext>。"),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("待去水印图片的本地路径"),
		),
		mcp.WithString("producer",
			mcp.Description("水印厂商提示（内置 gemini，其他需 learn-watermark 学习）。留空则自动检测（依赖 TC260 元数据或通用检测）"),
		),
		mcp.WithString("output_path",
			mcp.Description("输出路径（可选，默认 <原图>_clean<ext>）"),
		),
	)
}

func newAddWatermarkTool() mcp.Tool {
	return mcp.NewTool("add_watermark",
		mcp.WithDescription("向图片添加可见 AI 水印（仅用于创建去水印算法的测试样本，不注入任何元数据）。gemini 使用注册 alpha map；未知名称按文字渲染。完全离线运行，无需 API Key。输出为 <原图>_watermarked.png。"),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("待加水印图片的本地路径"),
		),
		mcp.WithString("producer",
			mcp.Required(),
			mcp.Description("水印厂商名（内置 gemini，其他需 learn-watermark 学习）或自定义文字"),
		),
		mcp.WithString("output_path",
			mcp.Description("输出路径（可选，默认 <原图>_watermarked.png）"),
		),
	)
}

func newSearchIdeasTool() mcp.Tool {
	return mcp.NewTool("search_ideas",
		mcp.WithDescription("搜索本地灵感库，返回 AI 图片生成提示词及可下载的参考图片。支持多语言关键词搜索——如果用户输入的是中文、日文等非英文，建议先用英文翻译再搜索，同时也用原语言搜索一次，合并结果以获得更全面的匹配。支持随机获取（设置 random=true）。"),
		mcp.WithString("keywords",
			mcp.Description("搜索关键词，支持中文/英文/日文等多语言。建议同时用英文翻译后再搜一次以匹配更多结果。留空则返回随机灵感。"),
		),
		mcp.WithInteger("limit",
			mcp.Description("返回结果数量上限（默认 5）"),
		),
		mcp.WithBoolean("random",
			mcp.Description("设为 true 随机返回灵感，忽略 keywords"),
		),
	)
}

// searchIdeasHandler handles the search_ideas tool call.
func searchIdeasHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("invalid arguments"), nil
		}

		keywords, _ := args["keywords"].(string)
		random, _ := args["random"].(bool)
		limit := 5
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}

		var result string
		var err error
		if random || keywords == "" {
			result, err = ideas.SearchRandom(limit)
		} else {
			result, err = ideas.SearchText(keywords, limit)
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}

func newDescribeImageTool() mcp.Tool {
	return mcp.NewTool("describe_image",
		mcp.WithDescription("Read or write the caption/description of an image file. Provide file_path and optional caption to write. If caption is omitted, reads the current caption. Supports JPEG and PNG."),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path to the image file"),
		),
		mcp.WithString("caption",
			mcp.Description("Caption text to write. Omit to just read. Use empty string to clear."),
		),
	)
}

func describeImageHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("file_path")
		if err != nil {
			return mcp.NewToolResultError("file_path is required"), nil
		}
		if !filepath.IsAbs(path) {
			abs, err := filepath.Abs(path)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid path: %v", err)), nil
			}
			path = abs
		}

		caption := req.GetString("caption", "")

		if _, hasCaption := req.GetArguments()["caption"]; hasCaption {
			if err := service.WriteDescription(path, caption); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("write caption failed: %v", err)), nil
			}
			if caption == "" {
				return mcp.NewToolResultText(fmt.Sprintf("Caption cleared for %s", filepath.Base(path))), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Caption set: %s", caption)), nil
		}

		current, err := service.ReadDescription(path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("read caption failed: %v", err)), nil
		}
		if current == "" {
			return mcp.NewToolResultText(fmt.Sprintf("No caption set for %s", filepath.Base(path))), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Caption: %s", current)), nil
	}
}
