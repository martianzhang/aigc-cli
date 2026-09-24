package ocr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/ocr"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
)

// newOCREngine creates an OCR engine with the standard model setup and the
// global ocrScanLang setting.
func newOCREngine(modelsDir string) (*ocr.Engine, error) {
	return NewOCREngineWithLang(modelsDir, ocrScanLang)
}

// NewOCREngineWithLang creates an OCR engine with the given language setting.
func NewOCREngineWithLang(modelsDir, lang string) (*ocr.Engine, error) {
	ocrModelsDir := filepath.Join(modelsDir, "ocr")

	libPath, err := onnxrt.LibPath(modelsDir)
	if err != nil {
		libPath, err = onnxrt.EnsureInstalled(modelsDir, false)
		if err != nil {
			return nil, fmt.Errorf("ONNX Runtime not available: %w\n\nRun 'aigc-cli ocr init' first", err)
		}
	}

	detPath := filepath.Join(ocrModelsDir, "ch_PP-OCRv4_det_infer.onnx")
	recPath := filepath.Join(ocrModelsDir, "ch_PP-OCRv4_rec_infer.onnx")
	dictPath := filepath.Join(ocrModelsDir, "dict_zh.txt")

	if _, err := os.Stat(detPath); err != nil {
		return nil, fmt.Errorf("detection model not found: %w\n\nRun 'aigc-cli ocr init' first", err)
	}
	if _, err := os.Stat(recPath); err != nil {
		return nil, fmt.Errorf("recognition model not found: %w\n\nRun 'aigc-cli ocr init' first", err)
	}

	clsPath := filepath.Join(ocrModelsDir, "ch_ppocr_mobile_v2.0_cls_infer.onnx")

	enModelPath := filepath.Join(ocrModelsDir, "rec_en_PP-OCRv3_infer.onnx")
	enDictPath := filepath.Join(ocrModelsDir, "dict_en.txt")

	switch lang {
	case "zh":
		enModelPath = ""
		enDictPath = ""
	case "en":
	case "auto":
	default:
		return nil, fmt.Errorf("unsupported language %q, use: auto, zh, en", lang)
	}

	if enModelPath != "" {
		if _, err := os.Stat(enModelPath); err != nil {
			enModelPath = ""
		}
	}
	if enDictPath != "" {
		if _, err := os.Stat(enDictPath); err != nil {
			enDictPath = ""
		}
	}

	return ocr.NewEngine(libPath, detPath, recPath, clsPath, dictPath, 6625, "softmax_11.tmp_0", enModelPath, enDictPath, lang)
}

// saveOCRResult saves an OCR result to a Markdown file and optionally previews it.
func saveOCRResult(cmd *cobra.Command, result *ocr.OCRResult, inputPath string) error {
	outPath := ""
	outExt := ".md"
	if inputPath != "" && inputPath != "stdin" {
		ext := filepath.Ext(inputPath)
		outPath = strings.TrimSuffix(inputPath, ext) + outExt
	} else {
		dir := options.Shared.OutputDir
		if dir == "" {
			dir = "."
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
		outPath = filepath.Join(dir, fmt.Sprintf("ocr_%d%s", time.Now().Unix(), outExt))
	}

	rawText := ""
	if len(result.Pages) == 0 || len(result.Pages[0].Lines) == 0 {
		rawText = "(no text detected)"
	} else {
		rawText = result.Text
	}

	if err := os.WriteFile(outPath, []byte(rawText+"\n"), 0644); err != nil {
		return fmt.Errorf("save output: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Saved: %s\n", outPath)

	if ocrScanPreview {
		if len(result.Pages) == 0 || len(result.Pages[0].Lines) == 0 {
			fmt.Println("(no text detected)")
			return nil
		}
		fmt.Print(rawText)
		if !strings.HasSuffix(rawText, "\n") {
			fmt.Println()
		}
	}

	return nil
}

// DefaultModelsDir returns the shared ONNX models directory (where ONNX Runtime lives).
func DefaultModelsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aigc-cli", "models")
}

// defaultOCRModelsDir returns the OCR models subdirectory.
func defaultOCRModelsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aigc-cli", "models", "ocr")
}

// resolveModelsDir returns the models directory from the provider (if configured),
// falling back to the default hardcoded path.
func resolveModelsDir(cmdName string) string {
	p := options.Shared.ResolveProvider(cmdName)
	if p != nil && p.ModelsDir != "" {
		return p.ModelsDir
	}
	return DefaultModelsDir()
}
