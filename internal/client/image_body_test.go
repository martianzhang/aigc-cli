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

type geminiInputProbe struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Data     string `json:"data"`
	MimeType string `json:"mime_type"`
	URI      string `json:"uri"`
}

type geminiBodyProbe struct {
	Input []geminiInputProbe `json:"input"`
}

func decodeGeminiBody(t *testing.T, req *types.GenerateRequest) geminiBodyProbe {
	t.Helper()
	var probe geminiBodyProbe
	if err := json.Unmarshal([]byte(marshalBody(t, GeminiImageBody(req))), &probe); err != nil {
		t.Fatalf("unmarshal gemini body: %v", err)
	}
	return probe
}

func TestGeminiImageBodyMapsReferenceImages(t *testing.T) {
	const rawB64 = "aGVsbG8="

	t.Run("data uri becomes raw base64 with mime type", func(t *testing.T) {
		req := &types.GenerateRequest{
			Prompt:    "p",
			ImageURLs: []string{"data:image/png;base64," + rawB64},
		}
		body := marshalBody(t, GeminiImageBody(req))
		if want := `{"type":"image","data":"` + rawB64 + `","mime_type":"image/png"}`; !strings.Contains(body, want) {
			t.Errorf("gemini body = %s, want image item %s", body, want)
		}

		input := decodeGeminiBody(t, req).Input
		if len(input) != 2 {
			t.Fatalf("input length = %d, want 2 (text + image)", len(input))
		}
		img := input[1]
		if img.Type != "image" || img.Data != rawB64 || img.MimeType != "image/png" || img.URI != "" {
			t.Errorf("image item = %+v, want type=image data=%s mime_type=image/png uri empty", img, rawB64)
		}
		if strings.HasPrefix(img.Data, "data:") {
			t.Errorf("image data = %q, must be raw base64 without data: prefix", img.Data)
		}
	})

	t.Run("https url becomes uri", func(t *testing.T) {
		req := &types.GenerateRequest{
			Prompt:    "p",
			ImageURLs: []string{"https://example.com/a.png"},
		}
		body := marshalBody(t, GeminiImageBody(req))
		if want := `{"type":"image","uri":"https://example.com/a.png"}`; !strings.Contains(body, want) {
			t.Errorf("gemini body = %s, want image item %s", body, want)
		}

		input := decodeGeminiBody(t, req).Input
		if len(input) != 2 {
			t.Fatalf("input length = %d, want 2 (text + image)", len(input))
		}
		img := input[1]
		if img.Type != "image" || img.URI != "https://example.com/a.png" || img.Data != "" || img.MimeType != "" {
			t.Errorf("image item = %+v, want type=image uri=https://example.com/a.png", img)
		}
	})

	t.Run("text-only request keeps a single text item", func(t *testing.T) {
		req := &types.GenerateRequest{Prompt: "p"}
		input := decodeGeminiBody(t, req).Input
		if len(input) != 1 {
			t.Fatalf("input length = %d, want 1 (text only)", len(input))
		}
		if input[0].Type != "text" || input[0].Text != "p" {
			t.Errorf("text item = %+v, want type=text text=p", input[0])
		}
	})

	t.Run("multiple references keep order", func(t *testing.T) {
		const firstB64 = "Zmlyc3Q="
		second := "https://example.com/b.png"
		req := &types.GenerateRequest{
			Prompt:    "p",
			ImageURLs: []string{"data:image/jpeg;base64," + firstB64, second},
		}
		input := decodeGeminiBody(t, req).Input
		if len(input) != 3 {
			t.Fatalf("input length = %d, want 3 (text + two images)", len(input))
		}
		if input[1].Type != "image" || input[1].Data != firstB64 || input[1].MimeType != "image/jpeg" {
			t.Errorf("first image = %+v, want raw base64 %s with mime_type=image/jpeg", input[1], firstB64)
		}
		if input[2].Type != "image" || input[2].URI != second {
			t.Errorf("second image = %+v, want uri=%s", input[2], second)
		}
	})
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
