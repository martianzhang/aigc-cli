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
		resSpans, resReview, err := detectPage(ctx, pipeline, page.lines, page.text)
		if err != nil {
			return Result{}, err
		}
		base := text.Len()
		if base > 0 {
			base++
		}
		for _, s := range resSpans {
			s.Start, s.End = s.Start+base, s.End+base
			spans = append(spans, s)
		}
		for _, s := range resReview {
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
		resSpans, resReview, err := detectPage(ctx, pipeline, page.lines, page.text)
		if err != nil {
			return Result{}, err
		}
		rects := rectsForSpans(page.lines, resSpans)
		for li := range page.lines {
			compact := compactOCR(page.lines[li].text)
			if compact == "" {
				continue
			}
			lineDoc := redactlib.NewDocument("line", compact)
			if lineRes, err := pipeline.Detect(ctx, lineDoc); err == nil && len(lineRes.Spans) > 0 {
				rects = append(rects, rectsForCompactSpans(page.lines[li], lineRes.Spans)...)
			}
		}
		canvas, _ := flattenImage(page.image)
		for _, rect := range mergeRects(rects) {
			draw.Draw(canvas, expandRect(rect), image.NewUniform(color.Black), image.Point{}, draw.Src)
		}
		jpgPath := filepath.Join(filepath.Dir(page.path), fmt.Sprintf("redacted-%04d.jpg", i+1))
		if _, err := imgcodec.EncodeToFile(canvas, jpgPath, "jpg", 85); err != nil {
			return Result{}, fmt.Errorf("encode PDF page: %w", err)
		}
		base := text.Len()
		if base > 0 {
			base++
		}
		for _, s := range resSpans {
			s.Start, s.End = s.Start+base, s.End+base
			spans = append(spans, s)
		}
		for _, s := range resReview {
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
	if pdf.IsEncrypted(input) {
		return nil, nil, fmt.Errorf("PDF is encrypted; remove the password or protection first")
	}
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
			page.lines, page.text = pdfLinesToBoxes(m, img.Bounds())
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

func pdfLinesToBoxes(page pdf.PageLines, bounds image.Rectangle) ([]ocrLineBox, string) {
	sx := float64(bounds.Dx()) / page.Width
	sy := float64(bounds.Dy()) / page.Height
	var text strings.Builder
	out := make([]ocrLineBox, 0, len(page.Lines))
	for _, line := range page.Lines {
		start := text.Len()
		if start > 0 {
			text.WriteByte('\n')
			start++
		}
		text.WriteString(line.Text)
		x0 := int((line.X0 - page.OriginX) * sx)
		x1 := int((line.X1 - page.OriginX) * sx)
		y0 := int((page.Height - (line.Y1 - page.OriginY)) * sy)
		y1 := int((page.Height - (line.Y0 - page.OriginY)) * sy)
		lineBox := ocrLineBox{
			start: start, end: text.Len(), text: line.Text,
			rect: image.Rect(x0, y0, x1, y1), precise: true,
		}
		for _, seg := range line.Segments {
			sx0 := int((seg.X0 - page.OriginX) * sx)
			sx1 := int((seg.X1 - page.OriginX) * sx)
			// A segment with several runes has no per-character positions in
			// the text layer; splitting it is approximate, so mark the line as
			// imprecise and let callers widen edge boxes slightly.
			if len([]rune(seg.Text)) > 1 {
				lineBox.precise = false
			}
			// Keep every segment on the line's common vertical band so boxes
			// are uniform.
			lineBox.segments = append(lineBox.segments, runeSegments(start+seg.Start, seg.Text,
				image.Rect(sx0, y0, sx1, y1))...)
		}
		out = append(out, lineBox)
	}
	return out, text.String()
}

func canvasBounds(img image.Image) image.Rectangle {
	b := img.Bounds()
	return image.Rect(0, 0, b.Dx(), b.Dy())
}
