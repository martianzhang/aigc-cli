package chat

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/martianzhang/aigc-cli/internal/background"
	clibg "github.com/martianzhang/aigc-cli/internal/cli/background"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/watermark"
)

// executeRemoveBackground runs RMBG AI background removal and returns a text summary.
func executeRemoveBackground(argsJSON string) string {
	var a struct {
		FilePath     string `json:"file_path"`
		OutputPath   string `json:"output_path"`
		ReplaceColor string `json:"replace_color"`
		Autocrop     bool   `json:"autocrop"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if a.FilePath == "" {
		return "Error: file_path is required"
	}

	det, err := clibg.EnsureDetector()
	if err != nil {
		return fmt.Sprintf("Error: RMBG not available — run 'aigc-cli background init' first: %v", err)
	}

	f, err := os.Open(a.FilePath)
	if err != nil {
		return fmt.Sprintf("Error: cannot open file: %v", err)
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return fmt.Sprintf("Error: cannot decode image: %v", err)
	}

	opts := background.Defaults()
	if a.Autocrop {
		opts.Autocrop = true
	}

	var outPath string
	if a.OutputPath != "" {
		outPath = a.OutputPath
	} else {
		ext := filepath.Ext(a.FilePath)
		base := strings.TrimSuffix(filepath.Base(a.FilePath), ext)
		outPath = base + "_removebg.png"
	}

	if a.ReplaceColor != "" {
		c, err := service.ParseHexColor(a.ReplaceColor)
		if err != nil {
			return fmt.Sprintf("Error: invalid replace_color: %v", err)
		}
		outImg, _, err := background.ReplaceColor(img, c, &opts, det)
		if err != nil {
			return fmt.Sprintf("Error: background removal failed: %v", err)
		}
		if err := background.SavePNG(outPath, outImg); err != nil {
			return fmt.Sprintf("Error: save failed: %v", err)
		}
		return fmt.Sprintf("Background replaced. Output: %s", outPath)
	}

	outImg, _, err := background.RemoveBackground(img, &opts, det)
	if err != nil {
		return fmt.Sprintf("Error: background removal failed: %v", err)
	}
	if err := background.SavePNG(outPath, outImg); err != nil {
		return fmt.Sprintf("Error: save failed: %v", err)
	}
	return fmt.Sprintf("Background removed. Output: %s", outPath)
}

// executeRemoveWatermark runs visible-AI-watermark removal and returns a text summary.
func executeRemoveWatermark(argsJSON string) string {
	var a watermarkArgs
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if a.FilePath == "" {
		return "Error: file_path is required"
	}

	// Auto-load custom watermarks from config directory
	watermark.LoadWatermarkPNGsFromDir(options.WatermarkDir())

	res, err := watermark.RemoveFileHinted(a.FilePath, a.OutputPath, a.Producer)
	if err != nil {
		return fmt.Sprintf("Error: remove failed: %v", err)
	}
	if !res.Removed {
		return "No visible AI watermark detected/removed."
	}
	out := a.OutputPath
	if out == "" {
		ext := filepath.Ext(a.FilePath)
		out = strings.TrimSuffix(a.FilePath, ext) + "_clean" + ext
	}
	return fmt.Sprintf("Successfully removed watermark (engine: %s). Output: %s", res.Name, out)
}

// executeAddWatermark runs visible-AI-watermark addition and returns a text summary.
func executeAddWatermark(argsJSON string) string {
	var a watermarkArgs
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if a.FilePath == "" {
		return "Error: file_path is required"
	}
	if a.Producer == "" {
		return "Error: producer is required (known: gemini, or custom text)"
	}
	res, err := watermark.AddWatermarkFile(a.FilePath, a.OutputPath, a.Producer)
	if err != nil {
		return fmt.Sprintf("Error: add failed: %v", err)
	}
	out := a.OutputPath
	if out == "" {
		ext := filepath.Ext(a.FilePath)
		out = strings.TrimSuffix(a.FilePath, ext) + "_watermarked.png"
	}
	return fmt.Sprintf("Successfully added watermark (engine: %s). Output: %s", res.Name, out)
}

// --- Midjourney agent tools ---

// captionImageArgs is the JSON structure for caption_image tool arguments.
type captionImageArgs struct {
	FilePath string `json:"file_path"`
	Caption  string `json:"caption,omitempty"`
}

// executeCaptionImage reads or writes the caption of an image file.
func executeCaptionImage(argsJSON string) string {
	var args captionImageArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.FilePath == "" {
		return "Error: file_path is required"
	}

	// Resolve relative path
	path := args.FilePath
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Sprintf("Error: invalid path: %v", err)
		}
		path = abs
	}

	// Write mode
	if args.Caption != "" || args.Caption == "" && len(argsJSON) > 0 {
		// Check if "caption" key was actually present in the JSON
		var raw map[string]json.RawMessage
		if json.Unmarshal([]byte(argsJSON), &raw) == nil {
			if _, hasCaption := raw["caption"]; hasCaption {
				if err := service.WriteDescription(path, args.Caption); err != nil {
					return fmt.Sprintf("Error writing caption: %v", err)
				}
				if args.Caption == "" {
					return fmt.Sprintf("Caption cleared for %s", filepath.Base(path))
				}
				return fmt.Sprintf("Caption set: %s", args.Caption)
			}
		}
	}

	// Read mode
	current, err := service.ReadDescription(path)
	if err != nil {
		return fmt.Sprintf("Error reading caption: %v", err)
	}
	if current == "" {
		return fmt.Sprintf("No caption set for %s", filepath.Base(path))
	}
	return fmt.Sprintf("Caption: %s", current)
}
