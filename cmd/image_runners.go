package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/service"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// runOpenRouterDedicatedImage handles image generation via OpenRouter's
// dedicated Image API (POST /v1/images). Used for GPT Image, DALL-E, and
// most dedicated image models on OpenRouter. Returns standard OpenAI-compatible
// response with b64_json images.
func runOpenRouterDedicatedImage(c client.APIClient, req *types.GenerateRequest) error {
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

	for i, img := range orResp.Data {
		// Save base64 image
		if img.B64JSON != "" {
			prefix := fmt.Sprintf("image_%d", time.Now().Unix())
			filename, err := service.SaveBase64Image(shared.OutputDir, prefix, img.B64JSON, i)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d saved: %s\n", i+1, filename)
		} else if img.URL != "" {
			// Download from URL
			body, err := httpGet(img.URL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i, err)
				continue
			}
			ext := filepath.Ext(img.URL)
			if ext == "" {
				ext = ".png"
			}
			ts := time.Now().Unix()
			filename := filepath.Join(shared.OutputDir, fmt.Sprintf("image_%d_%d%s", ts, i, ext))
			if err := os.WriteFile(filename, body, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, img.URL)
			fmt.Printf("Saved: %s\n", filename)
		}
		if img.RevisedPrompt != "" {
			fmt.Printf("  Revised prompt: %s\n", img.RevisedPrompt)
		}
	}

	// Show usage / cost
	if orResp.Usage != nil {
		parts := []string{}
		if orResp.Usage.PromptTokens > 0 {
			parts = append(parts, fmt.Sprintf("%d in", orResp.Usage.PromptTokens))
		}
		if orResp.Usage.CompletionTokens > 0 {
			parts = append(parts, fmt.Sprintf("%d out", orResp.Usage.CompletionTokens))
		}
		if orResp.Usage.TotalTokens > 0 {
			parts = append(parts, fmt.Sprintf("%d total", orResp.Usage.TotalTokens))
		}
		tokenStr := ""
		if len(parts) > 0 {
			tokenStr = strings.Join(parts, " / ")
		}
		if tokenStr != "" || orResp.Usage.Cost > 0 {
			if tokenStr != "" {
				fmt.Printf("Tokens: %s", tokenStr)
			}
			if orResp.Usage.Cost > 0 {
				if tokenStr != "" {
					fmt.Printf(" | ")
				}
				fmt.Printf("Cost: $%.5f", orResp.Usage.Cost)
			}
			fmt.Println()
		}
	}

	return nil
}

// runSyncImage handles OpenAI/OpenRouter-compatible synchronous image generation.
func runSyncImage(c client.APIClient, req *types.GenerateRequest) error {
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
	for i, img := range syncResp.Data {
		// Save base64 image data
		if img.B64JSON != "" {
			taskID := fmt.Sprintf("image_sync_%d", syncResp.Created)
			filename, err := service.SaveBase64Image(shared.OutputDir, taskID, img.B64JSON, i)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
		} else if img.URL != "" {
			// Download from URL
			body, err := httpGet(img.URL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i, err)
				continue
			}
			ext := filepath.Ext(img.URL)
			if ext == "" {
				ext = ".png"
			}
			taskID := fmt.Sprintf("sync_%d", syncResp.Created)
			filename := filepath.Join(shared.OutputDir, fmt.Sprintf("image_%s_%d%s", taskID, i, ext))
			if err := os.WriteFile(filename, body, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
		} else {
			fmt.Printf("Image %d: <no data>\n", i+1)
			continue
		}
		if img.RevisedPrompt != "" {
			fmt.Printf("  Revised prompt: %s\n", img.RevisedPrompt)
		}
	}

	if syncResp.Usage != nil {
		parts := []string{}
		if syncResp.Usage.PromptTokens > 0 {
			parts = append(parts, fmt.Sprintf("%d in", syncResp.Usage.PromptTokens))
		}
		if syncResp.Usage.CompletionTokens > 0 {
			parts = append(parts, fmt.Sprintf("%d out", syncResp.Usage.CompletionTokens))
		}
		if syncResp.Usage.TotalTokens > 0 {
			parts = append(parts, fmt.Sprintf("%d total", syncResp.Usage.TotalTokens))
		}
		tokenStr := ""
		if len(parts) > 0 {
			tokenStr = strings.Join(parts, " / ")
		}
		if tokenStr != "" || syncResp.Usage.Cost > 0 {
			if tokenStr != "" {
				fmt.Printf("Tokens: %s", tokenStr)
			}
			if syncResp.Usage.Cost > 0 {
				if tokenStr != "" {
					fmt.Printf(" | ")
				}
				fmt.Printf("Cost: $%.5f", syncResp.Usage.Cost)
			}
			fmt.Println()
		}
	}

	return nil
}

// runAsyncImage handles APIMart-compatible asynchronous (task-based) image generation.
func runAsyncImage(c client.APIClient, req *types.GenerateRequest) error {
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
		if _, err := downloadImages(taskData.Result.Images, taskData.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
		}
	}

	fmt.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)
	return nil
}

// downloadImages downloads all generated images to the output directory.
// Returns paths to saved files.
func downloadImages(images []types.ImageResult, taskID string) ([]string, error) {
	var saved []string
	for i, img := range images {
		for j, url := range img.URL {
			data, err := service.FetchImage(url)
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
