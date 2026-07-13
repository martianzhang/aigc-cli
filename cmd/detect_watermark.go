package cmd

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"

	"github.com/martianzhang/apimart-cli/internal/watermark"
)

// findSeedPairs finds all numbered seed pairs for a given name.
func findSeedPairs(dir, name string) ([][2]string, error) {
	var pairs [][2]string
	for i := 0; i <= 99; i++ {
		var suffixes []string
		if i == 0 {
			suffixes = []string{"", ".0"}
		} else {
			suffixes = []string{fmt.Sprintf(".%d", i)}
		}
		var found bool
		for _, suffix := range suffixes {
			b, errB := findSeedFile(dir, name+suffix, "black")
			g, errG := findSeedFile(dir, name+suffix, "gray")
			if errB == nil && errG == nil {
				pairs = append(pairs, [2]string{b, g})
				found = true
				break
			}
		}
		if !found {
			if i == 0 {
				continue
			}
			break
		}
	}
	if len(pairs) == 0 {
		return nil, fmt.Errorf("no seed pair found for %s", name)
	}
	return pairs, nil
}

// loadImage decodes an image from a file path.
func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return img, nil
}

// findSeedFile looks for {name}.{type}.png or {name}.{type}.jpg in dir.
func findSeedFile(dir, name, typ string) (string, error) {
	exts := []string{".png", ".jpg", ".jpeg"}
	for _, ext := range exts {
		path := filepath.Join(dir, name+"."+typ+ext)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no %s seed found: tried %s.%s.{png,jpg,jpeg}", typ, name, typ)
}

// runLearnWatermark learns a custom watermark from seed images in the watermark dir.
func runLearnWatermark(name string) error {
	dir := watermarkDir()
	pairs, err := findSeedPairs(dir, name)
	if err != nil {
		// Try importing a pre-made alpha map.
		alphaPath := filepath.Join(dir, name+".alpha.png")
		if _, statErr := os.Stat(alphaPath); statErr != nil {
			alphaPath = filepath.Join(dir, name+".png")
			if _, statErr := os.Stat(alphaPath); statErr != nil {
				return fmt.Errorf("no seeds or alpha map found for %s", name)
			}
		}
		alphaImg, loadErr := loadImage(alphaPath)
		if loadErr != nil {
			return fmt.Errorf("read %s: %w", alphaPath, loadErr)
		}
		b := alphaImg.Bounds()
		if b.Dx() == 0 || b.Dy() == 0 {
			return fmt.Errorf("alpha map %s has zero dimensions", alphaPath)
		}

		strategy := detectLearnStrategy
		if strategy == "" {
			strategy = "alpha_blend"
		}
		lr := &watermark.LearnResult{
			Name:               name,
			NativeWidth:        detectNativeWidth,
			MarginXFrac:        detectMarginXFrac,
			MarginYFrac:        detectMarginYFrac,
			DetectThreshold:    detectDetectThreshold,
			RemoveStrategy:     strategy,
			OversubtractMargin: 0,
		}
		alphaData := make([]float64, b.Dx()*b.Dy())
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				r, _, _, _ := alphaImg.At(x, y).RGBA()
				alphaData[y*b.Dx()+x] = float64(r) / 65535.0
			}
		}
		lr.AlphaMap = &watermark.AlphaMap{Width: b.Dx(), Height: b.Dy(), Data: alphaData}

		outputPath := filepath.Join(dir, name+".watermark.png")
		if err := watermark.SaveWatermarkPNG(outputPath, lr); err != nil {
			return fmt.Errorf("save watermark: %w", err)
		}
		fmt.Printf("\nWatermark config saved: %s\n", outputPath)
		fmt.Printf("  Name:             %s\n", lr.Name)
		fmt.Printf("  Alpha map:        %dx%d\n", lr.AlphaMap.Width, lr.AlphaMap.Height)
		fmt.Printf("  Native width:     %dpx\n", lr.NativeWidth)
		fmt.Printf("  Margin X frac:    %.6f\n", lr.MarginXFrac)
		fmt.Printf("  Margin Y frac:    %.6f\n", lr.MarginYFrac)
		fmt.Printf("  Detect threshold: %.2f\n", lr.DetectThreshold)
		fmt.Printf("  Remove strategy:  %s\n", lr.RemoveStrategy)
		fmt.Println()
		fmt.Printf("Use: aigc-cli detect <image> --remove-watermark --producer %s\n", name)
		return nil
	}

	type seedPair struct {
		black, gray image.Image
	}
	var seedPairs []seedPair
	var firstBounds struct{ w, h int }
	for i, paths := range pairs {
		blackImg, err := loadImage(paths[0])
		if err != nil {
			return fmt.Errorf("read %s: %w", paths[0], err)
		}
		grayImg, err := loadImage(paths[1])
		if err != nil {
			return fmt.Errorf("read %s: %w", paths[1], err)
		}
		b := blackImg.Bounds()
		g := grayImg.Bounds()
		if b.Dx() != g.Dx() || b.Dy() != g.Dy() {
			return fmt.Errorf("black and gray must have same dimensions: %s %dx%d vs %dx%d",
				paths[0], b.Dx(), b.Dy(), g.Dx(), g.Dy())
		}
		if i == 0 {
			firstBounds.w, firstBounds.h = b.Dx(), b.Dy()
		} else if b.Dx() != firstBounds.w || b.Dy() != firstBounds.h {
			fmt.Printf("  \u26a0  Pair %d dimensions differ (%dx%d vs %dx%d), skipping\n",
				i+1, b.Dx(), b.Dy(), firstBounds.w, firstBounds.h)
			continue
		}
		sample := 40
		var blackLum, grayLum, blackVar, grayVar float64
		var bN float64
		for y := 0; y < sample && y < b.Dy(); y++ {
			for x := 0; x < sample && x < b.Dx(); x++ {
				br, bg, bb, _ := blackImg.At(x, y).RGBA()
				gr, gg, gb, _ := grayImg.At(x, y).RGBA()
				bL := (float64(br>>8) + float64(bg>>8) + float64(bb>>8)) / 3.0
				gL := (float64(gr>>8) + float64(gg>>8) + float64(gb>>8)) / 3.0
				blackLum += bL
				grayLum += gL
				blackVar += bL * bL
				grayVar += gL * gL
				bN++
			}
		}
		if bN > 0 {
			blackLum /= bN
			grayLum /= bN
			blackVar = math.Sqrt(blackVar/bN - blackLum*blackLum)
			grayVar = math.Sqrt(grayVar/bN - grayLum*grayLum)
		}
		if blackLum > 80 {
			fmt.Printf("  Pair %d: black bg=%.1f (too bright), skipping\n", i+1, blackLum)
			continue
		}
		bgTag := "[GOOD]"
		if blackLum > 5 || grayLum < 120 || grayLum > 135 {
			bgTag = "[WARN]"
		}
		noiseTag := "[GOOD]"
		if blackVar > 3 || grayVar > 3 {
			noiseTag = "[WARN]"
		}
		bName := filepath.Base(paths[0])
		fmt.Printf("  %s: black=%.1f gray=%.1f noise=%.1f/%.1f bg=%s noise=%s\n",
			bName, blackLum, grayLum, blackVar, grayVar, bgTag, noiseTag)

		seedPairs = append(seedPairs, seedPair{blackImg, grayImg})
	}

	if len(seedPairs) == 0 {
		return fmt.Errorf("no valid seed pairs found for %s", name)
	}

	sq := watermark.AssessSeedQuality(seedPairs[0].black, seedPairs[0].gray)
	fmt.Println("Seed quality:")
	fmt.Printf("  Background:  black=%.1f (expect ~0), gray=%.1f (expect ~128)  %s\n",
		sq.BlackBG, sq.GrayBG, scoreTag(sq.BGScore))
	fmt.Printf("  Gradient:    gx=%.4f gy=%.4f (threshold |g|<0.01)  %s\n",
		sq.Gx, sq.Gy, scoreTag(sq.GradientScore))
	fmt.Printf("  Edge noise:  black=%.1f, gray=%.1f (good<5, warn<15)  %s\n",
		sq.BlackNoise, sq.GrayNoise, scoreTag(sq.NoiseScore))
	fmt.Printf("  WM signal:   max=%.0f (good>50)  %s\n",
		sq.SignalMax, scoreTag(sq.SignalScore))
	if len(seedPairs) > 1 {
		fmt.Printf("  Pairs:       %d (averaged)\n", len(seedPairs))
	}

	if sq.BGScore == watermark.SeedFail || sq.NoiseScore == watermark.SeedFail {
		fmt.Println("  \u26a0  Low quality seeds -- alpha map may be noisy. Try regenerating seed images.")
	}

	strategy := detectLearnStrategy
	if strategy == "" {
		strategy = "alpha_blend"
	}

	blacks := make([]image.Image, len(seedPairs))
	grays := make([]image.Image, len(seedPairs))
	for i, sp := range seedPairs {
		blacks[i] = sp.black
		grays[i] = sp.gray
	}
	lr := watermark.LearnWatermarkMulti(blacks, grays, name, strategy)

	outputPath := filepath.Join(dir, name+".watermark.png")
	if err := watermark.SaveWatermarkPNG(outputPath, lr); err != nil {
		return fmt.Errorf("save watermark: %w", err)
	}

	fmt.Printf("\nWatermark config saved: %s\n", outputPath)
	fmt.Printf("  Name:             %s\n", lr.Name)
	fmt.Printf("  Alpha map:        %dx%d\n", lr.AlphaMap.Width, lr.AlphaMap.Height)
	fmt.Printf("  Native width:     %dpx\n", lr.NativeWidth)
	fmt.Printf("  Margin X frac:    %.6f\n", lr.MarginXFrac)
	fmt.Printf("  Margin Y frac:    %.6f\n", lr.MarginYFrac)
	fmt.Printf("  Detect threshold: %.2f\n", lr.DetectThreshold)
	fmt.Printf("  Remove strategy:  %s\n", lr.RemoveStrategy)
	fmt.Println()
	fmt.Printf("Use: aigc-cli detect <image> --remove-watermark --producer %s\n", name)

	return nil
}
