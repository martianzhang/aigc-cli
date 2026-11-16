package image

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// tinyPNG returns a valid 1x1 PNG so save/download paths exercise real bytes.
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// isolateImageOutput points generated files at a temp dir for one test.
func isolateImageOutput(t *testing.T) {
	t.Helper()
	oldDir, oldVerbose := options.Shared.OutputDir, options.Shared.Verbose
	options.Shared.OutputDir = t.TempDir()
	t.Cleanup(func() {
		options.Shared.OutputDir = oldDir
		options.Shared.Verbose = oldVerbose
	})
}

func TestRunOpenRouterDedicatedImageFiltersUnsupportedFields(t *testing.T) {
	isolateImageOutput(t)
	options.Shared.Verbose = true // exercise the dropped-field log
	pngB64 := base64.StdEncoding.EncodeToString(tinyPNG(t))

	var gotBody map[string]any
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[{"id":"tencent/hy-world-2.1-panorama",`+
				`"supported_parameters":{"n":{"type":"range","min":1,"max":1}}}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/images":
			gotPath = r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"created":1,"data":[{"b64_json":"`+pngB64+`"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	req := &types.GenerateRequest{
		Model:        "tencent/hy-world-2.1-panorama",
		Prompt:       "equirectangular 360 panorama",
		Size:         "auto",
		Quality:      "auto",
		OutputFormat: "png",
	}
	ctx := &imageDispatchCtx{p: &provider.EffectiveProvider{
		BaseURL:      srv.URL,
		ProviderType: provider.OpenRouter,
	}}

	saved, err := runOpenRouterDedicatedImage(client.New("k", srv.URL, ""), req, ctx)
	if err != nil {
		t.Fatalf("runOpenRouterDedicatedImage() error = %v", err)
	}
	if gotPath != "/v1/images" {
		t.Errorf("POST path = %q, want /v1/images", gotPath)
	}
	for _, field := range []string{"size", "quality", "output_format"} {
		if _, ok := gotBody[field]; ok {
			t.Errorf("request body kept unsupported %q: %v", field, gotBody)
		}
	}
	if gotBody["model"] != req.Model || gotBody["prompt"] != req.Prompt {
		t.Errorf("body = %v, want model+prompt", gotBody)
	}
	if len(saved) != 1 {
		t.Fatalf("saved = %v, want one file", saved)
	}
	if _, err := os.Stat(saved[0]); err != nil {
		t.Errorf("saved file missing: %v", err)
	}
}

func TestRunTencentPanoramaImageDownloadsSuperResolution(t *testing.T) {
	isolateImageOutput(t)
	pngBytes := tinyPNG(t)

	var gotBody map[string]any
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/wand/hunyuan-image/world2-panorama":
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"i","created":1,"status":"succeeded","data":[{`+
				`"super_resolution_image_url":"`+srv.URL+`/pano.png"}],"tokenhub_usage":{"total_tokens":500000}}`)
		case "/pano.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	req := &types.GenerateRequest{Model: "hy-world2-panorama", Prompt: "equirectangular 360 panorama"}
	ctx := &imageDispatchCtx{p: &provider.EffectiveProvider{
		BaseURL:      srv.URL,
		ProviderType: provider.Tencent,
	}}

	saved, err := runTencentPanoramaImage(client.New("k", srv.URL, ""), req, ctx)
	if err != nil {
		t.Fatalf("runTencentPanoramaImage() error = %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("saved = %v, want one file", saved)
	}
	got, err := os.ReadFile(saved[0])
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if !bytes.Equal(got, pngBytes) {
		t.Error("saved file does not match the downloaded panorama")
	}
	if _, err := os.Stat(filepath.Dir(saved[0])); err != nil {
		t.Errorf("output dir missing: %v", err)
	}
	for _, field := range []string{"size", "quality", "output_format"} {
		if _, ok := gotBody[field]; ok {
			t.Errorf("native body must not carry %q: %v", field, gotBody)
		}
	}
	if !strings.Contains(saved[0], "image_tencent_") {
		t.Errorf("filename %q should use the tencent task prefix", saved[0])
	}
}
