package image

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// writeLocalPNG writes a tiny decodable PNG (the data-URI gate validates it).
func writeLocalPNG(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "photo.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return path
}

func mustBuildPlan(t *testing.T, req *types.GenerateRequest, p *provider.EffectiveProvider) *imagePlan {
	t.Helper()
	pl, err := buildImagePlan(req, p)
	if err != nil {
		t.Fatalf("buildImagePlan() error = %v", err)
	}
	return pl
}

func planBodyJSON(t *testing.T, body any) string {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return strings.TrimRight(buf.String(), "\n")
}

func uploadServer(t *testing.T) *httptest.Server {
	t.Helper()
	var count int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&count, 1) - 1
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"url":"https://cdn.example.com/up%d.png"}`, n)
	}))
}

// TestBuildImagePlanRouting pins URL, body image value, upload count and note
// for every provider so preview and execution cannot drift.
func TestBuildImagePlanRouting(t *testing.T) {
	dir := t.TempDir()
	local := writeLocalPNG(t, dir)
	const remote = "https://example.com/a.png"

	providers := []struct {
		name         string
		p            *provider.EffectiveProvider
		wantURL      string
		wantUploads  int
		wantBodyFrag []string
		wantAbsent   []string
	}{
		{
			name:         "apimart local file uploads",
			p:            &provider.EffectiveProvider{BaseURL: "https://api.apimart.ai", APIKey: "k", ProviderType: provider.APIMart},
			wantURL:      "https://api.apimart.ai/v1/images/generations",
			wantUploads:  1,
			wantBodyFrag: []string{"UPLOAD_URL_0"},
			wantAbsent:   []string{local},
		},
		{
			name:         "openrouter embeds data uri",
			p:            &provider.EffectiveProvider{BaseURL: "https://openrouter.ai/api/v1", APIKey: "k", ProviderType: provider.OpenRouter},
			wantURL:      "https://openrouter.ai/api/v1/images",
			wantUploads:  0,
			wantBodyFrag: []string{"input_references", "data:image/png;base64,"},
		},
		{
			name:         "gemini emulates image input",
			p:            &provider.EffectiveProvider{BaseURL: "https://generativelanguage.googleapis.com", APIKey: "k", ProviderType: provider.Gemini},
			wantURL:      "https://generativelanguage.googleapis.com/v1/interactions",
			wantUploads:  0,
			wantBodyFrag: []string{`"type":"image"`, `"data":`, `"mime_type":"image/png"`},
		},
		{
			name:         "modelscope embeds data uri",
			p:            &provider.EffectiveProvider{BaseURL: "https://api-inference.modelscope.cn", APIKey: "k", ProviderType: provider.ModelScope},
			wantURL:      "https://api-inference.modelscope.cn/v1/images/generations",
			wantUploads:  0,
			wantBodyFrag: []string{`"image_url":"data:image/png;base64,`},
		},
		{
			name:         "zeekai edits embeds data uri",
			p:            &provider.EffectiveProvider{BaseURL: "https://api.zeekai.cc", APIKey: "k", ProviderType: provider.Zeekai},
			wantURL:      "https://api.zeekai.cc/v1/images/edits",
			wantUploads:  0,
			wantBodyFrag: []string{`"images":[{"image_url":"data:image/png;base64,`},
		},
		{
			name:         "agnes nests extra_body image",
			p:            &provider.EffectiveProvider{BaseURL: "https://api.agnes-ai.com", APIKey: "k", ProviderType: provider.Agnes},
			wantURL:      "https://api.agnes-ai.com/v1/images/generations",
			wantUploads:  0,
			wantBodyFrag: []string{`"extra_body":{"image":["data:image/png;base64,`},
		},
		{
			name:         "default sync keeps image_urls with data uri",
			p:            &provider.EffectiveProvider{BaseURL: "https://api.openai.com", APIKey: "k", ProviderType: provider.OpenAI},
			wantURL:      "https://api.openai.com/v1/images/generations",
			wantUploads:  0,
			wantBodyFrag: []string{`"image_urls":["data:image/png;base64,`},
		},
	}

	for _, tc := range providers {
		t.Run(tc.name, func(t *testing.T) {
			req := &types.GenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{local}}
			pl := mustBuildPlan(t, req, tc.p)
			if pl.URL != tc.wantURL {
				t.Errorf("URL = %q, want %q", pl.URL, tc.wantURL)
			}
			if len(pl.Uploads) != tc.wantUploads {
				t.Errorf("len(Uploads) = %d, want %d", len(pl.Uploads), tc.wantUploads)
			}
			body := planBodyJSON(t, pl.Body)
			for _, frag := range tc.wantBodyFrag {
				if !strings.Contains(body, frag) {
					t.Errorf("body missing %q, got:\n%s", frag, body)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(body, absent) {
					t.Errorf("body should not contain local path %q, got:\n%s", absent, body)
				}
			}
		})
	}

	t.Run("apimart preview placeholder", func(t *testing.T) {
		req := &types.GenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{local}}
		pl := mustBuildPlan(t, req, providers[0].p)
		if pl.Uploads[0].Path != local {
			t.Errorf("upload path = %q, want %q", pl.Uploads[0].Path, local)
		}
		if pl.UploadURL != "https://api.apimart.ai/v1/uploads/images" {
			t.Errorf("UploadURL = %q", pl.UploadURL)
		}
		if !strings.Contains(planBodyJSON(t, pl.Body), "<UPLOAD_URL_0>") {
			t.Errorf("preview body should embed the placeholder token")
		}
	})

	t.Run("apimart remote url not uploaded", func(t *testing.T) {
		req := &types.GenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{remote}}
		pl := mustBuildPlan(t, req, providers[0].p)
		if len(pl.Uploads) != 0 {
			t.Fatalf("len(Uploads) = %d, want 0", len(pl.Uploads))
		}
		if !strings.Contains(planBodyJSON(t, pl.Body), remote) {
			t.Errorf("remote URL should be preserved in the body")
		}
	})

	t.Run("apimart no image no upload", func(t *testing.T) {
		req := &types.GenerateRequest{Model: "m", Prompt: "p"}
		pl := mustBuildPlan(t, req, providers[0].p)
		if len(pl.Uploads) != 0 {
			t.Errorf("len(Uploads) = %d, want 0", len(pl.Uploads))
		}
	})

	t.Run("zeekai text-only falls back to sync", func(t *testing.T) {
		req := &types.GenerateRequest{Model: "m", Prompt: "p"}
		pl := mustBuildPlan(t, req, providers[4].p)
		if pl.URL != "https://api.zeekai.cc/v1/images/generations" {
			t.Errorf("URL = %q, want sync generations path", pl.URL)
		}
	})

	t.Run("zeekai edits renders plain note once", func(t *testing.T) {
		req := &types.GenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{local}}
		pl := mustBuildPlan(t, req, providers[4].p)
		curl := pl.RenderCurls("k")
		if !strings.Contains(curl, "# note: local image files are embedded as data: URIs") {
			t.Errorf("curl should contain the rendered note, got:\n%s", curl)
		}
		if strings.Contains(curl, "# note: # note:") {
			t.Errorf("note prefix should not be doubled, got:\n%s", curl)
		}
	})

	t.Run("ollama uses native endpoint", func(t *testing.T) {
		req := &types.GenerateRequest{Model: "m", Prompt: "p"}
		pl := mustBuildPlan(t, req, &provider.EffectiveProvider{BaseURL: "http://localhost:11434/v1", Type: types.ProviderOllama})
		if pl.URL != "http://localhost:11434/api/generate" {
			t.Errorf("URL = %q, want native /api/generate", pl.URL)
		}
	})
}

func TestBuildImagePlanDryRunNoNetwork(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	local := writeLocalPNG(t, dir)
	p := &provider.EffectiveProvider{BaseURL: srv.URL, APIKey: "k", ProviderType: provider.APIMart}
	req := &types.GenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{local}}

	pl := mustBuildPlan(t, req, p)
	curl := pl.RenderCurls(p.APIKey)
	if !strings.Contains(curl, "/uploads/images") {
		t.Errorf("preview should render the upload curl, got:\n%s", curl)
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("dry-run preview made %d network request(s), want 0", got)
	}
	if req.ImageURLs[0] != local {
		t.Fatalf("preview must not mutate the request, got %q", req.ImageURLs[0])
	}
}

func TestApplyUploadsAPIMartTyped(t *testing.T) {
	srv := uploadServer(t)
	defer srv.Close()

	c := client.NewWithProvider("k", srv.URL, "", types.ProviderOpenAI)
	dir := t.TempDir()
	local := writeLocalPNG(t, dir)
	p := &provider.EffectiveProvider{BaseURL: srv.URL, APIKey: "k", ProviderType: provider.APIMart}
	req := &types.GenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{local}}

	pl := mustBuildPlan(t, req, p)
	if err := pl.applyUploads(c, req); err != nil {
		t.Fatalf("applyUploads() error = %v", err)
	}
	if req.ImageURLs[0] != "https://cdn.example.com/up0.png" {
		t.Errorf("ImageURLs[0] = %q, want resolved URL", req.ImageURLs[0])
	}
	if !strings.Contains(planBodyJSON(t, pl.Body), "https://cdn.example.com/up0.png") {
		t.Errorf("plan body should reflect resolved URL after applyUploads")
	}
}

func TestApplyUploadsAPIMartRawJSON(t *testing.T) {
	srv := uploadServer(t)
	defer srv.Close()

	c := client.NewWithProvider("k", srv.URL, "", types.ProviderOpenAI)
	dir := t.TempDir()
	local := writeLocalPNG(t, dir)
	p := &provider.EffectiveProvider{BaseURL: srv.URL, APIKey: "k", ProviderType: provider.APIMart}
	raw, _ := json.Marshal(map[string]any{"model": "m", "prompt": "p", "image_urls": []string{local}})
	req := &types.GenerateRequest{RawJSON: raw}

	pl := mustBuildPlan(t, req, p)
	if len(pl.Uploads) != 1 {
		t.Fatalf("len(Uploads) = %d, want 1", len(pl.Uploads))
	}
	if err := pl.applyUploads(c, req); err != nil {
		t.Fatalf("applyUploads() error = %v", err)
	}
	var body struct {
		ImageURLs []string `json:"image_urls"`
	}
	if err := json.Unmarshal(req.RawJSON, &body); err != nil {
		t.Fatalf("unmarshal patched raw json: %v", err)
	}
	if len(body.ImageURLs) != 1 || body.ImageURLs[0] != "https://cdn.example.com/up0.png" {
		t.Errorf("patched raw image_urls = %v, want resolved URL", body.ImageURLs)
	}
}

func TestApplyUploadsNoopForEncodingProviders(t *testing.T) {
	dir := t.TempDir()
	local := writeLocalPNG(t, dir)
	p := &provider.EffectiveProvider{BaseURL: "https://openrouter.ai/api/v1", APIKey: "k", ProviderType: provider.OpenRouter}
	req := &types.GenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{local}}

	pl := mustBuildPlan(t, req, p)
	// nil client proves applyUploads touches the network only when uploads exist.
	if err := pl.applyUploads(nil, req); err != nil {
		t.Fatalf("applyUploads() error = %v", err)
	}
	if !strings.HasPrefix(req.ImageURLs[0], "data:image/png;base64,") {
		t.Errorf("request should already hold the encoded data URI, got %q", req.ImageURLs[0])
	}
}
