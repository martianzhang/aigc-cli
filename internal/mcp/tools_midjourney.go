package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/aigc-cli/internal/cli/midjourney"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// toURLs converts a single URL string to a slice (the MJ API expects []string).
func toURLs(url string) []string {
	if url == "" {
		return nil
	}
	return []string{url}
}

// ----- Tool definitions -----

func newMidjourneyImagineTool(desc string) mcp.Tool {
	return mcp.NewTool("midjourney_imagine",
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(desc),
		mcp.WithString("prompt",
			mcp.Required(),
			mcp.Description("Text description of the image"),
		),
		mcp.WithString("image_url",
			mcp.Description("Reference image URL for image-guided generation"),
		),
		mcp.WithString("aspect_ratio",
			mcp.Enum("1:1", "16:9", "9:16", "4:3", "3:4", "21:9"),
			mcp.Description("Output aspect ratio"),
		),
		mcp.WithString("style",
			mcp.Enum("raw", "expressive"),
			mcp.Description("Midjourney style mode"),
		),
		mcp.WithString("version",
			mcp.Enum("6.1", "7", "8", "8.1"),
			mcp.Description("Midjourney model version"),
		),
		mcp.WithString("speed",
			mcp.Enum("relax", "fast", "turbo"),
			mcp.Description("Generation speed mode"),
		),
		mcp.WithString("provider",
			mcp.Description(providerArgDesc),
		),
	)
}

func newMidjourneyDescribeTool(desc string) mcp.Tool {
	return mcp.NewTool("midjourney_describe",
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(desc),
		mcp.WithString("image_url",
			mcp.Required(),
			mcp.Description("URL of the image to describe"),
		),
		mcp.WithString("provider",
			mcp.Description(providerArgDesc),
		),
	)
}

func newMidjourneyRerollTool(desc string) mcp.Tool {
	return mcp.NewTool("midjourney_reroll",
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(desc),
		mcp.WithString("task_id",
			mcp.Required(),
			mcp.Description("Previous MJ task ID to reroll"),
		),
		mcp.WithString("provider",
			mcp.Description(providerArgDesc),
		),
	)
}

func newMidjourneyVideoTool(desc string) mcp.Tool {
	return mcp.NewTool("midjourney_video",
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(desc),
		mcp.WithString("image_url",
			mcp.Required(),
			mcp.Description("URL of the image to animate"),
		),
		mcp.WithString("prompt",
			mcp.Description("Optional text description"),
		),
		mcp.WithString("provider",
			mcp.Description(providerArgDesc),
		),
	)
}

// ----- Handlers -----

// midjourneyClient resolves the MJ provider from config and builds a client.
// providerRef is the optional per-call provider name; empty selects the
// configured default. Unlike the singleton midjourney.NewClient(), it never
// reads global CLI state.
func midjourneyClient(cfg *Config, providerRef string) (*client.Client, *mcp.CallToolResult) {
	p, errMsg := cfg.resolveProviderRef(options.ProviderNameMidjourney, providerRef)
	if errMsg != "" {
		return nil, mcp.NewToolResultError(errMsg)
	}
	if p.RequiresAPIKey() {
		return nil, mcp.NewToolResultError("API Key not configured")
	}
	return client.NewFromProvider(p), nil
}

// midjourneyImagineHandler handles the midjourney_imagine tool call.
func midjourneyImagineHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, errResult := midjourneyClient(cfg, request.GetString("provider", ""))
		if errResult != nil {
			return errResult, nil
		}
		prompt, err := request.RequireString("prompt")
		if err != nil {
			return mcp.NewToolResultError("prompt is required"), nil
		}

		req := &types.MJImagineRequest{
			Prompt:    prompt,
			ImageURLs: toURLs(request.GetString("image_url", "")),
			Size:      request.GetString("aspect_ratio", ""),
			Style:     request.GetString("style", ""),
			Version:   request.GetString("version", ""),
			Speed:     request.GetString("speed", ""),
		}
		// Merge config defaults
		if mj := options.MidjourneyDefaults(); mj != nil {
			mj.MergeIntoImagine(req)
		}

		text, err := midjourney.SubmitAndGetText(c, "imagine", req)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("midjourney imagine failed: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	}
}

// midjourneyDescribeHandler handles the midjourney_describe tool call.
func midjourneyDescribeHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, errResult := midjourneyClient(cfg, request.GetString("provider", ""))
		if errResult != nil {
			return errResult, nil
		}
		imageURL, err := request.RequireString("image_url")
		if err != nil {
			return mcp.NewToolResultError("image_url is required"), nil
		}

		req := &types.MJDescribeRequest{ImageURLs: toURLs(imageURL)}
		text, err := midjourney.SubmitAndGetText(c, "describe", req)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("midjourney describe failed: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	}
}

// midjourneyRerollHandler handles the midjourney_reroll tool call.
func midjourneyRerollHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, errResult := midjourneyClient(cfg, request.GetString("provider", ""))
		if errResult != nil {
			return errResult, nil
		}
		taskID, err := request.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}

		req := &types.MJRerollRequest{TaskID: taskID}
		text, err := midjourney.SubmitAndGetText(c, "reroll", req)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("midjourney reroll failed: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	}
}

// midjourneyVideoHandler handles the midjourney_video tool call.
func midjourneyVideoHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, errResult := midjourneyClient(cfg, request.GetString("provider", ""))
		if errResult != nil {
			return errResult, nil
		}
		imageURL, err := request.RequireString("image_url")
		if err != nil {
			return mcp.NewToolResultError("image_url is required"), nil
		}

		req := &types.MJVideoRequest{
			ImageURLs: toURLs(imageURL),
			Prompt:    request.GetString("prompt", ""),
		}
		text, err := midjourney.SubmitAndGetText(c, "video", req)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("midjourney video failed: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	}
}
