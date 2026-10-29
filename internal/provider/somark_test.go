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

// minimalPNG is a valid 1x1 PNG used as a fake document upload.
var minimalPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
	0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
	0x54, 0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
	0x00, 0x00, 0x03, 0x00, 0x01, 0x26, 0xE0, 0xFE,
	0x69, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E,
	0x44, 0xAE, 0x42, 0x60, 0x82,
}

// writeTempDocument writes the minimal PNG into a temp file and returns its path.
func writeTempDocument(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.png")
	if err := os.WriteFile(path, minimalPNG, 0644); err != nil {
		t.Fatalf("write temp document: %v", err)
	}
	return path
}

func TestSomarkParseURL(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{SomarkBaseURL, "https://somark.ai/api/v1/parse/sync"},
		{"https://somark.ai/api/v1/", "https://somark.ai/api/v1/parse/sync"},
		{"https://somark.ai", "https://somark.ai/api/v1/parse/sync"},
		{"https://free-oxr.somark.ai/api/v1", "https://free-oxr.somark.ai/api/v1/parse/sync"},
		{"https://somark.cn/api/v1", "https://somark.cn/api/v1/parse/sync"},
		// Self-hosted OXR: docker exposes http://host:8000/v1.
		{"http://localhost:8000/v1", "http://localhost:8000/v1/parse/sync"},
		{"http://localhost:8000", "http://localhost:8000/v1/parse/sync"},
		{"http://localhost:8000/api", "http://localhost:8000/api/v1/parse/sync"},
		{"", "https://somark.ai/api/v1/parse/sync"},
	}
	for _, tc := range tests {
		if got := SomarkParseURL(tc.base); got != tc.want {
			t.Errorf("SomarkParseURL(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}
}

func TestDetectSomark(t *testing.T) {
	for _, base := range []string{
		"https://somark.ai/api/v1",
		"https://somark.cn/api/v1",
		"https://free-oxr.somark.ai/api/v1",
	} {
		if got := Detect(base); got != Somark {
			t.Errorf("Detect(%q) = %v, want Somark", base, got)
		}
	}
	if Detect("https://api.apimart.ai") == Somark {
		t.Error("APIMart must not be detected as SoMark")
	}
}

func TestIsDocumentProvider(t *testing.T) {
	tests := []struct {
		name string
		p    *EffectiveProvider
		want bool
	}{
		{"nil", nil, false},
		{"explicit type", &EffectiveProvider{Type: types.ProviderSomark}, true},
		{"detected base", &EffectiveProvider{ProviderType: Somark}, true},
		{"openai", &EffectiveProvider{Type: types.ProviderOpenAI, ProviderType: OpenAI}, false},
	}
	for _, tc := range tests {
		if got := IsDocumentProvider(tc.p); got != tc.want {
			t.Errorf("%s: IsDocumentProvider = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSomarkRequestBody(t *testing.T) {
	// No --json: the defaults are used verbatim.
	body, err := somarkRequestBody(nil, nil)
	if err != nil {
		t.Fatalf("somarkRequestBody: %v", err)
	}
	if !strings.Contains(string(body), `"output_formats":["markdown","json"]`) {
		t.Errorf("default body = %s", body)
	}

	// --inject-json is merged on top of the defaults.
	body, err = somarkRequestBody(nil, json.RawMessage(`{"feature_config":{"enable_title_level_recognition":true}}`))
	if err != nil {
		t.Fatalf("somarkRequestBody: %v", err)
	}
	var merged map[string]any
	if err := json.Unmarshal(body, &merged); err != nil {
		t.Fatalf("decode default+inject body: %v", err)
	}
	if _, ok := merged["feature_config"]; !ok {
		t.Errorf("inject lost: %s", body)
	}
	if _, ok := merged["output_formats"]; !ok {
		t.Errorf("defaults lost: %s", body)
	}

	// --json replaces the defaults; --inject-json still layers on top.
	body, err = somarkRequestBody(
		json.RawMessage(`{"output_formats":["json"]}`),
		json.RawMessage(`{"output_formats":["markdown","json"]}`),
	)
	if err != nil {
		t.Fatalf("somarkRequestBody: %v", err)
	}
	var replaced map[string]any
	if err := json.Unmarshal(body, &replaced); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	formats, _ := replaced["output_formats"].([]any)
	if len(formats) != 3 {
		t.Errorf("inject arrays must append (json + markdown,json), got %v", replaced["output_formats"])
	}
}

func TestSomarkFormFields(t *testing.T) {
	fields, err := somarkFormFields(json.RawMessage(defaultParsePayload), "")
	if err != nil {
		t.Fatalf("somarkFormFields: %v", err)
	}
	if got := fields["api_key"]; len(got) != 1 || got[0] != placeholderKey {
		t.Errorf("api_key = %v, want placeholder", got)
	}
	if got := fields["output_formats"]; len(got) != 2 || got[0] != "markdown" || got[1] != "json" {
		t.Errorf("output_formats = %v", got)
	}
	var elements map[string]string
	if err := json.Unmarshal([]byte(fields["element_formats"][0]), &elements); err != nil {
		t.Fatalf("decode element_formats: %v", err)
	}
	if elements["table"] != "markdown" || elements["formula"] != "latex" || elements["image"] != "url" {
		t.Errorf("element_formats = %v", elements)
	}
}

func TestSomarkFormFields_RendersJSONTypes(t *testing.T) {
	body := json.RawMessage(`{
		"output_formats": ["markdown"],
		"feature_config": {"keep_header_footer": true},
		"extra": {"temperature": 0.2, "top_p": 0.9},
		"page_hint": 3,
		"api_key": "from-body",
		"file": "ignored.pdf"
	}`)
	fields, err := somarkFormFields(body, "sk-provider")
	if err != nil {
		t.Fatalf("somarkFormFields: %v", err)
	}
	if fields["api_key"][0] != "sk-provider" {
		t.Errorf("api_key = %v, the provider key must win", fields["api_key"])
	}
	if _, ok := fields["file"]; ok {
		t.Error("file is CLI-owned and must not come from the body")
	}
	if len(fields["output_formats"]) != 1 {
		t.Errorf("output_formats = %v", fields["output_formats"])
	}
	if fields["page_hint"][0] != "3" {
		t.Errorf("numbers must render without an exponent: %v", fields["page_hint"])
	}
	var extra map[string]float64
	if err := json.Unmarshal([]byte(fields["extra"][0]), &extra); err != nil {
		t.Fatalf("decode extra: %v", err)
	}
	if extra["temperature"] != 0.2 {
		t.Errorf("extra = %v", extra)
	}
}

func TestSomarkFormFields_RejectsNonObjectBody(t *testing.T) {
	if _, err := somarkFormFields(json.RawMessage(`["markdown"]`), ""); err == nil {
		t.Fatal("expected an error for a non-object request body")
	}
}

func TestParseDocument_success(t *testing.T) {
	var gotFields map[string][]string
	var gotFile string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/parse/sync" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotFields = r.MultipartForm.Value
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("form file: %v", err)
		} else {
			data, _ := io.ReadAll(file)
			gotFile = string(data)
		}
		io.WriteString(w, `{"code":0,"message":"ok","warnings":["enable_title_level_recognition is not supported"],
			"data":{"task_id":"abc123","error":null,"metadata":{"page_num":2,"file_type":".pdf"},
			"result":{"file_name":"doc.pdf","outputs":{"markdown":"# Title\n\nbody","json":{"pages":[]}}}}}`)
	}))
	defer srv.Close()

	p := &EffectiveProvider{Name: "oxr", Type: types.ProviderSomark, BaseURL: srv.URL + "/v1", APIKey: "dummy"}
	inject := json.RawMessage(`{"feature_config":{"enable_title_level_recognition":true}}`)
	res, err := ParseDocument(p, writeTempDocument(t), nil, inject)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if gotFile != string(minimalPNG) {
		t.Error("uploaded file body does not match the document")
	}
	if gotFields["api_key"][0] != "dummy" {
		t.Errorf("api_key = %v", gotFields["api_key"])
	}
	if !strings.Contains(gotFields["feature_config"][0], "enable_title_level_recognition") {
		t.Errorf("feature_config = %v", gotFields["feature_config"])
	}
	if res.TaskID != "abc123" || res.PageNum != 2 || res.Markdown != "# Title\n\nbody" {
		t.Errorf("unexpected result: %+v", res)
	}
	if string(res.JSON) != `{"pages":[]}` {
		t.Errorf("JSON = %s", res.JSON)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "not supported") {
		t.Errorf("Warnings = %v", res.Warnings)
	}
}

func TestDecodeSomarkResponse_ZipOutput(t *testing.T) {
	res, err := decodeSomarkResponse([]byte(`{"code":0,"data":{"result":{"outputs":{
		"zip":"https://oss.example.com/doc.zip"}}}}`))
	if err != nil {
		t.Fatalf("decodeSomarkResponse: %v", err)
	}
	if res.ZipURL != "https://oss.example.com/doc.zip" {
		t.Errorf("ZipURL = %q", res.ZipURL)
	}
}

func TestParseDocument_apiErrorCode(t *testing.T) {
	// The real API answers errors with HTTP 200, code != 0 and data: "".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"code":1107,"message":"Invalid API key.","data":""}`)
	}))
	defer srv.Close()

	p := &EffectiveProvider{Type: types.ProviderSomark, BaseURL: srv.URL + "/v1", APIKey: "bad"}
	_, err := ParseDocument(p, writeTempDocument(t), nil, nil)
	if err == nil {
		t.Fatal("expected error for non-zero code")
	}
	if !strings.Contains(err.Error(), "1107") || !strings.Contains(err.Error(), "Invalid API key.") {
		t.Errorf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "--api-key") {
		t.Errorf("error should hint at the missing key: %v", err)
	}
}

func TestSomarkErrorHint(t *testing.T) {
	if somarkErrorHint(1107) == "" || somarkErrorHint(1133) == "" {
		t.Error("known codes need hints")
	}
	if somarkErrorHint(9999) != "" {
		t.Error("unknown code must not produce a hint")
	}
}

func TestParseDocument_httpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, "upstream down")
	}))
	defer srv.Close()

	p := &EffectiveProvider{Type: types.ProviderSomark, BaseURL: srv.URL + "/v1"}
	_, err := ParseDocument(p, writeTempDocument(t), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("error = %v, want status 502", err)
	}
}

func TestParseDocument_missingFile(t *testing.T) {
	p := &EffectiveProvider{Type: types.ProviderSomark, BaseURL: "http://127.0.0.1:1/v1"}
	if _, err := ParseDocument(p, filepath.Join(t.TempDir(), "nope.pdf"), nil, nil); err == nil {
		t.Fatal("expected error for missing file")
	}
	if _, err := ParseDocument(nil, "x.pdf", nil, nil); err == nil {
		t.Fatal("expected error for nil provider")
	}
}

func TestSomarkUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/usage" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
		} else if got := r.MultipartForm.Value["api_key"]; len(got) != 1 || got[0] != "sk-test" {
			t.Errorf("api_key = %v", got)
		}
		io.WriteString(w, `{"code":0,"message":"","data":{"remaining_paid_pages":0,
			"remaining_free_pages_today":423,"remaining_free_pages_this_month":1923,
			"daily_free_limit":500,"monthly_free_limit":2000,
			"dashboard_url":"https://somark.tech/workbench"}}`)
	}))
	defer srv.Close()

	p := &EffectiveProvider{Type: types.ProviderSomark, BaseURL: srv.URL + "/v1", APIKey: "sk-test"}
	usage, err := GetSomarkUsage(p)
	if err != nil {
		t.Fatalf("GetSomarkUsage: %v", err)
	}
	if usage.Data.RemainingFreePagesToday != 423 || usage.Data.DailyFreeLimit != 500 {
		t.Errorf("usage = %+v", usage.Data)
	}
	if usage.Data.DashboardURL != "https://somark.tech/workbench" {
		t.Errorf("dashboard = %q", usage.Data.DashboardURL)
	}
}

func TestGetSomarkUsage_InvalidKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"code":1107,"message":"Invalid API key.","data":""}`)
	}))
	defer srv.Close()

	p := &EffectiveProvider{Type: types.ProviderSomark, BaseURL: srv.URL + "/v1", APIKey: "bad"}
	_, err := GetSomarkUsage(p)
	if err == nil || !strings.Contains(err.Error(), "1107") || !strings.Contains(err.Error(), "--api-key") {
		t.Fatalf("err = %v, want the key hint", err)
	}
	if _, err := GetSomarkUsage(nil); err == nil {
		t.Fatal("expected an error for a nil provider")
	}
}

func TestOCRImage_routesToSomark(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"code":0,"message":"ok","data":{"result":{"outputs":{"markdown":"\n# Invoice\n\ntotal 42\n"}}}}`)
	}))
	defer srv.Close()

	p := &EffectiveProvider{Type: types.ProviderSomark, BaseURL: srv.URL + "/v1"}
	text, err := OCRImage(p, writeTempDocument(t), "")
	if err != nil {
		t.Fatalf("OCRImage: %v", err)
	}
	if text != "# Invoice\n\ntotal 42" {
		t.Errorf("text = %q", text)
	}

	if _, err := DescribeImage(p, writeTempDocument(t), "describe"); err == nil {
		t.Error("DescribeImage should reject document-parse providers")
	}
}
