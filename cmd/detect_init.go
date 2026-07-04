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

const (
	ortVersion = "1.27.0"
	modelName  = "model.onnx"
	modelURL   = "https://huggingface.co/onnx-community/ai-image-detect-distilled-ONNX/resolve/main/onnx/model.onnx"
)

// detectInitCmd represents the `apimart-cli detect init` subcommand.
var detectInitCmd = &cobra.Command{
	Use:          "init",
	Short:        "Download ONNX Runtime and AIGC detection model",
	SilenceUsage: true,
	Long: `Download the ONNX Runtime shared library and the AIGC detection model.

The runtime and model are saved to ~/.config/apimart/models/ for offline
AIGC detection via the 'detect' command.

Proxy settings from config.yaml, env vars (HTTP_PROXY), or --http-proxy flag
are automatically respected.`,
	RunE: runDetectInit,
}

var detectForce bool

func runDetectInit(cmd *cobra.Command, args []string) error {
	modelsDir := filepath.Join(configDir(), "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", modelsDir, err)
	}

	// Use the global default transport which inherits proxy config from
	// ConfigureDefaultClient (called in root.go PersistentPreRunE).
	transport := http.DefaultClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client := &http.Client{
		Timeout:   600 * time.Second,
		Transport: transport,
	}

	// ── Download model ──
	modelPath := filepath.Join(modelsDir, modelName)
	if _, err := os.Stat(modelPath); err == nil && !detectForce {
		fmt.Fprintf(os.Stderr, "Model already exists: %s\n  Use --force to re-download.\n", modelPath)
	} else {
		fmt.Println("Downloading AIGC detection model (55MB)...")
		if err := downloadFile(client, modelURL, modelPath); err != nil {
			return fmt.Errorf("model download failed: %w", err)
		}
		fmt.Println("  Done.")
	}

	// ── Download ONNX Runtime ──
	ortInfo := getORTDownloadInfo()
	libName := ortInfo.libName
	libPath := filepath.Join(modelsDir, libName)

	// Check if the library is already extracted
	if _, err := os.Stat(libPath); err == nil && !detectForce {
		fmt.Fprintf(os.Stderr, "ONNX Runtime already exists: %s\n  Use --force to re-download.\n", libPath)
		return nil
	}

	fmt.Printf("Downloading ONNX Runtime %s (%s)...\n", ortVersion, runtime.GOOS)
	archivePath := filepath.Join(modelsDir, ortInfo.archiveName)
	if err := downloadFile(client, ortInfo.url, archivePath); err != nil {
		return fmt.Errorf("ONNX Runtime download failed: %w", err)
	}

	fmt.Println("  Extracting...")
	if err := extractRuntime(archivePath, modelsDir, libName, ortInfo); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Clean up archive
	os.Remove(archivePath)
	fmt.Printf("  Installed: %s\n", libPath)
	fmt.Println("\nDone! Run 'apimart-cli detect' to use AIGC detection.")
	return nil
}

// ortDownloadInfo holds platform-specific download information.
type ortDownloadInfo struct {
	url         string
	archiveName string
	libName     string
	// internalPath is the path of the library within the archive.
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
			arch = "x64_64" // actually there's no intel macOS build in recent ORT
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

// extractRuntime extracts the ONNX Runtime shared library from the archive.
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
}
