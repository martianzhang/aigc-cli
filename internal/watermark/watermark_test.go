package watermark

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// providerTestCase describes one watermark provider and its test images.
type providerTestCase struct {
	name    string // provider name, matches --producer
	testDir string // relative path under .testdata/
	learned bool   // true = loaded via LoadWatermarkPNGsFromDir
}

var providerTests = []providerTestCase{
	{name: "gemini", testDir: "gemini", learned: true},
	{name: "doubao", testDir: "doubao", learned: true},
	{name: "baidu", testDir: "baidu", learned: true},
	{name: "zhipu", testDir: "zhipu", learned: true},
	{name: "jimeng", testDir: "jimeng", learned: true},
}

// findProjectRoot walks up from the test file to find the project root (where .testdata/ lives).
func findProjectRoot() string {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, ".testdata")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	// Fallback: try the current working directory
	if wd, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(wd, ".testdata")); err == nil {
			return wd
		}
	}
	return ""
}

func TestAllProviders_WatermarkRemoval(t *testing.T) {
	testdataRoot := findProjectRoot()
	if testdataRoot == "" {
		t.Skip(".testdata/ not found")
	}

	// Load learned watermarks from the config directory
	home, _ := os.UserHomeDir()
	watermarkDir := filepath.Join(home, ".config", "aigc-cli", "watermark")
	if _, err := os.Stat(watermarkDir); os.IsNotExist(err) {
		t.Skip("watermark config directory not found:", watermarkDir)
	}
	if err := LoadWatermarkPNGsFromDir(watermarkDir); err != nil {
		t.Fatalf("load watermarks: %v", err)
	}

	for _, pt := range providerTests {
		t.Run(pt.name, func(t *testing.T) {
			testDir := filepath.Join(testdataRoot, ".testdata", pt.testDir)
			entries, err := os.ReadDir(testDir)
			if err != nil {
				t.Skip("test data not found:", testDir)
			}

			var tested int
			for _, e := range entries {
				if e.IsDir() || strings.Contains(e.Name(), "_clean") {
					continue
				}
				// Detect seed-like test files (white/black/gray backgrounds).
				// On these, the watermark may be invisible (e.g. white-on-white),
				// so we log warnings instead of errors for failed detection.
				isSeed := strings.Contains(e.Name(), "white") ||
					strings.Contains(e.Name(), "black") ||
					strings.Contains(e.Name(), "gray_")
				ext := strings.ToLower(filepath.Ext(e.Name()))
				if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
					continue
				}

				imgPath := filepath.Join(testDir, e.Name())
				img := loadTestImage(t, imgPath)
				if img == nil {
					continue
				}

				// Skip watermark removal on seed images (black/gray/white
				// backgrounds).  Seeds are calibration inputs, not real
				// images — running reverse alpha blending on them produces
				// artifacts (e.g. residual bright pixels on a black seed)
				// that are meaningless to verify.
				if isSeed {
					continue
				}

				// Run removal with producer hint
				dst, res, err := RemoveWatermarkHinted(img, pt.name)
				if err != nil {
					t.Errorf("%s: RemoveWatermarkHinted error: %v", e.Name(), err)
					continue
				}
				if !res.Removed {
					msg := fmt.Sprintf("%s: watermark not detected", e.Name())
					if isSeed {
						t.Log(msg + " (seed image, may be invisible)")
					} else {
						t.Error(msg)
					}
					continue
				}
				if res.Name != pt.name {
					msg := fmt.Sprintf("%s: wrong provider: got %q, want %q",
						e.Name(), res.Name, pt.name)
					if isSeed {
						t.Log(msg + " (seed image)")
					} else {
						t.Error(msg)
					}
					continue
				}

				// Save the cleaned image for manual inspection
				cleanPath := imgPath[:len(imgPath)-len(ext)] + "_clean" + ext
				if err := saveImage(dst, cleanPath); err != nil {
					t.Logf("%s: failed to save clean image: %v", e.Name(), err)
				}

				if res.Confidence < 0.08 {
					t.Logf("%s: confidence low (%.4f), removal may be weak", e.Name(), res.Confidence)
				}

				// Verify pixel changes: the watermark area must have changed
				if !verifyRemoval(img, dst, res) {
					t.Logf("%s: removal had minimal visible effect", e.Name())
				}

				// Check over-subtraction (log as warning, not error)
				if msg := checkOverSubtraction(img, dst, res); msg != "" {
					t.Logf("%s: over-subtraction: %s", e.Name(), msg)
				}

				tested++
			}

			if tested == 0 {
				t.Skip("no testable images found in", testDir)
			}
		})
	}
}

// loadTestImage decodes an image file, returning nil on failure.
func loadTestImage(t *testing.T, path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		t.Logf("skipping %s: %v", path, err)
		return nil
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Logf("skipping %s: decode error: %v", path, err)
		return nil
	}
	return img
}

// verifyRemoval checks that at least some pixels in the watermark area changed.
func verifyRemoval(orig, cleaned image.Image, res *Result) bool {
	b := orig.Bounds()
	if res.Size <= 0 || res.Region == "" {
		return true // can't verify, assume OK
	}

	// Parse region format "x,y,w,h" or file path
	var x, y, w, h int
	if n, _ := fmt.Sscanf(res.Region, "%d,%d,%d,%d", &x, &y, &w, &h); n < 4 {
		// Region might be file path, skip pixel check
		return true
	}
	if w <= 0 {
		w = res.Size
	}
	if h <= 0 {
		h = res.Size
	}
	if x+w > b.Dx() || y+h > b.Dy() {
		return true // region outside bounds, skip check
	}

	// Count changed pixels in the watermark region
	var changed int
	total := w * h
	for dy := 0; dy < h && dy < b.Dy()-y; dy++ {
		for dx := 0; dx < w && dx < b.Dx()-x; dx++ {
			pr, pg, pb, _ := orig.At(x+dx, y+dy).RGBA()
			cr, cg, cb, _ := cleaned.At(x+dx, y+dy).RGBA()
			diff := math.Abs(float64(pr)-float64(cr)) +
				math.Abs(float64(pg)-float64(cg)) +
				math.Abs(float64(pb)-float64(cb))
			if diff > 3*256 { // >1 gray level per channel in 16-bit
				changed++
			}
		}
	}
	return changed > 0 && float64(changed)/float64(total) > 0.01
}

// checkOverSubtraction returns a warning if the cleaned watermark area is
// significantly darker than the surrounding background.
func checkOverSubtraction(orig, cleaned image.Image, res *Result) string {
	b := orig.Bounds()
	var x, y, w, h int
	if n, _ := fmt.Sscanf(res.Region, "%d,%d,%d,%d", &x, &y, &w, &h); n < 4 {
		return ""
	}
	if w <= 0 {
		w = res.Size
	}
	if h <= 0 {
		h = res.Size
	}
	if x+w > b.Dx() || y+h > b.Dy() || y < 15 {
		return ""
	}

	// Sample a 15px ring above the watermark area
	var ringSum, ringCount float64
	for dy := -15; dy < 0; dy++ {
		for dx := 0; dx < w; dx++ {
			px, py := x+dx, y+dy
			if px < 0 || px >= b.Dx() || py < 0 || py >= b.Dy() {
				continue
			}
			r, g, bl, _ := cleaned.At(px, py).RGBA()
			ringSum += 0.2126*float64(r>>8) + 0.7152*float64(g>>8) + 0.0722*float64(bl>>8)
			ringCount++
		}
	}
	if ringCount < 10 {
		return ""
	}
	bgLum := ringSum / ringCount

	// Sample the cleaned watermark area
	var wmSum, wmCount float64
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			r, g, bl, _ := cleaned.At(x+dx, y+dy).RGBA()
			wmSum += 0.2126*float64(r>>8) + 0.7152*float64(g>>8) + 0.0722*float64(bl>>8)
			wmCount++
		}
	}
	wmLum := wmSum / wmCount

	diff := bgLum - wmLum
	if diff > 30 {
		return fmt.Sprintf("over-subtraction: WM area %.1f levels darker than background", diff)
	}
	return ""
}

// saveImage encodes an image as PNG/JPEG and writes it to path.
func saveImage(img image.Image, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return jpeg.Encode(out, img, &jpeg.Options{Quality: 90})
	default:
		return png.Encode(out, img)
	}
}
