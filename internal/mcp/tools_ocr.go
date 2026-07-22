package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/martianzhang/aigc-cli/internal/ocr"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/pdf"
)

// newOCRTextTool defines the ocr_text MCP tool.
func newOCRTextTool() mcp.Tool {
	return mcp.NewTool("ocr_text",
		mcp.WithDescription(`Recognize text in an image file using offline OCR.

Completely offline — no API key needed. Uses ONNX Runtime + PP-OCRv4 model
(DBNet detection + CRNN recognition).

Prerequisite: Run "aigc-cli ocr init" to download the model first.

Supports: JPEG, PNG, GIF, BMP, WebP, and PDF files.
- For text-based PDFs: extracts text directly without OCR.
- For scanned PDFs: renders pages to images first, then OCRs each page.

Output: Recognized text as Markdown. Use output_format="json" for structured
output with bounding boxes and confidence scores.`),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path to the image or PDF file"),
		),
		mcp.WithString("lang",
			mcp.Description("Language: auto (default), zh (Chinese only), en (English only)"),
		),
		mcp.WithString("output_format",
			mcp.Description("Output format: markdown (default) or json"),
		),
	)
}

// ocrTextHandler handles the ocr_text tool call.
func ocrTextHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, err := req.RequireString("file_path")
		if err != nil {
			return mcp.NewToolResultError("file_path is required"), nil
		}
		if !filepath.IsAbs(filePath) {
			abs, err := filepath.Abs(filePath)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid path: %v", err)), nil
			}
			filePath = abs
		}

		if _, err := os.Stat(filePath); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("file not found: %s", filePath)), nil
		}

		lang := req.GetString("lang", "auto")
		outputFormat := req.GetString("output_format", "markdown")

		modelsDir := filepath.Join(configDir(), "models")
		ocrModelsDir := filepath.Join(modelsDir, "ocr")

		libPath, err := onnxrt.LibPath(modelsDir)
		if err != nil {
			libPath, err = onnxrt.EnsureInstalled(modelsDir, false)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf(
					"ONNX Runtime not available. Run 'aigc-cli ocr init' first: %v", err)), nil
			}
		}

		detPath := filepath.Join(ocrModelsDir, "ch_PP-OCRv4_det_infer.onnx")
		recPath := filepath.Join(ocrModelsDir, "ch_PP-OCRv4_rec_infer.onnx")
		dictPath := filepath.Join(ocrModelsDir, "dict_zh.txt")
		clsPath := filepath.Join(ocrModelsDir, "ch_ppocr_mobile_v2.0_cls_infer.onnx")
		enModelPath := filepath.Join(ocrModelsDir, "rec_en_PP-OCRv3_infer.onnx")
		enDictPath := filepath.Join(ocrModelsDir, "dict_en.txt")

		if _, err := os.Stat(detPath); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf(
				"Detection model not found. Run 'aigc-cli ocr init' first: %v", err)), nil
		}
		if _, err := os.Stat(recPath); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf(
				"Recognition model not found. Run 'aigc-cli ocr init' first: %v", err)), nil
		}

		if lang == "zh" {
			enModelPath = ""
			enDictPath = ""
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

		engine, err := ocr.NewEngine(libPath, detPath, recPath, clsPath, dictPath,
			6625, "softmax_11.tmp_0", enModelPath, enDictPath, lang)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to create OCR engine: %v", err)), nil
		}
		defer engine.Close()

		var result *ocr.OCRResult

		if strings.EqualFold(filepath.Ext(filePath), ".pdf") {
			result, err = ocrPDF(engine, filePath)
		} else {
			f, openErr := os.Open(filePath)
			if openErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("cannot open file: %v", openErr)), nil
			}
			img, _, decodeErr := image.Decode(f)
			f.Close()
			if decodeErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("cannot decode image: %v", decodeErr)), nil
			}
			result, err = engine.Scan(img)
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("OCR failed: %v", err)), nil
		}

		if outputFormat == "json" {
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		}

		if result.Text == "" {
			return mcp.NewToolResultText("(no text detected)"), nil
		}
		return mcp.NewToolResultText(result.Text), nil
	}
}

// ocrPDF handles PDF input for the MCP OCR tool.
// Tries text extraction first; falls back to rendering + OCR for scanned PDFs.
func ocrPDF(engine *ocr.Engine, pdfPath string) (*ocr.OCRResult, error) {
	pages, err := pdf.ExtractText(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("read PDF: %w", err)
	}

	if !pdf.IsScanned(pages) {
		var textLines []string
		ocrPages := make([]ocr.OCRPage, 0, len(pages))
		for _, p := range pages {
			line := strings.TrimSpace(p.Text)
			if line != "" {
				textLines = append(textLines, line)
				ocrPages = append(ocrPages, ocr.OCRPage{
					Page: p.Page - 1,
					Lines: []ocr.OCRLine{{
						Text:       line,
						Confidence: 1.0,
					}},
				})
			}
		}
		return &ocr.OCRResult{
			Pages: ocrPages,
			Text:  strings.Join(textLines, "\n"),
		}, nil
	}

	tmpDir, err := os.MkdirTemp("", "aigc-cli-pdf-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	pngs, err := pdf.RenderToImages(pdfPath, tmpDir, 300)
	if err != nil {
		return nil, fmt.Errorf("render PDF: %w", err)
	}

	allPages := make([]ocr.OCRPage, 0, len(pngs))
	allText := make([]string, 0, len(pngs))

	for pageIdx, pngPath := range pngs {
		f, openErr := os.Open(pngPath)
		if openErr != nil {
			return nil, fmt.Errorf("open rendered page %d: %w", pageIdx+1, openErr)
		}
		img, _, decodeErr := image.Decode(f)
		f.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("decode rendered page %d: %w", pageIdx+1, decodeErr)
		}

		pageResult, scanErr := engine.Scan(img)
		if scanErr != nil {
			return nil, fmt.Errorf("OCR page %d: %w", pageIdx+1, scanErr)
		}
		for i := range pageResult.Pages {
			pageResult.Pages[i].Page = pageIdx
		}
		allPages = append(allPages, pageResult.Pages...)
		allText = append(allText, pageResult.Text)
	}

	return &ocr.OCRResult{
		Pages: allPages,
		Text:  strings.Join(allText, "\n"),
	}, nil
}
