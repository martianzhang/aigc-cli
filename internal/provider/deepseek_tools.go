package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// DeepSeek tool APIs (announced 2026-11-13) share the /v1/tools prefix and are
// not OpenAI-compatible: OCR and ASR take multipart/form-data, TTS returns raw
// audio bytes. These endpoints are routed by provider type, never by model.
const (
	// DeepSeekBaseURL is the DeepSeek API root. The OpenAI-compatible chat
	// path and every tool endpoint hang off it.
	DeepSeekBaseURL = "https://api.deepseek.com"

	// OCR purposes accepted by POST /v1/tools/ocr.
	DeepSeekOCRTextRecognition = "text_recognition"
	DeepSeekOCRRichLayout      = "rich_layout_recognition"

	// DeepSeekDefaultVoice is the TTS default when none is configured.
	DeepSeekDefaultVoice = "stella"

	// deepSeekMaxTextBytes is the TTS request body limit (64 KiB).
	deepSeekMaxTextBytes = 64 << 10
	// deepSeekMaxAudioBytes bounds a 30-minute WAV response (~86 MB at 24 kHz).
	deepSeekMaxAudioBytes = 128 << 20
	// deepSeekMaxASRBytes is the ASR upload cap enforced by the API.
	deepSeekMaxASRBytes = 8 << 20
	// deepSeekMaxJSONBytes bounds OCR/ASR JSON bodies.
	deepSeekMaxJSONBytes = 64 << 20

	// Server-side caps: OCR 180s total, ASR 10min upload, TTS up to 30 minutes of
	// audio. A caller-configured timeout still wins.
	deepSeekOCRTimeout = 210 * time.Second
	deepSeekASRTimeout = 11 * time.Minute
	deepSeekTTSTimeout = 30 * time.Minute
)

// DeepSeekOCRPage is one page of an OCR result.
type DeepSeekOCRPage struct {
	Index        int    `json:"index"`
	Text         string `json:"text"`
	FinishReason string `json:"finish_reason"`
}

// DeepSeekOCRUsage is the token usage reported by the OCR API (informational;
// billing is per recognised page).
type DeepSeekOCRUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// DeepSeekOCRResult is a successful /v1/tools/ocr response.
type DeepSeekOCRResult struct {
	ID        string            `json:"id"`
	Object    string            `json:"object"`
	Created   int64             `json:"created"`
	Purpose   string            `json:"purpose"`
	PageCount int               `json:"page_count"`
	Pages     []DeepSeekOCRPage `json:"pages"`
	Usage     DeepSeekOCRUsage  `json:"usage"`
}

// Text joins the per-page text in page order. Page-separated OCR output keeps
// its model-emitted <|ref|>/<|box|> markers, so callers can locate regions.
func (r *DeepSeekOCRResult) Text() string {
	if r == nil {
		return ""
	}
	parts := make([]string, 0, len(r.Pages))
	for _, page := range r.Pages {
		if text := strings.TrimSpace(page.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

// DeepSeekTranscription is a successful /v1/tools/asr response.
type DeepSeekTranscription struct {
	RequestID  string `json:"request_id"`
	Text       string `json:"text"`
	DurationMS int    `json:"duration_ms"`
}

// IsDeepSeekProvider reports whether p must use DeepSeek's dedicated tool
// endpoints instead of the OpenAI-compatible audio/vision paths. The detected
// type is also honoured so an explicit base_url with type: openai works.
func IsDeepSeekProvider(p *EffectiveProvider) bool {
	if p == nil {
		return false
	}
	return p.Type == types.ProviderDeepSeek || p.ProviderType == DeepSeek
}

// BuiltinDeepSeekProvider returns the built-in "deepseek" alias used when the
// user selects it with --provider without defining a providers entry. The API
// key comes from DEEPSEEK_API_KEY; callers may still fill it from config.
func BuiltinDeepSeekProvider() *EffectiveProvider {
	return &EffectiveProvider{
		Name:         "deepseek",
		Type:         types.ProviderDeepSeek,
		BaseURL:      DeepSeekBaseURL,
		APIKey:       os.Getenv("DEEPSEEK_API_KEY"),
		ProviderType: DeepSeek,
	}
}

// DeepSeekToolURL resolves one tool endpoint against a base URL. Both
// https://api.deepseek.com and an explicit .../v1 base are accepted, as are
// relays that mount the API under a custom prefix.
func DeepSeekToolURL(baseURL, tool string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = DeepSeekBaseURL
	}
	if !hasVersionSuffix(base) {
		base += "/v1"
	}
	return base + "/tools/" + strings.Trim(tool, "/")
}

// NormalizeDeepSeekOCRPurpose validates and canonicalises a --purpose value.
func NormalizeDeepSeekOCRPurpose(purpose string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(purpose)) {
	case "", DeepSeekOCRTextRecognition:
		return DeepSeekOCRTextRecognition, nil
	case DeepSeekOCRRichLayout:
		return DeepSeekOCRRichLayout, nil
	default:
		return "", fmt.Errorf("unsupported OCR purpose %q: use %s or %s",
			purpose, DeepSeekOCRTextRecognition, DeepSeekOCRRichLayout)
	}
}

// DeepSeekOCR recognises text in a PDF, Word/PowerPoint document, or image via
// POST /v1/tools/ocr. The server does layout analysis; purpose selects plain
// text extraction or per-page rich layout recognition.
func DeepSeekOCR(p *EffectiveProvider, filePath, purpose string) (*DeepSeekOCRResult, error) {
	if p == nil {
		return nil, fmt.Errorf("nil provider")
	}
	purpose, err := NormalizeDeepSeekOCRPurpose(purpose)
	if err != nil {
		return nil, err
	}

	body, contentType, err := deepSeekMultipart(filePath, purpose)
	if err != nil {
		return nil, err
	}
	data, err := deepSeekPost(p, "ocr", contentType, body.Bytes(), deepSeekOCRTimeout, deepSeekMaxJSONBytes, true)
	if err != nil {
		return nil, err
	}

	var result DeepSeekOCRResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode OCR response: %w", err)
	}
	return &result, nil
}

// DeepSeekTranscribe transcribes one audio file (16-bit PCM WAV or Ogg Opus)
// via POST /v1/tools/asr. Language is detected server-side; there is no model
// or language parameter.
func DeepSeekTranscribe(p *EffectiveProvider, filePath string) (*DeepSeekTranscription, error) {
	if p == nil {
		return nil, fmt.Errorf("nil provider")
	}
	if err := validateDeepSeekAudioFile(filePath); err != nil {
		return nil, err
	}
	body, contentType, err := deepSeekMultipart(filePath, "")
	if err != nil {
		return nil, err
	}
	data, err := deepSeekPost(p, "asr", contentType, body.Bytes(), deepSeekASRTimeout, deepSeekMaxJSONBytes, true)
	if err != nil {
		return nil, err
	}

	var result DeepSeekTranscription
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode ASR response: %w", err)
	}
	return &result, nil
}

// DeepSeekTTS synthesises speech via POST /v1/tools/tts and returns the raw
// audio bytes plus the response Content-Type. voiceID is stella or tide;
// format is wav or pcm. filterMarkdown nil means "omit" (server default true).
func DeepSeekTTS(p *EffectiveProvider, text, voiceID, format string, filterMarkdown *bool) ([]byte, string, error) {
	if p == nil {
		return nil, "", fmt.Errorf("nil provider")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, "", fmt.Errorf("text is required for DeepSeek TTS")
	}
	if len(text) > deepSeekMaxTextBytes {
		return nil, "", fmt.Errorf("DeepSeek TTS accepts at most 64 KiB of text per request (got %d bytes)", len(text))
	}

	voiceID = strings.ToLower(strings.TrimSpace(voiceID))
	if voiceID == "" {
		voiceID = DeepSeekDefaultVoice
	}
	if voiceID != "stella" && voiceID != "tide" {
		return nil, "", fmt.Errorf("unsupported DeepSeek voice %q: use stella (female) or tide (male)", voiceID)
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "wav"
	}
	if format != "wav" && format != "pcm" {
		return nil, "", fmt.Errorf("unsupported DeepSeek TTS format %q: use wav or pcm", format)
	}

	payload := map[string]any{
		"text":     text,
		"voice_id": voiceID,
		"format":   format,
	}
	if filterMarkdown != nil {
		payload["filter_markdown"] = *filterMarkdown
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("encode TTS request: %w", err)
	}

	data, err := deepSeekPost(p, "tts", "application/json", body, deepSeekTTSTimeout, deepSeekMaxAudioBytes, false)
	if err != nil {
		return nil, "", err
	}
	return data, "audio/" + format, nil
}

// validateDeepSeekAudioFile rejects formats and sizes the ASR endpoint cannot
// accept, before any bytes are uploaded.
func validateDeepSeekAudioFile(filePath string) error {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".wav", ".ogg", ".oga", ".opus":
	default:
		return fmt.Errorf("DeepSeek ASR accepts only 16-bit PCM WAV or Ogg Opus (got %q)", filepath.Ext(filePath))
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("read audio file: %w", err)
	}
	if info.Size() > deepSeekMaxASRBytes {
		return fmt.Errorf("audio file is %d MiB; DeepSeek ASR accepts at most 8 MiB", (info.Size()+(1<<20)-1)>>20)
	}
	return nil
}

// deepSeekMultipart builds a multipart body with an optional purpose field
// followed by the file part. The field order matters: the Files API rejects a
// request whose purpose comes after the file, so keep purpose first here too.
func deepSeekMultipart(filePath, purpose string) (*bytes.Buffer, string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if purpose != "" {
		if err := writer.WriteField("purpose", purpose); err != nil {
			return nil, "", fmt.Errorf("build multipart form: %w", err)
		}
	}
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, "", fmt.Errorf("build multipart form: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, "", fmt.Errorf("read file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("build multipart form: %w", err)
	}
	return &buf, writer.FormDataContentType(), nil
}

// deepSeekPost sends one tool request and returns the response body. Non-2xx
// statuses are surfaced as errors with the server message. checkBodyError also
// rejects in-body error objects, which OCR can return after a 200 has been
// sent; it is disabled for binary TTS responses.
func deepSeekPost(p *EffectiveProvider, tool, contentType string, body []byte, timeout time.Duration, limit int64, checkBodyError bool) ([]byte, error) {
	endpoint := DeepSeekToolURL(p.BaseURL, tool)
	req, err := http.NewRequestWithContext(Context(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	SetAttribution(req, p.BaseURL)

	resp, err := httpClient(p, requestTimeout(p, timeout)).Do(req)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds the %d-byte limit", limit)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, truncateError(string(data)))
	}
	if checkBodyError {
		if message := deepSeekErrorMessage(data); message != "" {
			return nil, fmt.Errorf("API error: %s", message)
		}
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty response from DeepSeek %s endpoint", tool)
	}
	return data, nil
}

// deepSeekErrorBody is the error object DeepSeek may return with HTTP 200.
type deepSeekErrorBody struct {
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Param   any    `json:"param"`
		Code    any    `json:"code"`
	} `json:"error"`
}

// deepSeekErrorMessage extracts an in-body error message, if any.
func deepSeekErrorMessage(data []byte) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return ""
	}
	var body deepSeekErrorBody
	if err := json.Unmarshal(trimmed, &body); err != nil || body.Error == nil {
		return ""
	}
	message := strings.TrimSpace(body.Error.Message)
	if message == "" {
		message = strings.TrimSpace(body.Error.Type)
	}
	if message == "" {
		message = "request failed"
	}
	return truncateError(message)
}
