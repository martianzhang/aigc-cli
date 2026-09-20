package image

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
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

Decode mode (--decode):
  --image-url also accepts base64 text files (data URI or raw base64) and
  inline data: URIs; --decode converts them to real images before upload.
  Without --prompt, --decode runs purely locally: decode/convert files and
  save them to the output dir (no API call). Combine with --output-format
  to convert image format (e.g. jpg → png).

Examples:
  aigc-cli image --prompt "A cat under starry sky"
  aigc-cli image --prompt prompt.txt --size "16:9"
  echo "..." | aigc-cli image --prompt -
  aigc-cli image --json request.json
  aigc-cli image --json '{"prompt":"a red fox","n":4}'
  aigc-cli image --edit --prompt "Change background to starry sky" --image-url photo.jpg
  aigc-cli image --edit --model "grok-imagine-1.5-edit-apimart" --prompt "Cyberpunk style" --image-url img.png --n 2
  aigc-cli image --decode --image-url image.txt
  aigc-cli image --decode --output-format png --image-url image.txt
  aigc-cli image --edit --decode --image-url image.txt --prompt "Make it cinematic"`,
	RunE: runImageGenerate,
}

func runImageGenerate(cmd *cobra.Command, args []string) error {
	// ----- Pure local compression mode (must be before buildImageRequest) -----
	// When --compress is set and no --prompt, skip API and compress local files directly.
	if genCompress != "" && genPrompt == "" {
		return runLocalCompress(genCompress, genImageURLs, genOutputFormat)
	}

	// ----- Pure local decode mode (must be before buildImageRequest) -----
	// When --decode is set and no --prompt, skip API and decode/convert local files.
	if genDecode && genPrompt == "" {
		return runLocalDecode(genImageURLs, genOutputFormat)
	}

	// ----- Step 1: Build the request -----
	req, err := buildImageRequest(cmd)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	// ----- Step 2: Merge config defaults -----
	if cfg, err := config.LoadDefaults(options.Shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
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
	if err := req.ValidateBackground(); err != nil {
		return err
	}

	// ----- Resolve provider (named provider > global > builtin) -----
	p := options.Shared.ResolveProvider(options.ProviderNameImage)
	isAPIMart := p.ProviderType == provider.APIMart
	isOpenRouter := p.ProviderType == provider.OpenRouter
	isAgnes := p.ProviderType == provider.Agnes
	isOllama := p.Type == types.ProviderOllama || provider.IsLocalEndpoint(p.BaseURL)
	isModelScope := p.ProviderType == provider.ModelScope
	isGemini := p.ProviderType == provider.Gemini
	isZeekai := p.ProviderType == provider.Zeekai

	// Strip APIMart-only fields for non-APIMart providers (e.g., Yunwu, OpenAI, Generic Relay).
	// `resolution` is an APIMart proprietary field not part of the OpenAI image API;
	// sending it to OpenAI-compatible providers can cause errors.
	// `output_format`, `background`, `moderation` are standard OpenAI parameters — kept as-is.
	if !isAPIMart {
		req.Resolution = ""
	}

	// ----- Step 4.5: --decode input preprocessing (provider-agnostic) -----
	// Convert base64 text files / data URIs in --image-url into inline data
	// URIs accepted by OpenAI-compatible APIs. Real image files and remote
	// URLs pass through to the existing upload path unchanged.
	// Runs before dry-run so the printed curl reflects the real request.
	if genDecode {
		decoded, err := service.DecodeImageURLsInline(req.ImageURLs, genOutputFormat)
		if err != nil {
			return fmt.Errorf("failed to decode image-urls: %w", err)
		}
		req.ImageURLs = decoded
		if req.MaskURL != "" {
			decodedMask, err := service.DecodeImageURLsInline([]string{req.MaskURL}, genOutputFormat)
			if err != nil {
				return fmt.Errorf("failed to decode mask-url: %w", err)
			}
			req.MaskURL = decodedMask[0]
		}
	}

	if genDryRun {
		fmt.Println(buildImageCurl(req, p))
		return nil
	}

	// ----- Edit mode checks -----
	if genEdit {
		if len(req.ImageURLs) == 0 {
			return fmt.Errorf("--image-url is required in edit mode")
		}
		if !isAPIMart {
			return fmt.Errorf("edit mode requires an APIMart provider (apimart.ai / apib.ai / aiuxu.com / aishuch.com)")
		}
	}

	// ----- Step 4: Resolve local image files (upload if needed) -----
	c := client.NewFromProvider(p)
	options.ApplyTimeout(c, "image", client.ImageTimeout)

	if err := resolveRequestImages(c, req, isAPIMart); err != nil {
		return err
	}

	// ----- Step 5: Print the request payload (verbose only) -----
	// Printed after image resolution so the dump matches what is actually sent
	// (uploaded URLs for APIMart, data URIs elsewhere) rather than raw local paths.
	if options.Shared.Verbose {
		_, body, _ := imageWireRequest(req, p)
		prettyReq, _ := json.MarshalIndent(body, "", "  ")
		fmt.Printf("Request:\n%s\n\n", string(prettyReq))
	}

	// Strategy table: first match wins, last entry is the default.
	ictx := &imageDispatchCtx{
		isAPIMart:     isAPIMart,
		isOpenRouter:  isOpenRouter,
		isModelScope:  isModelScope,
		isAgnes:       isAgnes,
		isGemini:      isGemini,
		isZeekai:      isZeekai,
		genEdit:       genEdit,
		isOllama:      isOllama,
		modelScopeKey: p.APIKey,
	}
	for _, s := range imageStrategies {
		if s.match(req, ictx) {
			saved, err := s.run(c, req, ictx)
			if err == nil && genPreview {
				for _, f := range saved {
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

func init() {
	registerImageGenerateFlags(imageCmd)
}

// Cmd returns the image command tree.
func Cmd() *cobra.Command { return imageCmd }
