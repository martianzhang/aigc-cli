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
	"github.com/martianzhang/aigc-cli/internal/pdf"
	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
)

type pdfPageData struct {
	meta  pdf.PageLines
	image image.Image
	path  string
	lines []ocrLineBox
	text  string
}

func scanPDF(ctx context.Context, input string, pipeline *redactlib.Pipeline, opts Options) (Result, error) {
	pages, cleanup, err := extractPDFPages(opts, input)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()
	var text strings.Builder
	var spans, review []redactlib.Span
	for _, page := range pages {
		doc := redactlib.NewDocument(fmt.Sprintf("page-%d", page.meta.Page), page.text)
		res, err := pipeline.Detect(ctx, doc)
		if err != nil {
			return Result{}, err
		}
		base := text.Len()
		if base > 0 {
			base++
		}
		for _, s := range res.Spans {
			s.Start, s.End = s.Start+base, s.End+base
			spans = append(spans, s)
		}
		for _, s := range res.Review {
			s.Start, s.End = s.Start+base, s.End+base
			review = append(review, s)
		}
		if text.Len() > 0 {
			text.WriteByte('\n')
		}
		text.WriteString(page.text)
	}
	return Result{Text: text.String(), Spans: spans, Review: review}, nil
}

func applyPDF(ctx context.Context, input, outPath string, pipeline *redactlib.Pipeline, opts Options) (Result, error) {
	pages, cleanup, err := extractPDFPages(opts, input)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()

	var text strings.Builder
	var spans, review []redactlib.Span
	for i, page := range pages {
		doc := redactlib.NewDocument(fmt.Sprintf("page-%d", page.meta.Page), page.text)
		res, err := pipeline.Detect(ctx, doc)
		if err != nil {
			return Result{}, err
		}
		black := lineSetForSpans(page.lines, res.Spans)
		for li := range page.lines {
			compact := compactOCR(page.lines[li].text)
			if compact == "" {
				continue
			}
			lineDoc := redactlib.NewDocument("line", compact)
			if lineRes, err := pipeline.Detect(ctx, lineDoc); err == nil && len(lineRes.Spans) > 0 {
				black[li] = true
			}
		}
		canvas, _ := flattenImage(page.image)
		for li, line := range page.lines {
			if !black[li] {
				continue
			}
			rect := expandRect(line.rect, canvas.Bounds().Dx(), canvas.Bounds().Dy())
			draw.Draw(canvas, rect, image.NewUniform(color.Black), image.Point{}, draw.Src)
		}
		jpgPath := filepath.Join(filepath.Dir(page.path), fmt.Sprintf("redacted-%04d.jpg", i+1))
		if _, err := imgcodec.EncodeToFile(canvas, jpgPath, "jpg", 85); err != nil {
			return Result{}, fmt.Errorf("encode PDF page: %w", err)
		}
		base := text.Len()
		if base > 0 {
			base++
		}
		for _, s := range res.Spans {
			s.Start, s.End = s.Start+base, s.End+base
			spans = append(spans, s)
		}
		for _, s := range res.Review {
			s.Start, s.End = s.Start+base, s.End+base
			review = append(review, s)
		}
		if text.Len() > 0 {
			text.WriteByte('\n')
		}
		text.WriteString(page.text)
	}

	imgPages := make([]pdfImagePage, 0, len(pages))
	for i, page := range pages {
		jpgPath := filepath.Join(filepath.Dir(page.path), fmt.Sprintf("redacted-%04d.jpg", i+1))
		data, err := os.ReadFile(jpgPath)
		if err != nil {
			return Result{}, err
		}
		b := canvasBounds(page.image)
		imgPages = append(imgPages, pdfImagePage{
			JPEG: data, WidthPx: b.Dx(), HeightPx: b.Dy(),
			WidthPt: page.meta.Width, HeightPt: page.meta.Height,
		})
	}
	if err := writeImagePDF(outPath, imgPages); err != nil {
		return Result{}, err
	}
	return Result{OutputPath: outPath, Text: text.String(), Spans: spans, Review: review}, nil
}

func extractPDFPages(opts Options, input string) ([]pdfPageData, func(), error) {
	meta, err := pdf.ExtractPageLines(input)
	if err != nil {
		return nil, nil, fmt.Errorf("read PDF: %w", err)
	}
	tmp, err := os.MkdirTemp("", "aigc-redact-pdf-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	images, err := pdf.RenderToImages(input, tmp, opts.DPI)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	if len(images) != len(meta) {
		cleanup()
		return nil, nil, fmt.Errorf("PDF page render mismatch: %d pages, %d images", len(meta), len(images))
	}
	engine := opts.OCR
	pages := make([]pdfPageData, 0, len(meta))
	for i, m := range meta {
		img, err := loadImage(images[i])
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		page := pdfPageData{meta: m, image: img, path: images[i]}
		if m.Rotation == 0 && m.Width > 0 && m.Height > 0 && len(m.Lines) > 0 {
			page.lines = pdfLinesToBoxes(m, img.Bounds())
			page.text = pdfText(page.lines)
		} else {
			if engine == nil {
				e, err := ocr.NewDefaultEngine(opts.ModelsDir, "auto")
				if err != nil {
					cleanup()
					return nil, nil, err
				}
				engine = e
			}
			lines, text, err := ocrImageWithEngine(img, engine)
			if err != nil {
				cleanup()
				return nil, nil, err
			}
			page.lines, page.text = lines, text
		}
		pages = append(pages, page)
	}
	return pages, cleanup, nil
}

func pdfLinesToBoxes(page pdf.PageLines, bounds image.Rectangle) []ocrLineBox {
	sx := float64(bounds.Dx()) / page.Width
	sy := float64(bounds.Dy()) / page.Height
	out := make([]ocrLineBox, 0, len(page.Lines))
	for _, line := range page.Lines {
		x0 := int((line.X0 - page.OriginX) * sx)
		x1 := int((line.X1 - page.OriginX) * sx)
		y0 := int((page.Height - (line.Y1 - page.OriginY)) * sy)
		y1 := int((page.Height - (line.Y0 - page.OriginY)) * sy)
		out = append(out, ocrLineBox{
			start: 0, end: 0, text: line.Text,
			rect: image.Rect(x0, y0, x1, y1),
		})
	}
	// Rebuild text/offsets now that line order is known.
	var text strings.Builder
	for i := range out {
		out[i].start = text.Len()
		if text.Len() > 0 {
			text.WriteByte('\n')
			out[i].start++
		}
		text.WriteString(out[i].text)
		out[i].end = text.Len()
	}
	return out
}

func pdfText(lines []ocrLineBox) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line.text)
	}
	return b.String()
}

func canvasBounds(img image.Image) image.Rectangle {
	b := img.Bounds()
	return image.Rect(0, 0, b.Dx(), b.Dy())
}
