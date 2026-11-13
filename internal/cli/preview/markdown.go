package preview

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
	"golang.org/x/term"

	"github.com/martianzhang/aigc-cli/internal/mdrender"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

const (
	// fallbackMarkdownWidth is the wrap width used when the terminal size is
	// unknown. It matches the conservative 80x24 grid used by inline previews.
	fallbackMarkdownWidth = 80
	// minMarkdownWidth keeps very narrow terminals readable.
	minMarkdownWidth = 20
)

// markdownExtensions are the file extensions rendered as Markdown. The OCR and
// knowledge commands write .md/.markdown, so both must be covered.
var markdownExtensions = []string{".md", ".markdown", ".mdown"}

// textExtensions are plain-text files shown in the terminal (and paged) instead
// of being handed to the system viewer.
var textExtensions = []string{".txt", ".text", ".log"}

// isMarkdownPath reports whether path names a local Markdown file. URLs are
// excluded: they are downloaded and handed to the system viewer elsewhere.
func isMarkdownPath(path string) bool {
	if isHTTPPath(path) {
		return false
	}
	return hasExtension(path, markdownExtensions)
}

// isTextPath reports whether path names a local plain-text file.
func isTextPath(path string) bool {
	if isHTTPPath(path) {
		return false
	}
	return hasExtension(path, textExtensions)
}

func isHTTPPath(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

func hasExtension(path string, extensions []string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, candidate := range extensions {
		if ext == candidate {
			return true
		}
	}
	return false
}

// markdownRenderAllowed reports whether the Markdown source may be styled.
// Styling needs a real terminal: piped or redirected output stays byte-for-byte
// source text so `preview notes.md > notes.txt` and `| grep` keep working. It is
// a variable so tests can exercise the rendering path without a pty.
var markdownRenderAllowed = func(raw bool) bool {
	if raw {
		return false
	}
	return stdoutIsTerminal()
}

// stdoutIsTerminal reports whether the current ui.Out() writer is a terminal.
// The writer is inspected instead of os.Stdout so tests and MCP output
// redirection are handled consistently. It is a variable so tests can simulate
// a terminal without a pty.
var stdoutIsTerminal = func() bool {
	file, ok := ui.Out().(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

// markdownWrapWidth returns the wrap width for the current terminal.
func markdownWrapWidth() int {
	file, ok := ui.Out().(*os.File)
	if !ok {
		return fallbackMarkdownWidth
	}
	cols, _, err := term.GetSize(int(file.Fd()))
	if err != nil || cols <= 0 {
		return fallbackMarkdownWidth
	}
	return max(cols, minMarkdownWidth)
}

// previewMarkdown writes a Markdown file to stdout. A real terminal gets the
// shared TUI rendering (with referenced images inlined where supported); pipes,
// redirects and --raw get the source byte-for-byte. Long output is paged.
func previewMarkdown(path string, raw bool, noPager bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	if !markdownRenderAllowed(raw) {
		writePagedText(string(data), noPager)
		return nil
	}

	src := string(data)
	width := markdownWrapWidth()
	if inlineImagesWillRender(src) {
		// Inline images are terminal escape sequences written in place; a pager
		// would break where they land, so stream the document instead.
		renderMarkdownWithImages(src, filepath.Dir(path), width)
		return nil
	}
	writePagedText(markdownDocumentText(src, width), noPager)
	return nil
}

// previewText writes a plain-text file to stdout, paging it when it does not
// fit on the terminal.
func previewText(path string, noPager bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	writePagedText(string(data), noPager)
	return nil
}

// markdownDocumentText renders a document to a single string. Image references
// stay as text: their inline rendering needs a live terminal writer.
func markdownDocumentText(src string, width int) string {
	var b strings.Builder
	writeText := func(part string) {
		text := markdownText(part, width)
		if text == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(text)
	}

	cursor := 0
	for _, ref := range markdownImageRefs(src) {
		if ref.start > cursor {
			writeText(src[cursor:ref.start])
		}
		cursor = ref.end
		writeText(imageMarkdown(ref))
	}
	if cursor < len(src) {
		writeText(src[cursor:])
	}
	return b.String()
}

// markdownText renders one Markdown segment at the given width. NO_COLOR keeps
// the layout but drops the escape sequences.
func markdownText(src string, width int) string {
	rendered := mdrender.Render(src, width)
	if rendered == "" {
		return ""
	}
	wrapped, _ := mdrender.WrapANSI(rendered, width)
	if os.Getenv("NO_COLOR") != "" {
		wrapped = xansi.Strip(wrapped)
	}
	if strings.TrimSpace(wrapped) == "" {
		return ""
	}
	return wrapped
}
