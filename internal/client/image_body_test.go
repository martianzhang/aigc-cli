package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

const rawImageJSON = `{"model":"m","prompt":"p","loras":"u/r","seed":1,"custom_x":9}`

func rawImageReq() *types.GenerateRequest {
	return &types.GenerateRequest{RawJSON: []byte(rawImageJSON)}
}

func marshalBody(t *testing.T, body interface{}) string {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return string(data)
}

func TestImageBodyBuildersForwardRawJSON(t *testing.T) {
	tests := []struct {
		name string
		body func(*types.GenerateRequest) interface{}
	}{
		{"openrouter", OpenRouterImageBody},
		{"gemini", GeminiImageBody},
		{"edits", ImageEditsBody},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := marshalBody(t, tc.body(rawImageReq())); got != rawImageJSON {
				t.Errorf("raw --json body = %s, want %s", got, rawImageJSON)
			}
		})
	}
}

func TestImageBodyBuildersMapTypedFields(t *testing.T) {
	req := &types.GenerateRequest{
		Model:     "m",
		Prompt:    "p",
		Size:      "1024x768",
		ImageURLs: []string{"photo.png"},
	}

	if got := marshalBody(t, OpenRouterImageBody(req)); !strings.Contains(got, `"input_references"`) {
		t.Errorf("openrouter typed body should map image_urls to input_references, got %s", got)
	}
	if got := marshalBody(t, ImageEditsBody(req)); !strings.Contains(got, `"images":[{"image_url":"photo.png"}]`) {
		t.Errorf("edits typed body should use images[].image_url, got %s", got)
	}
	if got := marshalBody(t, GeminiImageBody(req)); !strings.Contains(got, `"response_format"`) || !strings.Contains(got, `"input"`) {
		t.Errorf("gemini typed body should carry input/response_format, got %s", got)
	}
	if got := marshalBody(t, ImageEditsBody(req)); strings.Contains(got, "loras") {
		t.Errorf("typed body must not invent loras, got %s", got)
	}
}

func TestImagePathsSendRawJSONVerbatim(t *testing.T) {
	editsReq := rawImageReq()
	editsReq.ImageURLs = []string{"photo.png"}

	tests := []struct {
		name     string
		req      *types.GenerateRequest
		wantPath string
		call     func(*Client, *types.GenerateRequest) error
	}{
		{
			"openrouter posts raw body to /images",
			rawImageReq(),
			"/v1/images",
			func(c *Client, req *types.GenerateRequest) error {
				_, err := c.OpenRouterDedicatedImage(req)
				return err
			},
		},
		{
			"gemini posts raw body to /interactions",
			rawImageReq(),
			"/v1/interactions",
			func(c *Client, req *types.GenerateRequest) error {
				_, err := c.GeminiImageGenerate(req)
				return err
			},
		},
		{
			"edits posts raw body to /images/edits",
			editsReq,
			"/v1/images/edits",
			func(c *Client, req *types.GenerateRequest) error {
				_, err := c.ImageGenerateEdits(req)
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				b, _ := io.ReadAll(r.Body)
				gotBody = string(b)
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"created":1,"data":[{"b64_json":"dGVzdA=="}]}`))
			}))
			defer srv.Close()

			c := New("test-key", srv.URL, "")
			_ = tc.call(c, tc.req)

			if gotPath != tc.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tc.wantPath)
			}
			if gotBody != rawImageJSON {
				t.Errorf("body = %s, want %s", gotBody, rawImageJSON)
			}
		})
	}
}
