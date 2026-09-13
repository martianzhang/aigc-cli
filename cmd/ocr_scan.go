package cmd

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/martianzhang/aigc-cli/internal/provider"
)

func runOCRScan(cmd *cobra.Command, args []string) error {
	// Determine input path
	inputPath := ""
	if len(args) > 0 {
		inputPath = args[0]
	}

	// If no args, try stdin
	if inputPath == "" {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return errors.New("no input file specified and stdin is a terminal\n\nUsage:\n  aigc-cli ocr scan <image>\n  cat image.png | aigc-cli ocr scan")
		}
		// Read image from stdin — for online OCR we need a temp file, for now fall back to local
		img, _, err := image.Decode(os.Stdin)
		if err != nil {
			return fmt.Errorf("decode stdin image: %w", err)
		}
		return scanImage(cmd, img, "stdin")
	}

	// Check if file exists
	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("file not found: %s", inputPath)
	}

	// Handle PDF input — skip online OCR, use text extraction or scan
	if strings.EqualFold(filepath.Ext(inputPath), ".pdf") {
		return scanPDF(cmd, inputPath)
	}

	// ── Online OCR via LLM provider ──
	// Only activate when explicitly configured via defaults.ocr.provider
	// or --provider flag (p.Name non-empty), OR when type is ollama.
	// Global fallback (empty p.Name, type=openai) skips online mode.
	p := shared.ResolveProvider(ProviderNameOCR)
	if provider.IsOnlineProvider(p) {
		// Model priority: --model flag > p.Model (from provider config)
		if ocrScanModel != "" {
			p.Model = ocrScanModel
		} else if shared.Model != "" {
			p.Model = shared.Model
		}
		if p.Model == "" {
			return fmt.Errorf("model is required for online OCR: set via --model flag or providers.%s.model in config.yaml", p.Name)
		}
		text, err := provider.OCRImage(p, inputPath, ocrScanPrompt)
		if err != nil {
			return fmt.Errorf("online OCR failed: %w", err)
		}
		outPath := strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ".md"
		if err := os.WriteFile(outPath, []byte(text+"\n"), 0644); err != nil {
			return fmt.Errorf("save output: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Saved: %s\n", outPath)
		if ocrScanPreview {
			fmt.Print(text)
			if !strings.HasSuffix(text, "\n") {
				fmt.Println()
			}
		}
		return nil
	}

	// Try to decode as image
	f, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("unsupported image format: %w\n\nSupported formats: JPEG, PNG, GIF, BMP, WebP, AVIF, HEIC, JXL", err)
	}
	_ = format
	f.Close()

	return scanImage(cmd, img, inputPath)
}

func scanImage(cmd *cobra.Command, img image.Image, inputPath string) error {
	modelsDir := resolveModelsDir(ProviderNameOCR)
	engine, err := newOCREngine(modelsDir)
	if err != nil {
		return err
	}
	defer engine.Close()

	result, err := engine.Scan(img)
	if err != nil {
		return fmt.Errorf("OCR scan failed: %w", err)
	}

	// Apply spellcheck to final text and individual lines
	if ocrScanSpellcheck {
		result.Text = engine.SpellcheckText(result.Text)
		for pi := range result.Pages {
			for li := range result.Pages[pi].Lines {
				result.Pages[pi].Lines[li].Text = engine.SpellcheckText(result.Pages[pi].Lines[li].Text)
			}
		}
	}

	return saveOCRResult(cmd, result, inputPath)
}
