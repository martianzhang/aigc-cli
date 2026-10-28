package music

import (
	"fmt"
	"os"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// musicTrackURL picks the preferred downloadable URL from a track.
// Preference: AudioURL > WAVURL > FileURL > VideoURL.
func musicTrackURL(track types.MusicTrack) string {
	switch {
	case track.AudioURL != "":
		return track.AudioURL
	case track.WAVURL != "":
		return track.WAVURL
	case track.FileURL != "":
		return track.FileURL
	case track.VideoURL != "":
		return track.VideoURL
	default:
		return ""
	}
}

// downloadMusics downloads all generated tracks. Returns paths to saved files.
// When every track fails it returns an error so the command exits non-zero.
func downloadMusics(tracks []types.MusicTrack, taskID string) ([]string, error) {
	var saved []string
	attempted := 0
	var firstErr error
	for i, track := range tracks {
		url := musicTrackURL(track)
		if url == "" {
			continue
		}
		attempted++
		filename, err := service.DownloadFile(url, options.Shared.OutputDir, fmt.Sprintf("music_%s_%d", taskID, i))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to download music %d: %v\n", i, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		fmt.Printf("Saved: %s\n", filename)
		saved = append(saved, filename)
	}
	if attempted > 0 && len(saved) == 0 {
		return saved, fmt.Errorf("no music saved: %w", firstErr)
	}
	return saved, nil
}
