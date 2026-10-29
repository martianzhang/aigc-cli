package ocr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// scanWithSomark parses one document (PDF or image) through the SoMark / OXR
// /parse/sync API and writes the Markdown result. The server handles layout
// analysis, reading order, tables and formulas, so no local text extraction or
// engine selection happens here.
//
// API parameters come from the shared --json / --inject-json flags, exactly
// like every other provider request: no provider-specific flags are added.
func scanWithSomark(cmd *cobra.Command, p *provider.EffectiveProvider, inputPath string) error {
	warnIgnoredFlags(cmd)

	payload, inject, err := somarkRequestBody()
	if err != nil {
		return err
	}
	result, err := provider.ParseDocument(p, inputPath, payload, inject)
	if err != nil {
		return fmt.Errorf("document parse failed: %w", err)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(os.Stderr, "⚠ %s\n", warning)
	}
	if options.Shared.Verbose {
		timing := ""
		if result.ProcessingTimeMS > 0 {
			timing = fmt.Sprintf(" in %dms", result.ProcessingTimeMS)
		}
		fmt.Fprintf(os.Stderr, "Parsed %d page(s) %s%s (task %s)\n",
			result.PageNum, result.FileType, timing, result.TaskID)
	}

	text := result.Markdown

	// A zip export contains the Markdown plus its images, so it replaces the
	// per-file output: download the archive instead of writing a bare .md.
	if result.ZipURL != "" {
		base := strings.TrimSuffix(inputPath, filepath.Ext(inputPath))
		if writesToStdout(options.Shared.OutputDir) {
			printText(result.ZipURL)
			return nil
		}
		zipPath := base + ".zip"
		if err := service.SaveResource(result.ZipURL, zipPath); err != nil {
			return fmt.Errorf("download zip: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Saved: %s\n", zipPath)
		return nil
	}

	if strings.TrimSpace(text) == "" {
		text = "(no text detected)"
	}

	if writesToStdout(options.Shared.OutputDir) {
		if len(result.Markdown) > 0 {
			fmt.Fprintln(os.Stderr, "⚠ -o - keeps remote image URLs; images are not downloaded")
		}
		printText(text)
		return nil
	}

	// SoMark deletes parse-time image resources after 30 days, so pull the
	// images next to the Markdown and rewrite the links to relative paths.
	base := strings.TrimSuffix(inputPath, filepath.Ext(inputPath))
	assetDir := base + ".assets"
	text, images := service.LocalizeMarkdownImages(text, assetDir, filepath.Base(assetDir))
	for _, failure := range images.Failed {
		fmt.Fprintf(os.Stderr, "⚠ image not saved (link kept remote): %s\n", failure)
	}
	if len(images.Saved) > 0 {
		fmt.Fprintf(os.Stderr, "Saved: %d image(s) in %s\n", len(images.Saved), filepath.Base(assetDir))
	}

	outPath := base + ".md"
	if err := os.WriteFile(outPath, []byte(text+"\n"), 0644); err != nil {
		return fmt.Errorf("save output: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Saved: %s\n", outPath)

	if ocrScanPreview {
		printText(text)
	}
	return nil
}

// somarkRequestBody resolves the shared --json / --inject-json values into a
// request body for /parse/sync. Neither flag reads stdin: the project
// convention is an inline JSON string or a file containing it.
func somarkRequestBody() (json.RawMessage, json.RawMessage, error) {
	var payload json.RawMessage
	if options.Shared.JSONInput != "" {
		if options.Shared.JSONInput == "-" {
			return nil, nil, fmt.Errorf("--json does not read stdin; pass an inline JSON string or a file path")
		}
		data, err := service.ReadJSONInput(options.Shared.JSONInput)
		if err != nil {
			if !service.IsFile(options.Shared.JSONInput) {
				return nil, nil, fmt.Errorf("read --json: %s is neither an inline JSON value nor an existing file (stdin is not supported here)", options.Shared.JSONInput)
			}
			return nil, nil, fmt.Errorf("read --json: %w", err)
		}
		payload = data
	}
	inject, err := service.ReadInjectJSON(options.Shared.InjectJSON)
	if err != nil {
		return nil, nil, err
	}
	return payload, inject, nil
}

// flagsIgnoredBySomark lists ocr scan flags that have no meaning for a
// document-parsing provider, so an explicit use is reported instead of
// silently doing nothing.
var flagsIgnoredBySomark = []string{"prompt", "ask", "model", "pages", "engine", "lang", "spellcheck"}

// warnIgnoredFlags reports flags the user explicitly set that this provider
// cannot honor. A nil cmd (unit tests) skips the check.
func warnIgnoredFlags(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	for _, name := range flagsIgnoredBySomark {
		flag := cmd.Flags().Lookup(name)
		if flag != nil && flag.Changed {
			fmt.Fprintf(os.Stderr, "⚠ --%s has no effect on SoMark/OXR document parsing\n", name)
		}
	}
}
