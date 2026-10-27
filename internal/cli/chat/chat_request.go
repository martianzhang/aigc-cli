package chat

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// chatStdin is os.Stdin by default; tests replace it with a fixed reader.
var chatStdin io.Reader = os.Stdin

// chatStdinIsTerminal reports whether stdin is an interactive terminal. It is
// a var so tests can force the piped path.
var chatStdinIsTerminal = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// readStdinMessage returns piped stdin as a user message. It returns "" for an
// interactive terminal (the TUI owns stdin) and for empty input.
func readStdinMessage() (string, error) {
	if chatStdinIsTerminal() {
		return "", nil
	}
	data, err := io.ReadAll(chatStdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// buildChatRequest constructs a ChatRequest from --json or individual flags.
func buildChatRequest(cmd *cobra.Command) (*types.ChatRequest, error) {
	inject, err := service.ReadInjectJSON(chatInjectJSON)
	if err != nil {
		return nil, err
	}
	chatInjectRaw = inject
	if chatJSONFlag != "" {
		data, err := service.ReadJSONInput(chatJSONFlag)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		if err := rejectJSONContentParts(data); err != nil {
			return nil, err
		}
		req := &types.ChatRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}

		set := map[string]any{}
		if options.HasFlagChanged(cmd, "model") {
			set["model"] = options.Shared.Model
		}
		if options.HasFlagChanged(cmd, "system") || messageFlagChanged(cmd) {
			messages, err := flagMessages()
			if err != nil {
				return nil, err
			}
			if err := rejectJSONAttachments(messages); err != nil {
				return nil, err
			}
			set["messages"] = messages
		}
		if options.HasFlagChanged(cmd, "no-stream") {
			set["stream"] = !chatNoStream
		}
		if options.HasFlagChanged(cmd, "temperature") {
			set["temperature"] = chatTemperature
		}
		if options.HasFlagChanged(cmd, "max-output") {
			set["max_tokens"] = chatMaxTokens
		}

		merged, err := service.MergeJSONOverlay(data, set)
		if err != nil {
			return nil, fmt.Errorf("failed to merge flags into JSON input: %w", err)
		}
		req.RawJSON = merged
		if err := json.Unmarshal(merged, req); err != nil {
			return nil, fmt.Errorf("failed to parse merged JSON: %w", err)
		}
		req.InjectJSON = inject
		return req, nil
	}

	messages, err := flagMessages()
	if err != nil {
		return nil, err
	}
	req := &types.ChatRequest{
		Model:        options.Shared.Model,
		Messages:     messages,
		Stream:       !chatNoStream,
		OutputWriter: options.Stdout(),
	}
	// Piped stdin becomes one additional user message, appended after any
	// --message values. This makes `echo "..." | aigc-cli chat --message "..."`
	// combine both inputs instead of silently dropping stdin. --json owns
	// stdin in its own branch above, and a terminal stdin (the TUI) is skipped.
	text, err := readStdinMessage()
	if err != nil {
		return nil, err
	}
	if text != "" {
		expanded, images, warnings := expandMentions(text, readDenyPatterns())
		for _, warning := range warnings {
			fmt.Fprintf(options.Stderr(), "⚠️  %s\n", warning)
		}
		req.Messages = append(req.Messages, types.ChatMessage{
			Role:    "user",
			Content: expanded,
			Images:  images,
		})
	}
	if cmd.Flags().Changed("temperature") {
		v := chatTemperature
		req.Temperature = &v
	}
	options.SetIntFlag(cmd, "max-output", &req.MaxTokens, chatMaxTokens)

	req.InjectJSON = inject
	return req, nil
}

// rejectJSONAttachments refuses an image @-mention combined with --json.
//
// --json sends the request body verbatim (RawJSON wins over the typed fields),
// so an attachment has nowhere to go: merging the messages back into the raw
// body round-trips them through JSON text, where an image becomes a content
// parts array that ChatMessage.Content (a string) cannot accept. Rejecting the
// combination here replaces an opaque decode failure with an actionable error.
func rejectJSONAttachments(messages []types.ChatMessage) error {
	for _, m := range messages {
		if len(m.Images) == 0 {
			continue
		}
		return fmt.Errorf(
			"--json sends the request body verbatim, so an image attachment (%s) cannot be added; "+
				"drop --json and attach it with --message \"... @%s\" instead",
			m.Images[0].Name, m.Images[0].Name)
	}
	return nil
}

// rejectJSONContentParts refuses a --json body whose message content is a
// multimodal parts array. The typed ChatMessage models string content only, so
// such a body would fail deep inside the decode with a bare Go type error;
// --inject-json is not typed and appends the same parts verbatim.
func rejectJSONContentParts(data []byte) error {
	var probe struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil // malformed bodies are reported by the caller
	}
	for i, m := range probe.Messages {
		if len(m.Content) > 0 && m.Content[0] == '[' {
			return fmt.Errorf(
				"--json does not support multimodal content parts (messages[%d].content is an array); "+
					"pass them with --inject-json instead", i)
		}
	}
	return nil
}

func flagMessages() ([]types.ChatMessage, error) {
	messages := make([]types.ChatMessage, 0, len(chatMessages)+1)
	if chatSystem != "" {
		messages = append(messages, types.ChatMessage{Role: "system", Content: chatSystem})
	}
	deny := readDenyPatterns()
	for _, msg := range chatMessages {
		text, err := service.ResolvePromptFile(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to read message file: %w", err)
		}
		expanded, images, warnings := expandMentions(text, deny)
		for _, warning := range warnings {
			fmt.Fprintf(options.Stderr(), "⚠️  %s\n", warning)
		}
		messages = append(messages, types.ChatMessage{Role: "user", Content: expanded, Images: images})
	}
	return messages, nil
}

// sendChatRequest sends a single chat request and prints the response.
func sendChatRequest(_ *cobra.Command, req *types.ChatRequest) error {
	// Streaming default: on, unless --no-stream was passed
	if !chatNoStream {
		req.Stream = true
	}

	// Merge config defaults (only if not already set via --model flag or JSON)
	if req.Model == "" {
		if cfg := options.ChatDefaults(); cfg != nil && cfg.Model != "" {
			req.Model = cfg.Model
		}
	}

	p := options.Shared.ResolveProvider(options.ProviderNameChat)

	if chatDryRun {
		fmt.Println(buildChatCurl(req, p))
		return nil
	}

	c := client.NewFromProvider(p)
	req.OutputWriter = options.Stdout()

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

	// Web-search citations (to stderr, when the provider attached any)
	printSources(result)

	// Usage stats (to stderr, only with --verbose)
	if options.Shared.Verbose {
		printUsageStats(result, elapsed, p)
	}

	return nil
}

// buildChatCurl renders the equivalent curl for a chat request, using the
// resolved provider host and the same body the client sends.
func buildChatCurl(req *types.ChatRequest, p *provider.EffectiveProvider) string {
	base := client.NormalizeBaseURL(p.BaseURL)
	url := base + client.ChatPath
	body, _ := client.ChatBody(req, p.ZDR, p.BaseURL)
	auth := fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", service.MaskKey(p.APIKey))

	switch p.Type {
	case types.ProviderAnthropic:
		url = base + client.AnthropicChatPath
		body, _ = client.AnthropicChatBody(req)
		auth = fmt.Sprintf("  -H \"x-api-key: %s\" \\\n", service.MaskKey(p.APIKey))
		auth += fmt.Sprintf("  -H \"anthropic-version: %s\" \\\n", client.AnthropicVersion)
	case types.ProviderOpenAIResponses:
		url = base + client.ResponsesPath
		body, _ = client.ResponsesBody(req)
	}

	cmd := fmt.Sprintf("curl -X POST %s \\\n", url)
	cmd += auth
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(body))
	return cmd
}

// printUsageStats prints token/cost/timing stats to stderr. p identifies the
// provider so vendor-only counters (MiMo web search) are only shown for it.
func printUsageStats(result *types.ChatResponse, elapsed time.Duration, p *provider.EffectiveProvider) {
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
		if line := webSearchLine(p, result.Usage); line != "" {
			parts = append(parts, line)
		}
	}
	parts = append(parts, fmt.Sprintf("Time: %v", elapsed.Round(time.Millisecond)))
	fmt.Fprintln(options.Stderr(), "---  "+strings.Join(parts, "  |  "))
}

// toURLs converts a single URL string to a slice (for MJ API compatibility).
func toURLs(url string) []string {
	if url == "" {
		return nil
	}
	return []string{url}
}
