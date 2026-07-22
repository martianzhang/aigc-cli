package cmd

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/ocr"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/pdf"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/spf13/cobra"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// ocrCmd represents the `aigc-cli ocr` command group.
var ocrCmd = &cobra.Command{
	Use:          "ocr",
	Short:        "Offline OCR text recognition",
	SilenceUsage: true,
	Long: `Offline OCR text recognition using ONNX Runtime.
	
Detection (DBNet) + Recognition (CRNN/SVTR) pipeline, fully local.
No API key or internet connection required after model download.`,
}

// ocrInitCmd represents `aigc-cli ocr init`.
var ocrInitCmd = &cobra.Command{
	Use:          "init",
	Short:        "Download OCR models",
	SilenceUsage: true,
	RunE:         runOCRInit,
}

// ocrScanCmd represents `aigc-cli ocr scan`.
var ocrScanCmd = &cobra.Command{
	Use:          "scan [image]",
	Short:        "Recognize text in an image",
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runOCRScan,
}

var ocrScanPreview bool
var ocrScanLang string

func init() {
	ocrInitCmd.Flags().Bool("list", false, "List available model packs")
	ocrInitCmd.Flags().Bool("list-installed", false, "List installed model packs")

	ocrScanCmd.Flags().BoolVar(&ocrScanPreview, "preview", false, "Preview recognized text in terminal")
	ocrScanCmd.Flags().StringVar(&ocrScanLang, "lang", "auto", "Language: auto, zh (Chinese), en (English)")

	ocrCmd.AddCommand(ocrInitCmd)
	ocrCmd.AddCommand(ocrScanCmd)
	rootCmd.AddCommand(ocrCmd)
}

func runOCRInit(cmd *cobra.Command, args []string) error {
	listModels, _ := cmd.Flags().GetBool("list")
	listInstalled, _ := cmd.Flags().GetBool("list-installed")

	if listModels {
		fmt.Println("Available OCR model pack:")
		for _, m := range ocr.Models() {
			fmt.Printf("  %-20s  %s\n", m.ID, m.Description)
		}
		return nil
	}

	if listInstalled {
		modelsDir := defaultOCRModelsDir()
		fmt.Printf("Installed models in %s:\n", modelsDir)
		entries, err := os.ReadDir(modelsDir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("  (none installed)")
				return nil
			}
			return fmt.Errorf("read models dir: %w", err)
		}
		if len(entries) == 0 {
			fmt.Println("  (none installed)")
			return nil
		}
		for _, e := range entries {
			info, _ := e.Info()
			size := info.Size() / (1024 * 1024)
			if size == 0 {
				size = 1
			}
			fmt.Printf("  %-40s  %d MB\n", e.Name(), size)
		}
		return nil
	}

	modelPack, ok := ocr.FindModelByID("rapidocr")
	if !ok {
		return fmt.Errorf("unknown model pack: rapidocr")
	}

	modelsDir := defaultOCRModelsDir()
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("create models dir: %w", err)
	}

	fmt.Printf("Downloading %s...\n", modelPack.Name)
	for _, f := range modelPack.Files {
		outPath := filepath.Join(modelsDir, f.OutName)
		if _, err := os.Stat(outPath); err == nil {
			fmt.Printf("  ✓ %s (already exists)\n", f.OutName)
			continue
		}
		fmt.Printf("  Downloading %s (%d MB)...\n", f.OutName, f.SizeMB)
		if err := service.SaveResource(f.URL, outPath); err != nil {
			return fmt.Errorf("download %s: %w", f.OutName, err)
		}
		fmt.Printf("  ✓ %s\n", f.OutName)
	}

	fmt.Println("\nOCR models installed. Run 'aigc-cli ocr scan <image>' to test.")
	return nil
}

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
		// Read image from stdin
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

	// Handle PDF input
	if strings.EqualFold(filepath.Ext(inputPath), ".pdf") {
		return scanPDF(cmd, inputPath)
	}

	// Try to decode as image
	f, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("unsupported image format: %w\n\nSupported formats: JPEG, PNG, GIF, BMP, WebP", err)
	}
	_ = format
	f.Close()

	return scanImage(cmd, img, inputPath)
}

func scanImage(cmd *cobra.Command, img image.Image, inputPath string) error {
	modelsDir := defaultModelsDir()
	engine, err := newOCREngine(modelsDir)
	if err != nil {
		return err
	}
	defer engine.Close()

	result, err := engine.Scan(img)
	if err != nil {
		return fmt.Errorf("OCR scan failed: %w", err)
	}

	return saveOCRResult(cmd, result, inputPath)
}

// scanPDF handles PDF input: tries text extraction first; falls back to OCR
// for scanned/image-based PDFs.
func scanPDF(cmd *cobra.Command, pdfPath string) error {
	// Try text extraction first
	pages, err := pdf.ExtractText(pdfPath)
	if err != nil {
		return fmt.Errorf("read PDF: %w", err)
	}

	if !pdf.IsScanned(pages) {
		// Text-based PDF: use extracted text directly.
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

	pngs, err := renderPDFPages(pdfPath, 300)
	if err != nil {
		return err
	}
	defer os.RemoveAll(filepath.Dir(pngs[0]))

	allPages := make([]ocr.OCRPage, 0, len(pngs))
	allText := make([]string, 0, len(pngs))
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

		modelsDir := defaultModelsDir()
		engine, err := newOCREngine(modelsDir)
		if err != nil {
			return err
		}
		result, err := engine.Scan(img)
		engine.Close()
		if err != nil {
			return fmt.Errorf("OCR page %d: %w", pageIdx+1, err)
		}
		allPages = append(allPages, result.Pages...)
		allText = append(allText, result.Text)
	}

	result := &ocr.OCRResult{
		Pages: allPages,
		Text:  strings.Join(allText, "\n"),
	}
	return saveOCRResult(cmd, result, pdfPath)
}

func renderPDFPages(pdfPath string, dpi int) ([]string, error) {
	if _, err := exec.LookPath("mutool"); err != nil {
		return nil, errors.New(mutoolInstallHint())
	}

	tmpDir, err := os.MkdirTemp("", "aigc-cli-pdf-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	outPattern := filepath.Join(tmpDir, "page-%d.png")
	cmd := exec.Command("mutool", "draw",
		"-o", outPattern,
		"-r", strconv.Itoa(dpi),
		pdfPath,
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("mutool draw failed: %w", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("read output: %w", err)
	}

	var images []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".png") {
			images = append(images, filepath.Join(tmpDir, e.Name()))
		}
	}
	if len(images) == 0 {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("mutool produced no output images")
	}
	return images, nil
}

func mutoolInstallHint() string {
	return `this PDF appears to be a scanned document (no extractable text layer).
To OCR it, install mupdf-tools (mutool) to convert PDF pages to images:

  macOS:  brew install mupdf-tools
  Linux:  apt install mupdf-tools    # or pacman -S mupdf-tools
  Windows: https://mupdf.com/downloads

Then run 'aigc-cli ocr scan <file>' again.`
}

// newOCREngine creates an OCR engine with the standard model setup.
func newOCREngine(modelsDir string) (*ocr.Engine, error) {
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

	switch ocrScanLang {
	case "zh":
		enModelPath = ""
		enDictPath = ""
	case "en":
	case "auto":
	default:
		return nil, fmt.Errorf("unsupported language %q, use: auto, zh, en", ocrScanLang)
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

	return ocr.NewEngine(libPath, detPath, recPath, clsPath, dictPath, 6625, "softmax_11.tmp_0", enModelPath, enDictPath, ocrScanLang)
}

// saveOCRResult saves an OCR result to a markdown file and optionally previews it.
func saveOCRResult(cmd *cobra.Command, result *ocr.OCRResult, inputPath string) error {
	outPath := ""
	if inputPath != "" && inputPath != "stdin" {
		ext := filepath.Ext(inputPath)
		outPath = strings.TrimSuffix(inputPath, ext) + ".md"
	} else {
		dir := shared.OutputDir
		if dir == "" {
			dir = "."
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
		outPath = filepath.Join(dir, fmt.Sprintf("ocr_%d.md", time.Now().Unix()))
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

// defaultModelsDir returns the shared ONNX models directory (where ONNX Runtime lives).
func defaultModelsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aigc-cli", "models")
}

// defaultOCRModelsDir returns the OCR models subdirectory.
func defaultOCRModelsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aigc-cli", "models", "ocr")
}
