package watermark

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// TC260 metadata constants for watermark injection.
const (
	tc260ChunkKey = "TC260"
	tc260Label    = "1"
)

// AddWatermarkFile adds a visible watermark to an image file and saves the result.
// For known producers (gemini/doubao/jimeng), the watermark matches the AI provider's
// visible mark. For unknown producers, the producer text is rendered as a watermark.
//
// Known producers also get TC260 AIGC metadata injected into the output file
// (as a PNG text chunk), so the result appears AI-generated to detection tools.
// Gemini images skip metadata injection (C2PA requires cryptographic signing).
func AddWatermarkFile(inputPath, outputPath, producer string) (*Result, error) {
	f, err := os.Open(inputPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	f.Close()

	dst, res, err := AddWatermark(img, producer)
	if err != nil {
		return nil, err
	}

	if outputPath == "" {
		outputPath = strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + "_watermarked.png"
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return nil, err
	}
	defer out.Close()

	// Encode as PNG (supports metadata text chunks)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}

	encoded := buf.Bytes()

	// Inject TC260 metadata for known Chinese AI providers
	if producer == "doubao" || producer == "jimeng" {
		encoded, err = embedPNGTextChunk(encoded, tc260ChunkKey, makeTC260JSON(producer))
		if err != nil {
			return nil, fmt.Errorf("embed metadata: %w", err)
		}
	}

	if _, err := out.Write(encoded); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	res.Region = fmt.Sprintf("%s -> %s", inputPath, outputPath)
	return res, nil
}

// makeTC260JSON creates a TC260 JSON string for the given producer.
func makeTC260JSON(producer string) string {
	// Flat format: {"Label":"1","ContentProducer":"doubao"}
	data, _ := json.Marshal(map[string]string{
		"Label":           tc260Label,
		"ContentProducer": producer,
	})
	return string(data)
}

// embedPNGTextChunk inserts a tEXt chunk before IEND in PNG-encoded bytes.
// Properly parses PNG chunk structure to find the real IEND chunk.
func embedPNGTextChunk(pngData []byte, key, value string) ([]byte, error) {
	// Parse PNG chunks to find IEND (the chunk type marker, not a compressed
	// data coincidence).
	var iendIdx int
	found := false
	pos := 8 // skip PNG signature
	for pos+8 <= len(pngData) {
		length := int(binary.BigEndian.Uint32(pngData[pos : pos+4]))
		if chunkType := string(pngData[pos+4 : pos+8]); chunkType == "IEND" {
			iendIdx = pos
			found = true
			break
		}
		pos += 12 + length
	}
	if !found {
		return nil, fmt.Errorf("PNG: IEND chunk not found")
	}

	// Build tEXt chunk: length + "tEXt" + key\0value + CRC
	chunkData := append([]byte(key), 0)
	chunkData = append(chunkData, []byte(value)...)

	chunk := make([]byte, 4+4+len(chunkData)+4)
	binary.BigEndian.PutUint32(chunk[0:4], uint32(len(chunkData))) // length
	copy(chunk[4:8], "tEXt")                                       // type
	copy(chunk[8:8+len(chunkData)], chunkData)                     // data

	// CRC32 of type + data
	crc := crc32.NewIEEE()
	crc.Write(chunk[4 : 8+len(chunkData)])
	binary.BigEndian.PutUint32(chunk[8+len(chunkData):12+len(chunkData)], crc.Sum32())

	// Insert before IEND
	result := make([]byte, 0, len(pngData)+len(chunk))
	result = append(result, pngData[:iendIdx]...)
	result = append(result, chunk...)
	result = append(result, pngData[iendIdx:]...)
	return result, nil
}

// ProducerToConfig maps a TC260 ContentProducer value to a registered config name.
// Handles direct name match and substring match (e.g., "字节跳动 (ByteDance) — 豆包" → "doubao").
func ProducerToConfig(producer string) string {
	if producer == "" {
		return ""
	}
	if _, ok := findConfigByName(producer); ok {
		return producer
	}
	lower := strings.ToLower(producer)
	// Brand aliases: TC260 provider strings are descriptive (e.g.
	// "字节跳动 (ByteDance) — 豆包/即梦/火山引擎") and don't equal a config
	// name. Match by substring so ByteDance → doubao, Google → gemini, etc.
	brandAliases := map[string]string{
		"字节跳动":      "doubao",
		"bytedance": "doubao",
		"doubao":    "doubao",
		"jimeng":    "jimeng",
		"google":    "gemini",
		"gemini":    "gemini",
		"baidu":     "baidu",
	}
	for key, name := range brandAliases {
		if strings.Contains(lower, key) {
			return name
		}
	}
	for _, cfg := range registry {
		if strings.Contains(lower, cfg.Name) {
			return cfg.Name
		}
	}
	return ""
}

// DetectWatermark scans an image for registered watermark types and returns
// detections sorted by confidence (highest first).
func DetectWatermark(img image.Image) []Detection {
	var results []Detection

	for _, cfg := range registry {
		det := detectWatermark(img, cfg)
		if det != nil && det.confidence >= cfg.DetectThreshold {
			results = append(results, Detection{
				Name:       cfg.Name,
				Confidence: det.confidence,
				X:          det.x,
				Y:          det.y,
				Size:       det.size,
				W:          det.w,
				H:          det.h,
			})
		}
	}

	// Sort by confidence descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Confidence > results[i].Confidence {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

// RemoveWatermark detects and removes the best-matching watermark from an image.
// Returns the cleaned image and a result descriptor.
func RemoveWatermark(img image.Image) (*image.RGBA, *Result, error) {
	return RemoveWatermarkHinted(img, "")
}

// RemoveWatermarkHinted detects and removes a watermark, preferring a specific
// config when the producer is known from TC260 metadata (e.g., "doubao", "jimeng").
func RemoveWatermarkHinted(img image.Image, producer string) (*image.RGBA, *Result, error) {
	// When the producer is known, try the hinted config FIRST.
	// For PositionResolver configs (Doubao/Jimeng), use a relaxed threshold
	// because the exact position is known — no false positive risk.
	// For Gemini, use the normal threshold to avoid weak matches.
	if producer != "" {
		if cfg, ok := findConfigByName(producer); ok {
			det := detectWatermark(img, cfg)
			passThreshold := cfg.DetectThreshold
			if cfg.PositionResolver != nil {
				passThreshold = 0.08 // relaxed for known-position text marks
			}
			if det != nil && det.confidence >= passThreshold {
				dst := removeWatermark(img, det, cfg)
				region := fmt.Sprintf("%d,%d,%d,%d", det.x, det.y, det.size, det.size)
				return dst, &Result{
					Removed:    true,
					Name:       cfg.Name,
					Confidence: det.confidence,
					Size:       det.size,
					Region:     region,
				}, nil
			}
		}
	}

	// Fall back to generic detection
	detections := DetectWatermark(img)
	if len(detections) == 0 {
		return nil, &Result{Removed: false}, nil
	}

	best := detections[0]
	cfg, ok := findConfigByName(best.Name)
	if !ok {
		return nil, nil, fmt.Errorf("watermark: unknown config %q", best.Name)
	}

	// Create a candidate from the detection. Carry the detected rectangle
	// dimensions (w/h) so removeWatermark uses the full watermark bounds
	// (isTextWM path) rather than a square det.size region — required to
	// cover the whole watermark.
	det := &candidate{
		x:          best.X,
		y:          best.Y,
		size:       best.Size,
		w:          best.W,
		h:          best.H,
		confidence: best.Confidence,
	}

	dst := removeWatermark(img, det, cfg)

	region := fmt.Sprintf("%d,%d,%d,%d", det.x, det.y, det.size, det.size)
	return dst, &Result{
		Removed:    true,
		Name:       cfg.Name,
		Confidence: best.Confidence,
		Size:       det.size,
		Region:     region,
	}, nil
}

// RemoveFile loads an image, removes watermarks, and saves the result.
func RemoveFile(inputPath, outputPath string) (*Result, error) {
	return RemoveFileHinted(inputPath, outputPath, "")
}

// RemoveFileHinted is like RemoveFile but with a producer hint from metadata.
func RemoveFileHinted(inputPath, outputPath, producer string) (*Result, error) {
	f, err := os.Open(inputPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	f.Close()

	dst, res, err := RemoveWatermarkHinted(img, producer)
	if err != nil {
		return nil, err
	}
	if !res.Removed {
		return res, nil
	}

	if outputPath == "" {
		ext := filepath.Ext(inputPath)
		base := strings.TrimSuffix(inputPath, ext)
		outputPath = base + "_clean" + ext
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return nil, err
	}
	defer out.Close()

	ext := strings.ToLower(filepath.Ext(outputPath))
	switch ext {
	case ".jpg", ".jpeg", ".jfif":
		if err := jpeg.Encode(out, dst, &jpeg.Options{Quality: 95}); err != nil {
			return nil, err
		}
	default:
		if err := png.Encode(out, dst); err != nil {
			return nil, err
		}
	}

	res.Region = fmt.Sprintf("%s -> %s", inputPath, outputPath)
	return res, nil
}
