package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestTencentPanoramaBody(t *testing.T) {
	t.Run("defaults the model and keeps a public image url", func(t *testing.T) {
		req := &types.GenerateRequest{Prompt: "p", ImageURLs: []string{"https://example.com/in.png"}}
		got := bodyOf(t, TencentPanoramaBody, req)
		if !strings.Contains(got, `"model":"hy-world2-panorama"`) {
			t.Errorf("body = %s, want the default model", got)
		}
		if !strings.Contains(got, `"images":["https://example.com/in.png"]`) {
			t.Errorf("body = %s, want the public image url", got)
		}
	})

	t.Run("prompt cap counts characters, not bytes", func(t *testing.T) {
		req := &types.GenerateRequest{Prompt: strings.Repeat("景", 601)}
		if _, err := TencentPanoramaBody(req); err == nil || !strings.Contains(err.Error(), "at most 600") {
			t.Fatalf("error = %v, want the 600-character cap", err)
		}
		if _, err := TencentPanoramaBody(&types.GenerateRequest{Prompt: strings.Repeat("a", 600)}); err != nil {
			t.Fatalf("600 ASCII characters must be accepted: %v", err)
		}
	})

	t.Run("local files and data uris are rejected", func(t *testing.T) {
		for _, image := range []string{"photo.png", "data:image/png;base64,AAAA"} {
			req := &types.GenerateRequest{Prompt: "p", ImageURLs: []string{image}}
			if _, err := TencentPanoramaBody(req); err == nil || !strings.Contains(err.Error(), "public image URL") {
				t.Errorf("image %q error = %v, want a public-URL rejection", image, err)
			}
		}
	})

	t.Run("verbatim json wins", func(t *testing.T) {
		raw := `{"model":"hy-world2-panorama","prompt":"p","images":["https://example.com/a.png"]}`
		req := &types.GenerateRequest{RawJSON: []byte(raw)}
		got := bodyOf(t, TencentPanoramaBody, req)
		if got != raw {
			t.Errorf("raw body = %s, want %s", got, raw)
		}
	})
}

func TestTencentPanoramaGenerate(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"i","created":1787287171,"status":"succeeded",`+
			`"data":[{"cover_image_url":"https://cdn/cover.png","super_resolution_image_url":"https://cdn/panorama.png"}],`+
			`"tokenhub_usage":{"total_tokens":500000}}`)
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "")
	resp, err := c.TencentPanoramaGenerate(&types.GenerateRequest{Prompt: "p"})
	if err != nil {
		t.Fatalf("TencentPanoramaGenerate() error = %v", err)
	}
	if gotPath != "/v1"+TencentPanoramaPath {
		t.Errorf("path = %q, want /v1%s", gotPath, TencentPanoramaPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if !strings.Contains(string(gotBody), `"model":"hy-world2-panorama"`) {
		t.Errorf("body = %s, want the default model", gotBody)
	}
	if len(resp.Data) != 1 || resp.Data[0].URL != "https://cdn/panorama.png" {
		t.Errorf("data = %+v, want the super-resolution URL", resp.Data)
	}
	if resp.Created != 1787287171 {
		t.Errorf("created = %d", resp.Created)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 500000 {
		t.Errorf("usage = %+v, want 500000 tokens", resp.Usage)
	}
}

func TestTencentPanoramaGenerateFailureStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"i","status":"failed","data":[]}`)
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "")
	if _, err := c.TencentPanoramaGenerate(&types.GenerateRequest{Prompt: "p"}); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("error = %v, want a failed-status error", err)
	}
}
