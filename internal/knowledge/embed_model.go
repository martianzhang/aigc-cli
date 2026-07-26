package knowledge

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/service"
)

// modelsBaseURL is the shared model download host.
const modelsBaseURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1"

// EmbedModelID is the default embedding model identifier.
const EmbedModelID = "e5-small-multilingual"

// ModelFile describes a downloadable model file.
type ModelFile struct {
	URL     string
	OutName string
	SizeMB  int64
}

// EmbedModelFiles returns the files needed for the embedding model.
func EmbedModelFiles() []ModelFile {
	return []ModelFile{
		{
			URL:     modelsBaseURL + "/e5-small-multilingual-model.onnx",
			OutName: "model.onnx",
			SizeMB:  110,
		},
		{
			URL:     modelsBaseURL + "/e5-small-multilingual-tokenizer.json",
			OutName: "tokenizer.json",
			SizeMB:  16,
		},
	}
}

// EmbedModelDir returns the directory for the embedding model.
// modelsDir is the shared models directory (e.g. ~/.config/aigc-cli/models).
func EmbedModelDir(modelsDir string) string {
	return filepath.Join(modelsDir, EmbedModelID)
}

// InitEmbedModel downloads the embedding model files to the shared models dir.
func InitEmbedModel(modelsDir string, force bool) error {
	dir := EmbedModelDir(modelsDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create model dir: %w", err)
	}

	files := EmbedModelFiles()
	for _, f := range files {
		path := filepath.Join(dir, f.OutName)
		if !force {
			if _, err := os.Stat(path); err == nil {
				fmt.Printf("  Already exists: %s\n", f.OutName)
				continue
			}
		}
		size := ""
		if f.SizeMB > 0 {
			size = fmt.Sprintf(" (%d MB)", f.SizeMB)
		}
		fmt.Printf("  Downloading %s%s...\n", f.OutName, size)
		if err := service.SaveResource(f.URL, path); err != nil {
			return fmt.Errorf("download %s: %w", f.OutName, err)
		}
	}
	return nil
}
