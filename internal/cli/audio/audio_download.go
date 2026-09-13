package audio

import (
	"archive/tar"
	"compress/bzip2"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/audio"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// downloadModelFiles downloads all files for a model from the registry.
func downloadModelFiles(info audio.ModelInfo, modelsBaseDir string, force bool) error {
	modelDir := filepath.Join(modelsBaseDir, info.ID)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	for _, f := range info.Files {
		dest := filepath.Join(modelDir, f.Path)
		label := info.ID + "/" + f.Path
		if err := downloadSingleFile(f.URL, dest, label, force); err != nil {
			return fmt.Errorf("download %s/%s: %w", info.ID, f.Path, err)
		}
	}
	return nil
}

// downloadFromURL downloads a model from an arbitrary URL (outside registry).
func downloadFromURL(url, baseDir, name string, force bool) error {
	modelDir := filepath.Join(baseDir, name)
	os.MkdirAll(modelDir, 0755)
	filename := filepath.Base(url)
	dest := filepath.Join(modelDir, filename)
	return downloadSingleFile(url, dest, filename, force)
}

// downloadSingleFile downloads a single file, handling tar.bz2 extraction.
func downloadSingleFile(url, dest, label string, force bool) error {
	// For archives, check if the model directory already has content
	if strings.HasSuffix(label, ".tar.bz2") {
		extractedDir := strings.TrimSuffix(dest, ".tar.bz2")
		if fi, err := os.Stat(extractedDir); err == nil && fi.IsDir() {
			// Check if model.onnx exists inside (or any subdirectory)
			entries, _ := os.ReadDir(extractedDir)
			for _, e := range entries {
				if e.IsDir() || strings.HasSuffix(e.Name(), ".onnx") {
					if !force {
						fmt.Printf("%s: already installed (%s)\n", label, url)
						return nil
					}
					break
				}
			}
		}
	} else if _, err := os.Stat(dest); err == nil && !force {
		fmt.Printf("%s: already exists\n", dest)
		return nil
	}
	fmt.Printf("Downloading %s...\n", url)
	if err := service.SaveResource(url, dest); err != nil {
		return fmt.Errorf("download %s: %w", label, err)
	}
	if strings.HasSuffix(label, ".tar.bz2") {
		extractDir := strings.TrimSuffix(dest, ".tar.bz2")
		fmt.Printf("Extracting %s...\n", label)
		if err := extractTarBz2(dest, extractDir); err != nil {
			return fmt.Errorf("extract: %w", err)
		}
		os.Remove(dest)
	}
	return nil
}

// extractTarBz2 extracts a .tar.bz2 archive into the specified directory.
// The top-level directory inside the archive is stripped so files go directly
// into extractDir.
func extractTarBz2(archivePath, extractDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	bz2r := bzip2.NewReader(f)
	tarr := tar.NewReader(bz2r)

	// Detect the top-level directory name to strip it
	var topDir string
	for {
		header, err := tarr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if topDir == "" {
			// First entry: extract the top directory name
			parts := strings.SplitN(header.Name, "/", 2)
			if len(parts) > 1 {
				topDir = parts[0] + "/"
			}
		}

		// Strip the top directory
		relPath := strings.TrimPrefix(header.Name, topDir)
		if relPath == "" {
			continue // skip the top directory entry itself
		}

		target := filepath.Join(extractDir, relPath)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tarr)
			out.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func downloadAsset(baseURL, name, dir string) error {
	url := baseURL + "/" + name
	dest := filepath.Join(dir, name)
	if _, err := os.Stat(dest); err == nil {
		return nil
	}
	fmt.Printf("Downloading %s...\n", url)
	return service.SaveResource(url, dest)
}
