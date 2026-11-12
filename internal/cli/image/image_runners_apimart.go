package image

import (
	"encoding/json"
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/clientfactory"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

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
	ui.Printf("Model: %s\n", req.Model)
	ui.Printf("Response code: %d\n", resp.Code)
	ui.Printf("Task ID: %s\n", task.TaskID)
	ui.Printf("Status: %s\n\n", task.Status)

	ui.Println("Polling for completion...")
	taskData, err := c.PollTask(task.TaskID, clientfactory.TaskWait(types.MediaImage))
	if err != nil {
		return nil, fmt.Errorf("polling failed: %w", err)
	}

	if options.Shared.Verbose {
		prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
		ui.Printf("\nTask result:\n%s\n", string(prettyResult))
	}

	ui.Println()
	service.SavePromptBestEffort(options.Shared.SavePrompt, options.Shared.OutputDir, "image", taskData.ID, req.Prompt)

	ui.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)

	if taskData.Result == nil || len(taskData.Result.Images) == 0 {
		return nil, fmt.Errorf("task completed but returned no images")
	}
	saved, err := service.DownloadImages(taskData.Result.Images, options.Shared.OutputDir, taskData.ID)
	if err != nil {
		return nil, fmt.Errorf("image download failed: %w", err)
	}
	postProcessImages(saved)
	return saved, nil
}
