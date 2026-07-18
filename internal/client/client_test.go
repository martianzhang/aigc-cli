package client

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestProgressBar_zero(t *testing.T) {
	got := progressBar(0, 20)
	if !strings.Contains(got, "░") {
		t.Errorf("progressBar(0, 20) should be all dots, got %q", got)
	}
}

func TestProgressBar_full(t *testing.T) {
	got := progressBar(100, 20)
	if !strings.Contains(got, "█") {
		t.Errorf("progressBar(100, 20) should be all blocks, got %q", got)
	}
	if strings.Contains(got, "░") {
		t.Errorf("progressBar(100, 20) should have no dots, got %q", got)
	}
}

func TestProgressBar_half(t *testing.T) {
	got := progressBar(50, 20)
	blockCount := strings.Count(got, "█")
	dotCount := strings.Count(got, "░")
	if blockCount != 10 {
		t.Errorf("progressBar(50, 20) should have 10 blocks, got %d", blockCount)
	}
	if dotCount != 10 {
		t.Errorf("progressBar(50, 20) should have 10 dots, got %d", dotCount)
	}
}

func TestProgressBar_customWidth(t *testing.T) {
	got := progressBar(25, 40)
	blockCount := strings.Count(got, "█")
	if blockCount != 10 {
		t.Errorf("progressBar(25, 40) should have 10 blocks, got %d", blockCount)
	}
}

func TestIsLocalFile_exists(t *testing.T) {
	tmp, _ := os.CreateTemp("", "testfile")
	tmp.Close()
	defer os.Remove(tmp.Name())

	if !isLocalFile(tmp.Name()) {
		t.Errorf("isLocalFile(%q) should be true", tmp.Name())
	}
}

func TestIsLocalFile_notExists(t *testing.T) {
	if isLocalFile("/tmp/nonexistent_file_xyz") {
		t.Error("isLocalFile() should be false for nonexistent file")
	}
}

func TestIsLocalFile_directory(t *testing.T) {
	dir, _ := os.MkdirTemp("", "testdir")
	defer os.Remove(dir)

	if isLocalFile(dir) {
		t.Error("isLocalFile() should be false for directory")
	}
}

// ---------------------------------------------------------------------------
// hasVersionSuffix tests
// ---------------------------------------------------------------------------

func TestHasVersionSuffix_v1(t *testing.T) {
	if !HasVersionSuffix("https://api.openai.com/v1") {
		t.Error("HasVersionSuffix should detect /v1")
	}
}

func TestHasVersionSuffix_v2(t *testing.T) {
	if !HasVersionSuffix("https://relay.com/v2") {
		t.Error("HasVersionSuffix should detect /v2")
	}
}

func TestHasVersionSuffix_noVersion(t *testing.T) {
	if HasVersionSuffix("https://api.openai.com") {
		t.Error("HasVersionSuffix should be false for bare domain")
	}
}

func TestHasVersionSuffix_trailingSlash(t *testing.T) {
	if HasVersionSuffix("https://api.openai.com/") {
		t.Error("HasVersionSuffix should be false for trailing slash")
	}
}

func TestHasVersionSuffix_nonNumeric(t *testing.T) {
	if HasVersionSuffix("https://example.com/version") {
		t.Error("HasVersionSuffix should be false for non-numeric segment")
	}
}

func TestHasVersionSuffix_emptyLastSegment(t *testing.T) {
	if HasVersionSuffix("https://example.com/v") {
		t.Error("HasVersionSuffix should be false for just 'v'")
	}
}

// ---------------------------------------------------------------------------
// New client normalization tests
// ---------------------------------------------------------------------------

func TestNew_defaultBaseURL(t *testing.T) {
	c := New("test-key", "", "")
	if c.baseURL != defaultBaseURL+"/v1" {
		t.Errorf("New() with empty baseURL = %q, want %q", c.baseURL, defaultBaseURL+"/v1")
	}
}

func TestNew_appendsV1(t *testing.T) {
	c := New("test-key", "https://api.openai.com", "")
	if !strings.HasSuffix(c.baseURL, "/v1") {
		t.Errorf("New() should append /v1, got %q", c.baseURL)
	}
}

func TestNew_preservesExistingVersion(t *testing.T) {
	c := New("test-key", "https://openrouter.ai/api/v1", "")
	want := "https://openrouter.ai/api/v1"
	if c.baseURL != want {
		t.Errorf("New() = %q, want %q", c.baseURL, want)
	}
}

func TestNew_preservesV2(t *testing.T) {
	c := New("test-key", "https://relay.com/v2", "")
	want := "https://relay.com/v2"
	if c.baseURL != want {
		t.Errorf("New() = %q, want %q", c.baseURL, want)
	}
}

func TestNew_trimTrailingSlash(t *testing.T) {
	c := New("test-key", "https://api.openai.com/v1/", "")
	if strings.HasSuffix(c.baseURL, "/") {
		t.Errorf("New() should trim trailing slash, got %q", c.baseURL)
	}
}

func TestNew_httpClientConfigured(t *testing.T) {
	c := New("test-key", "https://api.openai.com/v1", "")
	if c.httpClient == nil {
		t.Fatal("New() should set httpClient")
	}
	if c.httpClient.Timeout != defaultHTTPTimeout {
		t.Errorf("httpClient.Timeout = %v, want %v", c.httpClient.Timeout, defaultHTTPTimeout)
	}
}

func TestNew_apiKeyStored(t *testing.T) {
	c := New("sk-my-key", "https://api.openai.com/v1", "")
	if c.apiKey != "sk-my-key" {
		t.Errorf("apiKey = %q", c.apiKey)
	}
}

// ---------------------------------------------------------------------------
// setOpenRouterHeaders tests
// ---------------------------------------------------------------------------

func TestSetOpenRouterHeaders_envNotSet(t *testing.T) {
	// Ensure env vars are not set
	os.Unsetenv("OPENAI_REFERER")
	os.Unsetenv("OPENAI_APP_TITLE")

	c := New("test-key", "https://openrouter.ai/api/v1", "")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	c.setOpenRouterHeaders(req)

	// When env vars are not set, built-in defaults should apply.
	if req.Header.Get("HTTP-Referer") == "" {
		t.Error("HTTP-Referer should fall back to default when env is empty")
	}
	if req.Header.Get("X-OpenRouter-Title") == "" {
		t.Error("X-OpenRouter-Title should fall back to default when env is empty")
	}
	if req.Header.Get("User-Agent") == "" {
		t.Error("User-Agent should be set by default")
	}
}

func TestSetOpenRouterHeaders_envSet(t *testing.T) {
	os.Setenv("OPENAI_REFERER", "https://myapp.com")
	os.Setenv("OPENAI_APP_TITLE", "MyApp")
	defer func() {
		os.Unsetenv("OPENAI_REFERER")
		os.Unsetenv("OPENAI_APP_TITLE")
	}()

	c := New("test-key", "https://openrouter.ai/api/v1", "")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	c.setOpenRouterHeaders(req)

	if req.Header.Get("HTTP-Referer") != "https://myapp.com" {
		t.Errorf("HTTP-Referer = %q", req.Header.Get("HTTP-Referer"))
	}
	if req.Header.Get("X-OpenRouter-Title") != "MyApp" {
		t.Errorf("X-OpenRouter-Title = %q", req.Header.Get("X-OpenRouter-Title"))
	}
}

// ---------------------------------------------------------------------------
// HTTP client behavior tests (httptest, no API key required)
// ---------------------------------------------------------------------------

func TestClientGetTask_notFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":404,"message":"not found"}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "")
	_, err := c.GetTask("task_nonexistent")
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestClientGetTask_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"code": 200,
			"data": {
				"id": "task_abc",
				"status": "completed",
				"progress": 100,
				"cost": 0.05,
				"actual_time": 12
			}
		}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "")
	task, err := c.GetTask("task_abc")
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if task.ID != "task_abc" {
		t.Errorf("task.ID = %q", task.ID)
	}
	if task.Status != "completed" {
		t.Errorf("task.Status = %q", task.Status)
	}
	if task.Cost != 0.05 {
		t.Errorf("task.Cost = %f", task.Cost)
	}
}

func TestClientImageGenerateSync_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"created": 1782527505,
			"data": [{"b64_json": "dGVzdCBpbWFnZSBkYXRh"}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 100, "cost": 0.006}
		}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "")
	req := &types.GenerateRequest{Model: "test-model", Prompt: "a cat"}
	resp, err := c.OpenRouterDedicatedImage(req)
	if err != nil {
		t.Fatalf("OpenRouterDedicatedImage() error = %v", err)
	}
	if resp.Created != 1782527505 {
		t.Errorf("Created = %d", resp.Created)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 image, got %d", len(resp.Data))
	}
	if resp.Data[0].B64JSON != "dGVzdCBpbWFnZSBkYXRh" {
		t.Errorf("B64JSON = %q", resp.Data[0].B64JSON)
	}
	if resp.Usage == nil || resp.Usage.Cost != 0.006 {
		t.Errorf("Usage.Cost = %v", resp.Usage)
	}
}

func TestClientImageGenerateSync_error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": {"message": "bad request"}}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "")
	_, err := c.OpenRouterDedicatedImage(&types.GenerateRequest{Model: "test", Prompt: "x"})
	if err == nil {
		t.Error("expected error for 400 response")
	}
}

func TestClientVideoSubmit_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "vid_abc123",
			"polling_url": "https://example.com/poll/vid_abc123",
			"status": "pending"
		}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "")
	req := &types.OpenRouterVideoRequest{Model: "google/veo-3.1", Prompt: "a dog"}
	resp, err := c.OpenRouterVideoSubmit(req)
	if err != nil {
		t.Fatalf("OpenRouterVideoSubmit() error = %v", err)
	}
	if resp.ID != "vid_abc123" {
		t.Errorf("ID = %q", resp.ID)
	}
	if resp.Status != "pending" {
		t.Errorf("Status = %q", resp.Status)
	}
}

func TestClientListModelsOpenAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"data": [
				{"id": "gpt-4o", "object": "model", "created": 1700000000, "owned_by": "openai"},
				{"id": "gpt-4.1-nano", "object": "model", "created": 1700000001, "owned_by": "openai"}
			]
		}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "")
	models, err := c.ListModelsOpenAI()
	if err != nil {
		t.Fatalf("ListModelsOpenAI() error = %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "gpt-4o" {
		t.Errorf("models[0].ID = %q", models[0].ID)
	}
}

func TestProgressBar_edgeCases(t *testing.T) {
	tests := []struct {
		pct   int
		width int
	}{
		{0, 10},
		{1, 10},
		{99, 10},
		{100, 10},
		{50, 1},
		{50, 100},
	}
	for _, tt := range tests {
		got := progressBar(tt.pct, tt.width)
		if utf8.RuneCountInString(got) != tt.width {
			t.Errorf("progressBar(%d, %d) length = %d, want %d", tt.pct, tt.width, len(got), tt.width)
		}
	}
}
