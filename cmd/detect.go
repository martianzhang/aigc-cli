package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/onnx"
	"github.com/martianzhang/apimart-cli/internal/service"
)

// detectCmd represents the `apimart-cli detect` command.
var detectCmd = &cobra.Command{
	Use:          "detect <file...>",
	Short:        "Detect watermarks and metadata in images",
	SilenceUsage: true,
	Long: `Detect watermarks and metadata in image files.

Scans images for C2PA Content Credentials, TC260 AIGC labels
(China GB 45438-2025), and SynthID invisible watermarks.
All metadata including file stats, dimensions, and embedded text
chunks is shown by default.

Use --ai to also run an offline ONNX model-based AIGC detection
(distilled Vision Transformer, 11.8M params). Requires downloading
the ONNX Runtime and model files first.

Supports PNG, JPEG, WebP, GIF, and BMP formats.

Examples:
  apimart-cli detect image.png
  apimart-cli detect --json image.png
  apimart-cli detect --ai image.png
  apimart-cli detect *.png
  cat image.png | apimart-cli detect`,
	RunE: runDetect,
}

var detectJSON bool
var detectAI bool

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
	// Initialize ONNX detector if --ai is set
	var aiDetector *onnx.Detector
	if detectAI {
		modelsDir := filepath.Join(configDir(), "models")
		libPath, err := onnx.DefaultLibPath(modelsDir)
		if err != nil {
			return fmt.Errorf("--ai: ONNX Runtime not found in %s.\n  Download it from: https://github.com/microsoft/onnxruntime/releases\n  Place onnxruntime.dll in: %s", modelsDir, modelsDir)
		}
		modelPath := onnx.DefaultModelPath(modelsDir)
		if _, err := os.Stat(modelPath); err != nil {
			return fmt.Errorf("--ai: model.onnx not found in %s.\n  Download from: https://huggingface.co/onnx-community/ai-image-detect-distilled-ONNX\n  Save as: %s", modelsDir, modelPath)
		}
		d, err := onnx.NewDetector(libPath, modelPath)
		if err != nil {
			return fmt.Errorf("--ai: failed to initialize AI detector: %w", err)
		}
		aiDetector = d
		defer aiDetector.Close()
	}

	if detectJSON {
		return detectFilesJSON(paths, pathOverride, aiDetector)
	}

	for _, path := range paths {
		result, err := service.DetectImage(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}
		if pathOverride != "" {
			result.Path = pathOverride
		}

		// Run AI detection if enabled
		if aiDetector != nil {
			aiResult, err := aiDetector.DetectFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "AI Detect error: %v\n", err)
			} else {
				label := "FAKE (AI Generated)"
				if !aiResult.IsFake {
					label = "REAL (Human)"
				}
				result.AIDetect = &service.AIDetectResult{
					FakeScore: aiResult.FakeScore,
					RealScore: aiResult.RealScore,
					Label:     label,
					Model:     "distilled-vit-11.8M",
				}
			}
		}

		if err := service.PrintDetectResult(os.Stdout, result, true); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
	return nil
}

func detectFilesJSON(paths []string, pathOverride string, aiDetector *onnx.Detector) error {
	var results []*service.DetectResult
	for _, path := range paths {
		result, err := service.DetectImage(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}
		if pathOverride != "" {
			result.Path = pathOverride
		}

		if aiDetector != nil {
			aiResult, err := aiDetector.DetectFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "AI Detect error: %v\n", err)
			} else {
				label := "FAKE (AI Generated)"
				if !aiResult.IsFake {
					label = "REAL (Human)"
				}
				result.AIDetect = &service.AIDetectResult{
					FakeScore: aiResult.FakeScore,
					RealScore: aiResult.RealScore,
					Label:     label,
					Model:     "distilled-vit-11.8M",
				}
			}
		}

		results = append(results, result)
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

// configDir returns the apimart config directory (~/.config/apimart).
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
	detectCmd.Flags().BoolVar(&detectAI, "ai", false, "run ONNX model-based AIGC detection (requires downloading model files)")
}
