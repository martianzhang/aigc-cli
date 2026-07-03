package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/service"
)

// detectCmd represents the `apimart-cli detect` command.
var detectCmd = &cobra.Command{
	Use:          "detect <file...>",
	Short:        "Detect watermarks and metadata in images",
	SilenceUsage: true,
	Long: `Detect watermarks and metadata in image files.

Scans images for C2PA Content Credentials, TC260 AIGC labels
(China GB 45438-2025), and SynthID invisible watermarks.
All metadata including file stats, dimensions, and embedded text
chunks is shown by default.

Supports PNG, JPEG, WebP, GIF, and BMP formats.

Examples:
  apimart-cli detect image.png
  apimart-cli detect --json image.png
  apimart-cli detect *.png
  cat image.png | apimart-cli detect`,
	RunE: runDetect,
}

var detectJSON bool

func runDetect(cmd *cobra.Command, args []string) error {
	// If files are passed as arguments, detect them
	if len(args) > 0 {
		return detectFiles(args, "")
	}

	// Check if stdin is piped
	stat, err := os.Stdin.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) != 0 {
		return fmt.Errorf("no files specified: pass file paths as arguments or pipe file data to stdin")
	}

	// Read from stdin and write to a temp file, then detect
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("no data read from stdin")
	}

	tmpFile, err := os.CreateTemp("", "apimart-detect-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	// Override path display to "(stdin)" for temp files from pipe
	return detectFiles([]string{tmpFile.Name()}, "(stdin)")
}

func detectFiles(paths []string, pathOverride string) error {
	if detectJSON {
		return detectFilesJSON(paths, pathOverride)
	}

	for _, path := range paths {
		result, err := service.DetectImage(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}
		if pathOverride != "" {
			result.Path = pathOverride
		}
		if err := service.PrintDetectResult(os.Stdout, result, true); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
	return nil
}

func detectFilesJSON(paths []string, pathOverride string) error {
	var results []*service.DetectResult
	for _, path := range paths {
		result, err := service.DetectImage(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}
		if pathOverride != "" {
			result.Path = pathOverride
		}
		results = append(results, result)
	}

	if len(results) == 0 {
		return nil
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if len(results) == 1 {
		return enc.Encode(results[0])
	}
	return enc.Encode(results)
}

func init() {
	rootCmd.AddCommand(detectCmd)
	detectCmd.Flags().BoolVar(&detectJSON, "json", false, "output results as JSON")
}
