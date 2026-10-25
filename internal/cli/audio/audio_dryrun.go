package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
)

// localDryRunField is one aligned "# key: value" line of a local dry-run plan.
type localDryRunField struct {
	key   string
	value string
}

// printLocalDryRun writes the plan for a local ONNX run. Local inference has
// no equivalent curl, so --dry-run prints the resolved parameters instead of
// loading a model or writing a file.
func printLocalDryRun(command string, fields []localDryRunField) {
	fmt.Printf("# %s dry run (local ONNX)\n", command)
	for _, f := range fields {
		fmt.Printf("# %-9s %s\n", f.key+":", f.value)
	}
	fmt.Println("# no model loaded, no file written")
}

// outputDir returns the directory local outputs are written to.
func outputDir() string {
	if options.Shared.OutputDir != "" {
		return options.Shared.OutputDir
	}
	return "."
}

// plannedAudioOutput previews the path saveAudioFile would use. The timestamp
// is only known at write time, so the pattern is shown instead.
func plannedAudioOutput(ext string) string {
	return filepath.Join(outputDir(), "audio_<timestamp>."+ext)
}

// plannedTranscriptionOutput mirrors saveTranscriptionFile's naming without
// creating anything: <basename>.md when free, else a timestamped name.
func plannedTranscriptionOutput(inputPath string) string {
	dir := outputDir()
	if inputPath != "" {
		if fi, err := os.Stat(inputPath); err == nil && !fi.IsDir() {
			base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
			candidate := filepath.Join(dir, base+".md")
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				return candidate
			}
		}
	}
	return filepath.Join(dir, "audio_<timestamp>.md")
}
