package mcp

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// writeMediaFile writes content to dir/name and returns the full path.
func writeMediaFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// firstTextBlock returns the leading TextContent of a tool result.
func firstTextBlock(t *testing.T, result *mcp.CallToolResult) mcp.TextContent {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content blocks")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("first block is %T, want mcp.TextContent", result.Content[0])
	}
	return text
}

func TestToolResultTextWithMediaPNG(t *testing.T) {
	dir := t.TempDir()
	raw := []byte("\x89PNG\r\n\x1a\nfake-png-bytes")
	path := writeMediaFile(t, dir, "out.png", raw)
	text := "Image saved: " + path

	result := toolResultTextWithMedia(text, path)

	if got := firstTextBlock(t, result).Text; got != text {
		t.Errorf("text = %q, want %q", got, text)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content blocks = %d, want 2", len(result.Content))
	}
	img, ok := result.Content[1].(mcp.ImageContent)
	if !ok {
		t.Fatalf("second block is %T, want mcp.ImageContent", result.Content[1])
	}
	if img.MIMEType != "image/png" {
		t.Errorf("mime = %q, want image/png", img.MIMEType)
	}
	if want := base64.StdEncoding.EncodeToString(raw); img.Data != want {
		t.Errorf("data = %q, want %q", img.Data, want)
	}
}

func TestToolResultTextWithMediaMP3(t *testing.T) {
	dir := t.TempDir()
	raw := []byte("ID3fake-mp3-bytes")
	path := writeMediaFile(t, dir, "speech.mp3", raw)
	text := "Speech saved: " + path

	result := toolResultTextWithMedia(text, path)

	if got := firstTextBlock(t, result).Text; got != text {
		t.Errorf("text = %q, want %q", got, text)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content blocks = %d, want 2", len(result.Content))
	}
	audio, ok := result.Content[1].(mcp.AudioContent)
	if !ok {
		t.Fatalf("second block is %T, want mcp.AudioContent", result.Content[1])
	}
	if audio.MIMEType != "audio/mpeg" {
		t.Errorf("mime = %q, want audio/mpeg", audio.MIMEType)
	}
	if want := base64.StdEncoding.EncodeToString(raw); audio.Data != want {
		t.Errorf("data = %q, want %q", audio.Data, want)
	}
}

func TestToolResultTextWithMediaEmpty(t *testing.T) {
	result := toolResultTextWithMedia("done")

	if len(result.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(result.Content))
	}
	if got := firstTextBlock(t, result).Text; got != "done" {
		t.Errorf("text = %q, want %q", got, "done")
	}
}

func TestToolResultTextWithMediaDisabled(t *testing.T) {
	dir := t.TempDir()
	path := writeMediaFile(t, dir, "out.png", []byte("png"))

	for _, value := range []string{"0", "false", "off", "OFF"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(mediaEmbedEnv, value)

			result := toolResultTextWithMedia("Image saved: "+path, path)

			if len(result.Content) != 1 {
				t.Fatalf("content blocks = %d, want 1 (text only)", len(result.Content))
			}
			if got := firstTextBlock(t, result).Text; got != "Image saved: "+path {
				t.Errorf("text = %q, want the original text unchanged", got)
			}
		})
	}
}

func TestToolResultTextWithMediaCountCap(t *testing.T) {
	dir := t.TempDir()
	paths := make([]string, 0, maxEmbeddedFiles+1)
	for i := 0; i <= maxEmbeddedFiles; i++ {
		paths = append(paths, writeMediaFile(t, dir, string(rune('a'+i))+".png", []byte("png")))
	}

	result := toolResultTextWithMedia("saved", paths...)

	if len(result.Content) != 1+maxEmbeddedFiles {
		t.Fatalf("content blocks = %d, want %d", len(result.Content), 1+maxEmbeddedFiles)
	}
	text := firstTextBlock(t, result).Text
	if !strings.Contains(text, "(1 file(s) not inlined: exceeds size or count limit)") {
		t.Errorf("text %q missing skip note", text)
	}
}

func TestToolResultTextWithMediaSizeCap(t *testing.T) {
	dir := t.TempDir()
	oversized := make([]byte, maxEmbeddedFileSize+1)
	path := writeMediaFile(t, dir, "big.png", oversized)

	result := toolResultTextWithMedia("saved", path)

	if len(result.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1 (text only)", len(result.Content))
	}
	text := firstTextBlock(t, result).Text
	if !strings.Contains(text, "(1 file(s) not inlined: exceeds size or count limit)") {
		t.Errorf("text %q missing skip note", text)
	}
}

func TestToolResultTextWithMediaSkipsUnknownExtensions(t *testing.T) {
	dir := t.TempDir()
	mp4 := writeMediaFile(t, dir, "clip.mp4", []byte("mp4"))
	txt := writeMediaFile(t, dir, "notes.txt", []byte("txt"))

	result := toolResultTextWithMedia("saved", mp4, txt)

	if len(result.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1 (text only)", len(result.Content))
	}
	if got := firstTextBlock(t, result).Text; got != "saved" {
		t.Errorf("text = %q, want %q without skip note", got, "saved")
	}
}

func TestToolResultTextWithMediaSkipsMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "ghost.png")

	result := toolResultTextWithMedia("saved", missing)

	if len(result.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1 (text only)", len(result.Content))
	}
	if got := firstTextBlock(t, result).Text; got != "saved" {
		t.Errorf("text = %q, want %q without skip note", got, "saved")
	}
}
