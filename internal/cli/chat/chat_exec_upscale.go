package chat

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/imgcodec"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	up "github.com/martianzhang/aigc-cli/internal/upscale"
)

// executeUpscale upscales an image with a local ONNX super-resolution model.
func executeUpscale(argsJSON string) string {
	var a struct {
		InputPath  string `json:"input_path"`
		OutputPath string `json:"output_path"`
		Model      string `json:"model"`
		Scale      int    `json:"scale"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if a.InputPath == "" {
		return "Error: input_path is required"
	}
	if _, err := os.Stat(a.InputPath); err != nil {
		return fmt.Sprintf("Error: input file not found: %v", err)
	}

	info, ok := up.ResolveModel(a.Model)
	if !ok {
		return fmt.Sprintf("Error: unknown model %q", a.Model)
	}

	modelsDir := filepath.Join(options.ConfigDir(), "models")
	libPath, err := onnxrt.LibPath(modelsDir)
	if err != nil {
		return fmt.Sprintf("Error: ONNX Runtime not found — run 'aigc-cli upscale init' first: %v", err)
	}
	modelPath := up.ModelPath(modelsDir, a.Model)
	if _, err := os.Stat(modelPath); err != nil {
		return fmt.Sprintf("Error: model not found — run 'aigc-cli upscale init --model %s' first", info.ID)
	}

	f, err := os.Open(a.InputPath)
	if err != nil {
		return fmt.Sprintf("Error: cannot open file: %v", err)
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return fmt.Sprintf("Error: cannot decode image: %v", err)
	}

	det, err := up.NewDetector(libPath, modelPath, info.Scale, info.Input(), info.Output())
	if err != nil {
		return fmt.Sprintf("Error: init upscaler: %v", err)
	}
	defer det.Close()

	out, err := det.Upscale(img)
	if err != nil {
		return fmt.Sprintf("Error: upscale failed: %v", err)
	}

	if a.Scale > 0 && a.Scale != det.Scale() {
		b := img.Bounds()
		dst := image.NewNRGBA(image.Rect(0, 0, b.Dx()*a.Scale, b.Dy()*a.Scale))
		draw.CatmullRom.Scale(dst, dst.Bounds(), out, out.Bounds(), draw.Src, nil)
		out = dst
	}

	outPath := a.OutputPath
	if outPath == "" {
		stem := strings.TrimSuffix(filepath.Base(a.InputPath), filepath.Ext(a.InputPath))
		outPath = filepath.Join(options.Shared.OutputDir, stem+"_upscaled.png")
	}
	if _, err := imgcodec.EncodeToFile(out, outPath, "png", 100); err != nil {
		return fmt.Sprintf("Error: save failed: %v", err)
	}
	return fmt.Sprintf("Upscaled with %s → %s (%dx%d). User can use /preview to view it.",
		info.ID, outPath, out.Bounds().Dx(), out.Bounds().Dy())
}
