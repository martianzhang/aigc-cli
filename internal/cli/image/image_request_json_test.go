package image

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
)

// newJSONImageCmd mirrors the production tree: --model is a root persistent
// flag, the image flags are local. registerImageGenerateFlags rebinds the flag
// globals, so each test starts from clean defaults. Registering on a throwaway
// command in Cleanup resets them again, so flags like --mode cannot leak.
func newJSONImageCmd(t *testing.T) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "aigc-cli"}
	root.PersistentFlags().StringVar(&options.Shared.Model, "model", "", "")
	cmd := &cobra.Command{Use: "image", SilenceUsage: true}
	registerImageGenerateFlags(cmd)
	root.AddCommand(cmd)

	t.Cleanup(func() {
		registerImageGenerateFlags(&cobra.Command{Use: "reset"})
		options.Shared.Model = ""
	})
	return cmd
}

func decodeRawBody(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("decode RawJSON: %v\nraw: %s", err, raw)
	}
	return body
}

func TestBuildImageRequestJSONFlagOverlay(t *testing.T) {
	const input = `{"prompt":"json prompt","model":"json-model","size":"1:1","n":2,` +
		`"loras":{"user/a":0.7},"seed":12345,"steps":30,"vendor_blob":{"x":[1,2]}}`

	vendorWant := map[string]any{
		"loras":       map[string]any{"user/a": json.Number("0.7")},
		"seed":        json.Number("12345"),
		"steps":       json.Number("30"),
		"vendor_blob": map[string]any{"x": []any{json.Number("1"), json.Number("2")}},
	}

	tests := []struct {
		name    string
		flags   map[string]string
		wantKey string
		wantVal any
	}{
		{"prompt", map[string]string{"prompt": "flag prompt"}, "prompt", "flag prompt"},
		{"size", map[string]string{"size": "16:9"}, "size", "16:9"},
		{"ratio", map[string]string{"ratio": "4:3"}, "ratio", "4:3"},
		{"resolution", map[string]string{"resolution": "2k"}, "resolution", "2k"},
		{"quality", map[string]string{"quality": "high"}, "quality", "high"},
		{"background", map[string]string{"background": "transparent"}, "background", "transparent"},
		{"moderation", map[string]string{"moderation": "low"}, "moderation", "low"},
		{"output-format", map[string]string{"output-format": "webp"}, "output_format", "webp"},
		{"output-compression", map[string]string{"output-compression": "80"}, "output_compression", json.Number("80")},
		{"n", map[string]string{"n": "3"}, "n", json.Number("3")},
		{"image-url", map[string]string{"image-url": "https://example.com/a.png"}, "image_urls", []any{"https://example.com/a.png"}},
		{"mask-url", map[string]string{"mask-url": "https://example.com/mask.png"}, "mask_url", "https://example.com/mask.png"},
		{"style", map[string]string{"style": "vivid"}, "style", "vivid"},
		{"response-format", map[string]string{"response-format": "b64_json"}, "response_format", "b64_json"},
		{"model root persistent", map[string]string{"model": "flag-model"}, "model", "flag-model"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newJSONImageCmd(t)
			for name, val := range tc.flags {
				target := cmd.Flags()
				if name == "model" {
					target = cmd.Parent().PersistentFlags()
				}
				if err := target.Set(name, val); err != nil {
					t.Fatalf("set --%s: %v", name, err)
				}
			}
			options.Shared.JSONInput = input

			req, err := buildImageRequest(cmd)
			if err != nil {
				t.Fatalf("buildImageRequest() error = %v", err)
			}

			body := decodeRawBody(t, req.RawJSON)
			if got := body[tc.wantKey]; !reflect.DeepEqual(got, tc.wantVal) {
				t.Errorf("key %q = %#v, want %#v", tc.wantKey, got, tc.wantVal)
			}
			for key, want := range vendorWant {
				if got := body[key]; !reflect.DeepEqual(got, want) {
					t.Errorf("vendor key %q = %#v, want %#v", key, got, want)
				}
			}

			wantPrompt := "json prompt"
			if v, ok := tc.flags["prompt"]; ok {
				wantPrompt = v
			}
			if got := body["prompt"]; got != wantPrompt {
				t.Errorf("RawJSON prompt = %#v, want %q", got, wantPrompt)
			}
			if req.Prompt != wantPrompt {
				t.Errorf("req.Prompt = %q, want %q", req.Prompt, wantPrompt)
			}
			wantSize := "1:1"
			if v, ok := tc.flags["size"]; ok {
				wantSize = v
			}
			if req.Size != wantSize {
				t.Errorf("req.Size = %q, want %q", req.Size, wantSize)
			}
			wantN := 2
			if v, ok := tc.flags["n"]; ok {
				wantN, _ = strconv.Atoi(v)
			}
			if req.N == nil || *req.N != wantN {
				t.Errorf("req.N = %v, want %d", req.N, wantN)
			}
			wantModel := "json-model"
			if v, ok := tc.flags["model"]; ok {
				wantModel = v
			}
			if req.Model != wantModel {
				t.Errorf("req.Model = %q, want %q", req.Model, wantModel)
			}
		})
	}
}

func TestBuildImageRequestJSONNoFlagsIsByteIdentical(t *testing.T) {
	inputs := []string{
		`{"prompt":"p","seed":1.0,"big":12345678901234567890,"html":"<b>&</b>"}`,
		`{"prompt":"p","nested":{"a":[1,2,3]},"unicode":"中文","loras":{"u/r":0.25}}`,
	}
	for i, input := range inputs {
		t.Run("case "+strconv.Itoa(i), func(t *testing.T) {
			cmd := newJSONImageCmd(t)
			options.Shared.JSONInput = input

			req, err := buildImageRequest(cmd)
			if err != nil {
				t.Fatalf("buildImageRequest() error = %v", err)
			}
			if !bytes.Equal(req.RawJSON, []byte(input)) {
				t.Errorf("RawJSON changed:\n got: %s\nwant: %s", req.RawJSON, input)
			}
			if req.Prompt != "p" {
				t.Errorf("req.Prompt = %q, want %q", req.Prompt, "p")
			}
		})
	}
}

func TestBuildImageRequestJSONPromptFromFlag(t *testing.T) {
	cmd := newJSONImageCmd(t)
	if err := cmd.Flags().Set("prompt", "x"); err != nil {
		t.Fatalf("set --prompt: %v", err)
	}
	options.Shared.JSONInput = `{"model":"m","seed":1}`

	req, err := buildImageRequest(cmd)
	if err != nil {
		t.Fatalf("buildImageRequest() error = %v", err)
	}
	if req.Prompt != "x" {
		t.Errorf("req.Prompt = %q, want %q", req.Prompt, "x")
	}
	if body := decodeRawBody(t, req.RawJSON); body["prompt"] != "x" {
		t.Errorf("RawJSON prompt = %#v, want %q", body["prompt"], "x")
	}
}

func TestBuildImageRequestJSONStillRequiresPrompt(t *testing.T) {
	cmd := newJSONImageCmd(t)
	options.Shared.JSONInput = `{"model":"m"}`

	_, err := buildImageRequest(cmd)
	if err == nil || !strings.Contains(err.Error(), "prompt is required in JSON input") {
		t.Fatalf("error = %v, want prompt-required JSON error", err)
	}
}

func TestBuildImageRequestJSONBehavioralFlagsIgnored(t *testing.T) {
	const input = `{"prompt":"p","seed":7}`
	cmd := newJSONImageCmd(t)
	for name, val := range map[string]string{
		"dry-run":     "true",
		"preview":     "true",
		"decode":      "true",
		"edit":        "true",
		"mode":        "sync",
		"save-prompt": "true",
		"compress":    "800KB",
	} {
		if err := cmd.Flags().Set(name, val); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	options.Shared.JSONInput = input

	req, err := buildImageRequest(cmd)
	if err != nil {
		t.Fatalf("buildImageRequest() error = %v", err)
	}
	if !bytes.Equal(req.RawJSON, []byte(input)) {
		t.Errorf("behavioral flags changed the body:\n got: %s\nwant: %s", req.RawJSON, input)
	}
}

func TestResolveRawImagePathsPreservesFidelity(t *testing.T) {
	local := writeLocalPNG(t, t.TempDir())
	resolve := func(paths []string) ([]string, error) {
		out := make([]string, len(paths))
		for i := range paths {
			out[i] = "resolved"
		}
		return out, nil
	}

	raw := []byte(`{"image_urls":["` + local + `"],"seed":1.0,"big":12345678901234567890,"html":"<b>&</b>"}`)
	out, err := resolveRawImagePaths(raw, resolve)
	if err != nil {
		t.Fatalf("resolveRawImagePaths() error = %v", err)
	}
	for _, want := range []string{
		`"seed":1.0`,
		`"big":12345678901234567890`,
		`"html":"<b>&</b>"`,
		`"image_urls":["resolved"]`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output missing %s:\n%s", want, out)
		}
	}
}
