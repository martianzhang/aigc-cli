package midjourney

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func buildMJCurl(action string, reqBody any) string {
	p := options.Shared.ResolveProvider(options.ProviderNameMidjourney)
	body, _ := json.Marshal(reqBody)
	url := client.NormalizeBaseURL(p.BaseURL) + "/midjourney/generations/" + action

	cmd := fmt.Sprintf("curl -X POST %s \\\n", url)
	cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", service.MaskKey(p.APIKey))
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(body))
	return cmd
}

// ============================================================================
// Runner + Display
// ============================================================================

// runMJSubmitAndPoll submits an MJ action, polls, and displays results.
func runMJSubmitAndPoll(c client.APIClient, action string, req any) error {
	if mjDryRun {
		fmt.Println(buildMJCurl(action, req))
		return nil
	}

	if options.Shared.Verbose {
		prettyReq, _ := json.MarshalIndent(req, "", "  ")
		fmt.Printf("Request:\n%s\n\n", string(prettyReq))
	}

	resp, err := c.MidjourneySubmit(action, req)
	if err != nil {
		return fmt.Errorf("submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return fmt.Errorf("submission returned no tasks")
	}

	task := resp.Data[0]
	fmt.Printf("Response code: %d\n", resp.Code)
	fmt.Printf("Task ID: %s\n", task.TaskID)
	fmt.Printf("Status: %s\n\n", task.Status)

	// If the task immediately entered MODAL state, don't wait
	if task.Status == "modal" {
		fmt.Println("Task entered MODAL state -- call `midjourney modal` to submit parameters.")
		return nil
	}

	fmt.Println("Polling for completion...")
	taskData, err := c.MidjourneyPollTask(task.TaskID)
	if err != nil {
		return fmt.Errorf("polling failed: %w", err)
	}

	displayMJResult(taskData)
	return nil
}

// displayMJResult prints the MJ task result in a human-readable format.
func displayMJResult(task *types.MJTaskData) {
	if task == nil {
		return
	}

	if options.Shared.Verbose {
		pretty, _ := json.MarshalIndent(task, "", "  ")
		fmt.Printf("\nTask result:\n%s\n", string(pretty))
	}

	fmt.Println()
	fmt.Printf("Action: %s | Status: %s\n", task.Action, task.Status)

	if task.FailReason != "" {
		fmt.Printf("Fail reason: %s\n", task.FailReason)
		return
	}

	if task.Status != "SUCCESS" && task.Status != "success" {
		fmt.Printf("Task is in state: %s\n", task.Status)
		if task.Status == "MODAL" || task.Status == "modal" {
			fmt.Println("Call `midjourney modal` with --task-id and --mask-url/--prompt to continue.")
		}
		return
	}

	if task.GridImageURL != "" {
		fmt.Printf("Grid image: %s\n", task.GridImageURL)
	}
	for i, u := range task.ImageURLs {
		fmt.Printf("Image %d: %s\n", i+1, u)
	}
	if task.VideoURL != "" {
		fmt.Printf("Video: %s\n", task.VideoURL)
	}
	for i, u := range task.VideoURLs {
		fmt.Printf("Video %d: %s\n", i+1, u)
	}
	if task.Prompt != "" {
		fmt.Printf("Prompt: %s\n", task.Prompt)
	}
	if task.Description != "" {
		fmt.Printf("Description: %s\n", task.Description)
	}
	if len(task.Buttons) > 0 {
		fmt.Println("\nFollow-up buttons:")
		for _, b := range task.Buttons {
			fmt.Printf("  [%s] customId: %s\n", b.Label, b.CustomID)
		}
	}

	if task.Cost > 0 || task.ActualTime > 0 {
		fmt.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
			task.ActualTime, task.Cost, task.CreditsCost)
	}
}

// midjourneyResultSummary returns a text summary of an MJ task result.
// Shared by CLI and agent loop.
func midjourneyResultSummary(task *types.MJTaskData) string {
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

// SubmitAndGetText submits an MJ action, polls for completion,
// downloads results, and returns a text summary. Shared by CLI and agent loop.
func SubmitAndGetText(c client.APIClient, action string, req any) (string, error) {
	resp, err := c.MidjourneySubmit(action, req)
	if err != nil {
		return "", fmt.Errorf("submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("submission returned no tasks")
	}

	task := resp.Data[0]

	// If the task immediately entered MODAL state, don't wait
	if task.Status == "modal" {
		return fmt.Sprintf("Task %s entered MODAL state. Call midjourney modal to submit parameters.", task.TaskID), nil
	}

	taskData, err := c.MidjourneyPollTask(task.TaskID)
	if err != nil {
		return "", fmt.Errorf("polling failed: %w", err)
	}

	// Download images if available
	if taskData.Status == "SUCCESS" || taskData.Status == "success" {
		if len(taskData.ImageURLs) > 0 {
			images := make([]types.ImageResult, len(taskData.ImageURLs))
			for i, u := range taskData.ImageURLs {
				images[i] = types.ImageResult{URL: []string{u}}
			}
			if saved, err := service.DownloadImages(images, options.Shared.OutputDir, taskData.ID); err == nil {
				for _, f := range saved {
					fmt.Printf("Saved: %s\n", f)
				}
			}
		}
		if taskData.VideoURL != "" {
			videos := []types.VideoResult{{URL: []string{taskData.VideoURL}}}
			if saved, err := service.DownloadVideos(videos, options.Shared.OutputDir, taskData.ID); err == nil {
				for _, f := range saved {
					fmt.Printf("Saved: %s\n", f)
				}
			}
		}
		if len(taskData.VideoURLs) > 0 {
			videos := make([]types.VideoResult, len(taskData.VideoURLs))
			for i, u := range taskData.VideoURLs {
				videos[i] = types.VideoResult{URL: []string{u}}
			}
			if saved, err := service.DownloadVideos(videos, options.Shared.OutputDir, taskData.ID); err == nil {
				for _, f := range saved {
					fmt.Printf("Saved: %s\n", f)
				}
			}
		}
	}

	return midjourneyResultSummary(taskData), nil
}

// ============================================================================
// MJ subcommand registration helper
// ============================================================================
