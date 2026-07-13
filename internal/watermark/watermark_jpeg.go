package watermark

import (
	"bufio"
	"encoding/binary"
	"io"
	"math"
	"os"
)

func EstimateJPEGQuality(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 90
	}
	defer f.Close()

	br := bufio.NewReader(f)
	// SOI marker
	soi := make([]byte, 2)
	if _, err := io.ReadFull(br, soi); err != nil || soi[0] != 0xFF || soi[1] != 0xD8 {
		return 90
	}

	for {
		b, err := br.ReadByte()
		if err != nil {
			return 90
		}
		// Skip 0xFF padding
		for b == 0xFF {
			b, err = br.ReadByte()
			if err != nil {
				return 90
			}
		}
		marker := b

		// SOS — start of scan data, stop looking
		if marker == 0xDA {
			break
		}
		// RST / EOI — no segment length
		if marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) {
			continue
		}

		lenBytes := make([]byte, 2)
		if _, err := io.ReadFull(br, lenBytes); err != nil {
			return 90
		}
		segLen := int(binary.BigEndian.Uint16(lenBytes)) - 2
		if segLen <= 0 {
			continue
		}

		// DQT marker — extract the first luminance table
		if marker == 0xDB {
			segData := make([]byte, segLen)
			if _, err := io.ReadFull(br, segData); err != nil {
				return 90
			}
			if q := qualityFromDQT(segData); q > 0 {
				return q
			}
			// DQT found but couldn't estimate quality — still try other markers
			continue
		}

		// Skip other segments
		if _, err := br.Discard(segLen); err != nil {
			return 90
		}
	}
	return 90
}

// qualityFromDQT extracts the luminance quantization table from a DQT segment
// and estimates the JPEG quality using the IJG formula.
func qualityFromDQT(dqtData []byte) int {
	if len(dqtData) < 65 {
		return 0
	}
	precision := (dqtData[0] >> 4) & 0x0F

	var table [64]int
	if precision == 0 {
		for i := 0; i < 64; i++ {
			table[i] = int(dqtData[1+i])
		}
	} else {
		if len(dqtData) < 129 {
			return 0
		}
		for i := 0; i < 64; i++ {
			table[i] = int(binary.BigEndian.Uint16(dqtData[1+i*2:]))
		}
	}

	// Estimate quality from each non-zero table entry using the IJG formula:
	//   Q >= 50:  tbl[i] = clamp((std[i]*(200-2*Q)+50)/100, 1, 255)
	//   Q <  50:  tbl[i] = clamp((std[i]*5000/Q+50)/100, 1, 255)
	var qSum float64
	var qCount float64

	for i := 0; i < 64; i++ {
		t := table[i]
		s := standardLumTable[i]
		if s == 0 || t == 0 {
			continue
		}

		// Try Q >= 50 formula first
		// 100*t ≈ s*(200-2*Q)+50  →  Q ≈ (200 - (100*t-50)/s) / 2
		num := float64(100*t - 50)
		den := float64(s)
		qHigh := (200.0 - num/den) / 2.0

		if qHigh >= 50 && qHigh <= 100 {
			qSum += qHigh
			qCount++
			continue
		}

		// Try Q < 50 formula
		// 100*t ≈ s*5000/Q+50  →  Q ≈ s*5000 / (100*t-50)
		if num > 0 {
			qLow := float64(s) * 5000.0 / num
			if qLow >= 1 && qLow < 50 {
				qSum += qLow
				qCount++
			}
		}
	}

	if qCount == 0 {
		return 0
	}

	q := int(math.Round(qSum / qCount))
	if q < 10 {
		return 10
	}
	if q > 100 {
		return 100
	}
	return q
}

// AddWatermarkFile adds a visible watermark to an image file and saves the result.
// For gemini, the watermark matches the AI provider's visible mark.
// For unknown producers, the producer text is rendered as a watermark.
//
// Note: This function only adds a visible watermark for testing the removal feature.
// It does NOT inject TC260 or any other metadata — the output is a plain PNG
// with no AIGC provenance claims.