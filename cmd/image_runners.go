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
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runOpenRouterDedicatedImage handles image generation via OpenRouter's
// dedicated Image API (POST /v1/images). Used for GPT Image, DALL-E, and
// most dedicated image models on OpenRouter. Returns standard OpenAI-compatible
// response with b64_json images.
func runOpenRouterDedicatedImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) ([]string, error) {
	start := time.Now()

	orResp, err := c.OpenRouterDedicatedImage(req)
	if err != nil {
		return nil, fmt.Errorf("OpenRouter image generation failed: %w", err)
	}

	elapsed := time.Since(start)

	fmt.Printf("Model: %s\n", req.Model)
	if orResp.Created > 0 {
		fmt.Printf("Created: %s\n", time.Unix(orResp.Created, 0).Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())

	var saved []string
	for i, img := range orResp.Data {
		if img.B64JSON != "" {
			prefix := fmt.Sprintf("image_%d", time.Now().Unix())
			filename, err := service.SaveBase64Image(shared.OutputDir, prefix, img.B64JSON, i)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d saved: %s\n", i+1, filename)
			saved = append(saved, filename)
		} else if img.URL != "" {
			taskID := fmt.Sprintf("%d", time.Now().Unix())
			filename, err := service.DownloadFile(img.URL, shared.OutputDir, fmt.Sprintf("image_%s_%d", taskID, i))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		}
		if img.RevisedPrompt != "" {
			fmt.Printf("  Revised prompt: %s\n", img.RevisedPrompt)
		}
	}

	postProcessImages(saved)
	printUsage(orResp.Usage)

	return saved, nil
}

// runSyncImage handles OpenAI/OpenRouter-compatible synchronous image generation.
func runSyncImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) ([]string, error) {
	start := time.Now()

	syncResp, err := c.ImageGenerateSync(req)
	if err != nil {
		return nil, fmt.Errorf("image generation failed: %w", err)
	}

	elapsed := time.Since(start)

	fmt.Printf("Model: %s\n", req.Model)
	if syncResp.Created > 0 {
		fmt.Printf("Created: %s\n", time.Unix(syncResp.Created, 0).Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())
	var saved []string
	for i, img := range syncResp.Data {
		if img.B64JSON != "" {
			taskID := fmt.Sprintf("image_sync_%d", syncResp.Created)
			filename, err := service.SaveBase64Image(shared.OutputDir, taskID, img.B64JSON, i)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		} else if img.URL != "" {
			taskID := fmt.Sprintf("sync_%d", syncResp.Created)
			filename, err := service.DownloadFile(img.URL, shared.OutputDir, fmt.Sprintf("image_%s_%d", taskID, i))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		} else {
			fmt.Printf("Image %d: <no data>\n", i+1)
			continue
		}
		if img.RevisedPrompt != "" {
			fmt.Printf("  Revised prompt: %s\n", img.RevisedPrompt)
		}
	}

	postProcessImages(saved)
	printUsage(syncResp.Usage)

	return saved, nil
}

// runAsyncImage handles APIMart-compatible asynchronous (task-based) image generation.
func runAsyncImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) ([]string, error) {
	resp, err := c.Submit(req)
	if err != nil {
		return nil, fmt.Errorf("submission failed: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("submission returned no tasks")
	}

	task := resp.Data[0]
	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Response code: %d\n", resp.Code)
	fmt.Printf("Task ID: %s\n", task.TaskID)
	fmt.Printf("Status: %s\n\n", task.Status)

	fmt.Println("Polling for completion...")
	taskData, err := c.PollTask(task.TaskID)
	if err != nil {
		return nil, fmt.Errorf("polling failed: %w", err)
	}

	if shared.Verbose {
		prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
		fmt.Printf("\nTask result:\n%s\n", string(prettyResult))
	}

	fmt.Println()
	savePromptFile(taskData.ID, req.Prompt)

	var saved []string
	if taskData.Result != nil && len(taskData.Result.Images) > 0 {
		saved, err = downloadImages(taskData.Result.Images, taskData.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
		} else {
			postProcessImages(saved)
		}
	}

	fmt.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)
	return saved, nil
}

// ollamaGenerateResponse is the response from Ollama's /api/generate for image models.
// Note: some models return "images" (array), others return "image" (single string).
type ollamaGenerateResponse struct {
	Model         string   `json:"model"`
	CreatedAt     string   `json:"created_at"`
	Response      string   `json:"response"`
	Done          bool     `json:"done"`
	DoneReason    string   `json:"done_reason"`
	Images        []string `json:"images,omitempty"`
	Image         string   `json:"image,omitempty"`
	TotalDuration int64    `json:"total_duration,omitempty"`
}

// ollamaGenerateImages sends a request to Ollama's /api/generate and returns saved filenames.
func ollamaGenerateImages(baseURL string, req *types.GenerateRequest) ([]string, error) {
	// Strip version suffix (e.g., /v1) to get raw Ollama endpoint.
	if idx := strings.LastIndex(baseURL, "/v"); idx > strings.LastIndex(baseURL, "://") {
		baseURL = baseURL[:idx]
	}
	url := baseURL + "/api/generate"
	body := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
		"stream": false,
	}

	bodyBytes, _ := json.Marshal(body)
	httpResp, err := http.Post(url, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("ollama returned HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaGenerateResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode ollama response: %w", err)
	}

	// Collect images: some models return "images" (array), others return "image" (single).
	var images []string
	if len(ollamaResp.Images) > 0 {
		images = ollamaResp.Images
	} else if ollamaResp.Image != "" {
		images = append(images, ollamaResp.Image)
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("ollama returned no images")
	}

	var saved []string
	for i, b64 := range images {
		prefix := fmt.Sprintf("image_ollama_%d", time.Now().Unix())
		filename, err := service.SaveBase64Image(shared.OutputDir, prefix, b64, i)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
			continue
		}
		saved = append(saved, filename)
	}
	return saved, nil
}

// runOllamaImage handles image generation via Ollama's native /api/generate endpoint.
func runOllamaImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) ([]string, error) {
	start := time.Now()
	saved, err := ollamaGenerateImages(c.BaseURL(), req)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Duration: %.1fs\n", time.Since(start).Seconds())
	for i, f := range saved {
		fmt.Printf("Image %d: %s\n", i+1, f)
	}
	postProcessImages(saved)
	return saved, nil
}

// runAgnesImage handles image generation via the Agnes API.
// Key differences from standard OpenAI:
//   - image URLs go in extra_body.image (not top-level image_urls)
//   - response_format must be in extra_body (handled in image.go transform)
//   - 2.1 Flash supports ratio for tiered sizing (e.g. "2K" + "16:9")
func runAgnesImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) ([]string, error) {
	// Transform ImageURLs into extra_body.image (Agnes requires it nested).
	if len(req.ImageURLs) > 0 {
		if req.ExtraBody == nil {
			req.ExtraBody = make(map[string]interface{})
		}
		req.ExtraBody["image"] = req.ImageURLs
		req.ImageURLs = nil // clear top-level, Agnes rejects it there
	}
	// Move ratio into extra_body for consistency with Agnes API structure.
	if req.Ratio != "" {
		if req.ExtraBody == nil {
			req.ExtraBody = make(map[string]interface{})
		}
		req.ExtraBody["ratio"] = req.Ratio
	}
	// Move response_format into extra_body (Agnes rejects it at top level).
	if req.ResponseFormat != "" {
		if req.ExtraBody == nil {
			req.ExtraBody = make(map[string]interface{})
		}
		req.ExtraBody["response_format"] = req.ResponseFormat
		req.ResponseFormat = ""
	}

	start := time.Now()
	syncResp, err := c.ImageGenerateSync(req)
	if err != nil {
		return nil, fmt.Errorf("agnes image generation failed: %w", err)
	}
	elapsed := time.Since(start)

	fmt.Printf("Model: %s\n", req.Model)
	if syncResp.Created > 0 {
		fmt.Printf("Created: %s\n", time.Unix(syncResp.Created, 0).Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())

	var saved []string
	for i, img := range syncResp.Data {
		if img.B64JSON != "" {
			taskID := fmt.Sprintf("agnes_%d", syncResp.Created)
			filename, err := service.SaveBase64Image(shared.OutputDir, taskID, img.B64JSON, i)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		} else if img.URL != "" {
			taskID := fmt.Sprintf("agnes_%d", syncResp.Created)
			filename, err := service.DownloadFile(img.URL, shared.OutputDir, fmt.Sprintf("image_%s_%d", taskID, i))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		} else {
			fmt.Printf("Image %d: <no data>\n", i+1)
			continue
		}
		if img.RevisedPrompt != "" {
			fmt.Printf("  Revised prompt: %s\n", img.RevisedPrompt)
		}
	}

	postProcessImages(saved)
	printUsage(syncResp.Usage)
	return saved, nil
}

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
		ext := ".png"
		if e := filepath.Ext(imgURL); e != "" {
			ext = e
		}
		taskID := fmt.Sprintf("modelscope_%d", time.Now().Unix())
		filename := filepath.Join(shared.OutputDir, fmt.Sprintf("image_%s_%d%s", taskID, i, ext))
		data, err := service.FetchBytes(imgURL)
		if err != nil {
			service.SaveBase64Fallback(shared.OutputDir, taskID, imgURL, 0)
			continue
		}
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

// downloadImages downloads all generated images to the output directory.
// Returns paths to saved files.
func downloadImages(images []types.ImageResult, taskID string) ([]string, error) {
	var saved []string
	for i, img := range images {
		for j, url := range img.URL {
			data, err := service.FetchBytes(url)
			if err != nil {
				// Save raw data as text file for manual recovery
				prefix := fmt.Sprintf("image_%s_%d_%d", taskID, i, j)
				service.SaveBase64Fallback(shared.OutputDir, prefix, url, 0)
				continue
			}

			ext := filepath.Ext(url)
			if ext == "" {
				ext = ".png"
			}
			filename := filepath.Join(shared.OutputDir, fmt.Sprintf("image_%s_%d_%d%s", taskID, i, j, ext))
			if err := os.WriteFile(filename, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
				continue
			}
			fmt.Printf("Saved: %s\n", filename)
			saved = append(saved, filename)
		}
	}
	return saved, nil
}

// runGeminiImage handles image generation via Gemini native generateContent API.
func runGeminiImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) ([]string, error) {
	start := time.Now()

	geminiResp, err := c.GeminiImageGenerate(req)
	if err != nil {
		return nil, fmt.Errorf("Gemini image generation failed: %w", err)
	}

	elapsed := time.Since(start)

	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())

	var saved []string
	for i, img := range geminiResp.Data {
		if strings.HasPrefix(img.URL, "data:") {
			prefix := fmt.Sprintf("image_%d", time.Now().Unix())
			filename, err := service.SaveBase64Image(shared.OutputDir, prefix, img.URL, i)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d saved: %s\n", i+1, filename)
			saved = append(saved, filename)
		} else if img.URL != "" {
			data, err := service.FetchBytes(img.URL)
			if err != nil {
				service.SaveBase64Fallback(shared.OutputDir, fmt.Sprintf("image_%d", time.Now().Unix()), img.URL, 0)
				continue
			}
			ext := filepath.Ext(img.URL)
			if ext == "" {
				ext = ".png"
			}
			filename := filepath.Join(shared.OutputDir, fmt.Sprintf("image_%d_%d%s", time.Now().Unix(), i, ext))
			if err := os.WriteFile(filename, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
				continue
			}
			fmt.Printf("Image %d saved: %s\n", i+1, filename)
			saved = append(saved, filename)
		}
	}

	postProcessImages(saved)
	return saved, nil
}
