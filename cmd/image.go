package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/service"
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
	Aliases:      []string{"img"},
	Short:        "Generate images (also: img)",
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
