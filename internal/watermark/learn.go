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

// SolveAlphaMap solves for the alpha map from black and gray seed images
// using the two-capture method.
func SolveAlphaMap(black, gray image.Image) *AlphaMap {
	b := black.Bounds()
	g := gray.Bounds()
	w, h := b.Dx(), b.Dy()
	if g.Dx() < w {
		w = g.Dx()
	}
	if g.Dy() < h {
		h = g.Dy()
	}
	data := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			br, bg, bb, _ := black.At(x, y).RGBA()
			gr, gg, gb, _ := gray.At(x, y).RGBA()
			bLum := (float64(br>>8) + float64(bg>>8) + float64(bb>>8)) / 3.0
			gLum := (float64(gr>>8) + float64(gg>>8) + float64(gb>>8)) / 3.0
			alphaB := bLum / 255.0
			alphaG := (gLum - 128.0) / 127.0
			alpha := (alphaB + alphaG) / 2.0
			if alpha < 0 {
				alpha = 0
			}
			if alpha > 1 {
				alpha = 1
			}
			data[y*w+x] = alpha
		}
	}
	return &AlphaMap{Width: w, Height: h, Data: data}
}

// TrimAlphaMap trims transparent edges from the alpha map.
func TrimAlphaMap(am *AlphaMap, threshold float64) (*AlphaMap, int, int, int, int) {
	if am.Width == 0 || am.Height == 0 {
		return am, 0, 0, 0, 0
	}
	minX, minY := am.Width, am.Height
	maxX, maxY := 0, 0
	for y := 0; y < am.Height; y++ {
		for x := 0; x < am.Width; x++ {
			if am.Data[y*am.Width+x] > threshold {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if minX > maxX || minY > maxY {
		return am, 0, 0, am.Width, am.Height
	}
	pad := 2
	minX = maxInt(0, minX-pad)
	minY = maxInt(0, minY-pad)
	maxX = minInt(am.Width-1, maxX+pad)
	maxY = minInt(am.Height-1, maxY+pad)
	cw := maxX - minX + 1
	ch := maxY - minY + 1
	data := make([]float64, cw*ch)
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			data[y*cw+x] = am.Data[(minY+y)*am.Width+(minX+x)]
		}
	}
	return &AlphaMap{Width: cw, Height: ch, Data: data}, minX, minY, cw, ch
}

// LearnResult holds the result of learning a watermark from seed images.
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

// LearnWatermark solves the alpha map from black+gray seed images and
// auto-derives all config parameters. Only name is required from the user.
func LearnWatermark(black, gray image.Image, name string) *LearnResult {
	b := black.Bounds()
	imgW := b.Dx()
	imgH := b.Dy()
	alpha := SolveAlphaMap(black, gray)
	trimmed, offsetX, offsetY, _, _ := TrimAlphaMap(alpha, 0.02)
	marginX := imgW - offsetX - trimmed.Width
	marginY := imgH - offsetY - trimmed.Height
	marginXFrac := float64(marginX) / float64(imgW)
	marginYFrac := float64(marginY) / float64(imgW)
	return &LearnResult{
		Name:               name,
		AlphaMap:           trimmed,
		NativeWidth:        imgW,
		MarginXFrac:        marginXFrac,
		MarginYFrac:        marginYFrac,
		DetectThreshold:    0.30,
		RemoveStrategy:     "alpha_blend",
		OversubtractMargin: 0,
	}
}

// SaveWatermarkPNG saves a learned watermark as a self-contained PNG file.
// The alpha map is stored as grayscale pixels; all metadata is embedded
// in PNG tEXt chunks.
func SaveWatermarkPNG(path string, lr *LearnResult) error {
	img := image.NewGray(image.Rect(0, 0, lr.AlphaMap.Width, lr.AlphaMap.Height))
	for y := 0; y < lr.AlphaMap.Height; y++ {
		for x := 0; x < lr.AlphaMap.Width; x++ {
			v := uint8(math.Round(lr.AlphaMap.Data[y*lr.AlphaMap.Width+x] * 255))
			img.SetGray(x, y, color.Gray{Y: v})
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
			alphaData[y*w+x] = float64(r>>8) / 255.0
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
	Register(Config{
		Type:               TypeUnknown,
		Name:               lr.Name,
		AlphaMap:           lr.AlphaMap,
		DefaultSize:        minInt(alphaW, alphaH),
		DefaultMarginX:     int(math.Round(float64(nativeW) * marginXFrac)),
		DefaultMarginY:     int(math.Round(float64(nativeW) * marginYFrac)),
		LogoColor:          [3]float64{255, 255, 255},
		DetectThreshold:    lr.DetectThreshold,
		MinSizeScale:       0.5,
		MaxSizeScale:       2.0,
		MarginRange:        16,
		RemoveStrategy:     removeStrategy,
		OversubtractMargin: lr.OversubtractMargin,
		PositionResolver: func(w, h int) []Position {
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
			my := int(math.Round(float64(w) * marginYFrac))
			x := w - mx - szW
			y := h - my - szH
			if x < 0 || y < 0 || x+szW > w || y+szH > h {
				return nil
			}
			return []Position{{X: x, Y: y, W: szW, H: szH}}
		},
	})
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
