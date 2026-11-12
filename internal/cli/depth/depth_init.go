package depth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/depth"
	"github.com/martianzhang/aigc-cli/internal/modelstore"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/options"
)

const (
	depthSmallURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1/depth-anything-v2-small.onnx"
	depthBaseURL  = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1/depth-anything-v2-base.onnx"
	depthLargeURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1/depth-anything-v2-large.onnx"
)

var (
	videoInitForce    bool
	videoInitModel    string
	videoInitAll      bool
	videoInitSkeleton bool
	videoInitFace     bool
)

func newInitCommand(deps func() Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "init",
		Short:        "Download ONNX Runtime and depth estimation models",
		SilenceUsage: true,
		Long:         `Download the ONNX Runtime shared library and Depth Anything V2 models used by the depth command.`,
		Example: `  aigc-cli depth init                                # default: depth-anything-v2-small
  aigc-cli depth init --model depth-anything-v2-base
  aigc-cli depth init --all --skeleton --face        # all models + pose + face assets`,
		RunE: func(cmd *cobra.Command, args []string) error {
			d = deps()
			return runDepthInit(cmd, args)
		},
	}
	cmd.Flags().BoolVar(&videoInitForce, "force", false, "re-download even if files already exist")
	cmd.Flags().StringVar(&videoInitModel, "model", "", fmt.Sprintf("download a specific depth model (default: %s; options: %s)", depth.DefaultModelID, strings.Join(depth.ListModelIDs(), ", ")))
	cmd.Flags().BoolVar(&videoInitAll, "all", false, "download all depth model variants (base/large are CC-BY-NC-4.0)")
	cmd.Flags().BoolVar(&videoInitSkeleton, "skeleton", false, "also download the skeleton (pose) model")
	cmd.Flags().BoolVar(&videoInitFace, "face", false, "also download the face detection cascades (pigo)")
	return cmd
}

func runDepthInit(_ *cobra.Command, _ []string) error {
	sharedDir := d.ModelsDir
	if err := os.MkdirAll(sharedDir, 0755); err != nil {
		return fmt.Errorf("create models dir: %w", err)
	}
	if _, err := onnxrt.EnsureInstalled(sharedDir, videoInitForce); err != nil {
		return err
	}
	// GPU runtime is optional: the CPU runtime is enough for local inference.
	_ = onnxrt.EnsureGPUInstalled(sharedDir, videoInitForce)

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
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("create depth dir: %w", err)
	}
	for _, id := range ids {
		info, _ := depth.ResolveModel(id)
		modelPath := depth.ModelPath(sharedDir, id)
		if _, err := os.Stat(modelPath); err == nil && !videoInitForce {
			fmt.Printf("%s already exists: %s\n  Use --force to re-download.\n", info.ID, modelPath)
			continue
		}
		fmt.Printf("Downloading %s (%s, %s)...\n", info.ID, info.Size, info.License)
		if err := modelstore.Install(options.Context(), nil, depthModelURL(id), modelPath, modelstore.Options{Force: videoInitForce}); err != nil {
			return fmt.Errorf("download %s failed: %w", id, err)
		}
		fmt.Println("  Done.")
	}

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

// downloadSkeletonModels downloads the human-pose model to models/skeleton/.
func downloadSkeletonModels(sharedDir string) error {
	const (
		url  = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1/yolov8n-pose.onnx"
		file = "yolov8n-pose.onnx"
	)
	dir := filepath.Join(sharedDir, "skeleton")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create skeleton dir: %w", err)
	}
	path := filepath.Join(dir, file)
	if _, err := os.Stat(path); err == nil && !videoInitForce {
		fmt.Printf("skeleton model already exists: %s\n", path)
		return nil
	}
	fmt.Printf("Downloading YOLOv8n-pose (skeleton, 13.5MB)...\n")
	if err := modelstore.Install(options.Context(), nil, url, path, modelstore.Options{Force: videoInitForce}); err != nil {
		return fmt.Errorf("download skeleton model failed: %w", err)
	}
	fmt.Println("  Done.")
	return nil
}

// downloadFaceModels downloads the pigo face cascades to models/face/.
func downloadFaceModels(sharedDir string) error {
	const baseURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1"
	files := []string{"facefinder", "puploc"}
	lps := []string{"lp38", "lp42", "lp44", "lp46", "lp81", "lp82", "lp84", "lp93", "lp312"}

	dir := filepath.Join(sharedDir, "face")
	lpsDir := filepath.Join(dir, "lps")
	if err := os.MkdirAll(lpsDir, 0755); err != nil {
		return fmt.Errorf("create face dir: %w", err)
	}

	download := func(url, path, label string) error {
		if _, err := os.Stat(path); err == nil && !videoInitForce {
			fmt.Printf("face model already exists: %s\n", path)
			return nil
		}
		fmt.Printf("Downloading face model: %s\n", label)
		if err := modelstore.Install(options.Context(), nil, url, path, modelstore.Options{Force: videoInitForce}); err != nil {
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
