package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/service"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// Tool definitions for Agent Loop
var agentToolDefs = []types.ToolDefinition{
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "generate_image",
			Description: "Generate an image based on a text description. Use this when the user asks you to create, draw, or generate an image.",
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
	_, err = runAgentLoop(c, &history, agentTools, maxIterations, cmd)
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

// readLineRaw reads one line from a raw-mode terminal.
// Echoes characters to stderr. Returns io.EOF on Ctrl+D.
func readLineRaw() (string, error) {
	var buf []byte
	for {
		var ch [1]byte
		n, err := os.Stdin.Read(ch[:])
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}
		switch ch[0] {
		case 4: // Ctrl+D — detect as raw byte on all platforms
			return "", io.EOF
		case 13: // Enter (CR)
			fmt.Fprint(os.Stderr, "\r\n")
			return string(buf), nil
		case 127, 8: // Backspace
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				fmt.Fprint(os.Stderr, "\b \b")
			}
		default:
			if ch[0] >= 32 { // printable
				buf = append(buf, ch[0])
				fmt.Fprint(os.Stderr, string(ch[0]))
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

	// Signal handling (Ctrl+C)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		os.Stdin.Close() // unblock stdin read so we exit cleanly
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
			continue
		}

		// Add user message to history
		history = append(history, types.ChatMessage{Role: "user", Content: input})

		// Run agent loop
		_, err = runAgentLoop(c, &history, agentTools, maxIterations, cmd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\r\nError: %v\r\n", err)
			history = history[:len(history)-1]
		}

		fmt.Fprint(os.Stderr, "\r\n")
	}
}

// runAgentLoop executes the tool-calling loop: send request → check tool_calls → execute → repeat.
// history is modified in-place (appended with assistant + tool messages).
// Returns the final ChatResponse (text response) or error.
func runAgentLoop(c *client.Client, history *[]types.ChatMessage, agentTools []types.ToolDefinition, maxIterations int, cmd *cobra.Command) (*types.ChatResponse, error) {
	// Merge defaults.chat.model into shared.Model if empty
	if shared.Model == "" && shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Chat != nil {
		shared.Model = shared.Cfg.Defaults.Chat.Model
	}

	turnCount := 0
	agentStart := time.Now()
	for turnCount < maxIterations {
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

	// Apply config defaults — LLM-provided values are hints, config is the ceiling
	if shared.Verbose {
		hasCfg := shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Image != nil
		fmt.Fprintf(os.Stderr, "\r\n[agent] shared.Cfg.Defaults.Image loaded: %v\r\n", hasCfg)
		if hasCfg {
			fmt.Fprintf(os.Stderr, "[agent]   model=%q size=%q resolution=%q quality=%q\r\n",
				shared.Cfg.Defaults.Image.Model,
				shared.Cfg.Defaults.Image.Size,
				shared.Cfg.Defaults.Image.Resolution,
				shared.Cfg.Defaults.Image.Quality)
		}
		fmt.Fprintf(os.Stderr, "[agent] req before config: model=%q size=%q quality=%q resolution=%q\r\n",
			req.Model, req.Size, req.Quality, req.Resolution)
	}
	if shared.Cfg != nil && shared.Cfg.Defaults != nil {
		imgCfg := shared.Cfg.Defaults.Image
		// Model: LLM doesn't provide, always from config
		if imgCfg != nil && imgCfg.Model != "" {
			req.Model = imgCfg.Model
		}
		// Quality: cap at config value (LLM can't choose more expensive)
		if imgCfg != nil && imgCfg.Quality != "" {
			req.Quality = imgCfg.Quality
		} else if req.Quality == "" {
			req.Quality = "low"
		}
		// Size: cap at config value
		if imgCfg != nil && imgCfg.Size != "" {
			req.Size = imgCfg.Size
		} else if req.Size == "" {
			req.Size = "1:1"
		}
		// Resolution
		if imgCfg != nil && imgCfg.Resolution != "" {
			req.Resolution = imgCfg.Resolution
		}
	} else {
		// No config — low-cost code defaults as safety net
		if req.Size == "" {
			req.Size = "1:1"
		}
		if req.Quality == "" {
			req.Quality = "low"
		}
	}
	if shared.Verbose {
		fmt.Fprintf(os.Stderr, "[agent] req after config: model=%q size=%q quality=%q resolution=%q\r\n",
			req.Model, req.Size, req.Quality, req.Resolution)
	}
	if req.Model == "" {
		return "Error: model is required for image generation (set via defaults.image.model in config.yaml)"
	}

	// Set timeout for image generation
	applyTimeout(c, "image", client.ImageTimeout)

	// Generate image via provider API — reuse existing client methods
	var savedFiles []string
	if isAPIMartProvider() {
		resp, err := c.Submit(req)
		if err != nil {
			return fmt.Sprintf("Error: image generation failed: %v", err)
		}
		if len(resp.Data) == 0 {
			return "Error: image submission returned no tasks"
		}
		taskData, err := c.PollTask(resp.Data[0].TaskID)
		if err != nil {
			return fmt.Sprintf("Error: image polling failed: %v", err)
		}
		if taskData.Result != nil && len(taskData.Result.Images) > 0 {
			if err := downloadImages(taskData.Result.Images, taskData.ID); err != nil {
				return fmt.Sprintf("Error: image download failed: %v", err)
			}
			savedFiles = append(savedFiles, fmt.Sprintf("image_%s_*", taskData.ID))
		}
	} else {
		resp, err := c.ImageGenerateSync(req)
		if err != nil {
			return fmt.Sprintf("Error: image generation failed: %v", err)
		}
		for i, img := range resp.Data {
			if img.B64JSON != "" {
				prefix := fmt.Sprintf("image_%d", time.Now().Unix())
				filename, err := service.SaveBase64Image(shared.OutputDir, prefix, img.B64JSON, i)
				if err != nil {
					continue
				}
				savedFiles = append(savedFiles, filename)
			} else if img.URL != "" {
				body, err := httpGet(img.URL)
				if err != nil {
					continue
				}
				ext := ".png"
				ts := time.Now().Unix()
				filename := fmt.Sprintf("image_%d_%d%s", ts, i, ext)
				dest := filename
				if shared.OutputDir != "" {
					dest = filepath.Join(shared.OutputDir, filename)
				}
				if err := os.WriteFile(dest, body, 0644); err != nil {
					continue
				}
				savedFiles = append(savedFiles, dest)
			}
		}
	}

	if len(savedFiles) == 0 {
		return "Error: image generation completed but no files were saved"
	}

	return fmt.Sprintf("Successfully generated %d image(s). Saved to: %s", len(savedFiles), strings.Join(savedFiles, ", "))
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

	// Apply config defaults — LLM-provided values are hints, config is the ceiling
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Video != nil {
		vidCfg := shared.Cfg.Defaults.Video
		if vidCfg.Model != "" {
			req.Model = vidCfg.Model
		}
		if vidCfg.Size != "" {
			req.Size = vidCfg.Size
		}
		if vidCfg.Resolution != "" {
			req.Resolution = vidCfg.Resolution
		}
		if vidCfg.Duration != nil {
			req.Duration = vidCfg.Duration
		}
	} else {
		// No config — low-cost code defaults as safety net
		if req.Size == "" {
			req.Size = "16:9"
		}
		if req.Resolution == "" {
			req.Resolution = "480p"
		}
	}
	if req.Model == "" {
		return "Error: model is required for video generation (set via defaults.video.model in config.yaml)"
	}

	// Set timeout for video generation
	applyTimeout(c, "video", client.VideoTimeout)

	var videoURLs []string
	if isOpenRouterProvider() {
		orReq := &types.OpenRouterVideoRequest{
			Model:  req.Model,
			Prompt: req.Prompt,
		}
		submitResp, err := c.OpenRouterVideoSubmit(orReq)
		if err != nil {
			return fmt.Sprintf("Error: video submission failed: %v", err)
		}
		pollResp, err := c.OpenRouterVideoPollUntilComplete(submitResp.PollingURL, 30*time.Second, 5*time.Minute)
		if err != nil {
			return fmt.Sprintf("Error: video polling failed: %v", err)
		}
		videoURLs = pollResp.UnsignedURLs
	} else {
		resp, err := c.VideoSubmit(req)
		if err != nil {
			return fmt.Sprintf("Error: video submission failed: %v", err)
		}
		if len(resp.Data) == 0 {
			return "Error: video submission returned no tasks"
		}
		taskData, err := c.PollTask(resp.Data[0].TaskID)
		if err != nil {
			return fmt.Sprintf("Error: video polling failed: %v", err)
		}
		if taskData.Result != nil {
			for _, vid := range taskData.Result.Videos {
				videoURLs = append(videoURLs, vid.URL...)
			}
		}
	}

	if len(videoURLs) == 0 {
		return "Error: video generation completed but no video URLs returned"
	}

	return fmt.Sprintf("Successfully generated %d video(s).", len(videoURLs))
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
