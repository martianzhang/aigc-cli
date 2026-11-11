package docredact

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
	"github.com/martianzhang/aigc-cli/internal/ocr"
	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
)

type ocrSegment struct {
	start, end int
	rect       image.Rectangle
}

type ocrTokenBox struct {
	start, end int
	rect       image.Rectangle
}

type ocrLineBox struct {
	start, end int
	rect       image.Rectangle
	text       string
	segments   []ocrSegment
	tokens     []ocrTokenBox
	precise    bool
}

func scanImage(ctx context.Context, input string, pipeline *redactlib.Pipeline, opts Options) (Result, error) {
	img, err := loadImage(input)
	if err != nil {
		return Result{}, err
	}
	canvas, _ := flattenImage(img)
	lines, text, err := ocrImage(canvas, opts)
	if err != nil {
		return Result{}, err
	}
	spans, review, err := detectPage(ctx, pipeline, lines, text)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Spans: spans, Review: review}, nil
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
	resSpans, resReview, err := detectPage(ctx, pipeline, lines, text)
	if err != nil {
		return Result{}, err
	}
	rects := rectsForSpans(lines, resSpans)
	// OCR often inserts spaces inside IDs/phones. Run the rules on a compacted
	// per-line text too; if it hits, black the line's text segments.
	for i := range lines {
		compact := compactOCR(lines[i].text)
		if compact == "" {
			continue
		}
		lineDoc := redactlib.NewDocument("line", compact)
		if lineRes, err := pipeline.Detect(ctx, lineDoc); err == nil && len(lineRes.Spans) > 0 {
			rects = append(rects, rectsForCompactSpans(lines[i], lineRes.Spans)...)
		}
	}
	for _, rect := range mergeRects(rects) {
		draw.Draw(canvas, expandRect(rect), image.NewUniform(color.Black), image.Point{}, draw.Src)
	}
	format := imageOutputFormat(input, alpha)
	if format == "jpg" && strings.ToLower(filepath.Ext(outPath)) != ".jpg" && strings.ToLower(filepath.Ext(outPath)) != ".jpeg" {
		outPath = strings.TrimSuffix(outPath, filepath.Ext(outPath)) + ".jpg"
	}
	if _, err := imgcodec.EncodeToFile(canvas, outPath, format, 85); err != nil {
		return Result{}, fmt.Errorf("encode image: %w", err)
	}
	return Result{OutputPath: outPath, Text: text, Spans: resSpans, Review: resReview}, nil
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
			rect := inkBounds(img, line.BBox)
			segs, precise := glyphSegments(img, line.Text, rect, start)
			lines = append(lines, ocrLineBox{
				start: start, end: text.Len(), text: line.Text,
				rect: rect, segments: segs, tokens: tokenBoxes(line.Text, segs), precise: precise,
			})
		}
	}
	return lines, text.String(), nil
}

// inkBounds trims the OCR line box to the actual non-background pixels. OCR
// detection often returns a full-width line box, while the text occupies only
// part of it; proportional character boxes would otherwise drift far right.
func inkBounds(img image.Image, box [4][2]int) image.Rectangle {
	r := bboxRect(box)
	b := img.Bounds().Intersect(r)
	if b.Empty() {
		return r
	}
	bg := color.NRGBAModel.Convert(img.At(b.Min.X, b.Min.Y)).(color.NRGBA)
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	found := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if absInt(int(c.R)-int(bg.R))+absInt(int(c.G)-int(bg.G))+absInt(int(c.B)-int(bg.B)) < 60 {
				continue
			}
			found = true
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if !found || maxX < minX || maxY < minY {
		return r
	}
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
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

func rectsForSpans(lines []ocrLineBox, spans []redactlib.Span) []image.Rectangle {
	var out []image.Rectangle
	for _, span := range spans {
		for _, line := range lines {
			if span.Start >= line.end || span.End <= line.start {
				continue
			}
			if len(line.tokens) > 0 {
				for _, tok := range line.tokens {
					if span.Start >= tok.end || span.End <= tok.start {
						continue
					}
					if span.Start <= tok.start && span.End >= tok.end {
						out = append(out, tok.rect)
						continue
					}
					// A span can cover only part of a token (for example an
					// address inside a longer line). Paint just the glyphs in
					// the intersection instead of the whole token.
					if line.precise {
						if r, ok := rectForRange(line.segments, span.Start, span.End); ok {
							out = append(out, r)
							continue
						}
					}
					out = append(out, tok.rect)
				}
				continue
			}
			if len(line.segments) == 0 {
				out = append(out, line.rect)
				continue
			}
			first, last := -1, -1
			for j, seg := range line.segments {
				if span.Start < seg.end && span.End > seg.start {
					if first < 0 {
						first = j
					}
					last = j
				}
			}
			if first < 0 {
				continue
			}
			// When the glyph split is unreliable, include one neighbour on each
			// side so short names/ethnicities cannot leak a character at the
			// edge. A precise split (including PDF text layers) is painted as-is.
			if !line.precise {
				if first > 0 {
					first--
				}
				if last+1 < len(line.segments) {
					last++
				}
			}
			rect := line.segments[first].rect
			for j := first + 1; j <= last; j++ {
				rect = rect.Union(line.segments[j].rect)
			}
			out = append(out, rect)
		}
	}
	return out
}

// rectForRange unions the glyph boxes overlapping a byte range.
func rectForRange(segs []ocrSegment, start, end int) (image.Rectangle, bool) {
	var r image.Rectangle
	found := false
	for _, seg := range segs {
		if seg.end <= start || seg.start >= end || seg.rect.Empty() {
			continue
		}
		if !found {
			r, found = seg.rect, true
			continue
		}
		r = r.Union(seg.rect)
	}
	return r, found
}

func runeSegments(base int, text string, rect image.Rectangle) []ocrSegment {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	out := make([]ocrSegment, 0, len(runes))
	byteOff := base
	weights := make([]float64, len(runes))
	total := 0.0
	for i, r := range runes {
		weights[i] = runeWidthWeight(r)
		total += weights[i]
	}
	if total <= 0 {
		total = float64(len(runes))
	}
	pos := 0.0
	for i, r := range runes {
		size := len(string(r))
		x0 := rect.Min.X + int(float64(rect.Dx())*(pos/total))
		pos += weights[i]
		x1 := rect.Min.X + int(float64(rect.Dx())*(pos/total))
		if i == len(runes)-1 {
			x1 = rect.Max.X
		}
		out = append(out, ocrSegment{
			start: byteOff, end: byteOff + size,
			rect: image.Rect(x0, rect.Min.Y, x1, rect.Max.Y),
		})
		byteOff += size
	}
	return out
}

var tokenLabels = []string{
	"统一社会信用代码", "公民身份号码", "身份证号码", "纳税人识别号",
	"委托诉讼代理人", "法定代表人", "建设工程规划许可证", "不动产权证书",
	"联系电话", "身份证号", "住所地", "开户行", "项目编号", "合同编号",
	"发票代码", "发票号码", "公民身份", "身份证", "纳税人", "负责人",
	"代理人", "审判长", "审判员", "书记员", "出生", "电话", "手机",
	"邮箱", "地址", "住址", "号码", "账号", "民族", "姓名", "原告",
	"被告", "第三人", "证人", "住",
}

func tokenBoxes(text string, segs []ocrSegment) []ocrTokenBox {
	runes := []rune(text)
	if len(runes) == 0 || len(segs) != len(runes) {
		return nil
	}
	skip := make([]int, len(runes))
	for _, label := range tokenLabels {
		lr := []rune(label)
		if len(lr) == 0 {
			continue
		}
		for i := 0; i+len(lr) <= len(runes); i++ {
			if skip[i] != 0 {
				continue
			}
			match := true
			for j := range lr {
				if runes[i+j] != lr[j] {
					match = false
					break
				}
			}
			if !match {
				continue
			}
			skip[i] = i + len(lr)
			i += len(lr) - 1
		}
	}
	var out []ocrTokenBox
	start := -1
	end := -1
	rect := image.Rectangle{}
	flush := func() {
		if start >= 0 && end > start {
			out = append(out, ocrTokenBox{start: start, end: end, rect: rect})
		}
		start, end, rect = -1, -1, image.Rectangle{}
	}
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if skip[i] > i {
			flush()
			i = skip[i] - 1
			continue
		}
		if tokenDelimiter(r) {
			flush()
			continue
		}
		if start < 0 {
			start = segs[i].start
			rect = segs[i].rect
		} else {
			rect = rect.Union(segs[i].rect)
		}
		end = segs[i].end
	}
	flush()
	return out
}

func tokenDelimiter(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case '，', ',', '。', '；', ';', '：', ':', '、', '（', '(', '）', ')',
		'【', '[', '】', ']', '《', '<', '》', '>', '！', '!', '？', '?':
		return true
	}
	return false
}

func runeWidthWeight(r rune) float64 {
	// CJK/full-width glyphs are roughly one em; ASCII letters/digits are
	// roughly half an em. This keeps mixed Chinese/ID lines aligned better
	// than an equal-width split.
	if r < 0x2E80 {
		if r == ' ' || r == '\t' {
			return 0.5
		}
		return 0.55
	}
	return 1.0
}

func mergeRects(rects []image.Rectangle) []image.Rectangle {
	if len(rects) < 2 {
		return rects
	}
	sort.Slice(rects, func(i, j int) bool {
		if rects[i].Min.Y != rects[j].Min.Y {
			return rects[i].Min.Y < rects[j].Min.Y
		}
		return rects[i].Min.X < rects[j].Min.X
	})
	out := []image.Rectangle{rects[0]}
	for _, r := range rects[1:] {
		last := &out[len(out)-1]
		if r.Min.Y <= last.Max.Y && overlapsVertically(*last, r) &&
			r.Min.X <= last.Max.X+2 && r.Min.X >= last.Min.X {
			if r.Max.X > last.Max.X {
				last.Max.X = r.Max.X
			}
			if r.Max.Y > last.Max.Y {
				last.Max.Y = r.Max.Y
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

// overlapsVertically requires at least half of the smaller box to overlap so
// boxes on adjacent text lines are never chained into one tall rectangle.
func overlapsVertically(a, b image.Rectangle) bool {
	overlap := a.Max.Y - b.Min.Y
	if b.Max.Y-a.Min.Y < overlap {
		overlap = b.Max.Y - a.Min.Y
	}
	if overlap <= 0 {
		return false
	}
	minHeight := a.Dy()
	if b.Dy() < minHeight {
		minHeight = b.Dy()
	}
	return overlap*2 >= minHeight
}

func expandRect(r image.Rectangle) image.Rectangle {
	// Scale the safety margin with the text size. A fixed few pixels bleeds
	// into neighbouring punctuation on small images and is too small on
	// high-DPI scans.
	pad := r.Dy() / 10
	if pad < 2 {
		pad = 2
	}
	if pad > 6 {
		pad = 6
	}
	return image.Rect(r.Min.X-pad, r.Min.Y-pad, r.Max.X+pad, r.Max.Y+pad)
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
	compact, _ := compactLine(s)
	return compact
}

// compactLine folds full-width text and removes whitespace while retaining a
// byte-offset map back to the original line. It lets compacted OCR text (where
// phones/IDs may contain spaces) be mapped to the original character boxes.
func compactLine(s string) (string, []int) {
	folded := redactlib.FoldWidth(s)
	var b strings.Builder
	offsets := make([]int, 0, len(folded.Text)+1)
	for i, r := range folded.Text {
		if r == ' ' || r == '\t' || r == '\n' || r == '\u3000' {
			continue
		}
		start := b.Len()
		b.WriteRune(r)
		for j := start; j < b.Len(); j++ {
			offsets = append(offsets, folded.ToOrig[i])
		}
	}
	offsets = append(offsets, len(s))
	return b.String(), offsets
}

func rectsForCompactSpans(line ocrLineBox, spans []redactlib.Span) []image.Rectangle {
	_, offsets := compactLine(line.text)
	var out []image.Rectangle
	for _, span := range spans {
		start, end := span.Start, span.End
		if start < 0 {
			start = 0
		}
		if end > len(offsets)-1 {
			end = len(offsets) - 1
		}
		if start >= end || start >= len(offsets) || end >= len(offsets) {
			continue
		}
		local := redactlib.Span{
			Start: line.start + offsets[start],
			End:   line.start + offsets[end],
		}
		out = append(out, rectsForSpans([]ocrLineBox{line}, []redactlib.Span{local})...)
	}
	return out
}
