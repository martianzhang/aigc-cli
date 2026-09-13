package video

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runVideoRemix handles the VEO3 remix (video extension) flow.
func runVideoRemix(cmd *cobra.Command) error {
	if vidTaskID == "" {
		return fmt.Errorf("--task-id is required in remix mode (the original video task ID)")
	}

	// Build remix request
	prompt, err := resolveVideoPrompt()
	if err != nil {
		return err
	}
	req := &types.VideoRemixRequest{
		Model:      options.Shared.Model,
		Prompt:     prompt,
		Resolution: vidResolution,
	}
	if vidSize != "" {
		req.AspectRatio = vidSize // --size maps to aspect_ratio in remix
	}
	if cmd.Flags().Changed("raw") {
		v := vidRaw
		req.Raw = &v
	}

	if req.Model == "" {
		return fmt.Errorf("--model is required in remix mode (must match the original video's model)")
	}
	if req.Prompt == "" {
		return fmt.Errorf("--prompt is required in remix mode")
	}

	if vidDryRun {
		curl := buildVideoRemixCurl(req)
		fmt.Println(curl)
		return nil
	}

	if options.Shared.Verbose {
		prettyReq, _ := json.MarshalIndent(req, "", "  ")
		fmt.Printf("Request:\n%s\n\n", string(prettyReq))
	}

	c := options.NewClient("video")
	options.ApplyTimeout(c, "video", client.VideoTimeout)
	resp, err := c.VideoRemixSubmit(vidTaskID, req)
	if err != nil {
		return fmt.Errorf("remix submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return fmt.Errorf("remix returned no tasks")
	}

	task := resp.Data[0]
	fmt.Printf("Provider: %s\n", options.Shared.ResolveProvider("video").ProviderType)
	fmt.Printf("Response code: %d\n", resp.Code)
	fmt.Printf("Task ID: %s\n", task.TaskID)
	fmt.Printf("Status: %s\n\n", task.Status)

	fmt.Println("Polling for completion...")
	taskData, err := c.PollTask(task.TaskID)
	if err != nil {
		return fmt.Errorf("polling failed: %w", err)
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
	if vidCropMargin != "" {
		if cropped, cerr := cropSavedVideos(saved); cerr != nil {
			fmt.Fprintf(os.Stderr, "Warning: video crop failed: %v\n", cerr)
		} else {
			saved = cropped
		}
	}

	fmt.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)

	if vidPreview {
		for _, f := range saved {
			if e := service.PreviewFile(f); e != nil {
				fmt.Fprintf(os.Stderr, "Warning: preview failed: %v\n", e)
			}
		}
	}

	return nil
}
