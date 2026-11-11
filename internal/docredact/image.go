package docredact

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
	"github.com/martianzhang/aigc-cli/internal/ocr"
	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
)

type ocrLineBox struct {
	start, end int
	rect       image.Rectangle
	text       string
}

func scanImage(ctx context.Context, input string, pipeline *redactlib.Pipeline, opts Options) (Result, error) {
	img, err := loadImage(input)
	if err != nil {
		return Result{}, err
	}
	canvas, _ := flattenImage(img)
	_, text, err := ocrImage(canvas, opts)
	if err != nil {
		return Result{}, err
	}
	doc := redactlib.NewDocument(filepath.Base(input), text)
	res, err := pipeline.Detect(ctx, doc)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Spans: res.Spans, Review: res.Review}, nil
}

func applyImage(ctx context.Context, input, outPath string, pipeline *redactlib.Pipeline, opts Options) (Result, error) {
	img, err := loadImage(input)
	if err != nil {
		return Result{}, err
	}
	canvas, alpha := flattenImage(img)
	lines, text, err := ocrImage(canvas, opts)
	if err != nil {
		return Result{}, err
	}
	doc := redactlib.NewDocument(filepath.Base(input), text)
	res, err := pipeline.Detect(ctx, doc)
	if err != nil {
		return Result{}, err
	}
	black := lineSetForSpans(lines, res.Spans)
	// OCR often inserts spaces inside IDs/phones. Run the rules on a compacted
	// per-line text too; if it hits, black the whole line.
	for i := range lines {
		compact := compactOCR(lines[i].text)
		if compact == "" {
			continue
		}
		lineDoc := redactlib.NewDocument("line", compact)
		if lineRes, err := pipeline.Detect(ctx, lineDoc); err == nil && len(lineRes.Spans) > 0 {
			black[i] = true
		}
	}

	for i, line := range lines {
		if !black[i] {
			continue
		}
		rect := expandRect(line.rect, canvas.Bounds().Dx(), canvas.Bounds().Dy())
		draw.Draw(canvas, rect, image.NewUniform(color.Black), image.Point{}, draw.Src)
	}
	format := imageOutputFormat(input, alpha)
	if format == "jpg" && strings.ToLower(filepath.Ext(outPath)) != ".jpg" && strings.ToLower(filepath.Ext(outPath)) != ".jpeg" {
		outPath = strings.TrimSuffix(outPath, filepath.Ext(outPath)) + ".jpg"
	}
	if _, err := imgcodec.EncodeToFile(canvas, outPath, format, 85); err != nil {
		return Result{}, fmt.Errorf("encode image: %w", err)
	}
	return Result{OutputPath: outPath, Text: text, Spans: res.Spans, Review: res.Review}, nil
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}

func ocrImage(img image.Image, opts Options) ([]ocrLineBox, string, error) {
	engine := opts.OCR
	if engine == nil {
		e, err := ocr.NewDefaultEngine(opts.ModelsDir, "auto")
		if err != nil {
			return nil, "", err
		}
		engine = e
	}
	return ocrImageWithEngine(img, engine)
}

func ocrImageWithEngine(img image.Image, engine OCREngine) ([]ocrLineBox, string, error) {
	result, err := engine.Scan(img)
	if err != nil {
		return nil, "", err
	}
	var text strings.Builder
	var lines []ocrLineBox
	for _, page := range result.Pages {
		for _, line := range page.Lines {
			start := text.Len()
			if start > 0 {
				text.WriteByte('\n')
				start++
			}
			text.WriteString(line.Text)
			lines = append(lines, ocrLineBox{
				start: start, end: text.Len(), text: line.Text,
				rect: bboxRect(line.BBox),
			})
		}
	}
	return lines, text.String(), nil
}

func bboxRect(box [4][2]int) image.Rectangle {
	x0, y0 := box[0][0], box[0][1]
	x1, y1 := x0, y0
	for _, p := range box[1:] {
		if p[0] < x0 {
			x0 = p[0]
		}
		if p[0] > x1 {
			x1 = p[0]
		}
		if p[1] < y0 {
			y0 = p[1]
		}
		if p[1] > y1 {
			y1 = p[1]
		}
	}
	return image.Rect(x0, y0, x1, y1)
}

func lineSetForSpans(lines []ocrLineBox, spans []redactlib.Span) map[int]bool {
	out := map[int]bool{}
	for _, span := range spans {
		for i, line := range lines {
			if span.Start < line.end && span.End > line.start {
				out[i] = true
			}
		}
	}
	return out
}

func expandRect(r image.Rectangle, width, height int) image.Rectangle {
	padX := width * 3 / 1000
	padY := height * 3 / 1000
	if padX < 2 {
		padX = 2
	}
	if padY < 2 {
		padY = 2
	}
	return image.Rect(r.Min.X-padX, r.Min.Y-padY, r.Max.X+padX, r.Max.Y+padY)
}

func flattenImage(img image.Image) (*image.RGBA, bool) {
	alpha := hasAlpha(img)
	bounds := img.Bounds()
	canvas := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	if alpha {
		draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		draw.Draw(canvas, canvas.Bounds(), img, bounds.Min, draw.Over)
	} else {
		draw.Draw(canvas, canvas.Bounds(), img, bounds.Min, draw.Src)
	}
	return canvas, alpha
}

func hasAlpha(img image.Image) bool {
	switch im := img.(type) {
	case *image.NRGBA:
		for i := 3; i < len(im.Pix); i += 4 {
			if im.Pix[i] != 255 {
				return true
			}
		}
		return false
	case *image.RGBA:
		for i := 3; i < len(im.Pix); i += 4 {
			if im.Pix[i] != 255 {
				return true
			}
		}
		return false
	case *image.Paletted:
		for _, c := range im.Palette {
			if _, _, _, a := c.RGBA(); a != 0xffff {
				return true
			}
		}
		return false
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				return true
			}
		}
	}
	return false
}

func imageOutputFormat(input string, alpha bool) string {
	if alpha {
		return "jpg"
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(input), "."))
	switch ext {
	case "jpg", "jpeg":
		return "jpg"
	case "png":
		return "png"
	case "webp":
		return "webp"
	case "avif":
		return "avif"
	case "jxl":
		return "jxl"
	default:
		return "jpg"
	}
}

func compactOCR(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if r == ' ' || r == '\t' || r == '\u3000' {
			continue
		}
		b.WriteRune(r)
	}
	return redactlib.FoldWidth(b.String()).Text
}
