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

// depthInitCmd 提供 depth init 子命令（下载 ONNX Runtime + 深度模型）。
var depthInitCmd = &cobra.Command{
	Use:          "init",
	Short:        "Download ONNX Runtime and depth estimation models",
	SilenceUsage: true,
	Long:         `Download the ONNX Runtime shared library and Depth Anything V2 models used by the depth command.`,
	RunE:         runDepthInit,
}

var (
	videoInitForce    bool
	videoInitModel    string
	videoInitAll      bool
	videoInitSkeleton bool
	videoInitFace     bool
)

func runDepthInit(cmd *cobra.Command, args []string) error {
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
		modelPath := depth.ModelPath(sharedDir, id)
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

	// Skeleton + face annotation models (HF/GitHub direct, until mirrored).
	if videoInitAll || videoInitSkeleton {
		if err := downloadSkeletonModels(sharedDir); err != nil {
			return err
		}
	}
	if videoInitAll || videoInitFace {
		if err := downloadFaceModels(sharedDir); err != nil {
			return err
		}
	}

	fmt.Println("\nModels installed!")
	return nil
}

// downloadSkeletonModels 下载人体骨架模型到 models/skeleton/。
func downloadSkeletonModels(sharedDir string) error {
	const (
		url  = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1/yolov8n-pose.onnx"
		file = "yolov8n-pose.onnx"
	)
	dir := filepath.Join(sharedDir, "skeleton")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, file)
	if _, err := os.Stat(path); err == nil && !videoInitForce {
		fmt.Printf("skeleton model already exists: %s\n", path)
		return nil
	}
	fmt.Printf("Downloading YOLOv8n-pose (skeleton, 13.5MB)...\n")
	if err := service.SaveResource(url, path); err != nil {
		return fmt.Errorf("download skeleton model failed: %w", err)
	}
	fmt.Println("  Done.")
	return nil
}

// downloadFaceModels 下载 pigo 人脸检测级联到 models/face/。
func downloadFaceModels(sharedDir string) error {
	const baseURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1"
	files := []string{"facefinder", "puploc"}
	lps := []string{"lp38", "lp42", "lp44", "lp46", "lp81", "lp82", "lp84", "lp93", "lp312"}

	dir := filepath.Join(sharedDir, "face")
	lpsDir := filepath.Join(dir, "lps")
	os.MkdirAll(lpsDir, 0755)

	download := func(url, path, label string) error {
		if _, err := os.Stat(path); err == nil && !videoInitForce {
			fmt.Printf("face model already exists: %s\n", path)
			return nil
		}
		fmt.Printf("Downloading face model: %s\n", label)
		if err := service.SaveResource(url, path); err != nil {
			return fmt.Errorf("download %s failed: %w", label, err)
		}
		return nil
	}

	for _, f := range files {
		if err := download(baseURL+"/"+f, filepath.Join(dir, f), f); err != nil {
			return err
		}
	}
	for _, lp := range lps {
		if err := download(baseURL+"/"+lp, filepath.Join(lpsDir, lp), lp); err != nil {
			return err
		}
	}
	fmt.Println("  Face models done.")
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
	depthCmd.AddCommand(depthInitCmd)
	depthInitCmd.Flags().BoolVar(&videoInitForce, "force", false, "re-download even if files already exist")
	depthInitCmd.Flags().StringVar(&videoInitModel, "model", "", fmt.Sprintf("download a specific depth model (default: %s; options: %s)", depth.DefaultModelID, strings.Join(depth.ListModelIDs(), ", ")))
	depthInitCmd.Flags().BoolVar(&videoInitAll, "all", false, "download all depth model variants (base/large are CC-BY-NC-4.0)")
	depthInitCmd.Flags().BoolVar(&videoInitSkeleton, "skeleton", false, "also download the skeleton (pose) model")
	depthInitCmd.Flags().BoolVar(&videoInitFace, "face", false, "also download the face detection cascades (pigo)")
}
