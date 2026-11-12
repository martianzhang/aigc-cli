package midjourney

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/clientfactory"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// applyMJInject attaches the --inject-json value, if any, to an MJ request.
func applyMJInject(req any) error {
	return service.ApplyMJInject(req, mjInjectJSON)
}

func buildMJCurl(action string, reqBody any) string {
	p := options.Shared.ResolveProvider(options.ProviderNameMidjourney)
	url := client.NormalizeBaseURL(p.BaseURL) + "/midjourney/generations/" + action
	plan := &reqbuild.Plan{Method: http.MethodPost, URL: url, Body: reqBody}
	curl, err := plan.RenderCurlsErr(p.APIKey)
	if err != nil {
		return fmt.Sprintf("# failed to render preview: %v", err)
	}
	return curl
}

// ============================================================================
// Runner + Display
// ============================================================================

// runMJSubmitAndPoll submits an MJ action, polls, and displays results.
func runMJSubmitAndPoll(c client.APIClient, action string, req any) error {
	if err := options.RequireBaseURL("midjourney", options.Shared.ResolveProvider(options.ProviderNameMidjourney)); err != nil {
		return err
	}
	if err := applyMJInject(req); err != nil {
		return err
	}
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
	taskData, err := c.MidjourneyPollTask(task.TaskID, clientfactory.TaskWait(types.MediaMidjourney))
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

// ============================================================================
// MJ subcommand registration helper
// ============================================================================
