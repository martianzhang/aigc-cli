package service

import (
	"fmt"
	"os"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// DownloadVideos downloads all generated videos into outputDir. Returns paths
// to saved files.
func DownloadVideos(videos []types.VideoResult, outputDir, taskID string) ([]string, error) {
	var saved []string
	for i, vid := range videos {
		for j, url := range vid.URL {
			filename, err := DownloadFile(url, outputDir, fmt.Sprintf("video_%s_%d_%d", taskID, i, j))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download video %d-%d: %v\n", i, j, err)
				continue
			}
			fmt.Printf("Saved: %s\n", filename)
			saved = append(saved, filename)
		}
	}
	return saved, nil
}
