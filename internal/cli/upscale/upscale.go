// Package upscale implements the `aigc-cli upscale` command.
package upscale

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/service"
	up "github.com/martianzhang/aigc-cli/internal/upscale"
)

// Deps carries runtime configuration for the upscale command.
type Deps struct {
	OutputDir string
	Verbose   bool
	ModelsDir string
}

var d Deps

var (
	upInput   string
	upOutput  string
	upModel   string
	upScale   int
	upDryRun  bool
	upPreview bool
)

// NewCommand builds the `upscale` command tree.
func NewCommand(deps func() Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "upscale",
		Aliases:      []string{"sr"},
		Short:        "Upscale image resolution with a local AI super-resolution model (offline ONNX)",
		SilenceUsage: true,
		Long: `Upscale an image using a local Real-ESRGAN / Real-CUGAN / Swin2SR ONNX model.

Fully offline — no API key needed. The model runs on the shared ONNX Runtime
used by the other local commands (background, depth, ocr, ...). Large images are
processed in feathered tiles, so memory stays bounded.

Models are downloaded on demand; see 'aigc-cli upscale init --list' for the full
list, use cases and licenses. Quick pick:
  general photos:          realesr-general-x4v3 (default) | real-esrgan-x4plus (photo HQ, slow)
  anime / illustration:    real-cugan-2x | real-esrgan-x4plus-anime-6b | real-esrgan-animevideov3 (fastest)
  noisy / compressed:      swin2sr-realworld-x4 | swin2sr-compressed-x4
  clean / classical:       swin2sr-classical-x4
  lightweight 2x:          swin2sr-lightweight-x2

Examples:
  aigc-cli upscale init                      # download ONNX Runtime + default model
  aigc-cli upscale init --list               # list models and licenses
  aigc-cli upscale -i photo.jpg
  aigc-cli upscale -i photo.jpg -o photo_4x.png --scale 4
  aigc-cli upscale -i art.png --model real-cugan-2x --scale 2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			d = deps()
			return runUpscale(cmd, args)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&upInput, "input", "i", "", "Input image file")
	f.StringVarP(&upOutput, "output", "o", "", "Output image file (default: <stem>_upscaled.png in output dir)")
	f.StringVar(&upModel, "model", "", fmt.Sprintf("Model id (default: %s). Options: %s", up.DefaultModelID, strings.Join(up.ModelIDs(), ", ")))
	f.IntVar(&upScale, "scale", 0, "Output scale factor (default: model's native scale; other values resample the result)")
	f.BoolVar(&upDryRun, "dry-run", false, "Print the plan without running inference")
	f.BoolVar(&upPreview, "preview", false, "Open the result with the system default viewer")
	cmd.AddCommand(newInitCommand(deps))
	return cmd
}

func runUpscale(cmd *cobra.Command, args []string) error {
	if upInput == "" {
		return fmt.Errorf("input required: use --input/-i <file>")
	}
	if upScale < 0 {
		return fmt.Errorf("invalid --scale %d: must be >= 1, or omit for native scale", upScale)
	}

	info, ok := up.ResolveModel(upModel)
	if !ok {
		return fmt.Errorf("unknown model %q; available: %s", upModel, strings.Join(up.ModelIDs(), ", "))
	}
	modelPath := up.ModelPath(d.ModelsDir, upModel)

	outPath := upOutput
	if outPath == "" {
		stem := strings.TrimSuffix(filepath.Base(upInput), filepath.Ext(upInput))
		outPath = filepath.Join(d.OutputDir, stem+"_upscaled.png")
	}

	if upDryRun {
		fmt.Printf("# upscale dry run\n")
		fmt.Printf("# input:   %s\n", upInput)
		fmt.Printf("# output:  %s\n", outPath)
		fmt.Printf("# model:   %s (%s, native x%d, ~%.1fMB, %s)\n", info.ID, info.Name, info.Scale, info.SizeMB, info.License)
		fmt.Printf("# scale:   %s\n", scaleLabel(upScale, info.Scale))
		fmt.Printf("# file:    %s\n", modelPath)
		return nil
	}

	f, err := os.Open(upInput)
	if err != nil {
		return fmt.Errorf("open input: %w", err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode input: %w", err)
	}

	libPath, err := onnxrt.LibPath(d.ModelsDir)
	if err != nil {
		return fmt.Errorf("onnxruntime not found: %w\n  run 'aigc-cli upscale init' first", err)
	}
	if _, err := os.Stat(modelPath); err != nil {
		return fmt.Errorf("model not found: %s\n  run 'aigc-cli upscale init --model %s' first", modelPath, info.ID)
	}

	det, err := up.NewDetector(libPath, modelPath, info.Scale, info.Input(), info.Output())
	if err != nil {
		return fmt.Errorf("init upscaler: %w", err)
	}
	defer det.Close()

	b := img.Bounds()
	fmt.Printf("Upscaling %dx%d with %s (native x%d)...\n", b.Dx(), b.Dy(), info.ID, det.Scale())
	out, err := det.Upscale(img)
	if err != nil {
		return err
	}

	if upScale != 0 && upScale != det.Scale() {
		dst := image.NewNRGBA(image.Rect(0, 0, b.Dx()*upScale, b.Dy()*upScale))
		draw.CatmullRom.Scale(dst, dst.Bounds(), out, out.Bounds(), draw.Src, nil)
		out = dst
	}

	if _, err := imgcodec.EncodeToFile(out, outPath, "png", 100); err != nil {
		return fmt.Errorf("encode output: %w", err)
	}
	fmt.Printf("Saved: %s (%dx%d)\n", outPath, out.Bounds().Dx(), out.Bounds().Dy())

	if upPreview {
		if err := service.PreviewFile(outPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: preview failed: %v\n", err)
		}
	}
	return nil
}

func scaleLabel(requested, native int) string {
	if requested == 0 {
		return fmt.Sprintf("native (x%d)", native)
	}
	return fmt.Sprintf("%d", requested)
}
