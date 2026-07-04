package cmd

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/martianzhang/apimart-cli/internal/forensic"
	"github.com/martianzhang/apimart-cli/internal/onnx"
	"github.com/martianzhang/apimart-cli/internal/service"
)

var detectCmd = &cobra.Command{
	Use:          "detect <file...>",
	Short:        "Detect watermarks, metadata, and AIGC signals in images",
	SilenceUsage: true,
	Long: `Detect watermarks, metadata, and AIGC signals in image files.

Analyzes images through multiple signals:
  - C2PA Content Credentials (tamper-evident provenance metadata)
  - TC260 AIGC labels (China GB 45438-2025)
  - SynthID invisible watermarks (inferred from C2PA vendor)
  - FFT power spectrum analysis (pixel-level frequency artifacts)
  - ONNX model-based AI generation detection (requires download)

All signals are fused into a single AIGen confidence score with emoji.

Supports PNG, JPEG, WebP, GIF, and BMP formats.`,
	RunE: runDetect,
}

var detectJSON bool

func runDetect(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return detectFiles(args, "")
	}

	stat, err := os.Stdin.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) != 0 {
		return fmt.Errorf("no files specified: pass file paths as arguments or pipe file data to stdin")
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("no data read from stdin")
	}

	tmpFile, err := os.CreateTemp("", "apimart-detect-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	return detectFiles([]string{tmpFile.Name()}, "(stdin)")
}

func detectFiles(paths []string, pathOverride string) error {
	aiDetector := tryInitONNX()
	if aiDetector != nil {
		defer aiDetector.Close()
	}

	if detectJSON {
		return detectFilesJSON(paths, pathOverride, aiDetector)
	}

	for _, path := range paths {
		if err := detectOneFile(path, pathOverride, aiDetector); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
	return nil
}

func detectOneFile(path, pathOverride string, aiDetector *onnx.Detector) error {
	result, err := service.DetectImage(path)
	if err != nil {
		return fmt.Errorf("metadata: %w", err)
	}
	if pathOverride != "" {
		result.Path = pathOverride
	}

	var onnxScore float64 = -1
	var onnxModelSize string
	if aiDetector != nil {
		aiResult, err := aiDetector.DetectFile(path)
		if err == nil {
			onnxScore = aiResult.AIGenRate
			onnxModelSize = modelSizeLabel(aiDetector.ModelPath())
		}
	}

	fftScore := analyzeFFTFile(path)

	opts := forensic.Options{
		C2PAPresent:    result.C2PA != nil && result.C2PA.Present,
		C2PAVendor:     safeC2PAVendor(result.C2PA),
		C2PASource:     safeC2PASource(result.C2PA),
		TC260Present:   result.TC260 != nil && result.TC260.Present,
		TC260Provider:  safeTC260Provider(result.TC260),
		SynthIDPresent: result.SynthID != nil && result.SynthID.Present,
		SynthIDLikely:  result.SynthID != nil && result.SynthID.Likely,
		SynthIDSource:  safeSynthIDSource(result.SynthID),
		CameraPresent:  result.Camera != nil,
		CameraMake:     safeCameraMake(result.Camera),
		CameraModel:    safeCameraModel(result.Camera),
		ONNXScore:      onnxScore,
		ONNXModelSize:  onnxModelSize,
		FFTScore:       fftScore,
	}
	fr := forensic.Analyze(opts)

	result.AIDetect = &service.AIDetectResult{
		AIGenRate: fr.AIGenRate,
		Emoji:     fr.Emoji,
		Summary:   fr.Summary,
		Details:   buildDetails(fr),
	}

	return service.PrintDetectResult(os.Stdout, result, true)
}

func detectFilesJSON(paths []string, pathOverride string, aiDetector *onnx.Detector) error {
	var results []*service.DetectResult
	for _, path := range paths {
		if err := detectOneFileJSON(path, pathOverride, aiDetector, &results); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
	if len(results) == 0 {
		return nil
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if len(results) == 1 {
		return enc.Encode(results[0])
	}
	return enc.Encode(results)
}

func detectOneFileJSON(path, pathOverride string, aiDetector *onnx.Detector, results *[]*service.DetectResult) error {
	result, err := service.DetectImage(path)
	if err != nil {
		return fmt.Errorf("metadata: %w", err)
	}
	if pathOverride != "" {
		result.Path = pathOverride
	}

	var onnxScore float64 = -1
	var onnxModelSize string
	if aiDetector != nil {
		aiResult, err := aiDetector.DetectFile(path)
		if err == nil {
			onnxScore = aiResult.AIGenRate
			onnxModelSize = modelSizeLabel(aiDetector.ModelPath())
		}
	}

	fftScore := analyzeFFTFile(path)

	opts := forensic.Options{
		C2PAPresent:    result.C2PA != nil && result.C2PA.Present,
		C2PAVendor:     safeC2PAVendor(result.C2PA),
		C2PASource:     safeC2PASource(result.C2PA),
		TC260Present:   result.TC260 != nil && result.TC260.Present,
		TC260Provider:  safeTC260Provider(result.TC260),
		SynthIDPresent: result.SynthID != nil && result.SynthID.Present,
		SynthIDLikely:  result.SynthID != nil && result.SynthID.Likely,
		SynthIDSource:  safeSynthIDSource(result.SynthID),
		CameraPresent:  result.Camera != nil,
		CameraMake:     safeCameraMake(result.Camera),
		CameraModel:    safeCameraModel(result.Camera),
		ONNXScore:      onnxScore,
		ONNXModelSize:  onnxModelSize,
		FFTScore:       fftScore,
	}
	fr := forensic.Analyze(opts)

	result.AIDetect = &service.AIDetectResult{
		AIGenRate: fr.AIGenRate,
		Emoji:     fr.Emoji,
		Summary:   fr.Summary,
		Details:   buildDetails(fr),
	}

	*results = append(*results, result)
	return nil
}

// analyzeFFTFile loads an image and runs FFT spectral analysis.
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

// buildDetails creates a compact breakdown of all signals.
func buildDetails(r *forensic.Result) string {
	s := ""
	for _, sig := range r.Signals {
		if s != "" {
			s += "; "
		}
		s += fmt.Sprintf("%s=%.0f%%", sig.Name, sig.Score*100)
	}
	return s
}

// Helper functions to safely extract values from result pointers.

func safeC2PAVendor(r *service.C2PAResult) string {
	if r == nil { return "" }; return r.Vendor
}
func safeC2PASource(r *service.C2PAResult) string {
	if r == nil { return "" }; return r.Source
}
func safeTC260Provider(r *service.TC260Result) string {
	if r == nil { return "" }; return r.Provider
}
func safeSynthIDSource(r *service.SynthIDResult) string {
	if r == nil { return "" }; return r.Source
}
func safeCameraMake(r *service.CameraInfo) string {
	if r == nil { return "" }; return r.Make
}
func safeCameraModel(r *service.CameraInfo) string {
	if r == nil { return "" }; return r.Model
}

// tryInitONNX initializes the ONNX detector.
func tryInitONNX() *onnx.Detector {
	modelsDir := filepath.Join(configDir(), "models")
	libPath, err := onnx.DefaultLibPath(modelsDir)
	if err != nil {
		return nil
	}
	for _, f := range []string{"model-large.onnx", "model-small.onnx"} {
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

func modelSizeLabel(modelPath string) string {
	if filepath.Base(modelPath) == "model-large.onnx" {
		return "large"
	}
	return "small"
}

func configDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ".config/apimart"
	}
	return filepath.Join(home, ".config", "apimart")
}

func init() {
	rootCmd.AddCommand(detectCmd)
	detectCmd.Flags().BoolVar(&detectJSON, "json", false, "output results as JSON")
}
