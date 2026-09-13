package service

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// audioFormatExt maps an audio response_format to a file extension.
var audioFormatExt = map[string]string{
	"mp3":  ".mp3",
	"opus": ".opus",
	"aac":  ".aac",
	"flac": ".flac",
	"wav":  ".wav",
	"pcm":  ".pcm",
}

// SaveAudioFile saves raw audio bytes to outputDir with a timestamped filename.
// format is the response_format (mp3, wav, opus, ...). Returns the saved path.
func SaveAudioFile(data []byte, format, outputDir string) (string, error) {
	ext, ok := audioFormatExt[format]
	if !ok {
		ext = ".bin"
	}

	dir := outputDir
	if dir == "" {
		dir = "."
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	filename := filepath.Join(dir, fmt.Sprintf("audio_%d%s", time.Now().Unix(), ext))
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return "", fmt.Errorf("failed to save audio file: %w", err)
	}

	return filename, nil
}
