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

func TestImageEditsStrategyMatch(t *testing.T) {
	withImage := &types.GenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{"a.png"}}
	noImage := &types.GenerateRequest{Model: "m", Prompt: "p"}

	tests := []struct {
		name      string
		req       *types.GenerateRequest
		ctx       *imageDispatchCtx
		wantEdits bool
	}{
		{"edits enabled with images", withImage, &imageDispatchCtx{imageEdits: true}, true},
		{"edits enabled without images", noImage, &imageDispatchCtx{imageEdits: true}, false},
		{"edits disabled with images", withImage, &imageDispatchCtx{}, false},
		{"edits disabled without images", noImage, &imageDispatchCtx{}, false},
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

func TestResolveImageEditsMode(t *testing.T) {
	tests := []struct {
		name        string
		providerVal string
		flagVal     string
		want        string
		wantErr     bool
	}{
		{"both unset", "", "", "", false},
		{"provider json", "json", "", "json", false},
		{"flag json", "", "json", "json", false},
		{"flag wins", "json", "json", "json", false},
		{"unknown provider value", "xml", "", "", true},
		{"unknown flag value", "", "xml", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveImageEditsMode(tc.providerVal, tc.flagVal)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "json") {
					t.Errorf("error should list supported value: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("mode = %q, want %q", got, tc.want)
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
	_, _ = runImageEditsJSON(c, req, &imageDispatchCtx{imageEdits: true})

	if !strings.HasPrefix(gotURL, "data:image/png;base64,") {
		t.Fatalf("images[0].image_url = %q, want data:image/png;base64, prefix", gotURL)
	}
}
