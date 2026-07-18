package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
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
		if img.B64JSON != "" {
			prefix := fmt.Sprintf("image_%d", time.Now().Unix())
			filename, err := service.SaveBase64Image(shared.OutputDir, prefix, img.B64JSON, i)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d saved: %s\n", i+1, filename)
		} else if img.URL != "" {
			taskID := fmt.Sprintf("%d", time.Now().Unix())
			filename, err := service.DownloadFile(img.URL, shared.OutputDir, fmt.Sprintf("image_%s_%d", taskID, i))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
		}
		if img.RevisedPrompt != "" {
			fmt.Printf("  Revised prompt: %s\n", img.RevisedPrompt)
		}
	}

	printUsage(orResp.Usage)

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
		if img.B64JSON != "" {
			taskID := fmt.Sprintf("image_sync_%d", syncResp.Created)
			filename, err := service.SaveBase64Image(shared.OutputDir, taskID, img.B64JSON, i)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
		} else if img.URL != "" {
			taskID := fmt.Sprintf("sync_%d", syncResp.Created)
			filename, err := service.DownloadFile(img.URL, shared.OutputDir, fmt.Sprintf("image_%s_%d", taskID, i))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i, err)
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

	printUsage(syncResp.Usage)

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
