package image

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/httpx"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/clientfactory"
	"github.com/martianzhang/aigc-cli/internal/fsutil"
	"github.com/martianzhang/aigc-cli/internal/imgcodec"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
	"github.com/martianzhang/aigc-cli/internal/wait"
)

// --- ModelScope async image generation ---

// modelScopeSubmitResponse is the response from ModelScope async image submission.
type modelScopeSubmitResponse struct {
	TaskID string `json:"task_id"`
}

// modelScopeTaskResponse is the response from ModelScope task polling.
type modelScopeTaskResponse struct {
	TaskStatus   string            `json:"task_status"`   // "SUCCEED", "FAILED", "PENDING", "RUNNING"
	OutputImages []string          `json:"output_images"` // URLs to generated images
	ErrMsg       string            `json:"err_msg,omitempty"`
	Errors       *modelScopeErrors `json:"errors,omitempty"`
}

// modelScopeErrors is the structured failure detail ModelScope actually returns
// (errors.message), where the legacy flat err_msg is often empty.
type modelScopeErrors struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// errorMessage prefers the structured errors.message over the legacy err_msg.
func (r *modelScopeTaskResponse) errorMessage() string {
	if r.Errors != nil && r.Errors.Message != "" {
		return r.Errors.Message
	}
	if r.ErrMsg != "" {
		return r.ErrMsg
	}
	return "unknown error"
}

// buildModelScopeImageBody forwards a verbatim --json body as-is, otherwise
// builds from typed fields, embedding local images as data URIs (no upload endpoint).
func buildModelScopeImageBody(req *types.GenerateRequest) (map[string]any, error) {
	if len(req.RawJSON) > 0 {
		body := map[string]any{}
		if err := json.Unmarshal(req.RawJSON, &body); err != nil {
			return nil, fmt.Errorf("failed to parse json body: %w", err)
		}
		return body, nil
	}
	body := map[string]any{
		"model":  req.Model,
		"prompt": req.Prompt,
	}
	if req.Size != "" {
		body["size"] = req.Size
	}
	if req.N != nil {
		body["n"] = *req.N
	}
	if len(req.ImageURLs) == 0 {
		return body, nil
	}
	resolved, err := service.LocalFilesToDataURI(req.ImageURLs)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve image-urls: %w", err)
	}
	if len(resolved) == 1 {
		body["image_url"] = resolved[0]
	} else {
		body["image_url"] = resolved
	}
	return body, nil
}

// runModelScopeImage handles image generation via ModelScope's async task API.
// Flow: submit → poll → download.
func runModelScopeImage(c client.APIClient, req *types.GenerateRequest, ctx *imageDispatchCtx) ([]string, error) {
	start := time.Now()

	// Attribute errors to the provider that was actually selected. A bare ctx
	// (unit tests) keeps the historical runner label.
	label := "modelscope"
	apiKey := ""
	if ctx != nil {
		apiKey = ctx.modelScopeKey
	}
	if ctx != nil && ctx.p != nil {
		label = providerLabel(ctx.p)
		if ctx.p.APIKey != "" {
			apiKey = ctx.p.APIKey
		}
	}

	// ModelScope bypasses client.Client, so build the request client here with
	// the effective --timeout value. A nil Transport inherits
	// http.DefaultTransport, which carries the configured proxy.
	hc := &http.Client{Timeout: clientfactory.HTTPTimeout("image", client.ImageTimeout)}

	// Strip version suffix from base URL for task polling endpoint.
	baseURL := c.BaseURL()
	if idx := strings.LastIndex(baseURL, "/v"); idx > strings.LastIndex(baseURL, "://") {
		baseURL = baseURL[:idx]
	}

	// --- Step 1: Submit async task ---
	submitURL := baseURL + "/v1/images/generations"
	body, err := buildModelScopeImageBody(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	injected, err := types.MaybeInject(body, req.InjectJSON)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}

	bodyBytes, err := json.Marshal(injected)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to marshal request: %w", label, err)
	}
	httpReq, err := http.NewRequestWithContext(options.Context(), http.MethodPost, submitURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: failed to create submit request: %w", label, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("X-ModelScope-Async-Mode", "true")

	httpResp, err := hc.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: submit request failed: %w", label, err)
	}

	// Read quota headers before consuming body
	totalLimit := httpResp.Header.Get("Modelscope-Ratelimit-Requests-Limit")
	totalRemain := httpResp.Header.Get("Modelscope-Ratelimit-Requests-Remaining")
	modelLimit := httpResp.Header.Get("Modelscope-Ratelimit-Model-Requests-Limit")
	modelRemain := httpResp.Header.Get("Modelscope-Ratelimit-Model-Requests-Remaining")

	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := httpx.ReadAllLimited(httpResp.Body, httpx.DefaultMaxBodyBytes)
		return nil, fmt.Errorf("%s: submit returned HTTP %d: %s", label, httpResp.StatusCode, string(respBody))
	}

	var submitResp modelScopeSubmitResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&submitResp); err != nil {
		return nil, fmt.Errorf("%s: failed to decode submit response: %w", label, err)
	}
	if submitResp.TaskID == "" {
		return nil, fmt.Errorf("%s: submit returned empty task_id", label)
	}

	taskID := submitResp.TaskID
	ui.Printf("Model: %s\n", req.Model)
	ui.Printf("Task ID: %s\n", taskID)
	if totalLimit != "" {
		ui.Printf("Quota: %s/%s remaining (model: %s/%s)\n", totalRemain, totalLimit, modelRemain, modelLimit)
	}
	ui.Println("Polling for completion...")

	// --- Step 2: Poll task status ---
	pollInterval := client.DefaultPollInterval
	// ModelScope queues free-tier jobs, so the default wait is per-media
	// (image 10m) and overridable via defaults.image.timeout.
	maxWait := clientfactory.TaskWait(types.MediaImage)
	deadline := time.Now().Add(maxWait)

	var taskResp modelScopeTaskResponse
	for time.Now().Before(deadline) {
		if err := wait.Sleep(options.Context(), pollInterval); err != nil {
			return nil, err
		}

		pollURL := fmt.Sprintf("%s/v1/tasks/%s", baseURL, taskID)
		pollReq, err := http.NewRequestWithContext(options.Context(), http.MethodGet, pollURL, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to create poll request: %w", label, err)
		}
		pollReq.Header.Set("Authorization", "Bearer "+apiKey)
		pollReq.Header.Set("X-ModelScope-Task-Type", "image_generation")

		pollResp, err := hc.Do(pollReq)
		if err != nil {
			return nil, fmt.Errorf("%s: poll request failed: %w", label, err)
		}

		var tmpResp modelScopeTaskResponse
		if err := json.NewDecoder(pollResp.Body).Decode(&tmpResp); err != nil {
			pollResp.Body.Close()
			return nil, fmt.Errorf("%s: failed to decode poll response: %w", label, err)
		}
		pollResp.Body.Close()

		switch tmpResp.TaskStatus {
		case "SUCCEED", "SUCCEEDED":
			taskResp = tmpResp
			goto done
		case "FAILED":
			return nil, fmt.Errorf("%s: task %s failed: %s", label, taskID, tmpResp.errorMessage())
		case "PENDING", "RUNNING", "PROCESSING":
			continue
		case "CANCELED":
			return nil, fmt.Errorf("%s: task %s was canceled", label, taskID)
		case "UNKNOWN":
			return nil, fmt.Errorf("%s: task %s status unknown (may be expired)", label, taskID)
		default:
			return nil, fmt.Errorf("%s: unexpected task status %q for task %s", label, tmpResp.TaskStatus, taskID)
		}
	}

	return nil, fmt.Errorf("%s: task %s timed out after %.0fs", label, taskID, maxWait.Seconds())

done:
	// --- Step 3: Download images ---
	ui.Printf("Duration: %.1fs\n", time.Since(start).Seconds())

	service.SavePromptBestEffort(options.Shared.SavePrompt, options.Shared.OutputDir, "image", promptTaskID("modelscope", 0), req.Prompt)

	var saved []string
	for i, imgURL := range taskResp.OutputImages {
		data, err := service.FetchBytes(imgURL)
		if err != nil {
			service.SaveBase64Fallback(options.Shared.OutputDir, fmt.Sprintf("modelscope_%d", time.Now().Unix()), imgURL, 0)
			continue
		}
		// Prefer sniffed real format over URL extension (URLs often lack
		// an extension or carry query params, causing wrong file types).
		ext := imgcodec.SniffImageExt(data)
		if ext == "" {
			ext = service.ExtractImageExt(imgURL)
		}
		if ext == "" {
			ext = ".png"
		}
		taskID := fmt.Sprintf("modelscope_%d", time.Now().Unix())
		filename := filepath.Join(options.Shared.OutputDir, fmt.Sprintf("image_%s_%d%s", taskID, i, ext))
		if err := fsutil.WriteFileAtomic(filename, data, 0644); err != nil {
			ui.Eprintf("Warning: failed to save %s: %v\n", filename, err)
			continue
		}
		ui.Printf("Image %d: %s\n", i+1, filename)
		saved = append(saved, filename)
	}

	if len(saved) == 0 {
		return nil, fmt.Errorf("modelscope returned no images to save")
	}
	if req.N != nil && *req.N > len(saved) {
		ui.Eprintf("Warning: requested %d images, provider returned %d (the model may cap n)\n", *req.N, len(saved))
	}
	postProcessImages(saved)
	return saved, nil
}
