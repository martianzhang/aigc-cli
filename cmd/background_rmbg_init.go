package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/service"
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
	RunE:         runBackgroundInit,
}

var (
	bgInitForce bool
)

func runBackgroundInit(cmd *cobra.Command, args []string) error {
	// ONNX Runtime (shared across all features)
	sharedDir := filepath.Join(configDir(), "models")
	os.MkdirAll(sharedDir, 0755)
	if _, err := onnxrt.EnsureInstalled(sharedDir, bgInitForce); err != nil {
		return err
	}
	onnxrt.EnsureGPUInstalled(sharedDir, bgInitForce)

	// RMBG model
	modelsDir := rmbgModelsDir()
	os.MkdirAll(modelsDir, 0755)
	modelPath := filepath.Join(modelsDir, rmbgModelFilename)
	if _, err := os.Stat(modelPath); err == nil && !bgInitForce {
		fmt.Printf("RMBG model already exists: %s\n  Use --force to re-download.\n", modelPath)
		return nil
	}

	fmt.Printf("Downloading %s (%s)...\n", rmbgModelDesc, rmbgModelSize)
	if err := service.SaveResource(rmbgModelURL, modelPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	fmt.Println("  Done.")
	fmt.Println("\nRMBG 2.0 model installed!")
	return nil
}

func init() {
	backgroundCmd.AddCommand(backgroundInitCmd)
	backgroundInitCmd.Flags().BoolVar(&bgInitForce, "force", false, "re-download even if files already exist")
}
