package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// ============================================================================
// Flag registration helpers
// ============================================================================

// registerTaskActionFlags registers the common flags shared by task-action subcommands
// (upscale, variation, high-variation, low-variation, inpaint).
func registerTaskActionFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&mjTaskID, "task-id", "", "Parent task ID (required)")
	f.IntVar(&mjIndex, "index", 0, "Tile index (1-4, or omit for single-image tasks)")
	f.StringVar(&mjCustomID, "custom-id", "", "Button customId (bypasses auto matching)")
	f.StringVar(&mjSpeed, "speed", "", "Speed: relax (default), fast, turbo")
}

// registerImagineStructuredFlags registers the shared imagine structured-field flags
// used by imagine, edits, and similar.
func registerImagineStructuredFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&mjSize, "size", "", `Aspect ratio, e.g. "16:9", "1:1", "9:16"`)
	f.StringVar(&mjQuality, "quality", "", `Quality: "0.25", "0.5", "1", "2"`)
	f.StringVar(&mjStyle, "style", "", `Style override, e.g. "raw"`)
	f.StringVar(&mjVersion, "version", "", `MJ version: "8.1", "7", "6.1", "5.2", "5.1"`)
	f.IntVar(&mjSeed, "seed", 0, "Random seed for reproducibility")
	f.StringVar(&mjNegPrompt, "negative-prompt", "", `Negative prompt (--no)`)
	f.IntVar(&mjStylize, "stylize", 0, "Stylize value (--s, 0-1000)")
	f.IntVar(&mjChaos, "chaos", 0, "Chaos value (--c, 0-100)")
	f.IntVar(&mjWeird, "weird", 0, "Weird value (--w, 0-3000)")
	f.BoolVar(&mjTile, "tile", false, "Tile mode (--tile)")
	f.BoolVar(&mjNiji, "niji", false, "Niji model switch")
	f.Float64Var(&mjIw, "iw", 0, "Image weight (--iw, 0-3)")
	f.IntVar(&mjCw, "cw", 0, "Reference weight for character ref (--cw, 0-100)")
	f.IntVar(&mjSw, "sw", 0, "Style weight (--sw, 0-1000)")
	f.StringVar(&mjCref, "cref", "", "Character reference image URL (--cref)")
	f.StringVar(&mjSref, "sref", "", "Style reference image URL (--sref)")
	f.StringVar(&mjDref, "dref", "", "Depth reference image URL (--dref)")
	f.Float64Var(&mjDw, "dw", 0, "Depth weight (--dw, 0-100)")
	f.IntVar(&mjRepeat, "repeat", 0, "Repeat count (--repeat, 2-40)")
	f.BoolVar(&mjRaw, "raw", false, "Raw style (--raw, v5.1+)")
	f.BoolVar(&mjDraft, "draft", false, "Draft mode (--draft, v7+)")
	f.BoolVar(&mjHd, "hd", false, "HD mode (--hd, v8/v8.1)")
	f.IntVar(&mjStop, "stop", 0, "Early stop (--stop, 10-100)")
	f.StringVar(&mjExtra, "extra", "", "Extra flags appended verbatim (--xxx)")
}

// registerSharedFlags registers flags common to all MJ subcommands.
func registerSharedFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&mjPrompt, "prompt", "p", "", "Text prompt (or \"-\" for stdin)")
	f.StringArrayVar(&mjImageURLs, "image-url", nil, "Image URL or local path (repeatable)")
	f.StringVar(&mjTaskID, "task-id", "", "Parent task ID (required for follow-up actions)")
	f.IntVar(&mjIndex, "index", 0, "Tile index (1-4)")
	f.StringVar(&mjCustomID, "custom-id", "", "Button customId for direct action")
	f.StringVar(&mjSpeed, "speed", "", "Speed: relax (default), fast, turbo")
	f.BoolVar(&mjDryRun, "dry-run", false, "Print request parameters without calling API")
	f.StringVar(&mjJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
}

// ============================================================================
// Flag helpers (pointer-aware)
// ============================================================================

func setMJIntFlag(cmd *cobra.Command, name string, target **int, val int) {
	if cmd.Flags().Changed(name) {
		v := val
		*target = &v
	}
}

func setMJFloatFlag(cmd *cobra.Command, name string, target **float64, val float64) {
	if cmd.Flags().Changed(name) {
		v := val
		*target = &v
	}
}

func setMJBoolFlag(cmd *cobra.Command, name string, target **bool, val bool) {
	if cmd.Flags().Changed(name) {
		v := val
		*target = &v
	}
}

// ============================================================================
// Prompt resolver
// ============================================================================

func resolveMJPrompt(cmd *cobra.Command) (string, error) {
	input := mjPrompt
	if input == "" && cmd.Flags().Changed("prompt") {
		// prompt was set to empty explicitly -- that's OK for some endpoints
		return "", nil
	}
	if input == "" {
		// For imagine/prompt-required commands, we'll check later
		return "", nil
	}
	if input == "-" || isFile(input) {
		data, err := readInput(input)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt: %w", err)
		}
		return string(data), nil
	}
	return input, nil
}

// ============================================================================
// Curl builder
// ============================================================================

func buildMJCurl(action string, reqBody any) string {
	body, _ := json.Marshal(reqBody)
	base := shared.APIBase
	if base == "" {
		base = "https://api.apimart.ai/v1"
	}
	base = strings.TrimRight(base, "/")
	url := base + "/midjourney/generations/" + action

	cmd := fmt.Sprintf("curl -X POST %s \\\n", url)
	cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", shared.APIKey)
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

	if shared.Verbose {
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

	if shared.Verbose {
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

// midjourneySubmitAndGetText submits an MJ action, polls for completion,
// downloads results, and returns a text summary. Shared by CLI and agent loop.
func midjourneySubmitAndGetText(c client.APIClient, action string, req any) (string, error) {
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
			if saved, err := downloadImages(images, taskData.ID); err == nil {
				for _, f := range saved {
					fmt.Printf("Saved: %s\n", f)
				}
			}
		}
		if taskData.VideoURL != "" {
			videos := []types.VideoResult{{URL: []string{taskData.VideoURL}}}
			if saved, err := downloadVideos(videos, taskData.ID); err == nil {
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
			if saved, err := downloadVideos(videos, taskData.ID); err == nil {
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

// registerMJTaskActionSubcommand creates a task-action subcommand (upscale, variation, etc.).
func registerMJTaskActionSubcommand(name, short, long, action string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   name,
		Short: short,
		Long:  long,
		RunE: func(_ *cobra.Command, args []string) error {
			req, err := buildMJTaskActionReqFromJSON()
			if err != nil {
				return err
			}
			// Merge config defaults
			if cfg, err := config.LoadDefaults(shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil && cfg.Defaults.Midjourney != nil {
				// Only speed is relevant for task actions
				if req.Speed == "" && cfg.Defaults.Midjourney.Speed != "" {
					req.Speed = cfg.Defaults.Midjourney.Speed
				}
			}
			c := newMJClient()
			return runMJSubmitAndPoll(c, action, req)
		},
	}
	registerTaskActionFlags(cmd)
	cmd.Flags().StringVar(&mjJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	return cmd
}
