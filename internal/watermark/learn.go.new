package watermark

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// PNG tEXt metadata keys
const (
	txtName               = "name"
	txtNativeWidth        = "native_width"
	txtMarginXFrac        = "margin_x_frac"
	txtMarginYFrac        = "margin_y_frac"
	txtDetectThreshold    = "detect_threshold"
	txtRemoveStrategy     = "remove_strategy"
	txtOversubtractMargin = "oversubtract_margin"
)

// bgModel describes a per-pixel linear background model:
// bg(channel, x, y) = base + gx*col + gy*row.
type LearnResult struct {
	Name               string
	AlphaMap           *AlphaMap
	NativeWidth        int
	MarginXFrac        float64
	MarginYFrac        float64
	DetectThreshold    float64
	RemoveStrategy     string
	OversubtractMargin float64
}

// buildLearnResult creates a LearnResult from a computed alpha map.
func buildLearnResult(alpha *AlphaMap, imgW, imgH int, name, removeStrategy string) *LearnResult {
	var maxAlpha float64
	for _, v := range alpha.Data {
		if v > maxAlpha {
			maxAlpha = v
		}
	}
	trimThreshold := maxFloat(0.005, maxAlpha*0.02)
	trimmed, offsetX, offsetY, _, _ := TrimAlphaMap(alpha, trimThreshold)
	marginX := imgW - offsetX - trimmed.Width
	marginY := imgH - offsetY - trimmed.Height
	marginXFrac := float64(marginX) / float64(imgW)
	marginYFrac := float64(marginY) / float64(imgH)

	detectThreshold := estimateThreshold(trimmed)
	if name == "gemini" && detectThreshold < 0.25 {
		detectThreshold = 0.25
	}

	return &LearnResult{
		Name:               name,
		AlphaMap:           trimmed,
		NativeWidth:        imgW,
		MarginXFrac:        marginXFrac,
		MarginYFrac:        marginYFrac,
		DetectThreshold:    detectThreshold,
		RemoveStrategy:     removeStrategy,
		OversubtractMargin: 0,
	}
}

// LearnWatermark solves the alpha map from black+gray seed images and
// auto-derives all config parameters.
func LearnWatermark(black, gray image.Image, name string, removeStrategy string) *LearnResult {
	b := black.Bounds()
	alpha := SolveAlphaMap(black, gray)
	return buildLearnResult(alpha, b.Dx(), b.Dy(), name, removeStrategy)
}

// LearnWatermarkMulti averages multiple seed pairs for lower-noise alpha maps.
func LearnWatermarkMulti(blacks, grays []image.Image, name string, removeStrategy string) *LearnResult {
	if len(blacks) == 0 || len(grays) == 0 {
		return nil
	}
	b := blacks[0].Bounds()
	alpha := SolveAlphaMapMulti(blacks, grays)
	return buildLearnResult(alpha, b.Dx(), b.Dy(), name, removeStrategy)
}

// estimateThreshold computes a data-driven detection threshold from the
// trimmed alpha map.  Uses the 90th percentile of non-zero alpha values,
// clamped to [0.15, 0.40].
func estimateThreshold(am *AlphaMap) float64 {
	// Collect non-zero alpha values
	var vals []float64
	for _, v := range am.Data {
		if v > 0.001 {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return 0.30
	}

	// Sort descending, pick the 90th percentile.
	// Alpha maps are small (<10k pixels), so a full sort is fine.
	sort.Slice(vals, func(i, j int) bool { return vals[i] > vals[j] })
	p90Idx := int(float64(len(vals)-1) * 0.90)
	p90 := vals[p90Idx]

	if p90 < 0.15 {
		return 0.15
	}
	if p90 > 0.40 {
		return 0.40
	}
	return p90
}

// SaveWatermarkPNG saves a learned watermark as a self-contained PNG file.
// The alpha map is stored as grayscale pixels; all metadata is embedded
// in PNG tEXt chunks.
func SaveWatermarkPNG(path string, lr *LearnResult) error {
	// Save as 16-bit grayscale PNG to preserve float32 alpha precision.
	// uint8 would quantize alpha to 256 levels; uint16 gives 65536 levels
	// which is sufficient for lossless float32 storage.
	img := image.NewGray16(image.Rect(0, 0, lr.AlphaMap.Width, lr.AlphaMap.Height))
	for y := 0; y < lr.AlphaMap.Height; y++ {
		for x := 0; x < lr.AlphaMap.Width; x++ {
			v := uint16(math.Round(lr.AlphaMap.Data[y*lr.AlphaMap.Width+x] * 65535))
			img.SetGray16(x, y, color.Gray16{Y: v})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("encode alpha map PNG: %w", err)
	}
	meta := map[string]string{
		txtName:               lr.Name,
		txtNativeWidth:        strconv.Itoa(lr.NativeWidth),
		txtMarginXFrac:        floatToStr(lr.MarginXFrac),
		txtMarginYFrac:        floatToStr(lr.MarginYFrac),
		txtDetectThreshold:    floatToStr(lr.DetectThreshold),
		txtRemoveStrategy:     lr.RemoveStrategy,
		txtOversubtractMargin: floatToStr(lr.OversubtractMargin),
	}
	pngData := injectTextChunks(buf.Bytes(), meta)
	return os.WriteFile(path, pngData, 0644)
}

// injectTextChunks inserts tEXt metadata chunks after IHDR in a PNG stream.
func injectTextChunks(pngData []byte, meta map[string]string) []byte {
	const ihdrEnd = 33
	var textChunks []byte
	for k, v := range meta {
		textChunks = append(textChunks, buildTextChunk(k, v)...)
	}
	out := make([]byte, 0, len(pngData)+len(textChunks))
	out = append(out, pngData[:ihdrEnd]...)
	out = append(out, textChunks...)
	out = append(out, pngData[ihdrEnd:]...)
	return out
}

// buildTextChunk builds a PNG tEXt chunk.
func buildTextChunk(key, value string) []byte {
	payload := []byte(key + "\x00" + value)
	crcData := append([]byte("tEXt"), payload...)
	chunk := make([]byte, 4+4+len(payload)+4)
	binary.BigEndian.PutUint32(chunk[0:4], uint32(len(payload)))
	copy(chunk[4:8], "tEXt")
	copy(chunk[8:8+len(payload)], payload)
	crc := crc32IEEE(crcData)
	binary.BigEndian.PutUint32(chunk[8+len(payload):12+len(payload)], crc)
	return chunk
}

// crc32IEEE computes CRC-32/IEEE (used by PNG).
func crc32IEEE(data []byte) uint32 {
	var crc uint32 = 0xFFFFFFFF
	for _, b := range data {
		crc ^= uint32(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xEDB88320
			} else {
				crc >>= 1
			}
		}
	}
	return crc ^ 0xFFFFFFFF
}

// LoadWatermarkPNG loads a self-contained .watermark.png file.
func LoadWatermarkPNG(path string) (*LearnResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("watermark: read %s: %w", path, err)
	}
	meta := parseTextChunks(data)
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("watermark: decode alpha map PNG: %w", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	alphaData := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			// RGBA() always returns 16-bit values [0, 65535].
			// For 8-bit PNG: r = Y*257 (Go scales 8→16), so r/65535 = Y/255
			// For 16-bit PNG: r = Y directly, so r/65535 gives full precision
			alphaData[y*w+x] = float64(r) / 65535.0
		}
	}
	am := &AlphaMap{Width: w, Height: h, Data: alphaData}
	lr := &LearnResult{
		Name:               meta[txtName],
		AlphaMap:           am,
		NativeWidth:        parseInt(meta[txtNativeWidth], 0),
		MarginXFrac:        parseFloat(meta[txtMarginXFrac], 0),
		MarginYFrac:        parseFloat(meta[txtMarginYFrac], 0),
		DetectThreshold:    parseFloat(meta[txtDetectThreshold], 0.30),
		RemoveStrategy:     meta[txtRemoveStrategy],
		OversubtractMargin: parseFloat(meta[txtOversubtractMargin], 0),
	}
	if lr.RemoveStrategy == "" {
		lr.RemoveStrategy = "alpha_blend"
	}
	return lr, nil
}

// parseTextChunks extracts all tEXt chunk key-value pairs from a PNG file.
func parseTextChunks(data []byte) map[string]string {
	meta := make(map[string]string)
	if len(data) < 33 {
		return meta
	}
	pos := 33
	for pos+12 <= len(data) {
		if pos+4 > len(data) {
			break
		}
		chunkLen := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		pos += 4
		if pos+4 > len(data) {
			break
		}
		chunkType := string(data[pos : pos+4])
		pos += 4
		if chunkType == "IEND" {
			break
		}
		if chunkType == "tEXt" && chunkLen > 0 && pos+chunkLen <= len(data) {
			payload := data[pos : pos+chunkLen]
			nullIdx := -1
			for i, b := range payload {
				if b == 0 {
					nullIdx = i
					break
				}
			}
			if nullIdx > 0 && nullIdx < len(payload)-1 {
				meta[string(payload[:nullIdx])] = string(payload[nullIdx+1:])
			}
		}
		pos += chunkLen + 4
	}
	return meta
}

// RegisterWatermarkPNG loads a .watermark.png file and registers it.
func RegisterWatermarkPNG(path string) error {
	lr, err := LoadWatermarkPNG(path)
	if err != nil {
		return err
	}
	RegisterLearnResult(lr)
	return nil
}

// RegisterLearnResult creates a runtime Config from a LearnResult.
// It auto-selects the detection strategy based on alpha map shape:
//
//   - Square-like (aspect ratio < 2:1, e.g. banana/Gemini sparkle):
//     no PositionResolver → detectWatermark uses direct NCC matching
//     (same path as the built-in Gemini config).
//
//   - Rectangular (aspect ratio ≥ 2:1, e.g. doubao/baidu/zhipu text badges):
//     PositionResolver set → detectWatermark uses binary mask + NCC path.
//
// Pre-defined alpha gain profiles for different watermark types.
// The removal loop tries each gain in order and picks the one with
// the lowest NCC residual (best removal).  Lower gains are safer
// (less over-subtraction), higher gains remove more aggressively.
//
// SparkleAlphaGains: 0.6 → 0.8 → 1.0 → 1.3
//
//	For Gemini sparkle (diffuse glow, low contrast).  Gains > 1.3 risk
//	creating visible dark halos around the sparkle.  0.6-0.8 handle
//	the gentle alpha blend well; 1.0-1.3 are tried for images where
//	the watermark is particularly faint.
//
// TextAlphaGains: 1.0 → 1.5 → 2.0 → 2.5 → 3.0
//
//	For text badge watermarks (doubao/baidu/zhipu/jimeng).  The
//	two-capture method systematically underestimates text alpha
//	(learned max ≈ 0.25-0.35, true alpha ≈ 0.7-1.0) because the
//	text glyphs cover only a small fraction of the badge area while
//	the method averages over the entire badge.  Higher gains
//	compensate.  3.0 is the maximum — above this, even the NCC
//	residual increases from over-subtraction artifacts.
var (
	SparkleAlphaGains = []float64{0.6, 0.8, 1.0, 1.3}
	TextAlphaGains    = []float64{1.0, 1.5, 2.0, 2.5, 3.0}
)

func RegisterLearnResult(lr *LearnResult) {
	alphaW := lr.AlphaMap.Width
	alphaH := lr.AlphaMap.Height
	nativeW := lr.NativeWidth
	marginXFrac := lr.MarginXFrac
	marginYFrac := lr.MarginYFrac
	removeStrategy := RemoveAlphaBlend
	switch lr.RemoveStrategy {
	case "inpaint":
		removeStrategy = RemoveInpaint
	case "skip":
		removeStrategy = RemoveSkip
	}

	// Shared config fields
	cfg := Config{
		Type:               TypeUnknown,
		Name:               lr.Name,
		AlphaMap:           lr.AlphaMap,
		LogoColor:          [3]float64{255, 255, 255},
		DetectThreshold:    lr.DetectThreshold,
		RemoveStrategy:     removeStrategy,
		OversubtractMargin: lr.OversubtractMargin,
	}

	// Detect alpha map shape. Square-ish (< 2:1 aspect ratio) means
	// sparkle-like → use direct NCC path (no PositionResolver).
	// Rectangular (≥ 2:1) means text-like → use binary mask + NCC path.
	ratio := float64(maxInt(alphaW, alphaH)) / float64(minInt(alphaW, alphaH))
	// Per-type alpha gains: text watermarks need stronger removal because
	// the two-capture method systematically underestimates their alpha.
	// Assign alpha gain profile based on watermark shape
	if ratio < 2.0 {
		cfg.AlphaGains = SparkleAlphaGains
	} else {
		cfg.AlphaGains = TextAlphaGains
	}
	if ratio < 2.0 {
		// Sparkle-like: use direct NCC matching (no PositionResolver).
		// Position is computed from margins in detectWatermark's fallback.
		cfg.NativeWidth = nativeW
		cfg.DefaultSize = minInt(alphaW, alphaH)
		cfg.DefaultMarginX = int(math.Round(float64(nativeW) * marginXFrac))
		cfg.DefaultMarginY = int(math.Round(float64(nativeW) * marginYFrac))
	} else {
		// Text-like: use PositionResolver for binary mask + NCC path.
		cfg.NativeWidth = nativeW
		cfg.DefaultSize = minInt(alphaW, alphaH)
		cfg.DefaultMarginX = int(math.Round(float64(nativeW) * marginXFrac))
		cfg.DefaultMarginY = int(math.Round(float64(nativeW) * marginYFrac))
		cfg.MinSizeScale = 0.5
		cfg.MaxSizeScale = 2.0
		cfg.MarginRange = 16
		cfg.PositionResolver = func(w, h int) []Position {
			shorter := w
			if h < shorter {
				shorter = h
			}
			scale := float64(shorter) / float64(nativeW)
			szW := int(math.Round(float64(alphaW) * scale))
			szH := int(math.Round(float64(alphaH) * scale))
			if szW < 20 || szH < 10 {
				return nil
			}
			mx := int(math.Round(float64(w) * marginXFrac))
			my := int(math.Round(float64(h) * marginYFrac))
			x := w - mx - szW
			y := h - my - szH
			if x < 0 || y < 0 || x+szW > w || y+szH > h {
				return nil
			}
			return []Position{{X: x, Y: y, W: szW, H: szH}}
		}
	}

	Register(cfg)
}

// LoadWatermarkPNGsFromDir loads all .watermark.png files from a directory.
func LoadWatermarkPNGsFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var errs []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".watermark.png") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := RegisterWatermarkPNG(path); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", e.Name(), err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("watermark: load custom configs: %s", strings.Join(errs, "; "))
	}
	return nil
}

func floatToStr(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func parseFloat(s string, def float64) float64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}
