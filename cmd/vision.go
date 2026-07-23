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
	Short:        "Local image understanding: describe images offline",
	SilenceUsage: true,
	Long: `Local image understanding powered by ONNX-based vision model (Florence-2).

Describes images in natural language. No API key required — runs entirely
on your machine.

Commands:
  init        Download vision model
  describe    Describe an image

Use 'aigc-cli vision <command> --help' for details.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(visionCmd)
	visionCmd.PersistentFlags().StringVar(&visionModelFlag, "model", vision.DefaultModelVariant,
		"vision model variant: base-int8 (default)")
}

var (
	visionModelFlag   string
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

Uses Florence-2 (MIT license) for image captioning.

Use --list to see available model variants.`,
	RunE: runVisionInit,
}

func runVisionInit(cmd *cobra.Command, args []string) error {
	if visionListFlag {
		vision.ListModels()
		return nil
	}

	variant := visionModelFlag
	if _, err := vision.ResolveModelVariant(variant); err != nil {
		return err
	}

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

	fmt.Printf("[2/2] Downloading vision model (%s)...\n", variant)
	if err := vision.InitModel(modelsDir, variant, visionForceFlag); err != nil {
		return fmt.Errorf("download vision model: %w", err)
	}

	fmt.Printf("\nDone! Run 'aigc-cli vision describe <image>' to describe an image.\n")
	return nil
}

// ─── vision describe ────────────────────────────────────────────────────

var visionDescribeCmd = &cobra.Command{
	Use:          "describe <image>",
	Aliases:      []string{"desc"},
	Short:        "Describe an image in natural language",
	SilenceUsage: true,
	Long: `Describe an image using the local vision model.

Generates a detailed caption for the given image.
Supports JPEG, PNG, WebP formats.

Examples:
  aigc-cli vision describe photo.jpg`,
	RunE: runVisionDescribe,
}

func runVisionDescribe(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no input file specified\n\nUsage:\n  aigc-cli vision describe <image>")
	}

	inputPath := args[0]
	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("file not found: %s", inputPath)
	}

	ext := strings.ToLower(filepath.Ext(inputPath))
	if ext == ".mp4" || ext == ".mov" || ext == ".avi" || ext == ".webm" || ext == ".mkv" {
		return fmt.Errorf("video input is not yet supported in this version")
	}

	if !isImageFile(inputPath) {
		return fmt.Errorf("unsupported file format: %s\nSupported formats: JPEG, PNG, WebP", ext)
	}

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

	if !vision.IsReady(modelsDir, variant) {
		return fmt.Errorf("vision model not found (%s)\n\nRun 'aigc-cli vision init' first", variant)
	}

	tk, err := vision.NewTokenizer(
		filepath.Join(vision.VariantDir(modelsDir, variant), "vocab.json"),
		filepath.Join(vision.VariantDir(modelsDir, variant), "merges.txt"),
	)
	if err != nil {
		return fmt.Errorf("load tokenizer: %w", err)
	}

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

	result, err := engine.Describe(inputPath)
	if err != nil {
		return fmt.Errorf("inference failed: %w", err)
	}

	if result == "" {
		result = "(no output generated)"
	}
	fmt.Println(result)
	return nil
}

func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	}
	return false
}

func init() {
	visionCmd.AddCommand(visionInitCmd)
	visionInitCmd.Flags().BoolVar(&visionListFlag, "list", false, "list available model variants")
	visionInitCmd.Flags().BoolVar(&visionForceFlag, "force", false, "re-download even if files already exist")

	visionCmd.AddCommand(visionDescribeCmd)
	visionDescribeCmd.Flags().IntVar(&visionMaxTokens, "max-tokens", 512, "maximum number of tokens to generate")
	visionDescribeCmd.Flags().Float64Var(&visionTemperature, "temperature", 0.0, "sampling temperature (0 = greedy)")
	visionDescribeCmd.Flags().IntVar(&visionTopK, "top-k", 0, "top-k sampling (0 = off)")
}
