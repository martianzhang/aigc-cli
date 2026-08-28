package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	_ "golang.org/x/image/bmp"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
)

// Supported target formats for DecodeImageInput conversion.
var supportedDecodeTargets = map[string]bool{
	"png": true, "jpg": true, "jpeg": true, "webp": true, "avif": true, "jxl": true,
}

// imageExtMIME maps detected file extensions to MIME types for data URI output.
var imageExtMIME = map[string]string{
	".jpg": "image/jpeg", ".png": "image/png", ".gif": "image/gif",
	".bmp": "image/bmp", ".webp": "image/webp", ".avif": "image/avif",
	".heic": "image/heic", ".heif": "image/heif", ".jxl": "image/jxl",
}

// ImageExtMIME returns the MIME type for a detected image extension (e.g. ".jpg" → "image/jpeg").
// Returns "" when the extension is unknown.
func ImageExtMIME(ext string) string {
	return imageExtMIME[ext]
}

// DecodeImageInput decodes image content that may be a data URI, raw base64 text,
// or already-encoded image bytes, and optionally converts it to targetFormat.
//
// targetFormat accepts:
//   - image formats: png, jpg/jpeg, webp, avif, jxl (empty = keep detected format)
//   - base64: output pure base64 text (no prefix, base64 -d restores the image)
//   - datauri: output a data URI (data:<mime>;base64,...)
//
// Returns the output bytes and the file extension (with dot, e.g. ".jpg" or ".txt").
func DecodeImageInput(content []byte, targetFormat string) ([]byte, string, error) {
	raw, err := resolveImageBytes(content)
	if err != nil {
		return nil, "", err
	}
	ext := imgcodec.SniffImageExt(raw)
	if ext == "" {
		return nil, "", fmt.Errorf("decoded data is not a recognizable image")
	}

	target := strings.ToLower(strings.TrimSpace(targetFormat))
	if target == "jpeg" {
		target = "jpg"
	}
	switch target {
	case "base64":
		return []byte(base64.StdEncoding.EncodeToString(raw)), ".txt", nil
	case "datauri", "data-uri":
		mime := imageExtMIME[ext]
		if mime == "" {
			mime = "image"
		}
		return []byte("data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)), ".txt", nil
	case "":
		return raw, ext, nil
	}
	if !supportedDecodeTargets[target] {
		return nil, "", fmt.Errorf("unsupported target format %q (supported: png, jpg, webp, avif, jxl, base64, datauri)", targetFormat)
	}
	if "."+target == ext {
		return raw, ext, nil // already in target format
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", fmt.Errorf("decode image for conversion: %w", err)
	}
	var buf bytes.Buffer
	if _, err := imgcodec.Encode(&buf, img, target, 85); err != nil {
		return nil, "", fmt.Errorf("encode %s: %w", target, err)
	}
	return buf.Bytes(), "." + target, nil
}

// resolveImageBytes parses content into image bytes:
//   - data: URI (e.g. data:image/jpeg;base64,...)
//   - raw base64 text (decoded when the result sniffs as an image)
//   - already-encoded image bytes (passed through unchanged)
func resolveImageBytes(content []byte) ([]byte, error) {
	s := strings.TrimSpace(string(content))
	if s == "" {
		return nil, fmt.Errorf("empty input")
	}

	// data: URI
	if strings.HasPrefix(s, "data:") {
		commaIdx := strings.Index(s, ",")
		if commaIdx < 0 {
			return nil, fmt.Errorf("malformed data URI (missing comma)")
		}
		data, err := decodeBase64Any(s[commaIdx+1:])
		if err != nil {
			return nil, fmt.Errorf("decode data URI base64: %w", err)
		}
		if imgcodec.SniffImageExt(data) == "" {
			return nil, fmt.Errorf("data URI does not contain an image")
		}
		return data, nil
	}

	// Already-encoded image bytes
	if imgcodec.SniffImageExt([]byte(s)) != "" {
		return []byte(s), nil
	}

	// Raw base64 text (long enough to plausibly be an image payload)
	if len(s) > 20 {
		if data, err := decodeBase64Any(s); err == nil && imgcodec.SniffImageExt(data) != "" {
			return data, nil
		}
	}

	return nil, fmt.Errorf("input is not a data URI, base64 image, or image file")
}
