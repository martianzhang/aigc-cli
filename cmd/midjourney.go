package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/martianzhang/apimart-cli/internal/types"
)

var (
	mjClientOnce sync.Once
	mjClientInst client.APIClient
)

// newMJClient returns a singleton Midjourney API client, created once with
// the current shared config and MJ-specific timeout. Reuses the same client
// across all 27+ MJ subcommands instead of creating a new one each time.
func newMJClient() client.APIClient {
	mjClientOnce.Do(func() {
		mjClientInst = client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
		applyTimeout(mjClientInst, "midjourney", client.MJTimeout)
	})
	return mjClientInst
}

// ============================================================================
// MJ shared flag variables
// ============================================================================
var (
	mjPrompt    string
	mjImageURLs []string
	mjTaskID    string
	mjIndex     int
	mjCustomID  string
	mjSpeed     string
	mjDryRun    bool
	mjJSONInput string
)

// MJ imagine (structured) flag variables
var (
	mjSize      string
	mjQuality   string
	mjStyle     string
	mjVersion   string
	mjSeed      int
	mjNegPrompt string
	mjStylize   int
	mjChaos     int
	mjWeird     int
	mjTile      bool
	mjNiji      bool
	mjIw        float64
	mjCw        int
	mjSw        int
	mjCref      string
	mjSref      string
	mjDref      string
	mjDw        float64
	mjRepeat    int
	mjRaw       bool
	mjDraft     bool
	mjHd        bool
	mjStop      int
	mjExtra     string
)

// MJ blend-specific
var mjDimensions string

// MJ pan-specific
var mjDirection string

// MJ zoom-specific
var mjZoomRatio float64

// MJ modal-specific
var mjMaskURL string

// MJ video-specific
var (
	mjVideoType   string
	mjAnimateMode string
	mjMotion      string
	mjBatchSize   int
	mjEndURL      string
)

// ============================================================================
// Parent command: apimart-cli midjourney
// ============================================================================
var midjourneyCmd = &cobra.Command{
	Use:          "midjourney",
	Aliases:      []string{"mj"},
	Short:        "Midjourney image generation (APIMart async task-based)",
	SilenceUsage: true,
	Long: `Generate and edit images via Midjourney on APIMart.

Midjourney uses an async task model — you submit a job, get a task_id,
then poll for results. All MJ endpoints are under /v1/midjourney/.

Alias: mj (e.g. "apimart-cli mj imagine ...")

Subcommands:
  imagine          Text-to-image / image-guided (default entry)
  blend            Multi-image blend (2-4 images)
  describe         Image to text (reverse prompt)
  edits            Image edit (rewrite whole image)
  upscale          Upscale a tile (U1-U4)
  variation        Subtle variation (V1-V4)
  high-variation   High (strong) variation
  low-variation    Low (subtle) variation
  reroll           Regenerate the grid
  zoom             Zoom out / outpaint
  pan              Pan in a direction
  inpaint          Region inpaint entry (→ modal)
  modal            Submit mask + prompt for inpaint
  video            Image-to-video
  remix-strong     Strong reshape (v8/v8.1)
  remix-subtle     Subtle reshape (v8/v8.1)
  query            Get MJ task status

Examples:
  apimart-cli midjourney imagine --prompt "a cute cat --ar 16:9"
  apimart-cli mj imagine --prompt "a cute cat"  # same with alias
  apimart-cli midjourney blend --image-url a.png --image-url b.png
  apimart-cli midjourney upscale --task-id task_xxx --index 1
  apimart-cli midjourney query task_xxx`,
}

// ============================================================================
// Shared helpers
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
// Builders
// ============================================================================

// buildMJImagineReq builds MJImagineRequest from flags or --json.
func buildMJImagineReq(cmd *cobra.Command) (*types.MJImagineRequest, error) {
	if mjJSONInput != "" {
		data, err := readInput(mjJSONInput)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.MJImagineRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		if req.Prompt == "" {
			return nil, fmt.Errorf("prompt is required in JSON input")
		}
		return req, nil
	}

	prompt, err := resolveMJPrompt(cmd)
	if err != nil {
		return nil, err
	}

	req := &types.MJImagineRequest{
		Prompt:         prompt,
		ImageURLs:      mjImageURLs,
		Speed:          mjSpeed,
		Size:           mjSize,
		Quality:        mjQuality,
		Style:          mjStyle,
		Version:        mjVersion,
		NegativePrompt: mjNegPrompt,
		Cref:           mjCref,
		Sref:           mjSref,
		Dref:           mjDref,
		Extra:          mjExtra,
	}

	setMJIntFlag(cmd, "seed", &req.Seed, mjSeed)
	setMJIntFlag(cmd, "stylize", &req.Stylize, mjStylize)
	setMJIntFlag(cmd, "chaos", &req.Chaos, mjChaos)
	setMJIntFlag(cmd, "weird", &req.Weird, mjWeird)
	setMJIntFlag(cmd, "cw", &req.Cw, mjCw)
	setMJIntFlag(cmd, "sw", &req.Sw, mjSw)
	setMJIntFlag(cmd, "repeat", &req.Repeat, mjRepeat)
	setMJIntFlag(cmd, "stop", &req.Stop, mjStop)
	setMJFloatFlag(cmd, "iw", &req.Iw, mjIw)
	setMJFloatFlag(cmd, "dw", &req.Dw, mjDw)
	setMJBoolFlag(cmd, "tile", &req.Tile, mjTile)
	setMJBoolFlag(cmd, "niji", &req.Niji, mjNiji)
	setMJBoolFlag(cmd, "raw", &req.Raw, mjRaw)
	setMJBoolFlag(cmd, "draft", &req.Draft, mjDraft)
	setMJBoolFlag(cmd, "hd", &req.Hd, mjHd)

	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required (use --prompt or --json)")
	}
	return req, nil
}

// buildMJTaskActionReq builds MJTaskActionRequest from flags.
func buildMJTaskActionReq() (*types.MJTaskActionRequest, error) {
	if mjTaskID == "" {
		return nil, fmt.Errorf("--task-id is required")
	}
	req := &types.MJTaskActionRequest{
		TaskID:   mjTaskID,
		CustomID: mjCustomID,
		Speed:    mjSpeed,
	}
	if mjIndex > 0 {
		v := mjIndex
		req.Index = &v
	}
	return req, nil
}

// buildMJTaskActionReqFromJSON builds MJTaskActionRequest from --json or flags.
func buildMJTaskActionReqFromJSON() (*types.MJTaskActionRequest, error) {
	if mjJSONInput != "" {
		data, err := readInput(mjJSONInput)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.MJTaskActionRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		if req.TaskID == "" {
			return nil, fmt.Errorf("task_id is required in JSON input")
		}
		return req, nil
	}
	return buildMJTaskActionReq()
}

func resolveMJPrompt(cmd *cobra.Command) (string, error) {
	input := mjPrompt
	if input == "" && cmd.Flags().Changed("prompt") {
		// prompt was set to empty explicitly — that's OK for some endpoints
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
// Submit + Poll + Display runner
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
		fmt.Println("Task entered MODAL state — call `midjourney modal` to submit parameters.")
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

// ============================================================================
// Subcommand: imagine
// ============================================================================
var mjImagineCmd = &cobra.Command{
	Use:   "imagine",
	Short: "Text-to-image / image-guided generation",
	Long: `Generate images from a text prompt, optionally with reference images.

The default MJ entry point. Supports all MJ structured fields and native flags.

Examples:
  apimart-cli midjourney imagine --prompt "a cute cat --ar 16:9"
  apimart-cli midjourney imagine --prompt "a cat" --size "16:9" --version "6.1" --style raw
  apimart-cli midjourney imagine --prompt "luxury product" --image-url ref.png --iw 1.2
  apimart-cli midjourney imagine --json request.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		req, err := buildMJImagineReq(cmd)
		if err != nil {
			return err
		}

		// Merge config defaults
		if cfg, err := config.LoadDefaults(shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
			cfg.Defaults.Midjourney.MergeIntoImagine(req)
		}

		// Resolve local images
		if len(req.ImageURLs) > 0 {
			c := newMJClient()
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
		}

		c := newMJClient()
		return runMJSubmitAndPoll(c, "imagine", req)
	},
}

// ============================================================================
// Subcommand: blend
// ============================================================================
var mjBlendCmd = &cobra.Command{
	Use:   "blend",
	Short: "Multi-image blend (2-4 images)",
	Long: `Blend 2-4 images into a new image. No prompt is used — pure image blend.

Examples:
  apimart-cli midjourney blend --image-url a.png --image-url b.png
  apimart-cli midjourney blend --image-url a.png --image-url b.png --image-url c.png --dimensions PORTRAIT`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJBlendRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if len(req.ImageURLs) < 2 {
				return fmt.Errorf("at least 2 image_urls required")
			}
			c := newMJClient()
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
			return runMJSubmitAndPoll(c, "blend", req)
		}

		if len(mjImageURLs) < 2 {
			return fmt.Errorf("at least 2 --image-url required for blend")
		}

		req := &types.MJBlendRequest{
			ImageURLs:  mjImageURLs,
			Dimensions: mjDimensions,
			Size:       mjSize,
			Speed:      mjSpeed,
		}

		c := newMJClient()
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return fmt.Errorf("failed to resolve image-urls: %w", err)
		}
		req.ImageURLs = resolved
		return runMJSubmitAndPoll(c, "blend", req)
	},
}

// ============================================================================
// Subcommand: describe
// ============================================================================
var mjDescribeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Image to text (reverse prompt)",
	Long: `Reverse-engineer a prompt from an image. Returns 4 prompt suggestions.

Example:
  apimart-cli midjourney describe --image-url input.png`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJDescribeRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if len(req.ImageURLs) == 0 {
				return fmt.Errorf("image_urls is required")
			}
			c := newMJClient()
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
			return runMJSubmitAndPoll(c, "describe", req)
		}

		if len(mjImageURLs) == 0 {
			return fmt.Errorf("--image-url is required for describe")
		}

		req := &types.MJDescribeRequest{
			ImageURLs: mjImageURLs,
			Speed:     mjSpeed,
		}
		c := newMJClient()
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return fmt.Errorf("failed to resolve image-urls: %w", err)
		}
		req.ImageURLs = resolved
		return runMJSubmitAndPoll(c, "describe", req)
	},
}

// ============================================================================
// Subcommand: edits
// ============================================================================
var mjEditsCmd = &cobra.Command{
	Use:   "edits",
	Short: "Image edit (rewrite whole image)",
	Long: `Rewrite an entire image from a prompt + reference image.
Good for background replacement, style transfer, and content changes.

Example:
  apimart-cli midjourney edits --prompt "replace background with a modern kitchen" --image-url product.png`,
	RunE: func(cmd *cobra.Command, args []string) error {
		req, err := buildMJImagineReq(cmd) // Same structure as imagine
		if err != nil {
			return err
		}
		if len(req.ImageURLs) == 0 {
			return fmt.Errorf("--image-url is required for edits")
		}

		if cfg, err := config.LoadDefaults(shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
			cfg.Defaults.Midjourney.MergeIntoImagine(req)
		}

		if len(req.ImageURLs) > 0 {
			c := newMJClient()
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
		}

		c := newMJClient()
		return runMJSubmitAndPoll(c, "edits", req)
	},
}

// ============================================================================
// Task-action subcommands (upscale, variation, high-variation, low-variation, inpaint)
// ============================================================================
var mjUpscaleCmd = registerMJTaskActionSubcommand(
	"upscale",
	"Upscale a tile (U1-U4)",
	`Upscale one tile from the parent grid (U1-U4).

Composed locally from existing images — usually returns instantly.

Examples:
  apimart-cli midjourney upscale --task-id task_xxx --index 1
  apimart-cli midjourney upscale --task-id task_xxx --custom-id "MJ::JOB::upsample::1::abc"`,
	"upscale",
)

var mjVariationCmd = registerMJTaskActionSubcommand(
	"variation",
	"Subtle variation (V1-V4)",
	`Create a subtle variation (varySubtle) from one tile of an Imagine grid.

Examples:
  apimart-cli midjourney variation --task-id task_xxx --index 3`,
	"variation",
)

var mjHighVariationCmd = registerMJTaskActionSubcommand(
	"high-variation",
	"High (strong) variation",
	`Create a strong variation (varyStrong) from one tile of an Imagine grid.

Example:
  apimart-cli midjourney high-variation --task-id task_xxx --index 2`,
	"high-variation",
)

var mjLowVariationCmd = registerMJTaskActionSubcommand(
	"low-variation",
	"Low (subtle) variation",
	`Create a low (subtle) variation from one tile.

Example:
  apimart-cli midjourney low-variation --task-id task_xxx --index 4`,
	"low-variation",
)

// ============================================================================
// Subcommand: reroll
// ============================================================================
var mjRerollCmd = &cobra.Command{
	Use:   "reroll",
	Short: "Regenerate the grid (🔄)",
	Long: `Regenerate 4 images from the source task's prompt. No index needed - whole grid is rerolled.

Example:
  apimart-cli midjourney reroll --task-id task_xxx`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJRerollRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if req.TaskID == "" {
				return fmt.Errorf("task_id is required")
			}
			c := newMJClient()
			return runMJSubmitAndPoll(c, "reroll", req)
		}

		if mjTaskID == "" {
			return fmt.Errorf("--task-id is required for reroll")
		}
		req := &types.MJRerollRequest{
			TaskID:   mjTaskID,
			CustomID: mjCustomID,
			Speed:    mjSpeed,
		}
		c := newMJClient()
		return runMJSubmitAndPoll(c, "reroll", req)
	},
}

// ============================================================================
// Subcommand: zoom
// ============================================================================
var mjZoomCmd = &cobra.Command{
	Use:   "zoom",
	Short: "Zoom out / outpaint",
	Long: `Zoom out on a single image after Upscale. zoom_ratio < 2 uses Outpaint (1.5x),
>= 2 or omitted uses CustomZoom (2x).

Example:
  apimart-cli midjourney zoom --task-id task_xxx --zoom-ratio 1.5`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJZoomRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if req.TaskID == "" {
				return fmt.Errorf("task_id is required")
			}
			c := newMJClient()
			return runMJSubmitAndPoll(c, "zoom", req)
		}

		if mjTaskID == "" {
			return fmt.Errorf("--task-id is required for zoom")
		}
		req := &types.MJZoomRequest{
			TaskID:   mjTaskID,
			CustomID: mjCustomID,
			Speed:    mjSpeed,
		}
		if cmd.Flags().Changed("index") {
			v := mjIndex
			req.Index = &v
		}
		if cmd.Flags().Changed("zoom-ratio") {
			v := mjZoomRatio
			req.ZoomRatio = &v
		}
		c := newMJClient()
		return runMJSubmitAndPoll(c, "zoom", req)
	},
}

// ============================================================================
// Subcommand: pan
// ============================================================================
var mjPanCmd = &cobra.Command{
	Use:   "pan",
	Short: "Pan in a direction",
	Long: `Pan out in a direction on a single image after Upscale.
Direction: left, right, up, down.

Example:
  apimart-cli midjourney pan --task-id task_xxx --direction right`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJPanRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if req.TaskID == "" {
				return fmt.Errorf("task_id is required")
			}
			if req.Direction == "" && req.CustomID == "" {
				return fmt.Errorf("direction or custom_id is required")
			}
			c := newMJClient()
			return runMJSubmitAndPoll(c, "pan", req)
		}

		if mjTaskID == "" {
			return fmt.Errorf("--task-id is required for pan")
		}
		if mjDirection == "" && mjCustomID == "" {
			return fmt.Errorf("--direction (left/right/up/down) or --custom-id is required")
		}
		req := &types.MJPanRequest{
			TaskID:    mjTaskID,
			CustomID:  mjCustomID,
			Direction: mjDirection,
			Speed:     mjSpeed,
		}
		if cmd.Flags().Changed("index") {
			v := mjIndex
			req.Index = &v
		}
		c := newMJClient()
		return runMJSubmitAndPoll(c, "pan", req)
	},
}

// ============================================================================
// Subcommand: inpaint
// ============================================================================
var mjInpaintCmd = &cobra.Command{
	Use:   "inpaint",
	Short: "Region inpaint entry (→ modal)",
	Long: `Entry point for region inpaint (Vary Region). After submission, the task enters
MODAL state — then call "midjourney modal" with a mask + prompt.

Example:
  apimart-cli midjourney inpaint --task-id task_xxx`,
	RunE: func(_ *cobra.Command, args []string) error {
		req, err := buildMJTaskActionReqFromJSON()
		if err != nil {
			return err
		}
		c := newMJClient()
		return runMJSubmitAndPoll(c, "inpaint", req)
	},
}

// ============================================================================
// Subcommand: modal
// ============================================================================
var mjModalCmd = &cobra.Command{
	Use:   "modal",
	Short: "Submit mask + prompt for inpaint",
	Long: `Complete a MODAL-state inpaint task by supplying a mask + prompt.
With mask_url → inpaint (local repaint). Without → outpaint (expand).

Example:
  apimart-cli midjourney modal --task-id task_xxx --prompt "replace with red sofa" --mask-url mask.png`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJModalRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if req.TaskID == "" {
				return fmt.Errorf("task_id is required")
			}
			if req.MaskURL != "" {
				c := newMJClient()
				resolved, err := c.ResolveLocalImages([]string{req.MaskURL})
				if err != nil {
					return fmt.Errorf("failed to resolve mask-url: %w", err)
				}
				req.MaskURL = resolved[0]
			}
			c := newMJClient()
			return runMJSubmitAndPoll(c, "modal", req)
		}

		if mjTaskID == "" {
			return fmt.Errorf("--task-id is required for modal")
		}
		req := &types.MJModalRequest{
			TaskID:  mjTaskID,
			Prompt:  mjPrompt,
			MaskURL: mjMaskURL,
			Speed:   mjSpeed,
		}
		// Resolve local mask
		if req.MaskURL != "" {
			c := newMJClient()
			resolved, err := c.ResolveLocalImages([]string{req.MaskURL})
			if err != nil {
				return fmt.Errorf("failed to resolve mask-url: %w", err)
			}
			req.MaskURL = resolved[0]
		}
		c := newMJClient()
		return runMJSubmitAndPoll(c, "modal", req)
	},
}

// ============================================================================
// Subcommand: video
// ============================================================================
var mjVideoCmd = &cobra.Command{
	Use:   "video",
	Short: "Image-to-video",
	Long: `Generate a video from an image using MJ's image-to-video (i2v).
Text-to-video is NOT supported — a first frame is required.

Examples:
  apimart-cli midjourney video --image-url cat.png --batch-size 4
  apimart-cli midjourney video --task-id task_xxx --index 0 --animate-mode auto`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJVideoRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if len(req.ImageURLs) > 0 {
				c := newMJClient()
				resolved, err := c.ResolveLocalImages(req.ImageURLs)
				if err != nil {
					return fmt.Errorf("failed to resolve image-urls: %w", err)
				}
				req.ImageURLs = resolved
			}
			c := newMJClient()
			return runMJSubmitAndPoll(c, "video", req)
		}

		if len(mjImageURLs) == 0 && mjTaskID == "" {
			return fmt.Errorf("either --image-url or --task-id is required for video")
		}

		req := &types.MJVideoRequest{
			Prompt:      mjPrompt,
			ImageURLs:   mjImageURLs,
			TaskID:      mjTaskID,
			VideoType:   mjVideoType,
			AnimateMode: mjAnimateMode,
			Motion:      mjMotion,
			EndURL:      mjEndURL,
		}
		if cmd.Flags().Changed("index") {
			v := mjIndex
			req.Index = &v
		}
		if cmd.Flags().Changed("batch-size") {
			v := mjBatchSize
			req.BatchSize = &v
		}

		if len(req.ImageURLs) > 0 {
			c := newMJClient()
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
		}
		c := newMJClient()
		return runMJSubmitAndPoll(c, "video", req)
	},
}

// ============================================================================
// Subcommand: remix-strong / remix-subtle
// ============================================================================
var mjRemixStrongCmd = &cobra.Command{
	Use:   "remix-strong",
	Short: "Strong reshape (v8/v8.1 only)",
	Long: `Strong reshape of a v8/v8.1 parent image. Large change; composition/style may shift.

Example:
  apimart-cli midjourney remix-strong --task-id task_xxx --index 1`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMJRmix(cmd, "remix-strong")
	},
}

var mjRemixSubtleCmd = &cobra.Command{
	Use:   "remix-subtle",
	Short: "Subtle reshape (v8/v8.1 only)",
	Long: `Subtle reshape of a v8/v8.1 parent image. Small change; keeps subject/tone.

Example:
  apimart-cli midjourney remix-subtle --task-id task_xxx --index 1 --prompt "new style"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMJRmix(cmd, "remix-subtle")
	},
}

func runMJRmix(cmd *cobra.Command, action string) error {
	if mjJSONInput != "" {
		data, err := readInput(mjJSONInput)
		if err != nil {
			return fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.MJRemixRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return fmt.Errorf("failed to parse JSON: %w", err)
		}
		if req.TaskID == "" {
			return fmt.Errorf("task_id is required")
		}
		c := newMJClient()
		return runMJSubmitAndPoll(c, action, req)
	}

	if mjTaskID == "" {
		return fmt.Errorf("--task-id is required for remix")
	}

	req := &types.MJRemixRequest{
		TaskID: mjTaskID,
		Prompt: mjPrompt,
		Speed:  mjSpeed,
	}
	if cmd.Flags().Changed("index") {
		v := mjIndex
		req.Index = &v
	}
	c := newMJClient()
	return runMJSubmitAndPoll(c, action, req)
}

// ============================================================================
// Subcommand: query
// ============================================================================
var mjQueryCmd = &cobra.Command{
	Use:   "query <task-id>",
	Short: "Get MJ task status and result",
	Long: `Query a Midjourney task by its task ID.

Example:
  apimart-cli midjourney query task_xxx`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := args[0]
		c := newMJClient()
		task, err := c.MidjourneyGetTask(taskID)
		if err != nil {
			return fmt.Errorf("failed to query task: %w", err)
		}
		displayMJResult(task)

		// Download images if available
		if task.Status == "SUCCESS" && len(task.ImageURLs) > 0 {
			// Convert to ImageResult slices for downloadImages helper
			images := make([]types.ImageResult, len(task.ImageURLs))
			for i, u := range task.ImageURLs {
				images[i] = types.ImageResult{URL: []string{u}}
			}
			if _, err := downloadImages(images, task.ID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
			}
		}
		// Download videos if available
		if task.Status == "SUCCESS" && len(task.VideoURLs) > 0 {
			videos := make([]types.VideoResult, len(task.VideoURLs))
			for i, u := range task.VideoURLs {
				videos[i] = types.VideoResult{URL: []string{u}}
			}
			if _, err := downloadVideos(videos, task.ID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
			}
		}
		if task.Status == "SUCCESS" && task.VideoURL != "" {
			// Also download single video_url if present
			videos := []types.VideoResult{{URL: []string{task.VideoURL}}}
			if _, err := downloadVideos(videos, task.ID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
			}
		}
		return nil
	},
}

// ============================================================================
// Init — register all subcommands and flags
// ============================================================================
func init() {
	// --- imagine ---
	registerSharedFlags(mjImagineCmd)
	registerImagineStructuredFlags(mjImagineCmd)
	// Override to remove task-id from shared (not needed for imagine)
	mjImagineCmd.Flags().MarkHidden("task-id")
	mjImagineCmd.Flags().MarkHidden("index")
	mjImagineCmd.Flags().MarkHidden("custom-id")

	// --- blend ---
	mjBlendCmd.Flags().StringArrayVar(&mjImageURLs, "image-url", nil, "Image URLs or local paths (2-4 required, repeatable)")
	mjBlendCmd.Flags().StringVar(&mjDimensions, "dimensions", "", "Aspect preset: SQUARE (1:1), PORTRAIT (2:3), LANDSCAPE (3:2)")
	mjBlendCmd.Flags().StringVar(&mjSize, "size", "", `Free aspect ratio, e.g. "16:9" (takes priority over --dimensions)`)
	mjBlendCmd.Flags().StringVar(&mjSpeed, "speed", "", "Speed: relax (default), fast, turbo")
	mjBlendCmd.Flags().StringVar(&mjJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	mjBlendCmd.Flags().BoolVar(&mjDryRun, "dry-run", false, "Print request without calling API")

	// --- describe ---
	mjDescribeCmd.Flags().StringArrayVar(&mjImageURLs, "image-url", nil, "Image URL or local path (required)")
	mjDescribeCmd.Flags().StringVar(&mjSpeed, "speed", "", "Speed: relax (default), fast, turbo")
	mjDescribeCmd.Flags().StringVar(&mjJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	mjDescribeCmd.Flags().BoolVar(&mjDryRun, "dry-run", false, "Print request without calling API")

	// --- edits ---
	registerSharedFlags(mjEditsCmd)
	registerImagineStructuredFlags(mjEditsCmd)
	mjEditsCmd.Flags().MarkHidden("task-id")
	mjEditsCmd.Flags().MarkHidden("index")
	mjEditsCmd.Flags().MarkHidden("custom-id")

	// --- task-action subcommands (flags registered in registerMJTaskActionSubcommand) ---

	// --- reroll ---
	mjRerollCmd.Flags().StringVar(&mjTaskID, "task-id", "", "Parent task ID (required)")
	mjRerollCmd.Flags().StringVar(&mjCustomID, "custom-id", "", "Button customId for direct action")
	mjRerollCmd.Flags().StringVar(&mjSpeed, "speed", "", "Speed: relax (default), fast, turbo")
	mjRerollCmd.Flags().StringVar(&mjJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	mjRerollCmd.Flags().BoolVar(&mjDryRun, "dry-run", false, "Print request without calling API")

	// --- zoom ---
	mjZoomCmd.Flags().StringVar(&mjTaskID, "task-id", "", "Parent task ID (required)")
	mjZoomCmd.Flags().IntVar(&mjIndex, "index", 0, "Tile index (1-4)")
	mjZoomCmd.Flags().StringVar(&mjCustomID, "custom-id", "", "Button customId for direct action")
	mjZoomCmd.Flags().Float64Var(&mjZoomRatio, "zoom-ratio", 0, "Zoom ratio (<2 = 1.5x Outpaint, >=2 or omit = 2x CustomZoom)")
	mjZoomCmd.Flags().StringVar(&mjSpeed, "speed", "", "Speed: relax (default), fast, turbo")
	mjZoomCmd.Flags().StringVar(&mjJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	mjZoomCmd.Flags().BoolVar(&mjDryRun, "dry-run", false, "Print request without calling API")

	// --- pan ---
	mjPanCmd.Flags().StringVar(&mjTaskID, "task-id", "", "Parent task ID (required)")
	mjPanCmd.Flags().StringVar(&mjDirection, "direction", "", "Direction: left, right, up, down")
	mjPanCmd.Flags().IntVar(&mjIndex, "index", 0, "Tile index (1-4)")
	mjPanCmd.Flags().StringVar(&mjCustomID, "custom-id", "", "Button customId (bypasses direction matching)")
	mjPanCmd.Flags().StringVar(&mjSpeed, "speed", "", "Speed: relax (default), fast, turbo")
	mjPanCmd.Flags().StringVar(&mjJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	mjPanCmd.Flags().BoolVar(&mjDryRun, "dry-run", false, "Print request without calling API")

	// --- inpaint (flags via registerTaskActionFlags in registerMJTaskActionSubcommand) ---
	mjInpaintCmd.Flags().StringVar(&mjJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")

	// --- modal ---
	mjModalCmd.Flags().StringVar(&mjTaskID, "task-id", "", "Inpaint task ID (required, must be in MODAL state)")
	mjModalCmd.Flags().StringVarP(&mjPrompt, "prompt", "p", "", "Inpaint prompt (inherits parent if empty)")
	mjModalCmd.Flags().StringVar(&mjMaskURL, "mask-url", "", "Mask image URL or local path (white=repaint area)")
	mjModalCmd.Flags().StringVar(&mjSpeed, "speed", "", "Speed: relax (default), fast, turbo")
	mjModalCmd.Flags().StringVar(&mjJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	mjModalCmd.Flags().BoolVar(&mjDryRun, "dry-run", false, "Print request without calling API")

	// --- video ---
	mjVideoCmd.Flags().StringVarP(&mjPrompt, "prompt", "p", "", "Video prompt (optional)")
	mjVideoCmd.Flags().StringArrayVar(&mjImageURLs, "image-url", nil, "First frame image URL or local path")
	mjVideoCmd.Flags().StringVar(&mjTaskID, "task-id", "", "Reuse a SUCCESS imagine task ID")
	mjVideoCmd.Flags().IntVar(&mjIndex, "index", 0, "Which tile of the imagine (0-3, with --task-id)")
	mjVideoCmd.Flags().StringVar(&mjVideoType, "video-type", "", "Resolution tier: vid_1.1_i2v_480 (default), vid_1.1_i2v_720")
	mjVideoCmd.Flags().StringVar(&mjAnimateMode, "animate-mode", "", "manual (default) / auto (requires --task-id + --index)")
	mjVideoCmd.Flags().StringVar(&mjMotion, "motion", "", "low / high (default)")
	mjVideoCmd.Flags().IntVar(&mjBatchSize, "batch-size", 0, "Batch size: 1, 2, or 4 (billed ×N)")
	mjVideoCmd.Flags().StringVar(&mjEndURL, "end-url", "", "End frame URL (enables start/end transition)")
	mjVideoCmd.Flags().StringVar(&mjJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	mjVideoCmd.Flags().BoolVar(&mjDryRun, "dry-run", false, "Print request without calling API")

	// --- remix-strong ---
	mjRemixStrongCmd.Flags().StringVar(&mjTaskID, "task-id", "", "Parent v8/v8.1 task ID (required)")
	mjRemixStrongCmd.Flags().IntVar(&mjIndex, "index", 0, "Tile index (1-4, required)")
	mjRemixStrongCmd.Flags().StringVarP(&mjPrompt, "prompt", "p", "", "New prompt (inherits parent if empty)")
	mjRemixStrongCmd.Flags().StringVar(&mjSpeed, "speed", "", "Speed: relax (default), fast, turbo")
	mjRemixStrongCmd.Flags().StringVar(&mjJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	mjRemixStrongCmd.Flags().BoolVar(&mjDryRun, "dry-run", false, "Print request without calling API")

	// --- remix-subtle ---
	mjRemixSubtleCmd.Flags().StringVar(&mjTaskID, "task-id", "", "Parent v8/v8.1 task ID (required)")
	mjRemixSubtleCmd.Flags().IntVar(&mjIndex, "index", 0, "Tile index (1-4, required)")
	mjRemixSubtleCmd.Flags().StringVarP(&mjPrompt, "prompt", "p", "", "New prompt (inherits parent if empty)")
	mjRemixSubtleCmd.Flags().StringVar(&mjSpeed, "speed", "", "Speed: relax (default), fast, turbo")
	mjRemixSubtleCmd.Flags().StringVar(&mjJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	mjRemixSubtleCmd.Flags().BoolVar(&mjDryRun, "dry-run", false, "Print request without calling API")

	// --- Add subcommands to parent ---
	midjourneyCmd.AddCommand(mjImagineCmd)
	midjourneyCmd.AddCommand(mjBlendCmd)
	midjourneyCmd.AddCommand(mjDescribeCmd)
	midjourneyCmd.AddCommand(mjEditsCmd)
	midjourneyCmd.AddCommand(mjUpscaleCmd)
	midjourneyCmd.AddCommand(mjVariationCmd)
	midjourneyCmd.AddCommand(mjHighVariationCmd)
	midjourneyCmd.AddCommand(mjLowVariationCmd)
	midjourneyCmd.AddCommand(mjRerollCmd)
	midjourneyCmd.AddCommand(mjZoomCmd)
	midjourneyCmd.AddCommand(mjPanCmd)
	midjourneyCmd.AddCommand(mjInpaintCmd)
	midjourneyCmd.AddCommand(mjModalCmd)
	midjourneyCmd.AddCommand(mjVideoCmd)
	midjourneyCmd.AddCommand(mjRemixStrongCmd)
	midjourneyCmd.AddCommand(mjRemixSubtleCmd)
	midjourneyCmd.AddCommand(mjQueryCmd)

	// Silence usage on all MJ subcommands — errors are runtime API failures, not bad CLI args
	for _, sub := range midjourneyCmd.Commands() {
		sub.SilenceUsage = true
	}

	// --- Register parent command ---
	rootCmd.AddCommand(midjourneyCmd)
}
