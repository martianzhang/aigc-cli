package ocr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/ocr"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// newOCREngine creates an OCR engine with the standard model setup and the
// global ocrScanLang setting.
func newOCREngine(modelsDir string) (*ocr.Engine, error) {
	return NewOCREngineWithLang(modelsDir, ocrScanLang)
}

// NewOCREngineWithLang creates an OCR engine with the given language setting.
func NewOCREngineWithLang(modelsDir, lang string) (*ocr.Engine, error) {
	return ocr.NewDefaultEngine(modelsDir, lang)
}

// saveOCRResult saves an OCR result to a Markdown file and optionally previews it.
// With `-o -` the text goes to stdout and no file is written.
func saveOCRResult(_ *cobra.Command, result *ocr.OCRResult, inputPath string) error {
	rawText := ""
	if len(result.Pages) == 0 || len(result.Pages[0].Lines) == 0 {
		rawText = "(no text detected)"
	} else {
		rawText = result.Text
	}

	if writesToStdout(options.Shared.OutputDir) {
		printText(rawText)
		return nil
	}

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

	if err := os.WriteFile(outPath, []byte(rawText+"\n"), 0644); err != nil {
		return fmt.Errorf("save output: %w", err)
	}
	ui.Eprintf("Saved: %s\n", outPath)

	if ocrScanPreview {
		if len(result.Pages) == 0 || len(result.Pages[0].Lines) == 0 {
			ui.Println("(no text detected)")
			return nil
		}
		ui.Print(rawText)
		if !strings.HasSuffix(rawText, "\n") {
			ui.Println()
		}
	}

	return nil
}

// writesToStdout reports whether OCR output should go to stdout instead of a
// .md file. `-o -` is the conventional stdout marker.
func writesToStdout(outputDir string) bool {
	return outputDir == "-"
}

// printText writes OCR text to stdout, guaranteeing exactly one trailing newline.
func printText(text string) {
	ui.Print(text)
	if !strings.HasSuffix(text, "\n") {
		ui.Println()
	}
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
