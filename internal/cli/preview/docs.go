package preview

import (
	"errors"

	"github.com/martianzhang/aigc-cli/internal/docconvert"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// Hooks keep document conversion and the system-viewer fallback testable.
var (
	documentMarkdown  = docconvert.Markdown
	previewSystemFile = service.PreviewFile
)

// isDocumentPath reports whether path names a document that is converted to
// Markdown for terminal preview. URLs are handled by the system viewer path.
func isDocumentPath(path string) bool {
	if isHTTPPath(path) {
		return false
	}
	return docconvert.Supported(path)
}

// previewDocument converts a document to Markdown and shows it through the same
// renderer and pager as Markdown files. Content that Markdown cannot represent
// (scanned or structurally unusual PDFs) opens in the system default
// application immediately instead of showing a misleading preview.
func previewDocument(path string, opts commandOptions) error {
	markdown, err := documentMarkdown(path)
	if err != nil {
		if errors.Is(err, docconvert.ErrNoTextLayer) || errors.Is(err, docconvert.ErrComplexLayout) {
			return openDocumentWithSystemViewer(path, err)
		}
		return err
	}

	if !markdownRenderAllowed(opts.raw) {
		writePagedText(markdown, opts.noPager)
		return nil
	}
	writePagedText(markdownDocumentText(markdown, markdownWrapWidth()), opts.noPager)
	return nil
}

// openDocumentWithSystemViewer hands an unrepresentable document to the
// system default application. A non-terminal caller gets the reason instead, so
// scripts never spawn a GUI.
func openDocumentWithSystemViewer(path string, cause error) error {
	if !stdoutIsTerminal() {
		return cause
	}
	ui.Eprintf("Warning: %v\n", cause)
	ui.Eprintln("Opening it with the system viewer.")
	return previewSystemFile(path)
}
