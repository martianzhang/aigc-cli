package preview

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// markdownImageRef is one image reference in a Markdown document, located by
// its byte range in the source so the surrounding text can be rendered around it.
type markdownImageRef struct {
	start  int
	end    int
	alt    string
	target string
}

// Inline image support is injected so tests can exercise the image path without
// a terminal that speaks Kitty/Sixel.
var (
	inlinePreviewAvailable = service.InlinePreviewAvailable
	inlinePreviewFile      = service.InlinePreviewFile
)

var (
	// ![alt](target "title") and ![alt](<target with spaces>)
	markdownImageInlineRe = regexp.MustCompile(`!\[([^\]\r\n]*)\]\(\s*(?:<([^>\r\n]*)>|([^\s)]+))(?:\s+(?:"[^"\r\n]*"|'[^'\r\n]*'))?\s*\)`)
	// ![alt][id] reference-style images.
	markdownImageRefRe = regexp.MustCompile(`!\[([^\]\r\n]*)\]\[([^\]\r\n]*)\]`)
	// [id]: target
	markdownImageDefRe = regexp.MustCompile(`(?m)^\s{0,3}\[([^\]\r\n]+)\]:\s*<?([^\s>]+)>?`)
	// <img src="…" alt="…"> tags.
	htmlImgTagRe  = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	htmlSrcAttrRe = regexp.MustCompile(`(?is)\bsrc\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	htmlAltAttrRe = regexp.MustCompile(`(?is)\balt\s*=\s*(?:"([^"]*)"|'([^']*)')`)
)

// markdownImageRefs returns every image reference in document order, covering
// `![alt](target)`, `![alt][id]` and raw `<img src="...">` tags. Fenced code
// blocks, inline code spans, escaped markers and images wrapped in links are
// left alone.
func markdownImageRefs(src string) []markdownImageRef {
	defs := markdownImageDefinitions(src)
	var refs []markdownImageRef
	inFence := false
	offset := 0
	for _, line := range strings.SplitAfter(src, "\n") {
		body := strings.TrimRight(line, "\n")
		if isFenceLine(body) {
			inFence = !inFence
			offset += len(line)
			continue
		}
		if !inFence {
			refs = append(refs, lineImageRefs(body, offset, defs)...)
		}
		offset += len(line)
	}
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].start < refs[j].start })
	return dropOverlaps(refs)
}

// markdownImageDefinitions collects `[id]: target` definitions for
// reference-style images, ignoring fenced code blocks.
func markdownImageDefinitions(src string) map[string]string {
	defs := make(map[string]string)
	inFence := false
	for _, line := range strings.Split(src, "\n") {
		if isFenceLine(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, match := range markdownImageDefRe.FindAllStringSubmatch(line, -1) {
			defs[normalizeImageID(match[1])] = match[2]
		}
	}
	return defs
}

// lineImageRefs finds image references on one line. offset is the byte position
// of the line in the document.
func lineImageRefs(line string, offset int, defs map[string]string) []markdownImageRef {
	var refs []markdownImageRef

	inline := markdownImageInlineRe.FindAllStringSubmatchIndex(line, -1)
	for _, match := range inline {
		if escapedOrCode(line, match[0]) {
			continue
		}
		target := groupAt(line, match, 2)
		if target == "" {
			target = groupAt(line, match, 3)
		}
		refs = append(refs, markdownImageRef{
			start:  offset + match[0],
			end:    offset + match[1],
			alt:    groupAt(line, match, 1),
			target: target,
		})
	}

	for _, match := range markdownImageRefRe.FindAllStringSubmatchIndex(line, -1) {
		if escapedOrCode(line, match[0]) || overlapsAny(refs, offset+match[0], offset+match[1]) {
			continue
		}
		id := groupAt(line, match, 2)
		if id == "" {
			continue // collapsed/shortcut references are not supported
		}
		target, ok := defs[normalizeImageID(id)]
		if !ok {
			continue
		}
		refs = append(refs, markdownImageRef{
			start:  offset + match[0],
			end:    offset + match[1],
			alt:    groupAt(line, match, 1),
			target: target,
		})
	}

	for _, tag := range htmlImgTagRe.FindAllStringIndex(line, -1) {
		if overlapsAny(refs, offset+tag[0], offset+tag[1]) {
			continue
		}
		match := line[tag[0]:tag[1]]
		target, ok := firstRegexGroup(htmlSrcAttrRe, match)
		if !ok || target == "" {
			continue
		}
		alt, _ := firstRegexGroup(htmlAltAttrRe, match)
		refs = append(refs, markdownImageRef{
			start:  offset + tag[0],
			end:    offset + tag[1],
			alt:    strings.TrimSpace(alt),
			target: strings.TrimSpace(target),
		})
	}
	return refs
}

// groupAt returns capture group n of a FindAllStringSubmatchIndex match.
func groupAt(s string, match []int, n int) string {
	start, end := match[2*n], match[2*n+1]
	if start < 0 || end < 0 || start > end {
		return ""
	}
	return s[start:end]
}

// escapedOrCode reports whether the match at index start is escaped with a
// backslash, sits inside an inline code span, or is the image inside a link
// (`[![alt](img)](url)`), which must stay part of the link markup.
func escapedOrCode(line string, start int) bool {
	if start > 0 && (line[start-1] == '\\' || line[start-1] == '[') {
		return true
	}
	return strings.Count(line[:start], "`")%2 == 1
}

// overlapsAny reports whether [start,end) overlaps any reference in refs.
func overlapsAny(refs []markdownImageRef, start, end int) bool {
	for _, ref := range refs {
		if start < ref.end && ref.start < end {
			return true
		}
	}
	return false
}

// dropOverlaps removes later references that overlap an earlier one and, in
// particular, keeps an image nested in a link from being lifted out.
func dropOverlaps(refs []markdownImageRef) []markdownImageRef {
	kept := refs[:0]
	previousEnd := -1
	for _, ref := range refs {
		if ref.start < previousEnd {
			continue
		}
		kept = append(kept, ref)
		previousEnd = ref.end
	}
	return kept
}

// normalizeImageID follows CommonMark's reference-label normalization.
func normalizeImageID(id string) string {
	return strings.Join(strings.Fields(strings.ToLower(id)), " ")
}

// isFenceLine reports whether line opens or closes a fenced code block.
func isFenceLine(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

// firstRegexGroup returns the first non-empty capture group of re in s.
func firstRegexGroup(re *regexp.Regexp, s string) (string, bool) {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	for _, group := range m[1:] {
		if group != "" {
			return group, true
		}
	}
	return "", true
}

// inlineImagesWillRender reports whether the document will display at least one
// image as a terminal escape sequence.
func inlineImagesWillRender(src string) bool {
	return inlinePreviewAvailable() && len(markdownImageRefs(src)) > 0
}

// renderMarkdownWithImages renders the document in place, substituting inline
// images and their alt-text captions for the image references. It streams to
// stdout because the image escapes must be written by the terminal, so long
// documents with images are not paged.
func renderMarkdownWithImages(src, baseDir string, width int) {
	cursor := 0
	printText := func(part string) {
		if text := markdownText(part, width); text != "" {
			ui.Println(text)
		}
	}
	for _, ref := range markdownImageRefs(src) {
		if ref.start > cursor {
			printText(src[cursor:ref.start])
		}
		cursor = ref.end
		if !previewMarkdownImage(ref, baseDir) {
			printText(imageMarkdown(ref))
		}
	}
	if cursor < len(src) {
		printText(src[cursor:])
	}
}

// previewMarkdownImage shows one referenced image inline. It reports whether
// the image was displayed; false lets the caller keep the reference as text.
func previewMarkdownImage(ref markdownImageRef, baseDir string) bool {
	if !inlinePreviewAvailable() {
		return false
	}
	path, cleanup, ok := resolveMarkdownImage(ref.target, baseDir)
	if !ok {
		return false
	}
	if cleanup != nil {
		defer cleanup()
	}
	if !inlinePreviewFile(path) {
		return false
	}
	// Caption is the image alt text only; images without alt text get none.
	if caption := strings.TrimSpace(ref.alt); caption != "" {
		ui.Printf("  %s\n", caption)
	}
	return true
}

// resolveMarkdownImage turns a Markdown image target into a local file path.
// Remote and data: targets are fetched into a temporary file and cleaned up by
// the returned cleanup function.
func resolveMarkdownImage(target, baseDir string) (string, func(), bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", nil, false
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") ||
		strings.HasPrefix(target, "data:") {
		return downloadMarkdownImage(target)
	}
	path, ok := localMarkdownImagePath(target, baseDir)
	if !ok {
		return "", nil, false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", nil, false
	}
	return path, nil, true
}

// localMarkdownImagePath resolves a relative, absolute or file:// target
// against the document's directory.
func localMarkdownImagePath(target, baseDir string) (string, bool) {
	if filepath.IsAbs(target) {
		return filepath.Clean(target), true
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return "", false
	}
	if parsed.Scheme != "" {
		if parsed.Scheme != "file" {
			return "", false
		}
		path := parsed.Path
		if runtime.GOOS == "windows" {
			path = strings.TrimPrefix(path, "/")
		}
		return filepath.Clean(path), true
	}
	path := parsed.Path
	if path == "" {
		path = target
	}
	return filepath.Join(baseDir, path), true
}

// downloadMarkdownImage fetches a remote or data: image into a temp file.
func downloadMarkdownImage(target string) (string, func(), bool) {
	data, err := service.FetchBytes(target)
	if err != nil || len(data) == 0 {
		return "", nil, false
	}
	ext := imgcodec.SniffImageExt(data)
	if ext == "" {
		return "", nil, false
	}
	file, err := os.CreateTemp("", "aigc-cli-md-image-*"+ext)
	if err != nil {
		return "", nil, false
	}
	name := file.Name()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return "", nil, false
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", nil, false
	}
	return name, func() { _ = os.Remove(name) }, true
}

// imageMarkdown rebuilds the reference source for the text fallback.
func imageMarkdown(ref markdownImageRef) string {
	target := ref.target
	if strings.ContainsAny(target, " ()") {
		target = "<" + target + ">"
	}
	return "![" + ref.alt + "](" + target + ")"
}
