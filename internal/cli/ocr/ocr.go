package ocr

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/ocr"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// ocrCmd represents the `aigc-cli ocr` command group.
var ocrCmd = &cobra.Command{
	Use:          "ocr",
	Short:        "Offline OCR text recognition",
	SilenceUsage: true,
	Long: `Offline OCR text recognition using ONNX Runtime.
	
Detection (DBNet) + Recognition (CRNN/SVTR) pipeline, fully local.
No API key or internet connection required after model download.`,
	Example: `  aigc-cli ocr init                             # download OCR models first
  aigc-cli ocr scan invoice.png
  aigc-cli ocr scan invoice.png --json          # bounding boxes + confidence
  aigc-cli ocr scan report.pdf --pages "1-3"    # PDF pages, --engine auto|pdf|ocr`,
}

// ocrInitCmd represents `aigc-cli ocr init`.
var ocrInitCmd = &cobra.Command{
	Use:          "init",
	Short:        "Download OCR models",
	SilenceUsage: true,
	Example: `  aigc-cli ocr init                    # download the default model pack
  aigc-cli ocr init --list             # list available model packs
  aigc-cli ocr init --list-installed   # list installed packs`,
	RunE: runOCRInit,
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

var ocrScanJSON bool

var ocrScanPages string

var ocrScanSpellcheck bool

var ocrScanModel string // --model for online OCR override

var ocrScanPrompt string // --prompt for custom OCR prompt

var ocrScanEngine string // --engine: auto, pdf, ocr

func init() {
	ocrInitCmd.Flags().Bool("list", false, "List available model packs")
	ocrInitCmd.Flags().Bool("list-installed", false, "List installed model packs")

	ocrScanCmd.Flags().BoolVar(&ocrScanPreview, "preview", false, "Preview recognized text in terminal")
	ocrScanCmd.Flags().StringVar(&ocrScanLang, "lang", "auto", "Language: auto, zh (Chinese), en (English)")
	ocrScanCmd.Flags().BoolVar(&ocrScanJSON, "json", false, "Output as JSON with bounding boxes and confidence scores")
	ocrScanCmd.Flags().StringVar(&ocrScanPages, "pages", "", "Page range for PDF input (e.g. \"1-3,5\")")
	ocrScanCmd.Flags().BoolVar(&ocrScanSpellcheck, "spellcheck", true, "Auto-correct spelling errors using dictionary")
	ocrScanCmd.Flags().StringVar(&ocrScanModel, "model", "", "Model name for online OCR (overrides defaults.ocr.model)")
	ocrScanCmd.Flags().StringVarP(&ocrScanPrompt, "prompt", "p", "", `Custom prompt for online OCR. Overrides model default.
Examples:
  --prompt "Free OCR."                                  (deepseek-ocr)
  --prompt "<|grounding|>Convert the document to markdown." (deepseek-ocr, layout-aware markdown)
  --prompt "Table Recognition:"                         (glm-ocr, table mode)
  --prompt "Figure Recognition:"                        (glm-ocr, figure mode)
  --prompt "请识别图中的文字"                               (general)`)
	ocrScanCmd.Flags().StringVar(&ocrScanPrompt, "ask", "", "Alias for --prompt")
	ocrScanCmd.Flags().StringVar(&ocrScanEngine, "engine", "auto", "Processing engine: auto, pdf (rule-based), ocr")
	ocrCmd.AddCommand(ocrInitCmd)
	ocrCmd.AddCommand(ocrScanCmd)
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

	// Download English dictionary for word splitting & spellcheck.
	if err := downloadDict(modelsDir); err != nil {
		fmt.Printf("  ⚠ %v\n", err)
	}

	fmt.Println("\nOCR models installed. Run 'aigc-cli ocr scan <image>' to test.")
	return nil
}

// downloadDict downloads the English word list for OCR text post-processing.
// The file should be uploaded to aigc-cli-models release assets as dict_en_words.txt.
// Until it's available, OCR works without word splitting (uses system dict if present).
func downloadDict(modelsDir string) error {
	dictPath := filepath.Join(modelsDir, "dict_en_words.txt")
	if _, err := os.Stat(dictPath); err == nil {
		fmt.Println("  ✓ dict_en_words.txt (already exists)")
		return nil
	}
	fmt.Println("  Downloading dict_en_words.txt...")
	if err := service.SaveResource(
		"https://github.com/martianzhang/aigc-cli-models/releases/download/v1/dict_en_words.txt",
		dictPath,
	); err != nil {
		return fmt.Errorf("dict_en_words.txt not available yet (OCR works without it): %w", err)
	}
	fmt.Println("  ✓ dict_en_words.txt")
	return nil
}

// Cmd returns the ocr command tree.
func Cmd() *cobra.Command { return ocrCmd }
