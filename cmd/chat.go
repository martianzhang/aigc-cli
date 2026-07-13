package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/types"
	"github.com/martianzhang/apimart-cli/internal/watermark"
)

// chatStdout and chatStderr are the output writers used by the chat REPL.
// They default to chatStdout/chatStderr but can be overridden in tests to
// capture output without touching real file descriptors.
var chatStdout io.Writer = os.Stdout
var chatStderr io.Writer = os.Stderr

// Tool definitions for Agent Loop
var agentToolDefs = []types.ToolDefinition{
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "generate_image",
			Description: "Generate images via AI (cost-effective, recommended default). Images are saved to local files — do NOT invent URLs, use /preview to show them. Use this for most image generation tasks. For highly artistic/stylized results, consider midjourney_imagine instead.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"prompt": {"type": "string", "description": "Detailed text description of the image to generate"},
					"size": {"type": "string", "description": "Aspect ratio: 1:1, 16:9, 9:16, 4:3, 3:4", "enum": ["1:1", "16:9", "9:16", "4:3", "3:4"]},
					"n": {"type": "integer", "description": "Number of images to generate (1-4)", "minimum": 1, "maximum": 4},
					"quality": {"type": "string", "description": "Image quality (low=cheapest default, high=best quality but costs more)", "enum": ["auto", "low", "medium", "high"]}
				},
				"required": ["prompt"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "generate_video",
			Description: "Generate a video based on a text description. Videos are saved to local files — do NOT invent URLs, use /preview to show them. Use this when the user asks you to create, generate, or make a video.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"prompt": {"type": "string", "description": "Detailed text description of the video to generate"},
					"duration": {"type": "integer", "description": "Video duration in seconds (4-15)", "minimum": 4, "maximum": 15},
					"resolution": {"type": "string", "description": "Video resolution", "enum": ["480p", "720p", "1080p"]}
				},
				"required": ["prompt"]
			}`),
		},
	},
	// --- Midjourney tools ---
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "midjourney_imagine",
			Description: "Midjourney image generation (costs 5-10x more than generate_image and produces 4 variants). Only use when the user explicitly asks for Midjourney, or needs highly artistic/stylized/painted results. For most use cases, prefer generate_image instead.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"prompt": {"type": "string", "description": "Text description of the image"},
					"image_url": {"type": "string", "description": "Reference image URL for image-guided generation"},
					"aspect_ratio": {"type": "string", "enum": ["1:1","16:9","9:16","4:3","3:4","21:9"]},
					"style": {"type": "string", "enum": ["raw","expressive"]},
					"version": {"type": "string", "enum": ["6.1","7","8","8.1"]},
					"speed": {"type": "string", "enum": ["relax","fast","turbo"]}
				},
				"required": ["prompt"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "midjourney_describe",
			Description: "Get a text description of an image (reverse prompt). Upload an image URL and get back a prompt that MJ would use to generate it.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"image_url": {"type": "string", "description": "URL of the image to describe"}
				},
				"required": ["image_url"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "midjourney_reroll",
			Description: "Regenerate a Midjourney generation (same prompt, new results). Requires a previous MJ task ID.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"task_id": {"type": "string", "description": "Previous MJ task ID to reroll"}
				},
				"required": ["task_id"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "midjourney_video",
			Description: "Turn an image into a short video via Midjourney.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"image_url": {"type": "string", "description": "URL of the image to animate"},
					"prompt": {"type": "string", "description": "Optional text description"}
				},
				"required": ["image_url"]
			}`),
		},
	},
	// --- Prompt ideas ---
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "ideas",
			Description: "Search AI image prompt ideas from the local ideas database. Use when the user needs inspiration for image prompts, or use random=true for a random surprise idea.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"keywords": {"type": "string", "description": "Search keywords (ignored when random=true)"},
					"limit": {"type": "integer", "description": "Max results to return (default 5)"},
					"random": {"type": "boolean", "description": "Get random ideas instead of keyword search"}
				}
			}`),
		},
	},
	// --- Watermark tools ---
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "remove_watermark",
			Description: "⚠️ 检测并移除图片中的可见 AI 水印（内置 gemini，其他需通过 learn-watermark 学习）。仅限合法用途（如修复个人旧照片），禁止去除他人版权水印。完全离线运行，无需 API Key。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"file_path": {"type": "string", "description": "待去水印图片的本地路径"},
					"producer": {"type": "string", "description": "水印厂商提示（内置 gemini，其他需 learn-watermark 学习）。可留空，留空时自动检测"},
					"output_path": {"type": "string", "description": "输出路径（可选，默认 <原图>_clean<ext>）"}
				},
				"required": ["file_path"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "add_watermark",
			Description: "向图片添加可见 AI 水印（仅用于创建去水印算法的测试样本，不注入任何元数据）。gemini 使用注册水印样式；未知名称按文字渲染。完全离线运行，无需 API Key。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"file_path": {"type": "string", "description": "待加水印图片的本地路径"},
					"producer": {"type": "string", "description": "水印厂商名（内置 gemini，其他需 learn-watermark 学习）或自定义文字"},
					"output_path": {"type": "string", "description": "输出路径（可选，默认 <原图>_watermarked.png）"}
				},
				"required": ["file_path", "producer"]
			}`),
		},
	},
	// --- Account tools ---
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "balance",
			Description: "Query your API key balance or user account balance.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"scope": {"type": "string", "enum": ["token","user"], "description": "token=API key balance, user=account balance"}
				}
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "task",
			Description: "Query the status and result of an async task (image, video, MJ, etc.) by task ID.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"task_id": {"type": "string", "description": "Task ID to query"}
				},
				"required": ["task_id"]
			}`),
		},
	},
	// --- Web tools ---
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "web_fetch",
			Description: "Fetch a URL and return its text content. Use this when you need to read web pages, check current information, or access online resources.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {"type": "string", "description": "URL to fetch (http:// or https://)"},
					"max_length": {"type": "integer", "description": "Max characters to return (default 5000)"}
				},
				"required": ["url"]
			}`),
		},
	},
	// --- Local file tools ---
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "grep",
			Description: "Search files for a pattern (regex). Returns matching lines with file paths and line numbers. Searches the current directory by default. Use this when the user asks to find text in files, search code, or locate something in the project.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"pattern": {"type": "string", "description": "Regex pattern to search for (Go regexp syntax)"},
					"path": {"type": "string", "description": "Directory or file to search in (default: current directory)"},
					"include": {"type": "string", "description": "Glob pattern to filter files (e.g. '*.go', '**/*.md'). Only search files matching this pattern."},
					"ignore_case": {"type": "boolean", "description": "Case insensitive search (default: false)"},
					"context": {"type": "integer", "description": "Number of context lines before/after each match (default: 0)"},
					"max_matches": {"type": "integer", "description": "Max matches to return (default: 20, max: 100)"}
				},
				"required": ["pattern"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "read_file",
			Description: "Read the contents of a local text file (e.g. .txt, .md, .yaml, .json, .go). Use when the user asks about or references a local file.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"filepath": {"type": "string", "description": "Path to the local file to read (relative to current directory or absolute)"}
				},
				"required": ["filepath"]
			}`),
		},
	},
}

// chat flag variables
var (
	chatSystem      string
	chatMessages    []string
	chatTemperature float64
	chatMaxTokens   int
	chatNoStream    bool
	chatJSONFlag    string
	chatInteractive bool
)

// chatCmd represents the `aigc-cli chat` command.
var chatCmd = &cobra.Command{
	Use:          "chat",
	Short:        "Chat with AI models (streaming by default)",
	SilenceUsage: true,
	Long: `Start a chat conversation with AI models via the APIMart API.

Supports all major models: GPT, Claude, Gemini, DeepSeek, and more.
Streaming output is enabled by default. Model is optional — API default is used when omitted.
Use --verbose to show token usage, cost, and timing stats.

Agentic Chat:
  Chat supports tool calling by default — the LLM can call generate_image
  and generate_video tools to create images and videos within the
  conversation. Configure via defaults.chat in config.yaml.

Modes:
  - Interactive multi-turn (default without --message):
      aigc-cli chat
  - Single-turn with --message:
      aigc-cli chat --message "Hello"

Examples:
  aigc-cli chat --message "Hello, who are you?"
  aigc-cli chat --system "You are a poet" --message "Write a poem about AI"
  aigc-cli chat --message "What is Go?" --message "Can you give an example?" --no-stream
  aigc-cli chat --json '{"model":"gpt-5","messages":[{"role":"user","content":"Hi"}]}'`,
	RunE: runChat,
}

func runChat(cmd *cobra.Command, args []string) error {
	// --json mode is always single-turn
	if chatJSONFlag != "" {
		req, err := buildChatRequest(cmd)
		if err != nil {
			return err
		}
		return sendChatRequest(cmd, req)
	}

	// Determine mode:
	//   Piped stdin → single-turn (non-interactive)
	//   --interactive flag → interactive
	//   No --message → interactive (auto-detect)
	//   Otherwise → single-turn
	isPiped := !term.IsTerminal(int(os.Stdin.Fd()))
	isInteractive := !isPiped && (chatInteractive || !cmd.Flags().Changed("message"))

	if isInteractive {
		// Use the Bubble Tea TUI for interactive chat (refactored from runInteractiveChat)
		return runChatTUI(cmd)
	}

	// Single-turn mode with agent loop
	req, err := buildChatRequest(cmd)
	if err != nil {
		return err
	}

	// Load config for agent loop settings
	var chatCfg *types.ChatDefaults
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Chat != nil {
		chatCfg = shared.Cfg.Defaults.Chat
	}
	maxIterations := 10
	if chatCfg != nil && chatCfg.MaxIterations > 0 {
		maxIterations = chatCfg.MaxIterations
	}
	agentTools := buildAgentTools(chatCfg)

	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
	history := req.Messages
	_, err = runAgentLoop(context.Background(), c, &history, agentTools, maxIterations, cmd)
	return err
}

// generateImageArgs is the JSON structure for generate_image tool arguments.
type generateImageArgs struct {
	Prompt  string `json:"prompt"`
	Size    string `json:"size,omitempty"`
	N       int    `json:"n,omitempty"`
	Quality string `json:"quality,omitempty"`
}

// generateVideoArgs is the JSON structure for generate_video tool arguments.
type generateVideoArgs struct {
	Prompt     string `json:"prompt"`
	Duration   int    `json:"duration,omitempty"`
	Resolution string `json:"resolution,omitempty"`
}

// watermarkArgs is the JSON structure for remove_watermark / add_watermark tool arguments.
type watermarkArgs struct {
	FilePath   string `json:"file_path"`
	Producer   string `json:"producer,omitempty"`
	OutputPath string `json:"output_path,omitempty"`
}

// resolveFileRefs scans JSON argument strings for @filename patterns and
// replaces them with the file contents. e.g. {"prompt":"@prompt.md"} reads
// prompt.md and uses its content as the prompt value.
// Works recursively for nested objects and arrays.
func resolveFileRefs(argsJSON string) string {
	var raw interface{}
	if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return argsJSON // not valid JSON, return as-is
	}
	resolved := resolveFileRef(raw)
	data, _ := json.Marshal(resolved)
	return string(data)
}

// resolveFileRef recursively walks a parsed JSON value and replaces any
// string starting with "@" by reading the referenced file.
func resolveFileRef(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		if strings.HasPrefix(val, "@") {
			path := val[1:] // strip the @ prefix
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Sprintf("[Error: cannot read %s: %v]", path, err)
			}
			return string(content)
		}
		return val
	case map[string]interface{}:
		for k, nested := range val {
			val[k] = resolveFileRef(nested)
		}
		return val
	case []interface{}:
		for i, item := range val {
			val[i] = resolveFileRef(item)
		}
		return val
	default:
		return val
	}
}

// executeToolCall executes a single tool call and returns a text result for the LLM.
func executeToolCall(c *client.Client, tc types.ToolCall) string {
	// Expand @filename references in arguments before dispatching.
	// Skip read_file — it handles its own file access.
	args := tc.Function.Arguments
	if tc.Function.Name != "read_file" && strings.Contains(args, `"@`) {
		resolved := resolveFileRefs(args)
		if resolved != args {
			if shared.Verbose {
				fmt.Fprintf(chatStderr, "\r\n[agent] resolved @file refs in %s\r\n", tc.Function.Name)
			}
			args = resolved
		}
	}

	switch tc.Function.Name {
	case "generate_image":
		return executeGenerateImage(c, args)
	case "generate_video":
		return executeGenerateVideo(c, args)
	case "midjourney_imagine", "midjourney_describe", "midjourney_reroll", "midjourney_video":
		return executeMidjourney(c, tc.Function.Name, args)
	case "ideas":
		return executeIdeasSearch(args)
	case "balance":
		return executeBalanceQuery(args)
	case "task":
		return executeTaskQuery(args)
	case "web_fetch":
		return executeWebFetch(args)
	case "grep":
		return executeGrep(args)
	case "read_file":
		return executeReadFile(args)
	case "remove_watermark":
		return executeRemoveWatermark(args)
	case "add_watermark":
		return executeAddWatermark(args)
	default:
		return fmt.Sprintf("Error: unknown tool '%s'", tc.Function.Name)
	}
}

// summarizeToolResult returns a one-line summary of a tool's result for user display.

func summarizeToolResult(toolName, result string) string {
	// Truncate long results to first meaningful line
	firstLine := result
	if idx := strings.IndexAny(result, "\n\r"); idx > 0 {
		firstLine = result[:idx]
	}
	// Strip common prefixes for cleaner display
	firstLine = strings.TrimSpace(firstLine)
	if strings.HasPrefix(firstLine, "Successfully generated") {
		return firstLine
	}
	if strings.HasPrefix(firstLine, "Error:") {
		return firstLine
	}
	if len(firstLine) > 80 {
		firstLine = firstLine[:80] + "..."
	}
	return firstLine
}

// executeGenerateImage runs image generation and returns a text summary for the LLM.
// Uses defaults.image.model from config, NOT the chat model (shared.Model).
func executeGenerateImage(c *client.Client, argsJSON string) string {
	var args generateImageArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}

	req := &types.GenerateRequest{
		Prompt:  args.Prompt,
		Size:    args.Size,
		Quality: args.Quality,
	}
	if args.N > 0 {
		v := args.N
		req.N = &v
	}

	// Show actual config defaults that will be applied
	hasCfg := shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Image != nil
	if hasCfg {
		d := shared.Cfg.Defaults.Image
		var overrides []string
		if d.Model != "" {
			overrides = append(overrides, fmt.Sprintf("model=%s", d.Model))
		}
		if d.Quality != "" {
			overrides = append(overrides, fmt.Sprintf("quality=%s", d.Quality))
		}
		if d.Size != "" {
			overrides = append(overrides, fmt.Sprintf("size=%s", d.Size))
		}
		if d.Resolution != "" {
			overrides = append(overrides, fmt.Sprintf("resolution=%s", d.Resolution))
		}
		if len(overrides) > 0 {
			fmt.Fprintf(chatStderr, "\r\n[config] %s\r\n", strings.Join(overrides, " | "))
		}
	}

	// Use shared generation function (same logic as aigc-cli image)
	saved, err := generateImageAndSave(c, req)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return fmt.Sprintf("Successfully generated %d image(s).\nImages saved locally:\n  %s\nUser can use /preview to view them.", len(saved), strings.Join(saved, "\n  "))
}

// executeGenerateVideo runs video generation and returns a text summary for the LLM.
// Uses defaults.video.model from config, NOT the chat model (shared.Model).
func executeGenerateVideo(c *client.Client, argsJSON string) string {
	var args generateVideoArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}

	req := &types.VideoGenerateRequest{
		Prompt: args.Prompt,
	}
	if args.Duration > 0 {
		v := args.Duration
		req.Duration = &v
	}
	if args.Resolution != "" {
		req.Resolution = args.Resolution
	}

	// Use shared generation function (same logic as aigc-cli video)
	saved, err := generateVideoAndSave(c, req)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return fmt.Sprintf("Successfully generated %d video(s).\nFiles saved locally:\n  %s\nUser can use /preview to view them.", len(saved), strings.Join(saved, "\n  "))
}

// executeRemoveWatermark runs visible-AI-watermark removal and returns a text summary.
func executeRemoveWatermark(argsJSON string) string {
	var a watermarkArgs
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if a.FilePath == "" {
		return "Error: file_path is required"
	}

	// Auto-load custom watermarks from config directory
	watermark.LoadWatermarkPNGsFromDir(watermarkDir())

	res, err := watermark.RemoveFileHinted(a.FilePath, a.OutputPath, a.Producer)
	if err != nil {
		return fmt.Sprintf("Error: remove failed: %v", err)
	}
	if !res.Removed {
		return "No visible AI watermark detected/removed."
	}
	out := a.OutputPath
	if out == "" {
		ext := filepath.Ext(a.FilePath)
		out = strings.TrimSuffix(a.FilePath, ext) + "_clean" + ext
	}
	return fmt.Sprintf("Successfully removed watermark (engine: %s). Output: %s", res.Name, out)
}

// executeAddWatermark runs visible-AI-watermark addition and returns a text summary.
func executeAddWatermark(argsJSON string) string {
	var a watermarkArgs
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if a.FilePath == "" {
		return "Error: file_path is required"
	}
	if a.Producer == "" {
		return "Error: producer is required (known: gemini, or custom text)"
	}
	res, err := watermark.AddWatermarkFile(a.FilePath, a.OutputPath, a.Producer)
	if err != nil {
		return fmt.Sprintf("Error: add failed: %v", err)
	}
	out := a.OutputPath
	if out == "" {
		ext := filepath.Ext(a.FilePath)
		out = strings.TrimSuffix(a.FilePath, ext) + "_watermarked.png"
	}
	return fmt.Sprintf("Successfully added watermark (engine: %s). Output: %s", res.Name, out)
}

// --- Midjourney agent tools ---

func executeMidjourney(c *client.Client, toolName, argsJSON string) string {
	mjClient := newMJClient()
	// Propagate context for Ctrl+C cancellation
	if mj, ok := mjClient.(*client.Client); ok && c != nil {
		mj.SetContext(c.GetContext())
	}
	switch toolName {
	case "midjourney_imagine":
		var args struct {
			Prompt      string `json:"prompt"`
			ImageURL    string `json:"image_url"`
			AspectRatio string `json:"aspect_ratio"`
			Style       string `json:"style"`
			Version     string `json:"version"`
			Speed       string `json:"speed"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Sprintf("Error: invalid arguments: %v", err)
		}
		mjReq := &types.MJImagineRequest{
			Prompt:    args.Prompt,
			ImageURLs: toURLs(args.ImageURL),
			Size:      args.AspectRatio,
			Style:     args.Style,
			Version:   args.Version,
			Speed:     args.Speed,
		}
		// Merge config defaults
		if shared.Cfg != nil && shared.Cfg.Defaults != nil {
			shared.Cfg.Defaults.Midjourney.MergeIntoImagine(mjReq)
		}
		text, err := midjourneySubmitAndGetText(mjClient, "imagine", mjReq)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return text

	case "midjourney_describe":
		var args struct {
			ImageURL string `json:"image_url"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Sprintf("Error: invalid arguments: %v", err)
		}
		req := &types.MJDescribeRequest{ImageURLs: toURLs(args.ImageURL)}
		text, err := midjourneySubmitAndGetText(mjClient, "describe", req)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return text

	case "midjourney_reroll":
		var args struct {
			TaskID string `json:"task_id"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Sprintf("Error: invalid arguments: %v", err)
		}
		req := &types.MJRerollRequest{TaskID: args.TaskID}
		text, err := midjourneySubmitAndGetText(mjClient, "reroll", req)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return text

	case "midjourney_video":
		var args struct {
			ImageURL string `json:"image_url"`
			Prompt   string `json:"prompt"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Sprintf("Error: invalid arguments: %v", err)
		}
		req := &types.MJVideoRequest{ImageURLs: toURLs(args.ImageURL), Prompt: args.Prompt}
		text, err := midjourneySubmitAndGetText(mjClient, "video", req)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return text
	}
	return "Error: unknown midjourney tool"
}

// --- Ideas, balance, task agent tools ---

type ideasSearchArgs struct {
	Keywords string `json:"keywords"`
	Limit    int    `json:"limit"`
	Random   bool   `json:"random"`
}

type balanceQueryArgs struct {
	Scope string `json:"scope"`
}

type taskQueryArgs struct {
	TaskID string `json:"task_id"`
}

type webFetchArgs struct {
	URL       string `json:"url"`
	MaxLength int    `json:"max_length"`
}

func executeIdeasSearch(argsJSON string) string {
	var args ideasSearchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.Limit <= 0 {
		args.Limit = 5
	}
	if args.Random {
		text, err := searchIdeasRandom(args.Limit)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return text
	}
	if args.Keywords == "" {
		return "Error: keywords is required (or set random=true for random ideas)"
	}
	text, err := searchIdeasText(args.Keywords, args.Limit)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return text
}

func executeBalanceQuery(argsJSON string) string {
	var args balanceQueryArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.Scope == "" {
		args.Scope = "token"
	}
	text, err := getBalanceText(args.Scope)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return text
}

func executeTaskQuery(argsJSON string) string {
	var args taskQueryArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	text, err := queryTaskText(args.TaskID)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return text
}

func executeWebFetch(argsJSON string) string {
	var args webFetchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.URL == "" {
		return "Error: url is required"
	}
	if args.MaxLength <= 0 {
		args.MaxLength = 5000
	}
	if args.MaxLength > 50000 {
		args.MaxLength = 50000
	}

	// Use http.DefaultClient (configured with proxy at startup) + context timeout
	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer fetchCancel()
	httpReq, err := http.NewRequestWithContext(fetchCtx, "GET", args.URL, nil)
	if err != nil {
		return fmt.Sprintf("Error: invalid URL %s: %v", args.URL, err)
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Sprintf("Error: failed to fetch %s: %v", args.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Sprintf("Error: %s returned status %d", args.URL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(args.MaxLength)*3))
	if err != nil {
		return fmt.Sprintf("Error: failed to read response: %v", err)
	}

	// Try to extract text content from HTML
	content := string(body)
	if len(content) > args.MaxLength {
		content = content[:args.MaxLength] + "\n\n...(truncated)"
	}

	return fmt.Sprintf("Content from %s:\n\n%s", args.URL, content)
}

// printToolArgs prints tool call arguments as key-value pairs for user visibility.
// Long string values are truncated at 80 chars.
func printToolArgs(argsJSON string) {
	if argsJSON == "" || argsJSON == "{}" {
		return
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		// Not JSON — just print raw (truncated)
		raw := argsJSON
		if len(raw) > 120 {
			raw = raw[:120] + "..."
		}
		fmt.Fprintf(chatStderr, "  %s\r\n", raw)
		return
	}
	for k, v := range m {
		s := fmt.Sprintf("%v", v)
		if len(s) > 80 {
			s = s[:80] + "..."
		}
		fmt.Fprintf(chatStderr, "  %s=%s\r\n", k, s)
	}
}

// executeReadFile reads a local text file and returns its contents.
// Used by the read_file tool. Only allows readable text files.
func executeReadFile(argsJSON string) string {
	var args struct {
		Filepath string `json:"filepath"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.Filepath == "" {
		return "Error: filepath is required"
	}
	fpath := strings.TrimPrefix(args.Filepath, "@") // strip @ prefix if present
	// Security: only allow reading text files
	ext := strings.ToLower(filepath.Ext(fpath))
	switch ext {
	case ".txt", ".md", ".yaml", ".yml", ".json", ".go", ".py", ".js", ".ts",
		".css", ".html", ".sh", ".bash", ".toml", ".ini", ".cfg", ".conf",
		".xml", ".svg", ".env", ".example":
		// allowed
	default:
		return fmt.Sprintf("Error: cannot read %s files for security reasons", ext)
	}
	content, err := os.ReadFile(fpath)
	if err != nil {
		return fmt.Sprintf("Error: cannot read %s: %v", fpath, err)
	}
	if len(content) > 10000 {
		content = content[:10000]
		return fmt.Sprintf("File is large, showing first 10000 bytes:\n\n%s\n\n...(truncated)", string(content))
	}
	return fmt.Sprintf("```\n%s\n```", string(content))
}

// executeShellCommand runs a shell command and returns its output as a string.
// Has a 30s timeout. Precedence:
//   - SHELL env var (if set and executable)
//   - Windows: pwsh > powershell > cmd
//   - Others: zsh > bash > sh
//
// executeGrep searches files for a regex pattern and returns matching lines.
// grepArgs holds parsed arguments for the grep tool.
type grepArgs struct {
	Pattern    string
	Path       string
	Include    string
	IgnoreCase bool
	Context    int
	MaxMatches int
}

func executeGrep(argsJSON string) string {
	var raw struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		Include    string `json:"include"`
		IgnoreCase bool   `json:"ignore_case"`
		Context    int    `json:"context"`
		MaxMatches int    `json:"max_matches"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	args := grepArgs{
		Pattern:    raw.Pattern,
		Path:       raw.Path,
		Include:    raw.Include,
		IgnoreCase: raw.IgnoreCase,
		Context:    raw.Context,
		MaxMatches: raw.MaxMatches,
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.Pattern == "" {
		return "Error: pattern is required"
	}

	searchPath := args.Path
	if searchPath == "" {
		var err error
		searchPath, err = filepath.Abs(".")
		if err != nil {
			return fmt.Sprintf("Error: cannot get current directory: %v", err)
		}
	}
	if args.MaxMatches <= 0 {
		args.MaxMatches = 20
	} else if args.MaxMatches > 100 {
		args.MaxMatches = 100
	}
	if args.Context < 0 {
		args.Context = 0
	} else if args.Context > 10 {
		args.Context = 10
	}

	// Prefer rg (ripgrep), then grep, then Go fallback
	if hasExecutable("rg") {
		return grepWithRipgrep(&args, searchPath)
	}
	if hasExecutable("grep") {
		return grepWithGrep(&args, searchPath)
	}
	return grepGoImpl(&args, searchPath)
}

// grepWithRipgrep searches using ripgrep (rg) — the fastest option.
func grepWithRipgrep(args *grepArgs, searchPath string) string {
	rgArgs := []string{"--no-heading", "--line-number", "--color", "never"}
	if args.IgnoreCase {
		rgArgs = append(rgArgs, "-i")
	}
	if args.Context > 0 {
		rgArgs = append(rgArgs, "-C", fmt.Sprintf("%d", args.Context))
	}
	if args.Include != "" {
		rgArgs = append(rgArgs, "-g", args.Include)
	}
	// rg --max-count limits matches per file; we want total, so use -m
	rgArgs = append(rgArgs, "-m", fmt.Sprintf("%d", args.MaxMatches))
	rgArgs = append(rgArgs, "--", args.Pattern, searchPath)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "rg", rgArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// rg exits 1 when no matches — that's not an error
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return fmt.Sprintf("No matches found for pattern %q in %s", args.Pattern, searchPath)
		}
		return fmt.Sprintf("rg error: %v\n%s", err, string(out))
	}
	if len(out) == 0 {
		return fmt.Sprintf("No matches found for pattern %q in %s", args.Pattern, searchPath)
	}
	return string(out)
}

// grepWithGrep searches using system grep (GNU or BSD).
func grepWithGrep(args *grepArgs, searchPath string) string {
	grepArgs := []string{"-rn", "--color=never"} // recursive, line numbers
	if args.IgnoreCase {
		grepArgs = append(grepArgs, "-i")
	}
	if args.Context > 0 {
		grepArgs = append(grepArgs, "-C", fmt.Sprintf("%d", args.Context))
	} else {
		grepArgs = append(grepArgs, "-m", fmt.Sprintf("%d", args.MaxMatches))
	}
	if args.Include != "" {
		grepArgs = append(grepArgs, "--include", args.Include)
	}
	// Skip binary files and hidden dirs
	grepArgs = append(grepArgs, "--binary-files=without-match", "--exclude-dir=.git")
	grepArgs = append(grepArgs, "-e", args.Pattern, searchPath)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "grep", grepArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return fmt.Sprintf("No matches found for pattern %q in %s", args.Pattern, searchPath)
		}
		return fmt.Sprintf("grep error: %v\n%s", err, string(out))
	}
	if len(out) == 0 {
		return fmt.Sprintf("No matches found for pattern %q in %s", args.Pattern, searchPath)
	}
	// Limit output lines
	result := string(out)
	lines := strings.Split(result, "\n")
	if len(lines) > args.MaxMatches {
		lines = lines[:args.MaxMatches]
		result = strings.Join(lines, "\n") + "\n...(truncated, max matches reached)"
	}
	return result
}

// grepGoImpl is the pure-Go fallback when no system grep tool is available.
func grepGoImpl(args *grepArgs, searchPath string) string {
	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return fmt.Sprintf("Error: invalid regex pattern '%s': %v", args.Pattern, err)
	}

	isDir := false
	if fi, err := os.Stat(searchPath); err == nil {
		isDir = fi.IsDir()
	}

	type match struct {
		file    string
		lineNum int
		line    string
		before  []string
		after   []string
	}
	var matches []match

	walkFn := func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name != "." && name != ".." && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if name == "node_modules" || name == "vendor" || name == "dist" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if args.Include != "" {
			matched, err := filepath.Match(args.Include, info.Name())
			if err != nil || !matched {
				return nil
			}
		}
		ext := strings.ToLower(filepath.Ext(fpath))
		switch ext {
		case ".txt", ".md", ".yaml", ".yml", ".json", ".go", ".py", ".js", ".ts",
			".css", ".html", ".sh", ".bash", ".toml", ".ini", ".cfg", ".conf",
			".xml", ".svg", ".env", ".example":
		default:
			return nil
		}
		content, err := os.ReadFile(fpath)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			matchLine := line
			if args.IgnoreCase {
				matchLine = strings.ToLower(line)
			}
			if re.MatchString(matchLine) {
				m := match{
					file:    fpath,
					lineNum: i + 1,
					line:    line,
				}
				if args.Context > 0 {
					start := i - args.Context
					if start < 0 {
						start = 0
					}
					for j := start; j < i; j++ {
						m.before = append(m.before, lines[j])
					}
					end := i + args.Context
					if end >= len(lines) {
						end = len(lines) - 1
					}
					for j := i + 1; j <= end; j++ {
						m.after = append(m.after, lines[j])
					}
				}
				matches = append(matches, m)
				if len(matches) >= args.MaxMatches {
					return filepath.SkipAll
				}
			}
		}
		return nil
	}

	if isDir {
		filepath.Walk(searchPath, walkFn)
	} else {
		info, err := os.Stat(searchPath)
		if err != nil {
			return fmt.Sprintf("Error: cannot access %s: %v", searchPath, err)
		}
		walkFn(searchPath, info, nil)
	}

	if len(matches) == 0 {
		return fmt.Sprintf("No matches found for pattern %q in %s", args.Pattern, searchPath)
	}

	var b strings.Builder
	truncated := len(matches) >= args.MaxMatches
	currentFile := ""
	for _, m := range matches {
		if m.file != currentFile {
			currentFile = m.file
			fmt.Fprintf(&b, "\n%s:\n", m.file)
		}
		for _, l := range m.before {
			fmt.Fprintf(&b, "  %s\n", l)
		}
		fmt.Fprintf(&b, "> %4d:  %s\n", m.lineNum, strings.TrimRight(m.line, "\r"))
		for _, l := range m.after {
			fmt.Fprintf(&b, "  %s\n", l)
		}
	}
	if truncated {
		b.WriteString("\n...(truncated, max matches reached)")
	}
	return b.String()
}

func executeShellCommand(cmdLine string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	shell := os.Getenv("SHELL")
	if shell != "" && hasExecutable(shell) {
		cmd := exec.CommandContext(ctx, shell, "-c", cmdLine)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Sprintf("Error: %v\n%s", err, string(out))
		}
		return string(out)
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		switch {
		case hasExecutable("pwsh"):
			cmd = exec.CommandContext(ctx, "pwsh", "-NoProfile", "-Command", cmdLine)
		case hasExecutable("powershell"):
			cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", cmdLine)
		default:
			cmd = exec.CommandContext(ctx, "cmd", "/c", cmdLine)
		}
	} else {
		switch {
		case hasExecutable("zsh"):
			cmd = exec.CommandContext(ctx, "zsh", "-c", cmdLine)
		case hasExecutable("bash"):
			cmd = exec.CommandContext(ctx, "bash", "-c", cmdLine)
		default:
			cmd = exec.CommandContext(ctx, "sh", "-c", cmdLine)
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Error: %v\n%s", err, string(out))
	}
	return string(out)
}

// hasExecutable checks if a command is available in PATH.
func hasExecutable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func init() {
	f := chatCmd.Flags()
	f.StringVarP(&chatSystem, "system", "s", "", "System prompt to set AI behavior")
	f.StringArrayVar(&chatMessages, "message", nil, "User message (repeatable for multi-turn)")
	f.Float64VarP(&chatTemperature, "temperature", "t", 0, "Sampling temperature (0-2)")
	f.IntVar(&chatMaxTokens, "max-tokens", 0, "Maximum tokens in response")
	f.BoolVar(&chatNoStream, "no-stream", false, "Disable streaming, wait for full response")
	f.StringVar(&chatJSONFlag, "json", "", "JSON file, string, or \"-\" for stdin")
	f.BoolVarP(&chatInteractive, "interactive", "i", false, "Enter interactive multi-turn chat mode")

	rootCmd.AddCommand(chatCmd)
}
