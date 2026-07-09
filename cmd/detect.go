package cmd

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/martianzhang/apimart-cli/internal/forensic"
	"github.com/martianzhang/apimart-cli/internal/onnx"
	"github.com/martianzhang/apimart-cli/internal/service"
	"github.com/martianzhang/apimart-cli/internal/watermark"
)

var detectCmd = &cobra.Command{
	Use:          "detect <file...>",
	Short:        "Detect watermarks, metadata, and AIGC signals in images",
	SilenceUsage: true,
	Long: `AIGC 检测与研究工具 — 通过多信号融合分析图片是否为 AI 生成。

分析信号包括:
  - C2PA Content Credentials（防篡改溯源元数据）
  - TC260 AIGC 标签（国标 GB 45438-2025）
  - SynthID 隐形水印（从 C2PA 厂商推断）
  - FFT 频谱分析（像素级频域伪影）
  - ONNX 模型推理（需下载模型）
  - 可见 AI 水印检测（Gemini/豆包/即梦/百度/智谱清言）

所有信号融合为单一 AIGen 置信度评分（含 emoji）。

⚠️ 合规声明
--remove-watermark 功能仅用于验证水印检测算法的准确性，以及
在合法场景下（如修复个人旧照片）使用。使用前必须通过
--confirm 确认您尊重知识产权并遵守适用法律法规。
禁止用于去除他人版权图片的水印或任何侵权用途。

--add-watermark 仅用于为去水印算法创建测试样本，不注入任何元数据。

支持 PNG、JPEG、WebP、GIF、BMP 格式。`,
	RunE: runDetect,
}

var detectJSON bool
var detectPreview bool
var detectRemoveWM bool
var detectAddWM bool
var detectWmProducer string
var detectConfirmed bool

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

	tmpFile, err := os.CreateTemp("", "aigc-cli-detect-*")
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
	noiseScore := analyzeNoiseFile(path)
	jpegScore := analyzeJPEGFile(path)

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
		NoiseScore:     noiseScore,
		JPEGScore:      jpegScore,
	}

	// Detect visible AI watermarks (Gemini/Doubao/Jimeng/Baidu/Zhipu) for AI detection signal.
	// Only run when no ironclad C2PA/TC260 metadata exists — that's when a visible
	// watermark left behind on a re-saved/re-encoded image carries decisive weight.
	if (!opts.C2PAPresent || opts.C2PASource != "AI Generated") && !opts.TC260Present {
		f, fErr := os.Open(path)
		if fErr == nil {
			img, _, decErr := image.Decode(f)
			f.Close()
			if decErr == nil {
				if dets := watermark.DetectWatermark(img); len(dets) > 0 {
					opts.WatermarkPresent = true
					opts.WatermarkName = dets[0].Name
				}
			}
		}
	}

	fr := forensic.Analyze(opts)

	result.AIDetect = &service.AIDetectResult{
		AIGenRate: fr.AIGenRate,
		Emoji:     fr.Emoji,
		Summary:   fr.Summary,
		Details:   buildDetails(fr),
	}

	if err := service.PrintDetectResult(os.Stdout, result, true); err != nil {
		return err
	}
	if detectPreview && !detectRemoveWM {
		service.PreviewFile(path)
	}
	if detectRemoveWM {
		if !detectConfirmed {
			return fmt.Errorf("--confirm is required with --remove-watermark: you must confirm you respect intellectual property rights and comply with applicable laws")
		}
		outPath := cleanPath(path)
		// Use TC260 metadata as a hint for which watermark engine to prefer
		producer := detectWmProducer
		if producer == "" && result.TC260 != nil && result.TC260.Present {
			if cp := result.TC260.Fields[service.ContentProducerKey]; cp != "" {
				producer = watermark.ProducerToConfig(cp)
			}
			if producer == "" && result.TC260.Provider != "" {
				producer = watermark.ProducerToConfig(result.TC260.Provider)
			}
		}
		res, err := watermark.RemoveFileHinted(path, outPath, producer)
		if err == nil && res.Removed {
			fmt.Printf("  Watermark removed (%s) → %s\n", res.Name, outPath)
			if detectPreview {
				service.PreviewFile(outPath)
			}
		} else {
			if err := stripMetadata(path); err == nil {
				fmt.Printf("  AI metadata removed → %s\n", outPath)
			}
		}
	}
	if detectAddWM {
		producer := detectWmProducer
		if producer == "" {
			producer = "unknown"
		}
		outPath := strings.TrimSuffix(path, filepath.Ext(path)) + "_watermarked.png"
		res, err := watermark.AddWatermarkFile(path, outPath, producer)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		} else {
			metaNote := ""
			if producer == "doubao" || producer == "jimeng" {
				metaNote = " + TC260 metadata"
			}
			fmt.Printf("  Watermark added (%s%s) → %s\n", res.Name, metaNote, outPath)
			if detectPreview {
				service.PreviewFile(outPath)
			}
		}
	}
	return nil
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
	noiseScore := analyzeNoiseFile(path)
	jpegScore := analyzeJPEGFile(path)

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
		NoiseScore:     noiseScore,
		JPEGScore:      jpegScore,
	}
	if (!opts.C2PAPresent || opts.C2PASource != "AI Generated") && !opts.TC260Present {
		f, fErr := os.Open(path)
		if fErr == nil {
			img, _, decErr := image.Decode(f)
			f.Close()
			if decErr == nil {
				if dets := watermark.DetectWatermark(img); len(dets) > 0 {
					opts.WatermarkPresent = true
					opts.WatermarkName = dets[0].Name
				}
			}
		}
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

// tryInitONNX initializes the ONNX detector.
func tryInitONNX() *onnx.Detector {
	modelsDir := detectModelsDir()
	libPath, err := onnx.DefaultLibPath(modelsDir)
	if err != nil {
		return nil
	}

	// Determine preferred model from config, default to "vit-base"
	preferredID := "vit-base"
	if shared.Cfg != nil && shared.Cfg.Detect != nil && shared.Cfg.Detect.Model != "" {
		preferredID = shared.Cfg.Detect.Model
	}
	// Map model ID → filename, with fallback
	modelFiles := []string{modelFilename(preferredID)}
	// Add fallback if different
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

// modelFilename returns the ONNX filename for a model identifier.
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

// detectModelsDir returns the configured or default models directory.
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

// cleanPath returns the output path for a metadata-stripped copy.
func cleanPath(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + "_clean" + ext
}

// stripMetadata decodes and re-encodes the image, removing all embedded
// metadata (C2PA, TC260, EXIF, XMP, etc.). Go's image encoders only write
// essential pixel data, discarding ancillary chunks and metadata segments.
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
		return jpeg.Encode(out, img, &jpeg.Options{Quality: 95})
	default:
		return png.Encode(out, img)
	}
}

func configDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ".config/aigc-cli"
	}
	return filepath.Join(home, ".config", "aigc-cli")
}

func init() {
	rootCmd.AddCommand(detectCmd)
	detectCmd.Flags().BoolVar(&detectJSON, "json", false, "output results as JSON")
	detectCmd.Flags().BoolVar(&detectPreview, "preview", false, "open image in system viewer after detection")
	detectCmd.Flags().BoolVar(&detectRemoveWM, "remove-watermark", false, "⚠️  remove visible AI watermarks (requires --confirm). Only for legal use such as restoring personal photos. NOT for removing copyright watermarks.")
	detectCmd.Flags().BoolVar(&detectAddWM, "add-watermark", false, "add a visible AI watermark for testing removal (no metadata injected)")
	detectCmd.Flags().BoolVar(&detectConfirmed, "confirm", false, "confirm you respect intellectual property rights and comply with applicable laws (required with --remove-watermark)")
	detectCmd.Flags().StringVar(&detectWmProducer, "producer", "",
		`watermark producer override (`+strings.Join([]string{service.ProviderGemini, service.ProviderDoubao, service.ProviderJimeng, service.ProviderDoubaoSnap, service.ProviderBaidu, service.ProviderZhipu}, "/")+`)`+
			` (for --add-watermark: the text to render as watermark if not a known name)`)
}
