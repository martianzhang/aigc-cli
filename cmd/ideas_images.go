package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martianzhang/apimart-cli/internal/ideas"
	"github.com/martianzhang/apimart-cli/internal/service"
)

// saveIdeaImages downloads and saves idea images to the output directory.
func saveIdeaImages(entries []ideas.IdeaEntry) ([]string, error) {
	var saved []string
	if err := os.MkdirAll(shared.OutputDir, 0755); err != nil {
		return saved, fmt.Errorf("cannot create output directory: %w", err)
	}
	for _, e := range entries {
		for _, imgURL := range e.ImageURLs {
			if imgURL == "" {
				continue
			}
			path := filepath.Join(shared.OutputDir, filepath.Base(imgURL))
			if _, err := os.Stat(path); err == nil {
				saved = append(saved, path)
				continue
			}
			if err := service.SaveResource(imgURL, path); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download %s: %v\n", imgURL, err)
				continue
			}
			saved = append(saved, path)
		}
	}
	return saved, nil
}

func localImagePath(remoteURL string) string {
	if remoteURL == "" {
		return ""
	}
	return filepath.Join(shared.OutputDir, filepath.Base(remoteURL))
}
