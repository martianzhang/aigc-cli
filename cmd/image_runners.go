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
func runOpenRouterDedicatedImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) error {
	start := time.Now()

	orResp, err := c.OpenRouterDedicatedImage(req)
	if err != nil {
		return fmt.Errorf("OpenRouter image generation failed: %w", err)
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

	return nil
}

// runSyncImage handles OpenAI/OpenRouter-compatible synchronous image generation.
func runSyncImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) error {
	start := time.Now()

	syncResp, err := c.ImageGenerateSync(req)
	if err != nil {
		return fmt.Errorf("image generation failed: %w", err)
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

	return nil
}

// runAsyncImage handles APIMart-compatible asynchronous (task-based) image generation.
func runAsyncImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) error {
	resp, err := c.Submit(req)
	if err != nil {
		return fmt.Errorf("submission failed: %w", err)
	}

	if len(resp.Data) == 0 {
		return fmt.Errorf("submission returned no tasks")
	}

	task := resp.Data[0]
	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Response code: %d\n", resp.Code)
	fmt.Printf("Task ID: %s\n", task.TaskID)
	fmt.Printf("Status: %s\n\n", task.Status)

	fmt.Println("Polling for completion...")
	taskData, err := c.PollTask(task.TaskID)
	if err != nil {
		return fmt.Errorf("polling failed: %w", err)
	}

	if shared.Verbose {
		prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
		fmt.Printf("\nTask result:\n%s\n", string(prettyResult))
	}

	fmt.Println()
	savePromptFile(taskData.ID, req.Prompt)

	if taskData.Result != nil && len(taskData.Result.Images) > 0 {
		if saved, err := downloadImages(taskData.Result.Images, taskData.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
		} else {
			postProcessImages(saved)
		}
	}

	fmt.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)
	return nil
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

// runOllamaImage handles image generation via Ollama's native /api/generate endpoint.
// Ollama image models (x/flux2-klein, x/z-image-turbo, etc.) don't support the
// OpenAI-compatible /v1/images/generations endpoint — they use the native API instead.
func runOllamaImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) error {
	start := time.Now()

	// Strip version suffix (e.g., /v1) from client baseURL to get raw Ollama endpoint.
	baseURL := c.BaseURL()
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
		return fmt.Errorf("ollama request failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ollama returned HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaGenerateResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&ollamaResp); err != nil {
		return fmt.Errorf("failed to decode ollama response: %w", err)
	}

	elapsed := time.Since(start)

	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())

	// Collect images: some models return "images" (array), others return "image" (single).
	var images []string
	if len(ollamaResp.Images) > 0 {
		images = ollamaResp.Images
	} else if ollamaResp.Image != "" {
		images = append(images, ollamaResp.Image)
	}
	if len(images) == 0 {
		return fmt.Errorf("ollama returned no images (response: %s)", ollamaResp.Response)
	}

	var saved []string
	for i, b64 := range images {
		prefix := fmt.Sprintf("image_ollama_%d", time.Now().Unix())
		filename, err := service.SaveBase64Image(shared.OutputDir, prefix, b64, i)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
			continue
		}
		fmt.Printf("Image %d: %s\n", i+1, filename)
		saved = append(saved, filename)
	}

	postProcessImages(saved)
	return nil
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
