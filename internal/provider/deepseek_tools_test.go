package provider

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDeepSeekToolURL(t *testing.T) {
	tests := []struct {
		base, tool, want string
	}{
		{"https://api.deepseek.com", "ocr", "https://api.deepseek.com/v1/tools/ocr"},
		{"https://api.deepseek.com/", "asr", "https://api.deepseek.com/v1/tools/asr"},
		{"https://api.deepseek.com/v1", "tts", "https://api.deepseek.com/v1/tools/tts"},
		{"https://api.deepseek.com/v1/", "tts", "https://api.deepseek.com/v1/tools/tts"},
		{"https://relay.example.com/deepseek", "ocr", "https://relay.example.com/deepseek/v1/tools/ocr"},
		{"", "tts", DeepSeekBaseURL + "/v1/tools/tts"},
	}
	for _, tt := range tests {
		if got := DeepSeekToolURL(tt.base, tt.tool); got != tt.want {
			t.Errorf("DeepSeekToolURL(%q, %q) = %q, want %q", tt.base, tt.tool, got, tt.want)
		}
	}
}

func TestIsDeepSeekProvider(t *testing.T) {
	if IsDeepSeekProvider(nil) {
		t.Error("nil provider must not be DeepSeek")
	}
	if !IsDeepSeekProvider(&EffectiveProvider{Type: types.ProviderDeepSeek}) {
		t.Error("type=deepseek must be DeepSeek")
	}
	if !IsDeepSeekProvider(&EffectiveProvider{ProviderType: DeepSeek}) {
		t.Error("detected DeepSeek host must be DeepSeek")
	}
	if IsDeepSeekProvider(&EffectiveProvider{Type: types.ProviderOpenAI, ProviderType: OpenAI}) {
		t.Error("plain OpenAI provider must not be DeepSeek")
	}
}

func TestNormalizeDeepSeekOCRPurpose(t *testing.T) {
	for _, input := range []string{"", "text_recognition", "TEXT_RECOGNITION", " rich_layout_recognition "} {
		if _, err := NormalizeDeepSeekOCRPurpose(input); err != nil {
			t.Errorf("NormalizeDeepSeekOCRPurpose(%q) error = %v", input, err)
		}
	}
	if got, _ := NormalizeDeepSeekOCRPurpose("rich_layout_recognition"); got != DeepSeekOCRRichLayout {
		t.Errorf("purpose = %q, want %q", got, DeepSeekOCRRichLayout)
	}
	if _, err := NormalizeDeepSeekOCRPurpose("markdown"); err == nil {
		t.Error("unsupported purpose should fail")
	}
}

func TestDeepSeekOCR(t *testing.T) {
	file := writeTempFile(t, "doc.pdf", "%PDF-1.4")
	var gotPurpose, gotFile string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tools/ocr" {
			t.Errorf("path = %q, want /v1/tools/ocr", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		gotPurpose = r.FormValue("purpose")
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		gotFile = string(data)

		_, _ = w.Write([]byte(`{
			"id":"ocr-1","object":"ocr.result","created":1,"purpose":"text_recognition",
			"page_count":2,
			"pages":[{"index":0,"text":"page one","finish_reason":"stop"},{"index":1,"text":"page two","finish_reason":"length"}],
			"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}
		}`))
	}))
	defer srv.Close()

	p := &EffectiveProvider{Name: "deepseek", Type: types.ProviderDeepSeek, BaseURL: srv.URL, APIKey: "sk-test"}
	result, err := DeepSeekOCR(p, file, "text_recognition")
	if err != nil {
		t.Fatalf("DeepSeekOCR: %v", err)
	}
	if gotPurpose != "text_recognition" {
		t.Errorf("purpose = %q", gotPurpose)
	}
	if gotFile != "%PDF-1.4" {
		t.Errorf("file content = %q", gotFile)
	}
	if result.PageCount != 2 || len(result.Pages) != 2 {
		t.Fatalf("pages = %d/%d, want 2", result.PageCount, len(result.Pages))
	}
	if got := result.Text(); got != "page one\n\npage two" {
		t.Errorf("Text() = %q", got)
	}
	if result.Usage.TotalTokens != 7 {
		t.Errorf("usage = %+v", result.Usage)
	}
}

func TestDeepSeekOCR_inBodyError(t *testing.T) {
	file := writeTempFile(t, "doc.pdf", "x")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"message":"OCR did not finish in time","type":"gateway_timeout_error"}}`))
	}))
	defer srv.Close()

	p := &EffectiveProvider{BaseURL: srv.URL, APIKey: "k"}
	_, err := DeepSeekOCR(p, file, "")
	if err == nil || !strings.Contains(err.Error(), "OCR did not finish in time") {
		t.Fatalf("err = %v, want gateway error message", err)
	}
}

func TestDeepSeekTranscribe(t *testing.T) {
	file := writeTempFile(t, "speech.wav", "RIFF")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tools/asr" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if _, _, err := r.FormFile("file"); err != nil {
			t.Fatalf("form file: %v", err)
		}
		_, _ = w.Write([]byte(`{"request_id":"r1","text":"你好","duration_ms":3200}`))
	}))
	defer srv.Close()

	p := &EffectiveProvider{BaseURL: srv.URL, APIKey: "k"}
	result, err := DeepSeekTranscribe(p, file)
	if err != nil {
		t.Fatalf("DeepSeekTranscribe: %v", err)
	}
	if result.Text != "你好" || result.DurationMS != 3200 {
		t.Errorf("result = %+v", result)
	}
}

func TestDeepSeekTTS(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tools/tts" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("wav-bytes"))
	}))
	defer srv.Close()

	p := &EffectiveProvider{BaseURL: srv.URL, APIKey: "k"}
	data, contentType, err := DeepSeekTTS(p, "你好", "", "", nil)
	if err != nil {
		t.Fatalf("DeepSeekTTS: %v", err)
	}
	if string(data) != "wav-bytes" || contentType != "audio/wav" {
		t.Errorf("data = %q, contentType = %q", data, contentType)
	}
	if got["voice_id"] != "stella" || got["format"] != "wav" || got["text"] != "你好" {
		t.Errorf("payload = %v", got)
	}
	if _, ok := got["filter_markdown"]; ok {
		t.Errorf("filter_markdown should be omitted when nil: %v", got)
	}
}

func TestDeepSeekTTS_rejectsBadInput(t *testing.T) {
	p := &EffectiveProvider{BaseURL: "https://api.deepseek.com", APIKey: "k"}
	if _, _, err := DeepSeekTTS(p, "hi", "alloy", "wav", nil); err == nil {
		t.Error("expected unsupported voice error")
	}
	if _, _, err := DeepSeekTTS(p, "hi", "stella", "mp3", nil); err == nil {
		t.Error("expected unsupported format error")
	}
	if _, _, err := DeepSeekTTS(p, "  ", "stella", "wav", nil); err == nil {
		t.Error("expected empty text error")
	}
	if _, _, err := DeepSeekTTS(p, strings.Repeat("x", 64<<10+1), "stella", "wav", nil); err == nil {
		t.Error("expected text-too-large error")
	}
}

func TestDeepSeekPost_httpError(t *testing.T) {
	file := writeTempFile(t, "speech.wav", "x")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"file is too large: max 8 MiB"}}`, http.StatusRequestEntityTooLarge)
	}))
	defer srv.Close()

	p := &EffectiveProvider{BaseURL: srv.URL, APIKey: "k"}
	_, err := DeepSeekTranscribe(p, file)
	if err == nil || !strings.Contains(err.Error(), "413") || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("err = %v", err)
	}
}

func TestDeepSeekTranscribe_rejectsUnsupportedInput(t *testing.T) {
	p := &EffectiveProvider{BaseURL: "https://api.deepseek.com", APIKey: "k"}

	mp3 := writeTempFile(t, "speech.mp3", "ID3")
	if _, err := DeepSeekTranscribe(p, mp3); err == nil || !strings.Contains(err.Error(), "WAV") {
		t.Errorf("mp3 should be rejected, got %v", err)
	}

	large := writeTempFile(t, "large.wav", "")
	if err := os.Truncate(large, deepSeekMaxASRBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, err := DeepSeekTranscribe(p, large); err == nil || !strings.Contains(err.Error(), "8 MiB") {
		t.Errorf("oversized file should be rejected, got %v", err)
	}
}
