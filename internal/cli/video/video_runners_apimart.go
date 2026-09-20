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
// Local images are uploaded by videoPlan.applyUploads before dispatch.
func runAPIMartVideo(req *types.VideoGenerateRequest) ([]string, error) {
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
