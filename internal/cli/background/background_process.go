package background

import (
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/martianzhang/aigc-cli/internal/background"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
)

func processOneFile(path, outDir string, opts background.Options, doReplace bool, repColor color.Color, repImg image.Image, runOnlineBG bool, bgProvider *provider.EffectiveProvider) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	f.Close()

	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	// Online-only mode: generate via image API, skip local RMBG entirely.
	if runOnlineBG {
		defaultPrompt := "Remove the background from this image. Keep the main subject exactly as is. Replace the background with a solid white color."
		if doReplace && repColor != nil {
			defaultPrompt = fmt.Sprintf("Replace the background of this image with color %s.", repColor)
		} else if doReplace && repImg != nil {
			defaultPrompt = "Replace the background of this image with a new background from the reference image."
		}
		onlineOut, err := generateOnlineBackground(path, bgProvider, defaultPrompt, bgPrompt)
		if err != nil {
			return fmt.Errorf("online generation failed: %w", err)
		}
		fmt.Printf("Saved: %s → %s\n", filepath.Base(path), filepath.Base(onlineOut))
		return nil
	}

	if bgMaskOnly {
		gray, result, err := background.MaskOnly(img, &opts, rmbgDetector)
		if err != nil {
			return err
		}
		outPath := filepath.Join(outDir, base+"_mask.png")
		if err := background.SavePNG(outPath, gray); err != nil {
			return err
		}
		fmt.Printf("Saved: %s → %s\n", filepath.Base(path), filepath.Base(outPath))
		if bgJSON {
			fmt.Printf("  %dx%d\n", result.Width, result.Height)
		}
		return nil
	}

	if bgRemove && !doReplace {
		outImg, result, err := background.RemoveBackground(img, &opts, rmbgDetector)
		if err != nil {
			return err
		}
		outPath := filepath.Join(outDir, base+"_removebg.png")
		if err := background.SavePNG(outPath, outImg); err != nil {
			return err
		}
		fmt.Printf("Saved: %s → %s\n", filepath.Base(path), filepath.Base(outPath))
		if bgJSON {
			fmt.Printf("  %dx%d\n", result.Width, result.Height)
		}
		if bgPreview {
			service.PreviewFile(outPath)
		}
		return nil
	}

	if doReplace {
		var result *background.Result
		var err error

		var outImg *image.NRGBA
		if repColor != nil {
			outImg, result, err = background.ReplaceColor(img, repColor, &opts, rmbgDetector)
		} else {
			outImg, result, err = background.ReplaceImage(img, repImg, &opts, rmbgDetector)
		}
		if err != nil {
			return err
		}

		outPath := filepath.Join(outDir, base+"_replaced.png")
		if err := background.SavePNG(outPath, outImg); err != nil {
			return err
		}
		fmt.Printf("Saved: %s → %s\n", filepath.Base(path), filepath.Base(outPath))
		if bgJSON {
			fmt.Printf("  %dx%d\n", result.Width, result.Height)
		}
		if bgPreview {
			service.PreviewFile(outPath)
		}
		return nil
	}

	return nil
}
