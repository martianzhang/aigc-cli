package service

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"strings"
)

// ── GIF comment extraction ──

// readGIFInfo reads GIF comment extension blocks (0xFE).
func readGIFInfo(r io.Reader) string {
	br := bufio.NewReader(r)

	// Skip header (6 bytes: GIF87a/GIF89a) + logical screen descriptor (7 bytes)
	if _, err := br.Discard(13); err != nil {
		return ""
	}

	// Scan blocks for comment extensions
	for {
		b, err := br.ReadByte()
		if err != nil {
			break
		}
		switch b {
		case 0x21: // Extension introducer
			label, err := br.ReadByte()
			if err != nil {
				return ""
			}
			if label == 0xFE { // Comment extension
				if comment := readGIFSubBlocks(br); comment != "" {
					return comment
				}
			} else {
				skipGIFBlocks(br)
			}
		case 0x2C: // Image descriptor
			// Skip image descriptor + local color table + LZW data
			skipGIFImageData(br)
		case 0x3B: // Trailer
			return ""
		default:
			return ""
		}
	}
	return ""
}

// readGIFSubBlocks reads GIF sub-blocks until a zero-length terminator.
func readGIFSubBlocks(br *bufio.Reader) string {
	var result []byte
	for {
		size, err := br.ReadByte()
		if err != nil || size == 0 {
			break
		}
		block := make([]byte, size)
		if _, err := io.ReadFull(br, block); err != nil {
			return ""
		}
		result = append(result, block...)
	}
	return strings.TrimSpace(string(result))
}

// skipGIFBlocks skips sub-blocks until a zero-length terminator.
func skipGIFBlocks(br *bufio.Reader) {
	for {
		size, err := br.ReadByte()
		if err != nil || size == 0 {
			break
		}
		if _, err := br.Discard(int(size)); err != nil {
			return
		}
	}
}

// skipGIFImageData skips a GIF image descriptor + local color table + LZW data.
func skipGIFImageData(br *bufio.Reader) {
	// Image descriptor: 9 bytes
	if _, err := br.Discard(9); err != nil {
		return
	}
	// Skip LZW minimum code size (1 byte) + sub-blocks
	skipGIFBlocks(br)
}

// ── WebP RIFF chunk parsing ──

// webpInfo holds metadata extracted from WebP RIFF container.
type webpInfo struct {
	comment string
	hasC2PA bool
}

// readWebPInfo reads WebP RIFF container for EXIF/XMP/C2PA metadata.
func readWebPInfo(r io.Reader) webpInfo {
	var info webpInfo

	data, err := io.ReadAll(r)
	if err != nil || len(data) < 12 {
		return info
	}

	// Validate RIFF header
	if string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return info
	}

	fileSize := int(binary.LittleEndian.Uint32(data[4:8]))
	if fileSize+8 > len(data) {
		fileSize = len(data) - 8
	}

	pos := 12
	for pos+8 <= fileSize+8 && pos+8 <= len(data) {
		chunkID := string(data[pos : pos+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))

		if chunkSize < 0 || pos+8+chunkSize > len(data) {
			break
		}

		switch chunkID {
		case "EXIF":
			exifStart := pos + 8
			if chunkSize > 6 && string(data[exifStart:exifStart+6]) == "Exif\000\000" {
				exifData := data[exifStart+6 : exifStart+chunkSize]
				if sw := readExifSoftware(exifData); sw != "" {
					info.comment = "EXIF: " + sw
				}
			}
			// Check for JUMBF/C2PA in EXIF chunk
			if bytes.Contains(data[pos:pos+8+chunkSize], []byte("jumb")) ||
				bytes.Contains(data[pos:pos+8+chunkSize], []byte("C2PA")) {
				info.hasC2PA = true
			}
		case "XMP":
			if !info.hasC2PA {
				if bytes.Contains(data[pos:pos+8+chunkSize], []byte("jumb")) {
					info.hasC2PA = true
				}
			}
		}

		pos += 8 + chunkSize
		// Chunk data is padded to even bytes
		if chunkSize%2 != 0 {
			pos++
		}
	}

	return info
}
