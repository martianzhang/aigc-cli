package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// promptInfo pairs a workflow prompt template with its handler.
type promptInfo struct {
	prompt  mcp.Prompt
	handler server.PromptHandlerFunc
}

var productShotPrompt = mcp.NewPrompt("generate_product_shot",
	mcp.WithPromptTitle("Generate a product photo"),
	mcp.WithPromptDescription("Generate a commercial product photo (studio lighting, clean seamless background) with generate_image"),
	mcp.WithArgument("product",
		mcp.RequiredArgument(),
		mcp.ArgumentDescription("Product name or short description, e.g. \"ceramic coffee mug\""),
	),
	mcp.WithArgument("style",
		mcp.ArgumentDescription("Optional visual style, e.g. minimalist, luxury, tech, warm lifestyle"),
	),
	mcp.WithArgument("aspect_ratio",
		mcp.ArgumentDescription("Optional aspect ratio, e.g. 1:1, 4:5, 16:9"),
	),
)

var detectAIImagePrompt = mcp.NewPrompt("detect_ai_image",
	mcp.WithPromptTitle("Check if an image is AI-generated"),
	mcp.WithPromptDescription("Check whether an image is AI-generated and explain the C2PA / TC260 / SynthID / FFT / ONNX / watermark signals with detect_image"),
	mcp.WithArgument("file_path",
		mcp.RequiredArgument(),
		mcp.ArgumentDescription("Local path to the image file to inspect"),
	),
)

var imageToVideoPrompt = mcp.NewPrompt("image_to_video",
	mcp.WithPromptTitle("Animate a still image"),
	mcp.WithPromptDescription("Animate a still image into a short video with generate_video"),
	mcp.WithArgument("image_url",
		mcp.RequiredArgument(),
		mcp.ArgumentDescription("Image URL (or local path) to animate"),
	),
	mcp.WithArgument("prompt",
		mcp.ArgumentDescription("Optional motion prompt describing the camera move and subject motion"),
	),
)

var removeBackgroundPrompt = mcp.NewPrompt("remove_image_background",
	mcp.WithPromptTitle("Remove an image background"),
	mcp.WithPromptDescription("Remove an image background offline (optionally replace it with a color) using remove_background"),
	mcp.WithArgument("file_path",
		mcp.RequiredArgument(),
		mcp.ArgumentDescription("Local path to the image file to process"),
	),
	mcp.WithArgument("replace_color",
		mcp.ArgumentDescription("Optional hex background color, e.g. #FFFFFF. Omit to keep the background transparent"),
	),
)

var promptRegistry = []promptInfo{
	{productShotPrompt, handleGenerateProductShot},
	{detectAIImagePrompt, handleDetectAIImage},
	{imageToVideoPrompt, handleImageToVideo},
	{removeBackgroundPrompt, handleRemoveImageBackground},
}

// registerPrompts registers all MCP workflow prompt templates.
func registerPrompts(s *server.MCPServer) {
	for _, info := range promptRegistry {
		s.AddPrompt(info.prompt, info.handler)
	}
}

// ListPrompts prints all registered MCP prompts and exits.
func ListPrompts(_ *Config) {
	fmt.Println("Available MCP prompts:")
	for _, info := range promptRegistry {
		fmt.Printf("  %-26s  %s\n", info.prompt.Name, info.prompt.Description)
	}
}

// newPromptResult wraps text in a single user-role prompt message.
func newPromptResult(p mcp.Prompt, text string) *mcp.GetPromptResult {
	return mcp.NewGetPromptResult(p.Description, []mcp.PromptMessage{
		mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text)),
	})
}

// argOr returns the argument value, substituting a <placeholder> when absent.
func argOr(args map[string]string, key string) string {
	if v := strings.TrimSpace(args[key]); v != "" {
		return v
	}
	return "<" + key + ">"
}

// missingArgsNote returns a reminder for required arguments the user did not supply.
func missingArgsNote(args map[string]string, keys ...string) string {
	var missing []string
	for _, key := range keys {
		if strings.TrimSpace(args[key]) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("\nNote: no value was provided for %s. Ask the user for it before calling the tool.\n", strings.Join(missing, ", "))
}

func handleGenerateProductShot(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	product := argOr(args, "product")
	style := strings.TrimSpace(args["style"])
	ratio := strings.TrimSpace(args["aspect_ratio"])

	var b strings.Builder
	fmt.Fprintf(&b, "Create a commercial product photo of %s.\n\n", product)
	b.WriteString("Call the `generate_image` tool with a prompt that specifies:\n")
	b.WriteString("- professional studio lighting (soft key light with a subtle rim light)\n")
	b.WriteString("- a clean, neutral seamless background with no props\n")
	b.WriteString("- sharp focus, high detail, realistic textures, advertising-quality composition\n")
	if style != "" {
		fmt.Fprintf(&b, "- visual style: %s\n", style)
	}
	if ratio != "" {
		fmt.Fprintf(&b, "- aspect ratio: %s (pass it through the tool's `size` argument)\n", ratio)
	}
	b.WriteString(missingArgsNote(args, "product"))
	b.WriteString("\nThen show the generated image and its saved file path to the user.\n")
	return newPromptResult(productShotPrompt, b.String()), nil
}

func handleDetectAIImage(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	filePath := argOr(args, "file_path")

	var b strings.Builder
	b.WriteString("Determine whether this image is AI-generated:\n")
	fmt.Fprintf(&b, "  file_path: %s\n\n", filePath)
	b.WriteString("Call the `detect_image` tool with that `file_path`.\n")
	b.WriteString("Then explain the fused signals reported by the tool and give a clear verdict:\n")
	b.WriteString("- C2PA / Content Credentials\n")
	b.WriteString("- TC260 AIGC label (China GB 45438-2025)\n")
	b.WriteString("- SynthID invisible watermark\n")
	b.WriteString("- FFT spectrum\n")
	b.WriteString("- ONNX classifier score\n")
	b.WriteString("- visible watermark\n")
	b.WriteString(missingArgsNote(args, "file_path"))
	b.WriteString("\nState whether the image looks AI-generated, which signals drove the conclusion, and how confident the verdict is.\n")
	return newPromptResult(detectAIImagePrompt, b.String()), nil
}

func handleImageToVideo(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	imageURL := argOr(args, "image_url")
	motion := strings.TrimSpace(args["prompt"])

	var b strings.Builder
	b.WriteString("Animate this still image into a short video:\n")
	fmt.Fprintf(&b, "  image_url: %s\n\n", imageURL)
	b.WriteString("Call the `generate_video` tool and pass the image through its `image_urls` argument.\n")
	if motion != "" {
		fmt.Fprintf(&b, "Use this motion prompt: %s\n", motion)
	} else {
		b.WriteString("Derive a natural motion prompt from the image content (camera move plus subject motion).\n")
	}
	b.WriteString(missingArgsNote(args, "image_url"))
	b.WriteString("\nVideo generation is async and may take a while; once it finishes, report the saved video file path to the user.\n")
	return newPromptResult(imageToVideoPrompt, b.String()), nil
}

func handleRemoveImageBackground(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	filePath := argOr(args, "file_path")
	replaceColor := strings.TrimSpace(args["replace_color"])

	var b strings.Builder
	b.WriteString("Remove the background from this image:\n")
	fmt.Fprintf(&b, "  file_path: %s\n\n", filePath)
	b.WriteString("Call the `remove_background` tool (offline, no API key) with that `file_path`.\n")
	if replaceColor != "" {
		fmt.Fprintf(&b, "Set `replace_color` to %s so the background is filled with that color.\n", replaceColor)
	} else {
		b.WriteString("Do not set `replace_color`; keep the background transparent.\n")
	}
	b.WriteString(missingArgsNote(args, "file_path"))
	b.WriteString("\nReport the output path returned by the tool to the user.\n")
	return newPromptResult(removeBackgroundPrompt, b.String()), nil
}
