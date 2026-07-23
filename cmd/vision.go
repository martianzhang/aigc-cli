package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/vision"
)

// ─── vision root command ────────────────────────────────────────────────

var visionCmd = &cobra.Command{
	Use:          "vision",
	Short:        "Local image understanding: describe and ask questions about images",
	SilenceUsage: true,
	Long: `Local image understanding powered by ONNX-based vision models (Florence-2).

Offline image captioning and visual question answering (VQA).
No API key required — runs entirely on your machine.

Commands:
  init        Download vision model
  describe    Describe an image, or ask a question with --ask

Use 'aigc-cli vision <command> --help' for details.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(visionCmd)

	// Persistent flags for vision subcommands
	visionCmd.PersistentFlags().StringVar(&visionModelFlag, "model", vision.DefaultModelVariant,
		"vision model variant: base-int8 (default), base-fp16, large")
}

var (
	visionModelFlag   string
	visionAskFlag     string
	visionListFlag    bool
	visionForceFlag   bool
	visionMaxTokens   int
	visionTemperature float64
	visionTopK        int
)

// ─── vision init ────────────────────────────────────────────────────────

var visionInitCmd = &cobra.Command{
	Use:          "init",
	Short:        "Download vision model",
	SilenceUsage: true,
	Long: `Download the ONNX vision model and tokenizer files for local image understanding.

Models are saved to ~/.config/aigc-cli/models/vision/<variant>/.

Uses Florence-2 (MIT license) for image captioning and VQA.

Use --list to see available model variants.`,
	RunE: runVisionInit,
}

func runVisionInit(cmd *cobra.Command, args []string) error {
	// --list mode
	if visionListFlag {
		vision.ListModels()
		return nil
	}

	// Resolve model variant
	variant := visionModelFlag
	if _, err := vision.ResolveModelVariant(variant); err != nil {
		return err
	}

	// ── ONNX Runtime (shared across all features) ──
	modelsDir := vision.DefaultModelsDir()
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("create models directory: %w", err)
	}

	fmt.Println("[1/2] Ensuring ONNX Runtime...")
	if _, err := onnxrt.EnsureInstalled(modelsDir, visionForceFlag); err != nil {
		return err
	}
	if err := onnxrt.EnsureGPUInstalled(modelsDir, visionForceFlag); err != nil {
		return err
	}

	// ── Download vision model ──
	fmt.Printf("[2/2] Downloading vision model (%s)...\n", variant)
	if err := vision.InitModel(modelsDir, variant, visionForceFlag); err != nil {
		return fmt.Errorf("download vision model: %w", err)
	}

	fmt.Printf("\nDone! Run 'aigc-cli vision describe <image>' to describe an image,\n")
	fmt.Printf("or 'aigc-cli vision describe <image> --ask \"<question>\"' to ask a question.\n")
	return nil
}

// ─── vision describe ────────────────────────────────────────────────────

var visionDescribeCmd = &cobra.Command{
	Use:          "describe <image> [--ask \"question\"]",
	Aliases:      []string{"desc"},
	Short:        "Describe an image or answer a question about it",
	SilenceUsage: true,
	Long: `Describe an image using the local vision model.

Generates a detailed caption by default. Use --ask to ask a specific question
about the image (Visual Question Answering).

Supports images (JPEG, PNG, WebP).

Examples:
  aigc-cli vision describe photo.jpg
  aigc-cli vision describe photo.jpg --ask "What color is the car?"
  aigc-cli vision describe demo.mp4 --ask "Is the person holding a phone?"`,
	RunE: runVisionDescribe,
}

func runVisionDescribe(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no input file specified\n\nUsage:\n  aigc-cli vision describe <image> [--ask \"question\"]")
	}

	inputPath := args[0]

	// Check if file exists
	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("file not found: %s", inputPath)
	}

	// Check for video input (future support)
	ext := strings.ToLower(filepath.Ext(inputPath))
	isVideo := ext == ".mp4" || ext == ".mov" || ext == ".avi" || ext == ".webm" || ext == ".mkv"
	if isVideo {
		return fmt.Errorf("video input is not yet supported in this version")
	}

	// Verify image format (basic check)
	if !isImageFile(inputPath) {
		return fmt.Errorf("unsupported file format: %s\nSupported formats: JPEG, PNG, WebP", ext)
	}

	// ── Initialize engine ──
	modelsDir := vision.DefaultModelsDir()
	libPath, err := onnxrt.LibPath(modelsDir)
	if err != nil {
		libPath, err = onnxrt.EnsureInstalled(modelsDir, false)
		if err != nil {
			return fmt.Errorf("ONNX Runtime not available: %w\n\nRun 'aigc-cli vision init' first", err)
		}
	}

	variant := visionModelFlag
	if _, err := vision.ResolveModelVariant(variant); err != nil {
		return err
	}

	// Check models exist
	if !vision.IsReady(modelsDir, variant) {
		return fmt.Errorf("vision model not found (%s)\n\nRun 'aigc-cli vision init' first", variant)
	}

	// Load tokenizer
	tk, err := vision.NewTokenizer(
		filepath.Join(vision.VariantDir(modelsDir, variant), "vocab.json"),
		filepath.Join(vision.VariantDir(modelsDir, variant), "merges.txt"),
	)
	if err != nil {
		return fmt.Errorf("load tokenizer: %w", err)
	}

	// Create engine
	engine, err := vision.NewEngine(&vision.EngineConfig{
		ModelsDir:   modelsDir,
		Variant:     variant,
		LibPath:     libPath,
		Tokenizer:   tk,
		MaxTokens:   visionMaxTokens,
		Temperature: visionTemperature,
		TopK:        visionTopK,
	})
	if err != nil {
		return fmt.Errorf("create vision engine: %w", err)
	}
	defer engine.Close()

	// ── Run inference ──
	var result string
	if visionAskFlag != "" {
		// VQA mode
		result, err = engine.Ask(inputPath, visionAskFlag)
	} else {
		// Caption mode
		result, err = engine.Describe(inputPath)
	}
	if err != nil {
		return fmt.Errorf("inference failed: %w", err)
	}

	// ── Output ──
	if result == "" {
		result = "(no output generated)"
	}
	fmt.Println(result)

	return nil
}

// isImageFile checks whether the path points to a supported image file.
func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	}
	return false
}

func init() {
	// vision init
	visionCmd.AddCommand(visionInitCmd)
	visionInitCmd.Flags().BoolVar(&visionListFlag, "list", false, "list available model variants")
	visionInitCmd.Flags().BoolVar(&visionForceFlag, "force", false, "re-download even if files already exist")

	// vision describe
	visionCmd.AddCommand(visionDescribeCmd)
	visionDescribeCmd.Flags().StringVarP(&visionAskFlag, "ask", "a", "", "ask a question about the image (VQA mode)")
	visionDescribeCmd.Flags().IntVar(&visionMaxTokens, "max-tokens", 512, "maximum number of tokens to generate")
	visionDescribeCmd.Flags().Float64Var(&visionTemperature, "temperature", 0.0, "sampling temperature (0 = greedy)")
	visionDescribeCmd.Flags().IntVar(&visionTopK, "top-k", 0, "top-k sampling (0 = off)")
}
