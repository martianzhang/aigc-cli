package video

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/clientfactory"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
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
		ui.Println(curl)
		return nil
	}

	if options.Shared.Verbose {
		prettyReq, _ := json.MarshalIndent(req, "", "  ")
		ui.Printf("Request:\n%s\n\n", string(prettyReq))
	}

	c := clientfactory.NewClient("video")
	clientfactory.ApplyTimeout(c, "video", client.VideoTimeout)
	resp, err := c.VideoRemixSubmit(vidTaskID, req)
	if err != nil {
		return fmt.Errorf("remix submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return fmt.Errorf("remix returned no tasks")
	}

	task := resp.Data[0]
	ui.Printf("Provider: %s\n", options.Shared.ResolveProvider("video").ProviderType)
	ui.Printf("Response code: %d\n", resp.Code)
	ui.Printf("Task ID: %s\n", task.TaskID)
	ui.Printf("Status: %s\n\n", task.Status)

	ui.Println("Polling for completion...")
	taskData, err := c.PollTask(task.TaskID, clientfactory.TaskWait(types.MediaVideo))
	if err != nil {
		return fmt.Errorf("polling failed: %w", err)
	}

	if options.Shared.Verbose {
		prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
		ui.Printf("\nTask result:\n%s\n", string(prettyResult))
	}

	ui.Println()
	service.SavePromptBestEffort(options.Shared.SavePrompt, options.Shared.OutputDir, "video", taskData.ID, req.Prompt)
	var saved []string
	if taskData.Result == nil || len(taskData.Result.Videos) == 0 {
		return fmt.Errorf("task completed but returned no videos")
	}
	saved, err = service.DownloadVideos(taskData.Result.Videos, options.Shared.OutputDir, taskData.ID)
	if err != nil {
		return err
	}
	if vidCropMargin != "" {
		if cropped, cerr := cropSavedVideos(saved); cerr != nil {
			ui.Eprintf("Warning: video crop failed: %v\n", cerr)
		} else {
			saved = cropped
		}
	}

	ui.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)

	if vidPreview {
		for _, f := range saved {
			if e := service.PreviewFile(f); e != nil {
				ui.Eprintf("Warning: preview failed: %v\n", e)
			}
		}
	}

	return nil
}
