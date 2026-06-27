// Package mcp implements an MCP (Model Context Protocol) server.
// Supports APIMart and OpenRouter providers (auto-detected from base_url).
package mcp

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/apimart-cli/internal/provider"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// Config holds the configuration for the MCP server.
// It's a subset of the full CLI config, focused on what MCP tools need.
type Config struct {
	APIKey   string
	BaseURL  string
	Proxy    string
	Output   string
	Defaults *types.ConfigDefaults
}

// buildImageDesc builds the generate_image tool description with config defaults injected.
func buildImageDesc(d *types.ImageDefaults, baseURL string) string {
	b := new(strings.Builder)
	p := provider.Detect(baseURL)
	b.WriteString(fmt.Sprintf("Generate images via %s.\n\n当前配置（在 ~/.config/apimart/config.yaml 中修改）:\n", p))
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
	b.WriteString(fmt.Sprintf("Generate videos via %s (async submit → poll).\n\n当前配置（在 ~/.config/apimart/config.yaml 中修改）:\n", p))
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

// NewServer creates and configures an MCP server with all APIMart tools.
// Handlers are implemented as closures capturing the config.
func NewServer(cfg *Config) *server.MCPServer {
	s := server.NewMCPServer(
		"apimart-cli",
		"0.1.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)

	// Build descriptions with config defaults
	imgDesc := buildImageDesc(cfg.Defaults.Image, cfg.BaseURL)
	videoDesc := buildVideoDesc(cfg.Defaults.Video, cfg.BaseURL)

	// Register tools with config captured via closures
	s.AddTool(newGenerateImageTool(imgDesc), generateImageHandler(cfg))
	s.AddTool(newGenerateVideoTool(videoDesc), generateVideoHandler(cfg))
	s.AddTool(newListModelsTool(), listModelsHandler())
	s.AddTool(newGetModelPricingTool(), getModelPricingHandler())
	s.AddTool(newGetBalanceTool(), getBalanceHandler(cfg))
	s.AddTool(newGetTaskTool(), getTaskHandler(cfg))

	return s
}

// Run starts the MCP server with stdio transport.
func Run(cfg *Config) error {
	s := NewServer(cfg)
	return server.ServeStdio(s)
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
