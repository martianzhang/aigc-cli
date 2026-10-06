package service

import (
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
)

// maxLocalImageBytes caps local images embedded as data URIs at 32 MiB.
const maxLocalImageBytes = 32 << 20

// readLocalImage reads and validates a local image file, returning its bytes
// and MIME subtype. Shared by the data-URI and raw-base64 encoders so both
// enforce the same size cap and decodability check.
func readLocalImage(path string) ([]byte, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read image %q: %w", path, err)
	}
	if info.Size() > maxLocalImageBytes {
		return nil, "", fmt.Errorf("local image exceeds 32 MiB: %s", path)
	}
	if err := validateLocalImage(path); err != nil {
		return nil, "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read image %q: %w", path, err)
	}
	return data, imageMIME(data, path), nil
}

// ImageToDataURI reads a local image file and returns a data: URI
// (e.g. data:image/png;base64,...) accepted by providers without an
// upload endpoint (Agnes).
func ImageToDataURI(path string) (string, error) {
	data, mime, err := readLocalImage(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("data:image/%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data)), nil
}

// validateLocalImage rejects any local file that is not a decodable image
// (e.g. ~/.ssh/id_rsa), so it can never be embedded as an "image". A decoder
// that panics (the jpegli wasm decoder panics on some valid progressive JPEGs)
// is recovered and falls back to a magic-byte signature check, so a valid
// image is never rejected and the process never crashes.
func validateLocalImage(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to read image %q: %w", path, err)
	}
	defer f.Close()
	if _, _, err := decodeConfigSafe(f); err != nil {
		if data, readErr := os.ReadFile(path); readErr == nil && imgcodec.SniffImageExt(data) != "" {
			return nil
		}
		return fmt.Errorf("not a valid image file: %s", path)
	}
	return nil
}

// decodeConfigSafe runs image.DecodeConfig, converting a decoder panic into an
// error so a buggy decoder cannot crash the process.
func decodeConfigSafe(r io.Reader) (cfg image.Config, format string, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("image decoder panicked: %v", rec)
		}
	}()
	return image.DecodeConfig(r)
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
