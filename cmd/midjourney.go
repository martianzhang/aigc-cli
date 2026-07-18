package cmd

import (
	"sync"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/client"
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
// Parent command: aigc-cli midjourney
// ============================================================================
var midjourneyCmd = &cobra.Command{
	Use:          "midjourney",
	Aliases:      []string{"mj"},
	Short:        "Midjourney image generation (also: mj)",
	SilenceUsage: true,
	Long: `Generate and edit images via Midjourney on APIMart.

Midjourney uses an async task model — you submit a job, get a task_id,
then poll for results. All MJ endpoints are under /v1/midjourney/.

Alias: mj (e.g. "aigc-cli mj imagine ...")

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
  aigc-cli midjourney imagine --prompt "a cute cat --ar 16:9"
  aigc-cli mj imagine --prompt "a cute cat"  # same with alias
  aigc-cli midjourney blend --image-url a.png --image-url b.png
  aigc-cli midjourney upscale --task-id task_xxx --index 1
  aigc-cli midjourney query task_xxx`,
}

// ============================================================================
// Subcommand: imagine
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
