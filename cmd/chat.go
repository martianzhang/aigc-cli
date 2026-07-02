package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// Command history for readLineRaw up/down arrows.
var cmdHistory []string

// Tool definitions for Agent Loop
var agentToolDefs = []types.ToolDefinition{
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "generate_image",
			Description: "Generate images via AI (cost-effective, recommended default). Use this for most image generation tasks. For highly artistic/stylized results, consider midjourney_imagine instead.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"prompt": {"type": "string", "description": "Detailed text description of the image to generate"},
					"size": {"type": "string", "description": "Aspect ratio: 1:1, 16:9, 9:16, 4:3, 3:4", "enum": ["1:1", "16:9", "9:16", "4:3", "3:4"]},
					"n": {"type": "integer", "description": "Number of images to generate (1-4)", "minimum": 1, "maximum": 4},
					"quality": {"type": "string", "description": "Image quality", "enum": ["auto", "low", "medium", "high"]}
				},
				"required": ["prompt"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "generate_video",
			Description: "Generate a video based on a text description. Use this when the user asks you to create, generate, or make a video.",
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
			Description: "Search AI image prompt ideas from the local ideas database. Use when the user needs inspiration for image prompts.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"keywords": {"type": "string", "description": "Search keywords"},
					"limit": {"type": "integer", "description": "Max results to return"}
				},
				"required": ["keywords"]
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

// chatCmd represents the `apimart-cli chat` command.
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
      apimart-cli chat
  - Single-turn with --message:
      apimart-cli chat --message "Hello"

Examples:
  apimart-cli chat --message "Hello, who are you?"
  apimart-cli chat --system "You are a poet" --message "Write a poem about AI"
  apimart-cli chat --message "What is Go?" --message "Can you give an example?" --no-stream
  apimart-cli chat --json '{"model":"gpt-5","messages":[{"role":"user","content":"Hi"}]}'`,
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
	req.OutputWriter = os.Stdout

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
	fmt.Fprintln(os.Stderr, "---  "+strings.Join(parts, "  |  "))
}

// readLineRaw reads one line from a raw-mode terminal with readline shortcuts.
// Echoes characters to stderr. Returns io.EOF on Ctrl+D.
// Supports:
//
//	Ctrl+A  Home        Beginning of line
//	Ctrl+E  End         End of line
//	Ctrl+U  Kill        Clear whole line
//	Ctrl+K  Kill right  Clear to end of line
//	Ctrl+W  Backword    Delete word backwards
//	Ctrl+L  Clear       Clear screen
//	Ctrl+D  EOF         Exit
//	Up/Down arrows      Command history
//	Left/Right arrows   Move cursor
//
// Command history is shared across calls to readLineRaw.
func readLineRaw() (string, error) {
	buf := make([]byte, 0, 256)
	pos := 0 // cursor position within buf
	histIdx := -1

	// Package-level history slice shared across calls
	history := &cmdHistory

	for {
		var ch [3]byte
		n, err := os.Stdin.Read(ch[:])
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}

		// Multi-byte sequences (escape codes)
		if ch[0] == 27 && n >= 2 && ch[1] == '[' {
			switch ch[2] {
			case 'A': // Up arrow — history back
				if len(*history) > 0 && histIdx < len(*history)-1 {
					histIdx++
					// Clear current line
					for i := 0; i < len(buf); i++ {
						fmt.Fprint(os.Stderr, "\b \b")
					}
					// Load history entry
					buf = []byte((*history)[len(*history)-1-histIdx])
					pos = len(buf)
					fmt.Fprint(os.Stderr, string(buf))
				}
			case 'B': // Down arrow — history forward
				if histIdx > 0 {
					histIdx--
					// Clear current line
					for i := 0; i < len(buf); i++ {
						fmt.Fprint(os.Stderr, "\b \b")
					}
					buf = []byte((*history)[len(*history)-1-histIdx])
					pos = len(buf)
					fmt.Fprint(os.Stderr, string(buf))
				} else if histIdx == 0 {
					histIdx = -1
					// Clear and reset
					for i := 0; i < len(buf); i++ {
						fmt.Fprint(os.Stderr, "\b \b")
					}
					buf = buf[:0]
					pos = 0
				}
			case 'C': // Right arrow
				if pos < len(buf) {
					fmt.Fprint(os.Stderr, string(buf[pos]))
					pos++
				}
			case 'D': // Left arrow
				if pos > 0 {
					pos--
					fmt.Fprint(os.Stderr, "\b")
				}
			}
			continue
		}

		switch ch[0] {
		case 4: // Ctrl+D
			return "", io.EOF

		case 13: // Enter
			fmt.Fprint(os.Stderr, "\r\n")
			line := string(buf)
			// Save to history (non-empty, dedup last)
			if line != "" && (len(*history) == 0 || (*history)[len(*history)-1] != line) {
				*history = append(*history, line)
			}
			return line, nil

		case 127, 8: // Backspace
			if pos > 0 {
				// Remove character before cursor
				copy(buf[pos-1:], buf[pos:])
				buf = buf[:len(buf)-1]
				pos--
				// Redraw from cursor position
				fmt.Fprint(os.Stderr, "\b"+string(buf[pos:])+" ")
				// Move cursor back
				back := len(buf) - pos
				for i := 0; i < back+1; i++ {
					fmt.Fprint(os.Stderr, "\b")
				}
			}

		case 1: // Ctrl+A — beginning of line
			if pos > 0 {
				fmt.Fprint(os.Stderr, "\r")
				// Move cursor back pos positions from current
				for i := 0; i < pos; i++ {
					fmt.Fprint(os.Stderr, "\b")
				}
				pos = 0
			}

		case 5: // Ctrl+E — end of line
			if pos < len(buf) {
				fmt.Fprint(os.Stderr, string(buf[pos:]))
				pos = len(buf)
			}

		case 11: // Ctrl+K — kill to end of line
			if pos < len(buf) {
				// Clear from cursor to end
				for i := pos; i < len(buf); i++ {
					fmt.Fprint(os.Stderr, " ")
				}
				// Move back
				for i := pos; i < len(buf); i++ {
					fmt.Fprint(os.Stderr, "\b")
				}
				buf = buf[:pos]
			}

		case 21: // Ctrl+U — kill whole line
			if len(buf) > 0 {
				// Clear displayed text
				for i := 0; i < len(buf); i++ {
					fmt.Fprint(os.Stderr, "\b \b")
				}
				buf = buf[:0]
				pos = 0
			}

		case 12: // Ctrl+L — clear screen
			fmt.Fprint(os.Stderr, "\033[2J\033[H")
			// Re-prompt
			fmt.Fprint(os.Stderr, ">>> ")
			fmt.Fprint(os.Stderr, string(buf))

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
					fmt.Fprint(os.Stderr, "\b")
				}
				pos = start
				// Redraw from cursor
				fmt.Fprint(os.Stderr, string(buf[pos:]))
				// Clear leftover chars
				for i := 0; i < n; i++ {
					fmt.Fprint(os.Stderr, " ")
				}
				// Move back
				for i := 0; i < len(buf)-pos+n; i++ {
					fmt.Fprint(os.Stderr, "\b")
				}
			}

		default:
			if ch[0] >= 32 { // printable
				// Insert at cursor position
				buf = append(buf, 0)
				copy(buf[pos+1:], buf[pos:])
				buf[pos] = ch[0]
				// Redraw from cursor
				fmt.Fprint(os.Stderr, string(buf[pos:]))
				pos++
				// Move cursor back for characters after the inserted one
				for i := pos; i < len(buf); i++ {
					fmt.Fprint(os.Stderr, "\b")
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
// Agent Loop is enabled by default — LLM can call generate_image / generate_video tools.
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
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel()         // cancel pending HTTP calls
			os.Stdin.Close() // unblock stdin read so we exit cleanly
		case <-ctx.Done():
		}
	}()

	// Try raw terminal mode for cross-platform Ctrl+D detection
	isRaw := false
	var rawState *term.State
	if term.IsTerminal(int(os.Stdin.Fd())) {
		s, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err == nil {
			isRaw = true
			rawState = s
		}
	}
	if isRaw {
		defer term.Restore(int(os.Stdin.Fd()), rawState)
	}

	fmt.Fprint(os.Stderr, "\r\nInteractive chat mode. Type /exit or Ctrl+C to quit.\r\n")

	for {
		fmt.Fprint(os.Stderr, ">>> ")

		// Read one line, using raw mode if available
		var input string
		var err error
		if isRaw {
			input, err = readLineRaw()
		} else {
			input, err = readLineStdin()
		}
		if err == io.EOF {
			fmt.Fprint(os.Stderr, "\r\nBye!\r\n")
			return nil
		}
		if err != nil {
			return fmt.Errorf("input error: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Handle commands
		switch strings.ToLower(input) {
		case "/exit", "/quit", "/q", "exit", "quit", "bye", "goodbye", "退出", "再见":
			fmt.Fprint(os.Stderr, "\r\nBye!\r\n")
			return nil
		case "/clear", "/reset":
			history = history[:0]
			if chatSystem != "" {
				history = append(history, types.ChatMessage{Role: "system", Content: chatSystem})
			}
			fmt.Fprint(os.Stderr, "Conversation history cleared.\r\n")
			continue
		case "/help":
			fmt.Fprint(os.Stderr,
				"Available commands:\r\n"+
					"  /exit, /quit, /q  Exit\r\n"+
					"  exit, quit, bye   Same (without /)\r\n"+
					"  Ctrl+C            Exit\r\n"+
					"  Ctrl+D            Exit\r\n"+
					"  /clear, /reset    Clear conversation history\r\n"+
					"  /tools            List available tools\r\n"+
					"  /<tool> <json>    Directly call a tool (e.g. /generate_image {\"prompt\":\"a cat\"})\r\n"+
					"  /help             Show this help\r\n")
			modelDisplay := shared.Model
			if modelDisplay == "" {
				modelDisplay = "<API default>"
			}
			fmt.Fprintf(os.Stderr, "Model: %s | Stream: %v\r\n", modelDisplay, stream)
			if chatSystem != "" {
				fmt.Fprintf(os.Stderr, "System: %s\r\n", chatSystem)
			}
			if len(agentTools) > 0 {
				toolNames := make([]string, len(agentTools))
				for i, t := range agentTools {
					toolNames[i] = t.Function.Name
				}
				fmt.Fprintf(os.Stderr, "Tools: %s | Max iterations: %d\r\n", strings.Join(toolNames, ", "), maxIterations)
			}
			fmt.Fprint(os.Stderr, "Use -v/--verbose to show token & timing stats.\r\n")
			fmt.Fprint(os.Stderr, "Type /tools to list all tools with descriptions.\r\n")
			fmt.Fprint(os.Stderr, "Type /tools to see available tools. Use /{tool_name} <json> to call directly.\r\n")
			continue
		case "/tools":
			if len(agentTools) == 0 {
				fmt.Fprint(os.Stderr, "No tools available.\r\n")
			} else {
				fmt.Fprint(os.Stderr, "Available tools:\r\n")
				for _, t := range agentTools {
					fmt.Fprintf(os.Stderr, "  /%s\r\n", t.Function.Name)
				}
				fmt.Fprint(os.Stderr, "\r\nUsage: /<tool_name> <json_args>\r\n")
				fmt.Fprint(os.Stderr, "e.g. /generate_image {\"prompt\":\"a cat\"}\r\n")
			}
			continue
		}

		// Check for shell command: !<command>
		if strings.HasPrefix(strings.TrimSpace(input), "!") {
			cmdLine := strings.TrimSpace(input)[1:]
			if cmdLine != "" {
				fmt.Fprintf(os.Stderr, "\r\nRunning: %s\r\n", cmdLine)
				result := executeShellCommand(cmdLine)
				fmt.Fprintf(os.Stderr, "\r\nResult:\r\n%s\r\n", result)
				fmt.Fprint(os.Stderr, "\r\n")
				continue
			}
		}

		// Add user message to history
		history = append(history, types.ChatMessage{Role: "user", Content: input})

		// Run agent loop
		_, err = runAgentLoop(ctx, c, &history, agentTools, maxIterations, cmd)
		if err != nil {
			// cancelled — exit immediately
			if errors.Is(err, context.Canceled) {
				return nil
			}
			fmt.Fprintf(os.Stderr, "\r\nError: %v\r\n", err)
			history = history[:len(history)-1]
		}

		fmt.Fprint(os.Stderr, "\r\n")
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

		// Build request (always non-streaming internally for tool calling)
		req := &types.ChatRequest{
			Model:    shared.Model,
			Messages: *history,
			Stream:   false,
		}
		if len(agentTools) > 0 {
			req.Tools = agentTools
		}
		setFloatFlag(cmd, "temperature", &req.Temperature, chatTemperature)
		setIntFlag(cmd, "max-tokens", &req.MaxTokens, chatMaxTokens)

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

			// Execute each tool call
			for _, tc := range choice.Message.ToolCalls {
				toolResult := executeToolCall(c, tc)
				*history = append(*history, types.ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    toolResult,
				})
			}
			continue
		}

		// Text response — output to user
		textContent := choice.Message.Content
		if textContent != "" {
			fmt.Fprint(os.Stderr, "\r\n")
			fmt.Println(textContent)
		}

		// Append assistant response to history for subsequent turns
		*history = append(*history, choice.Message)

		// Verbose stats
		if shared.Verbose {
			printUsageStats(result, time.Since(agentStart))
		}

		if turnCount >= maxIterations {
			fmt.Fprintf(os.Stderr, "\r\nReached maximum iterations (%d). Start a new message to continue.\r\n", maxIterations)
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

	return allTools
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

// executeToolCall executes a single tool call and returns a text result for the LLM.
func executeToolCall(c *client.Client, tc types.ToolCall) string {
	switch tc.Function.Name {
	case "generate_image":
		return executeGenerateImage(c, tc.Function.Arguments)
	case "generate_video":
		return executeGenerateVideo(c, tc.Function.Arguments)
	case "midjourney_imagine", "midjourney_describe", "midjourney_reroll", "midjourney_video":
		return executeMidjourney(c, tc.Function.Name, tc.Function.Arguments)
	case "ideas":
		return executeIdeasSearch(tc.Function.Arguments)
	case "balance":
		return executeBalanceQuery(tc.Function.Arguments)
	case "task":
		return executeTaskQuery(tc.Function.Arguments)
	default:
		return fmt.Sprintf("Error: unknown tool '%s'", tc.Function.Name)
	}
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

	// Verbose debug: show config before generation
	if shared.Verbose {
		hasCfg := shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Image != nil
		fmt.Fprintf(os.Stderr, "\r\n[agent] image config loaded: %v\r\n", hasCfg)
		if hasCfg {
			fmt.Fprintf(os.Stderr, "[agent]   model=%q size=%q resolution=%q quality=%q\r\n",
				shared.Cfg.Defaults.Image.Model,
				shared.Cfg.Defaults.Image.Size,
				shared.Cfg.Defaults.Image.Resolution,
				shared.Cfg.Defaults.Image.Quality)
		}
	}

	// Use shared generation function (same logic as apimart-cli image)
	saved, err := generateImageAndSave(c, req)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return fmt.Sprintf("Successfully generated %d image(s).", len(saved))
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

	// Use shared generation function (same logic as apimart-cli video)
	saved, err := generateVideoAndSave(c, req)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return fmt.Sprintf("Successfully generated %d video(s).", len(saved))
}

// --- Midjourney agent tools ---

func executeMidjourney(c *client.Client, toolName, argsJSON string) string {
	mjClient := newMJClient()
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
}

type balanceQueryArgs struct {
	Scope string `json:"scope"`
}

type taskQueryArgs struct {
	TaskID string `json:"task_id"`
}

func executeIdeasSearch(argsJSON string) string {
	var args ideasSearchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
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
