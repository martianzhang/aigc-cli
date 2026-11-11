package docredact

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
)

type docxNode struct {
	innerStart, innerEnd int
	start, end           int
	decoded              string
}

type docxPart struct {
	name  string
	data  []byte
	nodes []docxNode
	text  string
}

var docxTokenRe = regexp.MustCompile(`(?s)<w:t\b[^>]*>(.*?)</w:t>|</w:p>`)

func scanDOCX(ctx context.Context, input string, pipeline *redactlib.Pipeline, opts Options) (Result, error) {
	parts, zr, err := extractDOCX(input)
	if err != nil {
		return Result{}, err
	}
	defer zr.Close()
	text, _ := combineDOCXText(parts)
	doc := redactlib.NewDocument(filepath.Base(input), text)
	res, err := pipeline.Detect(ctx, doc)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Spans: res.Spans, Review: res.Review}, nil
}

func applyDOCX(ctx context.Context, input, outPath string, pipeline *redactlib.Pipeline, opts Options) (Result, error) {
	parts, zr, err := extractDOCX(input)
	if err != nil {
		return Result{}, err
	}
	zr.Close()
	text, offsets := combineDOCXText(parts)
	doc := redactlib.NewDocument(filepath.Base(input), text)
	res, err := pipeline.Detect(ctx, doc)
	if err != nil {
		return Result{}, err
	}
	style := opts.Style
	if style == "" {
		style = "placeholder"
	}
	replacement, mapping, err := docxReplacements(text, res.Spans, style, opts)
	if err != nil {
		return Result{}, err
	}
	changed, err := rewriteDOCX(input, parts, offsets, res.Spans, replacement)
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(outPath, changed, 0o600); err != nil {
		return Result{}, err
	}
	return Result{OutputPath: outPath, Text: text, Spans: res.Spans, Review: res.Review, Mapping: mapping}, nil
}

// isOLEFile reports whether path is an OLE compound file. Encrypted OOXML and
// legacy .doc files share this container instead of a ZIP archive.
func isOLEFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var head [8]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return false
	}
	return bytes.Equal(head[:], []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
}

func extractDOCX(path string) ([]*docxPart, *zip.ReadCloser, error) {
	if isOLEFile(path) {
		return nil, nil, fmt.Errorf("DOCX is encrypted or a legacy .doc file; remove the password or convert it to .docx first")
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open docx: %w", err)
	}
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "word/embeddings/") || strings.HasPrefix(f.Name, "word/activeX/") {
			zr.Close()
			return nil, nil, fmt.Errorf("DOCX contains embedded objects or ActiveX controls; remove them or export to PDF before redacting")
		}
	}
	var parts []*docxPart
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if !docxPartName(name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			zr.Close()
			return nil, nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			zr.Close()
			return nil, nil, err
		}
		parts = append(parts, parseDOCXPart(name, data))
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].name < parts[j].name })
	return parts, zr, nil
}

func docxPartName(name string) bool {
	switch name {
	case "word/document.xml", "word/footnotes.xml", "word/endnotes.xml",
		"word/comments.xml", "word/glossary/document.xml":
		return true
	}
	base := filepath.Base(name)
	return strings.HasPrefix(base, "header") && strings.HasSuffix(base, ".xml") ||
		strings.HasPrefix(base, "footer") && strings.HasSuffix(base, ".xml")
}

func parseDOCXPart(name string, data []byte) *docxPart {
	p := &docxPart{name: name, data: data}
	var text strings.Builder
	for _, m := range docxTokenRe.FindAllSubmatchIndex(data, -1) {
		if m[0] == m[1] {
			continue
		}
		if m[2] >= 0 {
			inner := string(data[m[2]:m[3]])
			decoded := html.UnescapeString(inner)
			start := text.Len()
			text.WriteString(decoded)
			p.nodes = append(p.nodes, docxNode{
				innerStart: m[2], innerEnd: m[3],
				start: start, end: start + len(decoded), decoded: decoded,
			})
			continue
		}
		// Paragraph boundary.
		if text.Len() > 0 && !strings.HasSuffix(text.String(), "\n") {
			text.WriteByte('\n')
		}
	}
	p.text = text.String()
	return p
}

func combineDOCXText(parts []*docxPart) (string, []int) {
	var b strings.Builder
	offsets := make([]int, len(parts))
	for i, part := range parts {
		if i > 0 {
			b.WriteString("\n\n")
		}
		// The offset must point at the part's first byte, after the separator.
		offsets[i] = b.Len()
		b.WriteString(part.text)
	}
	return b.String(), offsets
}

type spanReplacement struct {
	offset int // span start in part text
	end    int
	text   string
}

func docxReplacements(text string, spans []redactlib.Span, style string, opts Options) ([]spanReplacement, *redactlib.Mapping, error) {
	switch style {
	case "placeholder", "mask", "black", "hash":
	default:
		return nil, nil, fmt.Errorf("unknown style %q (want placeholder, mask, black or hash)", style)
	}
	if opts.Masks == nil {
		opts.Masks, _ = redactlib.NewMaskResolver(nil)
	}
	var mapping *redactlib.Mapping
	placeholderByValue := map[string]string{}
	if style == "placeholder" || style == "hash" {
		doc := redactlib.NewDocument("docx", text)
		_, m, err := redactlib.Apply(doc, spans, redactlib.ApplyOptions{
			Style: style, Placeholder: opts.Placeholder, Masks: opts.Masks,
			SkipDate: opts.SkipDate, SkipAmount: opts.SkipAmount, Aliases: opts.Aliases,
		})
		if err != nil {
			return nil, nil, err
		}
		mapping = m
		for _, e := range m.Entities {
			placeholderByValue[e.Value] = e.Placeholder
			for _, alias := range e.Aliases {
				placeholderByValue[alias] = e.Placeholder
			}
		}
	}
	out := make([]spanReplacement, 0, len(spans))
	for _, span := range spans {
		replacement := placeholderByValue[span.Text]
		if replacement == "" {
			switch style {
			case "mask":
				replacement = redactlib.MaskValue(span.Text, span.Type, opts.Masks.Policy(span.Type))
			case "black":
				replacement = redactlib.BlackValue(span.Text, span.Type)
			case "hash":
				// Apply already assigned a hash placeholder.
				replacement = redactlib.MaskValue(span.Text, span.Type, opts.Masks.Policy(span.Type))
			default:
				replacement = span.Text
			}
		}
		out = append(out, spanReplacement{offset: span.Start, end: span.End, text: replacement})
	}
	return out, mapping, nil
}

func rewriteDOCX(input string, parts []*docxPart, offsets []int, spans []redactlib.Span, replacements []spanReplacement) ([]byte, error) {
	zr, err := zip.OpenReader(input)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	changed := map[string][]byte{}
	for i, part := range parts {
		base := offsets[i]
		local := make([]spanReplacement, 0)
		for j, span := range spans {
			if span.Start >= base && span.End <= base+len(part.text) {
				r := replacements[j]
				local = append(local, spanReplacement{offset: span.Start - base, end: span.End - base, text: r.text})
			}
		}
		if len(local) == 0 {
			continue
		}
		newXML := applyToDocxPart(part, local)
		changed[part.name] = newXML
	}

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			zw.Close()
			return nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			zw.Close()
			return nil, err
		}
		if d, ok := changed[f.Name]; ok {
			data = d
		}
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name: f.Name, Method: f.Method, Modified: f.Modified,
			ExternalAttrs: f.ExternalAttrs, Comment: f.Comment,
		})
		if err != nil {
			zw.Close()
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func applyToDocxPart(part *docxPart, spans []spanReplacement) []byte {
	sort.Slice(spans, func(i, j int) bool { return spans[i].offset < spans[j].offset })
	inserted := map[int]bool{}
	newTexts := make([]string, len(part.nodes))
	for ni, node := range part.nodes {
		var b strings.Builder
		cursor := node.start
		for si, span := range spans {
			if span.end <= node.start || span.offset >= node.end {
				continue
			}
			start := span.offset
			if start < node.start {
				start = node.start
			}
			if start > cursor {
				b.WriteString(part.text[cursor:start])
			}
			if !inserted[si] {
				b.WriteString(span.text)
				inserted[si] = true
			}
			cursor = span.end
			if cursor > node.end {
				cursor = node.end
			}
		}
		if cursor < node.end {
			b.WriteString(part.text[cursor:node.end])
		}
		newTexts[ni] = html.EscapeString(b.String())
	}
	var out bytes.Buffer
	cursor := 0
	for ni, node := range part.nodes {
		out.Write(part.data[cursor:node.innerStart])
		out.WriteString(newTexts[ni])
		cursor = node.innerEnd
	}
	out.Write(part.data[cursor:])
	return out.Bytes()
}

// RestoreDOCX replaces placeholders in DOCX text nodes using a mapping.
func RestoreDOCX(input, outPath string, m *redactlib.Mapping) error {
	parts, zr, err := extractDOCX(input)
	if err != nil {
		return err
	}
	defer zr.Close()
	values := map[string]string{}
	for _, e := range m.Entities {
		values[e.Placeholder] = e.Value
	}
	changed := map[string][]byte{}
	for _, part := range parts {
		data := part.data
		for ph, val := range values {
			data = bytes.ReplaceAll(data, []byte(html.EscapeString(ph)), []byte(html.EscapeString(val)))
		}
		if !bytes.Equal(data, part.data) {
			changed[part.name] = data
		}
	}
	if len(changed) == 0 {
		return fmt.Errorf("no placeholders found in %s", filepath.Base(input))
	}
	return rewriteZip(input, outPath, changed)
}

func rewriteZip(input, outPath string, changed map[string][]byte) error {
	zr, err := zip.OpenReader(input)
	if err != nil {
		return err
	}
	defer zr.Close()
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			zw.Close()
			return err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			zw.Close()
			return err
		}
		if d, ok := changed[f.Name]; ok {
			data = d
		}
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name: f.Name, Method: f.Method, Modified: f.Modified,
			ExternalAttrs: f.ExternalAttrs, Comment: f.Comment,
		})
		if err != nil {
			zw.Close()
			return err
		}
		if _, err := w.Write(data); err != nil {
			zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(outPath, out.Bytes(), 0o600)
}
