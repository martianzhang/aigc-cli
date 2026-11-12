// Package midjourney contains the shared submit/poll/summarize flow used by the
// MCP server, independent of cobra/CLI state.
package midjourney

import (
	"fmt"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// SubmitAndGetText submits an MJ action, polls for completion, downloads
// results, and returns a text summary.
func SubmitAndGetText(c client.APIClient, action string, req any, injectJSON string, taskWait time.Duration, outputDir string) (string, error) {
	if err := service.ApplyMJInject(req, injectJSON); err != nil {
		return "", err
	}
	resp, err := c.MidjourneySubmit(action, req)
	if err != nil {
		return "", fmt.Errorf("submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("submission returned no tasks")
	}

	task := resp.Data[0]
	if task.Status == "modal" {
		return fmt.Sprintf("Task %s entered MODAL state. Call midjourney modal to submit parameters.", task.TaskID), nil
	}

	taskData, err := c.MidjourneyPollTask(task.TaskID, taskWait)
	if err != nil {
		return "", fmt.Errorf("polling failed: %w", err)
	}

	var dlErr error
	if taskData.Status == "SUCCESS" || taskData.Status == "success" {
		if len(taskData.ImageURLs) > 0 {
			images := make([]types.ImageResult, len(taskData.ImageURLs))
			for i, u := range taskData.ImageURLs {
				images[i] = types.ImageResult{URL: []string{u}}
			}
			saved, derr := service.DownloadImages(images, outputDir, taskData.ID)
			if derr != nil {
				dlErr = derr
			}
			for _, f := range saved {
				ui.Printf("Saved: %s\n", f)
			}
		}
		if taskData.VideoURL != "" {
			videos := []types.VideoResult{{URL: []string{taskData.VideoURL}}}
			saved, derr := service.DownloadVideos(videos, outputDir, taskData.ID)
			if derr != nil && dlErr == nil {
				dlErr = derr
			}
			for _, f := range saved {
				ui.Printf("Saved: %s\n", f)
			}
		}
		if len(taskData.VideoURLs) > 0 {
			videos := make([]types.VideoResult, len(taskData.VideoURLs))
			for i, u := range taskData.VideoURLs {
				videos[i] = types.VideoResult{URL: []string{u}}
			}
			saved, derr := service.DownloadVideos(videos, outputDir, taskData.ID)
			if derr != nil && dlErr == nil {
				dlErr = derr
			}
			for _, f := range saved {
				ui.Printf("Saved: %s\n", f)
			}
		}
	}
	if dlErr != nil {
		return "", fmt.Errorf("download failed: %w", dlErr)
	}

	return ResultSummary(taskData), nil
}

// ResultSummary renders a human-readable MJ task summary.
func ResultSummary(task *types.MJTaskData) string {
	if task == nil {
		return "No result returned."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Action: %s | Status: %s", task.Action, task.Status)
	if task.FailReason != "" {
		fmt.Fprintf(&b, "\nFail reason: %s", task.FailReason)
		return b.String()
	}
	if task.Status == "SUCCESS" || task.Status == "success" {
		if task.GridImageURL != "" {
			fmt.Fprintf(&b, "\nGrid image URL: %s", task.GridImageURL)
		}
		for i, u := range task.ImageURLs {
			fmt.Fprintf(&b, "\nImage %d: %s", i+1, u)
		}
		if task.VideoURL != "" {
			fmt.Fprintf(&b, "\nVideo: %s", task.VideoURL)
		}
		for i, u := range task.VideoURLs {
			fmt.Fprintf(&b, "\nVideo %d: %s", i+1, u)
		}
		if task.Prompt != "" {
			fmt.Fprintf(&b, "\nPrompt: %s", task.Prompt)
		}
		if task.Description != "" {
			fmt.Fprintf(&b, "\nDescription: %s", task.Description)
		}
		if task.Cost > 0 || task.ActualTime > 0 {
			fmt.Fprintf(&b, "\nCompleted in %ds | Cost: $%.5f (%.4f credits)", task.ActualTime, task.Cost, task.CreditsCost)
		}
	} else {
		fmt.Fprintf(&b, "\nTask is in state: %s", task.Status)
	}
	return b.String()
}
