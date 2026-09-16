package service

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
)

// ImageToDataURI reads a local image file and returns a data: URI
// (e.g. data:image/png;base64,...) accepted by providers without an
// upload endpoint (Agnes).
func ImageToDataURI(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read image %q: %w", path, err)
	}
	mime := imageMIME(data, path)
	return fmt.Sprintf("data:image/%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data)), nil
}

// LocalFilesToDataURI converts entries that are local files into data URIs.
// Public URLs, existing data: URIs and non-file strings pass through unchanged.
func LocalFilesToDataURI(inputs []string) ([]string, error) {
	out := make([]string, len(inputs))
	copy(out, inputs)
	for i, in := range out {
		if !IsFile(in) {
			continue
		}
		uri, err := ImageToDataURI(in)
		if err != nil {
			return nil, err
		}
		out[i] = uri
	}
	return out, nil
}

// imageMIME resolves the image MIME subtype: sniff the bytes first, then fall
// back to the file extension, then default to png.
func imageMIME(data []byte, path string) string {
	if ext := imgcodec.SniffImageExt(data); ext != "" {
		return normalizeImageExt(ext)
	}
	return normalizeImageExt(filepath.Ext(path))
}

// normalizeImageExt maps a sniffed extension or file extension (with or without
// a leading dot) to the MIME subtype used in a data URI.
func normalizeImageExt(ext string) string {
	switch strings.TrimPrefix(strings.ToLower(ext), ".") {
	case "jpg", "jpeg":
		return "jpeg"
	case "png", "webp", "gif", "bmp":
		return strings.TrimPrefix(strings.ToLower(ext), ".")
	default:
		return "png"
	}
}
