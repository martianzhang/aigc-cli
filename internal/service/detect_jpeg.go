package service

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ── JPEG metadata parsing ──

type jpegInfo struct {
	comment   string
	software  string
	hasC2PA   bool
	tc260Data string      // China TC260 AIGC label (from EXIF/XMP)
	camera    *CameraInfo // EXIF camera metadata
}

// readJPEGInfo reads JPEG markers and extracts comment and software tags.
func readJPEGInfo(r io.Reader) jpegInfo {
	var info jpegInfo

	// JPEG starts with FF D8 (SOI)
	br := bufio.NewReader(r)
	soi := make([]byte, 2)
	if _, err := io.ReadFull(br, soi); err != nil || soi[0] != 0xFF || soi[1] != 0xD8 {
		return info
	}

	for {
		// Read marker
		b, err := br.ReadByte()
		if err != nil {
			break
		}
		if b != 0xFF {
			break
		}
		// Skip 0xFF padding bytes
		for b == 0xFF {
			b, err = br.ReadByte()
			if err != nil {
				return info
			}
		}
		markerType := b

		// SOS — start of scan data, stop parsing
		if markerType == 0xDA {
			break
		}
		// EOI / RST markers — no segment length
		if markerType == 0xD9 || (markerType >= 0xD0 && markerType <= 0xD7) {
			continue
		}

		// Read segment length (2 bytes, big-endian, includes self)
		lenBytes := make([]byte, 2)
		if _, err := io.ReadFull(br, lenBytes); err != nil {
			break
		}
		segLen := int(binary.BigEndian.Uint16(lenBytes)) - 2
		if segLen <= 0 {
			continue
		}

		segData := make([]byte, segLen)
		if _, err := io.ReadFull(br, segData); err != nil {
			break
		}

		switch markerType {
		case 0xFE: // COM — comment
			info.comment = strings.TrimSpace(string(segData))
		case 0xE1: // APP1 — EXIF / XMP
			if len(segData) > 6 && string(segData[:6]) == "Exif\000\000" {
				exifData := segData[6:]
				if sw := readExifSoftware(exifData); sw != "" {
					info.software = sw
				}
				// Scan EXIF for TC260 AIGC label (stored as JSON in EXIF)
				if tc260 := scanEXIFForTC260(exifData); tc260 != "" {
					info.tc260Data = tc260
				}
				info.camera = readExifCamera(exifData)
			}
		case 0xEB: // APP11 — JUMBF / C2PA
			if len(segData) > 4 && string(segData[:4]) == "JUMBF" {
				info.hasC2PA = true
			}
		}
	}

	return info
}

// readExifSoftware does a minimal parse of the EXIF IFD0 to find the Software tag (0x0131).
// exifData starts after the "Exif\0\0" header — the TIFF header.
func readExifSoftware(exifData []byte) string {
	if len(exifData) < 8 {
		return ""
	}
	// TIFF header: byte order (2 bytes) + magic (2 bytes) + offset to IFD0 (4 bytes)
	byteOrder := string(exifData[:2])
	var order binary.ByteOrder
	switch byteOrder {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return ""
	}

	if order.Uint16(exifData[2:4]) != 42 {
		return ""
	}

	ifdOffset := int(order.Uint32(exifData[4:8]))
	if ifdOffset+2 > len(exifData) {
		return ""
	}

	numEntries := int(order.Uint16(exifData[ifdOffset : ifdOffset+2]))
	offset := ifdOffset + 2
	for i := 0; i < numEntries && offset+12 <= len(exifData); i++ {
		tag := order.Uint16(exifData[offset : offset+2])
		// typeField := order.Uint16(exifData[offset+2 : offset+4])
		// count := order.Uint32(exifData[offset+4 : offset+8])
		valueOffset := order.Uint32(exifData[offset+8 : offset+12])

		if tag == 0x0131 { // Software
			// Try direct as ASCII (short values stored inline)
			sw := strings.TrimRight(string(exifData[offset+8:offset+12]), "\000")
			if !hasControlChars(sw) {
				return sw
			}
			// Otherwise read at value offset if reasonable
			if int(valueOffset)+64 <= len(exifData) {
				end := int(valueOffset) + 64
				if end > len(exifData) {
					end = len(exifData)
				}
				sw = strings.TrimRight(string(exifData[valueOffset:end]), "\000")
				if !hasControlChars(sw) && sw != "" {
					return sw
				}
			}
		}
		offset += 12
	}
	return ""
}

// scanEXIFForTC260 scans EXIF TIFF data for a TC260 AIGC label.
// The label is stored as a JSON object within the EXIF data, typically
// as {"AIGC": {"Label":"1","ContentProducer":"...",...}}.
// This is a raw byte scan since the label can be at any IFD/offset.
func scanEXIFForTC260(exifData []byte) string {
	// Look for the JSON key "AIGC" followed by the TC260 fields
	idx := bytes.Index(exifData, []byte(`"AIGC"`))
	if idx < 0 {
		// Also try "TC260" as key
		idx = bytes.Index(exifData, []byte(`"TC260"`))
	}
	if idx < 0 {
		return ""
	}
	// Find the value part after the key: look for { after :
	afterKey := exifData[idx:]
	braceIdx := bytes.IndexByte(afterKey, '{')
	if braceIdx < 0 {
		return ""
	}
	// Find matching closing brace (simple brace counting)
	depth := 0
	end := braceIdx
	for end < len(afterKey) {
		switch afterKey[end] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				jsonStr := string(afterKey[braceIdx : end+1])
				// Extract the ContentProducer from the inner JSON
				var parsed struct {
					AIGC struct {
						ContentProducer string `json:"ContentProducer"`
					} `json:"AIGC"`
				}
				if json.Unmarshal([]byte(jsonStr), &parsed) == nil && parsed.AIGC.ContentProducer != "" {
					return jsonStr
				}
				// Try flat parsing
				var flat map[string]string
				if json.Unmarshal([]byte(jsonStr), &flat) == nil {
					if cp, ok := flat[ContentProducerKey]; ok && cp != "" {
						return jsonStr
					}
				}
				// Try AIGC as nested map
				var nested map[string]map[string]string
				if json.Unmarshal([]byte(jsonStr), &nested) == nil {
					if aigc, ok := nested["AIGC"]; ok {
						if cp, ok := aigc[ContentProducerKey]; ok && cp != "" {
							return jsonStr
						}
					}
				}
				return ""
			}
		}
		end++
	}
	return ""
}

// readExifCamera parses EXIF IFD0 and ExifIFD SubIFD for camera metadata tags.
// Returns nil if no camera information is found.
func readExifCamera(exifData []byte) *CameraInfo {
	if len(exifData) < 8 {
		return nil
	}
	byteOrder := string(exifData[:2])
	var order binary.ByteOrder
	switch byteOrder {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil
	}
	if order.Uint16(exifData[2:4]) != 42 {
		return nil
	}
	ifdOffset := int(order.Uint32(exifData[4:8]))
	if ifdOffset+2 > len(exifData) {
		return nil
	}

	var camera CameraInfo
	var exifIFDOffset int // ExifIFD pointer (tag 0x8769)

	// Pass 1: scan IFD0 for Make/Model/LensModel and ExifIFD pointer
	numEntries := int(order.Uint16(exifData[ifdOffset : ifdOffset+2]))
	offset := ifdOffset + 2
	for i := 0; i < numEntries && offset+12 <= len(exifData); i++ {
		tag := order.Uint16(exifData[offset : offset+2])
		typ := order.Uint16(exifData[offset+2 : offset+4])
		count := order.Uint32(exifData[offset+4 : offset+8])

		switch tag {
		case 0x010F: // Make
			camera.Make = readExifASCII(exifData, order, offset, count)
		case 0x0110: // Model
			camera.Model = readExifASCII(exifData, order, offset, count)
		case 0xA434: // LensModel
			camera.LensModel = readExifASCII(exifData, order, offset, count)
		case 0x8769: // ExifIFD pointer — SubIFD with shooting metadata
			if typ == 4 && count == 1 { // LONG
				exifIFDOffset = int(order.Uint32(exifData[offset+8 : offset+12]))
			}
		}
		offset += 12
	}

	// Pass 2: scan ExifIFD SubIFD for shooting metadata (ISO, FNumber, etc.)
	if exifIFDOffset > 0 && exifIFDOffset+2 <= len(exifData) {
		subNumEntries := int(order.Uint16(exifData[exifIFDOffset : exifIFDOffset+2]))
		subOffset := exifIFDOffset + 2
		for i := 0; i < subNumEntries && subOffset+12 <= len(exifData); i++ {
			tag := order.Uint16(exifData[subOffset : subOffset+2])
			typ := order.Uint16(exifData[subOffset+2 : subOffset+4])
			count := order.Uint32(exifData[subOffset+4 : subOffset+8])

			switch tag {
			case 0x920A: // FocalLength
				if camera.FocalLength == "" {
					if num, den := readExifRATIONAL(exifData, order, subOffset, typ, count); den != 0 {
						camera.FocalLength = fmt.Sprintf("%.1fmm", float64(num)/float64(den))
					}
				}
			case 0x829D: // FNumber
				if camera.FNumber == "" {
					if num, den := readExifRATIONAL(exifData, order, subOffset, typ, count); den != 0 {
						camera.FNumber = fmt.Sprintf("%.1f", float64(num)/float64(den))
					}
				}
			case 0x8827: // ISOSpeedRatings
				if camera.ISO == "" {
					if v := readExifSHORT(exifData, order, subOffset, typ, count); v != 0 {
						camera.ISO = fmt.Sprintf("%d", v)
					}
				}
			case 0x829A: // ExposureTime
				if camera.ExposureTime == "" {
					if num, den := readExifRATIONAL(exifData, order, subOffset, typ, count); den != 0 {
						camera.ExposureTime = formatExposureTime(num, den)
					}
				}
			case 0xA434: // LensModel (some cameras put it in SubIFD)
				if camera.LensModel == "" {
					camera.LensModel = readExifASCII(exifData, order, subOffset, count)
				}
			}
			subOffset += 12
		}
	}

	if camera.Make == "" && camera.Model == "" && camera.LensModel == "" &&
		camera.FocalLength == "" && camera.FNumber == "" && camera.ISO == "" && camera.ExposureTime == "" {
		return nil
	}
	return &camera
}

// readExifASCII reads an ASCII string from an EXIF IFD entry.
// The entry starts at entryOffset and the value/offset field is at entryOffset+8.
func readExifASCII(exifData []byte, order binary.ByteOrder, entryOffset int, count uint32) string {
	if count == 0 {
		return ""
	}
	totalSize := int(count) // type=2, size=1 byte per count
	var data []byte
	if totalSize <= 4 {
		// Inline: stored in bytes 8-11 of the 12-byte entry
		data = exifData[entryOffset+8 : entryOffset+8+totalSize]
	} else {
		off := int(order.Uint32(exifData[entryOffset+8 : entryOffset+12]))
		if off+totalSize > len(exifData) {
			return ""
		}
		data = exifData[off : off+totalSize]
	}
	s := strings.TrimRight(string(data), "\x00")
	return strings.TrimSpace(s)
}

// readExifSHORT reads a SHORT value from an EXIF IFD entry.
func readExifSHORT(exifData []byte, order binary.ByteOrder, entryOffset int, typ uint16, count uint32) uint16 {
	if typ != 3 || count != 1 { // SHORT=3
		return 0
	}
	// Inline: 2 bytes stored in the low bytes of the value field
	return order.Uint16(exifData[entryOffset+8 : entryOffset+10])
}

// readExifRATIONAL reads a RATIONAL (two LONGs) from an EXIF IFD entry.
func readExifRATIONAL(exifData []byte, order binary.ByteOrder, entryOffset int, typ uint16, count uint32) (num, den uint32) {
	if typ != 5 || count != 1 { // RATIONAL=5
		return 0, 0
	}
	off := int(order.Uint32(exifData[entryOffset+8 : entryOffset+12]))
	if off+8 > len(exifData) {
		return 0, 0
	}
	num = order.Uint32(exifData[off : off+4])
	den = order.Uint32(exifData[off+4 : off+8])
	return num, den
}

// formatExposureTime formats a rational exposure time as "1/250" or "0.5000".
func formatExposureTime(num, den uint32) string {
	if den == 0 {
		return ""
	}
	if num == 1 {
		return fmt.Sprintf("1/%d", den)
	}
	val := float64(num) / float64(den)
	if val >= 1 {
		return fmt.Sprintf("%.2f", val)
	}
	return fmt.Sprintf("%.4f", val)
}
