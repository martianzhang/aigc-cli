package background

import (
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/martianzhang/aigc-cli/internal/background"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/rmbg"
	"github.com/martianzhang/aigc-cli/internal/service"
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
		dx, dy, err := service.ParseOffset(bgShadowOffset)
		if err != nil {
			return fmt.Errorf("invalid --shadow-offset: %w", err)
		}
		opts.ShadowOffset = [2]int{dx, dy}
		opts.ShadowBlur = bgShadowBlur
		opts.ShadowOpacity = bgShadowOpacity
		if bgShadowColor != "" {
			c, err := service.ParseHexColor(bgShadowColor)
			if err != nil {
				return fmt.Errorf("invalid --shadow-color: %w", err)
			}
			opts.ShadowColor = c
		}
	}

	// 输出目录统一使用全局 --output（默认 "."，可由 config 注入）
	outDir := options.Shared.OutputDir

	// 确定替换颜色或图片
	var repColor color.Color
	var repImg image.Image
	var doReplace bool
	if bgReplace != "" {
		doReplace = true
		if strings.HasPrefix(bgReplace, "#") {
			c, err := service.ParseHexColor(bgReplace)
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
	bgProvider := options.Shared.ResolveProvider(options.ProviderNameBackground)
	useOnlineBG := provider.IsOnlineProvider(bgProvider) || options.Shared.APIBaseSet

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

func tryInitRMBG() (*rmbg.Detector, error) {
	// ONNX Runtime lives in the shared models root
	libPath, err := rmbg.DefaultLibPath(filepath.Join(options.ConfigDir(), "models"))
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
	if cfg := options.BackgroundConfig(); cfg != nil && cfg.ModelsDir != "" {
		return cfg.ModelsDir
	}
	return filepath.Join(options.ConfigDir(), "models", "background")
}

func init() {

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

	// Online LLM 评估标志
	backgroundCmd.Flags().StringVar(&bgPrompt, "prompt", "", "custom prompt for online LLM assessment (requires --provider)")

	// 投影标志
	backgroundCmd.Flags().BoolVarP(&bgShadow, "shadow", "s", false, "add drop shadow behind subject")
	backgroundCmd.Flags().StringVar(&bgShadowOffset, "shadow-offset", "4,4", "shadow offset in pixels (\"dx,dy\")")
	backgroundCmd.Flags().IntVar(&bgShadowBlur, "shadow-blur", 6, "shadow blur radius in pixels")
	backgroundCmd.Flags().StringVar(&bgShadowColor, "shadow-color", "#000000", "shadow color (hex)")
	backgroundCmd.Flags().Float64Var(&bgShadowOpacity, "shadow-opacity", 40, "shadow opacity 0-100")
}

// Cmd returns the background command tree.
func Cmd() *cobra.Command { return backgroundCmd }

// Detector returns the cached RMBG detector (may be nil).
func Detector() *rmbg.Detector { return rmbgDetector }

// EnsureDetector lazily initializes and returns the RMBG detector.
func EnsureDetector() (*rmbg.Detector, error) {
	if rmbgDetector == nil {
		d, err := tryInitRMBG()
		if err != nil {
			return nil, err
		}
		rmbgDetector = d
	}
	return rmbgDetector, nil
}
