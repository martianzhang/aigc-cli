// Package preview implements the `aigc-cli preview` command.
package preview

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// Deps carries the runtime callbacks the preview command needs.
type Deps struct {
	ReadInput func(string) ([]byte, error)
}

// commandOptions collects the preview flags so run takes one value instead of a
// growing parameter list.
type commandOptions struct {
	detail   bool
	describe string
	raw      bool
	noPager  bool
}

// NewCommand builds the `preview` command. deps is resolved at run time.
func NewCommand(deps func() Deps) *cobra.Command {
	var opts commandOptions

	cmd := &cobra.Command{
		Use:          "preview <file...>",
		Aliases:      []string{"pr"},
		Short:        "Preview images, videos, Markdown and documents (also: pr, --detail for metadata, --describe to set caption)",
		SilenceUsage: true,
		Long: `Preview images, videos, Markdown and text files.

Images and videos open in the system default application. For image files, also
attempts inline terminal display when the terminal is supported (iTerm2/Kitty or
Sixel) and stdout is a real terminal. The inline payload is a preview-sized copy,
and images that cannot be shrunk small enough are handed to the system viewer
instead. Set AIGC_CLI_NO_INLINE_PREVIEW or pass --no-inline-preview to skip
inline rendering entirely.

Markdown files (.md/.markdown) are rendered with the same renderer as the chat
TUI when stdout is a real terminal, and the images they reference are inlined on
terminals that support Kitty/Sixel. Plain text files (.txt/.text/.log) are shown
as-is. EPUB, DOCX and local HTML documents are converted to Markdown first
(images inside them are not rendered).

PDFs are only shown as text when Markdown can represent their layout. Simple
text-layer PDFs are converted to Markdown; multi-column, rotated, partly scanned,
garbled or otherwise unrepresentable PDFs are opened with the system default
application instead. Output taller than the terminal is paged (like bat): $PAGER
when set, then less -R, then more. Pass --no-pager to print directly. Piped or
redirected output keeps Markdown source, and --raw forces it even in a terminal.

Examples:
  aigc-cli preview image_12345_0.png
  aigc-cli preview video_67890_0.mp4
  aigc-cli preview ocr_notes.md
  aigc-cli preview notes.txt
  aigc-cli preview book.epub
  aigc-cli preview paper.pdf
  aigc-cli preview *.png
  cat image.png | aigc-cli preview`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(deps(), args, opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.detail, "detail", "d", false, "show image details (C2PA, TC260, caption)")
	cmd.Flags().StringVar(&opts.describe, "describe", "", "write caption to image (reads from file if path exists, else uses as text)")
	cmd.Flags().BoolVar(&opts.raw, "raw", false, "print Markdown/text source without styling")
	cmd.Flags().BoolVar(&opts.noPager, "no-pager", false, "print directly instead of paging long output")
	return cmd
}

func run(d Deps, args []string, opts commandOptions) error {
	if len(args) > 0 {
		for _, arg := range args {
			if opts.describe != "" {
				caption, err := d.ReadInput(opts.describe)
				if err != nil {
					ui.Eprintf("Error reading caption: %v\n", err)
					continue
				}
				if err := service.WriteDescription(arg, string(caption)); err != nil {
					ui.Eprintf("Error writing caption to %s: %v\n", arg, err)
					continue
				}
			}
			if opts.detail {
				showDetail(arg)
			}
			if opts.describe == "" {
				switch {
				case isMarkdownPath(arg):
					if err := previewMarkdown(arg, opts.raw, opts.noPager); err != nil {
						ui.Eprintf("Warning: %v\n", err)
					}
				case isTextPath(arg):
					if err := previewText(arg, opts.noPager); err != nil {
						ui.Eprintf("Warning: %v\n", err)
					}
				case isDocumentPath(arg):
					if err := previewDocument(arg, opts); err != nil {
						ui.Eprintf("Warning: %v\n", err)
					}
				default:
					if err := service.PreviewFile(arg); err != nil {
						ui.Eprintf("Warning: %v\n", err)
					}
				}
			}
		}
		return nil
	}

	if opts.describe != "" {
		return fmt.Errorf("--describe requires a file path, not stdin")
	}

	stat, err := os.Stdin.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) != 0 {
		return fmt.Errorf("no files specified: pass file paths as arguments or pipe file data to stdin")
	}

	// stdin is user-supplied image/Markdown; imgcodec applies a pixel cap.
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("no data read from stdin")
	}

	tmpFile, err := os.CreateTemp("", "aigc-cli-preview-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	if opts.detail {
		showDetail(tmpFile.Name())
	}
	return service.PreviewFile(tmpFile.Name())
}

// showDetail prints the detect result and caption for an image file.
func showDetail(path string) {
	result, err := service.DetectImage(path)
	if err != nil {
		ui.Eprintf("  Detect error: %v\n", err)
		return
	}

	ui.Printf("\n%s %s %s\n", strings.Repeat("━", 3), filepath.Base(path), strings.Repeat("━", 3))
	ui.Printf("  Size:      %s", result.SizeHuman)
	if result.Width > 0 && result.Height > 0 {
		ui.Printf("  %dx%d", result.Width, result.Height)
	}
	ui.Println()
	ui.Printf("  Format:    %s\n", result.Format)

	if result.C2PA != nil && result.C2PA.Present {
		line := fmt.Sprintf("  C2PA:      %s", result.C2PA.Vendor)
		if result.C2PA.Source != "" {
			line += " / " + result.C2PA.Source
		}
		ui.Println(line)
	}
	if result.TC260 != nil && result.TC260.Present {
		line := "  TC260:     " + result.TC260.Provider
		if result.TC260.Data != "" {
			line += " / " + result.TC260.Data
		}
		ui.Println(line)
	}
	if desc, err := service.ReadDescription(path); err == nil && desc != "" {
		desc = strings.ReplaceAll(desc, "\n", "\n               ")
		ui.Printf("  Caption:    %s\n", desc)
	}
	ui.Println()
}
