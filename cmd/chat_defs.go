package cmd

import (
	"encoding/json"

	"github.com/martianzhang/apimart-cli/internal/types"
)

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
			Description: "Search AI image prompt ideas from the local ideas database. Supports cross-language keyword search — if the user's query is in Chinese, Japanese, etc., also translate to English and search both languages for broader results. Use random=true for a random surprise idea.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"keywords": {"type": "string", "description": "Search keywords (ignored when random=true). Supports Chinese, Japanese, English etc. — translate to English and search both for better cross-language results."},
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
	// --- Audio tools ---
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "generate_speech",
			Description: "Convert text to spoken audio (TTS). The audio is saved to a local file — provide the filename to the user.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"input": {"type": "string", "description": "Text to convert to speech"},
					"model": {"type": "string", "description": "TTS model (default: gpt-4o-mini-tts)"},
					"voice": {"type": "string", "description": "Voice name: alloy, nova, echo, fable, onyx, shimmer, coral, verse, ballad, ash, sage, marin, cedar"},
					"format": {"type": "string", "description": "Audio format: mp3, wav, opus, aac, flac, pcm (default: mp3)", "enum": ["mp3", "wav", "opus", "aac", "flac", "pcm"]}
				},
				"required": ["input", "voice"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "transcribe_audio",
			Description: "Transcribe an audio file to text (STT). Provide a local file path to the audio file.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"file_path": {"type": "string", "description": "Path to the local audio file to transcribe"},
					"model": {"type": "string", "description": "STT model (default: whisper-1)"},
					"language": {"type": "string", "description": "Language hint (ISO-639-1, e.g. en, ja, zh)"}
				},
				"required": ["file_path"]
			}`),
		},
	},
	// --- Web tools ---
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "web_fetch",
			Description: "Fetch a URL and return its text content. Use this when you need to read web pages, check current information, or access online resources. If the content is large, use offset to paginate through it.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {"type": "string", "description": "URL to fetch (http:// or https://)"},
					"max_length": {"type": "integer", "description": "Max characters to return (default 5000, max 50000)"},
					"offset": {"type": "integer", "description": "Byte offset to start reading from (default 0). Use this to paginate through large pages — the response tells you how many bytes remain and what offset to use next."}
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
			Description: "Read the contents of a local text file (e.g. .txt, .md, .yaml, .json, .go). Use when the user asks about or references a local file. Supports pagination via offset/limit for large files.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"filepath": {"type": "string", "description": "Path to the local file to read (relative to current directory or absolute)"},
					"offset": {"type": "integer", "description": "Byte offset to start reading from (default 0). Use this to read the next chunk — the response tells you the file size and what offset to use next."},
					"limit": {"type": "integer", "description": "Max bytes to return (default 10000, max 100000)"}
				},
				"required": ["filepath"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "find",
			Description: "Recursively find files by name/pattern under a directory (like fd/find). Supports glob patterns. Use this when the user asks to find files, list images in a folder, search for documents, or explore the project structure.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"pattern": {"type": "string", "description": "Glob or substring to match filenames against, e.g. \"*.png\", \"test*.go\", \"README\""},
					"path": {"type": "string", "description": "Root directory to search (default: current directory \".\")"},
					"max_results": {"type": "integer", "description": "Maximum results to return (default: 30, max: 100)"}
				},
				"required": ["pattern"]
			}`),
		},
	},
}
