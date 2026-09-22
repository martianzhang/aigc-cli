package ideas

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/ideas"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// --- keyword resolution ---

func resolveKeywords(args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", nil
	}
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "", nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("failed to read stdin: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// --- image + output helpers ---

func saveIdeaImages(entries []ideas.IdeaEntry, outputDir string) ([]string, error) {
	var saved []string
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return saved, fmt.Errorf("cannot create output directory: %w", err)
	}
	for _, e := range entries {
		for _, imgURL := range e.ImageURLs {
			if imgURL == "" {
				continue
			}
			path := filepath.Join(outputDir, filepath.Base(imgURL))
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

func localImagePath(remoteURL, outputDir string) string {
	if remoteURL == "" {
		return ""
	}
	return filepath.Join(outputDir, filepath.Base(remoteURL))
}

func outputMarkdown(results []ideas.SearchResult, keywords string, total int, savedFiles []string, preview bool) error {
	md := ideas.FormatResultsMarkdown(results, keywords, total)
	fmt.Println(md)

	for _, r := range results {
		if preview && len(savedFiles) > 0 {
			for range r.Entry.ImageURLs {
				if len(savedFiles) == 0 {
					break
				}
				f := savedFiles[0]
				savedFiles = savedFiles[1:]
				if e := service.PreviewFile(f); e != nil {
					fmt.Fprintf(os.Stderr, "Warning: preview failed: %v\n", e)
				}
			}
		}
	}
	return nil
}

func outputJSON(results []ideas.SearchResult, total int) error {
	out := struct {
		Total   int               `json:"total"`
		Results []ideas.IdeaEntry `json:"results"`
	}{Total: total}
	for _, r := range results {
		out.Results = append(out.Results, r.Entry)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
