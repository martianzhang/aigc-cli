package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// somarkUsageTimeout bounds the quota lookup; /usage never parses a document.
const somarkUsageTimeout = 30 * time.Second

// SomarkUsage is the POST /usage response: the remaining page quota of the
// account the API key belongs to. SoMark bills one page per parsed page, and
// the free pools reset daily/monthly.
type SomarkUsage struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    SomarkUsageData `json:"-"`
}

// SomarkUsageData is the `data` object of a successful /usage response.
type SomarkUsageData struct {
	RemainingPaidPages          int    `json:"remaining_paid_pages"`
	RemainingFreePagesToday     int    `json:"remaining_free_pages_today"`
	RemainingFreePagesThisMonth int    `json:"remaining_free_pages_this_month"`
	DailyFreeLimit              int    `json:"daily_free_limit"`
	MonthlyFreeLimit            int    `json:"monthly_free_limit"`
	DashboardURL                string `json:"dashboard_url"`
}

// GetSomarkUsage queries POST {base}/usage, which reports the quota of the
// account the key belongs to (and doubles as a key-validity check). The key
// travels as a form field, matching the parse endpoints. The free OXR endpoint
// has no usage API, so callers there get a 404.
func GetSomarkUsage(p *EffectiveProvider) (*SomarkUsage, error) {
	if p == nil {
		return nil, fmt.Errorf("nil provider")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("api_key", p.APIKey); err != nil {
		return nil, fmt.Errorf("build usage request: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("build usage request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, somarkAPIURL(p.BaseURL, "/usage"), &body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	SetAttribution(req, p.BaseURL)

	resp, err := httpClient(p, somarkUsageTimeout).Do(req)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, truncateError(string(data)))
	}

	// Errors answer with code != 0 and data: "" rather than an object, so the
	// payload is decoded only after the code check.
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if envelope.Code != 0 {
		message := envelope.Message
		if hint := somarkErrorHint(envelope.Code); hint != "" {
			message = message + " (" + hint + ")"
		}
		return nil, fmt.Errorf("usage query failed (code %d): %s", envelope.Code, message)
	}

	usage := SomarkUsage{Code: envelope.Code, Message: envelope.Message}
	if len(envelope.Data) > 0 && string(envelope.Data) != `""` && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, &usage.Data); err != nil {
			return nil, fmt.Errorf("decode response data: %w", err)
		}
	}
	return &usage, nil
}
