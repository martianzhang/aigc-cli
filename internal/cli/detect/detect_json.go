package detect

import (
	"encoding/json"
	"fmt"
	"image"
	"os"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/martianzhang/aigc-cli/internal/detect"
	"github.com/martianzhang/aigc-cli/internal/forensic"
	"github.com/martianzhang/aigc-cli/internal/onnx"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/watermark"
)

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

	fftScore := detect.AnalyzeFFTFile(path)
	noiseScore := detect.AnalyzeNoiseFile(path)
	jpegScore := detect.AnalyzeJPEGFile(path)

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
		LLMScore:       -1,
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
		Details:   detect.BuildDetails(fr),
	}

	*results = append(*results, result)
	return nil
}
