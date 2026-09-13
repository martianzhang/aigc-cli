package cmd

import (
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

	"github.com/martianzhang/aigc-cli/internal/ocr"
	"github.com/martianzhang/aigc-cli/internal/pdf"
	"github.com/martianzhang/aigc-cli/internal/provider"
)

// scanPDF handles PDF input: tries text extraction first; falls back to OCR
// for scanned/image-based PDFs.
func scanPDF(cmd *cobra.Command, pdfPath string) error {
	pages, err := pdf.ExtractText(pdfPath)
	if err != nil {
		return fmt.Errorf("read PDF: %w", err)
	}

	isNativePDF := !pdf.IsScanned(pages)

	engine := ocrScanEngine
	if engine == "auto" {
		if isNativePDF {
			engine = "pdf"
		} else {
			engine = "ocr"
		}
	}

	if engine == "pdf" && isNativePDF {
		return scanPDFWithLayout(cmd, pdfPath)
	}

	if isNativePDF {
		var textLines []string
		for _, p := range pages {
			line := strings.TrimSpace(p.Text)
			if line != "" {
				textLines = append(textLines, line)
			}
		}
		rawText := strings.Join(textLines, "\n")

		ocrPages := make([]ocr.OCRPage, 0, len(pages))
		for _, p := range pages {
			line := strings.TrimSpace(p.Text)
			if line != "" {
				ocrPages = append(ocrPages, ocr.OCRPage{
					Page: p.Page - 1,
					Lines: []ocr.OCRLine{{
						Text:       line,
						Confidence: 1.0,
					}},
				})
			}
		}

		result := &ocr.OCRResult{
			Pages: ocrPages,
			Text:  rawText,
		}
		return saveOCRResult(cmd, result, pdfPath)
	}

	// Scanned PDF: render to images, then OCR (online or local).
	tmpDir, err := os.MkdirTemp("", "aigc-cli-pdf-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	var pngs []string
	if ocrScanPages != "" {
		pageNums := parsePageRange(ocrScanPages)
		pngs, err = pdf.SelectedPages(pdfPath, tmpDir, pageNums, 300)
	} else {
		pngs, err = pdf.RenderToImages(pdfPath, tmpDir, 300)
	}
	if err != nil {
		return fmt.Errorf("render PDF: %w", err)
	}

	allPages := make([]ocr.OCRPage, 0, len(pngs))
	allText := make([]string, 0, len(pngs))

	useOnlineOCR := false
	var onlineP *provider.EffectiveProvider
	op := shared.ResolveProvider(ProviderNameOCR)
	if provider.IsOnlineProvider(op) {
		if ocrScanModel != "" {
			op.Model = ocrScanModel
		} else if shared.Model != "" {
			op.Model = shared.Model
		}
		if op.Model != "" {
			useOnlineOCR = true
			onlineP = op
		}
	}

	if useOnlineOCR {
		for pageIdx, pngPath := range pngs {
			text, err := provider.OCRImage(onlineP, pngPath, ocrScanPrompt)
			if err != nil {
				return fmt.Errorf("online OCR page %d: %w", pageIdx+1, err)
			}
			allPages = append(allPages, ocr.OCRPage{
				Page: pageIdx,
				Lines: []ocr.OCRLine{{
					Text:       text,
					Confidence: 1.0,
				}},
			})
			allText = append(allText, text)
		}
	} else {
		// Local ONNX OCR for each page
		modelsDir := resolveModelsDir(ProviderNameOCR)
		engine, err := newOCREngine(modelsDir)
		if err != nil {
			return err
		}
		defer engine.Close()

		for pageIdx, pngPath := range pngs {
			f, err := os.Open(pngPath)
			if err != nil {
				return fmt.Errorf("open rendered page %d: %w", pageIdx+1, err)
			}
			img, _, err := image.Decode(f)
			f.Close()
			if err != nil {
				return fmt.Errorf("decode rendered page %d: %w", pageIdx+1, err)
			}

			result, err := engine.Scan(img)
			if err != nil {
				return fmt.Errorf("OCR page %d: %w", pageIdx+1, err)
			}

			for i := range result.Pages {
				result.Pages[i].Page = pageIdx
			}
			allPages = append(allPages, result.Pages...)
			allText = append(allText, result.Text)
		}
	}

	result := &ocr.OCRResult{
		Pages: allPages,
		Text:  strings.Join(allText, "\n"),
	}
	return saveOCRResult(cmd, result, pdfPath)
}

// parsePageRange parses a page range string like "1-3,5,7-9" into a slice of
// page number strings for pdfcpu. Returns all pages as ["1"]..["N"] if the
// input is empty.
func parsePageRange(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// pdfcpu's RenderPagesForPDFFile accepts comma-separated page specs.
	// Split by comma and return each token as-is (handles "1-3", "5", etc.)
	var result []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func scanPDFWithLayout(cmd *cobra.Command, pdfPath string) error {
	blocks, err := pdf.ExtractTextWithLayout(pdfPath)
	if err != nil {
		return fmt.Errorf("extract PDF layout: %w", err)
	}

	markdown := pdf.BlocksToMarkdown(blocks)

	outPath := strings.TrimSuffix(pdfPath, filepath.Ext(pdfPath)) + ".md"
	if err := os.WriteFile(outPath, []byte(markdown), 0644); err != nil {
		return fmt.Errorf("save output: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Saved: %s\n", outPath)

	if ocrScanPreview {
		fmt.Print(markdown)
	}

	return nil
}
