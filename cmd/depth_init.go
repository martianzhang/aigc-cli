package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/depth"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// depth model download URLs (mirrored to the aigc-cli-models release).
const (
	depthSmallURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1/depth-anything-v2-small.onnx"
	depthBaseURL  = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1/depth-anything-v2-base.onnx"
	depthLargeURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1/depth-anything-v2-large.onnx"
)

var videoInitCmd = &cobra.Command{
	Use:          "init",
	Short:        "Download ONNX Runtime and depth estimation models",
	SilenceUsage: true,
	Long:         `Download the ONNX Runtime shared library and Depth Anything V2 models used by video --convert-to-depth.`,
	RunE:         runVideoInit,
}

var (
	videoInitForce bool
	videoInitModel string
	videoInitAll   bool
)

func runVideoInit(cmd *cobra.Command, args []string) error {
	sharedDir := filepath.Join(configDir(), "models")
	os.MkdirAll(sharedDir, 0755)
	if _, err := onnxrt.EnsureInstalled(sharedDir, videoInitForce); err != nil {
		return err
	}
	onnxrt.EnsureGPUInstalled(sharedDir, videoInitForce)

	// Which models to download: default only the default model;
	// --model selects one, --all downloads every variant.
	ids := []string{depth.DefaultModelID}
	if videoInitAll {
		ids = depth.ListModelIDs()
	} else if videoInitModel != "" {
		info, ok := depth.ResolveModel(videoInitModel)
		if !ok {
			return fmt.Errorf("unknown depth model %q (available: %s)", videoInitModel, strings.Join(depth.ListModelIDs(), ", "))
		}
		ids = []string{info.ID}
	}

	modelsDir := filepath.Join(sharedDir, "depth")
	os.MkdirAll(modelsDir, 0755)
	for _, id := range ids {
		info, _ := depth.ResolveModel(id)
		modelPath := filepath.Join(modelsDir, info.Filename)
		if _, err := os.Stat(modelPath); err == nil && !videoInitForce {
			fmt.Printf("%s already exists: %s\n  Use --force to re-download.\n", info.ID, modelPath)
			continue
		}
		fmt.Printf("Downloading %s (%s, %s)...\n", info.ID, info.Size, info.License)
		if err := service.SaveResource(depthModelURL(id), modelPath); err != nil {
			return fmt.Errorf("download %s failed: %w", id, err)
		}
		fmt.Println("  Done.")
	}
	fmt.Println("\nDepth models installed!")
	return nil
}

// depthModelURL returns the download URL for a model ID.
func depthModelURL(id string) string {
	switch id {
	case "depth-anything-v2-small":
		return depthSmallURL
	case "depth-anything-v2-base":
		return depthBaseURL
	case "depth-anything-v2-large":
		return depthLargeURL
	}
	return ""
}

func init() {
	videoCmd.AddCommand(videoInitCmd)
	videoInitCmd.Flags().BoolVar(&videoInitForce, "force", false, "re-download even if files already exist")
	videoInitCmd.Flags().StringVar(&videoInitModel, "model", "", "download a specific depth model (default: depth-anything-v2-small)")
	videoInitCmd.Flags().BoolVar(&videoInitAll, "all", false, "download all depth model variants (base/large are CC-BY-NC-4.0)")
}
