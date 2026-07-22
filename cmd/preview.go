package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/service"
)

var previewDetail bool
var previewDescribe string

// previewSavedFiles accumulates file paths saved during image/video generation
// for use by the --preview flag.
var previewSavedFiles []string

// previewLatestFiles finds the most recently modified files in the output
// directory whose names match the given prefix. Used after image/video
// generation to locate just-saved files for auto-preview.
func previewLatestFiles(prefix string) []string {
	entries, err := os.ReadDir(shared.OutputDir)
	if err != nil {
		return nil
	}
	type entry struct {
		name string
		time time.Time
	}
	var matched []entry
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		matched = append(matched, entry{
			name: filepath.Join(shared.OutputDir, e.Name()),
			time: info.ModTime(),
		})
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].time.After(matched[j].time)
	})
	if len(matched) > 3 {
		matched = matched[:3]
	}
	var paths []string
	for _, m := range matched {
		paths = append(paths, m.name)
	}
	return paths
}

// previewCmd represents the `aigc-cli preview` command.
var previewCmd = &cobra.Command{
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
	RunE: runPreview,
}

func runPreview(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		for _, arg := range args {
			if previewDescribe != "" {
				caption, err := readInput(previewDescribe)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error reading caption: %v\n", err)
					continue
				}
				if err := service.WriteDescription(arg, string(caption)); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing caption to %s: %v\n", arg, err)
					continue
				}
			}
			if previewDetail {
				showDetail(arg)
			}
			if previewDescribe == "" {
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

	if previewDescribe != "" {
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

	if previewDetail {
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

func init() {
	rootCmd.AddCommand(previewCmd)
	previewCmd.Flags().BoolVarP(&previewDetail, "detail", "d", false, "show image details (C2PA, TC260, caption)")
	previewCmd.Flags().StringVar(&previewDescribe, "describe", "", "write caption to image (reads from file if path exists, else uses as text)")
}
