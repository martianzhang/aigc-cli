package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/service"
)

const modelsBaseURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1"

// modelInfo maps model identifier to download URL and filename.
// Add new models here. The key is used in `--model` flag and `detect.model` config.
var modelInfo = map[string]struct {
	url      string
	filename string
	desc     string // human-readable description
	size     string // human-readable download size
}{
	"vit-base": {
		url:      modelsBaseURL + "/model-vit-base.onnx",
		filename: "model-vit-base.onnx",
		desc:     "ViT-Base, 86M params",
		size:     "327MB",
	},
	"distilled-vit": {
		url:      modelsBaseURL + "/model-distilled-vit.onnx",
		filename: "model-distilled-vit.onnx",
		desc:     "distilled ViT, 11.8M params",
		size:     "56MB",
	},
}

// detectInitCmd represents the `aigc-cli detect init` subcommand.
var detectInitCmd = &cobra.Command{
	Use:          "init",
	Short:        "Download ONNX Runtime and AIGC detection model",
	SilenceUsage: true,
	Long: `Download the ONNX Runtime shared library and the AIGC detection model.

The runtime and model are saved to ~/.config/aigc-cli/models/ for offline
AIGC detection via the 'detect' command.

Use --model to choose which ONNX model to download:
  vit-base (default)       - ViT-Base, 86M params, 327MB
  distilled-vit            - distilled ViT, 11.8M params, 56MB

The model can also be set in config.yaml:
  detect:
    model: "distilled-vit"`,
	RunE: runDetectInit,
}

var (
	detectForce   bool
	detectModelID string
)

func runDetectInit(cmd *cobra.Command, args []string) error {
	// Resolve model: CLI flag > config > default "vit-base"
	modelID := detectModelID
	if !cmd.Flags().Changed("model") && shared.Cfg != nil && shared.Cfg.Detect != nil && shared.Cfg.Detect.Model != "" {
		modelID = shared.Cfg.Detect.Model
	}
	info, ok := modelInfo[modelID]
	if !ok {
		return fmt.Errorf("unknown model %q (choose: vit-base, distilled-vit)", modelID)
	}

	// ── ONNX Runtime (shared across all features) ──
	sharedDir := filepath.Join(configDir(), "models")
	if err := os.MkdirAll(sharedDir, 0755); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", sharedDir, err)
	}
	if _, err := onnxrt.EnsureInstalled(sharedDir, detectForce); err != nil {
		return err
	}
	if err := onnxrt.EnsureGPUInstalled(sharedDir, detectForce); err != nil {
		return err
	}

	// ── Download model ──
	modelsDir := detectModelsDir()
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", modelsDir, err)
	}
	modelPath := filepath.Join(modelsDir, info.filename)
	if _, err := os.Stat(modelPath); err == nil && !detectForce {
		fmt.Printf("Model already exists: %s\n  Use --force to re-download.\n", modelPath)
		return nil
	}

	fmt.Printf("Downloading %s model - %s (%s)...\n", modelID, info.desc, info.size)
	if err := service.SaveResource(info.url, modelPath); err != nil {
		return fmt.Errorf("model download failed: %w", err)
	}
	fmt.Println("  Done.")

	fmt.Println("\nDone! Run 'aigc-cli detect' to use AIGC detection.")
	return nil
}

func init() {
	detectCmd.AddCommand(detectInitCmd)
	detectInitCmd.Flags().BoolVar(&detectForce, "force", false, "re-download even if files already exist")
	detectInitCmd.Flags().StringVar(&detectModelID, "model", "vit-base", "ONNX model: vit-base (86M, default), distilled-vit (11.8M)")
}
