package mcp

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	// maxEmbeddedFiles caps how many media files one tool result inlines.
	maxEmbeddedFiles = 4
	// maxEmbeddedFileSize caps the size of each inlined file (4 MiB).
	maxEmbeddedFileSize = 4 << 20
	// mediaEmbedEnv is the env var that toggles inline embedding (0/false/off = disabled).
	mediaEmbedEnv = "AIGC_MCP_EMBED_MEDIA"
)

// mediaMIME maps lowercased file extensions to embeddable media MIME types.
// Extensions missing here (e.g. .mp4/.mov — MCP has no video content type) are
// not embeddable and are skipped.
var mediaMIME = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
	".avif": "image/avif",
	".bmp":  "image/bmp",
	".jxl":  "image/jxl",
	".heic": "image/heic",
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".opus": "audio/opus",
	".aac":  "audio/aac",
	".flac": "audio/flac",
	".ogg":  "audio/ogg",
	".m4a":  "audio/mp4",
}

// toolResultTextWithMedia returns the unchanged text plus inline image/audio
// content blocks for embeddable media files, so MCP hosts can render generated
// media directly in the conversation. Files that are missing, unreadable,
// not embeddable, too large, or beyond the count cap are skipped; the result is
// never an error and the text block always comes first.
func toolResultTextWithMedia(text string, paths ...string) *mcp.CallToolResult {
	blocks, skipped := mediaContents(paths)
	if skipped > 0 {
		text = fmt.Sprintf("%s\n(%d file(s) not inlined: exceeds size or count limit)", text, skipped)
	}
	contents := make([]mcp.Content, 0, 1+len(blocks))
	contents = append(contents, mcp.NewTextContent(text))
	contents = append(contents, blocks...)
	return &mcp.CallToolResult{Content: contents}
}

// mediaContents reads embeddable files into image/audio content blocks.
// The second return value counts files skipped because of the size or count cap.
func mediaContents(paths []string) ([]mcp.Content, int) {
	if !mediaEmbeddingEnabled() {
		return nil, 0
	}
	var blocks []mcp.Content
	skipped := 0
	for _, path := range paths {
		mimeType, ok := mediaMIME[strings.ToLower(filepath.Ext(path))]
		if !ok {
			continue
		}
		if len(blocks) >= maxEmbeddedFiles {
			skipped++
			continue
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Size() > maxEmbeddedFileSize {
			skipped++
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		encoded := base64.StdEncoding.EncodeToString(data)
		if strings.HasPrefix(mimeType, "audio/") {
			blocks = append(blocks, mcp.NewAudioContent(encoded, mimeType))
			continue
		}
		blocks = append(blocks, mcp.NewImageContent(encoded, mimeType))
	}
	return blocks, skipped
}

// mediaEmbeddingEnabled reports whether inline embedding is on.
func mediaEmbeddingEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(mediaEmbedEnv))) {
	case "0", "false", "off":
		return false
	default:
		return true
	}
}
