package cmd

import (
	"github.com/spf13/cobra"
)

// Image-specific flag variables
var (
	genPrompt       string
	genSize         string
	genRatio        string
	genResolution   string
	genQuality      string
	genBackground   string
	genModeration   string
	genOutputFormat string
	genCompression  int
	genCompress     string // compress target: "800KB", "2MB", "85%"
	genN            int
	genImageURLs    []string
	genMaskURL      string
	genStyle        string
	genResponseFmt  string
	genDryRun       bool
	genEdit         bool // Grok Imagine 1.5 edit mode
	genPreview      bool
	genDecode       bool // decode base64 text files / convert image format for --image-url
)

// registerImageGenerateFlags adds the image generation flags to a command.
func registerImageGenerateFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&genPrompt, "prompt", "p", "", "Text description (auto-reads from file if path exists, or \"-\" for stdin)")
	f.StringVarP(&genSize, "size", "s", "", `Aspect ratio (e.g. "16:9", "1:1") or pixel dims (e.g. "1024x1024") or tier (e.g. "1K", "2K" for Agnes 2.1)`)
	f.StringVar(&genRatio, "ratio", "", `Aspect ratio for tiered sizing (Agnes 2.1+): "1:1", "16:9", "3:4", "4:3", "9:16", "2:3", "3:2", "21:9"`)
	f.StringVarP(&genResolution, "resolution", "r", "", "Resolution tier: 1k, 2k, 4k (APIMart only)")
	f.StringVarP(&genQuality, "quality", "q", "", "Quality: auto, low, medium, high")
	f.StringVar(&genBackground, "background", "", "Background mode: auto, opaque, transparent")
	f.StringVar(&genModeration, "moderation", "", "Moderation strength: auto, low (APIMart only)")
	f.StringVarP(&genOutputFormat, "output-format", "f", "", `Output format: png, jpeg, webp, avif, jxl; with --decode also: base64 (pure base64 text), datauri (data:<mime>;base64,...)`)
	f.StringVarP(&genCompress, "compress", "z", "", `Compress output: target size ("800KB", "2MB") or fixed quality ("85%")`)
	f.IntVar(&genCompression, "output-compression", 0, "Output compression level 0-100 (jpeg/webp only) (APIMart only)")
	f.IntVar(&genN, "n", 0, "Number of images to generate (1-4)")
	f.StringArrayVarP(&genImageURLs, "image-url", "i", nil, "Image input: URL or local file path (repeatable)")
	f.BoolVarP(&genDecode, "decode", "d", false, "Decode base64 text files (data URI / raw base64) in --image-url to real images; combine with --output-format to convert format")
	f.StringVar(&genMaskURL, "mask-url", "", "Mask image URL for inpainting (APIMart only)")
	f.StringVar(&genStyle, "style", "", "Image style: vivid, natural (OpenAI only)")
	f.StringVar(&genResponseFmt, "response-format", "", "Response format: url, b64_json (OpenAI/OpenRouter/Agnes)")
	f.BoolVar(&genDryRun, "dry-run", false, "Print request parameters without calling API")
	f.BoolVar(&genEdit, "edit", false, "Grok Imagine 1.5 Edit mode (requires --image-url)")
	f.BoolVar(&genPreview, "preview", false, "Open generated image with system default viewer")
	f.StringVar(&shared.JSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	f.StringVar(&shared.Mode, "mode", "", "Generation mode: auto (detect), sync, async (default: auto)")
	f.BoolVar(&shared.SavePrompt, "save-prompt", false, "save prompt to .md file alongside results")
}
