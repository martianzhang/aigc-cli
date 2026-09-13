package video

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runAPIMartVideo handles video generation via APIMart async task API.
func runAPIMartVideo(req *types.VideoGenerateRequest) ([]string, error) {
	// Resolve local image files in image_urls
	if len(req.ImageURLs) > 0 {
		c := options.NewClient("video")
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve image-urls: %w", err)
		}
		req.ImageURLs = resolved
	}
	// Resolve local image files in image_with_roles
	for i := range req.ImageWithRoles {
		c := options.NewClient("video")
		resolved, err := c.ResolveLocalImages([]string{req.ImageWithRoles[i].URL})
		if err != nil {
			return nil, fmt.Errorf("failed to resolve image-with-role: %w", err)
		}
		req.ImageWithRoles[i].URL = resolved[0]
	}

	c := options.NewClient("video")
	options.ApplyTimeout(c, "video", client.VideoTimeout)
	resp, err := c.VideoSubmit(req)
	if err != nil {
		return nil, fmt.Errorf("submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("submission returned no tasks")
	}

	task := resp.Data[0]
	fmt.Printf("Provider: %s\n", options.Shared.ResolveProvider("video").ProviderType)
	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Response code: %d\n", resp.Code)
	fmt.Printf("Task ID: %s\n", task.TaskID)
	fmt.Printf("Status: %s\n\n", task.Status)

	fmt.Println("Polling for completion...")
	taskData, err := c.PollTask(task.TaskID)
	if err != nil {
		return nil, fmt.Errorf("polling failed: %w", err)
	}

	if options.Shared.Verbose {
		prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
		fmt.Printf("\nTask result:\n%s\n", string(prettyResult))
	}

	fmt.Println()
	savePromptFile(taskData.ID, req.Prompt)
	var saved []string
	if taskData.Result != nil && len(taskData.Result.Videos) > 0 {
		saved, err = service.DownloadVideos(taskData.Result.Videos, options.Shared.OutputDir, taskData.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
		}
	}

	fmt.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)
	return saved, nil
}
