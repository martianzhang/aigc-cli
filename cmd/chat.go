package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/martianzhang/apimart-cli/internal/types"
)

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
	Use:   "chat",
	Short: "Chat with AI models (streaming by default)",
	Long: `Start a chat conversation with AI models via the APIMart API.

Supports all major models: GPT, Claude, Gemini, DeepSeek, and more.
Streaming output is enabled by default. Model is optional — API default is used when omitted.
Use --verbose to show token usage, cost, and timing stats.

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
	//   No --message → interactive (auto-detect)
	//   --interactive flag → interactive
	//   Otherwise → single-turn (existing behavior)
	isInteractive := chatInteractive || !cmd.Flags().Changed("message")

	if isInteractive {
		return runInteractiveChat(cmd)
	}

	// Single-turn mode (existing behavior)
	req, err := buildChatRequest(cmd)
	if err != nil {
		return err
	}
	return sendChatRequest(cmd, req)
}

// sendChatRequest applies defaults, sends the request, and prints output.
// Usage stats are shown only when --verbose is set.
func sendChatRequest(cmd *cobra.Command, req *types.ChatRequest) error {
	// Apply defaults
	if !cmd.Flags().Changed("stream") {
		req.Stream = true
	}

	// Merge config defaults
	if cfg, err := config.LoadDefaults(cfgFile); err == nil && cfg != nil && cfg.Defaults != nil && cfg.Defaults.Chat != nil {
		d := cfg.Defaults.Chat
		if d.Model != "" {
			req.Model = d.Model
		}
	}

	c := client.New(apiKey, apiBase, httpProxy)

	start := time.Now()
	result, err := c.ChatCompletion(req)
	if err != nil {
		return fmt.Errorf("chat failed: %w", err)
	}
	elapsed := time.Since(start)

	// Non-streaming: print result (streaming already printed by handleSSE)
	if !req.Stream && result != nil && len(result.Choices) > 0 {
		fmt.Println(result.Choices[0].Message.Content)
	}

	// Usage stats (to stderr, only with --verbose)
	if verbose {
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
func runInteractiveChat(cmd *cobra.Command) error {
	// Determine model (empty = use API default)
	if cfg, err := config.LoadDefaults(cfgFile); err == nil && cfg != nil && cfg.Defaults != nil && cfg.Defaults.Chat != nil {
		if model == "" && cfg.Defaults.Chat.Model != "" {
			model = cfg.Defaults.Chat.Model
		}
	}

	// Initialize conversation history
	history := []types.ChatMessage{}
	if chatSystem != "" {
		history = append(history, types.ChatMessage{Role: "system", Content: chatSystem})
	}

	c := client.New(apiKey, apiBase, httpProxy)
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
			modelDisplay := model
			if modelDisplay == "" {
				modelDisplay = "<API default>"
			}
			fmt.Fprintf(os.Stderr, "Model: %s | Stream: %v\r\n", modelDisplay, stream)
			if chatSystem != "" {
				fmt.Fprintf(os.Stderr, "System: %s\r\n", chatSystem)
			}
			fmt.Fprint(os.Stderr, "Use -v/--verbose to show token & timing stats.\r\n")
			continue
		}

		// Add user message to history
		history = append(history, types.ChatMessage{Role: "user", Content: input})

		// Build request
		req := &types.ChatRequest{
			Model:    model,
			Messages: history,
			Stream:   stream,
		}
		setFloatFlag(cmd, "temperature", &req.Temperature, chatTemperature)
		setIntFlag(cmd, "max-tokens", &req.MaxTokens, chatMaxTokens)

		// Send
		start := time.Now()
		result, err := c.ChatCompletion(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\r\nError: %v\r\n", err)
			// Remove failed user message from history to keep consistent state
			history = history[:len(history)-1]
			continue
		}
		elapsed := time.Since(start)

		// Append assistant response to history for subsequent turns
		if result != nil && len(result.Choices) > 0 {
			history = append(history, result.Choices[0].Message)
		}

		// Non-streaming: print result (streaming already printed by handleSSE)
		if !stream && result != nil && len(result.Choices) > 0 {
			fmt.Println(result.Choices[0].Message.Content)
		}

		// Verbose stats
		if verbose {
			printUsageStats(result, elapsed)
		}
		fmt.Fprint(os.Stderr, "\r\n")
	}
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
		Model:    model,
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
