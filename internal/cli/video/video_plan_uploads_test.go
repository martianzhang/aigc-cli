package video

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// TestBuildVideoPlanRoleUploads covers image_with_roles for the upload providers.
func TestBuildVideoPlanRoleUploads(t *testing.T) {
	local := testLocalImage(t)

	tests := []struct {
		name    string
		p       *provider.EffectiveProvider
		bodyKey string
	}{
		{"apimart", &provider.EffectiveProvider{APIKey: "k", BaseURL: "https://api.apimart.ai", ProviderType: provider.APIMart}, "image_with_roles"},
		{"openlux", &provider.EffectiveProvider{APIKey: "k", BaseURL: "https://api.openlux.ai", ProviderType: provider.OpenLux}, "images"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &types.VideoGenerateRequest{
				Model:          "m",
				ImageWithRoles: []types.ImageWithRole{{URL: local, Role: "first_frame"}},
			}
			plan, err := buildVideoPlan(req, tc.p)
			if err != nil {
				t.Fatalf("buildVideoPlan() error = %v", err)
			}
			if len(plan.Uploads) != 1 {
				t.Fatalf("uploads = %d, want 1", len(plan.Uploads))
			}
			if plan.Uploads[0].Label == "" {
				t.Error("upload Label should identify the source field")
			}
			if req.ImageWithRoles[0].URL != local {
				t.Errorf("buildVideoPlan must not mutate role paths, got %q", req.ImageWithRoles[0].URL)
			}

			m := marshalToMap(t, plan.Body)
			if tc.bodyKey == "images" {
				if got := firstString(m["images"]); got != "<UPLOAD_URL_0>" {
					t.Errorf("images[0] = %q, want placeholder", got)
				}
				return
			}
			roles, _ := m["image_with_roles"].([]any)
			if len(roles) != 1 {
				t.Fatalf("image_with_roles len = %d, want 1", len(roles))
			}
			first, _ := roles[0].(map[string]any)
			if got, _ := first["url"].(string); got != "<UPLOAD_URL_0>" {
				t.Errorf("image_with_roles[0].url = %q, want placeholder", got)
			}
			if got, _ := first["role"].(string); got != "first_frame" {
				t.Errorf("image_with_roles[0].role = %q, want first_frame", got)
			}
		})
	}
}

// TestBuildVideoPlanPollinations asserts the GET plan carries no body/uploads.
func TestBuildVideoPlanPollinations(t *testing.T) {
	p := &provider.EffectiveProvider{APIKey: "k", BaseURL: "https://gen.pollinations.ai", ProviderType: provider.Pollinations}
	req := &types.VideoGenerateRequest{Model: "community/x", Prompt: "a cat walking"}

	plan, err := buildVideoPlan(req, p)
	if err != nil {
		t.Fatalf("buildVideoPlan() error = %v", err)
	}
	if plan.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", plan.Method)
	}
	if !strings.Contains(plan.URL, "gen.pollinations.ai/video/a%20cat%20walking") {
		t.Errorf("url = %q, want root /video/{prompt}", plan.URL)
	}
	if plan.Body != nil {
		t.Errorf("body = %v, want nil for a GET plan", plan.Body)
	}
	if len(plan.Uploads) != 0 {
		t.Errorf("uploads = %d, want 0", len(plan.Uploads))
	}
}

// TestBuildVideoPlanPreviewIsNetworkFree guards that preview never hits the wire.
func TestBuildVideoPlanPreviewIsNetworkFree(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	local := testLocalImage(t)
	p := &provider.EffectiveProvider{APIKey: "sk-test", BaseURL: srv.URL, ProviderType: provider.APIMart}
	req := &types.VideoGenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{local}}

	plan, err := buildVideoPlan(req, p)
	if err != nil {
		t.Fatalf("buildVideoPlan() error = %v", err)
	}
	curl := plan.RenderCurls(p.APIKey)
	if curl == "" {
		t.Fatal("RenderCurls() returned empty")
	}
	if !strings.Contains(curl, "<UPLOAD_URL_0>") {
		t.Errorf("preview should carry the literal upload placeholder:\n%s", curl)
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("preview hit the network %d times, want 0", got)
	}
}

// TestVideoPlanApplyUploads writes resolved URLs back into both image fields.
func TestVideoPlanApplyUploads(t *testing.T) {
	const uploaded = "https://cdn.example.com/uploaded.png"
	var uploadHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploadHits.Add(1)
		if r.URL.Path != "/v1"+client.UploadPath {
			t.Errorf("upload path = %q, want %q", r.URL.Path, "/v1"+client.UploadPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"url":"`+uploaded+`"}`)
	}))
	defer srv.Close()

	local := testLocalImage(t)
	p := &provider.EffectiveProvider{APIKey: "sk-test", BaseURL: srv.URL, ProviderType: provider.APIMart}
	req := &types.VideoGenerateRequest{
		Model:          "m",
		Prompt:         "p",
		ImageURLs:      []string{local},
		ImageWithRoles: []types.ImageWithRole{{URL: local, Role: "first_frame"}},
	}

	plan, err := buildVideoPlan(req, p)
	if err != nil {
		t.Fatalf("buildVideoPlan() error = %v", err)
	}
	if len(plan.Uploads) != 2 {
		t.Fatalf("uploads = %d, want 2", len(plan.Uploads))
	}

	c := client.New(p.APIKey, p.BaseURL, "")
	if err := plan.applyUploads(c, req); err != nil {
		t.Fatalf("applyUploads() error = %v", err)
	}

	if req.ImageURLs[0] != uploaded {
		t.Errorf("ImageURLs[0] = %q, want %q", req.ImageURLs[0], uploaded)
	}
	if req.ImageWithRoles[0].URL != uploaded {
		t.Errorf("ImageWithRoles[0].URL = %q, want %q", req.ImageWithRoles[0].URL, uploaded)
	}
	if got := uploadHits.Load(); got != 2 {
		t.Errorf("upload hits = %d, want 2", got)
	}
}

// TestVideoPlanApplyUploadsNoUploadsSkipsClient guards the no-op fast path.
func TestVideoPlanApplyUploadsNoUploadsSkipsClient(t *testing.T) {
	req := &types.VideoGenerateRequest{Model: "m", Prompt: "p"}
	plan := &videoPlan{}

	if err := plan.applyUploads(nil, req); err != nil {
		t.Fatalf("applyUploads() error = %v", err)
	}
}
