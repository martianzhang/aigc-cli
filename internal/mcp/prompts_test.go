package mcp

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// promptTestConfig returns a minimal Config accepted by NewServer.
func promptTestConfig() *Config {
	return &Config{
		BaseURL:  "https://api.openai.com/v1",
		Defaults: &types.ConfigDefaults{},
	}
}

// promptHandler returns the handler registered for a prompt name.
func promptHandler(t *testing.T, s *server.MCPServer, name string) server.PromptHandlerFunc {
	t.Helper()
	entry, ok := s.ListPrompts()[name]
	if !ok {
		t.Fatalf("prompt %q is not registered", name)
	}
	if entry.Handler == nil {
		t.Fatalf("prompt %q has a nil handler", name)
	}
	return entry.Handler
}

// promptResultText extracts the text of the first message of a prompt result.
func promptResultText(t *testing.T, res *mcp.GetPromptResult) string {
	t.Helper()
	if res == nil {
		t.Fatal("expected non-nil prompt result")
	}
	if len(res.Messages) == 0 {
		t.Fatal("expected at least one prompt message")
	}
	if res.Messages[0].Role != mcp.RoleUser {
		t.Errorf("expected user role, got %q", res.Messages[0].Role)
	}
	content, ok := res.Messages[0].Content.(mcp.TextContent)
	if !ok {
		t.Fatalf("expected mcp.TextContent, got %T", res.Messages[0].Content)
	}
	return content.Text
}

// callPrompt invokes a registered prompt handler with the given arguments.
func callPrompt(t *testing.T, s *server.MCPServer, name string, args map[string]string) string {
	t.Helper()
	res, err := promptHandler(t, s, name)(context.Background(), mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{Name: name, Arguments: args},
	})
	if err != nil {
		t.Fatalf("prompt %q returned error: %v", name, err)
	}
	return promptResultText(t, res)
}

func requireContains(t *testing.T, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, text)
		}
	}
}

func TestPrompts_registered(t *testing.T) {
	s := NewServer(promptTestConfig())
	prompts := s.ListPrompts()
	if len(prompts) != 4 {
		t.Fatalf("expected 4 registered prompts, got %d", len(prompts))
	}

	type wantArg struct {
		arg      string
		required bool
	}
	cases := []struct {
		name string
		args []wantArg
	}{
		{"generate_product_shot", []wantArg{{"product", true}, {"style", false}, {"aspect_ratio", false}}},
		{"detect_ai_image", []wantArg{{"file_path", true}}},
		{"image_to_video", []wantArg{{"image_url", true}, {"prompt", false}}},
		{"remove_image_background", []wantArg{{"file_path", true}, {"replace_color", false}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry, ok := prompts[tc.name]
			if !ok {
				t.Fatalf("prompt %q is not registered", tc.name)
			}
			if entry.Prompt.Name != tc.name {
				t.Errorf("expected name %q, got %q", tc.name, entry.Prompt.Name)
			}
			if entry.Prompt.Title == "" {
				t.Error("expected non-empty title")
			}
			if entry.Prompt.Description == "" {
				t.Error("expected non-empty description")
			}
			if len(entry.Prompt.Arguments) != len(tc.args) {
				t.Fatalf("expected %d arguments, got %d", len(tc.args), len(entry.Prompt.Arguments))
			}
			for i, want := range tc.args {
				got := entry.Prompt.Arguments[i]
				if got.Name != want.arg {
					t.Errorf("argument %d: expected %q, got %q", i, want.arg, got.Name)
				}
				if got.Required != want.required {
					t.Errorf("argument %q: expected required=%t, got %t", want.arg, want.required, got.Required)
				}
				if got.Description == "" {
					t.Errorf("argument %q: expected non-empty description", want.arg)
				}
			}
		})
	}
}

func TestPrompts_handlersNameToolAndSubstituteArgs(t *testing.T) {
	s := NewServer(promptTestConfig())
	cases := []struct {
		name     string
		args     map[string]string
		wantTool string
		wantText []string
	}{
		{
			name:     "generate_product_shot",
			args:     map[string]string{"product": "ceramic coffee mug", "style": "minimalist", "aspect_ratio": "4:5"},
			wantTool: "generate_image",
			wantText: []string{"ceramic coffee mug", "minimalist", "4:5", "studio lighting", "seamless background"},
		},
		{
			name:     "detect_ai_image",
			args:     map[string]string{"file_path": "/tmp/ai-photo.jpg"},
			wantTool: "detect_image",
			wantText: []string{"/tmp/ai-photo.jpg", "C2PA", "TC260", "SynthID", "FFT", "ONNX", "watermark"},
		},
		{
			name:     "image_to_video",
			args:     map[string]string{"image_url": "https://example.com/still.png", "prompt": "slow dolly-in"},
			wantTool: "generate_video",
			wantText: []string{"https://example.com/still.png", "slow dolly-in"},
		},
		{
			name:     "remove_image_background",
			args:     map[string]string{"file_path": "/tmp/portrait.png", "replace_color": "#00FF00"},
			wantTool: "remove_background",
			wantText: []string{"/tmp/portrait.png", "#00FF00"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := callPrompt(t, s, tc.name, tc.args)
			requireContains(t, text, "`"+tc.wantTool+"`")
			requireContains(t, text, tc.wantText...)
		})
	}
}

func TestPrompts_missingRequiredArgStillReturnsTemplate(t *testing.T) {
	s := NewServer(promptTestConfig())
	cases := []struct {
		name        string
		placeholder string
	}{
		{"generate_product_shot", "<product>"},
		{"detect_ai_image", "<file_path>"},
		{"image_to_video", "<image_url>"},
		{"remove_image_background", "<file_path>"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := callPrompt(t, s, tc.name, map[string]string{})
			requireContains(t, text, tc.placeholder, "Ask the user")
		})
	}
}

func TestListPrompts_printsAllNames(t *testing.T) {
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	ListPrompts(&Config{})

	os.Stdout = origStdout
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read pipe: %v", err)
	}
	r.Close()

	got := string(out)
	var wants []string
	for _, info := range promptRegistry {
		wants = append(wants, info.prompt.Name, info.prompt.Description)
	}
	requireContains(t, got, wants...)
}
