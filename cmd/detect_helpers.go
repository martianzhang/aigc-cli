package cmd

import (
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/martianzhang/apimart-cli/internal/forensic"
	"github.com/martianzhang/apimart-cli/internal/onnx"
	"github.com/martianzhang/apimart-cli/internal/service"
	"github.com/martianzhang/apimart-cli/internal/watermark"
)

// --- analysis functions ---

func analyzeFFTFile(path string) float64 {
	f, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return -1
	}
	return forensic.AnalyzeFFT(img)
}

func analyzeNoiseFile(path string) float64 {
	f, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return -1
	}
	return forensic.AnalyzeNoiseResidual(img)
}

func analyzeJPEGFile(path string) float64 {
	return forensic.AnalyzeJPEGDoubleQuant(path)
}

// --- safety helpers ---

func safeC2PAVendor(r *service.C2PAResult) string {
	if r == nil {
		return ""
	}
	return r.Vendor
}
func safeC2PASource(r *service.C2PAResult) string {
	if r == nil {
		return ""
	}
	return r.Source
}
func safeTC260Provider(r *service.TC260Result) string {
	if r == nil {
		return ""
	}
	return r.Provider
}
func safeSynthIDSource(r *service.SynthIDResult) string {
	if r == nil {
		return ""
	}
	return r.Source
}
func safeCameraMake(r *service.CameraInfo) string {
	if r == nil {
		return ""
	}
	return r.Make
}
func safeCameraModel(r *service.CameraInfo) string {
	if r == nil {
		return ""
	}
	return r.Model
}

// --- ONNX init ---

func tryInitONNX() *onnx.Detector {
	modelsDir := detectModelsDir()
	libPath, err := onnx.DefaultLibPath(modelsDir)
	if err != nil {
		return nil
	}

	preferredID := "vit-base"
	if shared.Cfg != nil && shared.Cfg.Detect != nil && shared.Cfg.Detect.Model != "" {
		preferredID = shared.Cfg.Detect.Model
	}
	modelFiles := []string{modelFilename(preferredID)}
	for _, id := range []string{"vit-base", "distilled-vit"} {
		fn := modelFilename(id)
		if fn != modelFiles[0] {
			modelFiles = append(modelFiles, fn)
		}
	}

	for _, f := range modelFiles {
		modelPath := filepath.Join(modelsDir, f)
		if _, err := os.Stat(modelPath); err != nil {
			continue
		}
		d, err := onnx.NewDetector(libPath, modelPath)
		if err != nil {
			continue
		}
		return d
	}
	return nil
}

func modelFilename(modelID string) string {
	switch modelID {
	case "vit-base":
		return "model-vit-base.onnx"
	case "distilled-vit":
		return "model-distilled-vit.onnx"
	default:
		return "model-vit-base.onnx"
	}
}

func detectModelsDir() string {
	if shared.Cfg != nil && shared.Cfg.Detect != nil && shared.Cfg.Detect.ModelsDir != "" {
		return shared.Cfg.Detect.ModelsDir
	}
	return filepath.Join(configDir(), "models")
}

func modelSizeLabel(modelPath string) string {
	base := filepath.Base(modelPath)
	switch base {
	case "model-vit-base.onnx":
		return "vit-base"
	case "model-distilled-vit.onnx":
		return "distilled-vit"
	default:
		return base
	}
}

// --- path helpers ---

func configDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ".config/aigc-cli"
	}
	return filepath.Join(home, ".config", "aigc-cli")
}

func watermarkDir() string {
	return filepath.Join(configDir(), "watermark")
}

// --- watermark removal helpers ---

// cleanPath returns the output path for a metadata-stripped copy.
func cleanPath(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + "_clean" + ext
}

// stripMetadata decodes and re-encodes the image, removing all embedded
// metadata (C2PA, TC260, EXIF, XMP, etc.).
func stripMetadata(path string) error {
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

	outPath := cleanPath(path)
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		q := watermark.EstimateJPEGQuality(path)
		return jpeg.Encode(out, img, &jpeg.Options{Quality: q})
	default:
		return png.Encode(out, img)
	}
}

// scoreTag returns [GOOD] / [WARN] / [FAIL] for a seed quality level.
func scoreTag(l watermark.SeedQualityLevel) string {
	switch l {
	case watermark.SeedGood:
		return "[GOOD]"
	case watermark.SeedWarn:
		return "[WARN]"
	case watermark.SeedFail:
		return "[FAIL]"
	default:
		return "[?]"
	}
}
