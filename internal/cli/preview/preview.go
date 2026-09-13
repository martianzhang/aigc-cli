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
)

// Deps carries the runtime callbacks the preview command needs.
type Deps struct {
	ReadInput func(string) ([]byte, error)
}

// NewCommand builds the `preview` command. deps is resolved at run time.
func NewCommand(deps func() Deps) *cobra.Command {
	var detail bool
	var describe string

	cmd := &cobra.Command{
		Use:          "preview <file...>",
		Aliases:      []string{"pr"},
		Short:        "Preview images and videos (also: pr, --detail for metadata, --describe to set caption)",
		SilenceUsage: true,
		Long: `Preview images and videos by opening them with the system default application.

For image files, also attempts inline terminal display when using a
supported terminal (iTerm2, Kitty).

Examples:
  aigc-cli preview image_12345_0.png
  aigc-cli preview video_67890_0.mp4
  aigc-cli preview *.png
  cat image.png | aigc-cli preview`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(deps(), args, detail, describe)
		},
	}

	cmd.Flags().BoolVarP(&detail, "detail", "d", false, "show image details (C2PA, TC260, caption)")
	cmd.Flags().StringVar(&describe, "describe", "", "write caption to image (reads from file if path exists, else uses as text)")
	return cmd
}

func run(d Deps, args []string, detail bool, describe string) error {
	if len(args) > 0 {
		for _, arg := range args {
			if describe != "" {
				caption, err := d.ReadInput(describe)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error reading caption: %v\n", err)
					continue
				}
				if err := service.WriteDescription(arg, string(caption)); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing caption to %s: %v\n", arg, err)
					continue
				}
			}
			if detail {
				showDetail(arg)
			}
			if describe == "" {
				if strings.HasSuffix(strings.ToLower(arg), ".md") {
					if err := previewMarkdown(arg); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
					}
				} else {
					if err := service.PreviewFile(arg); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
					}
				}
			}
		}
		return nil
	}

	if describe != "" {
		return fmt.Errorf("--describe requires a file path, not stdin")
	}

	stat, err := os.Stdin.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) != 0 {
		return fmt.Errorf("no files specified: pass file paths as arguments or pipe file data to stdin")
	}

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
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	if detail {
		showDetail(tmpFile.Name())
	}
	return service.PreviewFile(tmpFile.Name())
}

func previewMarkdown(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	os.Stdout.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		fmt.Println()
	}
	return nil
}

// showDetail prints the detect result and caption for an image file.
func showDetail(path string) {
	result, err := service.DetectImage(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Detect error: %v\n", err)
		return
	}

	fmt.Printf("\n%s %s %s\n", strings.Repeat("━", 3), filepath.Base(path), strings.Repeat("━", 3))
	fmt.Printf("  Size:      %s", result.SizeHuman)
	if result.Width > 0 && result.Height > 0 {
		fmt.Printf("  %dx%d", result.Width, result.Height)
	}
	fmt.Println()
	fmt.Printf("  Format:    %s\n", result.Format)

	if result.C2PA != nil && result.C2PA.Present {
		line := fmt.Sprintf("  C2PA:      %s", result.C2PA.Vendor)
		if result.C2PA.Source != "" {
			line += " / " + result.C2PA.Source
		}
		fmt.Println(line)
	}
	if result.TC260 != nil && result.TC260.Present {
		line := "  TC260:     " + result.TC260.Provider
		if result.TC260.Data != "" {
			line += " / " + result.TC260.Data
		}
		fmt.Println(line)
	}
	if desc, err := service.ReadDescription(path); err == nil && desc != "" {
		desc = strings.ReplaceAll(desc, "\n", "\n               ")
		fmt.Printf("  Caption:    %s\n", desc)
	}
	fmt.Println()
}
