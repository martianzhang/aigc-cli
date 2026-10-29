package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/jsonmerge"
	"github.com/martianzhang/aigc-cli/internal/types"
)

const (
	// SomarkBaseURL is the hosted SoMark document-parse API. It is the default
	// endpoint for a provider declared with type: somark and no base_url.
	SomarkBaseURL = "https://somark.ai/api/v1"
	// placeholderKey fills the mandatory api_key form field for endpoints that
	// do not check it (the free OXR API, self-hosted OXR).
	placeholderKey = "dummy"
	// somarkTimeout is generous: SoMark allows up to 300 pages per request and
	// long documents take minutes.
	somarkTimeout = 15 * time.Minute
	// somarkMaxResponse caps the response body; base64 images can be large.
	somarkMaxResponse = 256 << 20
	// defaultParsePayload mirrors the API defaults, except that tables are
	// requested as Markdown since the CLI writes Markdown output.
	defaultParsePayload = `{"output_formats":["markdown","json"],"element_formats":{"image":"url","formula":"latex","table":"markdown"}}`
)

// ParseResult is a decoded successful /parse/sync response.
type ParseResult struct {
	TaskID           string
	FileName         string
	PageNum          int
	FileType         string
	ProcessingTimeMS int
	// Markdown is the full document in Markdown (empty unless requested).
	Markdown string
	// JSON is the raw structured result: pages → blocks with bbox and reading
	// order. Nil unless requested.
	JSON json.RawMessage
	// ZipURL is the download link for a requested zip export (Markdown plus
	// images). Present only when output_formats contains zip.
	ZipURL string
	// Catalog is the raw title tree; only present when title levels are on.
	Catalog json.RawMessage
	// Warnings lists parameters the server ignored (expected for OXR).
	Warnings []string
}

// IsDocumentProvider reports whether p speaks the SoMark document-parse
// protocol rather than OpenAI chat completions. Both an explicit
// type: somark and a known SoMark/OXR base URL qualify.
func IsDocumentProvider(p *EffectiveProvider) bool {
	if p == nil {
		return false
	}
	return p.Type == types.ProviderSomark || p.ProviderType == Somark
}

// SomarkParseURL builds the /parse/sync endpoint from a base URL.
func SomarkParseURL(baseURL string) string {
	return somarkAPIURL(baseURL, "/parse/sync")
}

// somarkAPIURL resolves one API path against a base URL. Bare SoMark hosts get
// the /api prefix the hosted API lives under, and a missing version segment
// defaults to /v1 so self-hosted OXR (http://host:8000) also works.
func somarkAPIURL(baseURL, path string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = SomarkBaseURL
	}
	if u, err := url.Parse(base); err == nil && (u.Path == "" || u.Path == "/") && IsSomark(base) {
		base += "/api/v1"
	}
	if strings.HasSuffix(base, "/api") {
		base += "/v1"
	}
	if !hasVersionSuffix(base) {
		base += "/v1"
	}
	return base + path
}

// ParseDocument uploads one document (PDF or image) to /parse/sync and waits
// for the parsed Markdown / JSON result.
//
// payload is the request body from --json (nil means the defaults above);
// inject is the --inject-json snippet merged on top. Both carry the unmodelled
// API parameters (output_formats, element_formats, feature_config, extra), so
// callers never need provider-specific flags. Top-level keys become form
// fields; CLI-owned keys (file, file_url, api_key) are always generated here.
func ParseDocument(p *EffectiveProvider, filePath string, payload, inject json.RawMessage) (*ParseResult, error) {
	if p == nil {
		return nil, fmt.Errorf("nil provider")
	}
	body, err := somarkRequestBody(payload, inject)
	if err != nil {
		return nil, err
	}
	fields, err := somarkFormFields(body, p.APIKey)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open document: %w", err)
	}
	defer file.Close()

	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("build multipart form: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("read document: %w", err)
	}
	for name, values := range fields {
		for _, value := range values {
			if err := writer.WriteField(name, value); err != nil {
				return nil, fmt.Errorf("build multipart form: %w", err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("build multipart form: %w", err)
	}

	endpoint := SomarkParseURL(p.BaseURL)
	req, err := http.NewRequest(http.MethodPost, endpoint, &form)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	SetAttribution(req, p.BaseURL)

	resp, err := httpClient(p, somarkTimeout).Do(req)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, somarkMaxResponse))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, truncateError(string(data)))
	}
	return decodeSomarkResponse(data)
}

// somarkRequestBody resolves the effective request body: --json verbatim,
// or the defaults, with --inject-json merged on top.
func somarkRequestBody(payload, inject json.RawMessage) (json.RawMessage, error) {
	body := payload
	if len(bytes.TrimSpace(body)) == 0 {
		body = json.RawMessage(defaultParsePayload)
	}
	if len(inject) == 0 {
		return body, nil
	}
	merged, err := jsonmerge.Inject(body, inject)
	if err != nil {
		return nil, fmt.Errorf("merge inject-json: %w", err)
	}
	return merged, nil
}

// somarkFormFields converts a request body into multipart form fields:
// scalars become one value, arrays repeat the field, and objects are sent as
// JSON strings (the API's element_formats / feature_config / extra shape).
func somarkFormFields(body json.RawMessage, apiKey string) (map[string][]string, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse request body (must be a JSON object): %w", err)
	}
	if apiKey == "" {
		apiKey = placeholderKey
	}
	fields := map[string][]string{"api_key": {apiKey}}
	for name, value := range payload {
		// The upload itself and the credential are CLI-owned.
		if name == "file" || name == "file_url" || name == "api_key" || value == nil {
			continue
		}
		values, err := somarkFieldValues(value)
		if err != nil {
			return nil, fmt.Errorf("form field %q: %w", name, err)
		}
		fields[name] = values
	}
	return fields, nil
}

// somarkFieldValues renders one JSON value as multipart field values.
func somarkFieldValues(value any) ([]string, error) {
	switch typed := value.(type) {
	case string:
		return []string{typed}, nil
	case bool:
		return []string{strconv.FormatBool(typed)}, nil
	case float64:
		return []string{strconv.FormatFloat(typed, 'f', -1, 64)}, nil
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			rendered, err := somarkFieldValues(item)
			if err != nil {
				return nil, err
			}
			values = append(values, rendered...)
		}
		return values, nil
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return []string{string(encoded)}, nil
	}
}

// somarkErrorHint turns a known API error code into an actionable hint.
func somarkErrorHint(code int) string {
	switch code {
	case 1107:
		return "check --api-key or providers.<name>.api_key"
	case 1133:
		return "the endpoint rejected a parameter; OXR does not support cross-page, title levels, zip output or Office files"
	default:
		return ""
	}
}

// somarkEnvelope mirrors the unified SoMark response body. code != 0 means
// the request failed while the HTTP status is still 200, and data is then an
// empty string rather than an object — so it is decoded lazily.
type somarkEnvelope struct {
	Code     int             `json:"code"`
	Message  string          `json:"message"`
	Warnings []string        `json:"warnings"`
	Data     json.RawMessage `json:"data"`
}

// somarkData is the successful `data` object of a parse response.
type somarkData struct {
	TaskID string `json:"task_id"`
	Error  any    `json:"error"`
	Meta   struct {
		PageNum          int    `json:"page_num"`
		FileType         string `json:"file_type"`
		ProcessingTimeMS int    `json:"processing_time_ms"`
	} `json:"metadata"`
	Result struct {
		FileName string          `json:"file_name"`
		Catalog  json.RawMessage `json:"catalog"`
		Outputs  struct {
			Markdown string          `json:"markdown"`
			JSON     json.RawMessage `json:"json"`
			Zip      string          `json:"zip"`
		} `json:"outputs"`
	} `json:"result"`
}

// decodeSomarkResponse converts a response body into a ParseResult.
func decodeSomarkResponse(data []byte) (*ParseResult, error) {
	var env somarkEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if env.Code != 0 {
		message := env.Message
		if hint := somarkErrorHint(env.Code); hint != "" {
			message = strings.TrimSpace(message + " (" + hint + ")")
		}
		if message != "" {
			return nil, fmt.Errorf("parse failed (code %d): %s", env.Code, message)
		}
		return nil, fmt.Errorf("parse failed (code %d)", env.Code)
	}
	var payload somarkData
	if len(env.Data) > 0 && string(env.Data) != "null" && string(env.Data) != `""` {
		if err := json.Unmarshal(env.Data, &payload); err != nil {
			return nil, fmt.Errorf("decode response data: %w", err)
		}
	}
	return &ParseResult{
		TaskID:           payload.TaskID,
		FileName:         payload.Result.FileName,
		PageNum:          payload.Meta.PageNum,
		FileType:         payload.Meta.FileType,
		ProcessingTimeMS: payload.Meta.ProcessingTimeMS,
		Markdown:         payload.Result.Outputs.Markdown,
		JSON:             payload.Result.Outputs.JSON,
		ZipURL:           payload.Result.Outputs.Zip,
		Catalog:          payload.Result.Catalog,
		Warnings:         env.Warnings,
	}, nil
}
