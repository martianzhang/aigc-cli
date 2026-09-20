package image

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func firstMatchRunName(req *types.GenerateRequest, ctx *imageDispatchCtx) string {
	for _, s := range imageStrategies {
		if s.match(req, ctx) {
			return runtime.FuncForPC(reflect.ValueOf(s.run).Pointer()).Name()
		}
	}
	return ""
}

func TestZeekaiStrategyRouting(t *testing.T) {
	withImage := &types.GenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{"a.png"}}
	noImage := &types.GenerateRequest{Model: "m", Prompt: "p"}

	tests := []struct {
		name      string
		req       *types.GenerateRequest
		ctx       *imageDispatchCtx
		wantEdits bool
	}{
		{"zeekai with image routes to edits", withImage, &imageDispatchCtx{isZeekai: true}, true},
		{"zeekai text-only falls through to sync", noImage, &imageDispatchCtx{isZeekai: true}, false},
		{"non-zeekai with image stays sync", withImage, &imageDispatchCtx{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.HasSuffix(firstMatchRunName(tc.req, tc.ctx), "runImageEditsJSON")
			if got != tc.wantEdits {
				t.Errorf("matched runImageEditsJSON = %v, want %v", got, tc.wantEdits)
			}
		})
	}
}

func TestUsesImageEditsJSON(t *testing.T) {
	withImage := &types.GenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{"a.png"}}
	noImage := &types.GenerateRequest{Model: "m", Prompt: "p"}

	tests := []struct {
		name string
		p    *provider.EffectiveProvider
		req  *types.GenerateRequest
		want bool
	}{
		{"zeekai with image", &provider.EffectiveProvider{ProviderType: provider.Zeekai}, withImage, true},
		{"zeekai without image", &provider.EffectiveProvider{ProviderType: provider.Zeekai}, noImage, false},
		{"openai with image", &provider.EffectiveProvider{ProviderType: provider.OpenAI}, withImage, false},
		{"nil provider", nil, withImage, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := usesImageEditsJSON(tc.p, tc.req); got != tc.want {
				t.Errorf("usesImageEditsJSON() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRunImageEditsJSON_localFileBecomesDataURI(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(path, png, 0o600); err != nil {
		t.Fatal(err)
	}

	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Images []struct {
				ImageURL string `json:"image_url"`
			} `json:"images"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("unmarshal request body: %v", err)
		}
		if len(body.Images) > 0 {
			gotURL = body.Images[0].ImageURL
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"created":1,"data":[]}`))
	}))
	defer srv.Close()

	c := client.NewWithProvider("test-key", srv.URL, "", types.ProviderOpenAI)
	req := &types.GenerateRequest{
		Model:     "my-model",
		Prompt:    "turn it blue",
		Size:      "1024x1024",
		ImageURLs: []string{path},
	}
	p := &provider.EffectiveProvider{BaseURL: "https://api.zeekai.cc", APIKey: "test-key", ProviderType: provider.Zeekai}
	if _, err := buildImagePlan(req, p); err != nil {
		t.Fatalf("buildImagePlan() error = %v", err)
	}
	_, _ = runImageEditsJSON(c, req, &imageDispatchCtx{isZeekai: true})

	if !strings.HasPrefix(gotURL, "data:image/png;base64,") {
		t.Fatalf("images[0].image_url = %q, want data:image/png;base64, prefix", gotURL)
	}
}
