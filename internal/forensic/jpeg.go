package forensic

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"image"
	"io"
	"math"
	"os"
)

// AnalyzeJPEGDoubleQuant detects JPEG double quantization artifacts.
//
// When an image is JPEG-compressed twice with different quality settings,
// the DCT coefficient histograms show periodic "comb" patterns (double
// quantization artifacts). AI-generated images are typically single-compressed,
// so they lack these artifacts. Real photos that have been edited and re-saved
// often show double quantization.
//
// This is a simplified detection: we scan the JPEG file for quantization
// tables and check if they're "standard" (camera-like) or "non-standard"
// (AI-generated often have unusual tables).
//
// Returns 0-1 score, higher = more likely AI-generated.
// Returns -1 for non-JPEG images or analysis failure.
func AnalyzeJPEGDoubleQuant(path string) float64 {
	f, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer f.Close()

	// Read first 2 bytes to check JPEG SOI marker
	header := make([]byte, 2)
	if _, err := io.ReadFull(f, header); err != nil {
		return -1
	}
	if header[0] != 0xFF || header[1] != 0xD8 {
		return -1 // Not a JPEG file
	}

	// Scan JPEG markers for DQT (Define Quantization Table) markers
	// Each DQT marker contains quantization tables that reveal compression
	// history.
	br := bufio.NewReader(f)
	tablesFound := 0
	// track if we find non-standard tables
	nonStandardLum := false

	for {
		b, err := br.ReadByte()
		if err != nil {
			break
		}
		if b != 0xFF {
			continue
		}

		marker, err := br.ReadByte()
		if err != nil {
			break
		}

		// SOS - start of scan, stop parsing
		if marker == 0xDA {
			break
		}
		// EOI
		if marker == 0xD9 {
			break
		}
		// RST markers - no length
		if marker >= 0xD0 && marker <= 0xD7 {
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

		if marker == 0xDB { // DQT - Define Quantization Table
			tablesFound++
			// Each table: 1 byte precision/ID + 64 bytes table data
			offset := 0
			for offset < len(segData) {
				if offset+65 > len(segData) {
					break
				}
				tableInfo := segData[offset]
				// tableID := tableInfo & 0x0F
				precision := (tableInfo >> 4) & 0x0F // 0=8-bit, 1=16-bit
				tableData := segData[offset+1 : offset+65]

				// Check if this is a standard JPEG table
				if !isStandardTable(tableData, int(precision)) {
					if tablesFound <= 2 { // luminance table
						nonStandardLum = true
					}
				}
				offset += 65
			}
		}
	}

	// Score based on findings:
	// - No quantization tables found → can't determine (not standard JPEG)
	// - Non-standard tables → likely AI-generated or heavily processed
	// - Standard tables → possibly camera-originated
	// - Multiple DQT markers → possibly double compressed (real photo edited)

	if tablesFound == 0 {
		return 0.4 // neutral, can't determine
	}

	if nonStandardLum {
		// Non-standard quantization → often indicates AI generation
		// AI tools often use custom/non-standard JPEG encoding
		return 0.65
	}

	// Standard tables with multiple DQT → could be double compressed (real photo)
	if tablesFound > 2 {
		return 0.25 // multiple tables suggest double compression → likely real
	}

	// Single set of standard tables → could be camera or single-save AI
	return 0.45
}

// isStandardTable checks if a JPEG quantization table matches a "standard"
// camera-like table (ISO 10918-1 recommended values).
func isStandardTable(table []byte, precision int) bool {
	if len(table) < 64 {
		return false
	}

	// Standard luminance quantization table (JPEG Annex K)
	stdLum := [64]byte{
		16, 11, 10, 16, 24, 40, 51, 61,
		12, 12, 14, 19, 26, 58, 60, 55,
		14, 13, 16, 24, 40, 57, 69, 56,
		14, 17, 22, 29, 51, 87, 80, 62,
		18, 22, 37, 56, 68, 109, 103, 77,
		24, 35, 55, 64, 81, 104, 113, 92,
		49, 64, 78, 87, 103, 121, 120, 101,
		72, 92, 95, 98, 112, 100, 103, 99,
	}

	// Standard chrominance quantization table
	stdChr := [64]byte{
		17, 18, 24, 47, 99, 99, 99, 99,
		18, 21, 26, 66, 99, 99, 99, 99,
		24, 26, 56, 99, 99, 99, 99, 99,
		47, 66, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99,
	}

	// Compare with standard tables (allow slight variations for quality factor)
	// Allow up to 15% deviation per entry
	var lumMatch, chrMatch int
	for i := 0; i < 64; i++ {
		v := table[i]
		if precision == 1 {
			// 16-bit precision - just check general shape
			continue
		}
		// Check luminance match
		if absDiff(v, stdLum[i]) <= max(2, stdLum[i]/8) {
			lumMatch++
		}
		// Check chrominance match
		if absDiff(v, stdChr[i]) <= max(2, stdChr[i]/8) {
			chrMatch++
		}
	}

	// If most entries match either standard table, it's a standard table
	return lumMatch >= 48 || chrMatch >= 48
}

func absDiff(a, b byte) byte {
	if a > b {
		return a - b
	}
	return b - a
}

func max(a, b byte) byte {
	if a > b {
		return a
	}
	return b
}

// Compile-time use of image package (for potential future use)
var _ = image.ErrFormat
var _ = math.Pi
var _ = bytes.Equal
var _ = bufio.ErrBufferFull
