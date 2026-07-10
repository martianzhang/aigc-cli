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
  - 可见 AI 水印检测（需通过 --learn-watermark 学习后方可检测）

所有信号融合为单一 AIGen 置信度评分（含 emoji）。

⚠️ 合规声明
本项目不内置任何厂商的水印去除能力。--remove-watermark 仅对用户
通过 --learn-watermark 自行学习的水印生效。用户应自行确保使用
行为符合适用法律法规。

--add-watermark 仅用于为去水印算法创建测试样本，不注入任何元数据。

支持 PNG、JPEG、WebP、GIF、BMP 格式。`,
	RunE: runDetect,
}

var detectJSON bool
var detectPreview bool
var detectRemoveWM bool
var detectAddWM bool
var detectWmProducer string
var detectLearnWM string // --learn-watermark {name}

func runDetect(cmd *cobra.Command, args []string) error {
	// --learn-watermark: learn a custom watermark from seed images
	if detectLearnWM != "" {
		return runLearnWatermark(detectLearnWM)
	}

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

// runLearnWatermark learns a custom watermark from seed images in the watermark dir.
func runLearnWatermark(name string) error {
	dir := watermarkDir()
	blackPath, err := findSeedFile(dir, name, "black")
	if err != nil {
		return fmt.Errorf("load %s black seed: %w", name, err)
	}
	grayPath, err := findSeedFile(dir, name, "gray")
	if err != nil {
		return fmt.Errorf("load %s gray seed: %w", name, err)
	}

	blackImg, err := loadImage(blackPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", blackPath, err)
	}
	grayImg, err := loadImage(grayPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", grayPath, err)
	}

	b := blackImg.Bounds()
	g := grayImg.Bounds()
	if b.Dx() != g.Dx() || b.Dy() != g.Dy() {
		return fmt.Errorf("black and gray images must have same dimensions: black=%dx%d, gray=%dx%d",
			b.Dx(), b.Dy(), g.Dx(), g.Dy())
	}

	// Seed quality assessment
	sq := watermark.AssessSeedQuality(blackImg, grayImg)
	fmt.Println("Seed quality:")
	fmt.Printf("  Background:  black=%.1f (expect ~0), gray=%.1f (expect ~128)  %s\n",
		sq.BlackBG, sq.GrayBG, scoreTag(sq.BGScore))
	fmt.Printf("  Gradient:    gx=%.4f gy=%.4f (threshold |g|<0.01)  %s\n",
		sq.Gx, sq.Gy, scoreTag(sq.GradientScore))
	fmt.Printf("  Edge noise:  black=%.1f, gray=%.1f (good<5, warn<15)  %s\n",
		sq.BlackNoise, sq.GrayNoise, scoreTag(sq.NoiseScore))
	fmt.Printf("  WM signal:   max=%.0f (good>50)  %s\n",
		sq.SignalMax, scoreTag(sq.SignalScore))

	if sq.BGScore == watermark.SeedFail || sq.NoiseScore == watermark.SeedFail {
		fmt.Println("  ⚠  Low quality seeds — alpha map may be noisy. Try regenerating seed images.")
	}

	lr := watermark.LearnWatermark(blackImg, grayImg, name)

	outputPath := filepath.Join(dir, name+".watermark.png")
	if err := watermark.SaveWatermarkPNG(outputPath, lr); err != nil {
		return fmt.Errorf("save watermark: %w", err)
	}

	fmt.Printf("\nWatermark config saved: %s\n", outputPath)
	fmt.Printf("  Name:             %s\n", lr.Name)
	fmt.Printf("  Alpha map:        %dx%d\n", lr.AlphaMap.Width, lr.AlphaMap.Height)
	fmt.Printf("  Native width:     %dpx\n", lr.NativeWidth)
	fmt.Printf("  Margin X frac:    %.6f\n", lr.MarginXFrac)
	fmt.Printf("  Margin Y frac:    %.6f\n", lr.MarginYFrac)
	fmt.Printf("  Detect threshold: %.2f\n", lr.DetectThreshold)
	fmt.Printf("  Remove strategy:  %s\n", lr.RemoveStrategy)
	fmt.Println()
	fmt.Printf("Use: aigc-cli detect <image> --remove-watermark --producer %s\n", name)

	return nil
}

// loadImage decodes an image from a file path.
func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return img, nil
}

// findSeedFile looks for {name}.{type}.png or {name}.{type}.jpg in dir.
func findSeedFile(dir, name, typ string) (string, error) {
	exts := []string{".png", ".jpg", ".jpeg"}
	for _, ext := range exts {
		path := filepath.Join(dir, name+"."+typ+ext)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no %s seed found: tried %s.%s.{png,jpg,jpeg}", typ, name, typ)
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
		// Load custom watermarks from the watermark directory (may not exist)
		_ = watermark.LoadWatermarkPNGsFromDir(watermarkDir())

		outPath := cleanPath(path)
		producer := detectWmProducer
		if producer == "" && result.TC260 != nil && result.TC260.Present {
			if cp := result.TC260.Fields[service.ContentProducerKey]; cp != "" {
				producer = watermark.ProducerToConfig(cp)
			}
			if producer == "" && result.TC260.Provider != "" {
				producer = watermark.ProducerToConfig(result.TC260.Provider)
			}
		}
		// C2PA vendor fallback (Gemini has no TC260, only C2PA)
		if producer == "" && result.C2PA != nil && result.C2PA.Present {
			producer = watermark.ProducerToConfig(result.C2PA.Vendor)
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

func configDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ".config/aigc-cli"
	}
	return filepath.Join(home, ".config", "aigc-cli")
}

// watermarkDir returns the directory for custom watermark configs.
func watermarkDir() string {
	return filepath.Join(configDir(), "watermark")
}

func init() {
	rootCmd.AddCommand(detectCmd)
	detectCmd.Flags().BoolVar(&detectJSON, "json", false, "output results as JSON")
	detectCmd.Flags().BoolVar(&detectPreview, "preview", false, "open image in system viewer after detection")
	detectCmd.Flags().BoolVar(&detectRemoveWM, "remove-watermark", false, "remove visible AI watermarks learned via --learn-watermark. Requires --producer {name}.")
	detectCmd.Flags().BoolVar(&detectAddWM, "add-watermark", false, "add a visible AI watermark for testing removal (no metadata injected)")
	detectCmd.Flags().StringVar(&detectWmProducer, "producer", "",
		`watermark producer name learned via --learn-watermark, e.g. "gemini"`)
	detectCmd.Flags().StringVar(&detectLearnWM, "learn-watermark", "", "learn a custom watermark from ~/.config/aigc-cli/watermark/{name}.black.png + {name}.gray.png")
}
