package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/service"
	"github.com/martianzhang/apimart-cli/internal/types"
	"github.com/martianzhang/apimart-cli/internal/watermark"
)

// chatStdout and chatStderr are the output writers used by the chat REPL.
// They default to chatStdout/chatStderr but can be overridden in tests to
// capture output without touching real file descriptors.
var (
	chatStdout io.Writer = os.Stdout
	chatStderr io.Writer = os.Stderr
)

// Command history for readLineRaw up/down arrows.
var cmdHistory []string

// errInterrupted is returned by readLineRaw when Ctrl+C is pressed in raw mode.
// In raw mode, Ctrl+C is byte 0x03 (not SIGINT), so we handle it as a sentinel error.
var errInterrupted = errors.New("interrupted")

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
			Description: "检测并移除图片中的可见 AI 水印（豆包/即梦/百度/智谱清言等），恢复原始图像。完全离线运行，无需 API Key。适合：用户要求去掉图片上的 AI 生成水印、清理 AI 痕迹。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"file_path": {"type": "string", "description": "待去水印图片的本地路径"},
					"producer": {"type": "string", "description": "水印厂商提示（gemini/doubao/jimeng/baidu/zhipu）。可留空，留空时自动检测"},
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
			Description: "向图片添加可见 AI 水印（用于测试去水印效果）。已知厂商使用其注册水印样式；未知名称按文字渲染。完全离线运行，无需 API Key。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"file_path": {"type": "string", "description": "待加水印图片的本地路径"},
					"producer": {"type": "string", "description": "水印厂商名（gemini/doubao/jimeng/baidu/zhipu）或自定义文字"},
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
		return runInteractiveChat(cmd)
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

// sendChatRequest applies defaults, sends the request, and prints output.
// Usage stats are shown only when --verbose is set.
func sendChatRequest(cmd *cobra.Command, req *types.ChatRequest) error {
	// Apply defaults
	if !cmd.Flags().Changed("stream") {
		req.Stream = true
	}

	// Merge config defaults
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Chat != nil {
		if shared.Cfg.Defaults.Chat.Model != "" {
			req.Model = shared.Cfg.Defaults.Chat.Model
		}
	}

	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
	req.OutputWriter = chatStdout

	start := time.Now()
	result, err := c.ChatCompletion(req)
	if err != nil {
		return fmt.Errorf("chat failed: %w", err)
	}
	elapsed := time.Since(start)

	// Non-streaming: print result (streaming already written to OutputWriter)
	if !req.Stream && result != nil && len(result.Choices) > 0 {
		fmt.Println(result.Choices[0].Message.Content)
	}

	// Usage stats (to stderr, only with --verbose)
	if shared.Verbose {
		printUsageStats(result, elapsed)
	}

	return nil
}

// printUsageStats prints token/cost/timing stats to stderr.
func printUsageStats(result *types.ChatResponse, elapsed time.Duration) {
	if result == nil {
		return
	}
	parts := []string{}
	if result.Model != "" {
		parts = append(parts, fmt.Sprintf("Model: %s", result.Model))
	}
	if result.Usage != nil {
		parts = append(parts, fmt.Sprintf("Tokens: %d↑ + %d↓ = %d",
			result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Usage.TotalTokens))
		if result.Usage.Cost > 0 {
			parts = append(parts, fmt.Sprintf("Cost: $%.6f", result.Usage.Cost))
		}
	}
	parts = append(parts, fmt.Sprintf("Time: %v", elapsed.Round(time.Millisecond)))
	fmt.Fprintln(chatStderr, "---  "+strings.Join(parts, "  |  "))
}

// getFileCompletions returns file/directory names matching the given prefix.
// Used by readLineRaw for Tab completion of file paths (/preview, @filename).
func getFileCompletions(prefix string) []string {
	dir := "."
	base := prefix
	if idx := strings.LastIndexAny(prefix, `/\`); idx >= 0 {
		dir = prefix[:idx]
		base = prefix[idx+1:]
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var matches []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, base) {
			continue
		}
		match := name
		if e.IsDir() {
			match += string(filepath.Separator)
		}
		if dir != "." {
			match = dir + string(filepath.Separator) + match
		}
		matches = append(matches, match)
	}
	return matches
}

// backspaceRune removes the rune before cursor position in buf.
// Returns new buf and cursor pos, or pos=-1 if no character to delete.
func backspaceRune(buf []byte, pos int) ([]byte, int) {
	if pos <= 0 {
		return buf, -1
	}
	// Find start of previous rune (walking back through continuation bytes)
	prev := pos - 1
	for prev >= 0 && buf[prev] >= 128 && buf[prev]&0xC0 == 0x80 {
		prev--
	}
	n := pos - prev
	copy(buf[prev:], buf[pos:])
	buf = buf[:len(buf)-n]
	return buf, prev
}

// redrawFrom redraws the buffer from cursor position on stderr.
func redrawFrom(buf []byte, pos int) {
	fmt.Fprint(chatStderr, "\b"+string(buf[pos:])+" ")
	for i := 0; i < len(buf)-pos+1; i++ {
		fmt.Fprint(chatStderr, "\b")
	}
}

// readLineRaw reads one line from a raw-mode terminal with readline shortcuts.
// Echoes characters to stderr. Returns io.EOF on Ctrl+D.
// Supports:
//
//	Ctrl+A  Home        Beginning of line
//	Ctrl+E  End         End of line
//	Tab     Command completion
//	Ctrl+U  Kill        Clear whole line
//	Ctrl+K  Kill right  Clear to end of line
//	Ctrl+W  Backword    Delete word backwards
//	Ctrl+L  Clear       Clear screen
//	Ctrl+D  EOF         Exit
//	\ + Enter           Line continuation (multi-line)
//	Up/Down arrows      Command history
//	Left/Right arrows   Move cursor
//
// Command history is shared across calls to readLineRaw.
// completions is a list of command strings for Tab completion.
func readLineRaw(completions []string) (string, error) {
	buf := make([]byte, 0, 256)
	pos := 0 // cursor position within buf
	histIdx := -1

	// Completion cycle state
	var cycleMatches []string // current cycle matches
	var cycleIdx int          // index into cycleMatches
	var cycleBase string      // original input that started the cycle

	// Package-level history slice shared across calls
	history := &cmdHistory

	// Buffered reader for efficient byte/rune reading
	reader := bufio.NewReaderSize(os.Stdin, 64)

	for {
		ch, err := reader.ReadByte()
		if err != nil {
			return "", err
		}

		// Multi-byte sequences (escape codes: arrow keys, etc.)
		if ch == 27 {
			// If nothing buffered, it's a bare Escape key press — clear current line
			if reader.Buffered() == 0 {
				for i := 0; i < len(buf); i++ {
					fmt.Fprint(chatStderr, "\b \b")
				}
				buf = buf[:0]
				pos = 0
				continue
			}
			ch2, err := reader.ReadByte()
			if err != nil {
				continue
			}
			if ch2 == '[' {
				// Terminal escape sequence: arrow keys (ESC [ A) or CSI u (ESC [ 13;5 u)
				dir, err := reader.ReadByte()
				if err != nil {
					continue
				}
				if dir >= '0' && dir <= '9' {
					// CSI u sequence: ESC [ <keycode> ; <modifier> u
					// Not all terminals support this, so we just drain it.
					for {
						b, err := reader.ReadByte()
						if err != nil || b == 'u' {
							break
						}
					}
				} else {
					switch dir {
					case 'A': // Up arrow — history back
						if len(*history) > 0 && histIdx < len(*history)-1 {
							histIdx++
							for i := 0; i < len(buf); i++ {
								fmt.Fprint(chatStderr, "\b \b")
							}
							buf = []byte((*history)[len(*history)-1-histIdx])
							pos = len(buf)
							fmt.Fprint(chatStderr, string(buf))
						}
					case 'B': // Down arrow — history forward
						if histIdx > 0 {
							histIdx--
							for i := 0; i < len(buf); i++ {
								fmt.Fprint(chatStderr, "\b \b")
							}
							buf = []byte((*history)[len(*history)-1-histIdx])
							pos = len(buf)
							fmt.Fprint(chatStderr, string(buf))
						} else if histIdx == 0 {
							histIdx = -1
							for i := 0; i < len(buf); i++ {
								fmt.Fprint(chatStderr, "\b \b")
							}
							buf = buf[:0]
							pos = 0
						}
					case 'C': // Right arrow
						if pos < len(buf) {
							fmt.Fprint(chatStderr, string(buf[pos]))
							pos++
						}
					case 'D': // Left arrow
						if pos > 0 {
							pos--
							fmt.Fprint(chatStderr, "\b")
						}
					}
				}
			}
			continue
		}

		switch ch {
		case 3: // Ctrl+C (raw mode: byte, not SIGINT)
			fmt.Fprint(chatStderr, "\r\n")
			return "", errInterrupted

		case 4: // Ctrl+D
			return "", io.EOF

		case 13: // Enter
			// Line continuation: trailing \ + Enter → insert newline instead of submitting
			if len(buf) > 0 && buf[len(buf)-1] == '\\' {
				buf[len(buf)-1] = '\n'
				fmt.Fprint(chatStderr, "\r\n")
				pos = len(buf)
				continue
			}
			fmt.Fprint(chatStderr, "\r\n")
			line := string(buf)
			// Save to history (non-empty, dedup last)
			if line != "" && (len(*history) == 0 || (*history)[len(*history)-1] != line) {
				*history = append(*history, line)
			}
			return line, nil

		case 127, 8: // Backspace
			if buf, pos = backspaceRune(buf, pos); pos >= 0 {
				redrawFrom(buf, pos)
			}

		case 9: // Tab — completion cycling
			if len(buf) > 0 && pos == len(buf) {
				inputLine := string(buf)

				// Detect file path completion context
				filePrefix := ""
				isFileCtx := false
				if strings.HasPrefix(inputLine, "/preview ") {
					filePrefix = strings.TrimPrefix(inputLine, "/preview ")
					isFileCtx = true
				} else if atIdx := strings.LastIndex(inputLine, "@"); atIdx >= 0 {
					filePrefix = inputLine[atIdx+1:]
					isFileCtx = true
				}

				// Check if we're continuing a cycle
				inCycle := len(cycleMatches) > 0 && inputLine == cycleBase

				if isFileCtx {
					if !inCycle {
						cycleMatches = getFileCompletions(filePrefix)
						cycleIdx = -1
						cycleBase = inputLine
					}
				} else if !isFileCtx {
					prefix := strings.TrimSpace(inputLine)
					if prefix == "" {
						break
					}
					if !inCycle {
						cycleMatches = nil
						for _, c := range completions {
							if strings.HasPrefix(c, prefix) {
								cycleMatches = append(cycleMatches, c)
							}
						}
						cycleIdx = -1
						cycleBase = inputLine
					}
				} else {
					break
				}

				if len(cycleMatches) == 0 {
					break
				}

				// Advance cycle
				cycleIdx = (cycleIdx + 1) % len(cycleMatches)
				match := cycleMatches[cycleIdx]

				// Replace buffer with the match
				for i := 0; i < len(buf); i++ {
					fmt.Fprint(chatStderr, "\b \b")
				}

				if isFileCtx {
					if strings.HasPrefix(cycleBase, "/preview ") {
						buf = []byte("/preview " + match)
					} else if atIdx := strings.LastIndex(cycleBase, "@"); atIdx >= 0 {
						buf = []byte(cycleBase[:atIdx+1] + match)
					}
				} else {
					buf = []byte(match)
				}
				pos = len(buf)
				cycleBase = string(buf) // keep cycle alive for next Tab
				fmt.Fprint(chatStderr, string(buf))
			}

		case 1: // Ctrl+A — beginning of line
			if pos > 0 {
				fmt.Fprint(chatStderr, "\r")
				// Move cursor back pos positions from current
				for i := 0; i < pos; i++ {
					fmt.Fprint(chatStderr, "\b")
				}
				pos = 0
			}

		case 5: // Ctrl+E — end of line
			if pos < len(buf) {
				fmt.Fprint(chatStderr, string(buf[pos:]))
				pos = len(buf)
			}

		case 11: // Ctrl+K — kill to end of line
			if pos < len(buf) {
				// Clear from cursor to end
				for i := pos; i < len(buf); i++ {
					fmt.Fprint(chatStderr, " ")
				}
				// Move back
				for i := pos; i < len(buf); i++ {
					fmt.Fprint(chatStderr, "\b")
				}
				buf = buf[:pos]
			}

		case 21: // Ctrl+U — kill whole line
			if len(buf) > 0 {
				// Clear displayed text
				for i := 0; i < len(buf); i++ {
					fmt.Fprint(chatStderr, "\b \b")
				}
				buf = buf[:0]
				pos = 0
			}

		case 12: // Ctrl+L — clear screen
			fmt.Fprint(chatStderr, "\033[2J\033[H")
			// Re-prompt
			fmt.Fprint(chatStderr, ">>> ")
			fmt.Fprint(chatStderr, string(buf))

		case 23: // Ctrl+W — delete word backwards
			if pos > 0 {
				// Find start of word to delete
				end := pos
				start := end
				// Skip spaces
				for start > 0 && buf[start-1] == ' ' {
					start--
				}
				// Skip word chars
				for start > 0 && buf[start-1] != ' ' {
					start--
				}
				// Delete from start to end
				n := end - start
				copy(buf[start:], buf[end:])
				buf = buf[:len(buf)-n]
				// Move cursor to start
				for i := 0; i < pos-start; i++ {
					fmt.Fprint(chatStderr, "\b")
				}
				pos = start
				// Redraw from cursor
				fmt.Fprint(chatStderr, string(buf[pos:]))
				// Clear leftover chars
				for i := 0; i < n; i++ {
					fmt.Fprint(chatStderr, " ")
				}
				// Move back
				for i := 0; i < len(buf)-pos+n; i++ {
					fmt.Fprint(chatStderr, "\b")
				}
			}

		default:
			if ch >= 32 { // printable — could be start of multi-byte UTF-8
				// Put the byte back and read a complete rune
				if err := reader.UnreadByte(); err != nil {
					// Fallback: insert byte directly
					buf = append(buf, 0)
					copy(buf[pos+1:], buf[pos:])
					buf[pos] = ch
					fmt.Fprint(chatStderr, string(buf[pos:]))
					pos++
					for i := pos; i < len(buf); i++ {
						fmt.Fprint(chatStderr, "\b")
					}
					continue
				}
				r, _, err := reader.ReadRune()
				if err != nil {
					continue
				}
				// Encode the rune back to UTF-8 bytes
				runeBytes := make([]byte, 4)
				n := utf8.EncodeRune(runeBytes, r)
				runeBytes = runeBytes[:n]
				// Insert all bytes at cursor position
				for _, b := range runeBytes {
					buf = append(buf, 0)
					copy(buf[pos+1:], buf[pos:])
					buf[pos] = b
					pos++
				}
				// Redraw from cursor
				fmt.Fprint(chatStderr, string(buf[pos-n:]))
				// Move cursor back for characters after the inserted ones
				for i := pos; i < len(buf); i++ {
					fmt.Fprint(chatStderr, "\b")
				}
			}
		}
	}
}

// readLineStdin reads one line from a non-terminal stdin (e.g. piped input).
func readLineStdin() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

// runInteractiveChat enters an interactive multi-turn chat REPL.
// Conversation history accumulates across turns. Streaming is enabled by default.
// Agent Loop is enabled by default — LLM can call generate_image / generate_video /
// remove_watermark / add_watermark and other registered tools.
func runInteractiveChat(cmd *cobra.Command) error {
	// Load chat config for Agent Loop settings
	var chatCfg *types.ChatDefaults
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Chat != nil {
		chatCfg = shared.Cfg.Defaults.Chat
		if shared.Model == "" && chatCfg.Model != "" {
			shared.Model = chatCfg.Model
		}
	}

	// Determine max iterations per user message (default 10)
	maxIterations := 10
	if chatCfg != nil && chatCfg.MaxIterations > 0 {
		maxIterations = chatCfg.MaxIterations
	}

	// Build allowed tool list based on tools/disable_tools config
	agentTools := buildAgentTools(chatCfg)

	// Initialize conversation history
	history := []types.ChatMessage{}
	if chatSystem != "" {
		history = append(history, types.ChatMessage{Role: "system", Content: chatSystem})
	}

	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
	stream := !chatNoStream

	// Signal handling (Ctrl+C) — cancel context to abort API calls / polling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.SetContext(ctx) // make all client HTTP requests and polling loops cancellable
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel() // cancel pending HTTP calls and polling loops
		case <-ctx.Done():
		}
	}()

	// Raw terminal mode state. Toggled on/off around blocking operations
	// because on Windows, raw mode prevents Ctrl+C from generating SIGINT.
	var rawState *term.State
	isRaw := false
	if term.IsTerminal(int(os.Stdin.Fd())) {
		s, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err == nil {
			isRaw = true
			rawState = s
		}
	}
	defer func() {
		if rawState != nil {
			term.Restore(int(os.Stdin.Fd()), rawState)
		}
	}()

	// exitRawMode restores cooked mode so Ctrl+C generates proper SIGINT.
	exitRawMode := func() {
		if rawState != nil {
			term.Restore(int(os.Stdin.Fd()), rawState)
			rawState = nil
		}
	}
	// enterRawMode re-enters raw mode for fancy input handling.
	enterRawMode := func() {
		if !isRaw {
			return
		}
		if rawState != nil {
			return // already in raw mode
		}
		if term.IsTerminal(int(os.Stdin.Fd())) {
			s, err := term.MakeRaw(int(os.Stdin.Fd()))
			if err == nil {
				rawState = s
			}
		}
	}

	fmt.Fprint(chatStderr, "\r\nInteractive chat mode. Type /help for commands, /exit or Ctrl+C to quit.\r\n")

	// Show current model and stream mode at startup
	modelDisplay := shared.Model
	if modelDisplay == "" {
		if chatCfg != nil && chatCfg.Model != "" {
			modelDisplay = chatCfg.Model
		} else {
			modelDisplay = "<API default>"
		}
	}
	streamMode := "stream"
	if chatNoStream {
		streamMode = "no-stream"
	}
	fmt.Fprintf(chatStderr, "Model: %s | Mode: %s\r\n", modelDisplay, streamMode)

	// Build Tab-completion candidates
	completions := []string{"/exit", "/clear", "/help", "/?", "/tools", "/preview"}
	for _, t := range agentTools {
		completions = append(completions, "/"+t.Function.Name)
	}

	for {
		fmt.Fprint(chatStderr, ">>> ")

		// Read one line, using raw mode if available
		var input string
		var err error
		if isRaw {
			input, err = readLineRaw(completions)
		} else {
			input, err = readLineStdin()
		}
		if err == io.EOF {
			fmt.Fprint(chatStderr, "\r\nBye!\r\n")
			return nil
		}
		if errors.Is(err, errInterrupted) {
			fmt.Fprint(chatStderr, "\r\nBye!\r\n")
			return nil
		}
		if err != nil {
			// stdin read error — clean exit
			return nil
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Multi-line input: starting with ``` enters multi-line mode.
		// Each line is read separately; empty line submits.
		if strings.HasPrefix(input, "```") {
			var lines []string
			rest := strings.TrimPrefix(input, "```")
			if rest != "" {
				lines = append(lines, rest)
			}
			for {
				fmt.Fprint(chatStderr, "... ")
				var line string
				var err error
				if isRaw {
					line, err = readLineRaw(completions)
				} else {
					line, err = readLineStdin()
				}
				if err != nil {
					break
				}
				if strings.TrimSpace(line) == "" {
					break
				}
				lines = append(lines, line)
			}
			input = strings.Join(lines, "\n")
			if input == "" {
				continue
			}
		}

		// Handle commands
		switch strings.ToLower(input) {
		case "/exit", "/quit", "/q", "exit", "quit", "bye", "goodbye", "退出", "再见":
			fmt.Fprint(chatStderr, "\r\nBye!\r\n")
			return nil
		case "/clear", "/reset":
			history = history[:0]
			if chatSystem != "" {
				history = append(history, types.ChatMessage{Role: "system", Content: chatSystem})
			}
			fmt.Fprint(chatStderr, "Conversation history cleared.\r\n")
			continue
		case "/compact":
			// Count non-system messages to determine if there's anything to compact
			nonSysCount := len(history)
			for _, msg := range history {
				if msg.Role == "system" {
					nonSysCount--
				}
			}
			if nonSysCount == 0 {
				fmt.Fprint(chatStderr, "Nothing to compact — conversation is empty.\r\n")
				continue
			}

			exitRawMode()

			fmt.Fprint(chatStderr, "\r\nCompacting conversation...\r\n")

			// Build a request: send the full history + a summarization instruction
			compactReq := &types.ChatRequest{
				Model:    shared.Model,
				Messages: history,
				Stream:   false,
			}
			compactReq.Messages = append(compactReq.Messages, types.ChatMessage{
				Role:    "user",
				Content: "Please provide a detailed summary of our conversation above. Capture: 1) the user's goals and requirements, 2) key decisions made, 3) any files created or modified, 4) current status of any ongoing work. Be thorough — this summary will replace the conversation history so nothing important should be lost.",
			})

			result, err := c.ChatCompletion(compactReq)
			if err != nil {
				fmt.Fprintf(chatStderr, "Compact failed: %v\r\n", err)
				enterRawMode()
				continue
			}

			summary := ""
			if result != nil && len(result.Choices) > 0 {
				summary = result.Choices[0].Message.Content
			}
			if summary == "" {
				fmt.Fprint(chatStderr, "Compact failed: got empty response.\r\n")
				enterRawMode()
				continue
			}

			oldCount := len(history)
			history = history[:0]
			if chatSystem != "" {
				history = append(history, types.ChatMessage{Role: "system", Content: chatSystem})
			}
			history = append(history, types.ChatMessage{
				Role:    "system",
				Content: "[Compacted conversation history]\n" + summary + "\n[End of compacted history]",
			})

			fmt.Fprintf(chatStderr, "\r\n✓ Compacted: %d messages → 1 summary\r\n", oldCount)
			enterRawMode()
			continue
		case "/help", "/?", "?":
			fmt.Fprint(chatStderr,
				"Available commands:\r\n"+
					"  /exit, /quit, /q  Exit\r\n"+
					"  exit, quit, bye   Same (without /)\r\n"+
					"  Ctrl+C            Exit\r\n"+
					"  Ctrl+D            Exit\r\n"+
					"  /clear, /reset    Clear conversation history\r\n"+
					"  /compact          Compact conversation (summarize to save context)\r\n"+
					"  /tools            List available tools\r\n"+
					"  /<tool> <json|--flags>  Direct tool call (no LLM)\r\n"+
					"  /preview <file>   Preview image/video with system viewer\r\n"+
					"  !<command>        Run a shell command\r\n"+
					"  \\ + Enter         Line continuation (multi-line input)\r\n"+
					"  ```...empty line  Multi-line paste mode\r\n"+
					"  /help             Show this help\r\n")
			modelDisplay := shared.Model
			if modelDisplay == "" {
				modelDisplay = "<API default>"
			}
			fmt.Fprintf(chatStderr, "Model: %s | Stream: %v\r\n", modelDisplay, stream)
			if chatSystem != "" {
				fmt.Fprintf(chatStderr, "System: %s\r\n", chatSystem)
			}
			if len(agentTools) > 0 {
				toolNames := make([]string, len(agentTools))
				for i, t := range agentTools {
					toolNames[i] = t.Function.Name
				}
				fmt.Fprintf(chatStderr, "Tools: %s | Max iterations: %d\r\n", strings.Join(toolNames, ", "), maxIterations)
			}
			fmt.Fprint(chatStderr, "Use -v/--verbose to show token & timing stats.\r\n")
			fmt.Fprint(chatStderr, "Use /{tool_name} <json> to call a tool directly (e.g. /generate_image {\"prompt\":\"a cat\"})\r\n")
			continue
		case "/tools":
			if len(agentTools) == 0 {
				fmt.Fprint(chatStderr, "No tools available.\r\n")
			} else {
				fmt.Fprint(chatStderr, "Available tools:\r\n")
				for _, t := range agentTools {
					fmt.Fprintf(chatStderr, "  /%s\r\n", t.Function.Name)
					if desc := t.Function.Description; desc != "" {
						fmt.Fprintf(chatStderr, "    %s\r\n", desc)
					}
				}
				fmt.Fprint(chatStderr, "\r\nUsage: /<tool_name> <json_args>\r\n")
				fmt.Fprint(chatStderr, "e.g. /generate_image {\"prompt\":\"a cat\"}\r\n")
			}
			continue
		}

		// Check for preview command: /preview <filepath>
		if strings.HasPrefix(strings.TrimSpace(input), "/preview") {
			parts := strings.SplitN(input, " ", 2)
			filePath := ""
			if len(parts) == 2 {
				filePath = strings.TrimSpace(parts[1])
			}
			if filePath == "" {
				fmt.Fprint(chatStderr, "Usage: /preview <filepath>\r\n\r\n")
				// Show recently generated files as hints
				recent := previewLatestFiles("")
				if len(recent) > 0 {
					fmt.Fprint(chatStderr, "Recent files:\r\n")
					for _, f := range recent {
						fmt.Fprintf(chatStderr, "  /preview %s\r\n", f)
					}
					fmt.Fprint(chatStderr, "\r\n")
				}
				continue
			}
			if err := service.PreviewFile(filePath); err != nil {
				fmt.Fprintf(chatStderr, "Preview failed: %v\r\n", err)
			}
			fmt.Fprint(chatStderr, "\r\n")
			continue
		}

		// Check for shell command: !<command>
		if strings.HasPrefix(strings.TrimSpace(input), "!") {
			cmdLine := strings.TrimSpace(input)[1:]
			if cmdLine != "" {
				exitRawMode()
				fmt.Fprintf(chatStderr, "\r\nRunning: %s\r\n", cmdLine)
				result := executeShellCommand(cmdLine)
				fmt.Fprintf(chatStderr, "\r\nResult:\r\n%s\r\n", result)
				fmt.Fprint(chatStderr, "\r\n")
				enterRawMode()
				continue
			}
		}

		// Direct tool call: /<tool_name> <json> or /<tool_name> --flag value
		// Non-structured args (bare text) fall through to LLM for interpretation.
		if strings.HasPrefix(input, "/") && !strings.HasPrefix(input, "//") {
			spaceIdx := strings.Index(input, " ")
			cmdName := input[1:]
			argsJSON := ""
			if spaceIdx > 0 {
				cmdName = input[1:spaceIdx]
				argsJSON = strings.TrimSpace(input[spaceIdx+1:])
			}
			// Accept either JSON or --flag / key=value format
			if !json.Valid([]byte(argsJSON)) && argsJSON != "" {
				if converted := parseFlagsToJSON(argsJSON); converted != "" {
					argsJSON = converted
				}
			}
			if json.Valid([]byte(argsJSON)) {
				// Silently ignore known CLI-only flags that have no tool parameter equivalent
				cliOnlyFlags := map[string]bool{"preview": true, "dry-run": true, "save-images": true, "verbose": true, "json": true}
				if argsJSON != "" && argsJSON != "{}" {
					var schema struct {
						Properties map[string]any `json:"properties"`
					}
					for _, t := range agentTools {
						if t.Function.Name == cmdName && len(t.Function.Parameters) > 0 {
							json.Unmarshal(t.Function.Parameters, &schema)
							var parsed map[string]any
							if err := json.Unmarshal([]byte(argsJSON), &parsed); err == nil {
								for k := range parsed {
									if _, ok := schema.Properties[k]; !ok && !cliOnlyFlags[k] {
										fmt.Fprintf(chatStderr, "\r\n[warning] /%s: unknown flag --%s\r\n", cmdName, k)
									}
								}
							}
							break
						}
					}
				}
				executed := false
				for _, t := range agentTools {
					if t.Function.Name == cmdName {
						executed = true
						exitRawMode()
						tc := types.ToolCall{
							ID:   "direct",
							Type: "function",
							Function: types.ToolCallFunction{
								Name:      cmdName,
								Arguments: argsJSON,
							},
						}
						result := executeToolCall(c, tc)
						fmt.Fprintf(chatStderr, "\r\n%s\r\n", result)
						fmt.Fprint(chatStderr, "\r\n")
						enterRawMode()
						break
					}
				}
				if executed {
					continue
				}
			}
			// Not valid JSON/flags or unknown command → fall through to LLM
		}

		// Add user message to history
		history = append(history, types.ChatMessage{Role: "user", Content: input})

		// Exit raw mode before blocking operations so Ctrl+C generates SIGINT
		exitRawMode()

		// Run agent loop
		_, err = runAgentLoop(ctx, c, &history, agentTools, maxIterations, cmd)
		if err != nil {
			// cancelled — exit immediately
			if errors.Is(err, context.Canceled) {
				return nil
			}
			fmt.Fprintf(chatStderr, "\r\nError: %v\r\n", err)
			history = history[:len(history)-1]
		}

		fmt.Fprint(chatStderr, "\r\n")

		// Re-enter raw mode for next prompt
		enterRawMode()
	}
}

// runAgentLoop executes the tool-calling loop: send request → check tool_calls → execute → repeat.
// history is modified in-place (appended with assistant + tool messages).
// Returns the final ChatResponse (text response) or error.
// If ctx is cancelled (Ctrl+C), returns immediately with context.Canceled.
func runAgentLoop(ctx context.Context, c *client.Client, history *[]types.ChatMessage, agentTools []types.ToolDefinition, maxIterations int, cmd *cobra.Command) (*types.ChatResponse, error) {
	// Merge defaults.chat.model into shared.Model if empty
	if shared.Model == "" && shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Chat != nil {
		shared.Model = shared.Cfg.Defaults.Chat.Model
	}

	turnCount := 0
	agentStart := time.Now()
	for turnCount < maxIterations {
		// Check for Ctrl+C between turns
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		turnCount++

		// Use streaming for all turns — text prints to stdout in real-time.
		// Tool-calling turns: text (model's reasoning) streams first, then tool_calls are detected.
		req := &types.ChatRequest{
			Model:        shared.Model,
			Messages:     *history,
			Stream:       true,
			OutputWriter: chatStdout,
		}
		if len(agentTools) > 0 {
			req.Tools = agentTools
		}
		setFloatFlag(cmd, "temperature", &req.Temperature, chatTemperature)
		setIntFlag(cmd, "max-tokens", &req.MaxTokens, chatMaxTokens)

		// Print a newline to separate from the prompt / previous output
		if turnCount > 1 {
			fmt.Fprint(chatStderr, "\r\n---\r\n")
		}
		fmt.Fprint(chatStderr, "\r\n")

		result, err := c.ChatCompletion(req)
		if err != nil {
			return nil, err
		}

		if len(result.Choices) == 0 {
			break
		}
		choice := result.Choices[0]

		// Check for tool calls
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) > 0 {
			// Add assistant message with tool calls to history
			*history = append(*history, choice.Message)

			// Execute each tool call with timing and result summary
			for _, tc := range choice.Message.ToolCalls {
				fmt.Fprintf(chatStderr, "\r\n[tool] %s:\r\n", tc.Function.Name)
				printToolArgs(tc.Function.Arguments)

				toolStart := time.Now()
				toolResult := executeToolCall(c, tc)
				elapsed := time.Since(toolStart).Round(time.Millisecond)

				// Show brief result summary to user
				resultSummary := summarizeToolResult(tc.Function.Name, toolResult)
				fmt.Fprintf(chatStderr, "\r\n[tool] done in %v: %s\r\n", elapsed, resultSummary)

				*history = append(*history, types.ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    toolResult,
				})
			}
			continue
		}

		// Text response — already streamed to stdout by handleSSE
		*history = append(*history, choice.Message)

		// Verbose stats
		if shared.Verbose {
			printUsageStats(result, time.Since(agentStart))
		}

		if turnCount >= maxIterations {
			fmt.Fprintf(chatStderr, "\r\nReached maximum iterations (%d). Start a new message to continue.\r\n", maxIterations)
		}

		return result, nil
	}

	return nil, nil
}

func buildChatRequest(cmd *cobra.Command) (*types.ChatRequest, error) {
	// JSON input
	if chatJSONFlag != "" {
		data, err := readInput(chatJSONFlag)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.ChatRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		return req, nil
	}

	var messages []types.ChatMessage

	if chatSystem != "" {
		messages = append(messages, types.ChatMessage{Role: "system", Content: chatSystem})
	}

	for _, msg := range chatMessages {
		messages = append(messages, types.ChatMessage{Role: "user", Content: msg})
	}

	// If no --message, read from stdin
	if len(messages) == 0 {
		data, err := readInput("-")
		if err != nil {
			return nil, fmt.Errorf("failed to read prompt from stdin: %w", err)
		}
		prompt := strings.TrimSpace(string(data))
		if prompt != "" {
			messages = append(messages, types.ChatMessage{Role: "user", Content: prompt})
		}
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("message is required (use --message or pipe to stdin)")
	}

	req := &types.ChatRequest{
		Model:    shared.Model,
		Messages: messages,
		Stream:   !chatNoStream,
	}

	setFloatFlag(cmd, "temperature", &req.Temperature, chatTemperature)
	setIntFlag(cmd, "max-tokens", &req.MaxTokens, chatMaxTokens)

	return req, nil
}

func setFloatFlag(cmd *cobra.Command, name string, target **float64, val float64) {
	if cmd.Flags().Changed(name) {
		v := val
		*target = &v
	}
}

// toURLs converts a single URL string to a slice (for MJ API compatibility).
func toURLs(url string) []string {
	if url == "" {
		return nil
	}
	return []string{url}
}

// buildAgentTools returns the list of tool definitions based on config.
// Applies tools (whitelist) and disable_tools (blacklist) glob patterns.
func buildAgentTools(cfg *types.ChatDefaults) []types.ToolDefinition {
	if cfg != nil && len(cfg.DisableTools) > 0 {
		// Check if all tools are disabled via "*"
		for _, pattern := range cfg.DisableTools {
			if matched, _ := path.Match(pattern, "*"); matched {
				return nil
			}
		}
	}

	// Start with all available tools
	allTools := agentToolDefs

	// Apply whitelist (tools)
	if cfg != nil && len(cfg.Tools) > 0 {
		hasWildcard := false
		for _, pattern := range cfg.Tools {
			if matched, _ := path.Match(pattern, "*"); matched {
				hasWildcard = true
				break
			}
		}
		if !hasWildcard {
			filtered := make([]types.ToolDefinition, 0)
			for _, t := range allTools {
				for _, pattern := range cfg.Tools {
					if matched, _ := path.Match(pattern, t.Function.Name); matched {
						filtered = append(filtered, t)
						break
					}
				}
			}
			allTools = filtered
		}
	}

	// Apply blacklist (disable_tools)
	if cfg != nil && len(cfg.DisableTools) > 0 {
		filtered := make([]types.ToolDefinition, 0)
		for _, t := range allTools {
			disabled := false
			for _, pattern := range cfg.DisableTools {
				if matched, _ := path.Match(pattern, t.Function.Name); matched {
					disabled = true
					break
				}
			}
			if !disabled {
				filtered = append(filtered, t)
			}
		}
		allTools = filtered
	}

	// Filter by provider — some tools only work with specific providers
	isAPIMart := isAPIMartProvider()

	providerFiltered := make([]types.ToolDefinition, 0)
	for _, t := range allTools {
		// MJ tools: only APIMart provider
		if strings.HasPrefix(t.Function.Name, "midjourney") && !isAPIMart {
			continue
		}
		// balance/task: only APIMart provider
		if (t.Function.Name == "balance" || t.Function.Name == "task") && !isAPIMart {
			continue
		}
		providerFiltered = append(providerFiltered, t)
	}

	return providerFiltered
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
// inferValue parses a string into the most specific type (bool→int→float→string).
func inferValue(s string) any {
	if s == "true" || s == "false" {
		return s == "true"
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

// parseFlagsToJSON converts "--flag value" or "key=value" style args to JSON.
// Bare words are collected and set as "keywords" for the tool.
func parseFlagsToJSON(s string) string {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return ""
	}
	hasStructured := false
	for _, p := range parts {
		if strings.HasPrefix(p, "--") || strings.Contains(p, "=") {
			hasStructured = true
			break
		}
	}
	if !hasStructured {
		return ""
	}
	obj := make(map[string]any)
	var bareWords []string
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if eq := strings.Index(p, "="); eq > 0 {
			obj[p[:eq]] = inferValue(p[eq+1:])
		} else if strings.HasPrefix(p, "--") {
			key := p[2:]
			if i+1 < len(parts) && !strings.HasPrefix(parts[i+1], "--") && !strings.Contains(parts[i+1], "=") {
				i++
				obj[key] = inferValue(parts[i])
			} else {
				obj[key] = true
			}
		} else {
			bareWords = append(bareWords, p)
		}
	}
	if len(bareWords) > 0 {
		if _, has := obj["keywords"]; !has {
			obj["keywords"] = strings.Join(bareWords, " ")
		}
	}
	data, _ := json.Marshal(obj)
	return string(data)
}

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
		return "Error: producer is required (known: gemini/doubao/jimeng/baidu/zhipu, or custom text)"
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
	note := ""
	if a.Producer == "doubao" || a.Producer == "jimeng" {
		note = " (TC260 metadata injected)"
	}
	return fmt.Sprintf("Successfully added watermark (engine: %s%s). Output: %s", res.Name, note, out)
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
		if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Midjourney != nil {
			d := shared.Cfg.Defaults.Midjourney
			if mjReq.Speed == "" && d.Speed != "" {
				mjReq.Speed = d.Speed
			}
			if mjReq.Version == "" && d.Version != "" {
				mjReq.Version = d.Version
			}
			if mjReq.Style == "" && d.Style != "" {
				mjReq.Style = d.Style
			}
			if mjReq.Size == "" && d.Size != "" {
				mjReq.Size = d.Size
			}
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
