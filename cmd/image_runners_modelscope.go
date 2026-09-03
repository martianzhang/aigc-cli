package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/imgcodec"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// --- ModelScope async image generation ---

// modelScopeSubmitResponse is the response from ModelScope async image submission.
type modelScopeSubmitResponse struct {
	TaskID string `json:"task_id"`
}

// modelScopeTaskResponse is the response from ModelScope task polling.
type modelScopeTaskResponse struct {
	TaskStatus   string   `json:"task_status"`   // "SUCCEED", "FAILED", "PENDING", "RUNNING"
	OutputImages []string `json:"output_images"` // URLs to generated images
	ErrMsg       string   `json:"err_msg,omitempty"`
}

// runModelScopeImage handles image generation via ModelScope's async task API.
// Flow: submit → poll → download.
func runModelScopeImage(c client.APIClient, req *types.GenerateRequest, ctx *imageDispatchCtx) ([]string, error) {
	start := time.Now()

	// Strip version suffix from base URL for task polling endpoint.
	baseURL := c.BaseURL()
	if idx := strings.LastIndex(baseURL, "/v"); idx > strings.LastIndex(baseURL, "://") {
		baseURL = baseURL[:idx]
	}

	// --- Step 1: Submit async task ---
	submitURL := baseURL + "/v1/images/generations"
	body := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
	}
	if req.Size != "" {
		body["size"] = req.Size
	}

	bodyBytes, _ := json.Marshal(body)
	httpReq, err := http.NewRequest("POST", submitURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("modelscope: failed to create submit request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+ctx.modelScopeKey)
	httpReq.Header.Set("X-ModelScope-Async-Mode", "true")

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("modelscope: submit request failed: %w", err)
	}

	// Read quota headers before consuming body
	totalLimit := httpResp.Header.Get("Modelscope-Ratelimit-Requests-Limit")
	totalRemain := httpResp.Header.Get("Modelscope-Ratelimit-Requests-Remaining")
	modelLimit := httpResp.Header.Get("Modelscope-Ratelimit-Model-Requests-Limit")
	modelRemain := httpResp.Header.Get("Modelscope-Ratelimit-Model-Requests-Remaining")

	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("modelscope: submit returned HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	var submitResp modelScopeSubmitResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&submitResp); err != nil {
		return nil, fmt.Errorf("modelscope: failed to decode submit response: %w", err)
	}
	if submitResp.TaskID == "" {
		return nil, fmt.Errorf("modelscope: submit returned empty task_id")
	}

	taskID := submitResp.TaskID
	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Task ID: %s\n", taskID)
	if totalLimit != "" {
		fmt.Printf("Quota: %s/%s remaining (model: %s/%s)\n", totalRemain, totalLimit, modelRemain, modelLimit)
	}
	fmt.Println("Polling for completion...")

	// --- Step 2: Poll task status ---
	pollInterval := 3 * time.Second
	maxWait := 180 * time.Second
	deadline := time.Now().Add(maxWait)

	var taskResp modelScopeTaskResponse
	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)

		pollURL := fmt.Sprintf("%s/v1/tasks/%s", baseURL, taskID)
		pollReq, err := http.NewRequest("GET", pollURL, nil)
		if err != nil {
			return nil, fmt.Errorf("modelscope: failed to create poll request: %w", err)
		}
		pollReq.Header.Set("Authorization", "Bearer "+ctx.modelScopeKey)
		pollReq.Header.Set("X-ModelScope-Task-Type", "image_generation")

		pollResp, err := http.DefaultClient.Do(pollReq)
		if err != nil {
			return nil, fmt.Errorf("modelscope: poll request failed: %w", err)
		}

		var tmpResp modelScopeTaskResponse
		if err := json.NewDecoder(pollResp.Body).Decode(&tmpResp); err != nil {
			pollResp.Body.Close()
			return nil, fmt.Errorf("modelscope: failed to decode poll response: %w", err)
		}
		pollResp.Body.Close()

		switch tmpResp.TaskStatus {
		case "SUCCEED", "SUCCEEDED":
			taskResp = tmpResp
			goto done
		case "FAILED":
			errMsg := tmpResp.ErrMsg
			if errMsg == "" {
				errMsg = "unknown error"
			}
			return nil, fmt.Errorf("modelscope: task %s failed: %s", taskID, errMsg)
		case "PENDING", "RUNNING", "PROCESSING":
			continue
		case "CANCELED":
			return nil, fmt.Errorf("modelscope: task %s was canceled", taskID)
		case "UNKNOWN":
			return nil, fmt.Errorf("modelscope: task %s status unknown (may be expired)", taskID)
		default:
			return nil, fmt.Errorf("modelscope: unexpected task status %q for task %s", tmpResp.TaskStatus, taskID)
		}
	}

	return nil, fmt.Errorf("modelscope: task %s timed out after %.0fs", taskID, maxWait.Seconds())

done:
	// --- Step 3: Download images ---
	fmt.Printf("Duration: %.1fs\n", time.Since(start).Seconds())

	var saved []string
	for i, imgURL := range taskResp.OutputImages {
		data, err := service.FetchBytes(imgURL)
		if err != nil {
			service.SaveBase64Fallback(shared.OutputDir, fmt.Sprintf("modelscope_%d", time.Now().Unix()), imgURL, 0)
			continue
		}
		// Prefer sniffed real format over URL extension (URLs often lack
		// an extension or carry query params, causing wrong file types).
		ext := imgcodec.SniffImageExt(data)
		if ext == "" {
			ext = extractImageExt(imgURL)
		}
		if ext == "" {
			ext = ".png"
		}
		taskID := fmt.Sprintf("modelscope_%d", time.Now().Unix())
		filename := filepath.Join(shared.OutputDir, fmt.Sprintf("image_%s_%d%s", taskID, i, ext))
		if err := os.WriteFile(filename, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
			continue
		}
		fmt.Printf("Image %d: %s\n", i+1, filename)
		saved = append(saved, filename)
	}

	postProcessImages(saved)
	return saved, nil
}
