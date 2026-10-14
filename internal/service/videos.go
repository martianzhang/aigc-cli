package service

import (
	"fmt"
	"os"
	"strings"

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

// DownloadLastFrames downloads provider-returned last-frame images (a video
// task's result.images) into outputDir. Returns paths to saved files.
// Naming: <taskID>_lastframe.<ext>, or <taskID>_lastframe_<i>.<ext> when the
// provider returned more than one.
func DownloadLastFrames(urls []string, outputDir, taskID string) ([]string, error) {
	var saved []string
	for i, u := range urls {
		if strings.TrimSpace(u) == "" {
			continue
		}
		name := taskID + "_lastframe"
		if len(urls) > 1 {
			name = fmt.Sprintf("%s_lastframe_%d", taskID, i)
		}
		filename, err := DownloadFile(u, outputDir, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to download last frame %d: %v\n", i, err)
			continue
		}
		fmt.Printf("Saved: %s\n", filename)
		saved = append(saved, filename)
	}
	return saved, nil
}
