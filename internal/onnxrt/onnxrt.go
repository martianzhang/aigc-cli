// Package onnxrt handles ONNX Runtime shared library download, extraction, and
// path resolution. All ONNX-dependent commands (detect, background, audio, etc.)
// use this single entry point so the runtime is downloaded exactly once.
//
// Archives are mirrored at modelsBaseURL and verified against the SHA-256
// digests pinned in ortAssetSHA256 before extraction, so a tampered or replaced
// asset never reaches the native loader. The library is cached in the shared
// models directory (~/.config/aigc-cli/models/).
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
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/modelstore"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// Version is the ONNX Runtime version used by this project.
const Version = "1.27.0"

// modelsBaseURL is the unified model download source for aigc-cli.
const modelsBaseURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1"

// ortAssetSHA256 pins the SHA-256 of every mirrored ONNX Runtime archive for
// Version, keyed by archive name. The digests are the published GitHub release
// asset digests of the mirrored bytes (verified by hashing the downloads).
//
// A platform whose archive is absent here is never downloaded: the runtime is a
// native library that runs in-process, so an unverified copy must not be loaded.
// Only x64 Windows/Linux and arm64 macOS have published builds.
var ortAssetSHA256 = map[string]string{
	"onnxruntime-osx-arm64-" + Version + ".tgz":            "545e81c58152353acb0d1e8bd6ce4b62f830c0961f5b3acfedc790ffd76e477a",
	"onnxruntime-linux-x64-" + Version + ".tgz":            "547e40a48f1fe73e3f812d7c88a948612c23f896b91e4e2ee1e232d7b468246f",
	"onnxruntime-win-x64-" + Version + ".zip":              "c5c81710938e68079ff1a192b04897faabe4b43830d48f39f27ecd4e16138bfc",
	"onnxruntime-linux-x64-gpu_cuda13-" + Version + ".tgz": "1a3227e1dc2f53d9f877c93278af500b15e26d99aa5ade877692138b3ab7d351",
	"onnxruntime-win-x64-gpu_cuda13-" + Version + ".zip":   "d3e0d908bc9b59dcb59b8f453493f173c087dda105d8c110df0588d22d7c8b3e",
}

// ortDownloadInfo holds platform-specific download information for one
// ONNX Runtime package (CPU or GPU).
type ortDownloadInfo struct {
	url          string
	archiveName  string
	libName      string
	internalPath string
	sha256       string
}

// newORTDownloadInfo builds download info and attaches the pinned digest of the
// archive, if the mirror publishes one for this platform.
func newORTDownloadInfo(url, archiveName, libName, internalPath string) ortDownloadInfo {
	return ortDownloadInfo{
		url:          url,
		archiveName:  archiveName,
		libName:      libName,
		internalPath: internalPath,
		sha256:       ortAssetSHA256[path.Base(url)],
	}
}

// archiveExtractor unpacks one runtime archive into modelsDir.
type archiveExtractor func(archivePath, modelsDir string, info ortDownloadInfo) error

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
		ui.Printf("ONNX Runtime already installed: %s\n", libPath)
		return libPath, nil
	}

	if err := pinnedDigestError(info); err != nil {
		return "", err
	}
	ui.Printf("Downloading ONNX Runtime %s (%s)...\n", Version, runtime.GOOS)
	if err := installArchive(modelsDir, info, force, extractCPUArchive); err != nil {
		return "", fmt.Errorf("ONNX Runtime install failed: %w", err)
	}
	ui.Printf("  Installed: %s\n", libPath)
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
		ui.Printf("ONNX Runtime GPU already installed: %s\n", gpuPath)
		return nil
	}

	if err := pinnedDigestError(*gpu); err != nil {
		return err
	}
	ui.Printf("Downloading ONNX Runtime GPU %s (%s)...\n", Version, runtime.GOOS)
	if err := installArchive(modelsDir, *gpu, force, extractGPUArchive); err != nil {
		return fmt.Errorf("ONNX Runtime GPU install failed: %w", err)
	}
	ui.Printf("  Installed: %s\n", gpuPath)
	return nil
}

// pinnedDigestError refuses an archive that has no digest pinned in the binary.
// A native runtime is loaded into this process, so an unverified copy must never
// be written to disk — failing here also keeps "Downloading ..." off the screen
// for platforms without a published build.
func pinnedDigestError(info ortDownloadInfo) error {
	if info.sha256 != "" {
		return nil
	}
	return fmt.Errorf("no verified ONNX Runtime build for %s/%s: %s has no pinned SHA-256",
		runtime.GOOS, runtime.GOARCH, path.Base(info.url))
}

// installArchive downloads one pinned runtime archive, verifies its SHA-256,
// extracts it, and deletes the archive. Archives without a pinned digest are
// rejected before any network access.
func installArchive(modelsDir string, info ortDownloadInfo, force bool, extract archiveExtractor) error {
	if err := pinnedDigestError(info); err != nil {
		return err
	}

	archivePath := filepath.Join(modelsDir, info.archiveName)
	if err := modelstore.Install(context.Background(), nil, info.url, archivePath, modelstore.Options{
		Force:  force,
		SHA256: info.sha256,
	}); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	ui.Println("  Extracting...")
	if err := extract(archivePath, modelsDir, info); err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	removeArchive(archivePath)
	return nil
}

// removeArchive drops the downloaded archive, any resume fragment, and
// modelstore's checksum sidecar; only the extracted library stays installed.
func removeArchive(archivePath string) {
	for _, p := range []string{archivePath, archivePath + ".part", archivePath + ".sha256"} {
		_ = os.Remove(p)
	}
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
		info := newORTDownloadInfo(
			fmt.Sprintf("%s/onnxruntime-linux-x64-gpu_cuda13-%s.tgz", base, Version),
			fmt.Sprintf("onnxruntime-gpu_cuda13-%s.tgz", Version),
			libName,
			"", // extractGPUArchive extracts all .so files
		)
		return &info
	default: // windows
		info := newORTDownloadInfo(
			fmt.Sprintf("%s/onnxruntime-win-x64-gpu_cuda13-%s.zip", base, Version),
			fmt.Sprintf("onnxruntime-gpu_cuda13-%s.zip", Version),
			libName,
			"", // extractGPUArchive extracts all .dll files
		)
		return &info
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
		return newORTDownloadInfo(
			fmt.Sprintf("%s/onnxruntime-win-%s-%s.zip", base, arch, Version),
			fmt.Sprintf("onnxruntime-%s.zip", Version),
			"onnxruntime.dll",
			fmt.Sprintf("onnxruntime-win-%s-%s/lib/onnxruntime.dll", arch, Version),
		)
	case "darwin":
		arch := "arm64"
		if runtime.GOARCH == "amd64" {
			arch = "x64"
		}
		return newORTDownloadInfo(
			fmt.Sprintf("%s/onnxruntime-osx-%s-%s.tgz", base, arch, Version),
			fmt.Sprintf("onnxruntime-%s.tgz", Version),
			"libonnxruntime.dylib",
			fmt.Sprintf("onnxruntime-osx-%s-%s/lib/libonnxruntime.dylib", arch, Version),
		)
	default: // linux
		arch := "x64"
		if runtime.GOARCH == "arm64" {
			arch = "aarch64"
		}
		return newORTDownloadInfo(
			fmt.Sprintf("%s/onnxruntime-linux-%s-%s.tgz", base, arch, Version),
			fmt.Sprintf("onnxruntime-%s.tgz", Version),
			"libonnxruntime.so",
			fmt.Sprintf("onnxruntime-linux-%s-%s/lib/libonnxruntime.so", arch, Version),
		)
	}
}

// --- archive extraction ---

// extractCPUArchive installs the single main library from a CPU archive.
func extractCPUArchive(archivePath, modelsDir string, info ortDownloadInfo) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, modelsDir, info.internalPath, info.libName)
	}
	return extractTGZ(archivePath, modelsDir, info.internalPath, info.libName)
}

func extractZip(archivePath, modelsDir, internalPath, libName string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	target := filepath.Join(modelsDir, libName)
	for _, f := range r.File {
		if f.Name != internalPath {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.Create(target)
		if err != nil {
			_ = rc.Close()
			return err
		}

		_, copyErr := io.Copy(out, rc)
		_ = rc.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
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
	defer func() { _ = gzr.Close() }()

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

		_, copyErr := io.Copy(out, tarr)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	return fmt.Errorf("library not found in archive: %s", internalPath)
}

// extractGPUArchive extracts all shared library files from a GPU ONNX Runtime
// archive. GPU packages ship multiple provider DLLs (CUDA, TensorRT, shared)
// alongside the CUDA-enabled main runtime — all must be placed together in
// modelsDir for ORT to find them at runtime.
func extractGPUArchive(archivePath, modelsDir string, _ ortDownloadInfo) error {
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
	defer func() { _ = r.Close() }()

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
	defer func() { _ = gzr.Close() }()

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
