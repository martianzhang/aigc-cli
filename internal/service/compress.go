package service

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chai2010/webp"
	"github.com/soniakeys/quant/median"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // decoder registration
)

// CompressOptions defines how to compress an image.
type CompressOptions struct {
	TargetSize int64  // Target file size in bytes (0 = not set)
	Quality    int    // Fixed quality 1-100 (0 = auto-detect via binary search)
	Format     string // Output format: "jpg", "png", "webp" ("" = keep original)
}

// CompressResult holds the outcome of a compression.
type CompressResult struct {
	DstPath string // Output file path
	Before  int64  // Original file size in bytes
	After   int64  // Compressed file size in bytes
	Skipped bool   // True if compression was skipped
	Reason  string // Why it was skipped (if Skipped=true)
	Format  string // Output format: jpg, webp, png
	Quality int    // Quality used (0 for palette-based PNG)
}

// CompressImage reads an image from srcPath, re-encodes it according to opts,
// and writes the result alongside the original with a _compress suffix.
//
// TargetSize > 0 → binary-search the highest quality that fits ≤ TargetSize
// Quality > 0    → encode at that fixed quality (ignores TargetSize)
// Format == ""   → keep the original file extension
//
// Returns a CompressResult describing what happened.
func CompressImage(srcPath string, opts *CompressOptions) (*CompressResult, error) {
	// --- Open and decode source ---
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat source: %w", err)
	}
	beforeSize := srcInfo.Size()

	img, _, err := image.Decode(srcFile)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	// --- Determine output format ---
	ext := strings.ToLower(filepath.Ext(srcPath))
	outFmt := opts.Format
	if outFmt == "" {
		// Keep original format
		switch ext {
		case ".jpg", ".jpeg", ".jfif":
			outFmt = "jpg"
		case ".png":
			outFmt = "png"
		case ".webp":
			outFmt = "webp"
		default:
			outFmt = "jpg"
		}
	}

	// --- Build output path ---
	origBase := strings.TrimSuffix(srcPath, ext)
	dstPath := origBase + "_compress." + outFmt
	if strings.EqualFold(filepath.Ext(dstPath), ext) {
		// same extension, just insert suffix before extension
		dstPath = origBase + "_compress" + ext
	}

	// --- Quality / target-size / auto logic ---
	if opts.Quality > 0 {
		return encodeWithQuality(img, dstPath, outFmt, opts.Quality, beforeSize)
	}

	if opts.TargetSize > 0 {
		if beforeSize <= opts.TargetSize {
			return &CompressResult{
				DstPath: srcPath,
				Before:  beforeSize,
				After:   beforeSize,
				Skipped: true,
				Reason:  fmt.Sprintf("already ≤ target (%d ≤ %d bytes)", beforeSize, opts.TargetSize),
			}, nil
		}
		if outFmt == "png" {
			return handlePNGOutput(img, srcPath, dstPath, opts, beforeSize)
		}
		return binarySearchQuality(img, dstPath, outFmt, opts.TargetSize, beforeSize)
	}

	// Auto mode: user-specified format (opts.Format) or Squoosh-style JPEG q75
	return autoCompress(img, srcPath, opts.Format, opts, beforeSize)
}

// encodeWithQuality encodes the image at the given quality and returns the result.
func encodeWithQuality(img image.Image, dstPath, outFmt string, quality int, beforeSize int64) (*CompressResult, error) {
	afterSize, err := encodeImage(img, dstPath, outFmt, quality)
	if err != nil {
		return nil, err
	}
	if afterSize >= beforeSize {
		os.Remove(dstPath)
		return &CompressResult{
			DstPath: dstPath, Before: beforeSize, After: afterSize,
			Skipped: true, Format: outFmt, Quality: quality,
			Reason: fmt.Sprintf("compressed not smaller (%d ≥ %d bytes)", afterSize, beforeSize),
		}, nil
	}
	return &CompressResult{
		DstPath: dstPath, Before: beforeSize, After: afterSize,
		Format: outFmt, Quality: quality,
	}, nil
}

func binarySearchQuality(img image.Image, dstPath, outFmt string, targetSize, beforeSize int64) (*CompressResult, error) {
	origBounds := img.Bounds()
	origW := origBounds.Dx()
	origH := origBounds.Dy()

	scales := []float64{1.0, 0.75, 0.5, 0.35, 0.2, 0.1}

	for _, scale := range scales {
		workImg := img
		if scale < 1.0 {
			newW := int(float64(origW) * scale)
			newH := int(float64(origH) * scale)
			if newW < 1 {
				newW = 1
			}
			if newH < 1 {
				newH = 1
			}
			dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
			draw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
			workImg = dst
		}

		lo, hi := 5, 95
		bestQuality := -1

		for lo <= hi {
			mid := (lo + hi) / 2
			size, err := tryEncode(workImg, dstPath, outFmt, mid)
			if err != nil {
				return nil, err
			}

			if size <= targetSize {
				bestQuality = mid
				lo = mid + 1
			} else {
				hi = mid - 1
			}
		}

		if bestQuality > 0 {
			// Re-encode at best quality (file was overwritten by later iterations)
			os.Remove(dstPath)
			finalSize, err := tryEncode(workImg, dstPath, outFmt, bestQuality)
			if err != nil {
				return nil, err
			}
			if finalSize >= beforeSize {
				os.Remove(dstPath)
				return nil, fmt.Errorf("compressed not smaller (%d ≥ %d bytes)", finalSize, beforeSize)
			}
			return &CompressResult{
				DstPath: dstPath,
				Before:  beforeSize,
				After:   finalSize,
			}, nil
		}
	}

	smallW := int(float64(origW)*0.1 + 0.5)
	smallH := int(float64(origH)*0.1 + 0.5)
	if smallW < 1 {
		smallW = 1
	}
	if smallH < 1 {
		smallH = 1
	}
	smallImg := image.NewRGBA(image.Rect(0, 0, smallW, smallH))
	draw.BiLinear.Scale(smallImg, smallImg.Bounds(), img, img.Bounds(), draw.Over, nil)
	finalSize, err := tryEncode(smallImg, dstPath, outFmt, 5)
	if err != nil {
		return nil, err
	}
	if finalSize >= beforeSize {
		os.Remove(dstPath)
		return nil, fmt.Errorf("cannot meet target (smallest: %d bytes at %dx%d q=5, target %d bytes)",
			finalSize, smallW, smallH, targetSize)
	}
	return &CompressResult{
		DstPath: dstPath,
		Before:  beforeSize,
		After:   finalSize,
	}, nil
}

// tryEncode writes the image to dstPath and returns the resulting file size.
// The caller is responsible for cleaning up the file if needed.
func tryEncode(img image.Image, dstPath, outFmt string, quality int) (int64, error) {
	f, err := os.Create(dstPath)
	if err != nil {
		return 0, fmt.Errorf("create temp: %w", err)
	}
	defer f.Close()

	if err := encodeImageTo(img, f, outFmt, quality); err != nil {
		f.Close()
		os.Remove(dstPath)
		return 0, err
	}
	f.Close()

	info, err := os.Stat(dstPath)
	if err != nil {
		return 0, fmt.Errorf("stat output: %w", err)
	}
	return info.Size(), nil
}

// encodeImage encodes the image to dstPath at the given quality and returns the file size.
func encodeImage(img image.Image, dstPath, outFmt string, quality int) (int64, error) {
	return tryEncode(img, dstPath, outFmt, quality)
}

// encodeImageTo writes the encoded image to w.
func encodeImageTo(img image.Image, w *os.File, outFmt string, quality int) error {
	switch outFmt {
	case "jpg", "jpeg":
		return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
	case "webp":
		return webp.Encode(w, img, &webp.Options{Quality: float32(quality), Lossless: false})
	case "png":
		return png.Encode(w, img)
	default:
		return fmt.Errorf("unsupported output format: %s", outFmt)
	}
}

// handlePNGOutput compresses PNG via color quantization (palette reduction).
// Uses Floyd-Steinberg dithering for visual quality.
// Palette sizes tried: 256 → 128 → 64 → 32 → 16 → 8 → 4 → 2.
func handlePNGOutput(img image.Image, srcPath, dstPath string, opts *CompressOptions, beforeSize int64) (*CompressResult, error) {
	// Fixed quality: map 1-100 to palette size (1→2, 100→256)
	if opts.Quality > 0 {
		nColors := 2 + (opts.Quality * 254 / 100)
		return encodePalettedPNG(img, dstPath, nColors, beforeSize)
	}

	// Target size: try decreasing palette sizes
	if opts.TargetSize > 0 {
		if beforeSize <= opts.TargetSize {
			return &CompressResult{
				DstPath: srcPath, Before: beforeSize, After: beforeSize,
				Skipped: true, Reason: fmt.Sprintf("already ≤ target (%d ≤ %d bytes)", beforeSize, opts.TargetSize),
			}, nil
		}
		paletteSizes := []int{256, 192, 160, 128, 96, 64, 48, 32, 24, 16, 12, 8, 6, 4, 2}
		for _, n := range paletteSizes {
			result, err := encodePalettedPNG(img, dstPath, n, beforeSize)
			if err != nil {
				return nil, err
			}
			if !result.Skipped && result.After <= opts.TargetSize {
				return result, nil
			}
			if result.Skipped {
				// Smaller than original but bigger than target — keep going
				continue
			}
			// File exists, check next palette
		}
		// All palette sizes exceeded target — try resize + paletted
		bounds := img.Bounds()
		w, h := bounds.Dx(), bounds.Dy()
		scales := []float64{0.75, 0.5, 0.35, 0.2, 0.1}
		for _, scale := range scales {
			nw, nh := int(float64(w)*scale), int(float64(h)*scale)
			if nw < 1 {
				nw = 1
			}
			if nh < 1 {
				nh = 1
			}
			smallImg := image.NewRGBA(image.Rect(0, 0, nw, nh))
			draw.BiLinear.Scale(smallImg, smallImg.Bounds(), img, img.Bounds(), draw.Over, nil)
			for _, n := range paletteSizes {
				result, err := encodePalettedPNG(smallImg, dstPath, n, beforeSize)
				if err != nil {
					return nil, err
				}
				if !result.Skipped && result.After <= opts.TargetSize {
					return result, nil
				}
			}
		}
		return nil, fmt.Errorf("cannot meet target %d bytes for PNG", opts.TargetSize)
	}

	paletteSizes := []int{256, 192, 160, 128, 96, 64, 48, 32, 24, 16, 12, 8, 6, 4, 2}
	best := &CompressResult{
		DstPath: srcPath, Before: beforeSize, After: beforeSize,
		Skipped: true, Reason: "cannot compress further",
	}
	for _, n := range paletteSizes {
		result, err := encodePalettedPNG(img, dstPath, n, beforeSize)
		if err != nil {
			return nil, err
		}
		if !result.Skipped {
			return result, nil
		}
	}
	return best, nil
}

// encodePalettedPNG quantizes the image to nColors using median cut + FloydSteinberg dithering.
func encodePalettedPNG(img image.Image, dstPath string, nColors int, beforeSize int64) (*CompressResult, error) {
	bounds := img.Bounds()
	mq := median.Quantizer(nColors)
	quantPal := mq.Palette(img)
	pal := quantPal.ColorPalette()
	dst := image.NewPaletted(bounds, pal)
	draw.FloydSteinberg.Draw(dst, bounds, img, image.Point{})
	enc := &png.Encoder{CompressionLevel: png.BestCompression}
	f, err := os.Create(dstPath)
	if err != nil {
		return nil, fmt.Errorf("create: %w", err)
	}
	defer f.Close()
	if err := enc.Encode(f, dst); err != nil {
		f.Close()
		os.Remove(dstPath)
		return nil, fmt.Errorf("encode paletted png: %w", err)
	}
	f.Close()
	info, err := os.Stat(dstPath)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	afterSize := info.Size()
	if afterSize >= beforeSize {
		os.Remove(dstPath)
		return &CompressResult{
			DstPath: dstPath, Before: beforeSize, After: afterSize,
			Skipped: true, Format: "png",
			Reason: fmt.Sprintf("paletted %d-color PNG not smaller (%d ≥ %d bytes)", nColors, afterSize, beforeSize),
		}, nil
	}
	return &CompressResult{DstPath: dstPath, Before: beforeSize, After: afterSize, Format: "png"}, nil
}

// autoCompress auto-selects compression based on output format.
// No format specified → Squoosh-style JPEG q75.
// Format specified → auto quality for that format.
func autoCompress(img image.Image, srcPath, outFmt string, opts *CompressOptions, beforeSize int64) (*CompressResult, error) {
	origBase := strings.TrimSuffix(srcPath, filepath.Ext(srcPath))
	switch outFmt {
	case "jpg", "jpeg":
		dst := origBase + "_compress.jpg"
		return encodeWithQuality(img, dst, outFmt, 75, beforeSize)
	case "webp":
		dst := origBase + "_compress.webp"
		return encodeWithQuality(img, dst, outFmt, 75, beforeSize)
	case "png":
		dst := origBase + "_compress.png"
		for _, n := range []int{256, 192, 160, 128, 96, 64, 48, 32, 24, 16} {
			r, err := encodePalettedPNG(img, dst, n, beforeSize)
			if err != nil {
				return nil, err
			}
			if !r.Skipped {
				return r, nil
			}
		}
		return &CompressResult{
			DstPath: srcPath, Before: beforeSize, After: beforeSize,
			Skipped: true, Reason: "cannot compress further",
		}, nil
	default:
		dst := origBase + "_compress.jpg"
		return encodeWithQuality(img, dst, "jpg", 75, beforeSize)
	}
}

// ParseCompressOption parses a --compress flag value.
// Supported formats:
//   - "800KB" or "800kb"  → TargetSize = 800 * 1024
//   - "2MB" or "2mb"      → TargetSize = 2 * 1024 * 1024
//   - "85%"               → Quality = 85
//
// Returns targetSize, quality, error.
func ParseCompressOption(val string) (targetSize int64, quality int, err error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0, 0, fmt.Errorf("empty compress value")
	}

	if strings.EqualFold(val, "auto") {
		return 0, 0, nil
	}

	// Percentage → fixed quality
	if strings.HasSuffix(val, "%") {
		pctStr := strings.TrimSuffix(val, "%")
		q, err := strconv.Atoi(pctStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid quality percentage %q", val)
		}
		if q < 1 || q > 100 {
			return 0, 0, fmt.Errorf("quality must be 1-100, got %d", q)
		}
		return 0, q, nil
	}

	// Size string
	upper := strings.ToUpper(val)
	var multiplier int64 = 1
	var numStr string

	switch {
	case strings.HasSuffix(upper, "KB"):
		multiplier = 1024
		numStr = strings.TrimSuffix(upper, "KB")
	case strings.HasSuffix(upper, "MB"):
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(upper, "MB")
	case strings.HasSuffix(upper, "K"):
		multiplier = 1024
		numStr = strings.TrimSuffix(upper, "K")
	case strings.HasSuffix(upper, "M"):
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(upper, "M")
	case strings.HasSuffix(upper, "B"):
		multiplier = 1
		numStr = strings.TrimSuffix(upper, "B")
	default:
		numStr = upper
	}

	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid compress size %q", val)
	}
	if num <= 0 {
		return 0, 0, fmt.Errorf("compress size must be positive, got %q", val)
	}

	return num * multiplier, 0, nil
}
