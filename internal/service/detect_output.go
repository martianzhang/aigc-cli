package service

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// PrintImageInfo writes image metadata to w.
// Backward-compatible wrapper that prints everything (verbose mode).
func PrintImageInfo(w io.Writer, path string) error {
	result, err := DetectImage(path)
	if err != nil {
		return err
	}
	return PrintDetectResult(w, result, true)
}

// PrintDetectResult writes detection results to w.
// When verbose=false: only print C2PA and TC260 watermark info.
// When verbose=true: print everything (file stats, dimensions, format, all metadata).
func PrintDetectResult(w io.Writer, result *DetectResult, verbose bool) error {
	if verbose {
		fmt.Fprintf(w, "━━━ %s ━━━\n", filepath.Base(result.Path))
		fmt.Fprintf(w, "  Size:     %s\n", result.SizeHuman)
		fmt.Fprintf(w, "  Modified: %s\n", result.Modified.Format("2006-01-02 15:04:05"))
		if result.Format != "unknown" {
			fmt.Fprintf(w, "  Format:   %s\n", result.Format)
			fmt.Fprintf(w, "  Dims:     %d x %d\n", result.Width, result.Height)
		} else {
			fmt.Fprintf(w, "  (not a recognized image format)\n")
		}
	}

	if result.C2PA != nil && result.C2PA.Present {
		printC2PAInfo(w, result.C2PA.Vendor, result.C2PA.Software, result.C2PA.Version, result.C2PA.Source)
	}

	if result.SynthID != nil {
		printSynthID(w, result.SynthID)
	}

	if result.TC260 != nil && result.TC260.Present {
		printTC260(w, result.TC260)
	}

	if result.AIDetect != nil {
		printAIDetect(w, result.AIDetect)
	}

	if verbose {
		printCameraInfo(w, result.Camera)
		if result.Comment != "" {
			fmt.Fprintf(w, "  Comment:  %s\n", truncateMeta(result.Comment))
		}
		if result.Software != "" {
			fmt.Fprintf(w, "  Software: %s\n", truncateMeta(result.Software))
		}
		if len(result.Metadata) > 0 {
			fmt.Fprintf(w, "  Metadata:\n")
			for _, key := range []string{"Software", "Description", "Generation", "parameters", "Title"} {
				if v, ok := result.Metadata[key]; ok {
					fmt.Fprintf(w, "    %s: %s\n", key, truncateMeta(v))
					delete(result.Metadata, key)
				}
			}
			for k, v := range result.Metadata {
				fmt.Fprintf(w, "    %s: %s\n", k, truncateMeta(v))
			}
		}
	}

	return nil
}

// PrintDetectResultJSON writes detection results as JSON to w.
func PrintDetectResultJSON(w io.Writer, result *DetectResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// printC2PAInfo writes the C2PA watermark section.
func printC2PAInfo(w io.Writer, vendor, software, version, source string) {
	fmt.Fprintf(w, "  Watermark: C2PA Content Credentials\n")
	if vendor != "" {
		fmt.Fprintf(w, "    Vendor:   %s\n", vendor)
	}
	if software != "" {
		s := software
		if version != "" {
			s += " " + version
		}
		fmt.Fprintf(w, "    Software: %s\n", s)
	}
	if source != "" {
		fmt.Fprintf(w, "    Source:   %s\n", source)
	}
}

// printSynthID formats and prints SynthID watermark inference.
func printSynthID(w io.Writer, result *SynthIDResult) {
	if result.Present {
		fmt.Fprintf(w, "  Watermark: SynthID (Google invisible watermark)\n")
	} else if result.Likely {
		fmt.Fprintf(w, "  Watermark: SynthID likely (inferred from metadata)\n")
	} else {
		return
	}
	if result.Source != "" {
		fmt.Fprintf(w, "    Source:   %s\n", result.Source)
	}
	if result.Inference != "" {
		fmt.Fprintf(w, "    Note:     %s\n", result.Inference)
	}
}

// printAIDetect prints the fused AIGC detection result with emoji.
func printAIDetect(w io.Writer, result *AIDetectResult) {
	fmt.Fprintf(w, "  AI Detect:  %s\n", result.Summary)
	if result.Details != "" {
		fmt.Fprintf(w, "    %s\n", result.Details)
	}
}

// printTC260 formats and prints the TC260 AIGC label data.
func printTC260(w io.Writer, result *TC260Result) {
	fmt.Fprintf(w, "  Watermark: TC260 AIGC Label (China GB 45438-2025)\n")
	if result.Provider != "" {
		fmt.Fprintf(w, "    Provider: %s\n", result.Provider)
	}
	pretty := formatTC260(result.Fields)
	if pretty != "" {
		fmt.Fprintln(w, pretty)
	} else if result.Data != "" {
		fmt.Fprintf(w, "    Data: %s\n", truncateMeta(result.Data))
	}
}

// formatTC260 formats a parsed TC260 label map into display lines.
func formatTC260(data map[string]string) string {
	var result strings.Builder
	for k, v := range data {
		if v == "" {
			continue
		}
		if k == ContentProducerKey {
			if name := resolveContentProducer(v); name != "" {
				fmt.Fprintf(&result, "    Provider: %s\n", name)
				continue
			}
		}
		fmt.Fprintf(&result, "    %s: %s\n", k, v)
	}
	return strings.TrimSuffix(result.String(), "\n")
}

// printCameraInfo formats and prints EXIF camera metadata.
func printCameraInfo(w io.Writer, camera *CameraInfo) {
	if camera == nil || (camera.Make == "" && camera.Model == "" && camera.LensModel == "" &&
		camera.FocalLength == "" && camera.FNumber == "" && camera.ISO == "" && camera.ExposureTime == "") {
		fmt.Fprintf(w, "  Camera:   (none)\n")
		fmt.Fprintf(w, "    (no camera EXIF found — likely not a real photograph)\n")
		return
	}
	fmt.Fprintf(w, "  Camera:\n")
	if camera.Make != "" {
		fmt.Fprintf(w, "    Make:         %s\n", camera.Make)
	}
	if camera.Model != "" {
		fmt.Fprintf(w, "    Model:        %s\n", camera.Model)
	}
	if camera.LensModel != "" {
		fmt.Fprintf(w, "    Lens:         %s\n", camera.LensModel)
	}
	if camera.FocalLength != "" {
		fmt.Fprintf(w, "    FocalLength:  %s\n", camera.FocalLength)
	}
	if camera.FNumber != "" {
		fmt.Fprintf(w, "    Aperture:     f/%s\n", camera.FNumber)
	}
	if camera.ISO != "" {
		fmt.Fprintf(w, "    ISO:          %s\n", camera.ISO)
	}
	if camera.ExposureTime != "" {
		fmt.Fprintf(w, "    Shutter:      %s\n", camera.ExposureTime)
	}
}
