package chat

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

type chatTestCmd struct {
	root *cobra.Command
	cmd  *cobra.Command
}

func newChatTestCmd(t *testing.T) *chatTestCmd {
	t.Helper()
	chatSystem, chatMessages, chatTemperature, chatMaxTokens = "", nil, 0, 0
	chatNoStream, chatJSONFlag = false, ""
	options.Shared.Model = ""

	root := &cobra.Command{Use: "aigc-cli"}
	var dummyString string
	var dummyInt int
	var dummyBool bool
	root.PersistentFlags().StringVarP(&options.Shared.Model, "model", "m", "", "")
	root.PersistentFlags().StringVar(&dummyString, "config", "", "")
	root.PersistentFlags().StringVar(&dummyString, "api-key", "", "")
	root.PersistentFlags().StringVar(&dummyString, "api-base", "", "")
	root.PersistentFlags().StringVar(&dummyString, "http-proxy", "", "")
	root.PersistentFlags().StringVar(&dummyString, "provider", "", "")
	root.PersistentFlags().StringVar(&dummyString, "output", "", "")
	root.PersistentFlags().BoolVarP(&dummyBool, "verbose", "v", false, "")
	root.PersistentFlags().IntVar(&dummyInt, "timeout", 0, "")
	root.PersistentFlags().BoolVar(&dummyBool, "print-config", false, "")

	cmd := &cobra.Command{Use: "chat", SilenceUsage: true}
	f := cmd.Flags()
	f.StringVarP(&chatSystem, "system", "s", "", "")
	f.StringArrayVar(&chatMessages, "message", nil, "")
	f.Float64VarP(&chatTemperature, "temperature", "t", 0, "")
	f.IntVar(&chatMaxTokens, "max-output", 0, "")
	f.IntVar(&dummyInt, "context-size", 0, "")
	f.BoolVar(&chatNoStream, "no-stream", false, "")
	f.StringVar(&chatJSONFlag, "json", "", "")
	f.BoolVar(&dummyBool, "interactive", false, "")
	f.BoolVar(&dummyBool, "dry-run", false, "")
	root.AddCommand(cmd)

	t.Cleanup(func() {
		chatSystem, chatMessages, chatTemperature, chatMaxTokens = "", nil, 0, 0
		chatNoStream, chatJSONFlag = false, ""
		options.Shared.Model = ""
	})
	return &chatTestCmd{root: root, cmd: cmd}
}

func setTestFlag(t *testing.T, fs *pflag.FlagSet, name, value string) {
	t.Helper()
	if err := fs.Set(name, value); err != nil {
		t.Fatalf("set --%s=%s: %v", name, value, err)
	}
}

func decodeTestJSON(t *testing.T, data []byte) any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return v
}

func testFloat(t *testing.T, v any) float64 {
	t.Helper()
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			t.Fatalf("number %v: %v", n, err)
		}
		return f
	case float64:
		return n
	default:
		t.Fatalf("not a number: %#v", v)
		return 0
	}
}

func testInt(t *testing.T, v any) int {
	t.Helper()
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			t.Fatalf("number %v: %v", n, err)
		}
		return int(i)
	case float64:
		return int(n)
	default:
		t.Fatalf("not a number: %#v", v)
		return 0
	}
}

func assertTypedFromBody(t *testing.T, req *types.ChatRequest, body map[string]any) {
	t.Helper()
	if want, ok := body["model"]; ok && req.Model != want.(string) {
		t.Errorf("req.Model = %q, want %q", req.Model, want)
	}
	if want, ok := body["stream"]; ok && req.Stream != want.(bool) {
		t.Errorf("req.Stream = %v, want %v", req.Stream, want)
	}
	if want, ok := body["temperature"]; ok {
		if req.Temperature == nil {
			t.Errorf("req.Temperature = nil, want %v", want)
		} else if got, wantF := *req.Temperature, testFloat(t, want); got != wantF {
			t.Errorf("req.Temperature = %v, want %v", got, wantF)
		}
	}
	if want, ok := body["max_tokens"]; ok {
		if req.MaxTokens == nil {
			t.Errorf("req.MaxTokens = nil, want %v", want)
		} else if got, wantI := *req.MaxTokens, testInt(t, want); got != wantI {
			t.Errorf("req.MaxTokens = %d, want %d", got, wantI)
		}
	}
	if want, ok := body["messages"]; ok {
		raw, err := json.Marshal(req.Messages)
		if err != nil {
			t.Fatalf("marshal req.Messages: %v", err)
		}
		if got := decodeTestJSON(t, raw); !reflect.DeepEqual(got, want) {
			t.Errorf("req.Messages = %s, want %v", raw, want)
		}
	}
}

func TestBuildChatRequestJSONOverlay(t *testing.T) {
	tests := []struct {
		name      string
		jsonInput string
		local     [][2]string
		root      [][2]string
		wantBody  map[string]any
		wantRaw   string
	}{
		{
			name:      "json only is byte-identical",
			jsonInput: `{"model":"m","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high"}`,
			wantRaw:   `{"model":"m","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high"}`,
			wantBody: map[string]any{
				"model":            "m",
				"messages":         []any{map[string]any{"role": "user", "content": "hi"}},
				"reasoning_effort": "high",
			},
		},
		{
			name:      "temperature overrides json and keeps vendor keys",
			jsonInput: `{"model":"m","messages":[{"role":"user","content":"hi"}],"temperature":1,"reasoning_effort":"high"}`,
			local:     [][2]string{{"temperature", "0.7"}},
			wantBody: map[string]any{
				"model":            "m",
				"messages":         []any{map[string]any{"role": "user", "content": "hi"}},
				"temperature":      json.Number("0.7"),
				"reasoning_effort": "high",
			},
		},
		{
			name:      "root model flag overrides json model",
			jsonInput: `{"model":"from-json","messages":[{"role":"user","content":"hi"}]}`,
			root:      [][2]string{{"model", "gpt-5"}},
			wantBody: map[string]any{
				"model":    "gpt-5",
				"messages": []any{map[string]any{"role": "user", "content": "hi"}},
			},
		},
		{
			name:      "system and message rebuild the messages array",
			jsonInput: `{"model":"m","messages":[{"role":"user","content":"old"}],"reasoning_effort":"high"}`,
			local:     [][2]string{{"system", "be brief"}, {"message", "hello"}, {"message", "again"}},
			wantBody: map[string]any{
				"model":            "m",
				"reasoning_effort": "high",
				"messages": []any{
					map[string]any{"role": "system", "content": "be brief"},
					map[string]any{"role": "user", "content": "hello"},
					map[string]any{"role": "user", "content": "again"},
				},
			},
		},
		{
			name:      "no-stream writes false explicitly",
			jsonInput: `{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":true}`,
			local:     [][2]string{{"no-stream", "true"}},
			wantBody: map[string]any{
				"model":    "m",
				"messages": []any{map[string]any{"role": "user", "content": "hi"}},
				"stream":   false,
			},
		},
		{
			name:      "max-output sets max_tokens",
			jsonInput: `{"model":"m","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`,
			local:     [][2]string{{"max-output", "2048"}},
			wantBody: map[string]any{
				"model":      "m",
				"messages":   []any{map[string]any{"role": "user", "content": "hi"}},
				"max_tokens": json.Number("2048"),
			},
		},
		{
			name:      "all flags override and behavioral flags stay out of the body",
			jsonInput: `{"model":"from-json","messages":[{"role":"user","content":"old"}],"stream":true,"temperature":1,"max_tokens":10,"reasoning_effort":"high","vendor":{"nested":1}}`,
			local: [][2]string{
				{"system", "sys"},
				{"message", "one"},
				{"message", "two"},
				{"temperature", "0.7"},
				{"max-output", "2048"},
				{"no-stream", "true"},
				{"dry-run", "true"},
				{"interactive", "true"},
				{"context-size", "4096"},
			},
			root: [][2]string{
				{"model", "gpt-5"},
				{"provider", "openai"},
				{"verbose", "true"},
				{"timeout", "30"},
				{"api-key", "sk-test"},
				{"api-base", "https://example.test/v1"},
				{"http-proxy", "http://proxy.test"},
				{"output", "/tmp/out"},
				{"print-config", "true"},
				{"config", "/tmp/config.yaml"},
			},
			wantBody: map[string]any{
				"model": "gpt-5",
				"messages": []any{
					map[string]any{"role": "system", "content": "sys"},
					map[string]any{"role": "user", "content": "one"},
					map[string]any{"role": "user", "content": "two"},
				},
				"stream":           false,
				"temperature":      json.Number("0.7"),
				"max_tokens":       json.Number("2048"),
				"reasoning_effort": "high",
				"vendor":           map[string]any{"nested": json.Number("1")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := newChatTestCmd(t)
			setTestFlag(t, tc.cmd.Flags(), "json", tt.jsonInput)
			for _, kv := range tt.local {
				setTestFlag(t, tc.cmd.Flags(), kv[0], kv[1])
			}
			for _, kv := range tt.root {
				setTestFlag(t, tc.root.PersistentFlags(), kv[0], kv[1])
			}

			req, err := buildChatRequest(tc.cmd)
			if err != nil {
				t.Fatalf("buildChatRequest: %v", err)
			}
			if tt.wantRaw != "" && string(req.RawJSON) != tt.wantRaw {
				t.Errorf("RawJSON = %s, want byte-identical %s", req.RawJSON, tt.wantRaw)
			}
			if tt.wantBody != nil {
				if got := decodeTestJSON(t, req.RawJSON); !reflect.DeepEqual(got, tt.wantBody) {
					t.Errorf("body = %v, want %v", got, tt.wantBody)
				}
				assertTypedFromBody(t, req, tt.wantBody)
			}
		})
	}
}

func TestBuildChatRequestJSONFileOverlay(t *testing.T) {
	raw := `{"model":"m","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high"}`
	path := filepath.Join(t.TempDir(), "chat.json")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write json file: %v", err)
	}

	tc := newChatTestCmd(t)
	setTestFlag(t, tc.cmd.Flags(), "json", path)
	setTestFlag(t, tc.cmd.Flags(), "temperature", "0.7")

	req, err := buildChatRequest(tc.cmd)
	if err != nil {
		t.Fatalf("buildChatRequest: %v", err)
	}
	want := map[string]any{
		"model":            "m",
		"messages":         []any{map[string]any{"role": "user", "content": "hi"}},
		"reasoning_effort": "high",
		"temperature":      json.Number("0.7"),
	}
	if got := decodeTestJSON(t, req.RawJSON); !reflect.DeepEqual(got, want) {
		t.Errorf("body = %v, want %v", got, want)
	}
	assertTypedFromBody(t, req, want)
}

func TestBuildChatRequestFlagsOnly(t *testing.T) {
	tc := newChatTestCmd(t)
	setTestFlag(t, tc.root.PersistentFlags(), "model", "gpt-4o")
	setTestFlag(t, tc.cmd.Flags(), "system", "sys")
	setTestFlag(t, tc.cmd.Flags(), "message", "hi")
	setTestFlag(t, tc.cmd.Flags(), "temperature", "0.7")
	setTestFlag(t, tc.cmd.Flags(), "max-output", "100")
	setTestFlag(t, tc.cmd.Flags(), "no-stream", "true")

	req, err := buildChatRequest(tc.cmd)
	if err != nil {
		t.Fatalf("buildChatRequest: %v", err)
	}
	if req.RawJSON != nil {
		t.Errorf("RawJSON = %s, want nil without --json", req.RawJSON)
	}
	if req.Model != "gpt-4o" || req.Stream || req.Temperature == nil || *req.Temperature != 0.7 {
		t.Errorf("typed request mismatch: %+v", req)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 100 {
		t.Errorf("MaxTokens = %v, want 100", req.MaxTokens)
	}
	if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
		t.Errorf("Messages = %+v, want system+user", req.Messages)
	}
}
