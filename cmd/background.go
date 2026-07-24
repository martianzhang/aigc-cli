package cmd

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/background"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/rmbg"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/spf13/cobra"
)

// rmbgDetector 是全局缓存的 RMBG Detector 实例（惰性初始化）。
var rmbgDetector *rmbg.Detector

var backgroundCmd = &cobra.Command{
	Use:          "background <file...>",
	Aliases:      []string{"bg"},
	Short:        "Remove or replace image background using AI (also: bg)",
	SilenceUsage: true,
	Long: `Remove or replace the background from images using RMBG 2.0 AI semantic segmentation.

Powered by BRIA AI's RMBG 2.0 (BiRefNet) ONNX model — works on any image type,
not just solid-color backgrounds.

First use: run 'aigc-cli background init' to download the model.

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
	bgPrompt        string
)

func runBackground(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no files specified: pass one or more image file paths as arguments")
	}
	if !bgRemove && bgReplace == "" && !bgMaskOnly {
		return fmt.Errorf("specify --remove, --replace, or --mask-only")
	}

	opts := background.Defaults()
	if bgAutocrop {
		opts.Autocrop = true
	}
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

	// 确定输出目录
	outDir := bgOutput
	if outDir == "" {
		outDir = "."
	}

	// 确定替换颜色或图片
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

	// ── Online provider check ──
	bgProvider := shared.ResolveProvider(ProviderNameBackground)
	useOnlineBG := provider.IsOnlineProvider(bgProvider) || shared.APIBaseSet

	// Only init local RMBG when not using online generation.
	if !useOnlineBG {
		if rmbgDetector == nil {
			d, err := tryInitRMBG()
			if err != nil {
				return fmt.Errorf("RMBG not available: %w\n  Run 'aigc-cli background init' to download the model", err)
			}
			rmbgDetector = d
		}
		modelName := filepath.Base(rmbgDetector.ModelPath())
		fmt.Printf("Using Model: %s\n", modelName)
	}

	for _, arg := range args {
		start := time.Now()
		if err := processOneFile(arg, outDir, opts, doReplace, repColor, repImg, useOnlineBG, bgProvider); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", arg, err)
		} else {
			fmt.Printf("Time Cost: %s\n", time.Since(start).Round(time.Millisecond))
		}
	}

	return nil
}

func processOneFile(path, outDir string, opts background.Options, doReplace bool, repColor color.Color, repImg image.Image, runOnlineBG bool, bgProvider *provider.EffectiveProvider) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	f.Close()

	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	// Online-only mode: generate via image API, skip local RMBG entirely.
	if runOnlineBG {
		defaultPrompt := "Remove the background from this image. Keep the main subject exactly as is. Replace the background with a solid white color."
		if doReplace && repColor != nil {
			defaultPrompt = fmt.Sprintf("Replace the background of this image with color %s.", repColor)
		} else if doReplace && repImg != nil {
			defaultPrompt = "Replace the background of this image with a new background from the reference image."
		}
		onlineOut, err := generateOnlineBackground(path, bgProvider, defaultPrompt, bgPrompt)
		if err != nil {
			return fmt.Errorf("online generation failed: %w", err)
		}
		fmt.Printf("Saved: %s → %s\n", filepath.Base(path), filepath.Base(onlineOut))
		return nil
	}

	if bgMaskOnly {
		gray, result, err := background.MaskOnly(img, &opts, rmbgDetector)
		if err != nil {
			return err
		}
		outPath := filepath.Join(outDir, base+"_mask.png")
		if err := background.SavePNG(outPath, gray); err != nil {
			return err
		}
		fmt.Printf("Saved: %s → %s\n", filepath.Base(path), filepath.Base(outPath))
		if bgJSON {
			fmt.Printf("  %dx%d\n", result.Width, result.Height)
		}
		return nil
	}

	if bgRemove && !doReplace {
		outImg, result, err := background.RemoveBackground(img, &opts, rmbgDetector)
		if err != nil {
			return err
		}
		outPath := filepath.Join(outDir, base+"_removebg.png")
		if err := background.SavePNG(outPath, outImg); err != nil {
			return err
		}
		fmt.Printf("Saved: %s → %s\n", filepath.Base(path), filepath.Base(outPath))
		if bgJSON {
			fmt.Printf("  %dx%d\n", result.Width, result.Height)
		}
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
			outImg, result, err = background.ReplaceColor(img, repColor, &opts, rmbgDetector)
		} else {
			outImg, result, err = background.ReplaceImage(img, repImg, &opts, rmbgDetector)
		}
		if err != nil {
			return err
		}

		outPath := filepath.Join(outDir, base+"_replaced.png")
		if err := background.SavePNG(outPath, outImg); err != nil {
			return err
		}
		fmt.Printf("Saved: %s → %s\n", filepath.Base(path), filepath.Base(outPath))
		if bgJSON {
			fmt.Printf("  %dx%d\n", result.Width, result.Height)
		}
		if bgPreview {
			service.PreviewFile(outPath)
		}
		return nil
	}

	return nil
}

func tryInitRMBG() (*rmbg.Detector, error) {
	// ONNX Runtime lives in the shared models root
	libPath, err := rmbg.DefaultLibPath(filepath.Join(configDir(), "models"))
	if err != nil {
		return nil, err
	}

	modelPath := rmbg.DefaultModelPath(rmbgModelsDir())
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("RMBG model not found at %s", modelPath)
	}

	return rmbg.NewDetector(libPath, modelPath)
}

// rmbgModelsDir 返回 RMBG 模型存储目录。
func rmbgModelsDir() string {
	if shared.Cfg != nil && shared.Cfg.Background != nil && shared.Cfg.Background.ModelsDir != "" {
		return shared.Cfg.Background.ModelsDir
	}
	return filepath.Join(configDir(), "models", "background")
}

// --- 工具函数 ---

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

func init() {
	rootCmd.AddCommand(backgroundCmd)

	// 主要操作标志
	backgroundCmd.Flags().BoolVar(&bgRemove, "remove", false, "remove background (output transparent PNG)")
	backgroundCmd.Flags().BoolVar(&bgRemove, "rm", false, "shorthand for --remove")
	backgroundCmd.Flags().StringVarP(&bgReplace, "replace", "r", "", "replace background: hex color (#RRGGBB) or image path")
	backgroundCmd.Flags().BoolVar(&bgMaskOnly, "mask-only", false, "output grayscale alpha mask for debugging")

	// Autocrop 标志
	backgroundCmd.Flags().BoolVarP(&bgAutocrop, "autocrop", "c", false, "crop to foreground bounding box")
	backgroundCmd.Flags().BoolVar(&bgAutocrop, "ac", false, "shorthand for --autocrop")
	backgroundCmd.Flags().StringVar(&bgPadding, "padding", "", "padding: single value (\"20\") or four values (\"10,20,10,20\": top,right,bottom,left)")
	backgroundCmd.Flags().StringVar(&bgAspectRatio, "aspect-ratio", "", "force output aspect ratio (e.g. \"1:1\", \"16:9\")")
	backgroundCmd.Flags().StringVar(&bgAspectRatio, "ar", "", "shorthand for --aspect-ratio")

	// 输出标志
	backgroundCmd.Flags().BoolVarP(&bgJSON, "json", "j", false, "JSON output")
	backgroundCmd.Flags().BoolVarP(&bgPreview, "preview", "p", false, "open result in system viewer")
	backgroundCmd.Flags().StringVarP(&bgOutput, "output", "o", "", "output directory (default: current directory)")

	// Online LLM 评估标志
	backgroundCmd.Flags().StringVar(&bgPrompt, "prompt", "", "custom prompt for online LLM assessment (requires --provider)")

	// 投影标志
	backgroundCmd.Flags().BoolVarP(&bgShadow, "shadow", "s", false, "add drop shadow behind subject")
	backgroundCmd.Flags().StringVar(&bgShadowOffset, "shadow-offset", "4,4", "shadow offset in pixels (\"dx,dy\")")
	backgroundCmd.Flags().IntVar(&bgShadowBlur, "shadow-blur", 6, "shadow blur radius in pixels")
	backgroundCmd.Flags().StringVar(&bgShadowColor, "shadow-color", "#000000", "shadow color (hex)")
	backgroundCmd.Flags().Float64Var(&bgShadowOpacity, "shadow-opacity", 40, "shadow opacity 0-100")
}

// generateOnlineBackground generates a background-modified image via the image API.
// Reuses the existing image generation pipeline (sync for OpenAI, async for APIMart, native for Ollama).
func generateOnlineBackground(imagePath string, p *provider.EffectiveProvider, defaultPrompt, userPrompt string) (string, error) {
	prompt := userPrompt
	if prompt == "" {
		prompt = defaultPrompt
	}
	req := &types.GenerateRequest{
		Model:     p.Model,
		Prompt:    prompt,
		ImageURLs: []string{imagePath},
	}
	// Ollama native API
	if p.Type == types.ProviderOllama || provider.IsLocalEndpoint(p.BaseURL) {
		saved, err := ollamaGenerateImages(p.BaseURL, req)
		if err != nil {
			return "", err
		}
		if len(saved) == 0 {
			return "", fmt.Errorf("no images saved")
		}
		return saved[0], nil
	}
	c := client.NewFromProvider(p)
	if len(req.ImageURLs) > 0 {
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return "", fmt.Errorf("resolve image failed: %w", err)
		}
		req.ImageURLs = resolved
	}
	// APIMart: async submit → poll → download (reusing existing helpers)
	if p.ProviderType == provider.APIMart {
		subResp, err := c.Submit(req)
		if err != nil {
			return "", fmt.Errorf("submit failed: %w", err)
		}
		if len(subResp.Data) == 0 {
			return "", fmt.Errorf("submit returned no tasks")
		}
		taskData, err := c.PollTask(subResp.Data[0].TaskID)
		if err != nil {
			return "", fmt.Errorf("poll failed: %w", err)
		}
		if taskData.Result == nil || len(taskData.Result.Images) == 0 {
			return "", fmt.Errorf("no images in task result")
		}
		saved, err := downloadImages(taskData.Result.Images, taskData.ID)
		if err != nil {
			return "", fmt.Errorf("download failed: %w", err)
		}
		if len(saved) == 0 {
			return "", fmt.Errorf("no images downloaded")
		}
		return saved[0], nil
	}
	// Sync provider (OpenAI, OpenRouter, etc.)
	resp, err := c.ImageGenerateSync(req)
	if err != nil {
		return "", fmt.Errorf("online generation failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("no images returned")
	}
	img := resp.Data[0]
	prefix := fmt.Sprintf("bg_online_%d", time.Now().Unix())
	if img.B64JSON != "" {
		return service.SaveBase64Image(shared.OutputDir, prefix, img.B64JSON, 0)
	}
	if img.URL != "" {
		return service.DownloadFile(img.URL, shared.OutputDir, prefix)
	}
	return "", fmt.Errorf("no image data in response")
}
