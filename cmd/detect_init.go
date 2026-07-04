package cmd

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const ortVersion = "1.27.0"

// modelURLs maps model size to download URL and filename.
var modelInfo = map[string]struct {
	url      string
	filename string
	size     string // human-readable size
}{
	"small": {
		url:      "https://huggingface.co/onnx-community/ai-image-detect-distilled-ONNX/resolve/main/onnx/model.onnx",
		filename: "model-small.onnx",
		size:     "56MB",
	},
	"large": {
		url:      "https://huggingface.co/onnx-community/ai-image-detection-ONNX/resolve/main/onnx/model.onnx",
		filename: "model-large.onnx",
		size:     "327MB",
	},
}

// detectInitCmd represents the `apimart-cli detect init` subcommand.
var detectInitCmd = &cobra.Command{
	Use:          "init",
	Short:        "Download ONNX Runtime and AIGC detection model",
	SilenceUsage: true,
	Long: `Download the ONNX Runtime shared library and the AIGC detection model.

The runtime and model are saved to ~/.config/apimart/models/ for offline
AIGC detection via the 'detect' command.

Use --size to choose model capacity:
  large (default) - ViT-Base, 86M params, 327MB
  small           - distilled ViT, 11.8M params, 56MB

Proxy settings from config.yaml, env vars (HTTP_PROXY), or --http-proxy flag
are automatically respected.`,
	RunE: runDetectInit,
}

var (
	detectForce     bool
	detectModelSize string
)

func runDetectInit(cmd *cobra.Command, args []string) error {
	info, ok := modelInfo[detectModelSize]
	if !ok {
		return fmt.Errorf("unknown model size %q (choose: small, large)", detectModelSize)
	}

	modelsDir := filepath.Join(configDir(), "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", modelsDir, err)
	}

	transport := http.DefaultClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client := &http.Client{
		Timeout:   600 * time.Second,
		Transport: transport,
	}

	// ── Download ONNX Runtime (shared across all models) ──
	ortInfo := getORTDownloadInfo()
	libName := ortInfo.libName
	libPath := filepath.Join(modelsDir, libName)
	if _, err := os.Stat(libPath); err != nil || detectForce {
		fmt.Printf("Downloading ONNX Runtime %s (%s)...\n", ortVersion, runtime.GOOS)
		archivePath := filepath.Join(modelsDir, ortInfo.archiveName)
		if err := downloadFile(client, ortInfo.url, archivePath); err != nil {
			return fmt.Errorf("ONNX Runtime download failed: %w", err)
		}
		fmt.Println("  Extracting...")
		if err := extractRuntime(archivePath, modelsDir, libName, ortInfo); err != nil {
			return fmt.Errorf("extraction failed: %w", err)
		}
		os.Remove(archivePath)
		fmt.Printf("  Installed: %s\n", libPath)
	} else {
		fmt.Printf("ONNX Runtime already installed: %s\n", libPath)
	}

	// ── Download model ──
	modelPath := filepath.Join(modelsDir, info.filename)
	if _, err := os.Stat(modelPath); err == nil && !detectForce {
		fmt.Printf("Model already exists: %s\n  Use --force to re-download.\n", modelPath)
		return nil
	}

	fmt.Printf("Downloading %s model (%s)...\n", detectModelSize, info.size)
	if err := downloadFile(client, info.url, modelPath); err != nil {
		return fmt.Errorf("model download failed: %w", err)
	}
	fmt.Println("  Done.")

	fmt.Println("\nDone! Run 'apimart-cli detect' to use AIGC detection.")
	return nil
}

// ortDownloadInfo holds platform-specific download information.
type ortDownloadInfo struct {
	url          string
	archiveName  string
	libName      string
	internalPath string
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

// downloadFile downloads a URL to a local file path.
func downloadFile(client *http.Client, url, dest string) error {
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmpDest := dest + ".tmp"
	f, err := os.Create(tmpDest)
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", tmpDest, err)
	}

	written, err := io.Copy(f, resp.Body)
	if err != nil {
		f.Close()
		os.Remove(tmpDest)
		return fmt.Errorf("download failed: %w", err)
	}
	f.Close()

	if written == 0 {
		os.Remove(tmpDest)
		return fmt.Errorf("downloaded file is empty")
	}

	if err := os.Rename(tmpDest, dest); err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}
	return nil
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
		if header.Name != internalPath {
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
	detectInitCmd.Flags().StringVar(&detectModelSize, "size", "large", "model size: large (327MB, default) or small (56MB)")
}
