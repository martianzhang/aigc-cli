package cmd

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/service"
)

const ortVersion = "1.27.0"

// modelInfo maps model identifier to download URL and filename.
// Add new models here. The key is used in `--model` flag and `detect.model` config.
var modelInfo = map[string]struct {
	url      string
	filename string
	desc     string // human-readable description
	size     string // human-readable download size
}{
	"vit-base": {
		url:      "https://huggingface.co/onnx-community/ai-image-detection-ONNX/resolve/main/onnx/model.onnx",
		filename: "model-vit-base.onnx",
		desc:     "ViT-Base, 86M params",
		size:     "327MB",
	},
	"distilled-vit": {
		url:      "https://huggingface.co/onnx-community/ai-image-detect-distilled-ONNX/resolve/main/onnx/model.onnx",
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
    model: "distilled-vit"

Proxy settings from config.yaml, env vars (HTTP_PROXY), or --http-proxy flag
are automatically respected.`,
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

	modelsDir := detectModelsDir()
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", modelsDir, err)
	}

	// ── Download ONNX Runtime CPU (shared across all models) ──
	if err := downloadORT(modelsDir, getORTDownloadInfo(), detectForce); err != nil {
		return err
	}
	// ── Download ONNX Runtime GPU (where available) ──
	if gpu := getGPUORTDownloadInfo(); gpu != nil {
		gpuPath := filepath.Join(modelsDir, gpu.libName)
		if _, err := os.Stat(gpuPath); err != nil || detectForce {
			fmt.Printf("Downloading ONNX Runtime GPU %s (%s)...\n", ortVersion, runtime.GOOS)
			archivePath := filepath.Join(modelsDir, gpu.archiveName)
			if err := service.SaveResource(gpu.url, archivePath); err != nil {
				return fmt.Errorf("ONNX Runtime GPU download failed: %w", err)
			}
			fmt.Println("  Extracting...")
			if err := extractRuntime(archivePath, modelsDir, gpu.libName, *gpu); err != nil {
				return fmt.Errorf("GPU extraction failed: %w", err)
			}
			os.Remove(archivePath)
			fmt.Printf("  Installed: %s\n", gpuPath)
		} else {
			fmt.Printf("ONNX Runtime GPU already installed: %s\n", gpuPath)
		}
	}

	// ── Download model ──
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

// ortDownloadInfo holds platform-specific download information.
type ortDownloadInfo struct {
	url          string
	archiveName  string
	libName      string
	internalPath string
}

// downloadORT downloads and extracts an ONNX Runtime package, unless it already exists.
func downloadORT(modelsDir string, info ortDownloadInfo, force bool) error {
	libPath := filepath.Join(modelsDir, info.libName)
	if _, err := os.Stat(libPath); err == nil && !force {
		fmt.Printf("ONNX Runtime already installed: %s\n", libPath)
		return nil
	}
	fmt.Printf("Downloading ONNX Runtime %s (%s)...\n", ortVersion, runtime.GOOS)
	archivePath := filepath.Join(modelsDir, info.archiveName)
	if err := service.SaveResource(info.url, archivePath); err != nil {
		return fmt.Errorf("ONNX Runtime download failed: %w", err)
	}
	fmt.Println("  Extracting...")
	if err := extractRuntime(archivePath, modelsDir, info.libName, info); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}
	os.Remove(archivePath)
	fmt.Printf("  Installed: %s\n", libPath)
	return nil
}

// gpuLibName returns the GPU ONNX Runtime library filename for the current platform.
// Returns empty string if there is no separate GPU package (macOS uses CoreML in the same binary).
func gpuLibName() string {
	switch runtime.GOOS {
	case "linux":
		return "libonnxruntime_gpu.so"
	case "windows":
		return "onnxruntime_gpu.dll"
	default:
		return ""
	}
}

// getGPUORTDownloadInfo returns download info for the GPU ONNX Runtime package.
// Returns nil on platforms without a separate GPU package (macOS, linux arm64).
func getGPUORTDownloadInfo() *ortDownloadInfo {
	base := fmt.Sprintf("https://github.com/microsoft/onnxruntime/releases/download/v%s", ortVersion)
	libName := gpuLibName()
	if libName == "" {
		return nil
	}
	switch runtime.GOOS {
	case "linux":
		if runtime.GOARCH != "amd64" {
			return nil
		}
		return &ortDownloadInfo{
			url:          fmt.Sprintf("%s/onnxruntime-linux-x64-cuda-%s.tgz", base, ortVersion),
			archiveName:  fmt.Sprintf("onnxruntime-cuda-%s.tgz", ortVersion),
			libName:      libName,
			internalPath: fmt.Sprintf("onnxruntime-linux-x64-cuda-%s/lib/libonnxruntime.so", ortVersion),
		}
	default: // windows
		return &ortDownloadInfo{
			url:          fmt.Sprintf("%s/onnxruntime-win-x64-cuda-%s.zip", base, ortVersion),
			archiveName:  fmt.Sprintf("onnxruntime-cuda-%s.zip", ortVersion),
			libName:      libName,
			internalPath: fmt.Sprintf("onnxruntime-win-x64-cuda-%s/lib/onnxruntime.dll", ortVersion),
		}
	}
}

func getORTDownloadInfo() ortDownloadInfo {
	base := fmt.Sprintf("https://github.com/microsoft/onnxruntime/releases/download/v%s", ortVersion)
	switch runtime.GOOS {
	case "windows":
		arch := "x64"
		if runtime.GOARCH == "arm64" {
			arch = "arm64"
		}
		return ortDownloadInfo{
			url:          fmt.Sprintf("%s/onnxruntime-win-%s-%s.zip", base, arch, ortVersion),
			archiveName:  fmt.Sprintf("onnxruntime-%s.zip", ortVersion),
			libName:      "onnxruntime.dll",
			internalPath: fmt.Sprintf("onnxruntime-win-%s-%s/lib/onnxruntime.dll", arch, ortVersion),
		}
	case "darwin":
		arch := "arm64"
		if runtime.GOARCH == "amd64" {
			arch = "x64_64"
		}
		return ortDownloadInfo{
			url:          fmt.Sprintf("%s/onnxruntime-osx-%s-%s.tgz", base, arch, ortVersion),
			archiveName:  fmt.Sprintf("onnxruntime-%s.tgz", ortVersion),
			libName:      "libonnxruntime.dylib",
			internalPath: fmt.Sprintf("onnxruntime-osx-%s-%s/lib/libonnxruntime.dylib", arch, ortVersion),
		}
	default: // linux
		arch := "x64"
		if runtime.GOARCH == "arm64" {
			arch = "aarch64"
		}
		return ortDownloadInfo{
			url:          fmt.Sprintf("%s/onnxruntime-linux-%s-%s.tgz", base, arch, ortVersion),
			archiveName:  fmt.Sprintf("onnxruntime-%s.tgz", ortVersion),
			libName:      "libonnxruntime.so",
			internalPath: fmt.Sprintf("onnxruntime-linux-%s-%s/lib/libonnxruntime.so", arch, ortVersion),
		}
	}
}

func extractRuntime(archivePath, modelsDir, libName string, info ortDownloadInfo) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, modelsDir, info.internalPath, libName)
	}
	return extractTGZ(archivePath, modelsDir, info.internalPath, libName)
}

func extractZip(archivePath, modelsDir, internalPath, libName string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	target := filepath.Join(modelsDir, libName)
	for _, f := range r.File {
		if f.Name != internalPath {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, rc)
		return err
	}
	return fmt.Errorf("library not found in archive: %s", internalPath)
}

func extractTGZ(archivePath, modelsDir, internalPath, libName string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tarr := tar.NewReader(gzr)
	target := filepath.Join(modelsDir, libName)

	for {
		header, err := tarr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Normalize path: tar entries may be prefixed with "./"
		name := strings.TrimPrefix(header.Name, "./")
		if name != internalPath {
			continue
		}
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, tarr)
		return err
	}
	return fmt.Errorf("library not found in archive: %s", internalPath)
}

func init() {
	detectCmd.AddCommand(detectInitCmd)
	detectInitCmd.Flags().BoolVar(&detectForce, "force", false, "re-download even if files already exist")
	detectInitCmd.Flags().StringVar(&detectModelID, "model", "vit-base", "ONNX model: vit-base (86M, default), distilled-vit (11.8M)")
}
