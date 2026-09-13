package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// saveAudioFile saves raw audio bytes to the shared output directory.
func saveAudioFile(data []byte, format string) (string, error) {
	return service.SaveAudioFile(data, format, options.Shared.OutputDir)
}

// saveTranscriptionFile saves transcription text to a .md file.
// If inputPath is a file path, use its basename with .md extension (e.g. speech.mp3 → speech.md).
// If that file already exists, or inputPath is empty/non-file, fall back to audio_<timestamp>.md.
// Returns the full path to the saved file.
func saveTranscriptionFile(text string, inputPath string) (string, error) {
	dir := options.Shared.OutputDir
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	filename := ""
	if inputPath != "" {
		if fi, err := os.Stat(inputPath); err == nil && !fi.IsDir() {
			base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
			candidate := filepath.Join(dir, base+".md")
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				filename = candidate
			}
		}
	}
	if filename == "" {
		filename = filepath.Join(dir, fmt.Sprintf("audio_%d.md", time.Now().Unix()))
	}

	if err := os.WriteFile(filename, []byte(text), 0644); err != nil {
		return "", fmt.Errorf("failed to save transcription: %w", err)
	}
	return filename, nil
}

// audioFormatFromContentType extracts the best-guess response_format from a Content-Type header.
func audioFormatFromContentType(contentType string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "mpeg"):
		return "mp3"
	case strings.Contains(ct, "opus"):
		return "opus"
	case strings.Contains(ct, "aac"):
		return "aac"
	case strings.Contains(ct, "flac"):
		return "flac"
	case strings.Contains(ct, "wav"):
		return "wav"
	case strings.Contains(ct, "pcm"):
		return "pcm"
	default:
		return "mp3"
	}
}
