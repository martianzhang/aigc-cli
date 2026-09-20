package video

import (
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// testLocalImage writes a tiny decodable PNG and returns its path.
func testLocalImage(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "seed.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create temp image: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode temp image: %v", err)
	}
	return path
}

// marshalToMap renders a preview body to a generic map regardless of its type.
func marshalToMap(t *testing.T, body any) map[string]any {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	return m
}

// firstBodyImage extracts the first image value a provider body carries.
func firstBodyImage(t *testing.T, ptype provider.Type, body any) string {
	t.Helper()
	if body == nil {
		return ""
	}
	m := marshalToMap(t, body)
	switch ptype {
	case provider.OpenRouter:
		frames, _ := m["frame_images"].([]any)
		if len(frames) == 0 {
			return ""
		}
		frame, _ := frames[0].(map[string]any)
		img, _ := frame["image_url"].(map[string]any)
		s, _ := img["url"].(string)
		return s
	case provider.Yunwu, provider.Agnes:
		return firstString(m["images"])
	default:
		return firstString(m["image_urls"])
	}
}

func firstString(v any) string {
	list, _ := v.([]any)
	if len(list) == 0 {
		return ""
	}
	s, _ := list[0].(string)
	return s
}

// TestBuildVideoPlanImageRouting asserts one plan per provider across input kinds.
func TestBuildVideoPlanImageRouting(t *testing.T) {
	local := testLocalImage(t)
	const remote = "https://cdn.example.com/cat.png"

	tests := []struct {
		name        string
		p           *provider.EffectiveProvider
		wantMethod  string
		wantURL     string
		wantUploads int    // expected uploads for the local-file case
		wantLocal   string // exact body value for local case; empty => data: URI
		wantDataURI bool
	}{
		{
			name:        "apimart uploads and inlines placeholders",
			p:           &provider.EffectiveProvider{APIKey: "k", BaseURL: "https://api.apimart.ai", ProviderType: provider.APIMart},
			wantMethod:  http.MethodPost,
			wantURL:     "https://api.apimart.ai/v1/videos/generations",
			wantUploads: 1,
			wantLocal:   "<UPLOAD_URL_0>",
		},
		{
			name:        "yunwu uploads and inlines placeholders",
			p:           &provider.EffectiveProvider{APIKey: "k", BaseURL: "https://yunwu.ai", ProviderType: provider.Yunwu},
			wantMethod:  http.MethodPost,
			wantURL:     "https://yunwu.ai/v1/video/create",
			wantUploads: 1,
			wantLocal:   "<UPLOAD_URL_0>",
		},
		{
			name:        "openrouter encodes data URIs",
			p:           &provider.EffectiveProvider{APIKey: "k", BaseURL: "https://openrouter.ai/api/v1", ProviderType: provider.OpenRouter},
			wantMethod:  http.MethodPost,
			wantURL:     "https://openrouter.ai/api/v1/videos",
			wantUploads: 0,
			wantDataURI: true,
		},
		{
			name:        "agnes encodes data URIs",
			p:           &provider.EffectiveProvider{APIKey: "k", BaseURL: "https://apihub.agnes-ai.com/v1", ProviderType: provider.Agnes},
			wantMethod:  http.MethodPost,
			wantURL:     "https://apihub.agnes-ai.com/v1/videos",
			wantUploads: 0,
			wantDataURI: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("no image", func(t *testing.T) {
				req := &types.VideoGenerateRequest{Model: "m", Prompt: "p"}
				plan, err := buildVideoPlan(req, tc.p)
				if err != nil {
					t.Fatalf("buildVideoPlan() error = %v", err)
				}
				if plan.Method != tc.wantMethod || plan.URL != tc.wantURL {
					t.Errorf("method/url = %s %s, want %s %s", plan.Method, plan.URL, tc.wantMethod, tc.wantURL)
				}
				if len(plan.Uploads) != 0 {
					t.Errorf("uploads = %d, want 0", len(plan.Uploads))
				}
				if got := firstBodyImage(t, tc.p.ProviderType, plan.Body); got != "" {
					t.Errorf("body image = %q, want empty", got)
				}
			})

			t.Run("remote url", func(t *testing.T) {
				req := &types.VideoGenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{remote}}
				plan, err := buildVideoPlan(req, tc.p)
				if err != nil {
					t.Fatalf("buildVideoPlan() error = %v", err)
				}
				if len(plan.Uploads) != 0 {
					t.Errorf("uploads = %d, want 0 for a remote URL", len(plan.Uploads))
				}
				if got := firstBodyImage(t, tc.p.ProviderType, plan.Body); got != remote {
					t.Errorf("body image = %q, want %q", got, remote)
				}
			})

			t.Run("local file", func(t *testing.T) {
				req := &types.VideoGenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{local}}
				plan, err := buildVideoPlan(req, tc.p)
				if err != nil {
					t.Fatalf("buildVideoPlan() error = %v", err)
				}
				if len(plan.Uploads) != tc.wantUploads {
					t.Fatalf("uploads = %d, want %d", len(plan.Uploads), tc.wantUploads)
				}
				if tc.wantUploads > 0 {
					if plan.Uploads[0].Path != local {
						t.Errorf("upload path = %q, want %q", plan.Uploads[0].Path, local)
					}
					wantUploadURL := "https://" + strings.TrimPrefix(strings.TrimPrefix(tc.p.BaseURL, "https://"), "http://") + "/v1" + client.UploadPath
					if plan.UploadURL != wantUploadURL {
						t.Errorf("upload URL = %q, want %q", plan.UploadURL, wantUploadURL)
					}
				}
				got := firstBodyImage(t, tc.p.ProviderType, plan.Body)
				if tc.wantDataURI {
					if !strings.HasPrefix(got, "data:image/") {
						t.Errorf("body image = %q, want a data: URI", got)
					}
					return
				}
				if got != tc.wantLocal {
					t.Errorf("body image = %q, want %q", got, tc.wantLocal)
				}
				if req.ImageURLs[0] != local {
					t.Errorf("buildVideoPlan must not mutate local paths, got %q", req.ImageURLs[0])
				}
			})
		})
	}
}

// TestBuildVideoPlanAPIMartPreviewTokensLiteral guards that the APIMart preview
// renders copy-pasteable <UPLOAD_URL_i> tokens, not HTML-escaped ones.
func TestBuildVideoPlanAPIMartPreviewTokensLiteral(t *testing.T) {
	local := testLocalImage(t)
	p := &provider.EffectiveProvider{APIKey: "k", BaseURL: "https://api.apimart.ai", ProviderType: provider.APIMart}
	req := &types.VideoGenerateRequest{
		Model:          "veo3",
		Prompt:         "p",
		ImageURLs:      []string{local},
		ImageWithRoles: []types.ImageWithRole{{URL: local, Role: "first_frame"}},
	}

	plan, err := buildVideoPlan(req, p)
	if err != nil {
		t.Fatalf("buildVideoPlan() error = %v", err)
	}
	curl := plan.RenderCurls(p.APIKey)

	for _, token := range []string{"<UPLOAD_URL_0>", "<UPLOAD_URL_1>"} {
		if !strings.Contains(curl, token) {
			t.Errorf("preview must render literal token %s:\n%s", token, curl)
		}
	}
	if strings.Contains(curl, `\u003c`) || strings.Contains(curl, `\u003e`) {
		t.Errorf("preview must not HTML-escape placeholder tokens:\n%s", curl)
	}
}
