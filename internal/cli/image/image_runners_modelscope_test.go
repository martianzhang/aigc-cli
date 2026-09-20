package image

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestBuildModelScopeImageBody(t *testing.T) {
	dir := t.TempDir()
	path := writeLocalPNG(t, dir)

	t.Run("text to image sends no image_url", func(t *testing.T) {
		body, err := buildModelScopeImageBody(&types.GenerateRequest{
			Model:  "LZFlzf10203810/3dbaimo",
			Prompt: "baimo",
			Size:   "1024x768",
		})
		if err != nil {
			t.Fatalf("buildModelScopeImageBody() error = %v", err)
		}
		if _, ok := body["image_url"]; ok {
			t.Errorf("image_url should be absent for text-only requests")
		}
		if body["model"] != "LZFlzf10203810/3dbaimo" {
			t.Errorf("model = %v", body["model"])
		}
		if body["prompt"] != "baimo" {
			t.Errorf("prompt = %v", body["prompt"])
		}
		if body["size"] != "1024x768" {
			t.Errorf("size = %v", body["size"])
		}
	})

	t.Run("size omitted when empty", func(t *testing.T) {
		body, err := buildModelScopeImageBody(&types.GenerateRequest{Model: "m", Prompt: "p"})
		if err != nil {
			t.Fatalf("buildModelScopeImageBody() error = %v", err)
		}
		if _, ok := body["size"]; ok {
			t.Errorf("size should be absent when empty")
		}
	})

	t.Run("single local image becomes a data URI string", func(t *testing.T) {
		body, err := buildModelScopeImageBody(&types.GenerateRequest{
			Model:     "LZFlzf10203810/3dbaimo",
			Prompt:    "baimo",
			ImageURLs: []string{path},
		})
		if err != nil {
			t.Fatalf("buildModelScopeImageBody() error = %v", err)
		}
		got, ok := body["image_url"].(string)
		if !ok {
			t.Fatalf("image_url type = %T, want string", body["image_url"])
		}
		if !strings.HasPrefix(got, "data:image/png;base64,") {
			t.Errorf("image_url = %q, want data:image/png;base64,... prefix", got)
		}
	})

	t.Run("multiple local images become an array", func(t *testing.T) {
		body, err := buildModelScopeImageBody(&types.GenerateRequest{
			Model:     "LZFlzf10203810/3dbaimo",
			Prompt:    "baimo",
			ImageURLs: []string{path, path},
		})
		if err != nil {
			t.Fatalf("buildModelScopeImageBody() error = %v", err)
		}
		got, ok := body["image_url"].([]string)
		if !ok {
			t.Fatalf("image_url type = %T, want []string", body["image_url"])
		}
		if len(got) != 2 {
			t.Fatalf("image_url length = %d, want 2", len(got))
		}
		for i, u := range got {
			if !strings.HasPrefix(u, "data:image/png;base64,") {
				t.Errorf("image_url[%d] = %q, want data URI", i, u)
			}
		}
	})

	t.Run("public URL passes through unchanged", func(t *testing.T) {
		const url = "https://example.com/a.png"
		body, err := buildModelScopeImageBody(&types.GenerateRequest{
			Model:     "LZFlzf10203810/3dbaimo",
			Prompt:    "baimo",
			ImageURLs: []string{url},
		})
		if err != nil {
			t.Fatalf("buildModelScopeImageBody() error = %v", err)
		}
		if body["image_url"] != url {
			t.Errorf("image_url = %v, want %s", body["image_url"], url)
		}
	})

	t.Run("nonexistent path passes through unchanged", func(t *testing.T) {
		missing := filepath.Join(dir, "nope.png")
		body, err := buildModelScopeImageBody(&types.GenerateRequest{
			Model:     "m",
			Prompt:    "p",
			ImageURLs: []string{missing},
		})
		if err != nil {
			t.Fatalf("buildModelScopeImageBody() error = %v", err)
		}
		if body["image_url"] != missing {
			t.Errorf("image_url = %v, want passthrough %s", body["image_url"], missing)
		}
	})
}

func TestModelScopeTaskResponseErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		resp modelScopeTaskResponse
		want string
	}{
		{
			name: "structured errors.message wins over err_msg",
			resp: modelScopeTaskResponse{
				ErrMsg: "legacy",
				Errors: &modelScopeErrors{Code: 400, Message: "Output data may contain inappropriate content."},
			},
			want: "Output data may contain inappropriate content.",
		},
		{
			name: "falls back to err_msg",
			resp: modelScopeTaskResponse{ErrMsg: "boom"},
			want: "boom",
		},
		{
			name: "empty errors.message falls back to err_msg",
			resp: modelScopeTaskResponse{ErrMsg: "boom", Errors: &modelScopeErrors{Code: 400}},
			want: "boom",
		},
		{
			name: "nothing provided yields unknown error",
			resp: modelScopeTaskResponse{},
			want: "unknown error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.resp.errorMessage(); got != tt.want {
				t.Errorf("errorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildModelScopeImageBodyVerbatimRawJSON(t *testing.T) {
	raw := `{"model":"krea/Krea-2-Turbo","prompt":"p",` +
		`"loras":{"user/a":0.7,"user/b":0.3},"seed":12345,"steps":30,` +
		`"some_vendor_only_field":{"nested":[1,2]}}`
	body, err := buildModelScopeImageBody(&types.GenerateRequest{RawJSON: []byte(raw)})
	if err != nil {
		t.Fatalf("buildModelScopeImageBody() error = %v", err)
	}

	if body["seed"] != float64(12345) {
		t.Errorf("seed = %#v, want 12345", body["seed"])
	}
	if body["steps"] != float64(30) {
		t.Errorf("steps = %#v, want 30", body["steps"])
	}
	loras, ok := body["loras"].(map[string]interface{})
	if !ok {
		t.Fatalf("loras type = %T, want map", body["loras"])
	}
	if loras["user/a"] != 0.7 || loras["user/b"] != 0.3 {
		t.Errorf("loras = %#v, want unmodified 0.7/0.3", loras)
	}
	if _, ok := body["some_vendor_only_field"]; !ok {
		t.Errorf("unmodelled vendor field was dropped: %#v", body)
	}
}

func TestResolveRawImagePaths(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "in.png")
	if err := os.WriteFile(local, []byte("\x89PNG\r\n\x1a\nx"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	resolve := func(paths []string) ([]string, error) {
		out := make([]string, len(paths))
		for i, p := range paths {
			out[i] = "resolved:" + filepath.Base(p)
		}
		return out, nil
	}

	t.Run("scalar image_url", func(t *testing.T) {
		out, err := resolveRawImagePaths([]byte(`{"model":"m","image_url":"`+local+`"}`), resolve)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !strings.Contains(string(out), `"image_url":"resolved:in.png"`) {
			t.Errorf("got %s", out)
		}
	})

	t.Run("array image_urls", func(t *testing.T) {
		out, err := resolveRawImagePaths([]byte(`{"image_urls":["`+local+`"]}`), resolve)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !strings.Contains(string(out), `"image_urls":["resolved:in.png"]`) {
			t.Errorf("got %s", out)
		}
	})

	t.Run("remote url untouched", func(t *testing.T) {
		raw := []byte(`{"image_urls":["https://example.com/a.png"]}`)
		out, err := resolveRawImagePaths(raw, resolve)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if string(out) != string(raw) {
			t.Errorf("got %s, want unchanged", out)
		}
	})

	t.Run("unrelated keys preserved", func(t *testing.T) {
		out, err := resolveRawImagePaths(
			[]byte(`{"model":"m","seed":42,"image_urls":["`+local+`"]}`), resolve)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !strings.Contains(string(out), `"seed":42`) {
			t.Errorf("seed dropped: %s", out)
		}
	})
}

func TestResolveRawImagePathsNestedExtraBody(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "in.png")
	if err := os.WriteFile(local, []byte("\x89PNG\r\n\x1a\nx"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	resolve := func(paths []string) ([]string, error) {
		out := make([]string, len(paths))
		for i := range paths {
			out[i] = "resolved"
		}
		return out, nil
	}

	raw := []byte(`{"model":"m","prompt":"p","extra_body":{"image":["` + local + `"]}}`)
	out, err := resolveRawImagePaths(raw, resolve)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(string(out), `"image":["resolved"]`) {
		t.Errorf("nested extra_body.image not resolved: %s", out)
	}

	t.Run("absent extra_body is untouched", func(t *testing.T) {
		plain := []byte(`{"model":"m"}`)
		got, err := resolveRawImagePaths(plain, resolve)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if string(got) != string(plain) {
			t.Errorf("got %s, want unchanged", got)
		}
	})
}
