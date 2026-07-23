// Package onnxrt handles ONNX Runtime shared library download, extraction, and
// path resolution. All ONNX-dependent commands (detect, background, audio, etc.)
// use this single entry point so the runtime is downloaded exactly once.
//
// The library is downloaded from Microsoft's official GitHub releases and cached
// in the shared models directory (~/.config/aigc-cli/models/).
//
// Usage:
//
//	libPath, err := onnxrt.EnsureInstalled(modelsDir, force)
//	if err != nil { ... }
//	// libPath -> pass to onnx.NewDetector() / rmbg.NewDetector() / etc.
package onnxrt

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

	"github.com/martianzhang/aigc-cli/internal/service"
)

// Version is the ONNX Runtime version used by this project.
const Version = "1.27.0"

// modelsBaseURL is the unified model download source for aigc-cli.
const modelsBaseURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1"

// ortDownloadInfo holds platform-specific download information for one
// ONNX Runtime package (CPU or GPU).
type ortDownloadInfo struct {
	url          string
	archiveName  string
	libName      string
	internalPath string
}

// EnsureInstalled downloads and extracts the ONNX Runtime CPU shared library
// into modelsDir, unless it already exists (or force is true).
// Returns the path to the installed shared library.
func EnsureInstalled(modelsDir string, force bool) (string, error) {
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return "", fmt.Errorf("create models directory: %w", err)
	}

	info := getORTDownloadInfo()
	libPath := filepath.Join(modelsDir, info.libName)

	if _, err := os.Stat(libPath); err == nil && !force {
		fmt.Printf("ONNX Runtime already installed: %s\n", libPath)
		return libPath, nil
	}

	fmt.Printf("Downloading ONNX Runtime %s (%s)...\n", Version, runtime.GOOS)
	archivePath := filepath.Join(modelsDir, info.archiveName)
	if err := service.SaveResource(info.url, archivePath); err != nil {
		return "", fmt.Errorf("ONNX Runtime download failed: %w", err)
	}
	fmt.Println("  Extracting...")
	if err := extractRuntime(archivePath, modelsDir, info.libName, info); err != nil {
		return "", fmt.Errorf("extraction failed: %w", err)
	}
	os.Remove(archivePath)
	fmt.Printf("  Installed: %s\n", libPath)
	return libPath, nil
}

// EnsureGPUInstalled downloads and extracts the GPU ONNX Runtime package on
// platforms that have a separate GPU variant (Linux CUDA, Windows). On macOS
// and linux/arm64 this is a no-op (GPU support is built into the CPU package).
func EnsureGPUInstalled(modelsDir string, force bool) error {
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("create models directory: %w", err)
	}

	gpu := getGPUORTDownloadInfo()
	if gpu == nil {
		return nil
	}

	gpuPath := filepath.Join(modelsDir, gpu.libName)
	if _, err := os.Stat(gpuPath); err == nil && !force {
		fmt.Printf("ONNX Runtime GPU already installed: %s\n", gpuPath)
		return nil
	}

	fmt.Printf("Downloading ONNX Runtime GPU %s (%s)...\n", Version, runtime.GOOS)
	archivePath := filepath.Join(modelsDir, gpu.archiveName)
	if err := service.SaveResource(gpu.url, archivePath); err != nil {
		return fmt.Errorf("ONNX Runtime GPU download failed: %w", err)
	}
	fmt.Println("  Extracting...")
	if err := extractGPUArchive(archivePath, modelsDir); err != nil {
		return fmt.Errorf("GPU extraction failed: %w", err)
	}
	os.Remove(archivePath)
	fmt.Printf("  Installed: %s\n", gpuPath)
	return nil
}

// LibPath returns the path to the ONNX Runtime shared library in modelsDir.
// The main library is the same for CPU and GPU (GPU support comes via provider
// DLLs loaded dynamically by ORT at runtime, placed alongside it).
func LibPath(modelsDir string) (string, error) {
	var name string
	switch runtime.GOOS {
	case "darwin":
		name = "libonnxruntime.dylib"
	case "linux":
		name = "libonnxruntime.so"
	default: // windows
		name = "onnxruntime.dll"
	}

	c := filepath.Join(modelsDir, name)
	if _, err := os.Stat(c); err == nil {
		return c, nil
	}
	return "", fmt.Errorf("ONNX Runtime library not found in %s", modelsDir)
}

// --- platform helpers ---

// gpuLibName returns the CUDA provider library filename for the current
// platform, or empty string if there is no separate GPU package.
// Modern ONNX Runtime (1.17+) uses provider DLLs instead of a separate _gpu library.
func gpuLibName() string {
	switch runtime.GOOS {
	case "linux":
		return "libonnxruntime_providers_cuda.so"
	case "windows":
		return "onnxruntime_providers_cuda.dll"
	default:
		return ""
	}
}

// getGPUORTDownloadInfo returns download info for the GPU ONNX Runtime
// package. Returns nil on platforms without a separate GPU build.
func getGPUORTDownloadInfo() *ortDownloadInfo {
	base := modelsBaseURL
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
			url:         fmt.Sprintf("%s/onnxruntime-linux-x64-gpu_cuda13-%s.tgz", base, Version),
			archiveName: fmt.Sprintf("onnxruntime-gpu_cuda13-%s.tgz", Version),
			libName:     libName,
			// internalPath unused — extractGPUArchive extracts all .so files
		}
	default: // windows
		return &ortDownloadInfo{
			url:         fmt.Sprintf("%s/onnxruntime-win-x64-gpu_cuda13-%s.zip", base, Version),
			archiveName: fmt.Sprintf("onnxruntime-gpu_cuda13-%s.zip", Version),
			libName:     libName,
			// internalPath unused — extractGPUArchive extracts all .dll files
		}
	}
}

// getORTDownloadInfo returns download info for the CPU ONNX Runtime package
// for the current OS and architecture.
func getORTDownloadInfo() ortDownloadInfo {
	base := modelsBaseURL
	switch runtime.GOOS {
	case "windows":
		arch := "x64"
		if runtime.GOARCH == "arm64" {
			arch = "arm64"
		}
		return ortDownloadInfo{
			url:          fmt.Sprintf("%s/onnxruntime-win-%s-%s.zip", base, arch, Version),
			archiveName:  fmt.Sprintf("onnxruntime-%s.zip", Version),
			libName:      "onnxruntime.dll",
			internalPath: fmt.Sprintf("onnxruntime-win-%s-%s/lib/onnxruntime.dll", arch, Version),
		}
	case "darwin":
		arch := "arm64"
		if runtime.GOARCH == "amd64" {
			arch = "x64"
		}
		return ortDownloadInfo{
			url:          fmt.Sprintf("%s/onnxruntime-osx-%s-%s.tgz", base, arch, Version),
			archiveName:  fmt.Sprintf("onnxruntime-%s.tgz", Version),
			libName:      "libonnxruntime.dylib",
			internalPath: fmt.Sprintf("onnxruntime-osx-%s-%s/lib/libonnxruntime.dylib", arch, Version),
		}
	default: // linux
		arch := "x64"
		if runtime.GOARCH == "arm64" {
			arch = "aarch64"
		}
		return ortDownloadInfo{
			url:          fmt.Sprintf("%s/onnxruntime-linux-%s-%s.tgz", base, arch, Version),
			archiveName:  fmt.Sprintf("onnxruntime-%s.tgz", Version),
			libName:      "libonnxruntime.so",
			internalPath: fmt.Sprintf("onnxruntime-linux-%s-%s/lib/libonnxruntime.so", arch, Version),
		}
	}
}

// --- archive extraction ---

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

// extractGPUArchive extracts all shared library files from a GPU ONNX Runtime
// archive. GPU packages ship multiple provider DLLs (CUDA, TensorRT, shared)
// alongside the CUDA-enabled main runtime — all must be placed together in
// modelsDir for ORT to find them at runtime.
func extractGPUArchive(archivePath, modelsDir string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractAllFromZip(archivePath, modelsDir)
	}
	return extractAllFromTGZ(archivePath, modelsDir)
}

func extractAllFromZip(archivePath, modelsDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	extracted := 0
	for _, f := range r.File {
		// Only extract .dll files from the lib/ subdirectory
		if !strings.HasSuffix(f.Name, ".dll") {
			continue
		}
		baseName := filepath.Base(f.Name)
		if baseName == "" {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.Create(filepath.Join(modelsDir, baseName))
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
		extracted++
	}

	if extracted == 0 {
		return fmt.Errorf("no .dll files found in GPU archive")
	}
	return nil
}

func extractAllFromTGZ(archivePath, modelsDir string) error {
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
	extracted := 0
	for {
		header, err := tarr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(header.Name, "./")

		// Only extract .so files from the lib/ subdirectory
		if !strings.Contains(name, "/lib/") {
			continue
		}
		if !strings.HasSuffix(name, ".so") {
			continue
		}
		// Skip symlinks — the real file (e.g. libonnxruntime.so.1.27.0)
		// is already listed as a regular entry; the symlink is redundant.
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			continue
		}

		baseName := filepath.Base(name)
		if baseName == "" {
			continue
		}

		out, err := os.Create(filepath.Join(modelsDir, baseName))
		if err != nil {
			return err
		}

		_, err = io.Copy(out, tarr)
		out.Close()
		if err != nil {
			return err
		}
		extracted++
	}

	if extracted == 0 {
		return fmt.Errorf("no .so files found in GPU archive")
	}
	return nil
}
