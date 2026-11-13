package ocr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// scanWithDeepSeek recognises one document (PDF, Word, PowerPoint or image)
// through DeepSeek's POST /v1/tools/ocr endpoint. The server performs layout
// analysis and returns per-page text, so no local engine selection happens.
//
// The only API parameter is `purpose` (--purpose); the OpenAI-style prompt,
// model, language and page-range flags do not apply.
func scanWithDeepSeek(cmd *cobra.Command, p *provider.EffectiveProvider, inputPath string) error {
	warnDeepSeekIgnoredFlags(cmd)

	purpose, err := provider.NormalizeDeepSeekOCRPurpose(ocrScanPurpose)
	if err != nil {
		return err
	}
	result, err := provider.DeepSeekOCR(p, inputPath, purpose)
	if err != nil {
		return fmt.Errorf("DeepSeek OCR failed: %w", err)
	}

	if options.Shared.Verbose {
		ui.Eprintf("Recognized %d page(s) with purpose %s (id %s)\n",
			result.PageCount, result.Purpose, result.ID)
		if result.Usage.TotalTokens > 0 {
			ui.Eprintf("Tokens: %d in / %d out\n",
				result.Usage.InputTokens, result.Usage.OutputTokens)
		}
	}
	for _, page := range result.Pages {
		if page.FinishReason == "length" {
			ui.Eprintf("⚠ page %d hit the output limit; its text is incomplete\n", page.Index+1)
		}
	}

	text := result.Text()
	if strings.TrimSpace(text) == "" {
		text = "(no text detected)"
	}

	if writesToStdout(options.Shared.OutputDir) {
		printText(text)
		return nil
	}

	outPath := strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ".md"
	if err := os.WriteFile(outPath, []byte(text+"\n"), 0644); err != nil {
		return fmt.Errorf("save output: %w", err)
	}
	ui.Eprintf("Saved: %s\n", outPath)

	if ocrScanPreview {
		printText(text)
	}
	return nil
}

// flagsIgnoredByDeepSeek lists ocr scan flags that POST /v1/tools/ocr does not
// accept. Explicit use is reported instead of being silently ignored.
var flagsIgnoredByDeepSeek = []string{
	"prompt", "ask", "model", "pages", "engine", "lang", "spellcheck", "json", "inject-json",
}

func warnDeepSeekIgnoredFlags(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	for _, name := range flagsIgnoredByDeepSeek {
		flag := cmd.Flags().Lookup(name)
		if flag != nil && flag.Changed {
			ui.Eprintf("⚠ --%s has no effect on DeepSeek OCR\n", name)
		}
	}
}
