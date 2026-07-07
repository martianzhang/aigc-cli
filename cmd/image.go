package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/martianzhang/apimart-cli/internal/provider"
	"github.com/martianzhang/apimart-cli/internal/service"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// Image-specific flag variables
var (
	genPrompt       string
	genSize         string
	genResolution   string
	genQuality      string
	genBackground   string
	genModeration   string
	genOutputFormat string
	genCompression  int
	genN            int
	genImageURLs    []string
	genMaskURL      string
	genStyle        string
	genResponseFmt  string
	genDryRun       bool
	genEdit         bool // Grok Imagine 1.5 edit mode
	genPreview      bool
)

// imageCmd represents the `aigc-cli image` command.
var imageCmd = &cobra.Command{
	Use:          "image",
	Short:        "Generate images (supports OpenAI sync & APIMart async)",
	SilenceUsage: true,
	Long: `Generate images via any OpenAI-compatible API.

Supports text-to-image, image-to-image, inpainting, and Grok image editing.
Works with OpenAI, OpenRouter (sync), and APIMart (async task-based).

You can specify parameters via flags, or pass a complete JSON request
via the --json flag (file path, JSON string, or "-" for stdin).

Edit mode (--edit):
  Grok Imagine 1.5 Edit edits images based on a source image + prompt.
  Requires --edit + --image-url + --prompt, forces async mode.
  Model defaults to grok-imagine-1.5-edit-apimart.

Examples:
  aigc-cli image --prompt "A cat under starry sky"
  aigc-cli image --prompt prompt.txt --size "16:9"
  echo "..." | aigc-cli image --prompt -
  aigc-cli image --json request.json
  aigc-cli image --json '{"prompt":"a red fox","n":4}'
  aigc-cli image --edit --prompt "Change background to starry sky" --image-url photo.jpg
  aigc-cli image --edit --model "grok-imagine-1.5-edit-apimart" --prompt "Cyberpunk style" --image-url img.png --n 2`,
	RunE: runImageGenerate,
}

func runImageGenerate(cmd *cobra.Command, args []string) error {
	// ----- Step 1: Build the request -----
	req, err := buildImageRequest(cmd)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	// ----- Step 2: Merge config defaults -----
	if cfg, err := config.LoadDefaults(shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
		cfg.Defaults.Image.MergeIntoImage(req)
	}

	// ----- Step 3: Apply defaults for remaining empty fields -----
	if req.Model == "" {
		if genEdit {
			req.Model = "grok-imagine-1.5-edit-apimart"
		} else {
			return fmt.Errorf("model is required: set via --model flag or defaults.image.model in config.yaml")
		}
	}
	if req.Size == "" && !genEdit {
		req.Size = "1:1"
	}
	if req.Quality == "" && !genEdit {
		req.Quality = "auto"
	}
	if req.OutputFormat == "" && !genEdit {
		req.OutputFormat = "png"
	}

	if genDryRun {
		curl := buildImageCurl(req)
		fmt.Println(curl)
		return nil
	}

	// ----- Edit mode checks -----
	if genEdit {
		if len(req.ImageURLs) == 0 {
			return fmt.Errorf("--image-url is required in edit mode")
		}
		if !isAPIMartProvider() {
			return fmt.Errorf("edit mode requires an APIMart provider (apimart.ai / apib.ai / aiuxu.com / aishuch.com)")
		}
	}

	// ----- Step 4: Print the request payload (verbose only) -----
	if shared.Verbose {
		prettyReq, _ := json.MarshalIndent(req, "", "  ")
		fmt.Printf("Request:\n%s\n\n", string(prettyReq))
	}

	// ----- Step 5: Resolve local image files (upload if needed) -----
	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
	applyTimeout(c, "image", client.ImageTimeout)

	if isAPIMartProvider() {
		if len(req.ImageURLs) > 0 {
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
		}
		if req.MaskURL != "" {
			resolved, err := c.ResolveLocalImages([]string{req.MaskURL})
			if err != nil {
				return fmt.Errorf("failed to resolve mask-url: %w", err)
			}
			req.MaskURL = resolved[0]
		}
	}

	// Strategy table: first match wins, last entry is the default.
	ictx := &imageDispatchCtx{
		isAPIMart:    isAPIMartProvider(),
		isOpenRouter: isOpenRouterProvider(),
		genEdit:      genEdit,
	}
	for _, s := range imageStrategies {
		if s.match(req, ictx) {
			err := s.run(c, req)
			if err == nil && genPreview {
				previewSavedFiles = previewLatestFiles("image_")
				for _, f := range previewSavedFiles {
					if e := service.PreviewFile(f); e != nil {
						fmt.Fprintf(os.Stderr, "Warning: preview failed: %v\n", e)
					}
				}
			}
			return err
		}
	}
	return nil
}

// imageDispatchCtx holds provider/mode context for image strategy matching.
// Built from local variables in runImageGenerate, not global state.
type imageDispatchCtx struct {
	isAPIMart    bool
	isOpenRouter bool
	genEdit      bool
}

// imageStrategy defines a dispatch rule for image generation.
type imageStrategy struct {
	match func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool
	run   func(client.APIClient, *types.GenerateRequest) error
}

// imageStrategies is the ordered dispatch table for image generation.
// First match wins. Add a new entry here when adding a new provider or model type.
var imageStrategies = []imageStrategy{
	{
		// OpenRouter: all image models → Unified Image API (POST /v1/images)
		match: func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool {
			return ctx.isOpenRouter && !ctx.genEdit
		},
		run: runOpenRouterDedicatedImage,
	},
	{
		// APIMart: async task-based generation
		match: func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool {
			return ctx.isAPIMart
		},
		run: runAsyncImage,
	},
	// Default: OpenAI-compatible synchronous generation
	{
		match: func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool { return true },
		run:   runSyncImage,
	},
}

// runOpenRouterDedicatedImage handles image generation via OpenRouter's
// dedicated Image API (POST /v1/images). Used for GPT Image, DALL-E, and
// most dedicated image models on OpenRouter. Returns standard OpenAI-compatible
// response with b64_json images.
func runOpenRouterDedicatedImage(c client.APIClient, req *types.GenerateRequest) error {
	start := time.Now()

	orResp, err := c.OpenRouterDedicatedImage(req)
	if err != nil {
		return fmt.Errorf("OpenRouter image generation failed: %w", err)
	}

	elapsed := time.Since(start)

	fmt.Printf("Model: %s\n", req.Model)
	if orResp.Created > 0 {
		fmt.Printf("Created: %s\n", time.Unix(orResp.Created, 0).Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())

	for i, img := range orResp.Data {
		// Save base64 image
		if img.B64JSON != "" {
			prefix := fmt.Sprintf("image_%d", time.Now().Unix())
			filename, err := service.SaveBase64Image(shared.OutputDir, prefix, img.B64JSON, i)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d saved: %s\n", i+1, filename)
		} else if img.URL != "" {
			// Download from URL
			body, err := httpGet(img.URL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i, err)
				continue
			}
			ext := filepath.Ext(img.URL)
			if ext == "" {
				ext = ".png"
			}
			ts := time.Now().Unix()
			filename := filepath.Join(shared.OutputDir, fmt.Sprintf("image_%d_%d%s", ts, i, ext))
			if err := os.WriteFile(filename, body, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, img.URL)
			fmt.Printf("Saved: %s\n", filename)
		}
		if img.RevisedPrompt != "" {
			fmt.Printf("  Revised prompt: %s\n", img.RevisedPrompt)
		}
	}

	// Show usage / cost
	if orResp.Usage != nil {
		parts := []string{}
		if orResp.Usage.PromptTokens > 0 {
			parts = append(parts, fmt.Sprintf("%d in", orResp.Usage.PromptTokens))
		}
		if orResp.Usage.CompletionTokens > 0 {
			parts = append(parts, fmt.Sprintf("%d out", orResp.Usage.CompletionTokens))
		}
		if orResp.Usage.TotalTokens > 0 {
			parts = append(parts, fmt.Sprintf("%d total", orResp.Usage.TotalTokens))
		}
		tokenStr := ""
		if len(parts) > 0 {
			tokenStr = strings.Join(parts, " / ")
		}
		if tokenStr != "" || orResp.Usage.Cost > 0 {
			if tokenStr != "" {
				fmt.Printf("Tokens: %s", tokenStr)
			}
			if orResp.Usage.Cost > 0 {
				if tokenStr != "" {
					fmt.Printf(" | ")
				}
				fmt.Printf("Cost: $%.5f", orResp.Usage.Cost)
			}
			fmt.Println()
		}
	}

	return nil
}

// runSyncImage handles OpenAI/OpenRouter-compatible synchronous image generation.
func runSyncImage(c client.APIClient, req *types.GenerateRequest) error {
	start := time.Now()

	syncResp, err := c.ImageGenerateSync(req)
	if err != nil {
		return fmt.Errorf("image generation failed: %w", err)
	}

	elapsed := time.Since(start)

	fmt.Printf("Model: %s\n", req.Model)
	if syncResp.Created > 0 {
		fmt.Printf("Created: %s\n", time.Unix(syncResp.Created, 0).Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())
	for i, img := range syncResp.Data {
		// Save base64 image data
		if img.B64JSON != "" {
			taskID := fmt.Sprintf("image_sync_%d", syncResp.Created)
			filename, err := service.SaveBase64Image(shared.OutputDir, taskID, img.B64JSON, i)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
		} else if img.URL != "" {
			// Download from URL
			body, err := httpGet(img.URL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i, err)
				continue
			}
			ext := filepath.Ext(img.URL)
			if ext == "" {
				ext = ".png"
			}
			taskID := fmt.Sprintf("sync_%d", syncResp.Created)
			filename := filepath.Join(shared.OutputDir, fmt.Sprintf("image_%s_%d%s", taskID, i, ext))
			if err := os.WriteFile(filename, body, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
		} else {
			fmt.Printf("Image %d: <no data>\n", i+1)
			continue
		}
		if img.RevisedPrompt != "" {
			fmt.Printf("  Revised prompt: %s\n", img.RevisedPrompt)
		}
	}

	if syncResp.Usage != nil {
		parts := []string{}
		if syncResp.Usage.PromptTokens > 0 {
			parts = append(parts, fmt.Sprintf("%d in", syncResp.Usage.PromptTokens))
		}
		if syncResp.Usage.CompletionTokens > 0 {
			parts = append(parts, fmt.Sprintf("%d out", syncResp.Usage.CompletionTokens))
		}
		if syncResp.Usage.TotalTokens > 0 {
			parts = append(parts, fmt.Sprintf("%d total", syncResp.Usage.TotalTokens))
		}
		tokenStr := ""
		if len(parts) > 0 {
			tokenStr = strings.Join(parts, " / ")
		}
		if tokenStr != "" || syncResp.Usage.Cost > 0 {
			if tokenStr != "" {
				fmt.Printf("Tokens: %s", tokenStr)
			}
			if syncResp.Usage.Cost > 0 {
				if tokenStr != "" {
					fmt.Printf(" | ")
				}
				fmt.Printf("Cost: $%.5f", syncResp.Usage.Cost)
			}
			fmt.Println()
		}
	}

	return nil
}

// runAsyncImage handles APIMart-compatible asynchronous (task-based) image generation.
func runAsyncImage(c client.APIClient, req *types.GenerateRequest) error {
	resp, err := c.Submit(req)
	if err != nil {
		return fmt.Errorf("submission failed: %w", err)
	}

	if len(resp.Data) == 0 {
		return fmt.Errorf("submission returned no tasks")
	}

	task := resp.Data[0]
	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Response code: %d\n", resp.Code)
	fmt.Printf("Task ID: %s\n", task.TaskID)
	fmt.Printf("Status: %s\n\n", task.Status)

	fmt.Println("Polling for completion...")
	taskData, err := c.PollTask(task.TaskID)
	if err != nil {
		return fmt.Errorf("polling failed: %w", err)
	}

	if shared.Verbose {
		prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
		fmt.Printf("\nTask result:\n%s\n", string(prettyResult))
	}

	fmt.Println()
	savePromptFile(taskData.ID, req.Prompt)

	if taskData.Result != nil && len(taskData.Result.Images) > 0 {
		if _, err := downloadImages(taskData.Result.Images, taskData.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
		}
	}

	fmt.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)
	return nil
}

// isOpenRouterProvider determines whether the current base URL points to OpenRouter.
func isOpenRouterProvider() bool {
	return provider.IsOpenRouter(shared.APIBase)
}

// isAPIMartProvider determines whether to use APIMart async mode.
// Known APIMart domains: apimart.ai, apib.ai, aiuxu.com, aishuch.com
// Known sync domains: openai.com, openrouter.ai
// All other domains default to sync (OpenAI-compatible relay).
func isAPIMartProvider() bool {
	switch shared.Mode {
	case "async":
		return true
	case "sync":
		return false
	default: // auto — detect from base URL
		base := shared.APIBase
		if base == "" {
			base = "https://api.apimart.ai"
		}
		return provider.IsAPIMart(base)
	}
}

// buildImageRequest constructs a GenerateRequest from --json or individual flags.
func buildImageRequest(cmd *cobra.Command) (*types.GenerateRequest, error) {
	if shared.JSONInput != "" {
		return parseJSONInput()
	}

	prompt, err := resolvePrompt()
	if err != nil {
		return nil, err
	}

	req := &types.GenerateRequest{
		Model:          shared.Model,
		Prompt:         prompt,
		Size:           genSize,
		Resolution:     genResolution,
		Quality:        genQuality,
		Background:     genBackground,
		Moderation:     genModeration,
		OutputFormat:   genOutputFormat,
		ImageURLs:      genImageURLs,
		MaskURL:        genMaskURL,
		Style:          genStyle,
		ResponseFormat: genResponseFmt,
	}

	if cmd.Flags().Changed("output-compression") {
		v := genCompression
		req.OutputCompression = &v
	}
	if cmd.Flags().Changed("n") {
		v := genN
		req.N = &v
	}

	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required (use --prompt or --json)")
	}

	return req, nil
}

// buildImageCurl generates an equivalent curl command for an image generation request.
func buildImageCurl(req *types.GenerateRequest) string {
	body, _ := json.Marshal(req)
	base := shared.APIBase
	if base == "" {
		base = "https://api.apimart.ai/v1" // matches client.defaultBaseURL
	}
	base = strings.TrimRight(base, "/")
	url := base + "/images/generations"

	cmd := fmt.Sprintf("curl -X POST %s \\\n", url)
	cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", shared.APIKey)
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(body))
	return cmd
}

// registerImageGenerateFlags adds the image generation flags to a command.
func registerImageGenerateFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&genPrompt, "prompt", "p", "", "Text description (auto-reads from file if path exists, or \"-\" for stdin)")
	f.StringVarP(&genSize, "size", "s", "", `Aspect ratio (e.g. "16:9", "1:1") or pixel dims (e.g. "1024x1024")`)
	f.StringVarP(&genResolution, "resolution", "r", "", "Resolution tier: 1k, 2k, 4k (APIMart only)")
	f.StringVarP(&genQuality, "quality", "q", "", "Quality: auto, low, medium, high")
	f.StringVar(&genBackground, "background", "", "Background mode: auto, opaque, transparent (APIMart only)")
	f.StringVar(&genModeration, "moderation", "", "Moderation strength: auto, low (APIMart only)")
	f.StringVarP(&genOutputFormat, "output-format", "f", "", "Output format: png, jpeg, webp")
	f.IntVar(&genCompression, "output-compression", 0, "Output compression level 0-100 (jpeg/webp only) (APIMart only)")
	f.IntVar(&genN, "n", 0, "Number of images to generate (1-4)")
	f.StringArrayVar(&genImageURLs, "image-url", nil, "Reference image URL (repeatable) (APIMart only)")
	f.StringVar(&genMaskURL, "mask-url", "", "Mask image URL for inpainting (APIMart only)")
	f.StringVar(&genStyle, "style", "", "Image style: vivid, natural (OpenAI only)")
	f.StringVar(&genResponseFmt, "response-format", "", "Response format: url, b64_json (OpenAI/OpenRouter)")
	f.BoolVar(&genDryRun, "dry-run", false, "Print request parameters without calling API")
	f.BoolVar(&genEdit, "edit", false, "Grok Imagine 1.5 Edit mode (requires --image-url)")
	f.BoolVar(&genPreview, "preview", false, "Open generated image with system default viewer")
	f.StringVar(&shared.JSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	f.StringVar(&shared.Mode, "mode", "", "Generation mode: auto (detect), sync, async (default: auto)")
	f.BoolVar(&shared.SavePrompt, "save-prompt", false, "save prompt to .md file alongside results")
}

func init() {
	registerImageGenerateFlags(imageCmd)
	rootCmd.AddCommand(imageCmd)
}

// The following helpers are shared across image commands and aliases.

// resolvePrompt resolves the prompt text from --prompt flag.
// Defaults to stdin when --prompt is not specified.
func resolvePrompt() (string, error) {
	input := genPrompt
	if input == "" {
		input = "-"
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

// readInput reads content from a file path, stdin ("-"), or returns the raw string.
func readInput(input string) ([]byte, error) {
	switch {
	case input == "-":
		// Don't block if stdin is a terminal with no piped input
		stat, err := os.Stdin.Stat()
		if err == nil && (stat.Mode()&os.ModeCharDevice) != 0 {
			return nil, fmt.Errorf("stdin is a terminal — pipe input or use --prompt")
		}
		return io.ReadAll(os.Stdin)
	case isFile(input):
		return os.ReadFile(input)
	default:
		return []byte(input), nil
	}
}

// parseJSONInput reads JSON from file path, string literal, or stdin.
func parseJSONInput() (*types.GenerateRequest, error) {
	data, err := readInput(shared.JSONInput)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON input: %w", err)
	}

	req := &types.GenerateRequest{}
	if err := json.Unmarshal(data, req); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required in JSON input")
	}

	return req, nil
}

// applyTimeout sets the HTTP client timeout from CLI flag / config, falling back to modDefault.
// Priority: --timeout flag > defaults.{mod}.timeout > timeout > modDefault.
func applyTimeout(c client.APIClient, modKey string, modDefault time.Duration) {
	d := modDefault
	// 1. CLI --timeout flag (global override)
	if shared.TimeoutFlag > 0 {
		d = time.Duration(shared.TimeoutFlag) * time.Second
		c.SetTimeout(d)
		return
	}
	// 2. Config file
	if cfg, err := config.LoadDefaults(shared.CfgFile); err == nil && cfg != nil {
		var modTimeout *int
		if cfg.Defaults != nil {
			switch modKey {
			case "image":
				if cfg.Defaults.Image != nil {
					modTimeout = cfg.Defaults.Image.Timeout
				}
			case "video":
				if cfg.Defaults.Video != nil {
					modTimeout = cfg.Defaults.Video.Timeout
				}
			case "midjourney":
				if cfg.Defaults.Midjourney != nil {
					modTimeout = cfg.Defaults.Midjourney.Timeout
				}
			}
		}
		if modTimeout != nil && *modTimeout > 0 {
			d = time.Duration(*modTimeout) * time.Second
		} else if cfg.Timeout != nil && *cfg.Timeout > 0 {
			d = time.Duration(*cfg.Timeout) * time.Second
		}
	}
	c.SetTimeout(d)
}

// isFile returns true if the given path points to an existing file.
func isFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// downloadImages downloads all generated images to the output directory.
// Returns paths to saved files.
func downloadImages(images []types.ImageResult, taskID string) ([]string, error) {
	var saved []string
	for i, img := range images {
		for j, url := range img.URL {
			data, err := service.FetchImage(url)
			if err != nil {
				// Save raw data as text file for manual recovery
				prefix := fmt.Sprintf("image_%s_%d_%d", taskID, i, j)
				service.SaveBase64Fallback(shared.OutputDir, prefix, url, 0)
				continue
			}

			ext := filepath.Ext(url)
			if ext == "" {
				ext = ".png"
			}
			filename := filepath.Join(shared.OutputDir, fmt.Sprintf("image_%s_%d_%d%s", taskID, i, j, ext))
			if err := os.WriteFile(filename, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
				continue
			}
			fmt.Printf("Saved: %s\n", filename)
			saved = append(saved, filename)
		}
	}
	return saved, nil
}

// savePromptFile saves the generation prompt to image_{taskID}.md.
func savePromptFile(taskID, prompt string) {
	if !shared.SavePrompt {
		return
	}
	service.SavePrompt(shared.OutputDir, taskID, prompt)
}

// loadImageDefaults returns the user's image config defaults.
// Tries shared.Cfg first (fast), falls back to reading from file.
func loadImageDefaults() *types.ImageDefaults {
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Image != nil {
		return shared.Cfg.Defaults.Image
	}
	// Fallback: load from file directly
	if cfg, err := config.Load(shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
		return cfg.Defaults.Image
	}
	return nil
}

// httpGet performs an HTTP GET or resolves a data URI / base64 string.
func httpGet(rawURL string) ([]byte, error) {
	return service.FetchImage(rawURL)
}

// generateImageAndSave generates images via the configured provider and saves them to disk.
// Handles config merge, timeout, API dispatch, and download. Returns paths to saved files.
// Shared by CLI (image command) and agent loop (chat) — single source of truth.
// Supports APIMart async and OpenAI-compatible sync providers.
func generateImageAndSave(c client.APIClient, req *types.GenerateRequest) ([]string, error) {
	// Always load the user's config — shared.Cfg may be nil if PersistentPreRunE
	// hasn't run (e.g., direct call from agent loop without CLI entry).
	imgCfg := loadImageDefaults()

	if shared.Verbose {
		if imgCfg != nil {
			fmt.Fprintf(os.Stderr, "\r\n[image] cfg: model=%s quality=%s size=%s res=%s\r\n",
				imgCfg.Model, imgCfg.Quality, imgCfg.Size, imgCfg.Resolution)
		} else {
			fmt.Fprintf(os.Stderr, "\r\n[image] WARNING: no image config loaded (check defaults.image in config.yaml)\r\n")
		}
		fmt.Fprintf(os.Stderr, "[image] req before: model=%s quality=%s size=%s res=%s\r\n",
			req.Model, req.Quality, req.Size, req.Resolution)
	}

	// Check if LLM is allowed to override config (default: false = config wins)
	allowOverride := false
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Chat != nil {
		allowOverride = shared.Cfg.Defaults.Chat.AllowToolOverride
	}

	if imgCfg != nil {
		if !allowOverride {
			// Config is the ceiling — force values regardless of LLM
			if imgCfg.Model != "" {
				req.Model = imgCfg.Model
			}
			if imgCfg.Quality != "" {
				req.Quality = imgCfg.Quality
			}
			if imgCfg.Size != "" {
				req.Size = imgCfg.Size
			}
			if imgCfg.Resolution != "" {
				req.Resolution = imgCfg.Resolution
			}
		} else {
			// allow_tool_override=true — LLM takes priority, config only fills empty fields
			imgCfg.MergeIntoImage(req)
		}
	}
	// Code defaults for fields the user didn't configure
	if req.Size == "" {
		req.Size = "1:1"
	}
	if req.Quality == "" {
		req.Quality = "low"
	}
	if req.Resolution == "" {
		req.Resolution = "1k"
	}
	if shared.Verbose {
		fmt.Fprintf(os.Stderr, "[image] req after:  model=%s quality=%s size=%s res=%s\r\n",
			req.Model, req.Quality, req.Size, req.Resolution)
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model is required: set via defaults.image.model in config.yaml")
	}

	// Set timeout
	applyTimeout(c, "image", client.ImageTimeout)

	// Dispatch based on provider
	if isAPIMartProvider() {
		resp, err := c.Submit(req)
		if err != nil {
			return nil, fmt.Errorf("submission failed: %w", err)
		}
		if len(resp.Data) == 0 {
			return nil, fmt.Errorf("submission returned no tasks")
		}

		taskData, err := c.PollTask(resp.Data[0].TaskID)
		if err != nil {
			return nil, fmt.Errorf("polling failed: %w", err)
		}

		savePromptFile(taskData.ID, req.Prompt)
		if taskData.Result != nil && len(taskData.Result.Images) > 0 {
			saved, err := downloadImages(taskData.Result.Images, taskData.ID)
			if err != nil {
				return saved, err
			}
			return saved, nil
		}
		return nil, fmt.Errorf("no images in task result")
	}

	// OpenAI-compatible sync
	resp, err := c.ImageGenerateSync(req)
	if err != nil {
		return nil, fmt.Errorf("image generation failed: %w", err)
	}

	var saved []string
	for i, img := range resp.Data {
		if img.B64JSON != "" {
			taskID := fmt.Sprintf("image_sync_%d", resp.Created)
			filename, saveErr := service.SaveBase64Image(shared.OutputDir, taskID, img.B64JSON, i)
			if saveErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, saveErr)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		} else if img.URL != "" {
			body, getErr := httpGet(img.URL)
			if getErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i, getErr)
				continue
			}
			ext := filepath.Ext(img.URL)
			if ext == "" {
				ext = ".png"
			}
			taskID := fmt.Sprintf("sync_%d", resp.Created)
			filename := filepath.Join(shared.OutputDir, fmt.Sprintf("image_%s_%d%s", taskID, i, ext))
			if writeErr := os.WriteFile(filename, body, 0644); writeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, writeErr)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		}
	}
	if len(saved) == 0 {
		return nil, fmt.Errorf("no images saved")
	}
	return saved, nil
}
