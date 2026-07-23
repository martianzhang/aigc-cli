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
	Skipped bool   // True if compression was skipped (already small, or lossless)
	Reason  string // Why it was skipped (if Skipped=true)
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

	// --- Handle PNG lossless special case ---
	if outFmt == "png" {
		return handlePNGOutput(srcPath, dstPath, beforeSize)
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
		return binarySearchQuality(img, dstPath, outFmt, opts.TargetSize, beforeSize)
	}

	// Auto mode: try descending qualities, pick the highest that reduces file size.
	autoQualities := []int{92, 88, 85, 80, 75, 70, 60, 50, 40, 30}
	for _, q := range autoQualities {
		result, err := encodeWithQuality(img, dstPath, outFmt, q, beforeSize)
		if err != nil {
			return nil, err
		}
		if !result.Skipped {
			return result, nil
		}
	}
	return &CompressResult{
		DstPath: srcPath,
		Before:  beforeSize,
		After:   beforeSize,
		Skipped: true,
		Reason:  "cannot compress further without visible quality loss",
	}, nil
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
			DstPath: dstPath,
			Before:  beforeSize,
			After:   afterSize,
			Skipped: true,
			Reason:  fmt.Sprintf("compressed not smaller (%d ≥ %d bytes)", afterSize, beforeSize),
		}, nil
	}
	return &CompressResult{
		DstPath: dstPath,
		Before:  beforeSize,
		After:   afterSize,
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

// handlePNGOutput special-cases PNG output (lossless, skip compression).
func handlePNGOutput(srcPath, dstPath string, beforeSize int64) (*CompressResult, error) {
	// PNG re-encode: just copy since PNG is lossless
	if err := copyFile(srcPath, dstPath); err != nil {
		return nil, fmt.Errorf("copy png: %w", err)
	}
	afterInfo, err := os.Stat(dstPath)
	if err != nil {
		return nil, fmt.Errorf("stat copy: %w", err)
	}
	afterSize := afterInfo.Size()
	if afterSize >= beforeSize {
		os.Remove(dstPath)
		return &CompressResult{
			DstPath: srcPath,
			Before:  beforeSize,
			After:   afterSize,
			Skipped: true,
			Reason:  "PNG is lossless — cannot meaningfully compress",
		}, nil
	}
	return &CompressResult{
		DstPath: dstPath,
		Before:  beforeSize,
		After:   afterSize,
	}, nil
}

// copyFile copies src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
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
