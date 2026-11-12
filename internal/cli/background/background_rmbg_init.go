package background

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/modelstore"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

const (
	rmbgModelID       = "rmbg-2.0"
	rmbgModelFilename = "model-rmbg-2.0.onnx"
	rmbgModelURL      = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1/model-rmbg-2.0.onnx"
	rmbgModelDesc     = "RMBG 2.0 (BiRefNet), 176M params, INT8 quantized"
	rmbgModelSize     = "366MB"
)

var backgroundInitCmd = &cobra.Command{
	Use:          "init",
	Short:        "Download ONNX Runtime and RMBG 2.0 model",
	SilenceUsage: true,
	Long:         `Download the ONNX Runtime shared library and the RMBG 2.0 model.`,
	Example: `  aigc-cli background init           # download ONNX Runtime + RMBG 2.0 model
  aigc-cli background init --force   # re-download even if already installed`,
	RunE: runBackgroundInit,
}

var (
	bgInitForce bool
)

func runBackgroundInit(cmd *cobra.Command, args []string) error {
	// ONNX Runtime (shared across all features)
	sharedDir := filepath.Join(options.ConfigDir(), "models")
	if err := os.MkdirAll(sharedDir, 0755); err != nil {
		return fmt.Errorf("create models dir: %w", err)
	}
	if _, err := onnxrt.EnsureInstalled(sharedDir, bgInitForce); err != nil {
		return err
	}
	// GPU runtime is optional: the CPU runtime is enough for local inference.
	_ = onnxrt.EnsureGPUInstalled(sharedDir, bgInitForce)

	// RMBG model
	modelsDir := rmbgModelsDir()
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("create rmbg dir: %w", err)
	}
	modelPath := filepath.Join(modelsDir, rmbgModelFilename)
	if _, err := os.Stat(modelPath); err == nil && !bgInitForce {
		ui.Printf("RMBG model already exists: %s\n  Use --force to re-download.\n", modelPath)
		return nil
	}

	ui.Printf("Downloading %s (%s)...\n", rmbgModelDesc, rmbgModelSize)
	if err := modelstore.Install(options.Context(), nil, rmbgModelURL, modelPath, modelstore.Options{Force: bgInitForce}); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	ui.Println("  Done.")
	ui.Println("\nRMBG 2.0 model installed!")
	return nil
}

func init() {
	backgroundCmd.AddCommand(backgroundInitCmd)
	backgroundInitCmd.Flags().BoolVar(&bgInitForce, "force", false, "re-download even if files already exist")
}
