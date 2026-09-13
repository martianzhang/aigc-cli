package cmd

import (
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
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/watermark"
)

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
		LLMScore:       -1, // -1 = unavailable; set below if online LLM provider is configured
	}

	// Detect visible AI watermarks for AI detection signal.
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

	// ── Online LLM assessment as additional signal ──
	dp := shared.ResolveProvider(ProviderNameDetect)
	if provider.IsOnlineProvider(dp) {
		assessment, err := provider.DescribeImage(dp, path, "Analyze this image and determine if it was AI-generated. "+
			"Look for visual artifacts, unnatural patterns, and any signs of AI generation. "+
			"Reply with only a number 0-100 where 0=certainly human, 100=certainly AI, then a brief reason.")
		if err == nil {
			opts.LLMScore = detect.ParseLLMScore(assessment)
			opts.LLMDetail = assessment
		}
	}

	fr := forensic.Analyze(opts)

	result.AIDetect = &service.AIDetectResult{
		AIGenRate: fr.AIGenRate,
		Emoji:     fr.Emoji,
		Summary:   fr.Summary,
		Details:   detect.BuildDetails(fr),
	}

	if err := service.PrintDetectResult(os.Stdout, result, true); err != nil {
		return err
	}

	if opts.WatermarkPresent {
		f, fErr := os.Open(path)
		if fErr == nil {
			img, _, decErr := image.Decode(f)
			f.Close()
			if decErr == nil {
				b := img.Bounds()
				imgW, imgH := b.Dx(), b.Dy()
				regions := watermark.DetectWatermarkRegions(img)
				if len(regions) > 0 {
					bounds := watermark.ComputeCropBounds(imgW, imgH, regions)
					if bounds.Valid {
						fmt.Printf("  Croped Size: %dx%d\n", bounds.W, bounds.H)
					}
				}
			}
		}
	}

	if detectPreview && !detectRemoveWM {
		service.PreviewFile(path)
	}
	applyWatermarkActions(path, result)

	return nil
}
