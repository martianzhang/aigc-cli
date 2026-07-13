package service

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"strings"
)

// ── PNG chunk parsing ──

// readPNGChunks reads PNG chunks and returns:
//   - textChunks: tEXt/zTXt/iTXt keyword→value pairs
//   - c2paData: raw bytes of the first C2PA-related custom chunk (caBX, C2PA, etc.)
//
// The reader must be at offset 0.
func readPNGChunks(r io.Reader) (textChunks map[string]string, c2paData []byte) {
	textChunks = make(map[string]string)

	sig := make([]byte, 8)
	if _, err := io.ReadFull(r, sig); err != nil {
		return nil, nil
	}

	br := bufio.NewReader(r)
chunks:
	for {
		hdr := make([]byte, 8)
		if _, err := io.ReadFull(br, hdr); err != nil {
			break
		}
		length := binary.BigEndian.Uint32(hdr[:4])
		chunkType := string(hdr[4:8])

		data := make([]byte, length)
		if _, err := io.ReadFull(br, data); err != nil {
			break
		}

		crc := make([]byte, 4)
		if _, err := io.ReadFull(br, crc); err != nil {
			break
		}

		switch chunkType {
		case "tEXt":
			if k, v, ok := parsePNGText(data); ok {
				if k == "AIGC" {
					k = "TC260" // TC260 label in tEXt chunk
				}
				textChunks[k] = v
			}
		case "zTXt":
			if k, v, ok := parsePNGCompressedText(data); ok {
				textChunks[k] = v
			}
		case "iTXt":
			if k, v, ok := parsePNGInternationalText(data); ok {
				textChunks[k] = v
			}
		case "C2PA", "caBX":
			if c2paData == nil {
				c2paData = data
			}
		case "AIGC":
			// China TC260 AIGC label (GB 45438-2025)
			textChunks["TC260"] = string(data)
		case "IEND":
			break chunks
		}
	}

	return textChunks, c2paData
}

// extractC2PAInfo scans JUMBF/C2PA binary data for human-readable
// provenance strings (vendor, software agent, source type).
func extractC2PAInfo(data []byte) (vendor, software, version, sourceType string) {
	if len(data) == 0 {
		return "", "", "", ""
	}

	// Scan for printable ASCII runs
	var current []byte
	strSet := make(map[string]bool)
	addString := func(s string) {
		if len(s) > 4 && !strSet[s] {
			strSet[s] = true
		}
	}

	for _, b := range data {
		if b >= 32 && b < 127 {
			current = append(current, b)
		} else {
			if len(current) > 4 {
				addString(string(current))
			}
			current = nil
		}
	}
	if len(current) > 4 {
		addString(string(current))
	}

	// Extract vendor from organization names
	for s := range strSet {
		low := strings.ToLower(s)
		if strings.Contains(low, "openai") && vendor == "" {
			vendor = s
		}
	}

	// Extract software name + version from entries like "gpt-image" + "2.0"
	for s := range strSet {
		low := strings.ToLower(s)
		if strings.Contains(low, "gpt") || strings.Contains(low, "dall") ||
			strings.Contains(low, "midjourney") || strings.Contains(low, "stable") ||
			strings.Contains(low, "firefly") || strings.Contains(low, "imagen") {
			software = s
		}
		if len(s) <= 10 && len(s) > 0 && (s[0] >= '0' && s[0] <= '9') &&
			strings.Contains(s, ".") && version == "" {
			// Looks like a version number like "2.0"
			version = s
		}
	}

	// Extract digitalSourceType (last path segment of URL)
	for s := range strSet {
		if strings.Contains(s, "digitalsourcetype/") {
			if idx := strings.LastIndex(s, "/"); idx >= 0 && idx+1 < len(s) {
				sourceType = s[idx+1:]
				// Convert camelCase to readable
				sourceType = camelToWords(sourceType)
			}
		}
	}

	return vendor, software, version, sourceType
}

// parsePNGText parses a tEXt chunk: null-terminated keyword + Latin-1 text.
func parsePNGText(data []byte) (key, value string, ok bool) {
	nullIdx := bytes.IndexByte(data, 0)
	if nullIdx < 0 || nullIdx > 79 {
		return "", "", false
	}
	key = string(data[:nullIdx])
	value = string(data[nullIdx+1:])
	return key, value, true
}

// parsePNGCompressedText parses a zTXt chunk:
// null-terminated keyword + compression method (1 byte) + compressed data.
func parsePNGCompressedText(data []byte) (key, value string, ok bool) {
	nullIdx := bytes.IndexByte(data, 0)
	if nullIdx < 0 || nullIdx > 79 || nullIdx+1 >= len(data) {
		return "", "", false
	}
	key = string(data[:nullIdx])
	compressed := data[nullIdx+2:] // skip null + compression method
	if len(compressed) == 0 {
		return key, "", true
	}
	rc, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return "", "", false
	}
	defer rc.Close()
	decompressed, err := io.ReadAll(rc)
	if err != nil {
		return "", "", false
	}
	return key, string(decompressed), true
}

// parsePNGInternationalText parses an iTXt chunk.
func parsePNGInternationalText(data []byte) (key, value string, ok bool) {
	nullIdx := bytes.IndexByte(data, 0)
	if nullIdx < 0 || nullIdx > 79 || nullIdx+3 >= len(data) {
		return "", "", false
	}
	key = string(data[:nullIdx])
	rest := data[nullIdx+1:] // rest = [comp_flag, comp_method, lang\0, trans_keyword\0, text...]
	if len(rest) < 3 {
		return "", "", false
	}
	compressionFlag := rest[0]
	// compressionMethod := rest[1] — always 0 for deflate

	// Language tag starts at rest[2], null-terminated
	langStart := 2
	langEnd := langStart
	for langEnd < len(rest) && rest[langEnd] != 0 {
		langEnd++
	}
	if langEnd >= len(rest) {
		return "", "", false
	}
	// Translated keyword: null-terminated after language tag
	tkStart := langEnd + 1
	tkEnd := tkStart
	for tkEnd < len(rest) && rest[tkEnd] != 0 {
		tkEnd++
	}
	if tkEnd >= len(rest) {
		return "", "", false
	}
	textBytes := rest[tkEnd+1:]

	if compressionFlag == 1 {
		rc, err := zlib.NewReader(bytes.NewReader(textBytes))
		if err != nil {
			return "", "", false
		}
		defer rc.Close()
		decompressed, err := io.ReadAll(rc)
		if err != nil {
			return "", "", false
		}
		value = string(decompressed)
	} else {
		value = string(textBytes)
	}
	return key, value, true
}
