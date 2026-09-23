package video

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// This file proves the conformance contract of buildVideoPlan: for every
// provider the body the command previews (--dry-run/--verbose) equals the body
// the real client puts on the wire. Each test follows the production path —
// buildVideoPlan → applyUploads → the runner's client call — against a real
// httptest server, then compares the plan body with the captured bytes.
// The harness lives in video_conformance_helpers_test.go, the body comparison
// in video_conformance_compare_test.go.

// TestVideoConformanceAPIMartLocalUpload proves an APIMart local image previews
// as an upload token and is sent as the uploaded URL.
func TestVideoConformanceAPIMartLocalUpload(t *testing.T) {
	local := testLocalImage(t)
	srv := newWireServer(t)
	p := conformanceProvider(srv.URL, provider.APIMart)
	req := &types.VideoGenerateRequest{Model: "veo3", Prompt: "a cat", ImageURLs: []string{local}}

	plan, rec := runConformance(t, srv, p, req, submitVideo)
	if len(plan.Uploads) != 1 {
		t.Fatalf("plan uploads = %d, want 1", len(plan.Uploads))
	}
	if got := srv.uploadCount(); got != 1 {
		t.Errorf("upload requests = %d, want 1", got)
	}
	if rec.Method != http.MethodPost || rec.Path != "/v1"+client.VideoSubmitPath {
		t.Errorf("sent %s %s, want POST /v1%s", rec.Method, rec.Path, client.VideoSubmitPath)
	}
	assertSentBody(t, rec.Body, local)
	requirePreviewMatchesSent(t, plan.Body, []string{conformanceUploadedURL}, rec.Body)
}

// TestVideoConformanceAPIMartFirstLastFrame proves both framed images upload and
// land back in image_with_roles with their roles intact.
func TestVideoConformanceAPIMartFirstLastFrame(t *testing.T) {
	first := testLocalImage(t)
	last := testLocalImage(t)
	srv := newWireServer(t)
	p := conformanceProvider(srv.URL, provider.APIMart)
	req := &types.VideoGenerateRequest{
		Model:  "veo3",
		Prompt: "a cat walking",
		ImageWithRoles: []types.ImageWithRole{
			{URL: first, Role: "first_frame"},
			{URL: last, Role: "last_frame"},
		},
	}

	plan, rec := runConformance(t, srv, p, req, submitVideo)
	if len(plan.Uploads) != 2 {
		t.Fatalf("plan uploads = %d, want 2", len(plan.Uploads))
	}
	if got := srv.uploadCount(); got != 2 {
		t.Errorf("upload requests = %d, want 2", got)
	}

	var sent struct {
		ImageWithRoles []types.ImageWithRole `json:"image_with_roles"`
	}
	if err := json.Unmarshal([]byte(rec.Body), &sent); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	if len(sent.ImageWithRoles) != 2 {
		t.Fatalf("sent image_with_roles = %d entries, want 2: %s", len(sent.ImageWithRoles), rec.Body)
	}
	wantRoles := []string{"first_frame", "last_frame"}
	for i, role := range sent.ImageWithRoles {
		if role.Role != wantRoles[i] || role.URL != conformanceUploadedURL {
			t.Errorf("sent image_with_roles[%d] = %+v, want role %s with %s", i, role, wantRoles[i], conformanceUploadedURL)
		}
	}
	assertSentBody(t, rec.Body, first, last)
	requirePreviewMatchesSent(t, plan.Body, []string{conformanceUploadedURL, conformanceUploadedURL}, rec.Body)
}

// TestVideoConformanceAPIMartRemoteURL proves a public URL is never uploaded
// and reaches the wire unchanged.
func TestVideoConformanceAPIMartRemoteURL(t *testing.T) {
	srv := newWireServer(t)
	p := conformanceProvider(srv.URL, provider.APIMart)
	req := &types.VideoGenerateRequest{Model: "veo3", Prompt: "a cat", ImageURLs: []string{conformanceRemoteURL}}

	plan, rec := runConformance(t, srv, p, req, submitVideo)
	if len(plan.Uploads) != 0 {
		t.Fatalf("plan uploads = %d, want 0 for a remote URL", len(plan.Uploads))
	}
	if got := srv.uploadCount(); got != 0 {
		t.Errorf("upload requests = %d, want 0", got)
	}
	if !strings.Contains(rec.Body, conformanceRemoteURL) {
		t.Errorf("sent body should pass the remote URL through unchanged: %s", rec.Body)
	}
	requirePreviewMatchesSent(t, plan.Body, nil, rec.Body)
}

// TestVideoConformanceOpenLuxLocalUpload proves an OpenLux local image uploads and
// the sent images list carries the uploaded URL.
func TestVideoConformanceOpenLuxLocalUpload(t *testing.T) {
	local := testLocalImage(t)
	srv := newWireServer(t)
	p := conformanceProvider(srv.URL, provider.OpenLux)
	req := &types.VideoGenerateRequest{Model: "veo3", Prompt: "a cat", ImageURLs: []string{local}}

	plan, rec := runConformance(t, srv, p, req, submitOpenLuxVideo)
	if len(plan.Uploads) != 1 {
		t.Fatalf("plan uploads = %d, want 1", len(plan.Uploads))
	}
	if got := srv.uploadCount(); got != 1 {
		t.Errorf("upload requests = %d, want 1", got)
	}
	if rec.Method != http.MethodPost || rec.Path != "/v1"+client.OpenLuxVideoSubPath {
		t.Errorf("sent %s %s, want POST /v1%s", rec.Method, rec.Path, client.OpenLuxVideoSubPath)
	}

	var sent struct {
		Images []string `json:"images"`
	}
	if err := json.Unmarshal([]byte(rec.Body), &sent); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	if len(sent.Images) != 1 || sent.Images[0] != conformanceUploadedURL {
		t.Errorf("sent images = %v, want [%s]", sent.Images, conformanceUploadedURL)
	}
	assertSentBody(t, rec.Body, local)
	requirePreviewMatchesSent(t, plan.Body, []string{conformanceUploadedURL}, rec.Body)
}

// TestVideoConformanceOpenRouterLocalEncode proves OpenRouter's in-place data
// URI encoding is what both the preview and the send carry.
func TestVideoConformanceOpenRouterLocalEncode(t *testing.T) {
	local := testLocalImage(t)
	srv := newWireServer(t)
	p := conformanceProvider(srv.URL, provider.OpenRouter)
	req := &types.VideoGenerateRequest{Model: "google/veo-3.1", Prompt: "a cat", ImageURLs: []string{local}}

	plan, rec := runConformance(t, srv, p, req, submitOpenRouterVideo)
	if len(plan.Uploads) != 0 {
		t.Fatalf("plan uploads = %d, want 0 (OpenRouter encodes in place)", len(plan.Uploads))
	}
	if want := "/v1" + client.OpenRouterVideosPath; rec.Method != http.MethodPost || rec.Path != want {
		t.Errorf("sent %s %s, want POST %s", rec.Method, rec.Path, want)
	}
	if !strings.Contains(rec.Body, "data:image/png;base64,") {
		t.Errorf("sent body should embed the local file as a data URI: %s", rec.Body)
	}
	if strings.Contains(rec.Body, local) {
		t.Errorf("sent body leaked local path %q: %s", local, rec.Body)
	}
	requirePreviewMatchesSent(t, plan.Body, nil, rec.Body)
}

// TestVideoConformanceAgnesLocalEncode proves Agnes's reference-mode data URI
// body is what both the preview and the send carry.
func TestVideoConformanceAgnesLocalEncode(t *testing.T) {
	local := testLocalImage(t)
	srv := newWireServer(t)
	p := conformanceProvider(srv.URL, provider.Agnes)
	req := &types.VideoGenerateRequest{Model: "agnes-video-2.5-flash", Prompt: "a cat", ImageURLs: []string{local}}

	plan, rec := runConformance(t, srv, p, req, submitAgnesVideo)
	if len(plan.Uploads) != 0 {
		t.Fatalf("plan uploads = %d, want 0 (Agnes has no upload endpoint)", len(plan.Uploads))
	}
	if want := "/v1" + client.AgnesVideoSubmitPath; rec.Method != http.MethodPost || rec.Path != want {
		t.Errorf("sent %s %s, want POST %s", rec.Method, rec.Path, want)
	}

	var sent struct {
		Mode   string   `json:"mode"`
		Images []string `json:"images"`
	}
	if err := json.Unmarshal([]byte(rec.Body), &sent); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	if sent.Mode != "reference" || len(sent.Images) != 1 || !strings.HasPrefix(sent.Images[0], "data:image/png;base64,") {
		t.Errorf("sent body should carry a reference-mode data URI, got mode=%q images=%v", sent.Mode, sent.Images)
	}
	if strings.Contains(rec.Body, local) {
		t.Errorf("sent body leaked local path %q: %s", local, rec.Body)
	}
	requirePreviewMatchesSent(t, plan.Body, nil, rec.Body)
}

// TestVideoConformancePollinationsGET proves the GET plan's URL is the URL the
// real client requests, with no body and no uploads.
func TestVideoConformancePollinationsGET(t *testing.T) {
	duration := 4
	srv := newWireServer(t)
	p := conformanceProvider(srv.URL, provider.Pollinations)
	req := &types.VideoGenerateRequest{Model: "community/NamanSoni78/Seedance-2.5", Prompt: "a cat walking", Duration: &duration}

	plan, rec := runConformance(t, srv, p, req, generatePollinationsVideo)
	if plan.Method != http.MethodGet || plan.Body != nil || len(plan.Uploads) != 0 {
		t.Fatalf("plan = %s body=%v uploads=%d, want GET with no body and no uploads", plan.Method, plan.Body, len(plan.Uploads))
	}
	if rec.Method != http.MethodGet {
		t.Errorf("sent method = %s, want GET", rec.Method)
	}
	if rec.Body != "" {
		t.Errorf("GET sent a body: %q", rec.Body)
	}
	if got := srv.uploadCount(); got != 0 {
		t.Errorf("upload requests = %d, want 0", got)
	}
	if want := "http://" + rec.Host + rec.URI; plan.URL != want {
		t.Errorf("plan URL = %q, sent URL = %q", plan.URL, want)
	}
}
