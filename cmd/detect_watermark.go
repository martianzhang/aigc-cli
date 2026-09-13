package cmd

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
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
