package cmd

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/background"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/spf13/cobra"
)

var backgroundCmd = &cobra.Command{
	Use:          "background <file...>",
	Short:        "Remove or replace image background (chroma key)",
	SilenceUsage: true,
	Long: `Remove or replace the solid-color background from images.

Uses CIELAB ΔE chroma keying — pure Go, no external models or APIs.
Best for AI-generated images with solid or gradient backgrounds.

Examples:
  aigc-cli background input.png --remove
  aigc-cli background input.png --replace "#FF0000"
  aigc-cli background input.png --remove --ac --ar "1:1" --padding 20`,
	RunE: runBackground,
}

var (
	bgRemove        bool
	bgReplace       string
	bgMaskOnly      bool
	bgBGColor       string
	bgTolerance     float64
	bgFeather       int
	bgFGThreshold   float64
	bgSmooth        int
	bgErode         int
	bgClose         int
	bgAutocrop      bool
	bgPadding       string
	bgAspectRatio   string
	bgJSON          bool
	bgPreview       bool
	bgOutput        string
	bgShadow        bool
	bgShadowOffset  string
	bgShadowBlur    int
	bgShadowColor   string
	bgShadowOpacity float64
	bgAI            string // --ai "custom prompt" or --ai (uses default)
)

func runBackground(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no files specified: pass one or more image file paths as arguments")
	}

	if !bgRemove && bgReplace == "" && !bgMaskOnly {
		return fmt.Errorf("specify --remove, --replace, or --mask-only")
	}

	opts := background.Defaults()
	if bgTolerance != 0 {
		opts.Tolerance = bgTolerance
	}
	if cmd.Flags().Changed("feather") || cmd.Flags().Changed("feather-radius") {
		opts.Feather = bgFeather
	}
	if bgFGThreshold != 0 {
		opts.FgThreshold = bgFGThreshold
	}
	if cmd.Flags().Changed("smooth") {
		opts.Smooth = bgSmooth
	}
	if cmd.Flags().Changed("erode") || cmd.Flags().Changed("erode-radius") {
		opts.Erode = bgErode
	}
	if cmd.Flags().Changed("close") || cmd.Flags().Changed("close-radius") {
		opts.Close = bgClose
	}
	opts.Autocrop = bgAutocrop
	if bgAspectRatio != "" {
		opts.AspectRatio = bgAspectRatio
	}
	if bgPadding != "" {
		p, err := background.ParsePadding(bgPadding)
		if err != nil {
			return fmt.Errorf("invalid --padding: %w", err)
		}
		opts.Padding = p
	}

	if bgBGColor != "" {
		c, err := parseHexColor(bgBGColor)
		if err != nil {
			return fmt.Errorf("invalid --bg-color: %w", err)
		}
		opts.BgColor = c
	}

	if bgShadow {
		opts.Shadow = true
		dx, dy, err := parseOffset(bgShadowOffset)
		if err != nil {
			return fmt.Errorf("invalid --shadow-offset: %w", err)
		}
		opts.ShadowOffset = [2]int{dx, dy}
		opts.ShadowBlur = bgShadowBlur
		opts.ShadowOpacity = bgShadowOpacity
		if bgShadowColor != "" {
			c, err := parseHexColor(bgShadowColor)
			if err != nil {
				return fmt.Errorf("invalid --shadow-color: %w", err)
			}
			opts.ShadowColor = c
		}
	}

	// Determine output dir
	outDir := bgOutput
	if outDir == "" {
		outDir = "."
	}

	// Determine replace color or image
	var repColor color.Color
	var repImg image.Image
	var doReplace bool
	if bgReplace != "" {
		doReplace = true
		if strings.HasPrefix(bgReplace, "#") {
			c, err := parseHexColor(bgReplace)
			if err != nil {
				return fmt.Errorf("invalid --replace color: %w", err)
			}
			repColor = c
		} else {
			f, err := os.Open(bgReplace)
			if err != nil {
				return fmt.Errorf("cannot open --replace image %s: %w", bgReplace, err)
			}
			defer f.Close()
			img, _, err := image.Decode(f)
			if err != nil {
				return fmt.Errorf("cannot decode --replace image %s: %w", bgReplace, err)
			}
			repImg = img
		}
	}

	for _, arg := range args {
		target := arg
		if bgAI != "" {
			regenerated, err := aiRegenerate(arg, bgAI, outDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "AI regeneration failed for %s: %v\n", arg, err)
				continue
			}
			target = regenerated
		}
		if err := processOneFile(target, outDir, opts, doReplace, repColor, repImg); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", arg, err)
		}
	}

	return nil
}

func processOneFile(path, outDir string, opts background.Options, doReplace bool, repColor color.Color, repImg image.Image) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	f.Close()

	_ = format

	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	if bgMaskOnly {
		gray, result, err := background.MaskOnly(img, &opts)
		if err != nil {
			return err
		}
		outPath := filepath.Join(outDir, base+"_mask.png")
		if err := background.SavePNG(outPath, gray); err != nil {
			return err
		}
		fmt.Printf("%s -> %s  (tolerance=%.1f)\n", filepath.Base(path), filepath.Base(outPath), result.ToleranceUsed)
		if bgJSON {
			fmt.Printf("  %dx%d", result.Width, result.Height)
			if result.DetectedBgColor != "" {
				fmt.Printf("  bg=%s", result.DetectedBgColor)
			}
			fmt.Println()
		}
		return nil
	}

	if bgRemove && !doReplace {
		outImg, result, err := background.RemoveBackground(img, &opts)
		if err != nil {
			return err
		}
		outPath := filepath.Join(outDir, base+"_removebg.png")
		if err := background.SavePNG(outPath, outImg); err != nil {
			return err
		}
		fmt.Printf("%s -> %s  (tolerance=%.1f", filepath.Base(path), filepath.Base(outPath), result.ToleranceUsed)
		if result.DetectedBgColor != "" {
			fmt.Printf(", bg=%s", result.DetectedBgColor)
		}
		fmt.Printf(")\n")
		if bgPreview {
			service.PreviewFile(outPath)
		}
		return nil
	}

	if doReplace {
		var result *background.Result
		var err error

		var outImg *image.NRGBA
		if repColor != nil {
			outImg, result, err = background.ReplaceColor(img, repColor, &opts)
		} else {
			outImg, result, err = background.ReplaceImage(img, repImg, &opts)
		}
		if err != nil {
			return err
		}

		outPath := filepath.Join(outDir, base+"_replaced.png")
		if err := background.SavePNG(outPath, outImg); err != nil {
			return err
		}
		fmt.Printf("%s -> %s  (tolerance=%.1f", filepath.Base(path), filepath.Base(outPath), result.ToleranceUsed)
		if result.DetectedBgColor != "" {
			fmt.Printf(", bg=%s", result.DetectedBgColor)
		}
		fmt.Printf(")\n")
		if bgPreview {
			service.PreviewFile(outPath)
		}
		return nil
	}

	return nil
}

// parseHexColor parses "#RRGGBB" or "RRGGBB" to color.RGBA.
func parseHexColor(s string) (color.RGBA, error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.RGBA{}, fmt.Errorf("color must be 6-digit hex, got %q", s)
	}
	var r, g, b uint8
	n, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b)
	if err != nil || n != 3 {
		return color.RGBA{}, fmt.Errorf("invalid hex color %q", s)
	}
	return color.RGBA{R: r, G: g, B: b, A: 255}, nil
}

func parseOffset(s string) (int, int, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("offset must be in format \"dx,dy\", got %q", s)
	}
	var dx, dy int
	if _, err := fmt.Sscanf(parts[0], "%d", &dx); err != nil {
		return 0, 0, fmt.Errorf("invalid dx: %s", parts[0])
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &dy); err != nil {
		return 0, 0, fmt.Errorf("invalid dy: %s", parts[1])
	}
	return dx, dy, nil
}

// aiRegenerate sends the input image to the image generation API with a prompt
// to replace the background with a solid color, then downloads the result.
// Returns the path to the downloaded file.
var aiPromptTemplates = map[string]string{
	"default": "Regenerate exactly the same subject on a pure white solid background, no shadows, no gradient, flat uniform lighting, keep all details of the subject identical",
	"white":   "Regenerate exactly the same subject on a pure white solid background (#FFFFFF), studio lighting, no shadows, no gradient, keep all details identical",
	"human":   "Regenerate exactly the same person on a pure white solid background, keep all details of the clothing, hair, and face identical, no shadows, flat lighting",
	"product": "Regenerate exactly the same product on a pure white solid background, studio product photography lighting, no shadows, no reflections on background",
	"good":    "Regenerate the same subject on a clean solid background, well-lit, no shadows or gradients, keep all details identical",
}

func aiRegenerate(inputPath, userPrompt string, outDir string) (string, error) {
	prompt := userPrompt
	if tmpl, ok := aiPromptTemplates[userPrompt]; ok {
		prompt = tmpl
	}

	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot find self path: %w", err)
	}

	// Generate to a temp dir so we can find the exact file,
	// then move it to outDir with a predictable name.
	tmpDir, err := os.MkdirTemp("", "aigc-cli-ai-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	args := []string{"image", "--prompt", prompt, "--image-url", inputPath, "--output", tmpDir}
	if shared.Model != "" {
		args = append(args, "-m", shared.Model)
	}

	cmd := exec.Command(self, args...)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("image generation failed: %w", err)
	}

	// Find the generated PNG (only one should be in the temp dir)
	entries, _ := os.ReadDir(tmpDir)
	var srcPath string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".png") {
			srcPath = filepath.Join(tmpDir, e.Name())
			break
		}
	}
	if srcPath == "" {
		return "", fmt.Errorf("no PNG generated by image command")
	}

	// Move to output directory with _ai suffix
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	dstPath := filepath.Join(outDir, base+"_ai.png")
	if err := os.Rename(srcPath, dstPath); err != nil {
		// Fallback: copy
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			return "", err
		}
	}
	fmt.Printf("AI regenerated: %s\n", filepath.Base(dstPath))
	return dstPath, nil
}

func init() {
	rootCmd.AddCommand(backgroundCmd)

	// Main operation flags
	backgroundCmd.Flags().BoolVar(&bgRemove, "remove", false, "remove background (output transparent PNG)")
	backgroundCmd.Flags().StringVar(&bgReplace, "replace", "", "replace background: hex color (#RRGGBB) or image path")
	backgroundCmd.Flags().BoolVar(&bgMaskOnly, "mask-only", false, "output grayscale alpha mask for debugging")

	// Autocrop flags
	backgroundCmd.Flags().BoolVar(&bgAutocrop, "autocrop", false, "crop to foreground bounding box")
	backgroundCmd.Flags().BoolVar(&bgAutocrop, "ac", false, "shorthand for --autocrop")
	backgroundCmd.Flags().StringVar(&bgPadding, "padding", "", "padding: single value (\"20\") or four values (\"10,20,10,20\": top,right,bottom,left)")
	backgroundCmd.Flags().StringVar(&bgAspectRatio, "aspect-ratio", "", "force output aspect ratio (e.g. \"1:1\", \"16:9\")")
	backgroundCmd.Flags().StringVar(&bgAspectRatio, "ar", "", "shorthand for --aspect-ratio")

	// Tuning flags (with smart defaults)
	backgroundCmd.Flags().StringVar(&bgBGColor, "bg-color", "", "manually specify background color (e.g. \"#FFFFFF\")")
	backgroundCmd.Flags().Float64Var(&bgTolerance, "tolerance", 0, "color distance threshold (0 = auto)")
	backgroundCmd.Flags().IntVar(&bgFeather, "feather-radius", -1, "edge feathering radius in pixels (-1 = auto)")
	backgroundCmd.Flags().Float64Var(&bgFGThreshold, "fg-threshold", 1.5, "foreground protection multiplier")
	backgroundCmd.Flags().IntVar(&bgSmooth, "smooth", 1, "alpha mask smoothing passes (0 = disable)")
	backgroundCmd.Flags().IntVar(&bgErode, "erode-radius", 0, "edge erosion radius in pixels (0 = disable)")
	backgroundCmd.Flags().IntVar(&bgClose, "close-radius", 0, "morphological closing radius to fill interior holes (0 = disable)")

	// Output flags
	backgroundCmd.Flags().BoolVar(&bgJSON, "json", false, "JSON output")
	backgroundCmd.Flags().BoolVar(&bgPreview, "preview", false, "open result in system viewer")
	backgroundCmd.Flags().StringVarP(&bgOutput, "output", "o", "", "output directory (default: current directory)")

	// Shadow flags
	backgroundCmd.Flags().BoolVar(&bgShadow, "shadow", false, "add drop shadow behind subject")
	backgroundCmd.Flags().StringVar(&bgShadowOffset, "shadow-offset", "4,4", "shadow offset in pixels (\"dx,dy\")")
	backgroundCmd.Flags().IntVar(&bgShadowBlur, "shadow-blur", 6, "shadow blur radius in pixels")
	backgroundCmd.Flags().StringVar(&bgShadowColor, "shadow-color", "#000000", "shadow color (hex)")
	backgroundCmd.Flags().Float64Var(&bgShadowOpacity, "shadow-opacity", 40, "shadow opacity 0-100")

	// AI-assisted removal
	backgroundCmd.Flags().StringVar(&bgAI, "ai", "", `AI-assisted background removal: first regenerate image with solid background via API, then remove it. Built-in templates: default, white, human, product, good. Or provide a custom prompt: --ai "your prompt"`)
}
