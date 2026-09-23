package video

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// This file is the wire harness for the video conformance tests: a real HTTP
// server that records everything it receives, plus the provider/send plumbing
// that replays the production execution path.

// conformanceUploadedURL is the fake upload endpoint's response URL: upload
// provider tests assert the real send carries it.
const conformanceUploadedURL = "https://cdn.example/uploaded.png"

// conformanceRemoteURL stands in for an image the user already hosts publicly.
const conformanceRemoteURL = "https://cdn.example.com/remote.png"

// wireRecord is one HTTP request exactly as the server received it.
type wireRecord struct {
	Method string
	Host   string
	Path   string
	URI    string
	Body   string
}

// wireServer serves the generation endpoint plus, when the provider uploads,
// POST /v1/uploads/images. It records every request so a test can compare the
// plan body with the bytes the real client sent.
type wireServer struct {
	*httptest.Server
	mu      sync.Mutex
	records []wireRecord
	uploads int
}

func newWireServer(t *testing.T) *wireServer {
	t.Helper()
	ws := &wireServer{}
	ws.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusInternalServerError)
			return
		}
		isUpload := strings.HasSuffix(r.URL.Path, client.UploadPath)
		ws.mu.Lock()
		ws.records = append(ws.records, wireRecord{
			Method: r.Method,
			Host:   r.Host,
			Path:   r.URL.Path,
			URI:    r.URL.RequestURI(),
			Body:   string(body),
		})
		if isUpload {
			ws.uploads++
		}
		ws.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if isUpload {
			_, _ = io.WriteString(w, `{"url":"`+conformanceUploadedURL+`"}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(ws.Close)
	return ws
}

// generation returns the last recorded request that is not an upload.
func (ws *wireServer) generation(t *testing.T) wireRecord {
	t.Helper()
	ws.mu.Lock()
	defer ws.mu.Unlock()
	for i := len(ws.records) - 1; i >= 0; i-- {
		if !strings.HasSuffix(ws.records[i].Path, client.UploadPath) {
			return ws.records[i]
		}
	}
	t.Fatal("no generation request recorded")
	return wireRecord{}
}

// uploadCount returns how many upload requests the server received.
func (ws *wireServer) uploadCount() int {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return ws.uploads
}

// conformanceProvider builds the provider production resolves for a backend:
// OpenAI protocol, key configured, base URL pointed at the fake server.
func conformanceProvider(baseURL string, pt provider.Type) *provider.EffectiveProvider {
	return &provider.EffectiveProvider{
		Type:         types.ProviderOpenAI,
		APIKey:       "sk-conformance",
		BaseURL:      baseURL,
		ProviderType: pt,
	}
}

// conformanceSend is the real client call a runner performs for one provider.
type conformanceSend func(c *client.Client, req *types.VideoGenerateRequest) error

// runConformance executes the production path for one scenario: build the plan,
// resolve its uploads with the real client, perform the send, then return the
// plan and the request the server captured.
func runConformance(t *testing.T, ws *wireServer, p *provider.EffectiveProvider, req *types.VideoGenerateRequest, send conformanceSend) (*videoPlan, wireRecord) {
	t.Helper()
	plan, err := buildVideoPlan(req, p)
	if err != nil {
		t.Fatalf("buildVideoPlan() error = %v", err)
	}
	c := client.NewFromProvider(p)
	if err := plan.applyUploads(c, req); err != nil {
		t.Fatalf("applyUploads() error = %v", err)
	}
	if err := send(c, req); err != nil {
		t.Fatalf("send request: %v", err)
	}
	return plan, ws.generation(t)
}

// The submit* helpers are the runner submission calls, kept as named functions
// so each conformance test sends exactly what the runner sends.

func submitVideo(c *client.Client, req *types.VideoGenerateRequest) error {
	_, err := c.VideoSubmit(req)
	return err
}

func submitOpenLuxVideo(c *client.Client, req *types.VideoGenerateRequest) error {
	_, err := c.OpenLuxVideoSubmit(req)
	return err
}

func submitOpenRouterVideo(c *client.Client, req *types.VideoGenerateRequest) error {
	_, err := c.OpenRouterVideoSubmit(openRouterVideoBody(req))
	return err
}

func submitAgnesVideo(c *client.Client, req *types.VideoGenerateRequest) error {
	_, err := c.AgnesVideoSubmit(req)
	return err
}

func generatePollinationsVideo(c *client.Client, req *types.VideoGenerateRequest) error {
	_, _, err := c.PollinationsVideoGenerate(req)
	return err
}
