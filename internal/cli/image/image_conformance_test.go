package image

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// This file locks the "one source of truth" property of the image plan
// refactor: the body a --dry-run/--verbose preview shows (plan.Body) must equal
// the body the real client writes to the wire, per provider.
//
// Each case builds the plan with buildImagePlan, drives the same request
// through the real client method the dispatcher uses, and compares the preview
// against the bytes a real httptest.Server observed.
//
// The plan is built against the provider's normal BaseURL (an explicit
// ProviderType forces routing) while the client is pointed at the test server,
// because buildImagePlan routes any loopback BaseURL to the Ollama endpoint
// before it consults ProviderType — a 127.0.0.1 provider base would exercise a
// different strategy than the one under test. APIMart is the exception: its
// check runs first, so its plan is built directly on the conformance server.

// capturedRequest is one request body the conformance server observed.
type capturedRequest struct {
	path string
	body []byte
}

// conformanceServer records every request body and answers the endpoints the
// real image runners call. The generation response is deliberately minimal and
// provider-agnostic: OpenAI-compatible clients ignore the APIMart fields, and
// ModelScope's submit path stops on the missing task_id without entering its
// 3s poll loop.
type conformanceServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []capturedRequest
	uploads  int
}

func newConformanceServer(t *testing.T) *conformanceServer {
	t.Helper()
	cs := &conformanceServer{}
	cs.Server = httptest.NewServer(http.HandlerFunc(cs.handle))
	t.Cleanup(cs.Close)
	return cs
}

func (cs *conformanceServer) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	cs.mu.Lock()
	cs.requests = append(cs.requests, capturedRequest{path: r.URL.Path, body: body})
	upload := strings.HasSuffix(r.URL.Path, client.UploadPath)
	uploadURL := ""
	if upload {
		uploadURL = conformanceUploadURL(cs.uploads)
		cs.uploads++
	}
	cs.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch {
	case upload:
		_, _ = fmt.Fprintf(w, `{"url":%q}`, uploadURL)
	case strings.HasSuffix(r.URL.Path, client.GeminiInteractionsPath):
		_, _ = io.WriteString(w, `{"id":"conformance","steps":[{"type":"model_output","content":[{"type":"image","data":"aGVsbG8="}]}]}`)
	default:
		_, _ = io.WriteString(w, `{"code":0,"created":1,"data":[]}`)
	}
}

// conformanceUploadURL is the URL the fake upload endpoint returns for the nth upload.
func conformanceUploadURL(n int) string {
	return fmt.Sprintf("https://cdn.example/uploaded-%d.png", n)
}

// lastBodyFor returns the most recent body captured for path, failing when the
// request never reached the server.
func (cs *conformanceServer) lastBodyFor(t *testing.T, path string) []byte {
	t.Helper()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i := len(cs.requests) - 1; i >= 0; i-- {
		if cs.requests[i].path == path {
			return cs.requests[i].body
		}
	}
	paths := make([]string, len(cs.requests))
	for i, captured := range cs.requests {
		paths[i] = captured.path
	}
	t.Fatalf("no request captured for %s (captured: %v)", path, paths)
	return nil
}

func (cs *conformanceServer) uploadCount() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.uploads
}

// jsonDrift returns "" when both slices decode to the same JSON value, and a
// readable diff otherwise. Key order and HTML escaping are not part of JSON
// semantics, so the comparison happens on decoded values.
func jsonDrift(preview, sent []byte) string {
	var want, got any
	if err := json.Unmarshal(preview, &want); err != nil {
		return fmt.Sprintf("preview body is not valid JSON: %v\n  preview: %s", err, preview)
	}
	if err := json.Unmarshal(sent, &got); err != nil {
		return fmt.Sprintf("sent body is not valid JSON: %v\n  sent: %s", err, sent)
	}
	if !reflect.DeepEqual(want, got) {
		return fmt.Sprintf("preview and sent bodies differ:\n  preview: %s\n  sent:    %s", preview, sent)
	}
	return ""
}

// resolveUploads substitutes upload placeholder tokens with the URLs the fake
// upload endpoint returned. Substitution runs on the decoded JSON document so a
// token inside a verbatim --json body resolves even when the preview pipeline
// JSON-escaped it, and every substitution must actually fire — a preview that
// never embedded a placeholder would make the comparison vacuous.
func resolveUploads(t *testing.T, preview []byte, substitutions map[string]string) []byte {
	t.Helper()
	if len(substitutions) == 0 {
		return preview
	}
	var value any
	if err := json.Unmarshal(preview, &value); err != nil {
		t.Fatalf("preview body is not valid JSON: %v\n  preview: %s", err, preview)
	}
	used := map[string]bool{}
	resolved, err := json.Marshal(resolveUploadValue(value, substitutions, used))
	if err != nil {
		t.Fatalf("marshal resolved preview body: %v", err)
	}
	for token := range substitutions {
		if !used[token] {
			t.Fatalf("preview body never embedded placeholder %s:\n  preview: %s", token, preview)
		}
	}
	return resolved
}

func resolveUploadValue(value any, substitutions map[string]string, used map[string]bool) any {
	switch v := value.(type) {
	case string:
		if url, ok := substitutions[v]; ok {
			used[v] = true
			return url
		}
		return v
	case []any:
		for i := range v {
			v[i] = resolveUploadValue(v[i], substitutions, used)
		}
		return v
	case map[string]any:
		for key := range v {
			v[key] = resolveUploadValue(v[key], substitutions, used)
		}
		return v
	default:
		return v
	}
}

// requirePreviewMatchesSent is the conformance assertion: the preview body,
// with upload placeholders resolved, must equal the body actually sent.
func requirePreviewMatchesSent(t *testing.T, preview, sent []byte, substitutions map[string]string) {
	t.Helper()
	if drift := jsonDrift(resolveUploads(t, preview, substitutions), sent); drift != "" {
		t.Fatalf("preview body drifted from the body actually sent: %s", drift)
	}
}

// conformanceRequest is the shared fixture: one local reference image plus
// enough fields to exercise each provider's body mapping.
func conformanceRequest(local string) *types.GenerateRequest {
	n := 2
	return &types.GenerateRequest{
		Model:     "conformance-model",
		Prompt:    "a conformance cat",
		Size:      "1024x1024",
		Quality:   "high",
		N:         &n,
		ImageURLs: []string{local},
	}
}

func submitImage(c *client.Client, req *types.GenerateRequest) error {
	_, err := c.Submit(req)
	return err
}

// imageConformanceCase describes one provider's preview-vs-wire comparison.
type imageConformanceCase struct {
	name string
	// providerBase is the BaseURL the plan is built against; empty means the
	// conformance server itself, which only APIMart supports (its routing check
	// runs before buildImagePlan's loopback-to-Ollama branch).
	providerBase string
	providerType provider.Type
	// wantSuffix is appended to the normalized base by both the plan URL and
	// the client's request path.
	wantSuffix string
	req        func(local, mask string) *types.GenerateRequest
	send       func(c *client.Client, req *types.GenerateRequest) error
	// allowSendErr tolerates the error ModelScope's runner returns when the
	// fake submit response carries no task_id (the test only needs the POST).
	allowSendErr  bool
	wantUploads   int
	substitutions map[string]string
	wantSent      []string
	wantAbsent    []string
}

func imageConformanceCases() []imageConformanceCase {
	imageInput := func(local, _ string) *types.GenerateRequest { return conformanceRequest(local) }

	return []imageConformanceCase{
		{
			name:         "openai-compatible sync embeds the local file as a data uri",
			providerBase: "https://api.openai.com",
			providerType: provider.OpenAI,
			wantSuffix:   client.ImageSubmitPath,
			req:          imageInput,
			send: func(c *client.Client, req *types.GenerateRequest) error {
				_, err := c.ImageGenerateSync(req)
				return err
			},
			wantSent: []string{`"image_urls":["data:image/png;base64,`},
		},
		{
			name:         "openrouter dedicated api maps image_urls to input_references",
			providerBase: "https://openrouter.ai/api/v1",
			providerType: provider.OpenRouter,
			wantSuffix:   client.OpenRouterImagesPath,
			req:          imageInput,
			send: func(c *client.Client, req *types.GenerateRequest) error {
				_, err := c.OpenRouterDedicatedImage(req)
				return err
			},
			wantSent: []string{`"input_references":[{"image_url":{"url":"data:image/png;base64,`},
		},
		{
			name:         "gemini interactions maps the data uri into input items",
			providerBase: "https://generativelanguage.googleapis.com",
			providerType: provider.Gemini,
			wantSuffix:   client.GeminiInteractionsPath,
			req:          imageInput,
			send: func(c *client.Client, req *types.GenerateRequest) error {
				_, err := c.GeminiImageGenerate(req)
				return err
			},
			wantSent: []string{`"type":"image"`, `"mime_type":"image/png"`},
		},
		{
			name:         "modelscope async submit embeds the data uri",
			providerBase: "https://api-inference.modelscope.cn",
			providerType: provider.ModelScope,
			wantSuffix:   client.ImageSubmitPath,
			req:          imageInput,
			send: func(c *client.Client, req *types.GenerateRequest) error {
				_, err := runModelScopeImage(c, req, &imageDispatchCtx{modelScopeKey: "conformance-key"})
				return err
			},
			allowSendErr: true,
			wantSent:     []string{`"image_url":"data:image/png;base64,`},
		},
		{
			name:         "zeekai image input routes to images/edits",
			providerBase: "https://api.zeekai.cc",
			providerType: provider.Zeekai,
			wantSuffix:   client.ImageEditsPath,
			req:          imageInput,
			send: func(c *client.Client, req *types.GenerateRequest) error {
				_, err := c.ImageGenerateEdits(req)
				return err
			},
			wantSent:   []string{`"images":[{"image_url":"data:image/png;base64,`},
			wantAbsent: []string{`"image_urls"`},
		},
		{
			name:         "agnes nests the data uri under extra_body.image",
			providerBase: "https://api.agnes-ai.com",
			providerType: provider.Agnes,
			wantSuffix:   client.ImageSubmitPath,
			req:          imageInput,
			send: func(c *client.Client, req *types.GenerateRequest) error {
				_, err := c.ImageGenerateSync(req)
				return err
			},
			wantSent:   []string{`"extra_body":{"image":["data:image/png;base64,`},
			wantAbsent: []string{`"image_urls"`},
		},
		{
			name:          "apimart typed fields upload the local file and submit the resolved url",
			providerType:  provider.APIMart,
			wantSuffix:    client.ImageSubmitPath,
			req:           imageInput,
			send:          submitImage,
			wantUploads:   1,
			substitutions: map[string]string{reqbuild.Placeholder(0): conformanceUploadURL(0)},
			wantSent:      []string{`"image_urls":["https://cdn.example/uploaded-0.png"`},
		},
		{
			name:         "apimart typed fields keep remote urls untouched without uploads",
			providerType: provider.APIMart,
			wantSuffix:   client.ImageSubmitPath,
			req: func(_, _ string) *types.GenerateRequest {
				return &types.GenerateRequest{
					Model:     "conformance-model",
					Prompt:    "a conformance cat",
					ImageURLs: []string{"https://example.com/a.png"},
				}
			},
			send:     submitImage,
			wantSent: []string{`"image_urls":["https://example.com/a.png"]`},
		},
		{
			name:         "apimart uploads both the image and the mask",
			providerType: provider.APIMart,
			wantSuffix:   client.ImageSubmitPath,
			req: func(local, mask string) *types.GenerateRequest {
				req := conformanceRequest(local)
				req.MaskURL = mask
				return req
			},
			send:          submitImage,
			wantUploads:   2,
			substitutions: map[string]string{reqbuild.Placeholder(0): conformanceUploadURL(0), reqbuild.Placeholder(1): conformanceUploadURL(1)},
			wantSent:      []string{`"image_urls":["https://cdn.example/uploaded-0.png"`, `"mask_url":"https://cdn.example/uploaded-1.png"`},
		},
		{
			name:         "apimart verbatim json body has its local path uploaded",
			providerType: provider.APIMart,
			wantSuffix:   client.ImageSubmitPath,
			req: func(local, _ string) *types.GenerateRequest {
				raw, _ := json.Marshal(map[string]any{
					"model":      "conformance-model",
					"prompt":     "a conformance cat",
					"image_urls": []string{local},
					"seed":       42,
				})
				return &types.GenerateRequest{RawJSON: raw}
			},
			send:          submitImage,
			wantUploads:   1,
			substitutions: map[string]string{reqbuild.Placeholder(0): conformanceUploadURL(0)},
			wantSent:      []string{`"image_urls":["https://cdn.example/uploaded-0.png"`, `"seed":42`},
		},
	}
}

// TestImageConformancePreviewMatchesSent is the regression lock for the image
// plan refactor: per provider, the preview body equals the wire body.
func TestImageConformancePreviewMatchesSent(t *testing.T) {
	local := writeLocalPNG(t, t.TempDir())
	mask := writeLocalPNG(t, t.TempDir())

	for _, tc := range imageConformanceCases() {
		t.Run(tc.name, func(t *testing.T) {
			cs := newConformanceServer(t)
			c := client.NewWithProvider("conformance-key", cs.URL, "", types.ProviderOpenAI)

			// APIMart routes on ProviderType before the loopback-to-Ollama
			// branch, so it points straight at the conformance server; every
			// other provider keeps its real BaseURL so the plan routes to the
			// strategy under test.
			base := tc.providerBase
			if base == "" {
				base = cs.URL
			}
			p := &provider.EffectiveProvider{BaseURL: base, APIKey: "conformance-key", ProviderType: tc.providerType}

			req := tc.req(local, mask)
			pl := mustBuildPlan(t, req, p)

			if wantURL := client.NormalizeBaseURL(base) + tc.wantSuffix; pl.URL != wantURL {
				t.Fatalf("plan URL = %q, want %q", pl.URL, wantURL)
			}
			// Snapshot the preview before applyUploads rewrites the request.
			preview := []byte(planBodyJSON(t, pl.Body))

			if err := pl.applyUploads(c, req); err != nil {
				t.Fatalf("applyUploads() error = %v", err)
			}
			if err := tc.send(c, req); err != nil && !tc.allowSendErr {
				t.Fatalf("real client send failed: %v", err)
			}

			// The conformance server URL has no version segment, so the client
			// normalizes it to /v1 — the path the server must have observed.
			sent := cs.lastBodyFor(t, "/v1"+tc.wantSuffix)

			requirePreviewMatchesSent(t, preview, sent, tc.substitutions)

			body := string(sent)
			for _, want := range tc.wantSent {
				if !strings.Contains(body, want) {
					t.Errorf("sent body missing %q:\n%s", want, body)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(body, absent) {
					t.Errorf("sent body must not contain %q:\n%s", absent, body)
				}
			}
			if got := cs.uploadCount(); got != tc.wantUploads {
				t.Errorf("upload requests = %d, want %d", got, tc.wantUploads)
			}
			// A local path or an unresolved placeholder on the wire means the
			// preview resolver and the upload resolver disagree.
			for _, leak := range []string{local, mask, reqbuild.Placeholder(0), reqbuild.Placeholder(1)} {
				if strings.Contains(body, leak) {
					t.Errorf("sent body leaked %q:\n%s", leak, body)
				}
			}
		})
	}
}

// TestImageConformanceComparisonIsSensitive proves the comparison above is not
// vacuous: mutating either side must make jsonDrift report a difference.
func TestImageConformanceComparisonIsSensitive(t *testing.T) {
	local := writeLocalPNG(t, t.TempDir())
	cs := newConformanceServer(t)
	c := client.NewWithProvider("conformance-key", cs.URL, "", types.ProviderOpenAI)
	p := &provider.EffectiveProvider{BaseURL: "https://api.openai.com", APIKey: "conformance-key", ProviderType: provider.OpenAI}

	req := conformanceRequest(local)
	pl := mustBuildPlan(t, req, p)
	preview := []byte(planBodyJSON(t, pl.Body))
	if _, err := c.ImageGenerateSync(req); err != nil {
		t.Fatalf("send: %v", err)
	}
	sent := cs.lastBodyFor(t, "/v1"+client.ImageSubmitPath)

	if drift := jsonDrift(preview, sent); drift != "" {
		t.Fatalf("control comparison must pass before mutating: %s", drift)
	}

	mutatedReq := conformanceRequest(local)
	mutatedReq.Prompt = "a different cat"
	mutatedPlan := mustBuildPlan(t, mutatedReq, p)
	if drift := jsonDrift([]byte(planBodyJSON(t, mutatedPlan.Body)), sent); drift == "" {
		t.Fatal("comparison accepted a mutated plan body; the conformance harness would be vacuous")
	}

	mutatedSent := bytes.Replace(sent, []byte("a conformance cat"), []byte("a different cat"), 1)
	if drift := jsonDrift(preview, mutatedSent); drift == "" {
		t.Fatal("comparison accepted a mutated sent body; the conformance harness would be vacuous")
	}
}
