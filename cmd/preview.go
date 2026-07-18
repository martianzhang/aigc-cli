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
	Short:        "Preview images and videos (also: pr)",
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
	// If files are passed as arguments, preview them
	if len(args) > 0 {
		for _, arg := range args {
			if err := service.PreviewFile(arg); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
		}
		return nil
	}

	// Check if stdin is piped
	stat, err := os.Stdin.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) != 0 {
		return fmt.Errorf("no files specified: pass file paths as arguments or pipe file data to stdin")
	}

	// Read from stdin and write to a temp file, then preview
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

	return service.PreviewFile(tmpFile.Name())
}

func init() {
	rootCmd.AddCommand(previewCmd)
}
